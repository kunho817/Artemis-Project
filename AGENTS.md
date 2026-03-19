# AGENTS.md — Artemis Project

> **이 파일은 모든 세션의 시작과 끝에서 반드시 참조·갱신되어야 합니다.**

---

## 프로젝트 개요

- **이름**: Artemis Project
- **설명**: Claude Code를 베이스로 하는 TUI 기반 Agent 코딩 시스템
- **언어**: Go
- **개발 방식**: 적층식(incremental) — 구조가 유동적이며, 기능 추가·수정·삭제가 빈번하게 발생
- **TUI 프레임워크**: charmbracelet/bubbletea + lipgloss + bubbles
- **개발 환경**: Windows (mkdir 등 셸 명령 시 경로 구분자 주의 — `\` 대신 `/` 사용 권장)

---

## LLM 프로바이더

| Provider | 용도 | 역할 매핑 | 엔드포인트 비고 |
|----------|------|-----------|-----------------|
| Claude | Coder, Engineer | claude-sonnet-4-6 | — |
| Gemini | Planner, Analyzer, Designer | gemini-3.1-pro-preview | TTFT 44.5s — 비동기 전용 |
| GPT | Orchestrator, Architect, Explorer | gpt-5.4 | — |
| GLM | Searcher, QA, Tester | glm-5 | Coding Plan 전용 엔드포인트 |
| VLLM | Code Generation (Local) | generate_code tool | http://localhost:8000, Qwen2.5-Coder-7B, opt-in |

---

## 아키텍처

### 디렉토리 구조

```
D:\Artemis_Project\
├── cmd/artemis/main.go            # 엔트리포인트
├── docs/
│   ├── getting-started.md         # 시작 가이드 (Korean)
│   ├── architecture.md            # 아키텍처 개요 (Korean)
│   └── configuration.md           # 설정 레퍼런스 (Korean)
├── internal/
│   ├── agent/
│   │   ├── agent.go               # Agent 인터페이스 + BaseAgent (LLM 호출, 이벤트, 프롬프트, SetTask/SetCritical/SetMemory/SetRepoMap/SetCategory/SetSkills/OverrideTask/SetContextBudget)
│   │   ├── category.go            # TaskCategory 타입 + 8개 카테고리 + CategoryConfig + 프로바이더/모델 매핑 + 카테고리 프롬프트
│   │   ├── skill.go               # Skill 구조체 + SkillRegistry + go:embed + 빌트인 스킬 로딩 + LoadCustomSkills(YAML frontmatter) + 글로벌/로컬 우선순위
│   │   ├── verify.go              # VerifyFunc 타입 + 빌트인 검증기 (Build/Test/Command/Chain) + ResolveVerifyFunc
│   │   ├── history.go              # HistoryWindow — 토큰 기반 대화 히스토리 관리 (SummarizeOldest/DropOldest/KeepAll)
│   │   ├── skills/                # 빌트인 스킬 마크다운 파일 (git-master/code-review/testing/documentation)
│   │   └── roles/
│   │       ├── prompts.go          # 13개 역할별 시스템 프롬프트 + OrchestratorPrompt (CATEGORIES/SKILLS/SPECIAL TOOLS/LSP TOOLS 섹션 포함)
│   │       └── roles.go            # RoleAgent 팩토리 + Orchestrator 태스크 오버라이드
│   ├── bus/
│   │   └── events.go              # AgentEvent 타입 + EventBus (buffered channel) + Background 이벤트 3종 + EventAgentWarn/EventRecoveryAttempt/EventReviewLoop
│   ├── config/
│   │   └── config.go              # 설정 구조체, Load/Save, AgentConfig, MemoryConfig, VectorConfig, RepoMapConfig, LSPConfig, SkillsConfig, GitHubConfig, VLLMConfig
│   ├── llm/
│   │   ├── provider.go            # Provider 인터페이스 + 팩토리 + StreamChunk (Reasoning/Usage 필드) + ModelInfo/MaxTokens
│   │   ├── claude.go              # Claude (Anthropic) 클라이언트 + 토큰 사용량 파싱
│   │   ├── gemini.go              # Gemini (Google) 클라이언트 + usageMetadata 파싱
│   │   ├── gpt.go                 # GPT (OpenAI) 클라이언트 + stream usage 파싱
│   │   ├── glm.go                 # GLM (ZhipuAI) 클라이언트 + Coding Plan + reasoning_content + usage
│   │   ├── vllm.go                # VLLM (vLLM) 클라이언트 — OpenAI-compatible API, API key optional, local code gen
│   │   ├── usage.go               # TokenUsage, ModelPricing, DefaultPricing, CalculateCost
│   │   ├── errors.go              # LLM 에러 분류기 (12 카테고리, 사용자 친화 메시지)
│   │   ├── errors_test.go         # 에러 분류 테스트
│   │   ├── fallback.go            # FallbackProvider (primary 실패 시 체인 재시도)
│   │   └── retry.go               # RetryProvider (지수 백오프 재시도)
│   │   ├── budget.go              # ContextBudget — P0-P6 우선순위 기반 토큰 배분 (System/History/Memory/RepoMap/Skills/Category/Task)
│   │   ├── models.go              # ModelRegistry — 프로바이더/모델별 MaxTokens/OutputTokens 매핑
│   │   └── tokencount.go          # CountTokens — tiktoken 기반 정확한 토큰 카운트 (cl100k_base 기본)
│   ├── memory/
│   │   ├── memory.go              # MemoryStore 인터페이스 + VectorSearcher 인터페이스 + 타입 + TokenBudget + RoleTagMap
│   │   ├── sqlite.go              # SQLiteStore — FTS5 풀텍스트 + RRF 하이브리드 검색, 스키마 마이그레이션 (v6: step_checkpoints)
│   │   ├── consolidate.go         # Consolidator — LLM 기반 세션 요약·사실 추출 + 시맨틱 중복 체크
│   │   ├── vector.go              # VectorStore — chromem-go 래퍼, 3 컬렉션, QueryEmbedding (VectorSearcher 구현체)
│   │   ├── embedding.go           # Voyage AI EmbeddingFunc (document/query input_type 분리)
│   │   ├── archive.go             # COLD tier — JSONL 아카이브 (세션별 원본 메시지 보존)
│   │   ├── archive_test.go        # COLD tier 테스트
│   │   ├── ctags.go               # EnsureCTags — 4-tier ctags 바이너리 해석 (PATH→cache→download→fail)
│   │   ├── parser.go              # SymbolParser 인터페이스 + CtagsParser (JSON 출력 파싱)
│   │   └── repomap.go             # RepoMapStore — 파일 인덱싱, FTS5 심볼 검색, 트리 포맷 출력
│   ├── lsp/
│   │   ├── client.go              # LSP Client — JSON-RPC over stdin/stdout, initialize/shutdown lifecycle, LSP method wrappers
│   │   ├── manager.go             # LSPManager — 다국어 Control Plane, ServerConfig, lazy loading, 확장자→서버 라우팅
│   │   └── process.go             # setHiddenProcessAttrs — Windows 프로세스 속성 스텁
│   ├── orchestrator/
│   │   ├── pipeline.go            # Phase, Pipeline 구조체 + 5단계 파이프라인 (레거시)
│   │   ├── plan.go                # ExecutionPlan + OrchestratorResponse + BackgroundTaskDef + JSON 파서 + IsReview/ReviewTarget/MaxReviewIterations
│   │   ├── background.go          # BackgroundTaskManager — goroutine 기반 병렬 백그라운드 태스크 관리
│   │   ├── engine.go              # Engine (Run/RunPlan/RunPlanFromStep + 3-Stage Failure Recovery + Review Feedback Loop + Checkpoint + Replan) + AgentBuilder
│   │   └── recovery.go            # RecoveryAction/RecoveryContext/RecoveryPrompter 인터페이스 + MaxRecoveryAttempts
│   │   └── replan.go              # ReplanContext/ReplanTrigger/Replanner 인터페이스 — 조건부 재계획
│   ├── mcp/
│   │   ├── client.go              # MCP Client — JSON-RPC over stdio, initialize/shutdown, tool call/list
│   │   ├── manager.go             # MCP Manager — 다중 서버 관리, 도구 자동 발견, Connect/Shutdown
│   │   └── process.go             # setHiddenProcessAttrs — Windows 프로세스 속성 스텁
│   ├── state/
│   │   ├── state.go               # SessionState (Blackboard) — 스레드 안전 Artifact 시스템 + ArtifactReview
│   │   └── checkpoint.go          # StepCheckpoint/IncompleteRun/CheckpointStore 인터페이스 — 파이프라인 체크포인트+재개
│   ├── tools/                     # 에이전트 도구 시스템 (22개 도구: read/write/list/search/shell/grep/patch/git×3/generate_code/lsp×6/run_tests/find_deps×2/ast×2)
│   ├── github/
│   │   ├── client.go              # GitHub API 래퍼 (issues/labels/branch/PR)
│   │   ├── syncer.go              # 이슈 동기화 + triage 상태 리포트
│   │   ├── processor.go           # 이슈 triage (LLM 기반 + heuristic 폴백) + FixEngine 연동 auto-fix + scaffold PR 폴백
│   │   ├── fixengine.go           # FixEngine 인터페이스 + AgentFixEngine (Orchestrator 기반 자동 수정)
│   │   └── worktree.go            # WorktreeManager — git worktree 생성/정리, mutex 보호
│   └── tui/
│       ├── app.go                 # 메인 모델 (App + CostState/SessionState/HistoryState 서브구조체) + Update 디스패치 + View + layoutState + computeLayout + shutdown()
│       ├── keybindings.go         # handleKeyMsg + handleMouseMsg + handleInputUpdate — Update()에서 추출
│       ├── overlay.go             # PlaceOverlay 합성 + Overlay 인터페이스 + OverlayKind/OverlayResult + 스타일 헬퍼 + OverlayRecovery/OverlayResume
│       ├── overlay_actions.go     # handleOverlay* 메서드 9개 — handleOverlayResult()에서 추출
│       ├── cmdpalette.go          # Command Palette (Ctrl+K) — 퍼지 검색, 8개 커맨드
│       ├── agentselector.go       # Agent Selector (Ctrl+A) — 에이전트 토글/티어/역할 매핑
│       ├── filepicker.go          # File Picker (Ctrl+O) — 디렉토리 탐색 + 파일 선택
│       ├── commands.go            # 슬래시 커맨드 핸들러 (/sessions, /load, /issues, /fix, /help)
│       ├── pipeline.go            # Orchestrator 라우팅 + 동적/레거시 파이프라인 실행 + Checkpoint/Replanner 와이어링 + executeResume + pipelineWg
│       ├── streaming.go           # 단일 모드 LLM 스트리밍 핸들러 + ContextBudget 기반 히스토리 관리 + 스트림 채널 drain
│       ├── events.go              # 에이전트 이벤트 핸들러 + 파이프라인 완료 처리 + 에이전트별 스트리밍 추적 + Warn/Recovery/ReviewLoop 이벤트
│       ├── recoverybridge.go      # RecoveryBridge (RecoveryPrompter 구현) + RecoveryRequest + RecoveryRequestMsg + waitForRecoveryRequest
│       ├── recoveryoverlay.go     # RecoveryOverlay (Overlay 구현) + RecoveryDecisionMsg + R/S/A 핫키 + 진단 표시
│       ├── resumeoverlay.go       # ResumeOverlay (Overlay 구현) + ResumeDecisionMsg + R/D/Esc 핫키 — 미완료 파이프라인 재개/폐기 선택
│       ├── replanner.go           # OrchestratorReplanner (Replanner 구현) — LLM 기반 조건부 재계획
│       ├── memory_init.go         # 메모리 시스템 초기화/종료/메시지 저장 (CodeIndex 5분 timeout) + CheckpointStore 와이어링
│       ├── github_init.go         # GitHub 이슈 트래커 초기화 (FixEngine/TriageLLM 와이어링)
│       ├── chat.go                # 대화 패널 (glamour 마크다운 렌더링, 메시지별 스트리밍 캐시)
│       ├── activity.go            # Activity 패널 (컨텍스트 정보 + 경과 시간 + 파일 변경) — 테마 색상 통합
│       ├── configview.go          # Config 뷰 코어 (7-tab 구조, Update, View, renderTabs, renderAgentsContent)
│       ├── configview_providers.go # Config 뷰 프로바이더 탭 (5 providers: Claude/Gemini/GPT/GLM/VLLM)
│       ├── configview_system.go   # Config 뷰 System 탭 (9 하위 탭: Memory/Tools/Vector/RepoMap/GitHub/Appearance/LSP/Skills/MCP)
│       ├── statusbar.go           # 하단 상태바 (모델, 티어, 토큰, 비용, 경과시간) + 동적 KeyHints (Default/Pipeline/Overlay/Config)
│       ├── styles.go              # lipgloss 스타일 정의 (RefreshStyles → theme.S 위임) + Diff 스타일 8종
│       ├── diffoverlay.go         # Diff Viewer (Ctrl+D) — 테마 기반 색상
│       └── theme/
│           ├── theme.go           # Theme 구조체, BuildStyles(), Load(), AvailableThemes(), mergeDefaults() + Diff 6색상 + go:embed 프리셋
│           └── presets/
│               ├── default.json   # 기본 테마 (Tailwind 팔레트 + diff 색상)
│               ├── dracula.json   # Dracula 테마 + diff 색상
│               └── tokyonight.json # Tokyo Night 테마 + diff 색상
├── training/                      # 코드 생성 LLM 학습 파이프라인 (Python, 보류)
├── go.mod
├── go.sum
└── AGENTS.md                      # 이 파일
```

### 키바인딩

| 키 | 동작 |
|----|------|
| `Enter` | 메시지 전송 |
| `Shift+Enter` | 줄바꿈 (textarea) |
| `Ctrl+S` | Config 뷰 진입/저장 |
| `Ctrl+K` | Command Palette |
| `Ctrl+A` | Agent Selector |
| `Ctrl+O` | File Picker |
| `Ctrl+D` | Diff Viewer |
| `Ctrl+L` | 화면 클리어 |
| `Ctrl+C` | 종료 |

---

## 현재 상태

### 완료된 Phase 목록

| Phase | 내용 | 세션 |
|-------|------|------|
| **A: 기반 강화** | Tool 강화 (FileLock, atomic write, auto-commit, /undo), 도구 10개→22개 | #25 |
| **B: 코드 생성** | VLLM Provider, generate_code 도구, 학습 파이프라인 (Python 3,335줄) | #26-27 |
| **C: 지능화** | tiktoken, ContextBudget P0-P6, HistoryWindow, Checkpoint/Resume, Review Loop, Re-planning | #28-29 |
| **D: 심화** | LSP Control Plane (6 도구), 테스트 러너, 의존성 그래프, ast-grep | #30 |
| **E: 에이전트 역량** | 커스텀 스킬 (YAML frontmatter), 자율 루프 (verify-gated), MCP 통합 | #31 |
| **F: UI 고도화** | 진행률 바, 토큰 추적, 테스트 결과 패널, Diff Overlay | #35 |
| **Dogfooding** | AD 구현 (React+TS, build PASS), 8버그 발견·수정 | #35 |
| **CLI/Headless** | `artemis chat`, `--headless`, `--multi`, `--race` | #35 |
| **안정화** | HTTP 타임아웃, maxToolIter=20, per-step timeout, Force-kill, Config View 9서브탭 | #35 |
| **문서화** | README.md (EN) + 3개 가이드 (KR) | #32-34 |
| **성능 최적화** | ToolDescriptions 캐시, SQLite pragma, async fact usage | #35 |
| **시그니처 기능** | ARTEMIS.md 로딩, Hooks, 병렬 Worktree+Race, 시맨틱 Context Engine, Flow Awareness | #35 |
| **TUI 대개편** | 5-Phase TUI Overhaul: 고루틴 안정화 5건, 테마 통합 13색상, 컴포넌트 분해 (1134→465줄), 레이아웃 통합, App 분해 | #36 |

### 미구현 / 보류

- [ ] 학습 파이프라인 실행 (코드 완성, 실행 보류 — 클라우드 GPU 필요)
- [ ] Python/TypeScript LSP 실제 활성화 (인프라 구축 완료, config에서 enabled=false)
- [ ] 콜그래프 분석 (golang.org/x/tools — 무거운 의존성, find_dependents로 대체)

---

## 핵심 설계 결정 (요약)

| 영역 | 결정 | 사유 |
|------|------|------|
| **아키텍처** | Orchestrator-First 동적 파이프라인 | Orchestrator가 의도 분류 → 필요한 에이전트만 선택적 호출 |
| **에이전트** | 13역할 + 카테고리 8종 + 스킬 시스템 | 역할별 프롬프트 + 카테고리별 모델 매핑 + 스킬 주입 |
| **프로바이더** | Premium/Budget 2-tier + FallbackProvider | 비용 효율 + 자동 장애 전환 |
| **메모리** | 3-Tier (HOT/WARM/COLD) + RRF 하이브리드 | SQLite FTS5 + Vector + JSONL 아카이브 |
| **LSP** | 자체 JSON-RPC 클라이언트 | 외부 의존성 zero, 다국어 인프라 |
| **MCP** | stdio 전송, 다중 서버 | 도구 무한 확장 가능 |
| **도구** | 22개 + Hooks(Pre/Post) + FlowTracker | 안전 훅 기본 등록, 작업 흐름 추적 |
| **TUI** | Hybrid Single/Split + 6 Overlay + 동적 KeyHints | 파이프라인 시 자동 Split, 완료 시 Single 복귀, 상태별 키 힌트 |
| **TUI 구조** | App 3-sub-struct + 컴포넌트 분해 6파일 | CostState/SessionState/HistoryState + configview 3분할 + overlay_actions/keybindings 추출 |
| **스트리밍** | 단일/다중 이중 경로 (의도적 분리) | 단일=채널 직접, 다중=EventBus — 통합 비용 불필요 |
| **CLI** | chat/headless/multi/race | TUI 없이 프로그래밍적 사용 가능 |
| **컨텍스트** | ContextBudget P0-P6 + CodeIndex + FlowTracker | 토큰 오버플로 방지 + 시맨틱 코드 자동 주입 + 작업 흐름 추적 |
| **실패 복구** | 3-Stage (Retry → Consultant → User) + Replan | 자동 복구 시도 → 사용자 개입 최소화 |
| **프로세스** | Force-kill + exec.CommandContext | 고아 프로세스 방지 |

---

## 세션 히스토리

| 세션 | 날짜 | 작업 내용 |
|------|------|-----------|
| #1-#4 | 2026-03-08 | 프로젝트 초기화, TUI, Config, LLM 4종, Agent 시스템, Pipeline |
| #5-#7 | 2026-03-09 | Orchestrator-First, Tool 5개, SSE 스트리밍, 대화 히스토리 |
| #8-#9 | 2026-03-09 | 영속 메모리 Phase 1 (SQLite FTS5), 세션 관리, 슬래시 커맨드 |
| #10-#11 | 2026-03-10 | Vector 검색 (Voyage AI), Repo-map (ctags) |
| #12-#13 | 2026-03-10 | UI Overhaul (glamour, textarea, 테마, Overlay 시스템) |
| #14-#18 | 2026-03-10 | 비용 추적, 에러 분류, GitHub 이슈, FixEngine, System 6서브탭 |
| #19-#24 | 2026-03-11~13 | 6-Phase Overhaul (Intent Gate, Scout/Consultant, Background, Category/Skill, Session Hierarchy, 3-Stage Recovery) |
| #25-#27 | 2026-03-13 | Phase A (도구 강화) + Phase B (vLLM, generate_code, 학습 파이프라인) |
| #28-#29 | 2026-03-13 | Phase C (tiktoken, ContextBudget, HistoryWindow, Checkpoint, Review, Replan) |
| #30 | 2026-03-14 | Phase D (LSP, 테스트 러너, 의존성 그래프, ast-grep) + 통합 테스트 7/7 |
| #31 | 2026-03-15 | Phase E 전체 (커스텀 스킬, 자율 루프, MCP) + 심화 통합 테스트 11/11 |
| #32-#34 | 2026-03-17 | 문서화 (README.md + 3 가이드) |
| #35 | 2026-03-17~18 | Phase F (UI 고도화) + Dogfooding (AD 구현) + CLI/Headless + 안정화 + 성능 최적화 + 시그니처 기능 5개 (ARTEMIS.md, Hooks, Worktree Race, Context Engine, Flow Awareness) |
| #36 | 2026-03-19 | TUI 대개편 5-Phase: P1 고루틴 안정화 (5 leak fix), P2 스타일 통합 (13 hardcoded→theme), P3 컴포넌트 분해 (configview 1134→465, app 911→685, +4 new files), P4 레이아웃 현대화 (computeLayout, 동적 key hints, min 80×24), P5 App 분해 (3 sub-structs, shutdown 통합, 스트리밍 문서화) |

---

## ⚠ 세션 운영 규칙

> **모든 AI 에이전트는 새 세션을 시작할 때 반드시 이 파일을 먼저 읽고, 세션 종료 시 아래 항목을 갱신해야 합니다.**

### 세션 시작 시
1. `AGENTS.md`를 읽어 현재 프로젝트 상태를 파악한다
2. 디렉토리 구조 변경이 있었는지 확인한다
3. 미구현 항목과 최근 세션 히스토리를 확인한다

### 세션 종료 시 (또는 유의미한 작업 완료 시)
1. **AGENTS.md 갱신**: 아래 항목을 확인하고 변경된 부분을 갱신한다
   - **디렉토리 구조**: 파일/폴더 추가·삭제·이동이 있었으면 갱신
   - **설계 결정 로그**: 새로운 아키텍처/기술 결정이 있었으면 추가
   - **현재 상태**: 구현 완료 / 미구현 체크리스트 갱신
   - **세션 히스토리**: 현재 세션의 작업 내용 기록
2. **Git Commit + Push**: AGENTS.md 갱신 후, 모든 변경사항을 commit하고 origin/main에 push한다
   - 커밋 메시지 형식: `feat/fix/refactor/chore: 세션 #N — 작업 요약`

### Git 커밋 규칙
- **Remote**: `origin` → `https://github.com/kunho817/Artemis-Project.git`
- **Branch**: `main`
- **커밋 메시지 Prefix**: `feat:` | `fix:` | `refactor:` | `chore:` | `docs:`
