# Artemis MVP 1 설계 문서

> 새 Artemis의 첫 번째 구현 단위는 기능 확장이 아니라 시스템 경계와 운영 모델을 검증하는 것이다.

---

## 0. 목적

MVP 1의 목적은 새 Artemis의 핵심 구조를 작게 구현하여 다음 전제를 검증하는 것이다.

```text
사용자 요청
→ Control Plane의 작업 생성
→ Agent Backend의 LangGraph 실행
→ Work Package 초안 생성
→ Control Plane의 상태/이벤트/승인 대기 저장
→ LangSmith trace 기록
```

MVP 1은 코드 수정 도구가 아니다.  
MVP 1은 Artemis가 사용자의 요청을 구조화된 개발 작업으로 바꾸고, 그 과정과 결과를 추적 가능한 상태로 남길 수 있는지 검증하는 foundation slice이다.

---

## 1. 확정된 방향

### 1.1 브랜치 전략

```text
main
→ 새 Artemis
→ GUI + Control Plane + Agent Backend 기반 재설계

legacy/go-tui
→ 기존 Go TUI 구현 보존
→ 참고 구현, 회귀 비교, 아이디어 저장소 역할
```

기존 Go 구현은 폐기하지 않는다.  
다만 새 Artemis의 런타임으로 직접 끌고 오지는 않고, Legacy 자산으로 보존한다.

### 1.2 제품 방향

Artemis는 단순한 로컬 코딩 도구가 아니라 개인 개발자를 위한 개발 조직 운영 시스템이다.

초기 구현은 local-first로 진행하되, 장기적으로 웹 기반 사용이나 협업 가능성을 막지 않는 구조를 선택한다.

### 1.3 Observability 방향

LangSmith는 기본값이다.

Artemis는 초보자용 블랙박스 도구가 아니라, 복잡한 agent workflow를 추적하고 개선할 수 있는 개발자용 시스템이다. 따라서 graph run, agent call, tool call, approval, verification 결과는 기본적으로 추적 가능해야 한다.

민감 정보 문제는 LangSmith를 끄는 방식보다 redaction policy로 해결한다.

---

## 2. MVP 1 한 문장 정의

**MVP 1은 사용자의 자연어 요청을 Work Package로 구조화하고, 그 과정의 상태, 이벤트, 승인 대기, LangSmith trace를 남기는 backend foundation이다.**

---

## 3. 핵심 원칙

### 3.1 Control Plane은 추론하지 않는다

Control Plane은 제품 상태, 승인, 이벤트, 결과물, 정책을 관리한다.  
Agent처럼 판단하거나 계획을 세우지 않는다.

### 3.2 Agent Backend는 제품 상태를 소유하지 않는다

Agent Backend는 intent 분류, context 수집, Work Package 초안 생성, risk hint, plan draft 같은 판단 결과를 생성한다.  
하지만 canonical product state는 Control Plane이 소유한다.

### 3.3 Agent는 자율 작업자가 아니라 검증 가능한 실행 노드이다

Agent는 무제한으로 행동하는 존재가 아니다.  
명확한 입력, 출력, 권한, 책임을 가진 실행 단위여야 한다.

### 3.4 모든 중요한 작업은 구조화된 산출물로 남긴다

Agent Backend의 결과는 raw text가 아니라 안정된 schema로 반환한다.

```text
- IntentResult
- ContextSummary
- WorkPackageDraft
- RiskHint
- FinalAgentRunResult
```

### 3.5 MVP 1은 read-only system이다

MVP 1에서는 프로젝트 파일을 수정하지 않는다.  
파일 쓰기, patch 적용, shell 실행, 테스트 실행은 후속 MVP 범위로 미룬다.

---

## 4. MVP 1 범위

### 4.1 포함 범위

```text
1. 새 main 브랜치 기준 repository 구조 정리
2. 기존 Go 구현을 legacy/go-tui로 보존
3. Control Plane Backend skeleton
4. Agent Backend skeleton
5. Control Plane ↔ Agent Backend API 계약
6. Work Package schema
7. Agent Run schema
8. Event schema
9. Approval 상태 모델
10. LangGraph root graph skeleton
11. classify_intent node
12. collect_context 기본 node
13. create_work_package node
14. validate_work_package node
15. read_file/list_files/grep/git_status read-only tools
16. LangSmith 기본 trace 연결
17. SQLite/JSONL 기반 최소 저장
18. API 테스트 또는 계약 테스트
```

