using System.Text.Json;

namespace Artemis.Core;

// Immutable, explicitly supplied per turn. Natural-language instructions cannot grant tool authority.
public sealed class RequestScope
{
    private readonly (string Path, bool Directory)[] targets;
    private readonly FileTools files;
    public bool AllowRead { get; }
    public bool AllowWrite { get; }
    public string Description { get; }

    public RequestScope(FileTools files, IEnumerable<string> paths, bool allowRead, bool allowWrite)
    {
        this.files = files; AllowRead = allowRead; AllowWrite = allowWrite;
        targets = paths.Where(p => !string.IsNullOrWhiteSpace(p)).Select(p =>
        {
            var raw = p.Trim();
            var full = files.Resolve(raw);
            var relative = Normalize(Path.GetRelativePath(files.Root, full));
            return (relative, relative == "." || raw.EndsWith('/') || raw.EndsWith('\\'));
        }).Distinct().OrderBy(t => t.relative, StringComparer.OrdinalIgnoreCase).ToArray();
        Description = JsonSerializer.Serialize(new { allow_read = AllowRead, allow_write = AllowWrite,
            targets = targets.Select(t => new { path = t.Path, recursive_directory = t.Directory }) });
    }

    private static string Normalize(string path) => path.Replace('\\', '/').TrimEnd('/');
    internal void EnsureProject(FileTools owner)
    {
        if (!owner.Root.Equals(files.Root, StringComparison.OrdinalIgnoreCase)) throw new UnauthorizedAccessException("요청 범위의 프로젝트가 다릅니다.");
    }
    private string Relative(FileTools owner, string path)
    {
        EnsureProject(owner);
        return Normalize(Path.GetRelativePath(files.Root, owner.Resolve(path)));
    }
    private static bool Within(string path, string directory) => directory == "." ||
        path.StartsWith(directory + "/", StringComparison.OrdinalIgnoreCase);
    public bool AllowsFile(FileTools owner, string path, bool write = false)
    {
        var relative = Relative(owner, path);
        return (write ? AllowWrite : AllowRead) && targets.Any(t =>
            relative.Equals(t.Path, StringComparison.OrdinalIgnoreCase) || (t.Directory && Within(relative, t.Path)));
    }
    public bool AllowsDirectory(FileTools owner, string path)
    {
        var relative = Relative(owner, path);
        return AllowRead && targets.Any(t =>
            (relative.Equals(t.Path, StringComparison.OrdinalIgnoreCase) && t.Directory) ||
            (t.Directory && Within(relative, t.Path)) || Within(t.Path, relative));
    }
    public void EnsureFile(FileTools owner, string path, bool write = false)
    {
        if (!AllowsFile(owner, path, write)) throw new UnauthorizedAccessException("이 요청에서 허용하지 않은 파일 또는 작업입니다.");
    }
    public void EnsureDirectory(FileTools owner, string path)
    {
        if (!AllowsDirectory(owner, path)) throw new UnauthorizedAccessException("이 요청에서 허용하지 않은 조회 경로입니다.");
    }
}
