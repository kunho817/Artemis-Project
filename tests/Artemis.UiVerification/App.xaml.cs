using System.IO;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.Json.Nodes;
using System.Windows;
using System.Windows.Controls;
using Artemis.Core;

namespace Artemis.UiVerification;

// A separate test executable: never reads the real credential store and never opens a network socket.
public partial class App : Application
{
    private void OnStartup(object sender, StartupEventArgs e)
    {
        var directory = Path.Combine(Path.GetTempPath(), "Artemis-UiVerification-" + Guid.NewGuid().ToString("N"));
        var store = new CredentialStore(directory);
        var account = new Account("fixture-client", "fixture-subject", "synthetic@example.invalid", "fixture-access",
            null, null, ["chatgpt.tokens.use.direct"], DateTimeOffset.UtcNow.AddDays(1));
        store.Save(new("urn:uuid:" + Guid.NewGuid(), [account], account.ClientId));
        var handler = new VerificationTransport();
        var window = new Desktop.MainWindow(new HttpClient(handler) { Timeout = Timeout.InfiniteTimeSpan }, store,
            "Local\\Artemis.UiVerification." + Guid.NewGuid())
        { Title = "Artemis · 모의 오류 검증 · 실제 계정/네트워크 사용 없음" };
        window.Loaded += (_, _) =>
        {
            ((TextBlock)window.FindName("AuthLabel")).Text = "모의 계정 · 네트워크 사용 없음";
            ((TextBox)window.FindName("Transcript")).Text = "검증 전용 HTTP 응답입니다. 실제 계정·네트워크를 사용하지 않습니다.\n프로젝트 폴더를 선택한 뒤 아래 명령을 입력하세요.\nSTREAM_DROP: 부분 출력 후 연결 종료\nUSAGE_LIMIT: 부분 출력 후 사용량 제한 오류\nRETRY_429: HTTP 429 여섯 번(재시도 최대 5회)\nHOLD: 부분 출력 후 대기, 중단 버튼 검증\nOK: 정상 완료\n";
            foreach (var name in new[] { "SignInButton", "SignOutButton", "RefreshTokenButton", "AccountPicker" })
            {
                var control = (Control)window.FindName(name);
                control.IsEnabled = false;
                control.IsEnabledChanged += (_, _) => { if (control.IsEnabled) control.IsEnabled = false; };
            }
        };
        handler.RequestObserved = (command, count) => window.Dispatcher.Invoke(() =>
            window.Title = $"Artemis · 모의 오류 검증 · {command} HTTP 요청 {count}회 · 네트워크 없음");
        window.Show();
    }
}

internal sealed class VerificationTransport : HttpMessageHandler
{
    private string lastCommand = "";
    private int count;
    public Action<string, int>? RequestObserved { get; set; }
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken ct)
    {
        if (request.RequestUri?.AbsolutePath == "/v1/models")
            return Reply("{\"models\":[{\"slug\":\"gpt-6.1-sol\",\"visibility\":\"list\"},{\"slug\":\"gpt-6.1-terra\",\"visibility\":\"list\"}]}");
        if (request.RequestUri?.AbsolutePath != "/v1/responses") throw new HttpRequestException("모의 전송에서 지원하지 않는 경로입니다.");
        var body = JsonNode.Parse(await request.Content!.ReadAsStringAsync(ct))!;
        var command = body["input"]!.AsArray().OfType<JsonObject>().Last(o => o["role"]?.ToString() == "user")["content"]!.ToString().Trim();
        if (command != lastCommand) { count = 0; lastCommand = command; }
        RequestObserved?.Invoke(command, ++count);
        if (command == "RETRY_429")
        {
            var response = new HttpResponseMessage(HttpStatusCode.TooManyRequests);
            response.Headers.RetryAfter = new(TimeSpan.FromMilliseconds(500)); return response;
        }
        var delta = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"모의 부분 출력\"}\n\n";
        return command switch
        {
            "STREAM_DROP" => Reply(delta),
            "USAGE_LIMIT" => Reply(delta + "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\"}}}\n\n"),
            "HOLD" => new(HttpStatusCode.OK) { Content = new StreamContent(new HoldingStream(Encoding.UTF8.GetBytes(delta))) },
            "OK" => Reply("data: {\"type\":\"response.output_text.delta\",\"delta\":\"모의 정상 응답\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"모의 정상 응답\"}]}]}}\n\n"),
            _ => throw new HttpRequestException("모의 명령은 STREAM_DROP / USAGE_LIMIT / RETRY_429 / HOLD / OK입니다.")
        };
    }
    private static HttpResponseMessage Reply(string content) => new(HttpStatusCode.OK) { Content = new StringContent(content, Encoding.UTF8) };
}

internal sealed class HoldingStream(byte[] prefix) : Stream
{
    private int position;
    public override bool CanRead => true;
    public override bool CanSeek => false;
    public override bool CanWrite => false;
    public override long Length => throw new NotSupportedException();
    public override long Position { get => position; set => throw new NotSupportedException(); }
    public override async ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken ct = default)
    {
        if (position < prefix.Length)
        {
            var length = Math.Min(buffer.Length, prefix.Length - position);
            prefix.AsMemory(position, length).CopyTo(buffer); position += length; return length;
        }
        await Task.Delay(Timeout.Infinite, ct); return 0;
    }
    public override int Read(byte[] buffer, int offset, int count) => throw new NotSupportedException();
    public override void Flush() { }
    public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
    public override void SetLength(long value) => throw new NotSupportedException();
    public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();
}