### 4.2 제외 범위

```text
- GUI 구현
- 실제 patch 생성
- 파일 쓰기
- shell 실행
- 테스트 실행
- Diff Viewer
- Brainstorming Room
- Memory/RAG/Vector DB
- Risk Radar
- Architecture Map
- 멀티 사용자/협업 기능
- 배포/업데이트 시스템
```

---

## 5. 시스템 구조

MVP 1에서도 backend는 분리한다.

```text
User / API Client
  ↓
Control Plane Backend
  ↓ internal API
Agent Backend / Intelligence Plane
  ↓ read-only tools
User Project Repository
```

장기 구조는 다음과 같다.

```text
GUI Client
  ↓
Control Plane Backend
  ↓
Agent Backend / Intelligence Plane
  ↓
Project Runtime / Tool Executor
  ↓
User Project Repository
```

MVP 1에서는 GUI가 없으므로 API client 또는 테스트 코드가 GUI 역할을 대신한다.

---

## 6. 책임 분리

### 6.1 Control Plane 책임

Control Plane은 운영 통제 계층이다.

```text
- 프로젝트 열기/등록
- 세션 생성
- 사용자 요청 접수
- AgentRun 생성
- Agent Backend 실행 요청
- WorkPackage canonical state 저장
- 승인/거부/보류/재개 상태 관리
- 이벤트 저장 및 중계
- artifact metadata 저장
- 작업 히스토리 저장
- GUI/API 표시용 상태 정규화
- 프로젝트별 policy 적용
```

Control Plane이 하지 않는 일:

```text
- Agent prompt 조립
- LangGraph node 직접 제어
- context 수집 전략 결정
- 코드 의미 리뷰
- risk 분석 자체 수행
- tool 직접 실행
- 모델 호출
```

### 6.2 Agent Backend 책임

Agent Backend는 판단과 실행 흐름 계층이다.

```text
- 사용자 요청 해석
- intent 분류
- context 수집 계획
- read-only tool 호출
- ContextSummary 생성
- WorkPackageDraft 생성
- RiskHint 생성
- LangGraph workflow 실행
- LangSmith trace 기록
- structured result 반환
```

Agent Backend가 하지 않는 일:

```text
- WorkPackage canonical state 소유
- 승인 상태 저장
- 사용자 설정 저장
- GUI 표시 상태 조립
- 장기 product state 직접 변경
```

---

## 7. MVP 1 LangGraph

### 7.1 Root Graph

```text
START
  ↓
classify_intent
  ↓
collect_context
  ↓
create_work_package
  ↓
validate_work_package
  ↓
emit_result
  ↓
END
```

### 7.2 Node 책임

#### classify_intent

사용자 요청을 작업 유형으로 분류한다.

예상 intent:

```text
- feature_request
- bug_investigation
- refactor_request
- architecture_question
- documentation_request
- planning_request
- unknown
```

#### collect_context

프로젝트의 최소 문맥을 read-only로 수집한다.

MVP 1에서 허용되는 context source:

```text
- repository root
- git status
- 파일 목록
- README.md
- AGENTS.md
- docs 디렉터리의 일부 문서
- rg 기반 관련 파일 후보
```

#### create_work_package

intent와 context를 기반으로 WorkPackageDraft를 생성한다.

#### validate_work_package

생성된 WorkPackageDraft가 필수 필드를 갖추었는지 검증한다.  
이 단계는 모델 판단이 아니라 schema validation과 policy validation 중심이다.

#### emit_result

최종 structured result를 생성하고 이벤트를 발행한다.

---

## 8. 최소 도메인 모델

### 8.1 Project

```text
Project
- id
- name
- root_path
- status
- created_at
- updated_at
```

### 8.2 Session

```text
Session
- id
- project_id
- title
- status
- created_at
- updated_at
```

### 8.3 AgentRun

```text
AgentRun
- id
- project_id
- session_id
- user_request
- status
- intent
- current_phase
- langsmith_trace_id
- created_at
- updated_at
```

AgentRun status:

```text
- queued
- running
- completed
- failed
- canceled
```

### 8.4 WorkPackage

```text
WorkPackage
- id
- project_id
- session_id
- source_agent_run_id
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
- approval_required
- approval_status
- completion_criteria
- status
- created_at
- updated_at
```

WorkPackage status:

