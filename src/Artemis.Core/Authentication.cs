using System.Diagnostics;
using System.Net;
using System.Net.Http.Json;
using System.Net.Sockets;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace Artemis.Core;

public sealed record Account(string ClientId, string Subject, string Email, string? AccessToken,
    string? RefreshToken, string? IdToken, string[] Scopes, DateTimeOffset ExpiresAt)
{
    public bool CanInvoke => AccessToken != null && Scopes.Contains("chatgpt.tokens.use.direct");
    public override string ToString() => $"{Email} · {ClientId}";
}
public sealed record AuthData(string HostId, List<Account> Accounts, string? ActiveClientId);

public sealed class CredentialStore
{
    private readonly string path;
    public CredentialStore(string directory) { Directory.CreateDirectory(directory); path = Path.Combine(directory, "accounts.dat"); }
    public AuthData Load() => File.Exists(path)
        ? JsonSerializer.Deserialize<AuthData>(ProtectedData.Unprotect(File.ReadAllBytes(path), null, DataProtectionScope.CurrentUser))
            ?? throw new IOException("인증 저장소를 읽을 수 없습니다.")
        : new("urn:uuid:" + Guid.NewGuid(), [], null);
    public void Save(AuthData data)
    {
        var bytes = ProtectedData.Protect(JsonSerializer.SerializeToUtf8Bytes(data), null, DataProtectionScope.CurrentUser);
        var temp = path + ".tmp";
        File.WriteAllBytes(temp, bytes); File.Move(temp, path, true);
    }
}

public sealed class Authentication : IDisposable
{
    private readonly HttpClient http;
    private readonly CredentialStore store;
    private readonly SemaphoreSlim gate = new(1);
    private readonly Mutex instanceLock;
    public AuthData Data { get; private set; }
    public Account? Active => Data.Accounts.Find(a => a.ClientId == Data.ActiveClientId);
    public Authentication(HttpClient http, CredentialStore store, string instanceLockName = "Local\\Artemis.Credentials.v1")
    {
        this.http = http; this.store = store;
        instanceLock = new Mutex(false, instanceLockName);
        var acquired = false;
        try { acquired = instanceLock.WaitOne(0); } catch (AbandonedMutexException) { acquired = true; }
        if (!acquired) { instanceLock.Dispose(); throw new IOException("다른 Artemis 창이 실행 중입니다."); }
        try { Data = store.Load(); store.Save(Data); }
        catch { instanceLock.ReleaseMutex(); instanceLock.Dispose(); throw; }
    }

    public void Select(string? clientId)
    {
        Data = Data with { ActiveClientId = clientId }; store.Save(Data);
    }

    private void Save(Account account)
    {
        var accounts = Data.Accounts.Where(a => a.ClientId != account.ClientId).ToList(); accounts.Add(account);
        var updated = Data with { Accounts = accounts, ActiveClientId = account.ClientId };
        store.Save(updated); Data = updated;
    }

    public async Task SignInAsync(Account? existing, CancellationToken ct)
    {
        await gate.WaitAsync(ct);
        try
        {
            var verifier = Random(); var state = Random(); var nonce = Random();
            var portPicker = new TcpListener(IPAddress.Loopback, 0); portPicker.Start();
            var port = ((IPEndPoint)portPicker.LocalEndpoint).Port; portPicker.Stop();
            var redirect = $"http://127.0.0.1:{port}/auth/callback";
            using var listener = new HttpListener(); listener.Prefixes.Add($"http://127.0.0.1:{port}/"); listener.Start();
            var query = new Dictionary<string, string>
            {
                ["client_id"] = existing?.ClientId ?? "dynamic_agent_client", ["ext_agent_host_id"] = Data.HostId,
                ["response_type"] = "code", ["redirect_uri"] = redirect,
                ["scope"] = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct",
                ["resource"] = "https://api.openai.com/v1", ["state"] = state, ["nonce"] = nonce,
                ["code_challenge_method"] = "S256", ["code_challenge"] = Base64(SHA256.HashData(Encoding.ASCII.GetBytes(verifier)))
            };
            if (existing == null) query["agent_name_hint"] = "Artemis";
            else if (existing.IdToken != null) query["id_token_hint"] = existing.IdToken;
            var url = "https://auth.openai.com/api/accounts/authorize?" + string.Join('&', query.Select(p => Uri.EscapeDataString(p.Key) + "=" + Uri.EscapeDataString(p.Value)));
            Process.Start(new ProcessStartInfo(url) { UseShellExecute = true });
            HttpListenerContext callback;
            while (true)
            {
                callback = await listener.GetContextAsync().WaitAsync(ct);
                if (callback.Request.Url?.AbsolutePath == "/auth/callback" && callback.Request.QueryString["state"] == state) break;
                callback.Response.StatusCode = 400; callback.Response.Close();
            }
            var values = callback.Request.QueryString;
            var message = Encoding.UTF8.GetBytes("Artemis received the sign-in callback. Return to the app to check the result.");
            callback.Response.ContentType = "text/plain; charset=utf-8";
            await callback.Response.OutputStream.WriteAsync(message, ct); callback.Response.Close();
            if (values["error"] != null) throw new IOException(values["error"] == "access_denied" ? "로그인이 취소되었거나 권한이 거부되었습니다." : "인증 서버가 로그인을 거부했습니다.");
            var client = values["client_id"] ?? existing?.ClientId;
            if (string.IsNullOrEmpty(client) || client == "dynamic_agent_client" || (existing != null && client != existing.ClientId))
                throw new IOException("OAuth 등록 ID가 올바르지 않습니다.");
            var code = values["code"] ?? throw new IOException("OAuth 코드가 없습니다.");
            var token = await Exchange(new() { ["grant_type"] = "authorization_code", ["client_id"] = client,
                ["code"] = code, ["code_verifier"] = verifier, ["redirect_uri"] = redirect, ["resource"] = "https://api.openai.com/v1" }, ct);
            var identity = await ValidateIdentityAsync(token.GetProperty("id_token").GetString()!, client, nonce, ct);
            if (existing != null && identity.Subject != existing.Subject) throw new IOException("선택한 계정과 인증 계정이 다릅니다.");
            Save(FromToken(token, client, identity.Subject, identity.Email, null));
        }
        finally { gate.Release(); }
    }

