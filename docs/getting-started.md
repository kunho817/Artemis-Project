# Artemis 시작하기

Artemis는 TUI 기반의 강력한 AI 코딩 어시스턴트입니다. 이 가이드를 통해 Artemis를 설치하고 첫 번째 코딩 작업을 시작하는 방법을 알아보세요.

## 1. 설치

Artemis를 실행하려면 Go 1.21 이상의 환경이 필요합니다.

### 빌드 방법
터미널에서 프로젝트 루트 디렉토리로 이동한 뒤 아래 명령어를 실행하세요.

**Linux / macOS:**
```bash
go build -o artemis ./cmd/artemis/
```

**Windows:**
```bash
go build -o artemis.exe ./cmd/artemis/
```

빌드가 완료되면 생성된 바이너리 파일을 실행하거나, 편의를 위해 시스템 PATH에 추가하여 어디서든 실행할 수 있게 설정하세요.

## 2. 초기 설정

Artemis를 처음 실행하면 `~/.artemis/config.json` 파일이 자동으로 생성됩니다. 실제 사용을 위해서는 LLM 프로바이더의 API 키를 설정해야 합니다.

1. 프로그램을 실행한 후 `Ctrl+S`를 눌러 설정(Settings) 화면을 엽니다.
2. 사용할 프로바이더의 API 키를 입력합니다. 최소 1개 이상의 키가 필요합니다.

### 프로바이더별 설정 정보
* **Gemini:** [Google AI Studio](https://aistudio.google.com/)에서 키 발급. 모델은 `gemini-3.1-pro-preview`를 권장합니다.
* **Claude:** [Anthropic Console](https://console.anthropic.com/)에서 키 발급. 모델은 `claude-sonnet-4-6`을 사용합니다.
* **GPT:** [OpenAI Platform](https://platform.openai.com/)에서 키 발급. 모델은 `gpt-5.4`를 지원합니다.
* **GLM:** [ZhipuAI](https://open.bigmodel.cn/)에서 키 발급. 모델은 `glm-5`이며 주로 코딩 계획 수립에 쓰입니다.

## 3. 첫 사용

설정을 마쳤다면 이제 Artemis와 대화를 시작할 수 있습니다.

### 실행 및 전송
* 실행: `./artemis` (또는 `artemis.exe`)
* 메시지 입력 후 `Ctrl+Enter`를 누르면 AI에게 전송됩니다.

### 대화 예시
Artemis에게 다음과 같은 질문을 던져보세요.
* "이 프로젝트의 구조를 설명해줘"
* "main.go 파일을 읽어줘"
* "HelloWorld 함수를 만들어줘"

Artemis는 단일 에이전트 모드와 멀티 에이전트 모드를 지원합니다. 기본적으로는 단일 모드로 작동하며, 복잡한 작업은 멀티 에이전트가 효율적입니다.

## 4. 에이전트 모드 설정

`Ctrl+A`를 누르면 에이전트 선택기(Agent Selector)가 나타납니다. 여기서 작업의 성격에 맞는 티어를 선택할 수 있습니다.

* **Premium Tier:** 4개 프로바이더를 모두 활용하여 최상의 결과를 냅니다.
* **Budget Tier:** Gemini와 GLM을 조합하여 비용 효율적으로 작동합니다.

멀티 에이전트 모드를 활성화하면 오케스트레이터(Orchestrator)가 사용자의 의도를 분석하고, 분석가, 설계자, 개발자 등 필요한 에이전트들에게 태스크를 자동으로 배분합니다.

## 5. 유용한 키바인딩

| 키 | 동작 |
|---|---|
| `Ctrl+Enter` | 메시지 전송 |
| `Ctrl+S` | 설정 화면 열기 / 저장 |
| `Ctrl+A` | 에이전트 선택기 열기 |
| `Ctrl+K` | 커맨드 팔레트 (빠른 명령 실행) |
| `Ctrl+O` | 파일 탐색기 열기 |
| `Ctrl+L` | 화면 지우기 및 세션 초기화 |
| `Tab` | 패널 간 포커스 전환 (멀티 에이전트 모드) |
| `Ctrl+C` | 프로그램 종료 |

## 6. 다음 단계

기본적인 사용법을 익혔다면 더 깊이 있는 기능을 살펴보세요.

* **커스텀 스킬:** 특정 작업 흐름을 자동화하고 싶다면 `skills` 문서를 참고하여 나만의 스킬을 정의해보세요.
* **LSP 설정:** 코드 분석 기능을 강화하려면 `configuration.md`의 LSP 섹션을 확인하세요.
* **GitHub 연동:** 이슈 트래커와 연동하여 버그를 자동으로 수정하고 싶다면 `configuration.md`의 GitHub 설정을 참고하세요.