```text
- draft
- pending_approval
- approved
- rejected
- canceled
- superseded
```

Approval status:

```text
- not_required
- pending
- approved
- rejected
```

### 8.5 ApprovalRequest

```text
ApprovalRequest
- id
- project_id
- session_id
- target_type
- target_id
- reason
- risk_level
- status
- created_at
- resolved_at
```

### 8.6 Event

```text
Event
- id
- project_id
- session_id
- agent_run_id
- type
- payload
- created_at
```

### 8.7 Artifact

MVP 1에서 artifact는 metadata 중심이다.

```text
Artifact
- id
- project_id
- session_id
- source_agent_run_id
- type
- title
- payload
- created_at
```

MVP 1 artifact type:

```text
- work_package_draft
- context_summary
- intent_result
- final_report
```

---

## 9. WorkPackageDraft Schema

Agent Backend는 다음 구조를 Control Plane으로 반환한다.

```json
{
  "title": "Add Brainstorming Room",
  "goal": "사용자가 여러 Agent 관점으로 아이디어를 검토할 수 있는 기능을 설계한다.",
  "background": "개인 개발자는 혼자서 대안 검토와 반론 생성을 하기 어렵다.",
  "scope": [
    "Brainstorming session 생성",
    "Agent role selection",
    "Discussion event stream",
    "Final recommendation summary"
  ],
  "out_of_scope": [
    "실시간 음성 회의",
    "외부 사용자 초대",
    "멀티 계정 협업"
  ],
  "related_files": [
    "docs/artemis_planning.md"
  ],
  "required_agents": [
    "ProductPlanner",
    "Architect",
    "SecurityReviewer",
    "DevilAdvocate"
  ],
  "implementation_steps": [
    "Brainstorming graph schema를 정의한다.",
    "Brainstorming API contract를 정의한다.",
    "event stream 형식을 확정한다."
  ],
  "verification": [
    "schema validation",
    "API contract test",
    "event stream test"
  ],
  "risks": [
    {
      "level": "medium",
      "description": "Agent 역할 수가 많아지면 latency와 비용이 증가할 수 있다."
    }
  ],
  "approval_required": true,
  "completion_criteria": [
    "Work Package가 생성된다.",
    "승인 대기 상태로 저장된다.",
    "관련 이벤트가 기록된다."
  ]
}
```

---

## 10. API 계약

### 10.1 Control Plane Public API

```http
POST /api/projects/open
POST /api/sessions
POST /api/work-packages/from-request
GET  /api/work-packages/{work_package_id}
GET  /api/agent-runs/{agent_run_id}
GET  /api/agent-runs/{agent_run_id}/events
POST /api/approvals/{approval_id}/approve
POST /api/approvals/{approval_id}/reject
```

### 10.2 Agent Backend Internal API

```http
POST /internal/agent-runs
GET  /internal/agent-runs/{agent_run_id}
GET  /internal/agent-runs/{agent_run_id}/events
POST /internal/agent-runs/{agent_run_id}/cancel
```

### 10.3 Work Package 생성 흐름

```text
1. Client가 Control Plane에 사용자 요청을 보낸다.
2. Control Plane이 AgentRun을 queued 상태로 생성한다.
3. Control Plane이 Agent Backend에 internal agent run을 요청한다.
4. Agent Backend가 LangGraph를 실행한다.
5. Agent Backend가 structured events와 final result를 반환한다.
6. Control Plane이 WorkPackage를 pending_approval 상태로 저장한다.
7. Control Plane이 ApprovalRequest를 생성한다.
8. Client는 events endpoint로 진행 상황을 확인한다.
```

---

## 11. Event 설계

### 11.1 MVP 1 Event Type

```text
agent_run.created
agent_run.started
agent_run.phase_changed
agent_run.completed
agent_run.failed
agent_run.canceled

context.collection_started
context.collection_completed

work_package.draft_created
work_package.validation_passed
work_package.validation_failed
work_package.created
work_package.pending_approval

approval.requested
approval.approved
approval.rejected

artifact.created

trace.langsmith_linked
```

### 11.2 Event 예시

```json
{
  "type": "agent_run.phase_changed",
  "agent_run_id": "run_001",
  "payload": {
    "phase": "collect_context"
  }
}
```

```json
{
  "type": "work_package.draft_created",
  "agent_run_id": "run_001",
  "payload": {
    "title": "Add Brainstorming Room",
    "risk_level": "medium"
  }
}
```

