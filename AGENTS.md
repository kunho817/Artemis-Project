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

---

## 아키텍처

### 디렉토리 구조

```
D:\Artemis_Project\
├── cmd/artemis/main.go            # 엔트리포인트
├── internal/
│   ├── agent/
│   │   ├── agent.go               # Agent 인터페이스 + BaseAgent (LLM 호출, 이벤트, 프롬프트, SetTask/SetCritical/SetMemory/SetRepoMap)
│   │   └── roles/
│   │       ├── prompts.go          # 11개 역할별 시스템 프롬프트 (Orchestrator 포함)
│   │       └── roles.go            # RoleAgent 팩토리 + Orchestrator 태스크 오버라이드
│   ├── bus/
│   │   └── events.go              # AgentEvent 타입 + EventBus (buffered channel)
│   ├── config/
│   │   └── config.go              # 설정 구조체, Load/Save, AgentConfig, MemoryConfig, VectorConfig, RepoMapConfig
│   ├── llm/
│   │   ├── provider.go            # Provider 인터페이스 + 팩토리 + StreamChunk (Reasoning 필드)
│   │   ├── claude.go              # Claude (Anthropic) 클라이언트
│   │   ├── gemini.go              # Gemini (Google) 클라이언트
│   │   ├── gpt.go                 # GPT (OpenAI) 클라이언트
│   │   ├── glm.go                 # GLM (ZhipuAI) 클라이언트 + Coding Plan + reasoning_content
│   │   ├── fallback.go            # FallbackProvider (primary 실패 시 체인 재시도)
│   │   └── retry.go               # RetryProvider (지수 백오프 재시도)
│   ├── memory/
│   │   ├── memory.go              # MemoryStore 인터페이스 + 타입 + TokenBudget + RoleTagMap + Symbol/SymbolKind/RepoMapRoleFilter
│   │   ├── sqlite.go              # SQLiteStore — FTS5 풀텍스트 + RRF 하이브리드 검색, 스키마 마이그레이션 (v3: repo_symbols)
│   │   ├── consolidate.go         # Consolidator — LLM 기반 세션 요약·사실 추출 + 시맨틱 중복 체크
│   │   ├── vector.go              # VectorStore — chromem-go 래퍼, 3 컬렉션, QueryEmbedding
│   │   ├── embedding.go           # Voyage AI EmbeddingFunc (document/query input_type 분리)
│   │   ├── ctags.go               # EnsureCTags — 4-tier ctags 바이너리 해석 (PATH→cache→download→fail)
│   │   ├── parser.go              # SymbolParser 인터페이스 + CtagsParser (JSON 출력 파싱)
│   │   └── repomap.go             # RepoMapStore — 파일 인덱싱, FTS5 심볼 검색, 트리 포맷 출력
│   ├── orchestrator/
│   │   ├── pipeline.go            # Phase, Pipeline 구조체 + 5단계 파이프라인 (레거시)
│   │   ├── plan.go                # ExecutionPlan — Orchestrator 동적 플램 + JSON 파서
│   │   └── engine.go              # Engine (Run: 고정 파이프라인 / RunPlan: 동적 플램)
│   ├── state/
│   │   └── state.go               # SessionState (Blackboard) — 스레드 안전 Artifact 시스템
│   ├── tools/                     # 에이전트 도구 시스템 (5개 도구)
│   └── tui/
│       ├── app.go                 # 메인 모델 + Update 디스패치 + View + 레이아웃 (Hybrid Single/Split) + 오버레이 라우팅
│       ├── overlay.go             # PlaceOverlay 합성 + Overlay 인터페이스 + OverlayKind/OverlayResult + 스타일 헬퍼
│       ├── cmdpalette.go          # Command Palette (Ctrl+K) — 퍼지 검색, 8개 커맨드
│       ├── agentselector.go       # Agent Selector (Ctrl+A) — 에이전트 토글/티어/역할 매핑
│       ├── filepicker.go          # File Picker (Ctrl+O) — 디렉토리 탐색 + 파일 선택
│       ├── commands.go            # 슬래시 커맨드 핸들러 (/sessions, /load, /help)
│       ├── pipeline.go            # Orchestrator 라우팅 + 동적/레거시 파이프라인 실행
│       ├── streaming.go           # 단일 모드 LLM 스트리밍 핸들러
│       ├── events.go              # 에이전트 이벤트 핸들러 + 파이프라인 완료 처리 + 에이전트별 스트리밍 추적
│       ├── memory_init.go         # 메모리 시스템 초기화/종료/메시지 저장
│       ├── chat.go                # 대화 패널 (glamour 마크다운 렌더링, 메시지별 스트리밍 캐시)
│       ├── activity.go            # Activity 패널 (컨텍스트 정보 + 경과 시간 + 파일 변경)
│       ├── configview.go          # Config 뷰 (6-tab: Claude|Gemini|GPT|GLM|Agents|System) + 테마 선택기
│       ├── statusbar.go           # 하단 상태바 (모델, 티어, 토큰, 비용, 세션 경과시간)
│       ├── styles.go              # lipgloss 스타일 정의 (RefreshStyles → theme.S 위임)
│       └── theme/
│           ├── theme.go           # Theme 구조체, BuildStyles(), Load(), AvailableThemes(), go:embed 프리셋
│           └── presets/
│               ├── default.json   # 기본 테마
│               ├── dracula.json   # Dracula 테마
│               └── tokyonight.json # Tokyo Night 테마
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
| `Ctrl+K` | Command Palette (커맨드 팔레트 — 퍼지 검색) |
| `Ctrl+A` | Agent Selector (에이전트 토글/티어 전환) |
| `Ctrl+O` | File Picker (파일 탐색/선택) |

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
