using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using Microsoft.Win32.SafeHandles;

namespace Artemis.Core;

public sealed record ChangeProposal(string Path, string? Before, string After, string? Hash);
public sealed record SearchHit(string Path, int Line, string Text);

// All filesystem access is local. Directories are held without delete sharing while operating.
public sealed class FileTools
{
    public const int MaxBytes = 1024 * 1024;
    private static readonly UTF8Encoding Utf8 = new(false, true);
    private static readonly HashSet<string> TextExtensions = new(StringComparer.OrdinalIgnoreCase)
    { ".txt", ".md", ".cs", ".xaml", ".csproj", ".props", ".targets", ".sln", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".config", ".html", ".css", ".js", ".jsx", ".ts", ".tsx", ".py", ".rs", ".go", ".c", ".h", ".cpp", ".sql", ".csv", ".gitignore", ".editorconfig" };
    public string Root { get; }

    public FileTools(string root)
    {
        Root = System.IO.Path.TrimEndingDirectorySeparator(System.IO.Path.GetFullPath(root));
        if (Root.StartsWith(@"\\") || new DriveInfo(System.IO.Path.GetPathRoot(Root)!).DriveType == DriveType.Network)
            throw new UnauthorizedAccessException("로컬 프로젝트 폴더만 허용됩니다.");
        for (var current = Root; Directory.GetParent(current) != null; current = Directory.GetParent(current)!.FullName)
            CheckName(System.IO.Path.GetFileName(current));
        if (!Directory.Exists(Root)) throw new IOException("프로젝트 폴더가 없습니다.");
        using var lease = Lease(Root);
    }

    private static void CheckName(string name)
    {
        var lower = name.ToLowerInvariant();
        if (lower is ".git" or ".ssh" or ".aws" or ".azure" or ".gnupg" or ".codex" or ".artemis" ||
            lower.StartsWith(".env") || lower.Contains("credential") || lower.Contains("secret") ||
            lower.Contains("token") || lower.StartsWith("id_rsa") || lower.StartsWith("id_ed25519") ||
            lower is ".npmrc" or ".pypirc" or ".netrc" or "auth.json" ||
            new[] { ".pem", ".key", ".pfx", ".p12", ".kdbx", ".jks" }.Any(lower.EndsWith))
            throw new UnauthorizedAccessException("제외된 비밀·설정 경로입니다.");
    }

    public string Resolve(string relative, bool text = false)
    {
        if (string.IsNullOrWhiteSpace(relative) || System.IO.Path.IsPathRooted(relative) || relative.Contains(':'))
            throw new UnauthorizedAccessException("프로젝트 상대 경로만 허용됩니다.");
        var parts = relative.Replace('\\', '/').Split('/', StringSplitOptions.RemoveEmptyEntries);
        foreach (var part in parts)
        {
            var stem = part.Split('.')[0].ToUpperInvariant();
            if (stem is "CON" or "PRN" or "AUX" or "NUL" ||
                (stem.Length == 4 && (stem.StartsWith("COM") || stem.StartsWith("LPT")) && char.IsDigit(stem[3])))
                throw new UnauthorizedAccessException("장치 경로는 허용되지 않습니다.");
            if (part == ".." || part.EndsWith(' ') || part.EndsWith('.'))
            {
                if (part != ".") throw new UnauthorizedAccessException("모호한 경로는 허용되지 않습니다.");
            }
            CheckName(part);
        }
        var full = System.IO.Path.GetFullPath(relative, Root);
        if (!full.Equals(Root, StringComparison.OrdinalIgnoreCase) &&
            !full.StartsWith(System.IO.Path.EndsInDirectorySeparator(Root) ? Root : Root + System.IO.Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase))
            throw new UnauthorizedAccessException("프로젝트 밖의 경로입니다.");
        if (text && !TextExtensions.Contains(System.IO.Path.GetExtension(full)) &&
            !TextExtensions.Contains(System.IO.Path.GetFileName(full)))
            throw new UnauthorizedAccessException("허용된 텍스트 확장자가 아닙니다.");
        return full;
    }

