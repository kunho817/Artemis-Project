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
│       ├── app.go                 # 메인 모델 + Update 디스패치 + View + 레이아웃 (Hybrid Single/Split) + 오버레이 라우팅 + Recovery 큐 + Resume 큐
│       ├── overlay.go             # PlaceOverlay 합성 + Overlay 인터페이스 + OverlayKind/OverlayResult + 스타일 헬퍼 + OverlayRecovery/OverlayResume
│       ├── cmdpalette.go          # Command Palette (Ctrl+K) — 퍼지 검색, 8개 커맨드
│       ├── agentselector.go       # Agent Selector (Ctrl+A) — 에이전트 토글/티어/역할 매핑
│       ├── filepicker.go          # File Picker (Ctrl+O) — 디렉토리 탐색 + 파일 선택
│       ├── commands.go            # 슬래시 커맨드 핸들러 (/sessions, /load, /issues, /fix, /help)
│       ├── pipeline.go            # Orchestrator 라우팅 + 동적/레거시 파이프라인 실행 + Checkpoint/Replanner 와이어링 + executeResume
│       ├── streaming.go           # 단일 모드 LLM 스트리밍 핸들러 + ContextBudget 기반 히스토리 관리
│       ├── events.go              # 에이전트 이벤트 핸들러 + 파이프라인 완료 처리 + 에이전트별 스트리밍 추적 + Warn/Recovery/ReviewLoop 이벤트
│       ├── recoverybridge.go      # RecoveryBridge (RecoveryPrompter 구현) + RecoveryRequest + RecoveryRequestMsg + waitForRecoveryRequest
│       ├── recoveryoverlay.go     # RecoveryOverlay (Overlay 구현) + RecoveryDecisionMsg + R/S/A 핫키 + 진단 표시
│       ├── resumeoverlay.go       # ResumeOverlay (Overlay 구현) + ResumeDecisionMsg + R/D/Esc 핫키 — 미완료 파이프라인 재개/폐기 선택
│       ├── replanner.go           # OrchestratorReplanner (Replanner 구현) — LLM 기반 조건부 재계획
│       ├── memory_init.go         # 메모리 시스템 초기화/종료/메시지 저장 (GitHub init 분리 후 경량화) + CheckpointStore 와이어링
│       ├── github_init.go         # GitHub 이슈 트래커 초기화 (FixEngine/TriageLLM 와이어링)
│       ├── chat.go                # 대화 패널 (glamour 마크다운 렌더링, 메시지별 스트리밍 캐시)
│       ├── activity.go            # Activity 패널 (컨텍스트 정보 + 경과 시간 + 파일 변경)
│       ├── configview.go          # Config 뷰 (7-tab: Claude|Gemini|GPT|GLM|VLLM|Agents|System) + System 6개 하위 탭(Memory/Tools/Vector/RepoMap/GitHub/Appearance)
│       ├── statusbar.go           # 하단 상태바 (모델, 티어, 토큰, 비용, 세션 경과시간)
│       ├── styles.go              # lipgloss 스타일 정의 (RefreshStyles → theme.S 위임)
│       └── theme/
│           ├── theme.go           # Theme 구조체, BuildStyles(), Load(), AvailableThemes(), go:embed 프리셋
│           └── presets/
│               ├── default.json   # 기본 테마
│               ├── dracula.json   # Dracula 테마
│               └── tokyonight.json # Tokyo Night 테마
├── training/                      # 코드 생성 LLM 학습 파이프라인
│   ├── pyproject.toml             # Python 패키지 설정
│   ├── requirements.txt           # Python 의존성
│   ├── .gitignore                 # 학습 데이터/모델/체크포인트 제외
│   ├── configs/
│   │   ├── data_config.yaml       # 데이터 수집/전처리 설정 (BigQuery/Stack v2/Quality/AST/Dedup/Format)
│   │   └── training_config.yaml   # 학습/배포 설정 (QLoRA/FIM/Evaluation/AWQ/vLLM)
│   ├── scripts/
│   │   ├── 01_collect.py          # 데이터 수집 CLI (BigQuery + Stack v2)
│   │   ├── 02_preprocess.py       # 전처리 CLI (AST 추출 → 중복 제거 → 포맷 변환)
│   │   ├── 03_train.py            # QLoRA 학습 CLI
│   │   ├── 04_evaluate.py         # 평가 CLI (exact_match/CodeBLEU/syntax/FIM)
│   │   └── 05_deploy.py           # 배포 CLI (merge → quantize → serve)
│   ├── src/
│   │   ├── __init__.py
│   │   ├── collect/
│   │   │   ├── __init__.py
│   │   │   ├── bigquery.py        # BigQueryCollector — GH repos 쿼리, 품질 필터, 별점 수집
│   │   │   └── stack_v2.py        # StackV2Collector — HuggingFace 스트리밍 수집
│   │   ├── preprocess/
│   │   │   ├── __init__.py
│   │   │   ├── go_ast.py          # GoASTExtractor — tree-sitter 기반 Go AST 심볼 추출
│   │   │   ├── dedup.py           # MinHashDedup — datasketch LSH 기반 중복 제거
│   │   │   └── format.py          # TrainingFormatter — 6가지 시나리오 ChatML/FIM 포맷 변환
│   │   ├── train/
│   │   │   ├── __init__.py
│   │   │   ├── qlora.py           # QLoRATrainer — BitsAndBytes 4-bit NF4 + LoRA r=64 + SFTTrainer
│   │   │   └── fim.py             # FIMDataCollator — 40% FIM/50% SPM 혼합 배칭, <|fim_pad|> 패딩
│   │   ├── evaluate/
│   │   │   ├── __init__.py
│   │   │   └── metrics.py         # EvaluationSuite — exact_match/pass@k/CodeBLEU/syntax/FIM 메트릭
│   │   └── deploy/
│   │       ├── __init__.py
│   │       ├── merge.py           # merge_adapters — QLoRA 어댑터 병합 (merge_and_unload)
│   │       ├── quantize.py        # quantize_awq — AutoAWQ 4-bit 양자화 (~3.5GB)
│   │       └── serve.py           # vLLM 서빙 설정 생성 + 런처 + 엔드포인트 테스트
│   └── data/
│       └── .gitkeep
├── go.mod
├── go.sum
└── AGENTS.md                      # 이 파일
```

### TUI 레이아웃 (C형 — Hybrid Single/Split)

**기본 모드 (Single Panel — 단일 프로바이더 모드):**
```
┌─────────────────────────────────────────────────────────┐
│ Artemis v0.1.0                                   Model  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  Chat 영역 (풀 너비, glamour 마크다운 렌더링)             │
│                                                         │
├─────────────────────────────────────────────────────────┤
│ │ Type a message... (Ctrl+Enter to send)                │
├─────────────────────────────────────────────────────────┤
│ Model · Single · Tk:0 · 12m │ [^↵]Send [^K]Palette [^A]Agents [^O]Files [^S]Settings [^L]Clear [^C]Quit │
└─────────────────────────────────────────────────────────┘
```

**파이프라인 모드 (Split Panel — 멀티 에이전트 실행 중 자동 전환):**
```
┌──────────────────────────────┬──────────────────────────┐
│ Artemis v0.1.0        Model  │  Context                 │
├──────────────────────────────┤  Session: abc12345       │
│                              │  Model: Claude           │
│  Chat 영역                    │──────────────────────────│
│  (glamour 마크다운)            │  Activity                │
│                              │  ● Coder: streaming...   │
│                              │  ✓ Analyzer: done (2.1s) │
│                              │──────────────────────────│
│                              │  Files Changed           │
├──────────────────────────────┤  ~ internal/tui/chat.go  │
│ │ textarea (1-8줄 자동 확장)  │                          │
├──────────────────────────────┴──────────────────────────┤
│ Model · Premium · Tk:0 · 1h23m │ [^↵]Send [^K]Palette [^A]Agents [^O]Files [^S]Settings [^L]Clear [^C]Quit │
└─────────────────────────────────────────────────────────┘
```

### 키바인딩

| 키 | 동작 |
|----|------|
| `Ctrl+Enter` | 메시지 전송 |
| `Enter` | 줄바꿈 (textarea) |
| `Tab` | 패널 포커스 전환 (Split 모드에서만 Chat ↔ Activity) |
| `Ctrl+S` | Config 뷰 진입/저장 |
| `Ctrl+L` | 화면 클리어 + Single 모드 복귀 |
| `Ctrl+C` | 종료 |
| `Ctrl+←/→` | Config 뷰 탭 전환 (Claude/Gemini/GPT/GLM/Agents/System) |
| `↑/↓` | Chat 스크롤 (화살표 키) |
| `PgUp/PgDn` | Chat 페이지 스크롤 |
| `마우스 휠` | Chat 스크롤 |
| `/sessions` | 이전 세션 목록 표시 |
| `/load <id>` | 이전 세션 로드 (대화 복원) |
| `/help` | 사용 가능한 명령어 표시 |
| `/undo` | 마지막 자동 커밋 되돌리기 |
| `Ctrl+K` | Command Palette (커맨드 팔레트 — 퍼지 검색) |
| `Ctrl+A` | Agent Selector (에이전트 토글/티어 전환) |
| `Ctrl+O` | File Picker (파일 탐색/선택) |
| `←/→` | Config System 탭 하위 탭 전환 (토글/사이클 필드에서) |

---

## 설계 결정 로그

| 날짜 | 결정 | 사유 |
|------|------|------|
| 2026-03-08 | TUI 레이아웃 B형(Two-Panel Split) 채택 | 5개 시안 중 사용자 선택 |
| 2026-03-08 | bubbletea + lipgloss + bubbles 채택 | Go TUI 생태계 표준, Elm architecture |
| 2026-03-08 | Config 저장 경로 `~/.artemis/config.json` | 표준 사용자 설정 경로 |
| 2026-03-08 | LLM Provider 인터페이스 추상화 | 4개 프로바이더 통일 인터페이스 (Send/Stream) |
| 2026-03-08 | Config View — Ctrl+S 진입 | Chat 뷰에서 단축키로 전환 |
| 2026-03-08 | Blackboard + Phased Pipeline 아키텍처 | Oracle 에이전트 설계, 5단계 순차 + 단계 내 병렬 |
| 2026-03-08 | Agent ≠ Provider 분리 | Provider = raw I/O, Agent = role+goal+state-aware worker |
| 2026-03-08 | AgentConfig — 역할↔프로바이더 매핑 | Config에서 런타임 변경 가능, DefaultAgentConfig로 기본값 제공 |
| 2026-03-08 | errgroup 기반 병렬 에이전트 실행 | 페이즈 내 에이전트 동시 실행, critical 실패 시 파이프라인 중단 |
| 2026-03-08 | 단일/멀티 에이전트 듀얼 모드 | cfg.Agents.Enabled로 전환, 하위 호환성 유지 |
| 2026-03-08 | FallbackProvider + Fallback 체인 | Gemini(종량제)/GLM(구독제) → primary 실패 시 자동 전환, 비용 효율 |
| 2026-03-08 | Premium/Budget 2-tier 시스템 | 기존 단일 매핑 → 2계층(Premium=4사/Budget=Gemini+GLM), tier별 역할 최적화, premium 실패 시 budget으로 fallback |
| 2026-03-08 | Config View 5-tab 분리 (Claude\|Gemini\|GPT\|GLM\|Agents) | Agent Pipeline 설정이 화면을 차지하여 프로바이더별 탭 + Agents 전용 탭으로 분리, Ctrl+←→ 전환 |
| 2026-03-09 | Orchestrator-First 라우팅 모델 | 고정 5단계 파이프라인 → Orchestrator가 먼저 의도 분석 후 동적 플램 생성, 필요한 에이전트만 선택적 호출 |
| 2026-03-09 | Orchestrator = 순수 조율자 | Orchestrator는 절대 직접 작업 수행 안 함, JSON ExecutionPlan만 생성 |
| 2026-03-09 | ExecutionPlan 동적 플램 시스템 | Step 단위 순차 실행, Step 내 Task 병렬 실행, 컨텍스트 분산 가능 |
| 2026-03-09 | Agent 태스크 오버라이드 (SetTask/SetCritical) | Orchestrator가 역할별 기본 태스크 대신 특정 태스크 지정 가능 |
| 2026-03-09 | 레거시 파이프라인 폴백 | Orchestrator LLM 호출/플램 파싱 실패 시 고정 5단계 파이프라인으로 자동 폴백 |
| 2026-03-09 | Agent 출력 이벤트 (EventAgentOutput) | 에이전트 LLM 응답을 EventBus → Chat 패널에 실시간 표시 |
| 2026-03-09 | 대화 히스토리 컨텍스트 유지 | 멀티턴 대화 지원 — user/assistant 메시지 누적, Ctrl+L로 초기화 |
| 2026-03-09 | 에이전트 Tool 시스템 (5개 툴) | read_file/write_file/list_dir/shell_exec/search_files, <tool_use> XML 형식, 최대 10회 반복 |
| 2026-03-09 | SSE 스트리밍 (Claude/GPT) | 실시간 청크 표시, Gemini/GLM은 Send 래핑 유지 |
| 2026-03-09 | Claude System 필드 분리 | Anthropic API system 파라미터 사용, 연속 user 메시지 방지 |
| 2026-03-09 | Gemini SystemInstruction 필드 분리 | systemInstruction 통합 필드 사용, 연속 user 메시지 방지 |
| 2026-03-09 | RetryProvider + 지수 백오프 | 1s→2s→4s 지수 백오프, 최대 2회 재시도, 비재시도 에러(401/403/400) 즉시 반환 |
| 2026-03-09 | 컨텍스트 타임아웃 정책 | 스트리밍=120s, Orchestrator=90s, 파이프라인=5min |
| 2026-03-09 | 파이프라인 결과 구조화 표시 | RoleAgent 메시지 타입 + 역할별 색상 스타일링, 구분선 + 헤더 |
| 2026-03-09 | Gemini SSE 스트리밍 | streamGenerateContent?alt=sse 엔드포인트, connection close 감지 (data: [DONE] 없음) |
| 2026-03-09 | GLM SSE 스트리밍 | OpenAI-compatible stream:true, data: [DONE] 종료, reasoning_content 필드 인지 |
| 2026-03-09 | 에이전트 대화 컨텍스트 공유 | SessionState.SetHistory() → BuildPromptWithContext에서 대화 히스토리 주입, 파이프라인 간 멀티턴 인지 |
| 2026-03-09 | Send() HTTP 상태 코드 체크 추가 | 4개 프로바이더 Send()에 resp.StatusCode != 200 조기 반환, Stream()은 이미 있었음 |
| 2026-03-09 | strings.Title() → cases.Title 교체 | Go 1.18+ 비권장 API 제거, golang.org/x/text/cases 사용 |
| 2026-03-09 | GLM reasoning_content 캡처 | StreamChunk.Reasoning 필드 추가, GLM Stream()/Send()에서 reasoning_content 파싱 |
| 2026-03-09 | SQLite-Centered 3-Tier 메모리 아키텍처 | HOT(Blackboard)/WARM(SQLite+FTS5)/COLD(JSONL 아카이브), modernc.org/sqlite 순수 Go |
| 2026-03-09 | MemoryStore 인터페이스 추상화 | 백엔드 교체 가능 설계, Phase 2 벡터검색/Phase 3 repo-map 확장 예약 필드 |
| 2026-03-09 | FTS5 풀텍스트 검색 | semantic_facts/session_summaries/decisions에 FTS5 인덱스, OR 기반 prefix 매칭 |
| 2026-03-09 | 스키마 마이그레이션 시스템 | schema_version 테이블로 버전 추적, 적층식 업그레이드 지원 |
| 2026-03-09 | LLM 기반 세션 Consolidation | 세션 종료 시 LLM으로 대화 요약 + 사실 추출 + 결정 기록, 중복 체크 포함 |
| 2026-03-09 | 역할별 메모리 태그 필터링 | RoleTagMap으로 11개 역할별 관심 태그 정의, 에이전트가 관련 사실만 조회 |
| 2026-03-09 | 토큰 버짓 시스템 | TokenBudget 구조체로 프롬프트 섹션별 토큰 상한 정의, 컨텍스트 오버플로 방지 |
| 2026-03-09 | Fact Decay (사실 감쇠) | 설정된 기간/사용 횟수 기준으로 오래된 저사용 사실 자동 정리 |
| 2026-03-09 | Agent.SetMemory() 인터페이스 확장 | Agent 인터페이스에 SetMemory 추가, BuildPromptWithContext에서 영속 메모리 자동 주입 |
| 2026-03-09 | 세션 메시지 영속 저장 (스키마 v2) | session_messages 테이블 + migrateV2, 모든 user/assistant 메시지 자동 저장 |
| 2026-03-09 | 슬래시 커맨드 시스템 | /sessions, /load, /help — handleCommand 라우팅, LLM 호출 전 인터셉트 |
| 2026-03-09 | 세션 이어하기 (Session Resume) | /load로 이전 세션 메시지 복원 → history + chat 패널 재생, sessionID 전환 |
| 2026-03-09 | ListSessions UNION 쿼리 | session_summaries + session_messages UNION으로 통합/비통합 세션 모두 표시 |
| 2026-03-09 | Tool 반복 횟수 설정 가능 | 기본 무제한 + config.json max_tool_iterations로 제한 설정 |
| 2026-03-09 | Config View 6-tab 확장 (System 탭 추가) | Memory(활성/통합/Fact Age/Use Count) + Tool(Max Iterations) 설정 UI, 프로바이더 탭은 기존 유지 |
| 2026-03-10 | 멀티 에이전트 스트리밍 (Option A) | Agent CallLLMWithTools에서 Stream() → 청크 EventBus 전송 → TUI 실시간 표시, tool_use는 전체 텍스트에서 후처리 파싱 |
| 2026-03-10 | streamAndAccumulate 패턴 | Stream() 실패 시 Send() 폴백, 스트리밍 시 EmitOutput 중복 방지 (streamed 플래그) |
| 2026-03-10 | 에이전트 스트리밍 이벤트 3종 | EventAgentStreamStart/Chunk/Done — TUI에서 빈 메시지 생성 → AppendToLast → pipelineOutputs 저장 |
| 2026-03-10 | chromem-go 벡터 저장소 채택 | 순수 Go, 임베딩 외 외부 의존성 無, sqlite-vec 전환 예약 |
| 2026-03-10 | Voyage AI 임베딩 모델 채택 | voyage-code-3 (1024d), 무료 200M 토큰, 코드 검색 OpenAI 대비 +13.8% |
| 2026-03-10 | Dual EmbeddingFunc (document/query) | Voyage input_type 요구사항 대응, chromem-go QueryEmbedding() 활용 |
| 2026-03-10 | RRF 하이브리드 검색 (FTS5 + Vector) | 벡터 0.7 + FTS5 0.3 가중치, k=60, 투명 폴백 (벡터 미사용 시 FTS5 전용) |
| 2026-03-10 | 비동기 자동 임베딩 (Auto-embed) | SaveFact/SaveSession/SaveDecision 시 goroutine으로 비동기 임베딩, 저장 미차단 |
| 2026-03-10 | 시맨틱 중복 체크 (Consolidation) | 코사인 유사도 > 0.85 → 중복 판정, VectorStore 미사용 시 단어 겹침 > 70% 폴백 |
| 2026-03-10 | VectorConfig (config.json) | enabled/provider/api_key/model/store_path, 기본 voyage-code-3, ~/.artemis/vectors/ |
| 2026-03-10 | Config View System 탭 8필드 확장 | 기존 5필드 + Vector Search 토글/Voyage API Key(마스킹)/Model명 추가 |
| 2026-03-10 | Universal Ctags 파서 백엔드 채택 | 100+ 언어 지원, JSON 출력, GPL-2.0 별도 프로세스 = mere aggregation |
| 2026-03-10 | 4-tier ctags 프로비저닝 | 커스텀경로→PATH→캐시→자동다운로드(Windows), Linux/macOS 설치 안내 |
| 2026-03-10 | RepoMapConfig (config.json) | enabled/max_tokens/update_on_write/exclude_patterns/ctags_path |
| 2026-03-10 | 스키마 v3 (repo_symbols) | repo_symbols 테이블 + repo_symbols_fts FTS5 + 3개 트리거, migrateV3 |
| 2026-03-10 | CtagsParser JSON 파싱 | --output-format=json --fields=+neKS, SymbolParser 인터페이스 추상화 |
| 2026-03-10 | SHA256 증분 인덱싱 | 파일 해시 비교로 변경되지 않은 파일 스킵, 불필요한 재파싱 방지 |
| 2026-03-10 | 역할별 심볼 필터링 (RepoMapRoleFilter) | Coder=전체, Architect=exported만, Tester=함수만 등 역할 최적화 |
| 2026-03-10 | 토큰 버짓 포맷팅 (FormatRepoMap) | 파일별 트리 출력, maxTokens(2048) 초과 시 중단, 프롬프트 무한확장 방지 |
| 2026-03-10 | Agent.SetRepoMap() 인터페이스 확장 | BuildPromptWithContext에서 "## Codebase Structure" 섹션으로 역할별 필터링된 심볼 주입 |
| 2026-03-10 | Config View System 탭 10필드 확장 | 기존 8필드 + RepoMap Enabled 토글 + Max Tokens 입력 추가 |
| 2026-03-10 | systemFieldToInputIdx 매핑 | System 탭 토글/입력 필드 불연속 배치 대응, 필드Idx→inputIdx 명시적 매핑 |
| 2026-03-10 | UI Overhaul: Hybrid Layout (C형) 채택 | 기본 Single Panel, 파이프라인 실행 시 자동 Split, 완료 시 Single 복귀 |
| 2026-03-10 | UI Overhaul: textarea 입력 시스템 | textinput → textarea 교체, Ctrl+Enter 전송, Enter 줄바꿈, 1-8줄 자동 확장 |
| 2026-03-10 | UI Overhaul: JSON 테마 시스템 | Theme 구조체 + go:embed 프리셋(default/dracula/tokyonight), RefreshStyles() 위임 패턴 |
| 2026-03-10 | UI Overhaul: glamour 마크다운 렌더링 | Assistant/Agent 메시지 glamour 렌더링, 스트리밍 중 plain text → 완료 시 glamour 캐시 |
| 2026-03-10 | UI Overhaul: 스트리밍 마크다운 캐시 | renderedCache[]로 완료된 메시지 캐시, 스트리밍 중 재렌더링 방지 |
| 2026-03-10 | UI Overhaul: tool_use XML → 코드 블록 | <tool_use> 태그를 ```xml 코드 블록으로 전처리 후 glamour 렌더링 |
| 2026-03-10 | UI Overhaul: Activity 패널 확장 | Context 섹션(Session/Model/Agents) + 경과시간 표시 + 3-section 레이아웃 |
| 2026-03-10 | UI Overhaul: StatusBar 리디자인 | 세션 경과시간 + 비용 표시 + middle-dot 구분자 |
| 2026-03-10 | UI Overhaul: app.go 분해 (God Object 제거) | 1410줄 → 496줄, 5개 파일 분리 (commands/pipeline/streaming/events/memory_init)
| 2026-03-10 | Config View 테마 선택기 (Appearance 섹션) | System 탭 fieldIdx 10에 테마 순환 토글 추가, AvailableThemes() 연동, 11필드 확장 |
| 2026-03-10 | 병렬 에이전트 스트리밍 레이스 컨디션 수정 | 단일 스칼라 → agentStreams map[string]*agentStreamInfo 에이전트별 추적, ChatPanel streamingMsgs map[int]bool 메시지별 상태 |
| 2026-03-10 | Overlay 다이얼로그 시스템 채택 | PlaceOverlay 문자 단위 합성(OpenCode MIT 적용), Overlay 인터페이스 + OverlayKind enum + OverlayResult 패턴 |
| 2026-03-10 | Command Palette (Ctrl+K) | 내부 퍼지 매칭, 8개 항목(Sessions/Load/Help/Clear/Settings/Toggle Agents/Switch Tier/Switch Theme) |
| 2026-03-10 | Agent Selector (Ctrl+A) | 에이전트 활성화/티어 토글 + 11개 역할→프로바이더 매핑 읽기 전용 표시 |
| 2026-03-10 | File Picker (Ctrl+O) | 디렉토리 탐색, .. 상위 이동, 디렉토리 우선 정렬, 스크롤 윈도우 |
| 2026-03-10 | Overlay 라우팅 순서 — Update() 인터셉트 | overlayKind != OverlayNone 시 모든 입력을 오버레이로 전달, 배경 인터랙션 차단 |
| 2026-03-10 | 토큰 사용량 추적 + 비용 계산 시스템 | TokenUsage/ModelPricing/CalculateCost + 4사 프로바이더 usage 파싱 + StatusBar 누적 비용 표시 |
| 2026-03-10 | LLM 에러 분류 시스템 (12 카테고리) | ClassifyError + 사용자 친화 메시지, 파이프라인/스트리밍 에러 서페이싱 |
| 2026-03-10 | COLD tier JSONL 아카이브 | 세션별 원본 메시지 ~/.artemis/archive/{sessionID}.jsonl, 셧다운 시 자동 아카이브 |
| 2026-03-10 | Tool 시스템 10개 확장 | 기존 5개 + grep(정규식)/patch_file(라인 편집)/git_status/git_diff/git_log |
| 2026-03-10 | 테마 확장 기능 | ExportTheme() + mergeDefaults() + partial themes 지원 + Command Palette "Export Theme" |
| 2026-03-10 | VectorSearcher 인터페이스 추출 | chromem-go → sqlite-vec 전환 준비, 3 소비자(SQLiteStore/Consolidator/TUI) 마이그레이션 |
| 2026-03-10 | System 탭 6개 하위 탭 시스템 | 뷰포트 스크롤 제거 → sysSubTab(Memory/Tools/Vector/RepoMap/GitHub/Appearance), ←/→ 하위 탭 전환(토글 필드+전용입력 탭), GitHub 하위 탭 8필드 |
| 2026-03-10 | Orchestrator-Driven FixEngine 아키텍처 | Orchestrator가 동적 플램으로 이슈 분석→코드 수정→검증, 모든 에이전트 활용 가능 |
| 2026-03-10 | Git Worktree 격리 실행 | fix별 임시 worktree 생성(git worktree add -b), main 작업트리 오염 방지, 완료 후 자동 정리 |
| 2026-03-10 | PipelineRunner 콜백 패턴 | github/ 패키지가 agent/orchestrator/tools 직접 import 회피, memory_init.go에서 클로저 주입 |
| 2026-03-10 | FixEngine → Scaffold PR 폴백 | FixEngine 실패 또는 변경사항 없을 시 기존 scaffold PR(GitHub API CreateBranch) 경로로 자동 전환 |
| 2026-03-10 | /fix 타임아웃 제거 | 코드 생성 시간 불확정성 → context.Background() 사용, 타임아웃으로 인한 불완전 결과 방지 |
| 2026-03-10 | Per-Fix ToolExecutor | worktree 경로 기반 NewToolExecutor 생성, 도구 실행이 worktree 내부로 격리 |
| 2026-03-10 | WorktreeManager mutex 보호 | git worktree add/remove가 .git/worktrees 메타데이터 공유 → sync.Mutex로 직렬화, Windows 파일 잠금 재시도(3회) |
| 2026-03-11 | LLM 기반 이슈 triage | 키워드 heuristic → LLM 분류로 교체, TriageLLM 콜백 패턴(github/ → llm/ 의존 회피), JSON 응답 파싱, heuristic 폴백 보존 |
| 2026-03-11 | TriageLLM 콜백 패턴 | PipelineRunner와 동일한 콜백 주입 패턴, analyzer 역할 프로바이더 사용, github/ 패키지 llm/ import 불필요 |
| 2026-03-11 | Triage 타임아웃 정책 | Auto-triage 배경 타임아웃 10s→2min(LLM 호출 감안), 개별 이슈 triage 30s 타임아웃 |
| 2026-03-11 | memory_init.go → github_init.go 분리 | 323줄→179줄 경량화, GitHub 초기화 전용 파일 분리(181줄), 관심사 분리 |
| 2026-03-11 | buildProvider 스코프 리팩토링 | PipelineRunner 클로저 내부 → initGitHub() 스코프로 추출, triageLLM/FixEngine 양쪽에서 공유 |
| 2026-03-12 | Intent Gate 4단계 의도 분류 시스템 | Orchestrator가 trivial/conversational/exploratory/complex 4단계로 의도 분류, 단순 요청은 Orchestrator→ExecutionPlan 우회 |
| 2026-03-12 | OrchestratorResponse 통합 포맷 | 기존 ExecutionPlan → intent 포함 OrchestratorResponse 확장, 레거시 JSON 호환 유지 (ParseOrchestratorResponse + ParsePlan 폴백) |
| 2026-03-12 | Intent 기반 레이아웃 분기 | trivial/conversational → Single 레이아웃 유지, complex/exploratory → Split 전환, handleOrchestratedSubmit에서 레이아웃 결정 지연 |
| 2026-03-12 | executeTrivial 경량 스트리밍 경로 | trivial 의도 시 Engine/EventBus 우회, 역할별 프로바이더로 직접 스트리밍 (handleSingleSubmit 패턴 재사용) |
| 2026-03-12 | SystemPromptForRole 헬퍼 | roles.go에서 역할별 시스템 프롬프트 추출 함수 — executeTrivial에서 전체 RoleAgent 생성 없이 프롬프트 접근 |
| 2026-03-12 | Orchestrator/Agent Overhaul 6단계 계획 수립 | Phase 1: Intent Gate → Phase 2: Scout/Consultant → Phase 3: BackgroundTaskManager → Phase 4: Category+Skill → Phase 5: 세션 계층 → Phase 6: 실패 복구 |
| 2026-03-12 | Scout/Consultant 2개 상위 에이전트 추가 | Scout(경량 탐색, gemini-flash) + Consultant(고품질 자문, read-only), 기존 11역할 유지 + 2역할 추가 = 13역할 |
| 2026-03-12 | Per-Role Model Override 시스템 | AgentConfig.ModelOverrides map[string]string으로 역할별 모델 지정, NewProviderWithModel() 팩토리, config struct 복사 방식으로 기존 config 오염 방지 |
| 2026-03-12 | Scout 기본 모델 gemini-3-flash-preview | Premium tier Scout은 gemini provider + flash 모델 오버라이드, Budget tier는 gemini 기본 모델(Pro) |
| 2026-03-12 | Consultant 기본 매핑 gpt(Premium)/gemini(Budget) | Consultant는 고품질 추론 필요 → Premium=GPT-5.4, Budget=Gemini Pro, read-only(코드 수정 불가) |
| 2026-03-12 | ArtifactConsultation 타입 추가 | Consultant 전용 아티팩트 타입, Scout은 기존 ArtifactExploration 재사용 |
| 2026-03-12 | buildProviderWithFallback 모델 오버라이드 통합 | ModelForRole() 체크 → 오버라이드 있으면 NewProviderWithModel(), 없으면 기존 NewProvider() 경로 |
| 2026-03-12 | BackgroundTaskManager 아키텍처 (Overhaul Phase 3) | goroutine 기반 병렬 백그라운드 태스크, 메인 파이프라인과 독립 실행, EventBus 공유로 inline TUI 표시 |
| 2026-03-12 | BackgroundTask 격리 SessionState | 백그라운드 태스크별 독립 SessionState(phase="background"), 메인 파이프라인 아티팩트 간섭 방지 |
| 2026-03-12 | Per-task context.WithCancel | 부모 컨텍스트 캐스케이드 + 개별 태스크 독립 취소 가능, CancelAll()로 일괄 취소 |
| 2026-03-12 | BackgroundTaskDef in ExecutionPlan | Orchestrator가 background_tasks 필드로 선택적 병렬 탐색/자문 지정, scout/consultant 최적 |
| 2026-03-12 | filterValidBackgroundTasks 방어적 검증 | 잘못된 bg task(빈 ID/에이전트/태스크, 미지 역할)를 조용히 제거, 전체 응답 실패 방지 |
| 2026-03-12 | WaitAll() before EventBus.Close() | 백그라운드 태스크 완료 대기 후 EventBus 닫기, 이벤트 유실/panic 방지 |
| 2026-03-12 | TaskCategory 타입 + 8개 카테고리 시스템 (Overhaul Phase 4) | visual-engineering/ultrabrain/deep/artistry/quick/unspecified-low/unspecified-high/writing, CategoryConfig로 tier별 프로바이더/모델 매핑, 카테고리별 행동 프롬프트 |
| 2026-03-12 | Skill 시스템 + SkillRegistry (Overhaul Phase 4) | go:embed 빌트인 스킬 4종(git-master/code-review/testing/documentation), SkillRegistry로 ID→Skill 해석, FormatSkillsContent로 프롬프트 주입 |
| 2026-03-12 | Agent SetCategory/SetSkills 인터페이스 확장 | Agent 인터페이스에 SetCategory/SetSkills 추가, BuildPromptWithContext에서 ## Skills + Category_Context 섹션 주입 |
| 2026-03-12 | AgentBuilder 시그니처 변경 (AgentTask 전달) | func(role string) → func(task AgentTask), 카테고리/스킬 정보 포함 전체 태스크 전달, engine/background/pipeline/github_init 일괄 갱신 |
| 2026-03-12 | buildProviderForTask 카테고리 기반 프로바이더 선택 | 카테고리 설정 시 DefaultCategoryConfigs에서 tier별 프로바이더/모델 사용, 미설정 시 역할 기반 폴백 |
| 2026-03-12 | OrchestratorPrompt CATEGORIES/SKILLS 섹션 추가 | 8개 카테고리 + 4개 스킬 설명, JSON 예시에 category/skills/direct_category/direct_skills 필드 포함 |
| 2026-03-12 | executeTrivial 카테고리/스킬 지원 | DirectCategory/DirectSkills를 executeTrivial에 전달, 카테고리 프로바이더 선택 + 행동 프롬프트/스킬 내용 주입 |
| 2026-03-12 | plan.go 관대한 카테고리 검증 | validatePlan/validateOrchestratorResponse에서 유효하지 않은 카테고리 조용히 제거 (전체 응답 실패 방지) |
| 2026-03-13 | Session Hierarchy (Overhaul Phase 5) | 부모-자식 세션 관계, 파이프라인 실행 추적(pipeline_runs), /load 시 자식 세션 생성, /sessions 트리 뷰 |
| 2026-03-13 | SQLite 스키마 v5 (Session Hierarchy) | parent_session_id 컬럼(session_summaries), pipeline_runs 테이블, pipeline_run_id 컬럼(session_messages) |
| 2026-03-13 | PipelineRun 영속 타입 | id/session_id/parent_run_id/intent/plan_json/status/created_at/completed_at — 파이프라인 실행 이력 추적 |
| 2026-03-13 | SessionState ID 확장 | id/sessionID/parentID 필드 추가, NewSessionStateWithID() 생성자, 백그라운드 태스크별 독립 run ID |
| 2026-03-13 | /load 자식 세션 생성 패턴 | 기존 세션 ID 재사용 대신 새 sessionID 생성 + parentSessionID 설정, 세션 이력 보존 |
| 2026-03-13 | /sessions 트리 뷰 표시 | parent-child 관계 기반 들여쓰기 트리 포맷, formatSessionEntry 재귀 헬퍼 |
| 2026-03-13 | 백그라운드 태스크 결과 DB 저장 | 파이프라인 완료 후 백그라운드 태스크 결과를 session_messages에 저장, pipeline_run_id 연결 |
| 2026-03-13 | 3-Stage Failure Recovery (Overhaul Phase 6) | Stage 1: RetryProvider(기존), Stage 2: Consultant 진단+재시도, Stage 3: RecoveryOverlay 사용자 결정(R/S/A) |
| 2026-03-13 | RecoveryPrompter 인터페이스 | Engine↔TUI 차단 콜백, context.Context 취소 지원, RecoveryContext로 실패 정보 전달 |
| 2026-03-13 | RecoveryBridge Channel-in-Message 패턴 | Engine goroutine→requestCh→TUI RecoveryRequestMsg, 사용자 결정→ReplyCh→Engine unblock |
| 2026-03-13 | RecoveryOverlay TUI 컴포넌트 | Overlay 인터페이스 구현, R/S/A 핫키+↑↓ 탐색+Enter 확인+Esc 중단, 진단/제안 표시 |
| 2026-03-13 | recoveryQueue 동시 실패 처리 | errgroup 병렬 에이전트 동시 실패 대비, 큐 방식 순차 표시, FIFO 처리 |
| 2026-03-13 | EventAgentWarn + EventRecoveryAttempt 이벤트 | 비-critical 실패 ⚠ 경고 + 🔄 복구 시도 Activity 패널 표시 |
| 2026-03-13 | NewEngine 4-param 시그니처 변경 | (pipeline, eb) → (pipeline, eb, prompter, consultant), nil 전달로 복구 비활성화 |
| 2026-03-13 | Consultant 진단 자동 재시도 | Consultant.Run() 실패 시 1회 자동 재시도, 2회 실패 시 진단 없이 Stage 3 진행 |
| 2026-03-13 | MaxRecoveryAttempts=3 강제 중단 | 복구 루프 무한 방지, 3회 시도 후 ActionAbort 강제 반환 |
| 2026-03-13 | Phase A: FileLockManager 도입 | per-file sync.Mutex 기반 동시 쓰기 보호, write_file/patch_file 양쪽에서 사용 |
| 2026-03-13 | Phase A: 원자적 파일 쓰기 (atomicWriteFile) | temp file → rename 패턴, 프로세스 크래시 시 파일 손상 방지, Windows os.Remove 선행 |
| 2026-03-13 | Phase A: write_file/patch_file 안전성 강화 | file lock + backup(.bak) + atomic write 3중 보호, patch_file CRLF 감지 + sort.Slice 정렬 |
| 2026-03-13 | Phase A: read_file offset/limit 파라미터 | 대용량 파일 부분 읽기, 500KB 상한, 메타데이터 헤더(총 라인/경로) 포함 |
| 2026-03-13 | Phase A: 도구 상한 조정 | grep 500매치/10MB, search 300매치/10MB, shell_exec 가변 타임아웃(max 300s)/200KB, git diff 500KB/log 100/30s, list_dir limit/include 파라미터 |
| 2026-03-13 | Phase A: Auto-commit + Undo 인프라 | ToolExecutor.SetAutoCommit() → 파일 변경 시 shadowCommit(git add+commit --no-verify), commitLog 스택으로 Undo(git reset --hard HEAD~1) |
| 2026-03-13 | Phase A: /undo 슬래시 커맨드 | tui/commands.go + Command Palette 10개 항목 확장, 마지막 자동 커밋 되돌리기 |
| 2026-03-13 | Phase A: isInsideDir 공유 유틸리티 이동 | read_file.go → tools.go로 이동, write_file/patch_file에서도 공유 사용 |
| 2026-03-13 | VLLM Provider (Phase B-c) | OpenAI-compatible vLLM 클라이언트, API key optional, Send/Stream, local code gen 전용 |
| 2026-03-13 | VLLMConfig (config.json) | VLLM ProviderConfig 필드 추가, endpoint=http://localhost:8000, model=qwen2.5-coder-7b, enabled=false (opt-in) |
| 2026-03-13 | generate_code 도구 (Phase B-d) | 3모드(instruction/fim/file), FIM 토큰 포맷(Qwen2.5-Coder), context_files 지원, cleanCodeOutput 후처리 |
| 2026-03-13 | SetCodeGenProvider 패턴 | ToolExecutor에 llm.Provider 주입, generate_code 도구 동적 등록, nil 시 graceful error |
| 2026-03-13 | Config View 7-tab 확장 (VLLM 탭 추가) | Claude|Gemini|GPT|GLM|VLLM|Agents|System, VLLM 탭 API Key(optional)/Endpoint/Model, Enabled=endpoint 기반 |
| 2026-03-13 | OrchestratorPrompt SPECIAL TOOLS 섹션 | generate_code 도구 설명 추가, coder 에이전트 보일러플레이트/FIM 생성 안내 |
| 2026-03-13 | tiktoken 기반 토큰 카운트 (Phase C-1) | cl100k_base 인코더, CountTokens 유틸리티, pkoukk/tiktoken-go 라이브러리 |
| 2026-03-13 | ContextBudget P0-P6 우선순위 시스템 (Phase C-2) | System(P0)→History(P1)→Memory(P2)→RepoMap(P3)→Skills(P4)→Category(P5)→Task(P6), AllocateBudget 자동 배분, Enforce 초과분 트리밍 |
| 2026-03-13 | ModelRegistry 모델별 토큰 한도 (Phase C-2) | 프로바이더/모델별 MaxTokens/OutputTokens 매핑, GetModelInfo/MaxTokensForModel, Provider.ModelInfo() 인터페이스 확장 |
| 2026-03-13 | HistoryWindow 토큰 기반 히스토리 관리 (Phase C-3) | SummarizeOldest(LLM 요약)/DropOldest/KeepAll 3가지 전략, 자동 감지(LLM 가용→요약, 불가→드롭) |
| 2026-03-13 | Agent.SetContextBudget 인터페이스 확장 (Phase C-4) | BuildPromptWithContext에서 ContextBudget 기반 섹션별 토큰 제한, 기존 무제한 모드 하위 호환 |
| 2026-03-13 | Orchestrator Plan에 context_budget 필드 (Phase C-4) | OrchestratorResponse.ContextBudget으로 에이전트별 토큰 배분 Orchestrator가 명시, executeTrivial/pipeline 양쪽 지원 |
| 2026-03-13 | StepCheckpoint + Auto Resume (Phase C-5) | state 패키지에 CheckpointStore 인터페이스, SQLite 스키마 v6(step_checkpoints 테이블), Engine.RunPlanFromStep() 재개, ResumeOverlay 사용자 선택 |
| 2026-03-13 | Import Cycle 해결: checkpoint 타입 state 패키지 배치 | agent→memory→orchestrator→agent 사이클 방지, state 패키지는 zero internal dependency |
| 2026-03-13 | Deferred Overlay 패턴 (pendingResumeRun) | initMemory 시점에 WindowSize 미확정→pendingResumeRun 저장→첫 WindowSizeMsg에서 오버레이 표시 |
| 2026-03-13 | Review Feedback Loop (Phase C-6) | ExecutionStep.IsReview/ReviewTarget/MaxReviewIterations, extractReviewIssues 패스 판정(LGTM/no issues/approved), 최대 2회 반복 |
| 2026-03-13 | Conditional Re-planning (Phase C-7) | Replanner 인터페이스, OrchestratorReplanner(LLM 기반), 듀얼 트리거(critical failure ActionAbort + review exhaustion), 90s 타임아웃 |
| 2026-03-14 | LSP Control Plane 아키텍처 (Phase D-1) | 중앙 레지스트리가 LSP 서버 관리, 에이전트에 도구로 노출, lazy loading, 확장자→서버 자동 라우팅 |
| 2026-03-14 | Custom JSON-RPC 클라이언트 | go.lsp.dev/protocol 대신 자체 JSON-RPC 구현, 외부 의존성 zero, Content-Length 헤더 프로토콜 |
| 2026-03-14 | LSP 다국어 인프라 + Go 우선 구현 | extensionToLanguage 매핑으로 다국어 인프라 구축, 실제 연동은 gopls(Go) 먼저, pyright/ts-server는 Enabled=false |
| 2026-03-14 | LSP 도구 6개 에이전트 노출 | lsp_diagnostics/definition/references/hover/rename/symbols, DidOpen/DidClose 라이프사이클 관리 |
| 2026-03-14 | LSPConfig (config.json) | enabled/auto_detect/servers(language→command+args+enabled), DefaultLSPConfig으로 gopls 기본 활성 |
| 2026-03-14 | OrchestratorPrompt LSP TOOLS 섹션 | LSP 도구 설명 + IMPORTANT LSP RULES (변경 후 diagnostics 필수, rename 전 references 필수 등) |
| 2026-03-14 | 구조화 테스트 러너 (Phase D-2) | go test -json 파싱, 패스/실패/스킵 카운트, 실패 세부 정보, JSON 파싱 실패 시 raw 폴백 |
| 2026-03-14 | 의존성 그래프 도구 (Phase D-2) | go list -json 기반 패키지 의존성 분석, stdlib/internal/external 분리, json.Decoder 연속 파싱 |
| 2026-03-14 | ast-grep CLI 래핑 (Phase D-2) | 3-tier 프로비저닝(custom→PATH→cache, 자동 다운로드 없음), ast_search/ast_replace 도구, dry-run 기본 |
| 2026-03-14 | OrchestratorPrompt D-2 도구 섹션 | TEST RUNNER + DEPENDENCY ANALYSIS + AST TOOLS 섹션 추가 |
| 2026-03-15 | 커스텀 스킬 시스템 (Phase E-1) | YAML frontmatter 파서(description/globs), LoadCustomSkills(글로벌/로컬), Source 우선순위(project>global>builtin), MatchSkillsForFile 자동 활성화 |
| 2026-03-15 | SkillsConfig (config.json) | enabled/global_dir/auto_load, DefaultSkillsConfig, GlobalSkillsDir() 해석 |
| 2026-03-15 | BuildOrchestratorPrompt 동적 스킬 주입 | 커스텀 스킬 description을 Orchestrator 프롬프트에 런타임 주입, OrchestratorPrompt const 유지 |
| 2026-03-15 | 자율 루프 (Phase E-2) | VerifyFunc 타입 + 빌트인 검증기 5종(Build/Test/BuildAndTest/Command/Chain), RunAutonomous verify-gated 루프, 반복 감지(동일 feedback 2회 → 중단), DefaultMaxAutonomousIterations=5 |
| 2026-03-15 | AgentTask Autonomous 필드 (Phase E-2) | autonomous/verify_with/max_retries JSON 필드, Orchestrator가 검증 전략 지정, buildAgent에서 SetAutonomous 와이어링 |
| 2026-03-15 | MCP Client 아키텍처 (Phase E-3) | 자체 JSON-RPC 클라이언트(LSP 패턴 재사용), stdio 전송, MCP 2024-11-05 프로토콜, 외부 의존성 zero |
| 2026-03-15 | MCP Manager (Phase E-3) | 다중 서버 관리, Connect()로 일괄 연결, DiscoveredTools()로 도구 자동 발견, 개별 서버 실패 non-fatal |
| 2026-03-15 | MCPTool 래퍼 (Phase E-3) | MCP 도구를 tools.Tool 인터페이스로 래핑, mcp_{serverid}_{toolname} 네이밍, JSON Schema → 파라미터 설명 변환 |
| 2026-03-15 | MCPConfig (config.json) | enabled/servers(id+command+args+env+enabled), DefaultMCPConfig(enabled=false opt-in) |
| 2026-03-17 | CLI/Headless 모드 | artemis chat (one-shot), --headless (interactive stdin/stdout), --multi (orchestrated), --race (경쟁 실행) |
| 2026-03-17 | Enter=Send, Shift+Enter=줄바꿈 | Windows 터미널에서 Ctrl+Enter 미작동 → Enter 전송으로 교체 |
| 2026-03-17 | HTTP Client 타임아웃 | 5개 프로바이더 전체 newHTTPClient() (180s timeout, Transport keep-alive) |
| 2026-03-17 | Orchestrator 타임아웃 증가 | 90초 → 5분 (복잡한 코드 생성 지원) |
| 2026-03-17 | maxToolIter 기본값 | 0(무제한) → 20 (runaway loop 방지) |
| 2026-03-17 | toolUsageGuidelines | 에이전트 시스템 프롬프트에 도구 사용 규칙 자동 주입 |
| 2026-03-17 | Engine per-step 타임아웃 | DefaultStepTimeout=3min, SetStepTimeout(), 각 step별 독립 타임아웃 |
| 2026-03-17 | TASK SIZING RULES | Orchestrator 프롬프트에 1태스크=1파일 원칙 추가 |
| 2026-03-17 | Force-kill LSP/MCP 프로세스 | Shutdown()에서 Process.Kill() 추가 — 고아 프로세스 방지 |
| 2026-03-17 | exec.CommandContext 전면 적용 | app.go git diff 호출에 10s 타임아웃 추가 |
| 2026-03-17 | 성능 최적화 | ToolDescriptions 캐시, SQLite 64MB cache+mmap+busy_timeout, async fact usage |
| 2026-03-18 | ARTEMIS.md 자동 로딩 | ARTEMIS.md/AGENTS.md/.artemis/RULES.md 검색 → P1 우선순위 프롬프트 주입 |
| 2026-03-18 | Hooks 시스템 | HookFunc Pre/Post 도구 실행, DangerousCommandHook, FilePathHook, LoggingPostHook |
| 2026-03-18 | 범용 ParallelWorktreeManager | tools/worktree.go — Create/GetDiff/MergeBack/CleanupAll, CLI --race 모드 |
| 2026-03-18 | 시맨틱 Context Engine | CodeIndex(코드 청크 분할+임베딩), VectorStore codeChunks 컬렉션, BuildPromptWithContext P3 자동 주입 |
| 2026-03-18 | Flow Awareness | FlowTracker(파일 접근 빈도+최근 편집 추적), FlowAwareHook(PostHook 자동 기록), BuildPromptWithContext P1 "Recent Activity" 자동 주입 |

