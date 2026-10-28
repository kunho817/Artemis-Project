using System.Collections.Concurrent;
using System.Globalization;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Documents;
using System.Windows.Interop;
using System.Windows.Media;
using System.Windows.Media.Imaging;
using System.Windows.Threading;
using Artemis.Core;
using Artemis.Desktop;

namespace Artemis.RenderingTests;

internal static class Program
{
    [STAThread]
    public static int Main(string[] args)
    {
        // This process never shows a Window, captures the desktop, or sends native input.
        RenderOptions.ProcessRenderMode = RenderMode.SoftwareOnly;
        var app = new Application { ShutdownMode = ShutdownMode.OnExplicitShutdown };
        app.Dispatcher.InvokeAsync(async () =>
        {
            var code = 0;
            try
            {
                var output = Path.GetFullPath(args.Length > 0 ? args[0] : "test-results/rendering");
                await new RenderSuite(output, args.Length > 1 ? Path.GetFullPath(args[1]) : null).Run();
            }
            catch (Exception ex) { Console.Error.WriteLine(ex); code = 1; }
            finally { app.Shutdown(code); }
        });
        return app.Run();
    }
}

internal sealed class RenderSuite(string output, string? baseline)
{
    private MainWindow window = null!;
    private Border surface = null!;
    private readonly FixtureHttp transport = new();
    private readonly List<object> frames = [];
    private readonly List<string> checks = [];
    private readonly List<string> failures = [];
    private readonly List<(string State, string File)> previews = [];
    private string project = "";
    private T Get<T>(string name) where T : FrameworkElement => (T)window.FindName(name);
    private string Status => Get<TextBlock>("StatusLabel").Text;
    private void Check(bool passed, string name)
    {
        if (!passed) throw new InvalidOperationException(name);
        checks.Add(name); Console.WriteLine("PASS " + name);
    }
    private void Click(string name)
    {
        var button = Get<Button>(name);
        if (!button.IsEnabled) throw new InvalidOperationException("Disabled button: " + name);
        button.RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
    }
    private async Task Until(Func<bool> condition)
    {
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(15));
        while (!condition()) await Task.Delay(10, timeout.Token);
        await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
    }
    public async Task Run()
    {
        if (baseline != null && Path.TrimEndingDirectorySeparator(output).Equals(Path.TrimEndingDirectorySeparator(baseline), StringComparison.OrdinalIgnoreCase))
            throw new ArgumentException("Output and baseline directories must differ; never overwrite a baseline during comparison.");
        Directory.CreateDirectory(output);
        var temp = Path.Combine(Path.GetTempPath(), "Artemis-RenderTests-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(temp); project = Path.Combine(temp, "project"); Directory.CreateDirectory(project);
        File.WriteAllText(Path.Combine(project, "requested.txt"), "ALLOWED_RENDER_BODY\n");
        File.WriteAllText(Path.Combine(project, "unrelated.txt"), "FORBIDDEN_RENDER_BODY\n");
        File.WriteAllText(Path.Combine(project, "conflict.txt"), "original\n");
        try
        {
            var store = new CredentialStore(Path.Combine(temp, "credentials"));
            var account = new Account("render-fixture", "fixture-subject", "render@example.invalid", "fake-access", "fake-refresh", null,
                ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
            store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
            window = new(new HttpClient(transport) { Timeout = Timeout.InfiniteTimeSpan }, store, "Local\\Artemis.RenderTests." + Guid.NewGuid());
            // Detach the real XAML content into a parentless rendering surface. No HWND / Show call.
            var content = (FrameworkElement)window.Content; window.Content = null;
            surface = new Border { Child = content, Background = window.Background, Resources = window.Resources };
            TextElement.SetFontFamily(surface, window.FontFamily); TextElement.SetFontSize(surface, window.FontSize);
            TextElement.SetForeground(surface, window.Foreground);
            Click("CatalogButton"); await Until(() => Status == "모델 목록 확인 완료");
            window.SelectProject(project);
            Get<TextBlock>("RootLabel").Text = @"C:\RenderFixture\프로젝트\긴 경로 표시 검사";
            Check(!Get<Button>("StopButton").IsEnabled && !Get<Button>("ApproveButton").IsEnabled, "Idle controls");
            Capture("idle");
            var send = Get<Button>("SendButton"); var transform = send.RenderTransform;
            send.RenderTransform = new TranslateTransform(5000, 0); surface.UpdateLayout();
            Check(Geometry(1920, 1048).Any(i => i.StartsWith("SendButton has empty/outside")), "Detector rejects deliberately offscreen button");
            send.RenderTransform = transform; surface.UpdateLayout();
            var hint = Get<TextBlock>("ScopeHint"); var height = hint.Height; hint.Height = 1; Layout(1920, 1048);
            Check(Geometry(1920, 1048).Any(i => i == "ScopeHint text clipped vertically"), "Detector rejects deliberately clipped text");
            hint.Height = height; Layout(1920, 1048);

            QueueCall("read_text_file", new { path = "unrelated.txt", start_line = 1, max_lines = 10 }, "범위 밖 조회 거부");
            Send("빈 범위 조회 검증", "", false);
            await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(transport.LastToolResult.Contains("error") && !transport.LastToolResult.Contains("FORBIDDEN_RENDER_BODY"), "Empty UI scope blocks tool output");

            QueueCall("read_text_file", new { path = "requested.txt", start_line = 1, max_lines = 10 }, "허용된 조회 완료");
            Send("허용된 파일 조회 검증", "requested.txt", false);
            await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(transport.LastToolResult.Contains("ALLOWED_RENDER_BODY"), "Scope control reaches harness");
            Check(Get<TextBox>("ScopePaths").Text == "" && !Get<CheckBox>("WriteScope").IsChecked!.Value, "Scope authority resets after send");

            QueueCall("write_text_file", new { path = "denied.txt", content = "거부할 변경\n" }, "변경 거부 확인");
            Send("거부 검증", "denied.txt", true);
            await Until(() => Status.StartsWith("사용자 확인 대기"));
            Check(!Get<Button>("SendButton").IsEnabled && !Get<TextBox>("ScopePaths").IsEnabled && Get<Button>("DenyButton").IsEnabled, "Approval state locks request and enables decision");
            Check(!File.Exists(Path.Combine(project, "denied.txt")), "Approval preview has no disk side effect");
            Capture("approval"); Click("DenyButton"); await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(transport.LastToolResult.Contains("denied") && !File.Exists(Path.Combine(project, "denied.txt")), "Deny button prevents write");
            Capture("denied");

            QueueCall("write_text_file", new { path = "created.txt", content = "승인한 변경\n" }, "승인된 변경 완료");
            Send("승인 검증", "created.txt", true); await Until(() => Status.StartsWith("사용자 확인 대기"));
            Click("ApproveButton"); await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(File.ReadAllText(Path.Combine(project, "created.txt")) == "승인한 변경\n", "Approve button applies exact scope-bound content");

            QueueCall("write_text_file", new { path = "conflict.txt", content = "proposed\n" }, "충돌 오류 확인");
            Send("충돌 검증", "conflict.txt", true); await Until(() => Status.StartsWith("사용자 확인 대기"));
            File.WriteAllText(Path.Combine(project, "conflict.txt"), "external\n");
            Click("ApproveButton"); await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(transport.LastToolResult.Contains("error") && File.ReadAllText(Path.Combine(project, "conflict.txt")) == "external\n", "Conflict keeps external content");
            Capture("conflict");

            transport.Enqueue(_ => Task.FromResult(FixtureHttp.Complete(FixtureHttp.Call("write_text_file", new { path = "stopped.txt", content = "no\n" }))));
            Send("승인 대기 중 중단", "stopped.txt", true); await Until(() => Status.StartsWith("사용자 확인 대기"));
            var count = transport.Bodies.Count; Click("StopButton"); await Until(() => Status.StartsWith("중단됨"));
            Check(!File.Exists(Path.Combine(project, "stopped.txt")) && transport.Bodies.Count == count, "Stop during approval prevents file and followup");

            var gate = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
            transport.Enqueue(_ => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
                { Content = new StreamContent(new GatedStream(FixtureHttp.Delta("스트리밍 부분 응답입니다.\n"), gate.Task)) }));
            Send("스트리밍 중단 검증", "", false);
            await Until(() => Get<TextBox>("Transcript").Text.Contains("스트리밍 부분 응답입니다."));
            Check(Status == "실행 중" && Get<Button>("StopButton").IsEnabled && !Get<Button>("SendButton").IsEnabled, "Streaming controls");
            Capture("streaming"); Click("StopButton"); await Until(() => Status.StartsWith("중단됨")); Capture("stopped");

            transport.Enqueue(_ => Task.FromResult(FixtureHttp.Reply(FixtureHttp.Delta("실패 전 부분 출력\n") +
                "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\"}}}\n\n")));
            Send("사용량 오류 검증", "", false); await Until(() => Status.StartsWith("실패"));
            Check(Status.Contains("subscription_sharing_usage_limit_exceeded") && Get<Button>("SendButton").IsEnabled, "Usage error restores controls without completion");
            Capture("error");

            transport.Enqueue(_ => Task.FromResult(FixtureHttp.Complete(FixtureHttp.Answer("복구 후 정상 완료"), "복구 후 정상 완료")));
            Send("복구 검증", "", false); await Until(() => Status.StartsWith("완료") && Get<Button>("SendButton").IsEnabled);
            Check(Get<TextBox>("Transcript").Text.Contains("복구 후 정상 완료"), "Recovery completes through UI events"); Capture("completed");

            Click("SignOutButton"); await Until(() => Status.StartsWith("로컬 연결 해제 완료"));
            Check(Get<TextBlock>("AuthLabel").Text == "연결 안 됨", "Malformed discovery still updates disconnected UI"); Capture("disconnected");
        }
        catch (Exception ex) { failures.Add(ex.Message); Console.Error.WriteLine("FAIL " + ex); }
        finally
        {
            window?.Close();
            if (!Path.GetFileName(temp).StartsWith("Artemis-RenderTests-") || Path.GetDirectoryName(temp) != Path.TrimEndingDirectorySeparator(Path.GetTempPath()))
                throw new IOException("Unsafe fixture cleanup path");
            Directory.Delete(temp, true);
            File.WriteAllText(Path.Combine(output, "results.json"), JsonSerializer.Serialize(new { checks, failures, frames, baseline }, new JsonSerializerOptions { WriteIndented = true }));
            WriteGallery();
        }
        Console.WriteLine($"{checks.Count} checks, {frames.Count} renders, {failures.Count} failures. {output}");
        if (failures.Count != 0) throw new InvalidOperationException("Rendering/control checks failed; see results.json");
    }
    private void Send(string prompt, string paths, bool write)
    {
        Get<TextBox>("Prompt").Text = prompt; Get<TextBox>("ScopePaths").Text = paths;
        Get<CheckBox>("ReadScope").IsChecked = true; Get<CheckBox>("WriteScope").IsChecked = write; Click("SendButton");
    }
    private void QueueCall(string name, object args, string answer)
    {
        transport.Enqueue(_ => Task.FromResult(FixtureHttp.Complete(FixtureHttp.Call(name, args))));
        transport.Enqueue(_ => Task.FromResult(FixtureHttp.Complete(FixtureHttp.Answer(answer), answer)));
    }
    private void Capture(string state)
    {
        foreach (var (width, height) in new[] { (1050, 688), (1380, 868), (1920, 1048) })
        foreach (var dpi in new[] { 96, 144 })
        {
            var scale = dpi / 96d; VisualTreeHelper.SetRootDpi(surface, new(scale, scale));
            Layout(width, height);
            // Pending ScrollToEnd requests depend on dispatcher timing and prior viewport sizes.
            // Normalize scroll positions after layout so identical fixtures produce comparable pixels.
            foreach (var control in new[] { "Transcript", "ToolLog", "ChangePreview" }) Get<TextBox>(control).ScrollToHome();
            surface.UpdateLayout();
            var name = $"{state}-{width}x{height}-{dpi}dpi.png";
            var issues = Geometry(width, height);
            var bitmap = new RenderTargetBitmap((int)Math.Ceiling(width * scale), (int)Math.Ceiling(height * scale), dpi, dpi, PixelFormats.Pbgra32);
            bitmap.Render(surface);
            var encoder = new PngBitmapEncoder(); encoder.Frames.Add(BitmapFrame.Create(bitmap));
            using (var stream = File.Create(Path.Combine(output, name))) encoder.Save(stream);
            var pixels = new byte[bitmap.PixelWidth * bitmap.PixelHeight * 4]; bitmap.CopyPixels(pixels, bitmap.PixelWidth * 4, 0);
            if (pixels.Where((_, i) => i % 4 == 3).Count(b => b > 0) < bitmap.PixelWidth * bitmap.PixelHeight * 0.95) issues.Add("Blank/transparent render");
            var colors = new HashSet<int>();
            for (var i = 0; i < pixels.Length; i += 64) colors.Add(BitConverter.ToInt32(pixels, i));
            if (colors.Count < 16) issues.Add("Render has no meaningful content variation");
            if (baseline != null) CompareBaseline(name, bitmap, pixels, issues);
            frames.Add(new { state, width, height, dpi, file = name, issues });
            previews.Add((state, name));
            foreach (var issue in issues) failures.Add(name + ": " + issue);
        }
    }
    private void Layout(double width, double height)
    {
        surface.Width = width; surface.Height = height; surface.InvalidateMeasure();
        surface.Measure(new(width, height)); surface.Arrange(new(0, 0, width, height)); surface.UpdateLayout();
    }
    private List<string> Geometry(int width, int height)
    {
        var issues = new List<string>(); var bounds = new Dictionary<string, Rect>();
        foreach (var name in new[] { "Transcript", "ScopePaths", "ReadScope", "WriteScope", "Prompt", "SendButton", "StopButton", "ToolLog", "ChangePreview", "ApproveButton", "DenyButton", "StatusLabel", "ScopeLabel", "ScopeHint" })
        {
            var element = Get<FrameworkElement>(name);
            var rect = element.TransformToAncestor(surface).TransformBounds(new Rect(element.RenderSize)); bounds[name] = rect;
            if (rect.Width < 1 || rect.Height < 1 || rect.Left < -0.5 || rect.Top < -0.5 || rect.Right > width + 0.5 || rect.Bottom > height + 0.5)
                issues.Add($"{name} has empty/outside bounds: {rect}");
            if (element is TextBlock text)
            {
                var probe = new TextBlock { Text = text.Text, FontFamily = text.FontFamily, FontSize = text.FontSize, FontWeight = text.FontWeight,
                    FontStyle = text.FontStyle, TextWrapping = text.TextWrapping };
                probe.Measure(new(text.ActualWidth, double.PositiveInfinity));
                var clip = LayoutInformation.GetLayoutClip(text);
                var visibleHeight = Math.Min(text.ActualHeight, clip?.Bounds.Height ?? double.PositiveInfinity);
                if (!double.IsNaN(text.Height)) visibleHeight = Math.Min(visibleHeight, text.Height);
                if (probe.DesiredSize.Height > visibleHeight + 1) issues.Add($"{name} text clipped vertically");
            }
        }
        var names = bounds.Keys.ToArray();
        for (var a = 0; a < names.Length; a++) for (var b = a + 1; b < names.Length; b++)
        {
            var overlap = Rect.Intersect(bounds[names[a]], bounds[names[b]]);
            if (!overlap.IsEmpty && overlap.Width > 1 && overlap.Height > 1) issues.Add($"{names[a]} overlaps {names[b]}");
        }
        return issues;
    }
    private void CompareBaseline(string name, BitmapSource actual, byte[] pixels, List<string> issues)
    {
        var path = Path.Combine(baseline!, name);
        if (!File.Exists(path)) { issues.Add("Missing approved baseline"); return; }
        using var stream = File.OpenRead(path);
        var decoded = BitmapDecoder.Create(stream, BitmapCreateOptions.None, BitmapCacheOption.OnLoad).Frames[0];
        if (decoded.PixelWidth != actual.PixelWidth || decoded.PixelHeight != actual.PixelHeight) { issues.Add("Baseline dimensions differ"); return; }
        var converted = new FormatConvertedBitmap(decoded, PixelFormats.Pbgra32, null, 0);
        var expected = new byte[pixels.Length]; converted.CopyPixels(expected, actual.PixelWidth * 4, 0);
        var different = 0;
        for (var i = 0; i < pixels.Length; i += 4)
            if (Math.Abs(pixels[i] - expected[i]) > 12 || Math.Abs(pixels[i + 1] - expected[i + 1]) > 12 ||
                Math.Abs(pixels[i + 2] - expected[i + 2]) > 12 || Math.Abs(pixels[i + 3] - expected[i + 3]) > 12) different++;
        if (different / (double)(actual.PixelWidth * actual.PixelHeight) > 0.005) issues.Add($"Baseline pixels differ: {different}");
    }
    private void WriteGallery()
    {
        var html = new StringBuilder("<!doctype html><meta charset='utf-8'><title>Artemis WPF rendering</title><style>body{background:#10151e;color:#e8edf5;font:14px sans-serif;margin:24px}section{margin-bottom:32px}img{width:100%;max-width:900px;border:1px solid #526277}a{color:#98c6ff}</style>");
        html.Append($"<h1>Artemis WPF rendering</h1><p>{checks.Count} control checks / {frames.Count} renders / {failures.Count} failures. No visible windows or desktop capture. Synthetic data only.</p>");
        foreach (var entry in previews) html.Append($"<section><h2>{entry.File}</h2><a href='{entry.File}'><img loading='lazy' src='{entry.File}'></a></section>");
        File.WriteAllText(Path.Combine(output, "index.html"), html.ToString());
    }
}

