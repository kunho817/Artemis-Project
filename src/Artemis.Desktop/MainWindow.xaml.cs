using System.ComponentModel;
using System.Diagnostics;
using System.Net.Http;
using System.Windows;
using System.Windows.Controls;
using Artemis.Core;
using Microsoft.Win32;

namespace Artemis.Desktop;

public partial class MainWindow : Window
{
    private readonly HttpClient http;
    private Authentication? auth;
    private ResponsesClient? responses;
    private Harness? harness;
    private FileTools? files;
    private CancellationTokenSource? operation;
    private TaskCompletionSource<bool>? approval;
    private IReadOnlySet<string> catalog = new HashSet<string>();
    private bool loadingAccounts;
    private bool closeAfterStop;
    public MainWindow() : this(null, null) { }

    // The separate UI verification executable supplies a synthetic transport and an isolated store.
    public MainWindow(HttpClient? client, CredentialStore? credentials, string instanceLockName = "Local\\Artemis.Credentials.v1")
    {
        http = client ?? new(new HttpClientHandler { AllowAutoRedirect = false }) { Timeout = Timeout.InfiniteTimeSpan };
        InitializeComponent();
        try
        {
            auth = new(http, credentials ?? new CredentialStore(System.IO.Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "Artemis")), instanceLockName);
            responses = new(http, auth.AccessTokenAsync); ReloadAccounts();
            Loaded += async (_, _) => { if (auth.Active?.CanInvoke == true) await RefreshCatalog(); };
        }
        catch (Exception ex)
        {
            MessageBox.Show(this, ex.Message, "Artemis 시작 실패", MessageBoxButton.OK, MessageBoxImage.Error);
            Loaded += (_, _) => Close();
        }
    }

    private void ReloadAccounts()
    {
        if (auth == null) return;
        loadingAccounts = true;
        AccountPicker.Items.Clear(); AccountPicker.Items.Add("새 계정 추가");
        foreach (var account in auth.Data.Accounts) AccountPicker.Items.Add(account);
        AccountPicker.SelectedItem = (object?)auth.Active ?? "새 계정 추가";
        loadingAccounts = false;
        AuthLabel.Text = auth.Active?.AccessToken == null ? "연결 안 됨" : auth.Active.CanInvoke ? "연결됨 · ChatGPT 플랜 사용" : "로그인됨 · 플랜 권한 없음";
    }

    private void RebuildHarness()
    {
        if (files == null || responses == null) return;
        harness = new(responses, files);
        harness.TextDelta += text => Dispatcher.Invoke(() => { Transcript.AppendText(text); Transcript.ScrollToEnd(); });
        harness.StateChanged += state => Dispatcher.Invoke(() => StatusLabel.Text = state switch
        {
            RunState.Running => "실행 중", RunState.AwaitingApproval => "사용자 확인 대기 · 파일은 아직 변경되지 않았습니다.",
            RunState.Completed => "완료 · 모델 최종 응답을 수신했습니다.", RunState.Stopped => "중단 · 실제 적용 내역은 도구 실행 기록을 확인하세요.",
            RunState.Failed => "실패 · 부분 응답은 완료로 처리되지 않았습니다.", _ => "대기"
        });
        harness.Retrying += n => Dispatcher.Invoke(() => StatusLabel.Text = $"모델 요청 재시도 {n}/5");
        harness.ToolFinished += record => Dispatcher.Invoke(() =>
        {
            ToolLog.AppendText($"{record.Name} · {record.Path}\n{record.Result}\n\n"); ToolLog.ScrollToEnd();
        });
        harness.Confirm = async (proposal, ct) =>
        {
            var pending = new TaskCompletionSource<bool>(TaskCreationOptions.RunContinuationsAsynchronously);
            await Dispatcher.InvokeAsync(() =>
            {
                approval = pending;
                ChangePreview.Text = $"대상: {proposal.Path}\n\n변경 전\n────────\n{proposal.Before ?? "(새 파일)"}\n\n변경 후\n────────\n{proposal.After}";
                ApproveButton.IsEnabled = DenyButton.IsEnabled = true;
            });
            try { return await pending.Task.WaitAsync(ct); }
            finally
            {
                await Dispatcher.InvokeAsync(() =>
                {
                    approval = null; ApproveButton.IsEnabled = DenyButton.IsEnabled = false;
                    ChangePreview.AppendText(ct.IsCancellationRequested ? "\n\n확인 취소 · 중단됨" : "\n\n확인 종료");
                });
            }
        };
    }

    private void ResetConversation()
    {
        ScopePaths.Clear(); ReadScope.IsChecked = true; WriteScope.IsChecked = false;
        Transcript.Clear(); ToolLog.Clear(); ChangePreview.Text = "승인할 변경이 없습니다."; RebuildHarness();
    }
    private void ChooseProject_Click(object sender, RoutedEventArgs e)
    {
        var picker = new OpenFolderDialog { Title = "Artemis 작업 프로젝트 선택" };
        if (picker.ShowDialog(this) != true) return;
        try { files = new FileTools(picker.FolderName); RootLabel.Text = files.Root; ResetConversation(); StatusLabel.Text = "프로젝트 선택됨 · 새 대화"; }
        catch (Exception ex) { StatusLabel.Text = ex.Message; }
    }
    private async void SignIn_Click(object sender, RoutedEventArgs e) => await WithOperation(async ct =>
    {
        StatusLabel.Text = "브라우저에서 인증을 완료하세요. 중단 버튼으로 취소할 수 있습니다.";
        await auth!.SignInAsync(AccountPicker.SelectedItem as Account, ct); ReloadAccounts(); ResetConversation();
        await LoadCatalog(ct); StatusLabel.Text = "로그인 완료";
    });
    private async void SignOut_Click(object sender, RoutedEventArgs e) => await WithOperation(async ct =>
    {
        bool revoked;
        try { revoked = await auth!.SignOutAsync(ct); }
        finally
        {
            catalog = new HashSet<string>(); ReloadAccounts(); ResetConversation();
            CatalogLabel.Text = auth!.Active?.CanInvoke == true ? "모델 목록을 다시 확인하세요." : "연결 해제됨";
        }
        StatusLabel.Text = revoked ? "연결 해제 완료" : "로컬 연결 해제 완료 · 원격 해제는 확인되지 않았습니다. ChatGPT 설정에서 앱을 해제할 수 있습니다.";
    });
    private async void AccountPicker_SelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (loadingAccounts || auth == null) return;
        try
        {
            auth.Select((AccountPicker.SelectedItem as Account)?.ClientId); catalog = new HashSet<string>();
            ReloadAccounts(); ResetConversation();
            if (auth.Active?.CanInvoke == true) await RefreshCatalog(); else CatalogLabel.Text = "로그인이 필요합니다.";
        }
        catch (Exception ex) { StatusLabel.Text = ex.Message; }
    }
    private async Task LoadCatalog(CancellationToken ct)
    {
        catalog = new HashSet<string>(); catalog = await responses!.AvailableModelsAsync(ct);
        CatalogLabel.Text = string.Join("\n", ResponsesClient.Models.Select(m => $"{m}: {(catalog.Contains(m) ? "목록에 있음 · 실제 호출 미검증" : "현재 계정 목록에 없음")}"));
    }
    private async Task RefreshCatalog() => await WithOperation(async ct =>
    {
        StatusLabel.Text = "모델 목록 확인 중";
        await LoadCatalog(ct); StatusLabel.Text = "모델 목록 확인 완료";
    });
    private async void RefreshCatalog_Click(object sender, RoutedEventArgs e) => await RefreshCatalog();
    private async void RefreshToken_Click(object sender, RoutedEventArgs e) => await WithOperation(async ct =>
    {
        StatusLabel.Text = "연결 토큰 갱신 중";
        await auth!.AccessTokenAsync(ct, true); ReloadAccounts();
        await LoadCatalog(ct); StatusLabel.Text = "토큰 갱신 완료 · 모델 목록 확인 완료";
    });
    private async void Send_Click(object sender, RoutedEventArgs e)
    {
        if (harness == null || auth?.Active?.CanInvoke != true) { StatusLabel.Text = "프로젝트 선택과 플랜 사용 권한이 있는 로그인이 필요합니다."; return; }
        var model = ((ComboBoxItem)ModelPicker.SelectedItem).Content.ToString()!;
        if (!catalog.Contains(model)) { StatusLabel.Text = "계정 모델 목록에 없습니다. 로그인·모델 목록을 확인하세요."; return; }
        var text = Prompt.Text.Trim(); if (text.Length == 0) return;
        RequestScope scope;
        try { scope = new(files!, ScopePaths.Text.Split('\n'), ReadScope.IsChecked == true, WriteScope.IsChecked == true); }
        catch (Exception ex) { StatusLabel.Text = "요청 범위 오류 · " + ex.Message; return; }
        Prompt.Clear(); ScopePaths.Clear(); WriteScope.IsChecked = false;
        Transcript.AppendText($"\n사용자\n{text}\n파일 범위: {scope.Description}\n\nArtemis · {model}\n");
        await WithOperation(ct => Task.Run(() => harness.RunAsync(text, model, scope, ct), ct));
        Transcript.AppendText("\n");
    }
    private async Task WithOperation(Func<CancellationToken, Task> action)
    {
        if (operation != null) return;
        using var source = new CancellationTokenSource(); operation = source; SetBusy(true);
        try { await action(source.Token); }
        catch (OperationCanceledException) { StatusLabel.Text = "중단됨 · 부분 응답과 도구 실행 내역을 확인하세요."; }
        catch (Exception ex) { StatusLabel.Text = "실패 · " + ex.Message; Transcript.AppendText("\n[실패] " + ex.Message + "\n"); }
        finally { operation = null; SetBusy(false); if (closeAfterStop) Close(); }
    }
    private void SetBusy(bool busy)
    {
        foreach (var control in new Control[] { ProjectButton, AccountPicker, SignInButton, SignOutButton, RefreshTokenButton, ModelPicker, SendButton, Prompt, ScopePaths, ReadScope, WriteScope, CatalogButton, NewConversationButton }) control.IsEnabled = !busy;
        StopButton.IsEnabled = busy;
    }
    private void Stop_Click(object sender, RoutedEventArgs e) { operation?.Cancel(); approval?.TrySetCanceled(); }
    private void Approve_Click(object sender, RoutedEventArgs e) => approval?.TrySetResult(true);
    private void Deny_Click(object sender, RoutedEventArgs e) => approval?.TrySetResult(false);
    private void NewConversation_Click(object sender, RoutedEventArgs e) { ResetConversation(); StatusLabel.Text = "새 대화 · 이전 대화는 저장되지 않습니다."; }
    private void Usage_Click(object sender, RoutedEventArgs e) => Process.Start(new ProcessStartInfo("https://chatgpt.com/settings/usage") { UseShellExecute = true });
    private void Window_Closing(object? sender, CancelEventArgs e)
    {
        if (operation != null) { e.Cancel = true; closeAfterStop = true; operation.Cancel(); approval?.TrySetCanceled(); return; }
        auth?.Dispose(); http.Dispose();
    }
}
