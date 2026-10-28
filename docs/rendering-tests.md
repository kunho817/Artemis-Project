# 창을 표시하지 않는 WPF 검증

`Artemis.RenderingTests`는 실제 MainWindow XAML과 이벤트 처리기·하네스를 별도 STA 프로세스에서 실행한다. 창의 콘텐츠를 분리해 소프트웨어 렌더링으로 PNG를 만든다. `Window.Show`, 데스크톱 캡처, 키보드·마우스 입력이나 Computer Use를 사용하지 않는다. Windows와 .NET 8 SDK가 필요하다.

## 실행

프로젝트 루트에서 실행한다. 기본 출력 경로는 Git에서 제외된 `test-results/rendering`이다.

```powershell
dotnet run --project tests/Artemis.RenderingTests -c Release
```

출력은 상태별 PNG 54개, 검사 결과 `results.json`, 이미지 갤러리 `index.html`이다. 실패하면 종료 코드 1을 반환한다. 갤러리는 필요할 때 사용자가 직접 연다.

기존 이미지와 비교하려면 서로 다른 출력·기준 디렉터리를 지정한다. 기준 이미지는 덮어쓰지 않는다.

```powershell
dotnet run --project tests/Artemis.RenderingTests -c Release -- test-results/rendering-next test-results/rendering
```

기준 이미지가 없거나 크기가 다르면 실패한다. BGRA 채널 중 하나라도 차이가 12를 넘는 픽셀이 전체의 0.5%를 넘으면 실패한다. 기준 이미지는 사람이 확인한 버전을 별도로 보존하는 방식으로 관리한다. 현재 저장소에 승인된 기준 이미지는 없으며, 이번 두 번의 실행 비교는 같은 환경에서의 반복 가능성 확인이다. 글꼴·Windows·WPF 버전 차이는 별도 검토가 필요하다.

## 검사 범위

- 실제 WPF 버튼 이벤트로 빈 범위 차단, 허용 범위 조회, 전송 후 권한 초기화, 승인 대기, 거부, 승인 쓰기, 외부 변경 충돌, 승인 대기/스트리밍 중단, 사용량 오류와 복구, 잘못된 인증 메타데이터에서도 연결 해제를 검사한다.
- 일반 동작 14개와 검사기 자체의 음성 대조 2개를 합쳐 16개다. 화면 밖으로 버튼을 옮기거나 텍스트 높이를 줄이면 검사기가 탐지하는지 확인한 뒤 원복한다.
- 대기·승인·거부·충돌·스트리밍·중단·오류·완료·연결 해제 9상태를 콘텐츠 크기 1050×688, 1380×868, 1920×1048 DIP와 96/144 DPI 조합으로 렌더링한다. 이 크기는 네이티브 창 테두리를 제외한 콘텐츠 영역이다.
- 주요 컨트롤의 빈 영역·화면 밖 배치·겹침, 선택한 라벨의 세로 잘림, 투명/단색 출력 여부를 검사한다. 스크롤 영역 전체의 모든 텍스트가 동시에 보이도록 요구하지는 않는다. 캡처 시 텍스트 영역 스크롤을 처음으로 고정한다.

HTTP는 합성 HttpMessageHandler만 사용하며 네트워크 전송이 없다. 합성 계정 저장소·프로젝트 파일은 고유 임시 디렉터리에 만들고 종료 시 제거한다. 실제 계정 저장소와 `.env`를 읽지 않는다. 폴더 선택은 같은 내부 처리 함수를 호출해 네이티브 대화상자를 열지 않는다.

## 해석과 한계

2026-10-28: **16개 검사·54개 렌더링 통과**, 동일 환경의 두 실행 이미지 비교도 통과했다. 최소 콘텐츠 크기의 대기/승인, 오류·144 DPI 등 일부 PNG를 직접 열어 확인했다. 전체 이미지에 대한 사람의 시각적 승인을 뜻하지 않는다.

실제 창의 테두리·작업 표시줄·폴더 선택/로그인 대화상자, 포커스·키보드 탐색·IME, 접근성, 모니터 간 이동과 하드웨어 합성, 실계정·실제 장애는 이 검사에 포함되지 않는다. DPI는 부모 없는 WPF 루트의 DPI 설정과 레이아웃/픽셀 렌더링 조합이며 실제 모니터 전환 검증은 아니다. [Microsoft의 Visual 렌더링 예제](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/graphics-multimedia/how-to-encode-a-visual-to-an-image-file)와 [VisualTreeHelper API](https://learn.microsoft.com/en-us/dotnet/api/system.windows.media.visualtreehelper)를 사용한다.
