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
var fullScope = new RequestScope(files, ["."], true, true);

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
        var h = new Harness(fake, files); await h.RunAsync("read sample", model, fullScope, ct); await h.RunAsync("follow up", model, fullScope, ct);
        Check(h.State == RunState.Completed && turn == 3);
    }
});
AsyncTest("Denied write produces denied call output without mutation", async () =>
{
    var turn = 0; var confirmed = false;
    var fake = new FakeModel((_, input, _) => ++turn == 1 ? new(Call("write_text_file", new { path = "denied.txt", content = "no" })) :
        input.Last()!["output"]!.ToString().Contains("denied") ? Answer() : throw new Exception("Missing denial"));
    var h = new Harness(fake, files) { Confirm = (p, _) => { Check(p.Before == null && !File.Exists(Path.Combine(root, p.Path))); confirmed = true; return Task.FromResult(false); } };
    await h.RunAsync("write", "gpt-6.1-sol", fullScope, ct); Check(confirmed && !File.Exists(Path.Combine(root, "denied.txt")));
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
        await h.RunAsync("write", "gpt-6.1-sol", fullScope, ct); Check(files.Read("approved.txt") == (conflict ? "external" : "after"));
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
    await new Harness(fake, files).RunAsync("read", "gpt-6.1-sol", fullScope, ct);
});
AsyncTest("Stop during approval prevents write, second tool and followup request", async () =>
{
    using var source = new CancellationTokenSource(); var calls = 0; var results = 0;
    var fake = new FakeModel((_, _, _) => { calls++; return new(Call("write_text_file", new { path = "stopped.txt", content = "no" }), Call("read_text_file", new { path = "sample.txt", start_line = 1, max_lines = 1 }, "c2")); });
    var h = new Harness(fake, files) { Confirm = (_, _) => { source.Cancel(); return Task.FromResult(true); } };
    h.ToolFinished += _ => results++;
    await RejectAsync(() => h.RunAsync("write", "gpt-6.1-sol", fullScope, source.Token));
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

Test("Forced refresh rotates persisted tokens while ordinary access reuses a valid token", () =>
{
    var store = new CredentialStore(Path.Combine(temp, "refresh-store"));
    var account = new Account("fixture-client", "sub", "test@example.invalid", "old-access", "old-refresh", null,
        ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
    store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
    var calls = 0;
    using var http = new HttpClient(new FakeHttp(async request =>
    {
        calls++; var form = await request.Content!.ReadAsStringAsync();
        Check(form.Contains("grant_type=refresh_token") && form.Contains("refresh_token=old-refresh") && form.Contains("client_id=fixture-client"));
        return new(HttpStatusCode.OK) { Content = new StringContent("{\"token_type\":\"Bearer\",\"access_token\":\"rotated-access\",\"refresh_token\":\"rotated-refresh\",\"expires_in\":3600}") };
    }));
    using var auth = new Authentication(http, store, "Local\\Artemis.Test." + Guid.NewGuid());
    Check(auth.AccessTokenAsync(ct).GetAwaiter().GetResult() == "old-access" && calls == 0);
    Check(auth.AccessTokenAsync(ct, true).GetAwaiter().GetResult() == "rotated-access" && calls == 1);
    var saved = store.Load().Accounts[0];
    Check(saved.RefreshToken == "rotated-refresh" && saved.CanInvoke && saved.ExpiresAt > DateTimeOffset.UtcNow.AddMinutes(50));
});
Test("Signout clears credentials and retains registration after unconfirmed revocation", () =>
{
    var store = new CredentialStore(Path.Combine(temp, "signout-store"));
    var account = new Account("fixture-client", "sub", "test@example.invalid", "fake-access", "fake-refresh", "fake-id",
        ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
    store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
    using var http = new HttpClient(new FakeHttp(request => Task.FromResult(request.Method == HttpMethod.Get
        ? new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent("{\"revocation_endpoint\":\"https://auth.openai.com/revoke\"}") }
        : new HttpResponseMessage(HttpStatusCode.ServiceUnavailable))));
    using var auth = new Authentication(http, store, "Local\\Artemis.Test." + Guid.NewGuid());
    Check(!auth.SignOutAsync(ct).GetAwaiter().GetResult());
    var saved = store.Load().Accounts[0];
    Check(saved.ClientId == account.ClientId && saved.Subject == account.Subject && saved.AccessToken == null && saved.RefreshToken == null && saved.IdToken == null && !saved.CanInvoke);
});

AsyncTest("Request-scoped read/list/search reject unrelated files and writes before confirmation", async () =>
{
    Directory.CreateDirectory(Path.Combine(root, "scoped"));
    File.WriteAllText(Path.Combine(root, "scoped/requested.txt"), "ALLOWED_BODY");
    File.WriteAllText(Path.Combine(root, "scoped/unrelated.txt"), "FORBIDDEN_BODY");
    var scope = new RequestScope(files, ["scoped/requested.txt"], true, false);
    var turn = 0; var approvals = 0;
    var fake = new FakeModel((_, input, _) =>
    {
        if (++turn == 1) return new(
            Call("read_text_file", new { path = "scoped/unrelated.txt", start_line = 1, max_lines = 10 }, "out"),
            Call("list_files", new { path = "scoped" }, "list"),
            Call("search_text", new { path = ".", query = "BODY" }, "search"),
            Call("read_text_file", new { path = "scoped/requested.txt", start_line = 1, max_lines = 10 }, "in"),
            Call("write_text_file", new { path = "scoped/requested.txt", content = "wrong" }, "write"));
        var results = input.OfType<JsonObject>().Where(o => o["type"]?.ToString() == "function_call_output")
            .ToDictionary(o => o["call_id"]!.ToString(), o => JsonNode.Parse(o["output"]!.ToString())!);
        Check(results["out"]["status"]!.ToString() == "error" && results["write"]["status"]!.ToString() == "error");
        Check(results.Values.All(r => !r.ToJsonString().Contains("FORBIDDEN_BODY")));
        Check(results["list"]["entries"]!.AsArray().Count == 1 && !results["list"].ToJsonString().Contains("unrelated"));
        Check(results["search"]["hits"]!.AsArray().Count == 1 && !results["search"].ToJsonString().Contains("unrelated"));
        Check(results["in"]["text"]!.ToString() == "ALLOWED_BODY"); return Answer();
    });
    var h = new Harness(fake, files) { Confirm = (_, _) => { approvals++; return Task.FromResult(true); } };
    await h.RunAsync("Read only requested.txt; exclude unrelated.txt", "gpt-6.1-sol", scope, ct);
    Check(approvals == 0 && files.Read("scoped/requested.txt") == "ALLOWED_BODY");
});
Test("Empty, read-disabled, directory-boundary and immutable request scopes", () =>
{
    var empty = new RequestScope(files, [], true, true);
    Reject(() => empty.EnsureFile(files, "sample.txt")); Reject(() => files.List(".", ct, empty));
    Reject(() => files.Search(".", "alpha", ct, empty)); Reject(() => empty.EnsureFile(files, "new.txt", true));
    var writeOnly = new RequestScope(files, ["sample.txt"], false, true);
    Reject(() => writeOnly.EnsureFile(files, "sample.txt")); Check(writeOnly.AllowsFile(files, "sample.txt", true));
    var paths = new List<string> { "scoped/sub/" };
    var folder = new RequestScope(files, paths, true, true); paths.Add(".");
    Check(folder.AllowsFile(files, "scoped/sub/new.txt", true));
    Check(!folder.AllowsFile(files, "scoped/sub-sibling/new.txt") && !folder.AllowsFile(files, "sample.txt"));
    Directory.CreateDirectory(Path.Combine(root, "scoped/sub"));
    Directory.CreateDirectory(Path.Combine(root, "scoped/sub-sibling"));
    File.WriteAllText(Path.Combine(root, "scoped/sub/inside.txt"), "INSIDE_BODY");
    File.WriteAllText(Path.Combine(root, "scoped/sub-sibling/outside.txt"), "OUTSIDE_BODY");
    var hits = files.Search(".", "BODY", ct, folder).Hits;
    Check(hits.Count == 1 && hits[0].Text == "INSIDE_BODY");
    Reject(() => new RequestScope(files, ["../outside/"], true, true));
});
AsyncTest("Scope changes do not replay content from an earlier broader turn", async () =>
{
    var turn = 0;
    var fake = new FakeModel((_, input, _) =>
    {
        turn++;
        if (turn == 1) return new(Call("read_text_file", new { path = "scoped/unrelated.txt", start_line = 1, max_lines = 1 }));
        if (turn == 2) Check(input.Last()!["output"]!.ToString().Contains("FORBIDDEN_BODY"));
        if (turn == 3) Check(input.Count == 1 && !input.ToJsonString().Contains("FORBIDDEN_BODY"));
        return Answer();
    });
    var h = new Harness(fake, files);
    await h.RunAsync("Initial broad fixture read", "gpt-6.1-sol", fullScope, ct);
    await h.RunAsync("Now only requested.txt", "gpt-6.1-sol", new RequestScope(files, ["scoped/requested.txt"], true, false), ct);
    Check(turn == 3);
    await RejectAsync(() => h.RunAsync("Wrong project scope", "gpt-6.1-sol", new RequestScope(new FileTools(outside), ["."], true, true), ct));
    Check(turn == 3, "Mismatched project scope must be rejected before a model request");
});
Test("Malformed discovery metadata never skips local signout", () =>
{
    foreach (var body in new[] { "{}", "not-json", "[]", "{\"revocation_endpoint\":null}", "{\"revocation_endpoint\":17}",
        "{\"revocation_endpoint\":\"not-a-uri\"}", "{\"revocation_endpoint\":\"https://untrusted.invalid/revoke\"}" })
    {
        var store = new CredentialStore(Path.Combine(temp, "malformed-logout-" + Guid.NewGuid().ToString("N")));
        var account = new Account("fixture-client", "sub", "test@example.invalid", "fake-access", "fake-refresh", "fake-id",
            ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
        store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
        using var http = new HttpClient(new FakeHttp(_ => Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent(body) })));
        using var auth = new Authentication(http, store, "Local\\Artemis.Test." + Guid.NewGuid());
        Check(!auth.SignOutAsync(ct).GetAwaiter().GetResult());
        var saved = store.Load().Accounts[0];
        Check(saved.ClientId == account.ClientId && saved.Subject == account.Subject && saved.AccessToken == null && saved.RefreshToken == null && saved.IdToken == null && !auth.Active!.CanInvoke);
    }
});
Test("Local signout storage failure is explicit and credentials are disabled in memory", () =>
{
    var directory = Path.Combine(temp, "blocked-logout"); var store = new CredentialStore(directory);
    var account = new Account("fixture-client", "sub", "test@example.invalid", "fake-access", null, "fake-id",
        ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddHours(1));
    store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
    using var http = new HttpClient(new FakeHttp(_ => throw new Exception("No remote call expected")));
    using var auth = new Authentication(http, store, "Local\\Artemis.Test." + Guid.NewGuid());
    Directory.CreateDirectory(Path.Combine(directory, "accounts.dat.tmp"));
    try { auth.SignOutAsync(ct).GetAwaiter().GetResult(); throw new Exception("Expected persistence failure"); }
    catch (IOException ex) { Check(ex.Message.Contains("저장하지 못했습니다")); }
    Check(auth.Active!.AccessToken == null && !auth.Active.CanInvoke);
    Check(store.Load().Accounts[0].AccessToken == "fake-access", "The failed persistence must not be reported as successful removal");
});
AsyncTest("Retry-After HTTP dates prevent early retries and elapsed dates retry immediately", async () =>
{
    foreach (var future in new[] { true, false })
    {
        var clock = new ManualClock(); var calls = 0;
        using var http = new HttpClient(new FakeHttp(_ =>
        {
            if (++calls > 1) return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK) { Content = new StringContent(Complete(Answer())) });
            var response = new HttpResponseMessage(HttpStatusCode.TooManyRequests);
            response.Headers.RetryAfter = new(clock.GetUtcNow().AddSeconds(future ? 5 : -5)); return Task.FromResult(response);
        }));
        var client = new ResponsesClient(http, _ => Task.FromResult("fixture"), clock);
        var pending = client.RespondAsync("gpt-6.1-sol", [], "", _ => { }, _ => { }, ct);
        if (future)
        {
            Check(calls == 1 && !pending.IsCompleted && clock.Delays.Single() == TimeSpan.FromSeconds(5));
            clock.Advance(TimeSpan.FromSeconds(4)); Check(calls == 1 && !pending.IsCompleted);
            clock.Advance(TimeSpan.FromSeconds(1));
        }
        await pending.WaitAsync(TimeSpan.FromSeconds(5));
        Check(calls == 2 && (future || clock.Delays.Count == 0));
    }
});
AsyncTest("Retry-After delta, fallback backoff and waiting cancellation remain supported", async () =>
{
    foreach (var delta in new[] { true, false })
    {
        var clock = new ManualClock(); var calls = 0;
        using var http = new HttpClient(new FakeHttp(_ =>
        {
            calls++; var response = new HttpResponseMessage(HttpStatusCode.ServiceUnavailable);
            if (delta) response.Headers.RetryAfter = new(TimeSpan.FromSeconds(3)); return Task.FromResult(response);
        }));
        using var source = new CancellationTokenSource();
        var pending = new ResponsesClient(http, _ => Task.FromResult("fixture"), clock)
            .RespondAsync("gpt-6.1-sol", [], "", _ => { }, _ => { }, source.Token);
        Check(clock.Delays.Single() == TimeSpan.FromSeconds(delta ? 3 : 1) && calls == 1);
        source.Cancel(); await RejectAsync(() => pending);
        clock.Advance(TimeSpan.FromHours(1)); Check(calls == 1);
    }
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

sealed class ManualClock : TimeProvider
{
    private DateTimeOffset now = new(2026, 10, 28, 0, 0, 0, TimeSpan.Zero);
    private readonly List<ManualTimer> timers = [];
    public List<TimeSpan> Delays { get; } = [];
    public override DateTimeOffset GetUtcNow() => now;
    public override ITimer CreateTimer(TimerCallback callback, object? state, TimeSpan dueTime, TimeSpan period)
    {
        if (period != Timeout.InfiniteTimeSpan) throw new NotSupportedException();
        var timer = new ManualTimer(callback, state, now + dueTime); timers.Add(timer); Delays.Add(dueTime); return timer;
    }
    public void Advance(TimeSpan span)
    {
        now += span;
        foreach (var timer in timers.ToArray()) if (!timer.Disposed && timer.Due <= now) timer.Fire();
    }
    private sealed class ManualTimer(TimerCallback callback, object? state, DateTimeOffset due) : ITimer
    {
        public DateTimeOffset Due { get; } = due;
        public bool Disposed { get; private set; }
        public void Fire() { Disposed = true; callback(state); }
        public bool Change(TimeSpan dueTime, TimeSpan period) => throw new NotSupportedException();
        public void Dispose() => Disposed = true;
        public ValueTask DisposeAsync() { Dispose(); return ValueTask.CompletedTask; }
    }
}
