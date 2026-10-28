using System.Diagnostics;
using System.Net;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using Artemis.Core;

// Dependency-free regression runner. All credentials/HTTP replies are synthetic; no live requests.
var suite = new List<(string, Func<Task>)>();
void Test(string name, Action run) => suite.Add((name, () => { run(); return Task.CompletedTask; }));
void AsyncTest(string name, Func<Task> run) => suite.Add((name, run));
void Check(bool condition, string message = "Assertion failed") { if (!condition) throw new Exception(message); }
void Reject(Action action) { try { action(); } catch { return; } throw new Exception("Operation was incorrectly accepted"); }
async Task RejectAsync(Func<Task> action) { try { await action(); } catch { return; } throw new Exception("Operation was incorrectly accepted"); }
var temp = Path.Combine(Path.GetTempPath(), "Artemis-Tests-" + Guid.NewGuid().ToString("N"));
var root = Path.Combine(temp, "project"); var outside = Path.Combine(temp, "outside");
Directory.CreateDirectory(root); Directory.CreateDirectory(outside);
File.WriteAllText(Path.Combine(root, "sample.txt"), "alpha\nbeta\nalpha\n", new UTF8Encoding(false));
File.WriteAllText(Path.Combine(outside, "outside.txt"), "outside");
var files = new FileTools(root);
var ct = CancellationToken.None;

Test("Volume-root project resolves child paths correctly", () =>
{
    var volume = new FileTools(Path.GetPathRoot(root)!);
    Check(volume.Resolve(Path.GetRelativePath(volume.Root, root)) == root);
});

Test("UTF-8 read/list/search return real paths and lines", () =>
{
    Check(files.Read("sample.txt").Contains("beta"));
    Check(files.List(".", ct).Entries.Contains("sample.txt"));
    var result = files.Search(".", "alpha", ct);
    Check(result.Hits.Count == 2 && result.Hits[1].Line == 3 && !result.Truncated);
});
Test("Root escape, ADS, devices and ambiguous names rejected", () =>
{
    foreach (var path in new[] { "../outside/outside.txt", "C:\\Windows\\win.ini", "sample.txt:stream", "NUL.txt", "COM1", "sub./x.txt", "sample.txt " })
        Reject(() => files.Read(path));
});
Test("Secrets and binaries excluded from all discovery", () =>
{
    foreach (var path in new[] { ".env", ".env.local", "credentials.json", "private.key", "auth.json", "image.png" })
    {
        File.WriteAllText(Path.Combine(root, path), "alpha"); Reject(() => files.Read(path));
        Reject(() => files.Propose(path, "x")); Check(!files.List(".", ct).Entries.Contains(path));
        Check(!files.Search(".", "alpha", ct).Hits.Any(h => h.Path == path));
    }
    Directory.CreateDirectory(Path.Combine(root, ".git"));
    Reject(() => files.List(".git", ct)); Reject(() => new FileTools(Path.Combine(root, ".git")));
    File.WriteAllBytes(Path.Combine(root, "binary.txt"), [0, 1, 2]); Reject(() => files.Read("binary.txt"));
    File.WriteAllBytes(Path.Combine(root, "invalid.txt"), [0xFF, 0xFE]); Reject(() => files.Read("invalid.txt"));
});
Test("Proposal leaves file untouched; create and edit verified", () =>
{
    var p = files.Propose("new.md", "created"); Check(!File.Exists(Path.Combine(root, "new.md")));
    Check(files.Apply(p, ct) == "created");
    var edit = files.Propose("new.md", "updated"); Check(files.Read("new.md") == "created");
    Check(files.Apply(edit, ct) == "updated" && files.Read("new.md") == "updated");
});
Test("Approval conflicts never overwrite current file", () =>
{
    var p = files.Propose("new.md", "overwrite"); File.WriteAllText(Path.Combine(root, "new.md"), "external");
    Reject(() => files.Apply(p, ct)); Check(files.Read("new.md") == "external");
    var create = files.Propose("collision.txt", "first"); File.WriteAllText(Path.Combine(root, "collision.txt"), "second");
    Reject(() => files.Apply(create, ct)); Check(files.Read("collision.txt") == "second");
});
Test("Cancelled apply does not create a file", () =>
{
    var p = files.Propose("cancel.txt", "x"); Reject(() => files.Apply(p, new CancellationToken(true)));
    Check(!File.Exists(Path.Combine(root, "cancel.txt")));
});
Test("Junction escape and junction swap after approval rejected", () =>
{
    var link = Path.Combine(root, "linked");
    using (var process = Process.Start(new ProcessStartInfo("cmd.exe")
    {
        Arguments = $"/c mklink /J \"{link}\" \"{outside}\"", CreateNoWindow = true,
        UseShellExecute = false, RedirectStandardOutput = true, RedirectStandardError = true
    })!) { process.WaitForExit(); Check(process.ExitCode == 0, "Cannot create test junction"); }
    try
    {
        Reject(() => files.Read("linked/outside.txt")); Reject(() => files.Propose("linked/new.txt", "x"));
        Check(!files.List(".", ct).Entries.Contains("linked/")); Reject(() => new FileTools(link));
    }
    finally { Directory.Delete(link); }
    var target = Path.Combine(root, "changing"); Directory.CreateDirectory(target);
    File.WriteAllText(Path.Combine(target, "outside.txt"), "inside");
    var p = files.Propose("changing/outside.txt", "bad");
    File.Delete(Path.Combine(target, "outside.txt")); Directory.Delete(target);
    using (var process = Process.Start(new ProcessStartInfo("cmd.exe") { Arguments = $"/c mklink /J \"{target}\" \"{outside}\"", CreateNoWindow = true, UseShellExecute = false, RedirectStandardOutput = true })!) process.WaitForExit();
    try { Reject(() => files.Apply(p, ct)); Check(File.ReadAllText(Path.Combine(outside, "outside.txt")) == "outside"); }
    finally { Directory.Delete(target); }
});
Test("Hardlinks rejected", () =>
{
    var link = Path.Combine(root, "hardlink.txt");
    using var process = Process.Start(new ProcessStartInfo("cmd.exe") { Arguments = $"/c mklink /H \"{link}\" \"{Path.Combine(outside, "outside.txt")}\"", CreateNoWindow = true, UseShellExecute = false, RedirectStandardOutput = true })!;
    process.WaitForExit(); Check(process.ExitCode == 0); Reject(() => files.Read("hardlink.txt"));
    Reject(() => files.Propose("hardlink.txt", "x"));
});