---

## 현재 상태

### 구현 완료
- [x] Go 모듈 초기화 + 프로젝트 구조
- [x] TUI 레이아웃 (B형 Two-Panel Split)
- [x] Chat 패널 (메시지 표시, viewport 스크롤, 텍스트 래핑)
- [x] Activity 패널 (작업 상태 + 파일 변경 목록)
- [x] Status Bar (모델명, 토큰, 키 힌트)
- [x] 텍스트 입력 + 포커스 전환
- [x] Config 시스템 (Load/Save, ~/.artemis/config.json)
- [x] Config View TUI (Ctrl+S, 프로바이더별 API 설정)
- [x] LLM Provider 추상화 인터페이스 (Send/Stream)
- [x] LLM 클라이언트 구현 (Claude, Gemini, GPT, GLM)
- [x] GLM Coding Plan 엔드포인트 별도 지원
- [x] Chat ↔ Config 뷰 전환
- [x] 비동기 LLM 호출 + 응답 표시
- [x] SessionState (Blackboard) — 스레드 안전 Artifact 시스템
- [x] EventBus — 에이전트 → TUI 이벤트 전달 (buffered channel)
- [x] Agent 인터페이스 + BaseAgent (LLM 호출, 이벤트 발행, 프롬프트 빌드)
- [x] RoleAgent 팩토리 — 10개 역할별 시스템 프롬프트 + 태스크 설명
- [x] Pipeline 구조체 — 5단계(Analysis→Planning→Architecture→Implementation→Verification)
- [x] Engine — errgroup 병렬 실행, critical 에이전트 실패 시 파이프라인 중단
- [x] AgentConfig — 역할↔프로바이더 매핑 (config.json 저장)
- [x] TUI 파이프라인 통합 — EventBus 이벤트 Activity 패널 표시, 듀얼 모드(단일/멀티)
- [x] 빌드 검증 통과 (go build + go vet clean)
- [x] FallbackProvider — primary 실패 시 fallback 체인 자동 재시도 (Gemini→GLM 기본)
- [x] Premium/Budget 2-tier AgentConfig — premium(4-provider) + budget(Gemini+GLM) 역할별 매핑, tier 전환
- [x] Config View에 AgentConfig 편집 UI 추가 — Agent 토글, Tier 선택, 역할 매핑 표시
- [x] Status Bar 모드/티어 표시 — Single/Multi + Premium/Budget 배지
- [x] Config View 6-tab 확장 — Claude|Gemini|GPT|GLM|Agents|System, Ctrl+←→ 탭 전환
- [x] Orchestrator-First 라우팅 — Orchestrator가 의도 분석 후 동적 플램 생성, 필요한 에이전트만 호출
- [x] ExecutionPlan 동적 플램 + JSON 파서 (orchestrator/plan.go)
- [x] Engine.RunPlan() — 동적 플램 실행 (Step 순차 + Task 병렬)
- [x] Agent SetTask()/SetCritical() — Orchestrator 태스크 오버라이드
- [x] OrchestratorPrompt — JSON 플램 스키마 포함 시스템 프롬프트
- [x] Agent 출력 이벤트 (EventAgentOutput) → Chat 패널 실시간 표시
- [x] 레거시 파이프라인 폴백 (Orchestrator 실패 시 고정 5단계 파이프라인으로 전환)
- [x] Chat 패널 스크롤 — viewport에 마우스 휠 + ↑↓/PgUp/PgDn 키 전달
- [x] Orchestrator 프롬프트 의도 분류 개선 — 모호/단순 메시지 대화형 라우팅, CRITICAL DISTINCTION 섹션
- [x] Pipeline 완료 시 빈 결과 에러 수정 — HaltedAt 조건 추가로 false positive 제거
- [x] 대화 히스토리 컨텍스트 유지 — 멀티턴 대화 지원, user/assistant 메시지 누적, Ctrl+L 초기화
- [x] 에이전트 Tool 시스템 (5개) — read_file, write_file, list_dir, shell_exec, search_files
- [x] Tool 실행 결과 파일 변경 연동 — EventFileChanged → Activity 패널 Files Changed
- [x] SSE 스트리밍 응답 (4사 전체) — Claude/GPT 실시간 청크, Gemini streamGenerateContent SSE, GLM OpenAI-compatible SSE
- [x] Claude API System 필드 분리 + Gemini SystemInstruction 필드 분리
- [x] RetryProvider — 지수 백오프(1s→2s→4s), 최대 2회 재시도, 비재시도 에러 즉시 반환
- [x] 컨텍스트 타임아웃 — 스트리밍 120s, Orchestrator 90s, 파이프라인 5min
- [x] 모든 Provider에 RetryProvider 래핑 적용 (initProvider + buildProviderWithFallback)
- [x] 파이프라인 결과 구조화 표시 — RoleAgent 메시지 타입, 역할별 색상 스타일링, 구분선+헤더
- [x] 에이전트 대화 컨텍스트 공유 — SessionState.history로 파이프라인 간 대화 히스토리 주입
- [x] Send() HTTP 상태 코드 체크 — 4개 프로바이더 Send() 메서드에 non-200 조기 반환
- [x] strings.Title() 비권장 API 교체 — golang.org/x/text/cases.Title 사용
- [x] GLM reasoning_content 캡처 — StreamChunk.Reasoning + GLM Send()/Stream() 파싱
- [x] 영속 메모리 시스템 (Phase 1) — SQLite+FTS5 기반 3-Tier 메모리 아키텍처
- [x] MemoryStore 인터페이스 + SQLiteStore 구현 — 사실/세션/파일/결정 CRUD + FTS5 검색
- [x] Consolidator — LLM 기반 세션 종료 시 요약·사실 추출·결정 기록
- [x] MemoryConfig — DB 경로, 자동 통합, Fact Decay 설정
- [x] Agent 메모리 통합 — BuildPromptWithContext에서 역할별 영속 사실 자동 주입
- [x] TUI 메모리 통합 — 초기화, 종료 시 통합, 파일 변경 추적, 에이전트에 메모리 전달
- [x] Tool 반복 횟수 Config 설정 — 기본 무제한(0), MaxToolIter config.json + Agent.SetMaxToolIter()
- [x] 세션 메시지 영속 저장 — session_messages 테이블(스키마 v2), 자동 저장 hooks (user/assistant/pipeline)
- [x] 슬래시 커맨드 시스템 — /sessions, /load <id>, /help
- [x] 세션 이어하기 — /load로 이전 세션 대화 복원 (history + chat 재생, sessionID 전환)
- [x] ListSessions — 통합+비통합 세션 UNION 쿼리 조회
- [x] Config View System 탭 — Memory 설정(Enabled/Consolidate/Fact Age/Min Uses) + Tool 설정(Max Iterations) + DB Path 표시
- [x] 멀티 에이전트 스트리밍 (Option A) — streamAndAccumulate + EventBus 청크 전송 + TUI AppendToLast 실시간 표시
- [x] 스트리밍 이벤트 시스템 — EventAgentStreamStart/Chunk/Done + EmitStream* 헬퍼
- [x] 스트리밍 중복 방지 — RoleAgent.Run()에서 streamed 플래그로 EmitOutput 스킵
- [x] 스트리밍 폴백 — Stream() 실패 시 Send() + EmitOutput 경로로 자동 전환
- [x] 벡터 검색 시스템 (Phase 2) — chromem-go + Voyage AI 기반 시맨틱 검색
- [x] VectorStore 래퍼 — chromem-go 영속 DB, 3 컬렉션(facts/sessions/decisions), QueryEmbedding
- [x] Voyage AI EmbeddingFunc — document/query input_type 분리, voyage-code-3 (1024d)
- [x] RRF 하이브리드 검색 — FTS5 + Vector 융합 (0.7/0.3 가중치, k=60), 투명 폴백
- [x] 자동 임베딩 — SaveFact/SaveSession/SaveDecision 시 비동기 goroutine 임베딩
- [x] 시맨틱 중복 체크 — Consolidator에서 코사인 유사도 > 0.85 중복 판정, 단어 겹침 폴백
- [x] VectorConfig — enabled/provider/api_key/model/store_path, config.json + Config View System 탭
- [x] Config View System 탭 8필드 — Vector Search 토글 + Voyage API Key(마스킹) + Model명 추가
- [x] Repo-map 시스템 (Phase 3) — Universal Ctags 기반 코드베이스 구조 인덱싱
- [x] EnsureCTags — 4-tier ctags 바이너리 해석 (PATH→cache→download→fail)
- [x] SymbolParser 인터페이스 + CtagsParser — JSON 출력 파싱, 언어별 export 감지
- [x] RepoMapStore — 파일 인덱싱(SHA256 증분), FTS5 심볼 검색, 트리 포맷 출력
- [x] SQLite 스키마 v3 — repo_symbols 테이블 + FTS5 + 트리거
- [x] RepoMapConfig — enabled/max_tokens/update_on_write/exclude_patterns/ctags_path
- [x] Agent.SetRepoMap() — BuildPromptWithContext에서 역할별 필터링 심볼 주입 ("## Codebase Structure")
- [x] TUI 레포맵 통합 — initMemory()에서 ctags 해석 + RepoMapStore 초기화 + 비동기 인덱싱 + 에이전트 전달
- [x] Config View System 탭 10필드 — RepoMap Enabled 토글 + Max Tokens 입력 + systemFieldToInputIdx 매핑
- [x] UI Overhaul: app.go God Object 분해 — 1410줄 → 496줄, commands/pipeline/streaming/events/memory_init 5개 파일 분리
- [x] UI Overhaul: JSON 테마 시스템 — Theme 구조체, BuildStyles(), go:embed 프리셋 3종(default/dracula/tokyonight), RefreshStyles() 위임
- [x] UI Overhaul: Hybrid Layout (C형) — 기본 Single Panel, 파이프라인 시 자동 Split, 완료 시 Single 복귀, Tab 무효화(Single 모드)
- [x] UI Overhaul: textarea 입력 — textinput → textarea 교체, Ctrl+Enter 전송, 1-8줄 자동 확장, 동적 높이 레이아웃 재계산
- [x] UI Overhaul: glamour 마크다운 렌더링 — Assistant/Agent 메시지 full markdown, 스트리밍 캐시(renderedCache), tool_use 전처리
- [x] UI Overhaul: Activity 패널 리디자인 — Context 섹션(Session/Model/Agents), 경과시간, 3-section 계층 구조
- [x] UI Overhaul: StatusBar 리디자인 — 세션 경과시간, 비용 표시, middle-dot 구분자, Ctrl+↵ Send 힌트
- [x] Config View System 탭 11필드 — Appearance 섹션 추가 (테마 순환 토글, AvailableThemes 연동)
- [x] 병렬 에이전트 스트리밍 레이스 컨디션 수정 — agentStreams map + streamingMsgs map 에이전트/메시지별 추적
- [x] Overlay 다이얼로그 시스템 — PlaceOverlay 합성 (charmbracelet/x/ansi + muesli/ansi), Overlay 인터페이스, OverlayKind/OverlayResult
- [x] Command Palette (Ctrl+K) — 퍼지 검색, 8개 커맨드 항목, 스크롤 가능 필터 리스트
- [x] Agent Selector (Ctrl+A) — 에이전트 활성화/티어 토글, 11개 역할→프로바이더 매핑 표시
- [x] File Picker (Ctrl+O) — 디렉토리 탐색, 파일 선택, .. 상위 이동, 스크롤 윈도우
- [x] 토큰 사용량 추적 + 비용 계산 시스템 — TokenUsage/ModelPricing/CalculateCost + 4사 프로바이더 usage 파싱 + StatusBar 누적 비용 표시
- [x] LLM 에러 분류 시스템 (12 카테고리) — ClassifyError + 사용자 친화 메시지, 파이프라인/스트리밍 에러 서페이싱
- [x] COLD tier JSONL 아카이브 — 세션별 원본 메시지 ~/.artemis/archive/{sessionID}.jsonl, 셧다운 시 자동 아카이브
- [x] Tool 시스템 10개 확장 — 기존 5개 + grep(정규식)/patch_file(라인 편집)/git_status/git_diff/git_log
- [x] 테마 확장 기능 — ExportTheme() + mergeDefaults() + partial themes 지원 + Command Palette "Export Theme"
- [x] VectorSearcher 인터페이스 추출 — chromem-go → sqlite-vec 전환 준비, 3 소비자 마이그레이션
- [x] System 탭 6개 하위 탭 시스템 — 뷰포트 스크롤 제거, sysSubTab(Memory/Tools/Vector/RepoMap/GitHub/Appearance), ←/→ 하위 탭 전환, GitHub 하위 탭 8필드(Enabled/Token/Owner/Repo/PollInterval/AutoTriage/AutoFix/BaseBranch)
- [x] GitHub Issue Processor (Phase 4) — internal/github/processor.go, heuristic triage + draft PR scaffolding(FixIssue)
- [x] GitHub TUI 명령 통합 — /issues 목록/동기화, /fix #N 비동기 실행 결과 메시지
- [x] GitHub syncer lifecycle 통합 — initMemory에서 Start/processor 초기화 + startup triage/report, shutdownMemory에서 Stop
- [x] FixEngine 인터페이스 + AgentFixEngine — Orchestrator 기반 GitHub 이슈 자동 수정 (PipelineRunner 콜백 패턴)
- [x] WorktreeManager — git worktree 생성/정리, mutex 직렬화, Windows 파일 잠금 재시도
- [x] Processor FixEngine 통합 — fixEngine 필드 추가, FixIssue에서 FixEngine→scaffold 폴백 흐름
- [x] TUI PipelineRunner 클로저 — memory_init.go에서 Orchestrator+Engine+ToolExecutor 와이어링, AgentFixEngine 생성
- [x] /fix 타임아웃 제거 — context.Background() 사용, 코드 생성 시간 제한 없음
- [x] Per-fix ToolExecutor — worktree 경로 기반 도구 격리 실행
- [x] LLM 기반 이슈 triage — TriageLLM 콜백, triagePrompt, llmTriage/heuristicTriage 듀얼 경로, JSON 파싱, mapTriageStatus
- [x] TriageLLM 와이어링 — github_init.go에서 analyzer 역할 프로바이더로 콜백 생성, NewProcessor에 주입
- [x] Auto-triage 타임아웃 증가 — 10s→2min (LLM 호출 감안)
- [x] 패키지 구조 정리 — memory_init.go에서 GitHub 초기화 분리 → github_init.go (323줄→179줄+181줄)
- [x] Intent Gate 의도 분류 시스템 (Overhaul Phase 1) — OrchestratorPrompt에 4단계 분류(trivial/conversational/exploratory/complex), OrchestratorResponse 타입, ParseOrchestratorResponse + 레거시 호환
- [x] Intent 기반 TUI 라우팅 — trivial(직접 스트리밍, Single), conversational(1-step plan, Single), exploratory(complex 폴백), complex(Split + ExecutionPlan)
- [x] executeTrivial 직접 스트리밍 경로 — Engine/EventBus 우회, role-specific 프로바이더 + SystemPromptForRole로 경량 실행
- [x] SystemPromptForRole 헬퍼 — roles.go에서 역할 시스템 프롬프트 추출 (RoleAgent 생성 없이 접근)
- [x] Scout/Consultant 에이전트 (Overhaul Phase 2) — RoleScout/RoleConsultant 상수, ScoutPrompt(경량 탐색)/ConsultantPrompt(read-only 자문), 13역할 체계
- [x] Per-Role Model Override — AgentConfig.ModelOverrides + ModelForRole() + NewProviderWithModel() 팩토리
- [x] Scout 기본 설정 — Premium: gemini+gemini-3-flash-preview, Budget: gemini 기본 모델
- [x] Consultant 기본 설정 — Premium: gpt(gpt-5.4), Budget: gemini(gemini-3.1-pro-preview)
- [x] buildProviderWithFallback 모델 오버라이드 통합 — ModelForRole() 체크 → NewProviderWithModel() 분기
- [x] OrchestratorPrompt AVAILABLE AGENTS 확장 — scout/consultant 설명 추가 (13개 에이전트)
- [x] agentDisplayName + modelForRole 확장 — scout/consultant 표시명 + 모델 오버라이드 우선 반환
- [x] RoleTagMap + RepoMapRoleFilter 확장 — scout(code/impl/arch/pattern/search, all symbols), consultant(nil=all tags, exported only)
- [x] BackgroundTaskManager (Overhaul Phase 3) — goroutine 기반 병렬 백그라운드 태스크 관리, TaskStatus 라이프사이클, per-task context 취소
- [x] BackgroundTask 격리 실행 — 독립 SessionState(phase="background"), 메인 파이프라인과 EventBus 공유, non-critical
- [x] BackgroundTaskDef + ExecutionPlan 확장 — OrchestratorResponse에 background_tasks 필드, ToExecutionPlan 변환
- [x] EventBackgroundTask{Start,Complete,Fail} 이벤트 3종 — bus/events.go 확장, TUI Activity 패널 inline 표시
- [x] executePlan() BackgroundTaskManager 와이어링 — pipeline.go에서 bgMgr 생성+Spawn+WaitAll+eb.Close 통합
- [x] OrchestratorPrompt background_tasks 섹션 — RULES에 백그라운드 태스크 가이드 추가, exploratory/complex JSON 예시 확장
- [x] filterValidBackgroundTasks 방어적 검증 — validateOrchestratorResponse에서 잘못된 bg task 필터링(silent strip)
- [x] TaskCategory 타입 + 8개 카테고리 시스템 (Overhaul Phase 4) — visual-engineering/ultrabrain/deep/artistry/quick/unspecified-low/unspecified-high/writing, CategoryConfig로 tier별 프로바이더/모델 매핑, 카테고리별 행동 프롬프트
- [x] Skill 시스템 + SkillRegistry (Overhaul Phase 4) — go:embed 빌트인 스킬 4종(git-master/code-review/testing/documentation), SkillRegistry로 ID→Skill 해석, FormatSkillsContent로 프롬프트 주입
- [x] Agent SetCategory/SetSkills 인터페이스 확장 — BuildPromptWithContext에서 ## Skills + Category_Context 섹션 주입
- [x] AgentBuilder 시그니처 변경 (AgentTask 전달) — func(role string) → func(task AgentTask), engine/background/pipeline/github_init 일괄 갱신
- [x] buildProviderForTask 카테고리 기반 프로바이더 선택 — CategoryConfig에서 tier별 프로바이더/모델 사용, 역할 기반 폴백
- [x] OrchestratorPrompt CATEGORIES/SKILLS 섹션 추가 — 8개 카테고리 + 4개 스킬 설명, JSON 예시에 category/skills 필드 포함
- [x] executeTrivial 카테고리/스킬 지원 — DirectCategory/DirectSkills 전달, 카테고리 프로바이더 선택 + 행동 프롬프트/스킬 내용 주입
- [x] plan.go 관대한 카테고리 검증 — 유효하지 않은 카테고리 조용히 제거 (전체 응답 실패 방지)
- [x] Session Hierarchy (Overhaul Phase 5) — 부모-자식 세션 관계, 파이프라인 실행 추적, 세션 이력 보존
- [x] SQLite 스키마 v5 — parent_session_id 컬럼(session_summaries), pipeline_runs 테이블, pipeline_run_id 컬럼(session_messages), migrateV5
- [x] PipelineRun 영속 타입 + MemoryStore 인터페이스 4개 메서드 확장 — SavePipelineRun/UpdatePipelineRun/GetPipelineRuns/GetChildSessions
- [x] SessionState ID 확장 — id/sessionID/parentID 필드, NewSessionStateWithID() 생성자, getter/setter 6개
- [x] executePlan/executeLegacyPipeline 파이프라인 런 생성 + 상태 추적 + 백그라운드 태스크 결과 DB 저장
- [x] /load 자식 세션 생성 — 새 sessionID + parentSessionID 설정, 세션 이력 보존
- [x] /sessions 트리 뷰 — parent-child 관계 기반 들여쓰기 표시, formatSessionEntry 재귀 헬퍼
- [x] shutdownMemory parentSessionID 연결 — 통합 세션 요약에 부모 세션 ID 연결
- [x] 3-Stage Failure Recovery (Overhaul Phase 6) — Stage 1: RetryProvider(기존), Stage 2: Consultant 진단, Stage 3: 사용자 RecoveryOverlay
- [x] RecoveryPrompter 인터페이스 + RecoveryContext + RecoveryAction (orchestrator/recovery.go)
- [x] Engine 3-Stage Recovery 통합 — attemptRecovery()/consultAgent()/retryFailedAgents()/emitNonCriticalWarnings(), NewEngine 4-param 시그니처
- [x] Agent.OverrideTask() 인터페이스 확장 — 복구 시 실패 에이전트 태스크 식별용
- [x] RecoveryBridge (tui/recoverybridge.go) — Channel-in-Message 패턴, Engine goroutine↔TUI 비동기 통신
- [x] RecoveryOverlay (tui/recoveryoverlay.go) — R/S/A 핫키, ↑↓ 탐색, 진단/제안 표시, Overlay 인터페이스 구현
- [x] OverlayRecovery 추가 — overlay.go OverlayKind enum 확장
- [x] App recoveryQueue + RecoveryRequestMsg/DecisionMsg 핸들러 — 동시 실패 큐 처리, 오버레이 순차 표시
- [x] EventAgentWarn + EventRecoveryAttempt TUI 핸들러 — Activity 패널 ⚠/🔄 표시
- [x] executePlan/executeLegacyPipeline RecoveryBridge 와이어링 — bridge 생성, NewEngine 4-param 호출, tea.Batch 병렬 리스닝
- [x] github_init.go NewEngine 호출 갱신 — nil, nil 전달로 복구 비활성화
- [x] Phase A: FileLockManager — per-file sync.Mutex 기반 동시 파일 쓰기 보호 (tools.go)
- [x] Phase A: 원자적 파일 쓰기 — atomicWriteFile (temp+rename), Windows 호환 (tools.go)
- [x] Phase A: write_file 강화 — file lock + backup(.bak) + atomic write 3중 보호
- [x] Phase A: patch_file 강화 — file lock + backup + atomic write + CRLF 감지 + sort.Slice
- [x] Phase A: read_file offset/limit — 대용량 파일 부분 읽기, 500KB 상한, 메타데이터 헤더
- [x] Phase A: 도구 상한 조정 — grep 500/10MB, search 300/10MB, shell_exec 300s/200KB, git diff 500KB/log 100/30s, list_dir limit/include
- [x] Phase A: Auto-commit + Undo — SetAutoCommit(), shadowCommit(), commitLog 스택, Undo(git reset --hard HEAD~1)
- [x] Phase A: /undo 슬래시 커맨드 — commands.go + Command Palette 10항목 확장
- [x] Phase A: isInsideDir 공유 유틸리티 — read_file.go → tools.go로 이동, write_file/patch_file 공유
- [x] VLLM Provider (Phase B-c) — OpenAI-compatible vLLM 클라이언트, API key optional, Send/Stream, local code gen 전용
- [x] VLLMConfig — ProviderConfig 필드, endpoint=http://localhost:8000, model=qwen2.5-coder-7b, enabled=false (opt-in)
- [x] generate_code 도구 (Phase B-d) — 3모드(instruction/fim/file), FIM 토큰 포맷(Qwen2.5-Coder), context_files, cleanCodeOutput
- [x] SetCodeGenProvider + ToolExecutor 통합 — llm.Provider 주입, generate_code 도구 동적 등록, toolList 11개 확장
- [x] Config View 7-tab 확장 — VLLM 탭 추가(API Key optional/Endpoint/Model), Enabled=endpoint 기반
- [x] TUI vLLM 와이어링 — app.go에서 cfg.VLLM.Enabled 코드제너레이션 프로바이더 자동 연결
- [x] OrchestratorPrompt SPECIAL TOOLS 섹션 — generate_code 도구 설명 추가
- [x] tiktoken 기반 토큰 카운트 (Phase C-1) — cl100k_base 인코더, CountTokens 유틸리티, pkoukk/tiktoken-go
- [x] ContextBudget P0-P6 우선순위 시스템 (Phase C-2) — System(P0)→History(P1)→Memory(P2)→RepoMap(P3)→Skills(P4)→Category(P5)→Task(P6), AllocateBudget 자동 배분, Enforce 트리밍
- [x] ModelRegistry 모델별 토큰 한도 (Phase C-2) — 프로바이더/모델별 MaxTokens/OutputTokens 매핑, GetModelInfo, Provider.ModelInfo() 인터페이스 확장
- [x] HistoryWindow 토큰 기반 히스토리 관리 (Phase C-3) — SummarizeOldest/DropOldest/KeepAll 3전략, 자동 감지(LLM 가용→요약, 불가→드롭)
- [x] Agent.SetContextBudget 인터페이스 확장 (Phase C-4) — BuildPromptWithContext에서 ContextBudget 기반 섹션별 토큰 제한, 기존 무제한 모드 하위 호환
- [x] Orchestrator Plan context_budget 필드 (Phase C-4) — OrchestratorResponse.ContextBudget으로 에이전트별 토큰 배분 명시
- [x] StepCheckpoint + Auto Resume (Phase C-5) — CheckpointStore 인터페이스, SQLite 스키마 v6, Engine.RunPlanFromStep() 재개, ResumeOverlay 사용자 선택
- [x] Review Feedback Loop (Phase C-6) — IsReview/ReviewTarget/MaxReviewIterations, extractReviewIssues 패스 판정, 최대 2회 반복
- [x] Conditional Re-planning (Phase C-7) — Replanner 인터페이스, OrchestratorReplanner(LLM 기반), 듀얼 트리거(failure + review exhaustion)
- [x] LSP Control Plane (Phase D-1) — internal/lsp/ 패키지, JSON-RPC over stdin/stdout, lazy loading, 확장자→서버 라우팅
- [x] LSP Client — custom JSON-RPC 클라이언트, initialize/shutdown lifecycle, publishDiagnostics 수신
- [x] LSPManager — 다국어 Control Plane, ServerConfig, ClientForFile/ClientForLanguage, graceful shutdown
- [x] LSP 도구 6개 — lsp_diagnostics/definition/references/hover/rename/symbols, ToolExecutor.SetLSPManager()
- [x] LSPConfig — enabled/auto_detect/servers(go→gopls/python→pyright/typescript→ts-server), DefaultLSPConfig
- [x] TUI LSP 통합 — initMemory에서 LSPManager 초기화+ToolExecutor 와이어링, shutdownMemory에서 서버 정리
- [x] OrchestratorPrompt LSP TOOLS 섹션 — 도구 설명 + IMPORTANT LSP RULES (변경 후 diagnostics 필수 등)

