# Artemis MVP 2 설계 문서

> MVP 2의 목표는 MVP 1 backend foundation을 사용자가 GUI에서 보고 조작할 수 있는 개발 운영 화면으로 연결하는 것이다.

---

## 0. 목적

MVP 1은 사용자 요청을 Work Package로 구조화하고, 상태, 이벤트, 승인 대기, trace correlation을 남기는 backend foundation을 검증했다.

MVP 2는 이 흐름을 GUI와 event stream으로 노출한다.

```text
사용자 GUI 입력
→ Control Plane 작업 생성
→ Agent Backend 실행
→ 실시간 이벤트 표시
→ Work Package 표시
→ 승인/거부 조작
→ local trace/event 확인
```


MVP 2는 아직 코드 수정 도구가 아니다.  
MVP 2는 Artemis가 “채팅창”이 아니라 “개발 운영 화면”으로 동작할 수 있는지 검증하는 GUI foundation slice이다.

---

## 1. 전제

### 1.1 MVP 1 완료 상태

MVP 2는 다음 MVP 1 결과 위에서 시작한다.

```text
- main은 새 Artemis 구현 라인이다.
- legacy/go-tui는 기존 Go TUI 보존 브랜치이다.
- Control Plane과 Agent Backend는 분리되어 있다.
- 사용자 요청은 WorkPackage로 변환된다.
- WorkPackage는 pending_approval 상태로 저장된다.
- AgentRun, Event, Artifact, ApprovalRequest가 저장된다.
- read-only tools만 허용된다.
- 계약 테스트와 FastAPI smoke가 통과한다.
```

### 1.2 Observability 결정

Observability는 기본값이다.  
하지만 기본 backend는 LangSmith Cloud가 아니라 Artemis local trace store이다.

```text
기본:
  Artemis local trace store

선택:
  self-hosted LangSmith endpoint
  explicit opt-in LangSmith Cloud endpoint
```

MVP 2는 local trace/event를 GUI에서 확인할 수 있는 최소 화면을 포함한다.

---

## 2. MVP 2 한 문장 정의

**MVP 2는 MVP 1의 Work Package 생성 흐름을 GUI에서 실행하고, 이벤트 스트림, 결과물, 승인 상태, local trace를 볼 수 있게 만드는 GUI + Event Stream slice이다.**

---

## 3. 핵심 원칙

### 3.1 GUI는 Agent Backend를 직접 호출하지 않는다

GUI는 반드시 Control Plane만 호출한다.

```text
GUI → Control Plane → Agent Backend
```

Agent Backend의 내부 graph, prompt, node, tool 세부사항은 GUI 계약에 노출하지 않는다.

### 3.2 Control Plane은 GUI용 product state를 제공한다

GUI가 필요한 데이터는 Control Plane이 정규화해서 제공한다.

```text
- AgentRun status
- event timeline
- WorkPackage detail
- ApprovalRequest status
- artifact metadata
- trace summary
```

### 3.3 Event stream은 MVP 2의 핵심 기능이다

MVP 2는 “요청을 보냈더니 나중에 결과만 나오는 화면”이 아니다.  
사용자는 AgentRun의 단계 변화, context 수집, WorkPackage draft 생성, 검증, 승인 대기 상태를 시간순으로 볼 수 있어야 한다.

### 3.4 GUI는 운영 도구처럼 설계한다

MVP 2 화면은 마케팅 랜딩 페이지가 아니다.  
조용하고 밀도 있는 개발 운영 UI여야 한다.

```text
- 상태 중심
- 작업 중심
- 결과물 중심
- 승인 중심
- 이벤트와 trace 중심
```

---

## 4. MVP 2 범위

### 4.1 포함 범위

```text
1. GUI app skeleton 생성
2. Control Plane API client 작성
3. 프로젝트 열기 화면 또는 기본 project selector
4. 세션 생성/선택 흐름
5. 사용자 요청 입력 화면
6. Work Package 생성 요청 실행
7. AgentRun 이벤트 스트림 표시
8. WorkPackage 결과 상세 표시
9. Approval approve/reject UI
10. AgentRun/WorkPackage 상태 새로고침
11. local trace/event viewer 최소 화면
12. Control Plane SSE endpoint 또는 polling fallback
13. GUI smoke/e2e 검증
```

### 4.2 제외 범위

```text
- 실제 patch 생성
- 파일 쓰기
- shell 실행
- 테스트 실행
- Diff Viewer
- 코드 리뷰 UI
- Brainstorming Room
- Risk Radar
- Architecture Map
- Memory/RAG/Vector DB UI
- 멀티 사용자 로그인/권한
- 실시간 협업
- Tauri packaging 배포
- plugin/extension 시스템
```

---

## 5. 권장 기술 방향

### 5.1 GUI Client

MVP 2는 React 기반 GUI로 시작한다.

권장:

```text
apps/gui
→ React
→ Vite
→ TypeScript
```