string Event(string type, object extra) => "data: " + JsonSerializer.Serialize(new { type, response = extra }) + "\n\n";
string Complete(JsonArray output) => Event("response.completed", new { status = "completed", output });
JsonObject Call(string name, object arguments, string id = "c1") => new()
{ ["type"] = "function_call", ["namespace"] = "project", ["name"] = name, ["arguments"] = JsonSerializer.Serialize(arguments), ["call_id"] = id };
JsonArray Answer() => new(new JsonObject { ["type"] = "message", ["role"] = "assistant", ["phase"] = "final_answer", ["content"] = new JsonArray(new JsonObject { ["type"] = "output_text", ["text"] = "done" }) });

AsyncTest("SSE multiline events and completed output", async () =>
{
    var text = "";
    var stream = ": keepalive\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\ndata: \"delta\":\"hello\"}\n\n" + Complete(Answer());
    var output = await ResponsesClient.ReadEventsAsync(new StringReader(stream), d => text += d, ct);
    Check(text == "hello" && output.Count == 1);
});
AsyncTest("Partial output, failed/incomplete and dropped streams never complete", async () =>
{
    foreach (var tail in new[] { "", Event("response.failed", new { error = new { code = "subscription_sharing_usage_limit_exceeded" } }), Event("response.incomplete", new { status = "incomplete" }) })
    {
        var text = "";
        await RejectAsync(() => ResponsesClient.ReadEventsAsync(new StringReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" + tail), d => text += d, ct));
        Check(text == "partial");
    }
});
AsyncTest("HTTP retries stop at five, schema matches SIWC route", async () =>
{
    var calls = 0; var retries = 0;
    using var http = new HttpClient(new FakeHttp(async request =>
    {
        calls++; Check(request.Headers.Authorization?.Parameter == "fake-access");
        var body = JsonNode.Parse(await request.Content!.ReadAsStringAsync())!;
        Check(body["store"]!.GetValue<bool>() == false && body["stream"]!.GetValue<bool>());
        Check(body["model"]!.ToString() == "gpt-6.1-sol" && body["tools"]![0]!["type"]!.ToString() == "namespace");
        Check(body["previous_response_id"] == null && body["max_output_tokens"] == null);
        var response = new HttpResponseMessage(HttpStatusCode.TooManyRequests);
        response.Headers.RetryAfter = new(TimeSpan.Zero); return response;
    }));
    var client = new ResponsesClient(http, _ => Task.FromResult("fake-access"));
    await RejectAsync(() => client.RespondAsync("gpt-6.1-sol", [], "scope", _ => { }, n => retries = n, ct));
    Check(calls == 6 && retries == 5);
});
AsyncTest("Failed stream is never retried", async () =>
{
    var calls = 0;
    using var http = new HttpClient(new FakeHttp(_ =>
    { calls++; return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n") }); }));
    await RejectAsync(() => new ResponsesClient(http, _ => Task.FromResult("fake")).RespondAsync("gpt-6.1-terra", [], "", _ => { }, _ => { }, ct));
    Check(calls == 1);
});
AsyncTest("Both exact models complete function-result loop and retain followup context", async () =>
{
    foreach (var model in ResponsesClient.Models)
    {
        var turn = 0;
        var fake = new FakeModel((chosen, input, token) =>
        {
            Check(chosen == model); turn++;
            if (turn == 1) return new(Call("read_text_file", new { path = "sample.txt", start_line = 1, max_lines = 2 }));
            if (turn == 2)
            {
                var result = input.Last()!; Check(result["call_id"]!.ToString() == "c1");
                Check(result["output"]!.ToString().Contains("beta"));
                Check(input.OfType<JsonObject>().Any(o => o["type"]?.ToString() == "function_call"));
            }
            if (turn == 3) Check(input.Count == 5 && input[3]!["phase"]!.ToString() == "final_answer");
            return Answer();
        });
        var h = new Harness(fake, files); await h.RunAsync("read sample", model, ct); await h.RunAsync("follow up", model, ct);
        Check(h.State == RunState.Completed && turn == 3);
    }
});
AsyncTest("Denied write produces denied call output without mutation", async () =>
{
    var turn = 0; var confirmed = false;
    var fake = new FakeModel((_, input, _) => ++turn == 1 ? new(Call("write_text_file", new { path = "denied.txt", content = "no" })) :
        input.Last()!["output"]!.ToString().Contains("denied") ? Answer() : throw new Exception("Missing denial"));
    var h = new Harness(fake, files) { Confirm = (p, _) => { Check(p.Before == null && !File.Exists(Path.Combine(root, p.Path))); confirmed = true; return Task.FromResult(false); } };
    await h.RunAsync("write", "gpt-6.1-sol", ct); Check(confirmed && !File.Exists(Path.Combine(root, "denied.txt")));
});
AsyncTest("Approved write uses actual verified contents and conflict errors", async () =>
{
    foreach (var conflict in new[] { false, true })
    {
        var turn = 0; File.WriteAllText(Path.Combine(root, "approved.txt"), "before");
        var fake = new FakeModel((_, input, _) =>
        {
            if (++turn == 1) return new(Call("write_text_file", new { path = "approved.txt", content = "after" }));
            var result = JsonNode.Parse(input.Last()!["output"]!.ToString())!;
            Check(result["status"]!.ToString() == (conflict ? "error" : "success"));
            if (!conflict) Check(result["actual_content"]!.ToString() == files.Read("approved.txt"));
            return Answer();
        });
        var h = new Harness(fake, files) { Confirm = (_, _) => { if (conflict) File.WriteAllText(Path.Combine(root, "approved.txt"), "external"); return Task.FromResult(true); } };
        await h.RunAsync("write", "gpt-6.1-sol", ct); Check(files.Read("approved.txt") == (conflict ? "external" : "after"));
    }
});
AsyncTest("Malformed and unknown tools return errors tied to call IDs", async () =>
{
    var turn = 0;
    var fake = new FakeModel((_, input, _) =>
    {
        if (++turn == 1) return new(Call("shell", new { path = "." }, "bad1"), Call("read_text_file", new { path = "sample.txt", start_line = 0, max_lines = 1 }, "bad2"));
        var results = input.OfType<JsonObject>().Where(o => o["type"]?.ToString() == "function_call_output").ToList();
        Check(results.Count == 2 && results.All(r => r["output"]!.ToString().Contains("error"))); return Answer();
    });
    await new Harness(fake, files).RunAsync("read", "gpt-6.1-sol", ct);
});
AsyncTest("Stop during approval prevents write, second tool and followup request", async () =>
{
    using var source = new CancellationTokenSource(); var calls = 0; var results = 0;
    var fake = new FakeModel((_, _, _) => { calls++; return new(Call("write_text_file", new { path = "stopped.txt", content = "no" }), Call("read_text_file", new { path = "sample.txt", start_line = 1, max_lines = 1 }, "c2")); });
    var h = new Harness(fake, files) { Confirm = (_, _) => { source.Cancel(); return Task.FromResult(true); } };
    h.ToolFinished += _ => results++;
    await RejectAsync(() => h.RunAsync("write", "gpt-6.1-sol", source.Token));
    Check(h.State == RunState.Stopped && calls == 1 && results == 0 && !File.Exists(Path.Combine(root, "stopped.txt")));
});
Test("OIDC signature, issuer, audience, expiry and nonce enforced", () =>
{
    using var rsa = RSA.Create(2048); var parameters = rsa.ExportParameters(false);
    var keys = JsonSerializer.SerializeToElement(new { keys = new[] { new { kid = "test", n = Authentication.Base64(parameters.Modulus!), e = Authentication.Base64(parameters.Exponent!) } } });
    string Jwt(string issuer = "https://auth.openai.com", string audience = "oaiapp_test", long? expiry = null, string nonce = "nonce")
    {
        var header = Authentication.Base64(JsonSerializer.SerializeToUtf8Bytes(new { alg = "RS256", kid = "test" }));
        var payload = Authentication.Base64(JsonSerializer.SerializeToUtf8Bytes(new { iss = issuer, aud = audience, sub = "subject", email = "synthetic@example.invalid", exp = expiry ?? DateTimeOffset.UtcNow.AddHours(1).ToUnixTimeSeconds(), nonce }));
        var message = header + "." + payload;
        return message + "." + Authentication.Base64(rsa.SignData(Encoding.ASCII.GetBytes(message), HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1));
    }
    Check(Authentication.ValidateIdentity(Jwt(), keys, "oaiapp_test", "nonce", DateTimeOffset.UtcNow).Subject == "subject");
    foreach (var token in new[] { Jwt(issuer: "https://evil.invalid"), Jwt(audience: "wrong"), Jwt(expiry: 1), Jwt(nonce: "wrong"), Jwt() + "x" })
        Reject(() => Authentication.ValidateIdentity(token, keys, "oaiapp_test", "nonce", DateTimeOffset.UtcNow));
});
Test("DPAPI store survives restart without plaintext tokens", () =>
{
    var directory = Path.Combine(temp, "store"); var store = new CredentialStore(directory); var data = store.Load();
    var account = new Account("oaiapp_test", "sub", "synthetic@example.invalid", "fake-access-token", "fake-refresh-token", "fake-id-token", ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
    store.Save(data with { Accounts = [account], ActiveClientId = account.ClientId });
    var loaded = new CredentialStore(directory).Load(); Check(loaded.HostId == data.HostId && loaded.Accounts[0].RefreshToken == "fake-refresh-token");
    Check(!Encoding.UTF8.GetString(File.ReadAllBytes(Path.Combine(directory, "accounts.dat"))).Contains("fake-access-token"));
});

var failed = 0;
try
{
    foreach (var (name, run) in suite)
    {
        try { await run(); Console.WriteLine("PASS " + name); }
        catch (Exception ex) { failed++; Console.WriteLine("FAIL " + name + ": " + ex); }
    }
}
finally
{
    if (!Path.GetFullPath(temp).StartsWith(Path.GetFullPath(Path.GetTempPath()), StringComparison.OrdinalIgnoreCase) || !Path.GetFileName(temp).StartsWith("Artemis-Tests-")) throw new Exception("Unsafe cleanup path");
    Directory.Delete(temp, true);
}
Console.WriteLine($"{suite.Count - failed}/{suite.Count} passed"); return failed == 0 ? 0 : 1;

sealed class FakeModel(Func<string, JsonArray, CancellationToken, JsonArray> run) : IModelClient
{
    public Task<JsonArray> RespondAsync(string model, JsonArray input, string instructions, Action<string> delta, Action<int> retry, CancellationToken ct)
    { ct.ThrowIfCancellationRequested(); return Task.FromResult(run(model, input, ct)); }
}
sealed class FakeHttp(Func<HttpRequestMessage, Task<HttpResponseMessage>> run) : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken ct) => run(request);
}