```json
{
  "type": "approval.requested",
  "agent_run_id": "run_001",
  "payload": {
    "approval_id": "approval_001",
    "target_type": "work_package",
    "target_id": "wp_001"
  }
}
```

---

## 12. Tool Layer

MVP 1은 read-only tool만 허용한다.

```text
read_file
list_files
grep
git_status
```

금지:

```text
write_file
patch_file
move_file
delete_file
run_command
run_test
run_build
git_commit
git_reset
external network call
```

Tool 호출은 Agent Backend 내부에서 수행하되, 결과 이벤트는 Control Plane이 저장할 수 있는 형태로 정규화한다.

---

## 13. Storage

MVP 1 저장소는 SQLite + JSONL로 시작한다.

### 13.1 SQLite

저장 대상:

```text
- projects
- sessions
- agent_runs
- work_packages
- approval_requests
- artifacts
```

### 13.2 JSONL

저장 대상:

```text
- events
- raw agent run logs
- raw tool call logs
```

Vector DB는 MVP 1 범위가 아니다.

---

## 14. LangSmith

LangSmith는 기본 추적 계층이다.

MVP 1에서 추적할 항목:

```text
- root graph run
- classify_intent node
- collect_context node
- create_work_package node
- validate_work_package node
- read-only tool calls
- final structured output
```

Control Plane의 Event와 Agent Backend의 LangSmith trace는 correlation id로 연결한다.

```text
agent_run_id
session_id
project_id
langsmith_trace_id
```

---

## 15. 테스트 전략

MVP 1은 계약과 상태 전이를 우선 검증한다.

필수 테스트:

```text
1. WorkPackage schema validation
2. AgentRun lifecycle test
3. Control Plane → Agent Backend contract test
4. Agent Backend final result contract test
5. Event append/read test
6. ApprovalRequest 생성 test
7. read-only tool permission test
8. LangGraph happy path test
9. LangGraph validation failure path test
10. LangSmith trace id 연결 test
```

테스트 완료의 기준은 “모델이 좋은 답변을 냈다”가 아니라 “상태, 이벤트, schema, trace가 일관되게 남는다”이다.

---

## 16. MVP 1 완료 조건

MVP 1은 다음 조건을 만족하면 완료로 본다.

```text
1. 사용자가 자연어 요청을 보낼 수 있다.
2. Control Plane에 AgentRun이 생성된다.
3. Agent Backend가 LangGraph를 실행한다.
4. 실행 이벤트가 저장된다.
5. WorkPackageDraft가 생성된다.
6. Control Plane이 WorkPackage를 저장한다.
7. WorkPackage가 pending_approval 상태가 된다.
8. ApprovalRequest가 생성된다.
9. LangSmith에서 trace를 확인할 수 있다.
10. MVP 1에서는 read-only tool만 사용된다.
11. 계약 테스트가 통과한다.
```

---

## 17. 후속 MVP와의 경계

MVP 1 이후 확장 순서:

```text
MVP 2
→ GUI + Event Stream

MVP 3
→ Implementation Pipeline
→ patch proposal
→ approval 후 patch apply
→ test execution
→ review

MVP 4
→ Brainstorming Room

MVP 5
→ Memory / Decision Log

MVP 6
→ Risk Radar / Quality Center
```

MVP 1에서 가장 중요한 것은 후속 MVP가 의존할 수 있는 안정된 backend foundation을 만드는 것이다.

---

## 18. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
새 Artemis MVP 1을 구현한다.

기존 Go TUI 구현은 legacy/go-tui로 보존하고, main은 새 구조로 시작한다.

MVP 1의 목표는 Control Plane과 Agent Backend를 분리한 상태에서 사용자 요청을 Work Package로 변환하고, 이벤트/상태/승인 대기/LangSmith trace까지 남기는 backend foundation vertical slice를 구현하는 것이다.

patch 생성, 파일 쓰기, GUI, 테스트 실행, Brainstorming Room, Memory/RAG/Vector DB, Risk Radar는 MVP 1 범위에서 제외한다.

핵심 원칙:
- Control Plane은 추론하지 않는다.
- Agent Backend는 제품 상태를 소유하지 않는다.
- MVP 1은 read-only tool만 허용한다.
- 모든 Agent Backend 결과는 structured schema로 반환한다.
- LangSmith는 기본 관측 계층이다.
```