    public Task<string> AccessTokenAsync(CancellationToken ct) => AccessTokenAsync(ct, false);

    public async Task<string> AccessTokenAsync(CancellationToken ct, bool forceRefresh)
    {
        await gate.WaitAsync(ct);
        try
        {
            var account = Active ?? throw new IOException("먼저 ChatGPT로 로그인하세요.");
            if (!account.CanInvoke) throw new IOException("ChatGPT 플랜 사용 권한이 없습니다. 다시 로그인하여 권한을 허용하세요.");
            if (forceRefresh || account.ExpiresAt <= DateTimeOffset.UtcNow.AddMinutes(2))
            {
                if (account.RefreshToken == null) throw new IOException("다시 로그인해야 합니다.");
                var token = await Exchange(new() { ["grant_type"] = "refresh_token", ["client_id"] = account.ClientId,
                    ["refresh_token"] = account.RefreshToken, ["resource"] = "https://api.openai.com/v1" }, ct);
                if (token.TryGetProperty("id_token", out var id))
                {
                    var identity = await ValidateIdentityAsync(id.GetString()!, account.ClientId, null, ct);
                    if (identity.Subject != account.Subject) throw new IOException("갱신 계정이 일치하지 않습니다.");
                }
                account = FromToken(token, account.ClientId, account.Subject, account.Email, account); Save(account);
            }
            if (!account.CanInvoke) throw new IOException("ChatGPT 플랜 사용 권한이 해제되었습니다.");
            return account.AccessToken!;
        }
        finally { gate.Release(); }
    }

    private static Account FromToken(JsonElement t, string client, string subject, string email, Account? old)
    {
        string? Get(string key) => t.TryGetProperty(key, out var p) ? p.GetString() : null;
        if (!string.Equals(Get("token_type"), "Bearer", StringComparison.OrdinalIgnoreCase)) throw new IOException("지원하지 않는 토큰 형식입니다.");
        var access = Get("access_token") ?? throw new IOException("액세스 토큰이 없습니다.");
        return new(client, subject, email, access, Get("refresh_token") ?? old?.RefreshToken, Get("id_token") ?? old?.IdToken,
            Get("scope")?.Split(' ', StringSplitOptions.RemoveEmptyEntries) ?? old?.Scopes ?? [],
            DateTimeOffset.UtcNow.AddSeconds(t.GetProperty("expires_in").GetInt32()));
    }

    private async Task<JsonElement> Exchange(Dictionary<string, string> form, CancellationToken ct)
    {
        using var response = await http.PostAsync("https://auth.openai.com/api/accounts/oauth/token", new FormUrlEncodedContent(form), ct);
        // Token response bodies must never appear in diagnostics.
        if (!response.IsSuccessStatusCode) throw new IOException($"토큰 요청 실패 (HTTP {(int)response.StatusCode}). 다시 로그인하세요.");
        return await response.Content.ReadFromJsonAsync<JsonElement>(ct);
    }