Tauri는 장기적으로 적합하지만 MVP 2에서는 packaging보다 GUI 계약과 운영 경험 검증이 더 중요하다.  
따라서 MVP 2는 browser-first React app으로 시작하고, 이후 같은 React app을 Tauri shell로 감싸는 방향을 열어둔다.

### 5.2 Backend

기존 MVP 1 backend를 유지한다.

```text
services/control_plane
services/agent_backend
```

MVP 2에서 backend는 GUI를 위해 다음을 보강한다.

```text
- event stream endpoint
- async AgentRun 생성 흐름
- trace/event summary endpoint
- project/session 조회 API
```

---

## 6. 시스템 구조

```text
React GUI
  ↓ HTTP / SSE
Control Plane
  ↓ internal HTTP
Agent Backend
  ↓ read-only tools
User Project Repository
```

MVP 2에서도 GUI는 Agent Backend를 직접 알지 않는다.

---

## 7. UX 구성

### 7.1 화면 레이아웃

MVP 2의 첫 화면은 Project Command Center의 최소 버전이다.

```text
┌─────────────────────────────────────────────────────────┐
│ Top Bar: Project, Session, Backend Status               │
├───────────────┬───────────────────────────┬─────────────┤
│ Sidebar       │ Main Work Package Panel   │ Activity    │
│               │                           │ Timeline    │
│ - Project     │ - Request input           │             │
│ - Sessions    │ - Current run status      │ - Events    │
│ - Work Queue  │ - WorkPackage detail      │ - Trace     │
│ - Settings    │ - Approval controls       │ - Artifacts │
└───────────────┴───────────────────────────┴─────────────┘
```

### 7.2 필수 화면

```text
1. Project Setup
   - project name
   - root path
   - open project

2. Work Package Request
   - natural-language request input
   - submit button
   - current AgentRun status

3. Activity Timeline
   - event type
   - timestamp
   - phase
   - compact payload summary

4. Work Package Detail
   - title
   - goal
   - background
   - scope
   - out_of_scope
   - related_files
   - required_agents
   - implementation_steps
   - verification
   - risks
   - completion_criteria

5. Approval Panel
   - pending approval state
   - approve
   - reject
   - resolved state

6. Local Trace / Event Viewer
   - trace id
   - root run summary
   - graph runtime
   - event list
   - artifact list
```

---

## 8. API 보강

MVP 1 API는 유지한다.  
MVP 2는 GUI와 event stream을 위해 다음 API를 추가하거나 보강한다.

### 8.1 Project / Session 조회

```http
GET  /api/projects
GET  /api/projects/{project_id}
GET  /api/sessions?project_id={project_id}
GET  /api/sessions/{session_id}
```

### 8.2 비동기 Work Package 요청

기존 MVP 1의 `POST /api/work-packages/from-request`는 smoke와 호환성을 위해 유지할 수 있다.

MVP 2에서는 GUI용 비동기 요청 endpoint를 추가한다.

```http
POST /api/work-package-requests
```

응답:

```json
{
  "agent_run_id": "run_001",
  "status": "queued",
  "events_url": "/api/agent-runs/run_001/events/stream"
}
```

완료 후 조회:

```http
GET /api/agent-runs/{agent_run_id}
GET /api/agent-runs/{agent_run_id}/result
```

### 8.3 Event Stream

우선순위:

```text
1. SSE
2. polling fallback
```

Endpoint:

```http
GET /api/agent-runs/{agent_run_id}/events/stream
```

SSE event 예시:

```text
event: agent_run.phase_changed
data: {"phase":"collect_context","agent_run_id":"run_001"}
```

Polling fallback:

```http
GET /api/agent-runs/{agent_run_id}/events?after={event_id}
```

### 8.4 Local Trace 조회

```http
GET /api/agent-runs/{agent_run_id}/trace
GET /api/agent-runs/{agent_run_id}/artifacts
```

---

## 9. Backend 보강 요구사항

### 9.1 Async AgentRun

MVP 1의 흐름은 synchronous smoke에 적합했다.  
MVP 2의 GUI event stream을 위해 Control Plane은 AgentRun을 background task로 실행할 수 있어야 한다.

필수 상태 전이:

```text
queued
→ running
→ completed
```

실패:

```text
queued/running
→ failed
```

취소:

```text
queued/running
→ canceled
```

### 9.2 Event Append + Broadcast

Control Plane은 event를 저장하고, 구독 중인 GUI client에 전달한다.

```text
append_event
→ JSONL/SQLite 저장
→ SSE broadcast
```

MVP 2에서는 단일 프로세스 broadcast로 충분하다.  
Redis/pub-sub은 범위에서 제외한다.

### 9.3 Local Trace Store

MVP 2는 local trace를 product feature로 취급한다.

최소 trace 구조:

```text
Trace
- id
- project_id
- session_id
- agent_run_id
- root_name
- status
- started_at
- ended_at
- metadata

TraceStep
- id
- trace_id
- parent_step_id
- name
- type
- status
- inputs_summary
- outputs_summary
- started_at
- ended_at
```

MVP 2에서 전체 LangSmith 대체 기능을 구현할 필요는 없다.  
하지만 GUI가 보여줄 수 있는 local trace summary는 있어야 한다.

---

## 10. GUI State Model

MVP 2 GUI는 다음 상태를 중심으로 구성한다.

```text
AppState
- backendStatus
- currentProject
- currentSession
- currentAgentRun
- eventTimeline
- currentWorkPackage
- currentApproval
- artifacts
- traceSummary
- errors
```

AgentRun 표시 상태:

```text
- idle
- queued
- running
- completed
- failed
- canceled
```

---

## 11. 테스트 전략

MVP 2는 backend contract와 frontend e2e를 함께 검증한다.

### 11.1 Backend 테스트

```text
1. async AgentRun 생성 test
2. event stream endpoint test
3. polling fallback test
4. approval approve/reject state update test
5. local trace summary endpoint test
6. 기존 MVP 1 contract regression test
```

### 11.2 Frontend 테스트

```text
1. GUI build test
2. request form rendering test
3. event timeline rendering test
4. WorkPackage detail rendering test
5. approve/reject interaction test
6. backend unavailable state test
```

### 11.3 E2E smoke

```text
1. Agent Backend server 시작
2. Control Plane server 시작
3. GUI dev server 시작
4. GUI에서 project open
5. GUI에서 request submit
6. timeline에 이벤트 표시 확인
7. WorkPackage detail 표시 확인
8. approve 클릭
9. approved 상태 표시 확인
```

가능하면 Playwright로 검증한다.

---

## 12. MVP 2 완료 조건

MVP 2는 다음 조건을 만족하면 완료로 본다.

```text
1. GUI에서 Control Plane 연결 상태를 확인할 수 있다.
2. GUI에서 project/session을 생성하거나 선택할 수 있다.
3. GUI에서 자연어 요청을 제출할 수 있다.
4. 제출 직후 AgentRun 상태가 표시된다.
5. AgentRun 이벤트가 timeline에 순서대로 표시된다.
6. WorkPackage 결과가 GUI에 구조화되어 표시된다.
7. ApprovalRequest가 GUI에 표시된다.
8. approve/reject 조작이 backend state에 반영된다.
9. local trace/event viewer에서 trace summary를 확인할 수 있다.
10. GUI build가 통과한다.
11. backend contract tests가 통과한다.
12. GUI e2e smoke가 통과한다.
```

---

## 13. MVP 3와의 경계

MVP 2는 Work Package를 승인해도 실제 구현을 시작하지 않는다.

MVP 3에서 시작할 일:

```text
- Implementation Pipeline
- patch proposal
- diff display
- approval 후 patch apply
- test execution
- review result
```

MVP 2에서 approve는 “작업 범위 승인”까지만 의미한다.

---

## 14. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
새 Artemis MVP 2를 구현한다.

MVP 1은 완료된 것으로 본다. 기존 Control Plane과 Agent Backend 구조를 유지한다.

MVP 2의 목표는 MVP 1의 Work Package 생성 흐름을 GUI에서 실행하고, AgentRun 이벤트 스트림, WorkPackage 결과, 승인/거부 상태, local trace/event summary를 확인할 수 있게 만드는 것이다.

GUI는 Control Plane만 호출해야 하며 Agent Backend를 직접 호출하면 안 된다.

MVP 2는 patch 생성, 파일 쓰기, 테스트 실행, Diff Viewer, Brainstorming Room, Risk Radar, Memory/RAG UI를 포함하지 않는다.

권장 구현:
- apps/gui React + Vite + TypeScript
- Control Plane SSE endpoint 또는 polling fallback
- async AgentRun 흐름
- local trace/event summary endpoint
- Playwright 또는 동등한 GUI e2e smoke

핵심 원칙:
- GUI는 개발 운영 화면이지 채팅 전용 UI가 아니다.
- Control Plane은 GUI용 product state를 정규화한다.
- Agent Backend 내부 구현은 GUI 계약에 노출하지 않는다.
- Observability는 local-first이다.
```

---

## 15. Implementation status

Session #48 started the MVP 2 foundation slice:

```text
- apps/gui React + Vite + TypeScript skeleton
- Control Plane project/session listing endpoints
- async POST /api/work-package-requests
- polling GET /api/agent-runs/{agent_run_id}/events?after={event_id}
- SSE GET /api/agent-runs/{agent_run_id}/events/stream
- GET /api/agent-runs/{agent_run_id}/result
- GET /api/agent-runs/{agent_run_id}/trace
- GET /api/agent-runs/{agent_run_id}/artifacts
- local trace summary tables
- neutral trace_id / external_trace_id naming
- MVP 2 backend startup script
- Playwright GUI e2e smoke runner
```