    private static IDisposable Lease(string directory)
    {
        var handles = new List<SafeFileHandle>();
        try
        {
            var ancestors = new Stack<string>();
            for (var current = directory; current != null; current = Directory.GetParent(current)?.FullName)
                ancestors.Push(current);
            foreach (var path in ancestors)
            {
                var handle = CreateFile(path, 0, 3, IntPtr.Zero, 3, 0x02200000, IntPtr.Zero);
                if (handle.IsInvalid) { handle.Dispose(); throw new Win32Exception(Marshal.GetLastWin32Error()); }
                handles.Add(handle);
                if ((File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
                    throw new UnauthorizedAccessException("링크·junction 경로는 허용되지 않습니다.");
                // Resolve Windows short-name aliases before applying secret-name exclusions.
                CheckName(System.IO.Path.GetFileName(CanonicalPath(handle)));
            }
            return new HandleLease(handles);
        }
        catch { foreach (var h in handles) h.Dispose(); throw; }
    }

    private FileStream Open(string full, FileAccess access, FileMode mode = FileMode.Open)
    {
        var handle = CreateFile(full, access == FileAccess.Read ? 0x80000000u : 0xC0000000u,
            access == FileAccess.Read ? 1u : 0u, IntPtr.Zero, mode == FileMode.CreateNew ? 1u : 3u,
            0x00200000, IntPtr.Zero);
        if (handle.IsInvalid) { handle.Dispose(); throw new Win32Exception(Marshal.GetLastWin32Error()); }
        try
        {
            if (!GetFileInformationByHandle(handle, out var info)) throw new Win32Exception(Marshal.GetLastWin32Error());
            if ((info.Attributes & (uint)(FileAttributes.ReparsePoint | FileAttributes.Directory)) != 0 || info.Links != 1)
                throw new UnauthorizedAccessException("링크·하드링크·폴더는 텍스트 파일로 열 수 없습니다.");
            Resolve(System.IO.Path.GetRelativePath(Root, CanonicalPath(handle)), true);
            return new FileStream(handle, access);
        }
        catch { handle.Dispose(); throw; }
    }

    private static byte[] Bytes(FileStream stream)
    {
        if (stream.Length > MaxBytes) throw new IOException("텍스트 파일 크기 한도는 1 MiB입니다.");
        var bytes = new byte[checked((int)stream.Length)];
        stream.ReadExactly(bytes);
        return bytes;
    }

    private static string Decode(byte[] bytes)
    {
        var text = Utf8.GetString(bytes);
        if (text.Any(c => char.IsControl(c) && c is not '\r' and not '\n' and not '\t'))
            throw new IOException("바이너리 또는 지원하지 않는 텍스트입니다.");
        return text;
    }

    public string Read(string path)
    {
        var full = Resolve(path, true);
        using var lease = Lease(System.IO.Path.GetDirectoryName(full)!);
        using var stream = Open(full, FileAccess.Read);
        return Decode(Bytes(stream));
    }

    public (IReadOnlyList<string> Entries, bool Truncated) List(string path, CancellationToken ct)
    {
        var full = Resolve(path);
        using var lease = Lease(full);
        var result = new List<string>();
        foreach (var entry in Directory.EnumerateFileSystemEntries(full))
        {
            ct.ThrowIfCancellationRequested();
            try
            {
                var relative = System.IO.Path.GetRelativePath(Root, entry);
                var attr = File.GetAttributes(entry);
                if ((attr & FileAttributes.ReparsePoint) != 0) continue;
                Resolve(relative, (attr & FileAttributes.Directory) == 0);
                if (result.Count == 1000) return (result, true);
                result.Add(relative + ((attr & FileAttributes.Directory) != 0 ? "/" : ""));
            }
            catch (UnauthorizedAccessException) { }
        }
        return (result, false);
    }

    public (IReadOnlyList<SearchHit> Hits, bool Truncated) Search(string path, string query, CancellationToken ct)
    {
        if (query.Length is 0 or > 1000) throw new ArgumentException("검색어는 1~1000자여야 합니다.");
        var hits = new List<SearchHit>();
        var pending = new Stack<string>(); pending.Push(path);
        var truncated = false;
        while (pending.TryPop(out var directory))
        {
            ct.ThrowIfCancellationRequested();
            var list = List(directory, ct); truncated |= list.Truncated;
            foreach (var entry in list.Entries)
            {
                ct.ThrowIfCancellationRequested();
                if (entry.EndsWith('/')) { pending.Push(entry.TrimEnd('/')); continue; }
                try
                {
                    var lines = Read(entry).Split('\n');
                    for (var i = 0; i < lines.Length; i++)
                    {
                        if (!lines[i].Contains(query, StringComparison.OrdinalIgnoreCase)) continue;
                        if (hits.Count == 200) return (hits, true);
                        hits.Add(new(entry, i + 1, lines[i][..Math.Min(lines[i].Length, 2000)]));
                    }
                }
                catch (Exception ex) when (ex is IOException or UnauthorizedAccessException or DecoderFallbackException or Win32Exception)
                { truncated = true; }
            }
        }
        return (hits, truncated);
    }

    public ChangeProposal Propose(string path, string content)
    {
        var full = Resolve(path, true);
        var encoded = Utf8.GetBytes(content);
        if (encoded.Length > MaxBytes) throw new ArgumentException("변경은 1 MiB 이하만 허용됩니다.");
        Decode(encoded);
        using var lease = Lease(System.IO.Path.GetDirectoryName(full)!);
        // GetAttributes distinguishes absence from permission errors; dangling links are not new files.
        try
        {
            _ = File.GetAttributes(full);
            using var stream = Open(full, FileAccess.Read);
            var bytes = Bytes(stream);
            return new(path, Decode(bytes), content, Convert.ToHexString(SHA256.HashData(bytes)));
        }
        catch (FileNotFoundException) { return new(path, null, content, null); }
    }

    public string Apply(ChangeProposal proposal, CancellationToken ct)
    {
        ct.ThrowIfCancellationRequested();
        var full = Resolve(proposal.Path, true);
        using var lease = Lease(System.IO.Path.GetDirectoryName(full)!);
        var bytes = Utf8.GetBytes(proposal.After);
        if (bytes.Length > MaxBytes) throw new ArgumentException("변경 크기 한도 초과.");
        Decode(bytes);
        // Exclusive handle prevents changes between fingerprint validation and the write.
        using var stream = Open(full, FileAccess.ReadWrite, proposal.Hash == null ? FileMode.CreateNew : FileMode.Open);
        if (proposal.Hash != null && Convert.ToHexString(SHA256.HashData(Bytes(stream))) != proposal.Hash)
            throw new IOException("충돌: 승인 후 파일이 변경되었습니다. 새 변경 확인이 필요합니다.");
        if (proposal.Hash != null) ct.ThrowIfCancellationRequested();
        stream.Position = 0;
        // Once writing starts, finish and verify even if cancellation arrives. Never automatically replay.
        stream.Write(bytes); stream.SetLength(bytes.Length); stream.Flush(true); stream.Position = 0;
        var actual = Decode(Bytes(stream));
        if (actual != proposal.After) throw new IOException("쓰기 결과를 확인할 수 없습니다. 자동 재시도하지 않습니다.");
        return actual;
    }

    private sealed class HandleLease(List<SafeFileHandle> handles) : IDisposable
    { public void Dispose() { foreach (var h in handles) h.Dispose(); } }
    private static string CanonicalPath(SafeFileHandle handle)
    {
        var buffer = new StringBuilder(32768);
        var length = GetFinalPathNameByHandle(handle, buffer, (uint)buffer.Capacity, 0);
        if (length == 0 || length >= buffer.Capacity) throw new IOException("실제 파일 경로를 확인할 수 없습니다.");
        var path = buffer.ToString();
        if (path.StartsWith(@"\\?\UNC\", StringComparison.OrdinalIgnoreCase)) throw new UnauthorizedAccessException("원격 경로는 허용되지 않습니다.");
        return path.StartsWith(@"\\?\") ? path[4..] : path;
    }
    [StructLayout(LayoutKind.Sequential)]
    private struct FileInfo
    {
        public uint Attributes;
        public System.Runtime.InteropServices.ComTypes.FILETIME Creation, Access, Write;
        public uint Volume, SizeHigh, SizeLow, Links, IndexHigh, IndexLow;
    }
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true, EntryPoint = "CreateFileW")]
    private static extern SafeFileHandle CreateFile(string path, uint access, uint share, IntPtr security, uint creation, uint flags, IntPtr template);
    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetFileInformationByHandle(SafeFileHandle handle, out FileInfo info);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true, EntryPoint = "GetFinalPathNameByHandleW")]
    private static extern uint GetFinalPathNameByHandle(SafeFileHandle handle, StringBuilder path, uint size, uint flags);
}
