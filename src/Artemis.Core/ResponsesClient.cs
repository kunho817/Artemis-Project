using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace Artemis.Core;

public interface IModelClient
{
    Task<JsonArray> RespondAsync(string model, JsonArray input, string instructions,
        Action<string> delta, Action<int> retry, CancellationToken ct);
}

public sealed class ResponsesClient(HttpClient http, Func<CancellationToken, Task<string>> accessToken) : IModelClient
{
    public static readonly string[] Models = ["gpt-6.1-sol", "gpt-6.1-terra"];
    public static JsonArray Tools => JsonNode.Parse("""
    [{"type":"namespace","name":"project","description":"Local project text tools. User scope only; writes require individual approval.","tools":[
      {"type":"function","name":"list_files","description":"List allowed immediate entries. Use path . for project root.","strict":true,"parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}},
      {"type":"function","name":"read_text_file","description":"Read UTF-8 text lines. start_line is 1-based; max_lines is 1 to 500.","strict":true,"parameters":{"type":"object","properties":{"path":{"type":"string"},"start_line":{"type":"integer"},"max_lines":{"type":"integer"}},"required":["path","start_line","max_lines"],"additionalProperties":false}},
      {"type":"function","name":"search_text","description":"Search literal text recursively under a relative directory path, returning paths and line numbers.","strict":true,"parameters":{"type":"object","properties":{"path":{"type":"string"},"query":{"type":"string"}},"required":["path","query"],"additionalProperties":false}},
      {"type":"function","name":"write_text_file","description":"Propose full UTF-8 contents of one file for creation or replacement in an existing directory. User sees before/after and approves each change.","strict":true,"parameters":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}}
    ]}]
    """)!.AsArray();

    public async Task<IReadOnlySet<string>> AvailableModelsAsync(CancellationToken ct)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, "https://api.openai.com/v1/models");
        request.Headers.Authorization = new("Bearer", await accessToken(ct));
        using var response = await http.SendAsync(request, ct);
        if (!response.IsSuccessStatusCode) throw await Failure(response, ct);
        var json = await response.Content.ReadFromJsonAsync<JsonElement>(ct);
        return json.GetProperty("models").EnumerateArray().Where(m => m.GetProperty("visibility").GetString() == "list")
            .Select(m => m.GetProperty("slug").GetString()!).ToHashSet(StringComparer.Ordinal);
    }

    public async Task<JsonArray> RespondAsync(string model, JsonArray input, string instructions,
        Action<string> delta, Action<int> retry, CancellationToken ct)
    {
        if (!Models.Contains(model)) throw new ArgumentException("선택하지 않은 모델입니다.");
        for (var attempt = 0; ; attempt++)
        {
            ct.ThrowIfCancellationRequested();
            using var request = new HttpRequestMessage(HttpMethod.Post, "https://api.openai.com/v1/responses");
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", await accessToken(ct));
            request.Content = JsonContent.Create(new JsonObject
            {
                ["model"] = model, ["input"] = input.DeepClone(), ["instructions"] = instructions,
                ["store"] = false, ["stream"] = true, ["include"] = new JsonArray("reasoning.encrypted_content"), ["tools"] = Tools
            });
            using var response = await http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, ct);
            // Only explicit transient HTTP rejections retry. Never replay a partially consumed stream.
            if ((response.StatusCode == HttpStatusCode.TooManyRequests || (int)response.StatusCode >= 500) && attempt < 5)
            {
                retry(attempt + 1);
                var delay = response.Headers.RetryAfter?.Delta ?? TimeSpan.FromSeconds(Math.Pow(2, attempt));
                await Task.Delay(delay, ct); continue;
            }
            if (!response.IsSuccessStatusCode) throw await Failure(response, ct);
            using var stream = await response.Content.ReadAsStreamAsync(ct);
            using var reader = new StreamReader(stream);
            return await ReadEventsAsync(reader, delta, ct);
        }
    }

    public static async Task<JsonArray> ReadEventsAsync(TextReader reader, Action<string> delta, CancellationToken ct)
    {
        var data = new List<string>();
        while (true)
        {
            var line = await reader.ReadLineAsync(ct);
            if (line == null && data.Count == 0) break;
            if (line is null or "")
            {
                if (data.Count == 0) continue;
                var json = string.Join('\n', data); data.Clear();
                if (json == "[DONE]") break;
                var e = JsonNode.Parse(json)!.AsObject(); var type = e["type"]?.GetValue<string>();
                switch (type)
                {
                    case "response.output_text.delta": delta(e["delta"]!.GetValue<string>()); break;
                    case "response.completed":
                        if (e["response"]?["status"]?.GetValue<string>() != "completed") throw new IOException("응답 완료 상태가 올바르지 않습니다.");
                        return e["response"]!["output"]!.DeepClone().AsArray();
                    case "response.failed": case "response.incomplete": case "error":
                        var code = e["response"]?["error"]?["code"]?.ToString() ?? e["code"]?.ToString() ?? type;
                        throw new IOException($"모델 응답 실패: {code}. 사용 권한·사용량을 확인하세요.");
                }
                if (line == null) break;
            }
            else if (line.StartsWith("data:")) data.Add(line[5..].TrimStart(' '));
        }
        throw new IOException("완료 이벤트 전에 연결이 종료되었습니다. 부분 응답은 완료되지 않았습니다.");
    }

    private static Task<IOException> Failure(HttpResponseMessage response, CancellationToken ct)
    {
        ct.ThrowIfCancellationRequested();
        var id = response.Headers.TryGetValues("x-request-id", out var ids) ? string.Join(",", ids) : "없음";
        return Task.FromResult(new IOException($"OpenAI 요청 실패: HTTP {(int)response.StatusCode}, 요청 ID {id}. 인증·모델 접근·사용량을 확인하세요."));
    }
}