    public async Task<bool> SignOutAsync(CancellationToken ct)
    {
        await gate.WaitAsync(ct);
        try
        {
            var account = Active; if (account == null) return true;
            var revoked = account.RefreshToken == null;
            try
            {
                if (!revoked)
                {
                    var config = await http.GetFromJsonAsync<JsonElement>("https://auth.openai.com/.well-known/openid-configuration", ct);
                    var endpoint = TrustedUri(config.GetProperty("revocation_endpoint").GetString()!);
                    using var response = await http.PostAsync(endpoint, new FormUrlEncodedContent(new Dictionary<string, string>
                    { ["token"] = account.RefreshToken!, ["token_type_hint"] = "refresh_token", ["client_id"] = account.ClientId }), ct);
                    revoked = response.IsSuccessStatusCode;
                }
            }
            catch (Exception ex) when (ex is HttpRequestException or IOException or OperationCanceledException or
                JsonException or KeyNotFoundException or InvalidOperationException or ArgumentException or FormatException)
            { revoked = false; }
            finally
            {
                // Stop using credentials in memory even if persisting the local signout fails.
                var cleared = account with { AccessToken = null, RefreshToken = null, IdToken = null, Scopes = [], ExpiresAt = default };
                Data = Data with { Accounts = Data.Accounts.Select(a => a.ClientId == account.ClientId ? cleared : a).ToList() };
                try { store.Save(Data); }
                catch (Exception ex) when (ex is IOException or UnauthorizedAccessException or CryptographicException)
                { throw new IOException("메모리에서 연결 해제했지만 로컬 토큰 제거를 저장하지 못했습니다. 저장소 권한·디스크 상태를 확인하세요.", ex); }
            }
            return revoked;
        }
        finally { gate.Release(); }
    }

    private async Task<(string Subject, string Email)> ValidateIdentityAsync(string jwt, string client, string? nonce, CancellationToken ct)
    {
        var config = await http.GetFromJsonAsync<JsonElement>("https://auth.openai.com/.well-known/openid-configuration", ct);
        var keys = await http.GetFromJsonAsync<JsonElement>(TrustedUri(config.GetProperty("jwks_uri").GetString()!), ct);
        return ValidateIdentity(jwt, keys, client, nonce, DateTimeOffset.UtcNow);
    }

    public static (string Subject, string Email) ValidateIdentity(string jwt, JsonElement jwks, string client, string? nonce, DateTimeOffset now)
    {
        var parts = jwt.Split('.'); if (parts.Length != 3) throw new IOException("ID 토큰 형식 오류.");
        using var header = JsonDocument.Parse(Unbase64(parts[0])); using var payload = JsonDocument.Parse(Unbase64(parts[1]));
        var h = header.RootElement; var p = payload.RootElement;
        if (h.GetProperty("alg").GetString() != "RS256") throw new IOException("ID 토큰 서명 알고리즘 오류.");
        var key = jwks.GetProperty("keys").EnumerateArray().First(k => k.GetProperty("kid").GetString() == h.GetProperty("kid").GetString());
        using var rsa = RSA.Create(); rsa.ImportParameters(new() { Modulus = Unbase64(key.GetProperty("n").GetString()!), Exponent = Unbase64(key.GetProperty("e").GetString()!) });
        if (!rsa.VerifyData(Encoding.ASCII.GetBytes(parts[0] + "." + parts[1]), Unbase64(parts[2]), HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1))
            throw new IOException("ID 토큰 서명 검증 실패.");
        var aud = p.GetProperty("aud");
        var validAudience = aud.ValueKind == JsonValueKind.String ? aud.GetString() == client : aud.EnumerateArray().Any(a => a.GetString() == client);
        if (p.GetProperty("iss").GetString() != "https://auth.openai.com" || !validAudience ||
            p.GetProperty("exp").GetInt64() <= now.ToUnixTimeSeconds() ||
            (p.TryGetProperty("nbf", out var nbf) && nbf.GetInt64() > now.ToUnixTimeSeconds() + 60) ||
            (aud.ValueKind == JsonValueKind.Array && aud.GetArrayLength() > 1 && !p.TryGetProperty("azp", out _)) ||
            (nonce != null && (!p.TryGetProperty("nonce", out var n) || n.GetString() != nonce)) ||
            (p.TryGetProperty("azp", out var azp) && azp.GetString() != client)) throw new IOException("ID 토큰 클레임 검증 실패.");
        var subject = p.GetProperty("sub").GetString();
        if (string.IsNullOrWhiteSpace(subject)) throw new IOException("ID 토큰 계정 정보 누락.");
        return (subject, p.TryGetProperty("email", out var email) ? email.GetString() ?? subject : subject);
    }

    private static Uri TrustedUri(string value)
    {
        var uri = new Uri(value);
        if (uri.Scheme != "https" || uri.Host != "auth.openai.com" || !uri.IsDefaultPort) throw new IOException("인증 서버 주소 오류.");
        return uri;
    }
    public static string Base64(byte[] bytes) => Convert.ToBase64String(bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_');
    private static byte[] Unbase64(string value) { var s = value.Replace('-', '+').Replace('_', '/'); return Convert.FromBase64String(s.PadRight((s.Length + 3) / 4 * 4, '=')); }
    private static string Random() => Base64(RandomNumberGenerator.GetBytes(32));
    public void Dispose() { instanceLock.ReleaseMutex(); instanceLock.Dispose(); gate.Dispose(); }
}
