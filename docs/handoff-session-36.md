# Session #36 Handoff: TUI 대대적 개편 + 최적화 + 안정화

> 이 문서는 세션 #35에서 다음 세션으로 전달하는 컨텍스트입니다.

---

## 즉시 수행할 작업

### 1. TUI 대대적 개편

**현재 상태**: TUI는 30+ 세션에 걸쳐 적층식으로 구축됨. 기능은 풍부하지만 UX 일관성이 부족.

**파일 목록** (모두 `internal/tui/`):
- `app.go` — 메인 모델, Update 디스패치, View, 레이아웃
- `chat.go` — 대화 패널 (glamour 마크다운)
- `activity.go` — Activity 패널 (컨텍스트/활동/파일/테스트 결과)
- `statusbar.go` — 하단 상태바
- `overlay.go` — 오버레이 시스템 (6종)
- `styles.go` — lipgloss 스타일
- `configview.go` — Config 뷰 (9 서브탭)
- `streaming.go` — LLM 스트리밍
- `events.go` — 에이전트 이벤트 핸들러
- `pipeline.go` — Orchestrator/파이프라인 실행
- `cmdpalette.go` — Command Palette (Ctrl+K)
- `agentselector.go` — Agent Selector (Ctrl+A)
- `filepicker.go` — File Picker (Ctrl+O)
- `diffoverlay.go` — Diff Viewer (Ctrl+D)
- `recoveryoverlay.go` — 실패 복구 오버레이
- `resumeoverlay.go` — 파이프라인 재개 오버레이
- `memory_init.go` — 메모리/서브시스템 초기화
- `github_init.go` — GitHub 이슈 트래커
- `replanner.go` — 조건부 재계획
- `recoverybridge.go` — Recovery 브릿지
- `commands.go` — 슬래시 커맨드
- `theme/` — 테마 시스템 (3 프리셋)

**개편 방향**:
1. **레이아웃 현대화** — Single/Split 하이브리드를 더 직관적으로. 파이프라인 실행 시 Activity가 옆에 나오는데, 기본 상태에서도 유용한 정보 표시
2. **입력 UX** — Enter=Send, Shift+Enter=줄바꿈 (이미 수정됨). textarea 높이 자동 조절
3. **에러/로딩 표시** — 현재 에러가 chat 메시지로만 표시. 토스트/알림 시스템 도입
4. **키바인딩 힌트** — 상태바에 현재 사용 가능한 키바인딩 동적 표시
5. **스타일 통일** — theme.go의 BuildStyles()를 모든 컴포넌트가 일관되게 사용
6. **반응형** — 터미널 크기 변경에 모든 컴포넌트가 올바르게 반응

### 2. 최적화

**이미 완료된 것**:
- ToolDescriptions 캐시
- SQLite 성능 pragma (64MB cache, mmap, busy_timeout)
- async fact usage increment
- strings.Builder pre-alloc

**추가로 확인할 것**:
- `chat.go` renderedCache가 대량 메시지(100+)에서 메모리 누수 없는지
- `events.go` EventBus 드레인이 goroutine 누수 없는지
- `pipeline.go` 파이프라인 완료 후 리소스 정리 (EventBus.Close 등)
- View() 함수가 매 프레임 불필요한 문자열 할당 하는지

### 3. 안정화

**알려진 이슈**:
- Config View: LSP/Skills/MCP 탭은 토글만 가능, 텍스트 입력 없음
- `Ctrl+C` 중 파이프라인 실행 중이면 graceful 종료 안 될 수 있음
- glamour 렌더링이 매우 긴 코드 블록에서 느릴 수 있음
- Overlay가 터미널이 매우 작을 때 (80x24) 잘릴 수 있음

---

## 프로젝트 현재 상태

### 수치
```
코드: ~29,000줄 Go + 3,335줄 Python
패키지: 14 internal + 1 cmd  
도구: 22+ (MCP 확장 가능)
에이전트: 13 역할
오버레이: 6종
테마: 3 프리셋 (default, dracula, tokyonight)
Config 서브탭: 9개
```

### 완료된 Phase
```
Phase A — 기반 강화 ✅
Phase B — 코드 생성 LLM ✅
Phase C — 지능화 ✅
Phase D — 심화 (LSP/AST/Test) ✅
Phase E — 에이전트 역량 확장 (Skills/자율루프/MCP) ✅
Phase F — UI 고도화 (진행률/토큰/테스트/Diff) ✅
```