### 미구현 / Placeholder
- [ ] 모델 전환 UI (현재 Config에서만 변경 가능 — 의도적 제외)

---

## 세션 히스토리

| 세션 | 날짜 | 작업 내용 |
|------|------|-----------|
| #1 | 2026-03-08 | 프로젝트 초기화, TUI 디자인 선정(B형), TUI 베이스 구현 완료 |
| #2 | 2026-03-08 | Config 시스템 + Config View + LLM Provider 4종 구현 + Chat↔Config 뷰 전환 |
| #3 | 2026-03-08 | 멀티 에이전트 시스템 코어 구현 — state/bus/agent/roles/orchestrator/engine + TUI 통합, AgentConfig 역할매핑, 듀얼 모드, FallbackProvider, Premium/Budget 2-tier 시스템 |
| #4 | 2026-03-08 | Config View 5-tab 분리 (프로바이더별 탭 + Agents 탭), 빌드 검증 통과 |
| #5 | 2026-03-09 | Orchestrator-First 라우팅 모델 구현 — ExecutionPlan 동적 플램, Agent SetTask/SetCritical, OrchestratorPrompt, EventAgentOutput 채팅 표시, 레거시 폴백, 빌드 검증 통과 + Chat 스크롤/Orchestrator 프롬프트 편향/Pipeline halt 에러 수정 |
| #6 | 2026-03-09 | 6대 기능 구현 — 대화 히스토리, Tool 시스템(5개), 파일 I/O 연동, SSE 스트리밍(Claude/GPT), RetryProvider+타임아웃, 파이프라인 결과 구조화 표시 |
| #7 | 2026-03-09 | 잔여 기능 완료 — Gemini/GLM 실제 SSE 스트리밍, 에이전트 대화 컨텍스트 공유(파이프라인 간 히스토리 유지) |
| #8 | 2026-03-09 | 마이너 개선 3건(Send HTTP 체크, strings.Title 교체, GLM reasoning_content) + 영속 메모리 시스템 Phase 1 구현 — SQLite+FTS5 3-Tier 메모리, Consolidation, 역할별 태그 필터링, Agent/TUI 통합 |
| #9 | 2026-03-09 | 세션 이어하기 기능 구현 — session_messages 테이블(스키마 v2), 메시지 자동 저장, 슬래시 커맨드(/sessions /load /help), Tool 반복 횟수 설정, Config View System 탭 추가(Memory+Tool), Status Bar 키힌트 갱신 |
| #10 | 2026-03-10 | 멀티 에이전트 스트리밍 빌드 검증 + 메모리 Phase 2 구현 — chromem-go 벡터 저장소, Voyage AI 임베딩(document/query), RRF 하이브리드 검색(FTS5+Vector), 자동 임베딩, 시맨틱 중복 체크, VectorConfig + Config View System 탭 8필드 확장, go build/go vet clean |
| #11 | 2026-03-10 | 메모리 Phase 3 구현 완료 — Repo-map 시스템 (Universal Ctags 파서, 4-tier 프로비저닝, SHA256 증분 인덱싱, FTS5 심볼 검색, 역할별 필터링, 토큰 버짓 포맷팅), RepoMapStore + CtagsParser + EnsureCTags 신규 파일 3개, SQLite 스키마 v3, Agent.SetRepoMap() 통합, TUI initMemory/shutdownMemory/에이전트 전달, Config View System 탭 10필드 확장 (RepoMap Enabled + Max Tokens), systemFieldToInputIdx 매핑, go build/go vet clean |
| #12 | 2026-03-10 | UI Overhaul 구현 — app.go 분해(1410→496줄, 5파일 분리), JSON 테마 시스템(3 프리셋), Hybrid Layout(Single↔Split 자동 전환), textarea 입력(Ctrl+Enter 전송, 1-8줄 자동 확장), glamour 마크다운 렌더링(스트리밍 캐시, tool_use 전처리), Activity 패널 리디자인(Context/Activity/Files 3섹션, 경과시간), StatusBar 리디자인(세션 경과시간, 비용, dot 구분자), go build/go vet clean |
| #13 | 2026-03-10 | Config View 테마 선택기 + 병렬 스트리밍 레이스 컨디션 수정 + Overlay 다이얼로그 시스템 구현 — PlaceOverlay 합성(OpenCode MIT), Command Palette(Ctrl+K 퍼지 검색), Agent Selector(Ctrl+A 토글/티어), File Picker(Ctrl+O 디렉토리 탐색), app.go 오버레이 라우팅·합성 통합, go build/go vet clean |
| #14 | 2026-03-10 | 8항목 개선 계획 완료 — 토큰 사용량/비용 추적 시스템, LLM 에러 분류(12카테고리), COLD tier JSONL 아카이브, Tool 시스템 10개 확장(grep/patch_file/git×3), 테마 확장(ExportTheme/partial themes), VectorSearcher 인터페이스 추출(sqlite-vec 준비), 36개 유닛 테스트, artemis.exe 빌드, go build/go vet clean |
| #15 | 2026-03-10 | Config View System 탭 뷰포트 스크롤 구현 — viewport.Model 도입, PgUp/PgDn/마우스 휠 스크롤, 필드 자동 추적(scrollToField), refreshSystemViewport 헬퍼, AGENTS.md 갱신, go build/go vet clean |
| #16 | 2026-03-10 | GitHub Issue Tracker 잔여 2개 컴포넌트 구현 — internal/github/processor.go(heuristic triage + draft PR scaffolding), TUI /issues /fix 명령 추가, App ghSyncer/ghProcessor 필드 연동, initMemory/shutdownMemory GitHub lifecycle 통합(startup triage/report), go build/go vet clean |
| #17 | 2026-03-10 | 세션 #16 잔여 작업(git commit/push) + System 탭 6개 하위 탭 시스템 — 뷰포트 스크롤 제거, sysSubTab(Memory/Tools/Vector/RepoMap/GitHub/Appearance) ←/→ 전환, GitHub 하위 탭 8필드, canSwitchSubTab 헬퍼(Tools 탭 전용입력 대응), go build/go vet clean |
| #18 | 2026-03-10 | Orchestrator-Driven FixEngine 구현 — FixEngine 인터페이스+AgentFixEngine(PipelineRunner 콜백), WorktreeManager(git worktree 격리+mutex+Windows 재시도), Processor FixEngine 통합(scaffold 폴백), TUI PipelineRunner 클로저 와이어링, /fix 타임아웃 제거, go build/go vet clean |
| #19 | 2026-03-11 | LLM 기반 이슈 triage + 패키지 구조 정리 + Orchestrator/Agent Overhaul Phase 1 — TriageLLM 콜백(analyzer 프로바이더), triagePrompt+JSON 파싱+heuristic 폴백, memory_init.go→github_init.go 분리(323→179+181줄), **Intent Gate 구현**: OrchestratorResponse 타입+ParseOrchestratorResponse(레거시 호환), OrchestratorPrompt 4단계 분류(trivial/conversational/exploratory/complex), Intent 기반 TUI 라우팅(trivial→Single 직접 스트리밍, conversational→Single+tools, complex→Split), executeTrivial 경량 경로, SystemPromptForRole 헬퍼, go build/go vet clean |
| #20 | 2026-03-12 | Orchestrator/Agent Overhaul Phase 1+2 — **Phase 1**: Intent Gate(4단계 분류, OrchestratorResponse, executeTrivial, SystemPromptForRole), **Phase 2**: Scout/Consultant 에이전트 추가(13역할), Per-Role Model Override(ModelOverrides+NewProviderWithModel+buildProviderWithFallback 통합), ArtifactConsultation, OrchestratorPrompt AVAILABLE AGENTS 확장, go build/go vet clean |
| #21 | 2026-03-12 | Overhaul Phase 3 빌드 검증 + BackgroundTaskManager 완성 — unused import 수정, filterValidBackgroundTasks 방어적 검증 추가, go build/go vet clean |
| #22 | 2026-03-12 | Overhaul Phase 4 구현 — **Category + Skill 시스템**: TaskCategory 타입(8개 카테고리) + CategoryConfig(tier별 프로바이더/모델 매핑+행동 프롬프트), Skill 구조체+SkillRegistry(go:embed 빌트인 4종), Agent SetCategory/SetSkills 인터페이스 확장, AgentBuilder 시그니처 변경(AgentTask 전달), buildProviderForTask(카테고리 기반 프로바이더 선택), OrchestratorPrompt CATEGORIES/SKILLS 섹션, executeTrivial 카테고리/스킬 지원, plan.go 관대한 검증, go build/go vet clean |
| #23 | 2026-03-13 | Overhaul Phase 5 구현 — **Session Hierarchy**: SQLite 스키마 v5(parent_session_id/pipeline_runs/pipeline_run_id), PipelineRun 영속 타입+MemoryStore 4개 메서드 확장, SessionState ID 확장(id/sessionID/parentID+NewSessionStateWithID), executePlan/executeLegacyPipeline 파이프라인 런 생성+상태 추적+BG 결과 DB 저장, /load 자식 세션 생성, /sessions 트리 뷰, shutdownMemory parentSessionID 연결, go build/go vet clean |
| #24 | 2026-03-13 | Overhaul Phase 6 구현 — **3-Stage Failure Recovery**: RecoveryPrompter 인터페이스+RecoveryContext(orchestrator/recovery.go), Engine 3-Stage 통합(attemptRecovery/consultAgent/retryFailedAgents/emitNonCriticalWarnings, NewEngine 4-param), Agent.OverrideTask() 확장, RecoveryBridge Channel-in-Message(tui/recoverybridge.go), RecoveryOverlay R/S/A 핫키(tui/recoveryoverlay.go), OverlayRecovery 추가, App recoveryQueue+핸들러, EventAgentWarn/EventRecoveryAttempt TUI 표시, executePlan/executeLegacyPipeline/github_init.go 전체 NewEngine 호출 갱신, go build/go vet clean |
| #25 | 2026-03-13 | Phase A 기반 강화 구현 — **Tool System Hardening**: FileLockManager(per-file mutex), atomicWriteFile(temp+rename), write_file/patch_file 3중 보호(lock+backup+atomic), read_file offset/limit(500KB 상한), 도구 상한 조정(grep 500/search 300/shell_exec 300s/git diff 500KB), Auto-commit+Undo 인프라(shadowCommit+commitLog+SetAutoCommit), /undo 슬래시 커맨드+Command Palette 10항목, isInsideDir 공유 유틸리티, go build/go vet/go test clean |
| #26 | 2026-03-13 | Phase B-c/B-d 구현 — **vLLM Provider + generate_code 도구**: VLLM Provider(OpenAI-compatible, API key optional, Send/Stream), VLLMConfig(endpoint=localhost:8000, model=qwen2.5-coder-7b, enabled=false opt-in), generate_code 도구(3모드 instruction/fim/file, FIM 토큰 Qwen2.5-Coder, context_files, cleanCodeOutput), SetCodeGenProvider(llm.Provider 주입+동적 등록), Config View 7-tab 확장(VLLM 탭+API Key optional/Endpoint/Model), OrchestratorPrompt SPECIAL TOOLS 섹션, TUI vLLM 와이어링(cfg.VLLM.Enabled 자동 연결), go build/go vet/go test clean |
| #27 | 2026-03-13 | Phase B-a/B-b 학습 파이프라인 구현 — **Training Pipeline**: training/ 디렉토리 28개 파일(3,335줄 Python), 데이터 수집(BigQueryCollector+StackV2Collector), 전처리(GoASTExtractor+MinHashDedup+TrainingFormatter 6시나리오), QLoRA 학습(QLoRATrainer+FIMDataCollator FIM/SPM 혼합), 평가(EvaluationSuite exact_match/pass@k/CodeBLEU/syntax/FIM), 배포(merge_adapters+quantize_awq+vLLM serve), CLI 스크립트 5개(01~05), YAML 설정 2개, py_compile 전체 통과 |
| #28 | 2026-03-13 | Phase C-1~C-4 구현 — **지능화 Part ①**: tiktoken 토큰 카운트(cl100k_base), ContextBudget P0-P6 우선순위 시스템(AllocateBudget/Enforce), ModelRegistry(프로바이더별 MaxTokens, Provider.ModelInfo()), HistoryWindow 3전략(Summarize/Drop/KeepAll), Agent.SetContextBudget+BuildPromptWithContext 리팩토링, Orchestrator context_budget 필드, TUI streaming ContextBudget 통합, go build/go vet/go test clean |
| #29 | 2026-03-13 | Phase C-5~C-7 구현 — **지능화 Part ②**: StepCheckpoint+Auto Resume(state/checkpoint.go, SQLite 스키마 v6, Engine.RunPlanFromStep, ResumeOverlay, Deferred Overlay 패턴), Review Feedback Loop(IsReview/ReviewTarget/MaxReviewIterations, extractReviewIssues, EventReviewLoop), Conditional Re-planning(Replanner 인터페이스, OrchestratorReplanner, 듀얼 트리거), go build/go vet/go test clean |
| #30 | 2026-03-14 | Phase D-1/D-2 구현 — **LSP Control Plane**: internal/lsp/ 패키지 3파일(client.go+manager.go+process.go), custom JSON-RPC 클라이언트, LSPManager(다국어 레지스트리+lazy loading), LSP 도구 6개, LSPConfig. **D-2**: 구조화 테스트 러너(go test -json 파싱), 의존성 그래프(go list -json, find_dependents/dependencies), ast-grep(3-tier 프로비저닝, ast_search/ast_replace dry-run), OrchestratorPrompt 전체 도구 섹션 갱신, 22개 도구 체계, go build/go vet/go test clean. **잔여 작업**: Config View LSP 하위 탭(7개 서브탭). **통합 테스트**: 7/7 통과 — Gemini Send/Stream, GLM Send, Memory SQLite, Tool Executor(4종), LSP gopls, Orchestrator 의도 분류 |
| #31 | 2026-03-15 | Phase D 잔여 작업 완료 + **통합 테스트 11/11** + Phase E 전체 구현 — Config View LSP 하위 탭(7→7 서브탭), 심화 통합 테스트 4/4(Orchestrator→ExecutionPlan, 멀티 에이전트 Engine, FallbackProvider, Checkpoint), **E-1 커스텀 스킬**: YAML frontmatter 파서(description/globs), LoadCustomSkills(글로벌/로컬), Source 우선순위(project>global>builtin), MatchSkillsForFile, SkillsConfig, BuildOrchestratorPrompt 동적 주입. **E-2 자율 루프**: VerifyFunc 5종(Build/Test/BuildAndTest/Command/Chain), RunAutonomous verify-gated 루프, 반복 감지, AgentTask autonomous/verify_with/max_retries. **E-3 MCP**: internal/mcp/ 3파일(client+manager+process), JSON-RPC/stdio, 다중 서버, 도구 자동 발견, MCPTool 래퍼, MCPConfig, go build/go vet/go test clean |
| #32 | 2026-03-17 | Artemis 시작 가이드(docs/getting-started.md) 작성 (Korean), AGENTS.md 디렉토리 구조 갱신 |
| #33 | 2026-03-17 | Artemis 아키텍처 개요(docs/architecture.md) 작성 (Korean), AGENTS.md 갱신 |
| #34 | 2026-03-17 | Artemis 설정 가이드(docs/configuration.md) 작성 (Korean), AGENTS.md 갱신 |
| #35 | 2026-03-17~18 | **대규모 세션**: Phase F UI 고도화(진행률 바+토큰 추적+테스트 결과+Diff Overlay) + Dogfooding(AD 구현 성공, 8버그 발견·수정) + CLI/Headless 모드(chat/--headless/--multi/--race) + Enter=Send 수정 + HTTP Client 타임아웃(180s 전체 프로바이더) + 안정화(Config View 9서브탭, maxToolIter=20, toolUsageGuidelines, per-step 타임아웃, Force-kill 프로세스, exec.CommandContext 전면 적용) + 문서화(README.md+3개 가이드) + 성능 최적화(ToolDescriptions 캐시, SQLite pragma, async fact) + 경쟁분석(15개 시스템) + **시그니처 기능 5개 구현**: ARTEMIS.md 자동 로딩, Hooks 시스템(Pre/Post+안전 훅 3종), 범용 ParallelWorktreeManager+CLI --race, 시맨틱 Context Engine(CodeIndex 코드 청크+임베딩+P3 자동 주입), Flow Awareness(FlowTracker 파일 접근 추적+P1 자동 주입) |
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
   - **LLM 프로바이더**: 연동 상태 변경 시 갱신
   - **키바인딩**: 추가·변경 시 갱신
