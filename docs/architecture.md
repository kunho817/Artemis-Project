# Artemis Project 아키텍처 개요

Artemis는 고성능 TUI(Terminal User Interface) 기반의 멀티 에이전트 코딩 시스템입니다. 이 문서는 Artemis의 전체적인 구조와 핵심 컴포넌트, 그리고 데이터 흐름을 설명합니다.

## 1. 시스템 개요

Artemis는 개발자의 생산성을 극대화하기 위해 설계된 지능형 에이전트 시스템입니다.

- **언어 및 프레임워크**: Go 언어로 작성되었으며, TUI 구현을 위해 `charmbracelet/bubbletea` 프레임워크를 사용합니다.
- **규모**: 14개의 내부 패키지와 22개 이상의 전문 도구, 그리고 13가지의 특화된 에이전트 역할을 포함하고 있습니다.
- **핵심 가치**: 복잡한 태스크를 스스로 분석하고, 최적의 실행 계획을 수립하며, 도구를 활용해 실제 코드를 수정하고 검증합니다.

## 2. 핵심 아키텍처

Artemis는 사용자의 의도를 먼저 분석한 뒤, 그 복잡도에 따라 실행 경로를 동적으로 결정합니다.

```
사용자 입력 → TUI (bubbletea)
  → Orchestrator (의도 분류: trivial/conversational/exploratory/complex)
    → trivial: 직접 응답 (Single Panel 모드)
    → complex: ExecutionPlan 생성 → Engine 실행 (Split Panel 모드)
      → Step 1: [Agent A] + [Agent B] (병렬 실행)
      → Step 2: [Agent C] (순차 실행)
      → Verification (검증) + Review Loop (피드백 루프)
```

## 3. 패키지 구조

Artemis는 관심사 분리를 위해 다음과 같은 패키지 구조를 가집니다.

- **agent/**: 에이전트 인터페이스와 13가지 역할, 스킬 시스템, 태스크 카테고리를 관리합니다.
- **llm/**: 5개 프로바이더(Claude, Gemini, GPT, GLM, VLLM)와의 통신 및 토큰 예산을 관리합니다.
- **orchestrator/**: 파이프라인 엔진, 동적 실행 계획(ExecutionPlan), 복구 로직을 담당합니다.
- **tools/**: 파일 I/O, Git 제어, LSP 연동, 테스트 실행 등 22개의 실제 작업 도구를 포함합니다.
- **memory/**: SQLite, FTS5, Vector DB를 활용한 3계층 메모리와 레포맵(Repo-map)을 관리합니다.
- **lsp/**: Language Server Protocol 클라이언트로 코드 분석 및 심볼 탐색을 지원합니다.
- **mcp/**: Model Context Protocol 클라이언트로 외부 도구 서버와 연동합니다.
- **tui/**: 터미널 UI 구성 요소, 오버레이 다이얼로그, 테마 시스템을 포함합니다.
- **state/**: 세션 상태(Blackboard)와 체크포인트를 관리하여 작업 재개를 지원합니다.
- **bus/**: 에이전트의 작업 상태를 TUI에 실시간으로 전달하는 이벤트 버스입니다.

## 4. 에이전트 시스템

Artemis는 각 분야에 특화된 13가지 에이전트 역할을 정의합니다.

- **주요 역할**: Orchestrator, Planner, Analyzer, Searcher, Explorer, Architect, Coder, Designer, Engineer, QA, Tester, Scout, Consultant
- **BaseAgent**: 모든 에이전트의 기반이 되며, LLM 호출, 도구 사용, 이벤트 발행 기능을 공통으로 제공합니다.
- **카테고리 시스템**: visual-engineering, ultrabrain, deep, artistry, quick 등 8가지 카테고리에 따라 에이전트의 행동 방식과 모델 선택이 최적화됩니다.

## 5. 실행 파이프라인

복잡한 요청은 체계적인 파이프라인을 통해 처리됩니다.

1. **Orchestration**: 사용자의 의도를 분석하고 JSON 형식의 실행 계획(ExecutionPlan)을 생성합니다.
2. **Engine Execution**: 실행 계획에 따라 단계를 순차적으로 진행하며, 각 단계 내의 태스크는 병렬로 처리합니다.
3. **Failure Recovery**: 작업 실패 시 Consultant 에이전트의 진단을 거치거나 사용자에게 결정을 요청하는 3단계 복구 프로세스를 가동합니다.
4. **Review & Re-planning**: 작업 결과를 검토하고, 필요시 자동으로 계획을 수정하거나 추가 작업을 수행합니다.
5. **Checkpoint**: 각 단계의 상태를 저장하여 예기치 못한 중단 시에도 마지막 지점부터 재개할 수 있습니다.

## 6. 메모리 시스템

에이전트가 프로젝트의 맥락을 정확히 파악할 수 있도록 3계층 메모리 구조를 사용합니다.

- **HOT (SessionState)**: 현재 세션의 실시간 아티팩트와 단기 기억을 관리합니다.
- **WARM (SQLite + FTS5)**: 과거 세션의 요약, 결정 사항, 사실 관계를 저장하고 검색합니다.
- **COLD (JSONL Archive)**: 모든 대화 원본을 아카이브하여 보존합니다.
- **Vector Search**: `chromem-go`와 Voyage AI를 사용하여 코드와 문서의 시맨틱 검색을 지원합니다.
- **Repo-map**: Universal Ctags를 사용하여 전체 코드베이스의 구조와 심볼 관계를 인덱싱합니다.

## 7. 데이터 흐름

Artemis 내부의 데이터는 다음과 같은 경로로 흐릅니다.

```
사용자 → TUI → Orchestrator → Engine → Agent → Tools → 파일 시스템
                                ↓
                            EventBus → TUI (실시간 상태 및 출력 표시)
                                ↓
                            Memory (영속적 사실 및 세션 저장)
```

이 구조를 통해 Artemis는 단순한 챗봇을 넘어, 실제 개발 환경과 상호작용하며 복잡한 문제를 해결하는 자율형 코딩 에이전트로 동작합니다.