internal sealed class FixtureHttp : HttpMessageHandler
{
    private readonly ConcurrentQueue<Func<CancellationToken, Task<HttpResponseMessage>>> scripts = new();
    public ConcurrentQueue<JsonNode> Bodies { get; } = new();
    public string LastToolResult => Bodies.Last()["input"]!.AsArray().OfType<JsonObject>().Last(o => o["type"]?.ToString() == "function_call_output")["output"]!.ToString();
    public void Enqueue(Func<CancellationToken, Task<HttpResponseMessage>> script) => scripts.Enqueue(script);
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken ct)
    {
        if (request.RequestUri?.AbsolutePath == "/v1/models") return Reply("{\"models\":[{\"slug\":\"gpt-6.1-sol\",\"visibility\":\"list\"},{\"slug\":\"gpt-6.1-terra\",\"visibility\":\"list\"}]}");
        if (request.RequestUri?.AbsolutePath == "/.well-known/openid-configuration") return Reply("{}");
        if (request.RequestUri?.AbsolutePath != "/v1/responses") throw new IOException("Unexpected fixture request; no network transport exists");
        Bodies.Enqueue(JsonNode.Parse(await request.Content!.ReadAsStringAsync(ct))!);
        return await (scripts.TryDequeue(out var script) ? script(ct) : throw new IOException("No scripted response"));
    }
    public static string Delta(string text) => "data: " + JsonSerializer.Serialize(new { type = "response.output_text.delta", delta = text }) + "\n\n";
    public static HttpResponseMessage Complete(JsonArray output, string text = "") => Reply(Delta(text) + "data: " + JsonSerializer.Serialize(new { type = "response.completed", response = new { status = "completed", output } }) + "\n\n");
    public static HttpResponseMessage Reply(string text) => new(HttpStatusCode.OK) { Content = new StringContent(text, Encoding.UTF8) };
    public static JsonArray Call(string name, object args) => new(new JsonObject { ["type"] = "function_call", ["namespace"] = "project", ["call_id"] = "fixture-call", ["name"] = name, ["arguments"] = JsonSerializer.Serialize(args) });
    public static JsonArray Answer(string text) => new(new JsonObject { ["type"] = "message", ["role"] = "assistant", ["content"] = new JsonArray(new JsonObject { ["type"] = "output_text", ["text"] = text }) });
}

internal sealed class GatedStream(string prefix, Task gate) : Stream
{
    private readonly byte[] bytes = Encoding.UTF8.GetBytes(prefix);
    private int position;
    public override bool CanRead => true;
    public override bool CanSeek => false;
    public override bool CanWrite => false;
    public override long Length => throw new NotSupportedException();
    public override long Position { get => position; set => throw new NotSupportedException(); }
    public override async ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken ct = default)
    {
        if (position == bytes.Length) { await gate.WaitAsync(ct); return 0; }
        var length = Math.Min(buffer.Length, bytes.Length - position);
        bytes.AsMemory(position, length).CopyTo(buffer); position += length; return length;
    }
    public override int Read(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    public override void Flush() { }
    public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
    public override void SetLength(long value) => throw new NotSupportedException();
    public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
}