### 최근 시그니처 기능 (세션 #35)
1. **ARTEMIS.md 자동 로딩** — ARTEMIS.md/AGENTS.md/.artemis/RULES.md 검색 → P1 프롬프트 주입
2. **Hooks 시스템** — Pre/Post 도구 실행 이벤트 (DangerousCommandHook, FilePathHook)
3. **병렬 Worktree + CLI --race** — 2개 프로바이더 경쟁 실행
4. **시맨틱 Context Engine** — 코드 청크 임베딩 + 자동 관련 코드 주입
5. **Flow Awareness** — FlowTracker 파일 접근 추적 + P1 자동 주입

### CLI/Headless 모드
```bash
artemis                           # TUI
artemis --headless                # stdin/stdout 대화
artemis chat "message"            # one-shot
artemis chat --multi "message"    # orchestrated
artemis chat --race "message"     # 2개 프로바이더 경쟁
artemis chat --agent NAME "msg"   # 특정 에이전트
artemis chat --dir PATH "msg"     # 작업 디렉토리 지정
```

### 자가 발전 검증 완료
- Artemis가 자기 코드를 읽고 → 버그 발견 → 수정 → 빌드 통과 확인됨
- ParseToolInvocations silent JSON drop 버그를 스스로 진단 + 수정

### Dogfooding 결과
- AD (React+TS): 12파일, 8 Artemis 호출, 빌드 PASS
- Novelist (React+TS 소설 작성기): 11파일, 8 Artemis 호출, 빌드 PASS
- Artemis 자체: 자가 분석 + 수정 + 빌드 PASS

---

## 키바인딩 현재 상태

| 키 | 동작 | 상태 |
|----|------|------|
| Enter | 메시지 전송 | ✅ 세션 #35에서 수정 |
| Shift+Enter | 줄바꿈 | ✅ |
| Ctrl+S | Config View | ✅ |
| Ctrl+K | Command Palette | ✅ |
| Ctrl+A | Agent Selector | ✅ |
| Ctrl+O | File Picker | ✅ |
| Ctrl+D | Diff Viewer | ✅ |
| Ctrl+L | 화면 클리어 | ✅ |
| Ctrl+C | 종료 | ⚠️ 파이프라인 중 graceful 미검증 |

---

## 타임아웃 현황 (세션 #35에서 제거)

| 위치 | 이전 | 현재 |
|------|------|------|
| LLM 호출 (pipeline, streaming) | 2-5분 | **없음** (context.Background) |
| Engine per-step | 3분 | **없음** |
| HTTP Client | 180초 | **없음** (Timeout: 0) |
| Orchestrator 호출 | 5분 | **없음** |
| verify (build/test) | 60-120초 | **유지** (빌드 무한루프 방지) |
| 보조 작업 (DB, git status) | 5초 | **유지** |

---

## 아키텍처 핵심 포인트 (TUI 개편 시 참고)

### TUI 렌더링 흐름
```
WindowSizeMsg → 크기 저장
KeyMsg → app.go Update() 라우팅
  → overlay 활성: overlay.Update()
  → config view: configView.Update()
  → 기본: key handler + textarea.Update()
AgentEventMsg → events.go 처리 → activity 갱신
View() → layout 결정 (Single/Split) → 각 컴포넌트 View() 합성
```

### EventBus → TUI 통신
```
Agent goroutine → EventBus.Emit(event)
  → TUI의 tea.Program가 정기적으로 폴링
  → AgentEventMsg로 변환
  → events.go에서 Activity/StatusBar 갱신
```

### 테마 시스템
```
theme/theme.go: Theme struct + BuildStyles()
theme/presets/: JSON 프리셋 (default.json, dracula.json, tokyonight.json)
styles.go: RefreshStyles() → theme.S 위임
```

---

## Git 상태
- Remote: `origin` → `https://github.com/kunho817/Artemis-Project.git`
- Branch: `main`
- Last commit: `790a223` — self-improvement (ParseToolInvocations error feedback)
- Working tree: projects/novelist 있으나 .gitignore됨

---

## 시작 시 할 것

1. `AGENTS.md` 읽기 (전체 프로젝트 지식)
2. `internal/tui/` 디렉토리 전체 파일 목록 확인
3. `app.go` View() 함수 분석 — 현재 레이아웃 이해
4. `styles.go` — 현재 스타일 시스템 이해
5. 개편 계획 수립 → 사용자 확인 → 구현