2. **Git Commit + Push**: AGENTS.md 갱신 후, 모든 변경사항을 commit하고 origin/main에 push한다
   - 커밋 메시지 형식: `feat/fix/refactor/chore: 세션 #N — 작업 요약`
   - 예시: `feat: session #14 — add streaming cost calculator + AGENTS.md update`
   - AGENTS.md만 변경된 경우: `chore: update AGENTS.md for session #N`

### 갱신 트리거 질문 (에이전트 자체 점검용)
- [ ] 이번 세션에서 새 파일이나 디렉토리를 만들었는가?
- [ ] 기존 구조를 변경(이동, 삭제, 리네임)했는가?
- [ ] 새로운 설계 결정을 내렸는가?
- [ ] 기능 구현 상태가 바뀌었는가?
- [ ] AGENTS.md에 기록되지 않은 새로운 컨텍스트가 있는가?

> 하나라도 **예**라면 AGENTS.md를 갱신한다.

### Git 커밋 규칙
> 세션 종료 시 반드시 `git add -A && git commit && git push origin main`을 수행한다.

- **Remote**: `origin` → `https://github.com/kunho817/Artemis-Project.git`
- **Branch**: `main`
- **커밋 메시지 Prefix**: `feat:` | `fix:` | `refactor:` | `chore:` | `docs:`
- **제외 파일**: `.gitignore`에 정의됨 (바이너리, IDE, OS 아티팩트, .artemis/)
