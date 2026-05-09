# Artemis MVP 5 설계 문서

> MVP 5의 목표는 MVP 4에서 생성된 DecisionRecord와 주요 실행 결과를 local-first Project Memory로 승격하고, Decision Log, Project Rules, Session Summary, Failure Memory를 검색/관리할 수 있는 첫 Memory slice를 만드는 것이다.

---

## 0. 목적

MVP 1은 자연어 요청을 Work Package로 구조화했다.
MVP 2는 그 흐름을 GUI와 event stream으로 조작 가능하게 만들었다.
MVP 3는 승인된 Work Package를 PatchSet, 검증, ReviewResult까지 이어지는 구현 파이프라인으로 연결했다.
MVP 4는 구현 전 토론과 결정을 Brainstorming Room과 DecisionRecord로 구조화했다.

MVP 5는 이 결정과 경험을 다음 작업에서 다시 찾을 수 있는 기억으로 만든다.

```text
DecisionRecord / Session / WorkPackage / ReviewResult
-> Memory extraction candidate 생성
-> 사용자 승인 또는 명시적 저장
-> ProjectMemoryItem 저장
-> Decision Log / Rules / Failure Memory / Session Summary 표시
-> local lexical search
-> 필요 시 다음 Work Package나 Brainstorming source context로 선택
```

MVP 5는 "모든 과거를 자동으로 prompt에 넣는 RAG 시스템"이 아니다.
MVP 5는 장기 프로젝트 맥락 손실을 줄이기 위한 local-first Memory foundation이다.

---

## 1. 전제

### 1.1 MVP 4 완료 상태

MVP 5는 다음 MVP 4 결과 위에서 시작한다.

```text
- BrainstormingSession을 topic 또는 source 기반으로 시작할 수 있다.
- 역할별 contribution, critique, option, DecisionBrief가 구조화되어 저장된다.
- DecisionBrief accept/reject가 가능하다.
- accepted DecisionBrief는 DecisionRecord로 저장된다.
- accepted DecisionRecord는 pending_approval WorkPackage로 변환할 수 있다.
- Brainstorming event timeline과 local trace가 GUI에 표시된다.
- GUI는 Control Plane만 호출한다.
```

### 1.2 MVP 5에서 처음 다루는 문제

MVP 4까지의 산출물은 존재하지만, 다음 세션이나 다음 작업에서 체계적으로 다시 쓰기 어렵다.

MVP 5는 다음 질문을 다룬다.

```text
- 과거에 어떤 결정을 했는가?
- 그 결정을 왜 했는가?
- 현재 프로젝트 규칙은 무엇인가?
- 이전 세션에서 어디까지 진행했는가?
- 어떤 실패가 반복되었는가?
- 다음 Work Package나 Brainstorming에서 어떤 memory를 참고해야 하는가?
```

### 1.3 Memory는 local-first product state다

MVP 5의 기본 저장소는 기존 Artemis SQLite와 JSONL event log다.

```text
기본:
  SQLite tables + FTS5 lexical index

선택 아님:
  Cloud memory store
  external vector database
  remote embedding API
```

Vector search와 automatic RAG는 MVP 5 완료 조건이 아니다.

---

## 2. MVP 5 한 문장 정의

**MVP 5는 DecisionRecord, session result, verification/review failure, project rule을 local Project Memory로 저장하고, GUI에서 Decision Log와 Memory Search를 통해 조회/관리하며, 선택된 memory를 다음 planning context로 연결하는 Memory / Decision Log slice이다.**

---

## 3. 핵심 원칙

### 3.1 Memory는 암묵적으로 생기지 않는다

MVP 5에서 memory는 명시적 source와 함께 생성되어야 한다.

허용:

```text
- accepted DecisionRecord에서 ADR memory 생성
- 사용자가 직접 Project Rule 생성
- session 종료 시 summary candidate 생성 후 저장
- failed/blocked ReviewResult에서 Failure Memory candidate 생성
```

금지:

```text
- 아무 이벤트나 자동으로 장기 memory 저장
- 사용자 승인 없는 project rule 생성
- 실패하지 않은 결과를 failure memory로 저장
- source link 없는 memory 생성
```

### 3.2 Memory는 Control Plane이 소유한다

Control Plane은 canonical Memory state를 저장하고 관리한다.

Control Plane 책임:

```text
- ProjectMemoryItem 저장
- source link 저장
- status 관리
- search index 갱신
- GUI용 상태 정규화
- memory usage event 기록
```

Control Plane이 하지 않는 일:

```text
- LLM으로 memory 요약 자체 판단
- 임의로 prompt에 memory 주입
- 외부 memory store 동기화
```

### 3.3 Agent Backend는 memory candidate만 만든다

Agent Backend는 source artifact를 읽고 MemoryCandidate를 만들 수 있다.

Agent Backend가 하지 않는 일:

```text
- canonical ProjectMemoryItem 저장
- memory status 변경
- search index 직접 조작
- memory를 자동으로 Work Package prompt에 주입
```

### 3.4 모든 Memory는 source-linked여야 한다

Memory는 반드시 근거를 가져야 한다.

source_type 예:

```text
- decision_record
- brainstorming_session
- work_package
- implementation_run
- verification_run
- review_result
- session
- manual
```

`manual` source도 사용자가 입력한 body와 생성 event를 source로 남긴다.

### 3.5 Search는 MVP 5에서 lexical-first다

MVP 5는 SQLite FTS5 또는 동등한 local full-text search로 시작한다.

```text
지원:
- keyword search
- type filter
- tag filter
- source filter
- status filter
- recency sort

제외:
- embedding
- semantic vector search
- reranker
- external web search
```

### 3.6 Memory context injection은 수동 선택만 허용한다

MVP 5에서 memory는 자동으로 Agent prompt에 들어가지 않는다.

허용:

```text
- GUI에서 memory item을 선택한다.
- 선택된 memory id를 Work Package request나 Brainstorming request에 명시적으로 포함한다.
- Control Plane이 selected memory snapshot을 source_context로 전달한다.
```

금지:

```text
- 모든 관련 memory 자동 주입
- hidden prompt context
- memory search 결과 자동 승인
```

---

## 4. MVP 5 범위

### 4.1 포함 범위

```text
1. ProjectMemoryItem domain model 추가
2. MemorySourceLink model 추가
3. MemoryExtractionRun model 추가
4. MemoryCandidate schema 추가
5. DecisionRecord -> ADR memory 승격
6. Session -> SessionSummary memory candidate 생성
7. ReviewResult/VerificationRun -> Failure Memory candidate 생성
8. Manual ProjectRule 생성/수정/archive
9. Memory status 관리
10. SQLite/FTS5 기반 local search
11. Memory list/search/filter API
12. Decision Log API
13. Memory item detail API
14. selected memory context API
15. GUI Memory View 추가
16. GUI Decision Log tab 추가
17. GUI Project Rules tab 추가
18. GUI Failure Memory tab 추가
19. GUI Memory Search
20. Memory item source artifact link 표시
21. Memory event/trace 기록
22. backend contract tests 추가
23. GUI e2e smoke 추가
```

### 4.2 제외 범위

```text
- Vector DB
- external embedding API
- semantic RAG
- 자동 context injection
- memory 기반 autonomous retry loop
- multi-user permission system
- remote sync
- cloud memory backend
- cross-project global memory
- codebase symbol index / repo-map
- risk radar scoring
- quality center
- file write 또는 patch 생성
```

---

## 5. 시스템 구조

```text
React GUI
  -> HTTP / SSE
Control Plane
  -> Memory Store / FTS index
  -> optional internal HTTP
Agent Backend
  -> Memory candidate generation
  -> read-only tools
User Project Repository
```

MVP 5에서도 GUI는 Agent Backend를 직접 호출하지 않는다.

---

## 6. Memory Pipeline

### 6.1 DecisionRecord 승격 흐름

```text
accepted DecisionRecord
  -> promote_to_memory 요청
  -> ADR MemoryCandidate 생성
  -> ProjectMemoryItem 저장
  -> MemorySourceLink 저장
  -> FTS index 갱신
  -> memory.item.created event
```

DecisionRecord는 이미 사용자가 accept한 산출물이므로, MVP 5에서는 승격 요청 자체를 사용자 의도로 본다.

### 6.2 Session summary 흐름

```text
Session
  -> summarize_session 요청
  -> AgentRun / BrainstormingSession / ImplementationRun / DecisionRecord 수집
  -> SessionSummary candidate 생성
  -> 사용자 저장 요청
  -> ProjectMemoryItem 저장
```

MVP 5에서 session summary는 자동 저장하지 않는다.

### 6.3 Failure Memory 흐름

```text
VerificationRun failed/blocked
또는 ReviewResult needs_changes/blocked
  -> create_failure_memory_candidate
  -> 원인, 증상, 회피책, 다음 행동 구조화
  -> 사용자 저장 요청
  -> ProjectMemoryItem 저장
```

Failure Memory는 성공 사례 저장소가 아니다.

### 6.4 Project Rule 흐름

```text
사용자 입력
  -> ProjectRule draft
  -> validation
  -> ProjectMemoryItem 저장
  -> 활성 rule로 표시
```

Project Rule은 다음 작업의 정책/선호로 사용할 수 있지만, MVP 5에서는 수동 선택 context로만 연결한다.

### 6.5 Memory Search 흐름

```text
query + filters
  -> local FTS search
  -> MemorySearchResult
  -> GUI 표시
  -> 사용자가 item 선택
  -> selected memory context로 다음 요청에 연결 가능
```

---

## 7. Memory Type

### 7.1 decision

Accepted DecisionRecord에서 생성되는 ADR 성격의 memory.

```text
포함:
- decision
- rationale
- consequences
- follow_up_actions
- linked_work_package_id
```

### 7.2 session_summary

하나의 Session에서 중요한 결과를 요약한 memory.

```text
포함:
- completed work
- open questions
- pending approvals
- generated decisions
- next actions
```

### 7.3 project_rule

프로젝트에 계속 적용할 규칙 또는 선호.

```text
예:
- GUI는 Control Plane만 호출한다.
- LangSmith Cloud는 기본값이 아니다.
- patch apply는 반드시 approval 이후에만 수행한다.
```

### 7.4 failure

실패나 blocked 상태에서 얻은 재발 방지 memory.

```text
포함:
- symptom
- root_cause
- affected_surface
- recovery_action
- prevention_hint
```

### 7.5 work_note

WorkPackage, ImplementationRun, ReviewResult에서 추출한 작업 맥락.

```text
포함:
- scope decision
- implementation constraint
- verification note
- deferred follow-up
```

MVP 5의 기본 UI에서는 `decision`, `project_rule`, `failure`, `session_summary`를 우선 표시한다.

---

## 8. 도메인 모델

### 8.1 ProjectMemoryItem

```text
ProjectMemoryItem
- id
- project_id
- type
- title
- summary
- body
- tags
- status
- importance
- confidence
- created_by
- source_count
- last_used_at
- created_at
- updated_at
```

type:

```text
- decision
- session_summary
- project_rule
- failure
- work_note
```

status:

```text
- active
- archived
- superseded
```

created_by:

```text
- user
- system
- agent
```

### 8.2 MemorySourceLink

```text
MemorySourceLink
- id
- memory_item_id
- source_type
- source_id
- relation
- created_at
```

relation:

```text
- derived_from
- supports
- contradicts
- supersedes
- follows_up
```

### 8.3 MemoryExtractionRun

```text
MemoryExtractionRun
- id
- project_id
- session_id
- source_type
- source_id
- status
- candidate_count
- created_memory_count
- trace_id
- created_at
- updated_at
```

status:

```text
- queued
- running
- candidate_ready
- completed
- failed
- canceled
```

### 8.4 MemoryCandidate

```text
MemoryCandidate
- id
- extraction_run_id
- type
- title
- summary
- body
- tags
- importance
- confidence
- source_links
- status
- created_at
```

status:

```text
- pending
- accepted
- rejected
```

MVP 5에서는 후보를 저장하지 않고 즉시 ProjectMemoryItem으로 만드는 shortcut도 허용한다. 단, API와 tests에서는 candidate 경로를 검증해야 한다.

### 8.5 MemorySearchResult

```text
MemorySearchResult
- item
- score
- matched_fields
- source_links
- snippet
```

score는 FTS rank 기반 정렬 보조 값이다.

---

## 9. API 설계

### 9.1 Memory Items

```http
GET  /api/projects/{project_id}/memory
POST /api/projects/{project_id}/memory
GET  /api/memory/items/{memory_item_id}
PATCH /api/memory/items/{memory_item_id}
POST /api/memory/items/{memory_item_id}/archive
POST /api/memory/items/{memory_item_id}/restore
```

manual ProjectRule 생성 요청:

```json
{
  "type": "project_rule",
  "title": "GUI calls Control Plane only",
  "summary": "The GUI must not call Agent Backend directly.",
  "body": "All GUI actions go through Control Plane APIs. Agent Backend remains an internal service boundary.",
  "tags": ["architecture", "gui", "control-plane"],
  "importance": "high"
}
```

### 9.2 Memory Search

```http
GET /api/projects/{project_id}/memory/search
```

query parameters:

```text
q
type
tag
status
source_type
source_id
limit
```

예:

```http
GET /api/projects/project_001/memory/search?q=Control+Plane&type=decision&limit=20
```

### 9.3 Decision Log

```http
GET  /api/projects/{project_id}/memory/decisions
POST /api/decision-records/{decision_record_id}/promote-to-memory
```

정책:

```text
- accepted DecisionRecord만 promote 가능하다.
- 동일 DecisionRecord에서 같은 active decision memory를 중복 생성하지 않는다.
- 기존 memory가 있으면 idempotent하게 기존 item을 반환한다.
```

### 9.4 Session Summary

```http
POST /api/sessions/{session_id}/memory-summary
GET  /api/sessions/{session_id}/memory-summary
```

정책:

```text
- summary candidate 생성과 저장을 분리한다.
- 저장된 summary는 session_summary memory item이 된다.
- session이 비어 있으면 blocked candidate를 반환한다.
```

### 9.5 Failure Memory

```http
POST /api/review-results/{review_result_id}/promote-failure-memory
POST /api/verification-runs/{verification_run_id}/promote-failure-memory
```

정책:

```text
- failed/blocked/needs_changes 상태만 failure memory로 승격할 수 있다.
- passed ReviewResult나 passed VerificationRun은 failure memory가 될 수 없다.
```

### 9.6 Selected Memory Context

```http
POST /api/sessions/{session_id}/selected-memory
GET  /api/sessions/{session_id}/selected-memory
DELETE /api/sessions/{session_id}/selected-memory/{memory_item_id}
```

정책:

```text
- 사용자가 선택한 memory만 다음 request context로 전달할 수 있다.
- selected memory는 자동으로 실행을 시작하지 않는다.
- selected memory snapshot은 event와 trace에 남긴다.
```

---

## 10. GUI 설계

MVP 5 GUI는 기존 Project Command Center에 Memory View를 추가한다.

### 10.1 필수 화면

```text
1. Memory tab
2. Memory search input
3. type/status/tag filter
4. Decision Log list
5. Project Rules list
6. Failure Memory list
7. Session Summary list
8. Memory item detail panel
9. source artifact links
10. DecisionRecord -> memory promote button
11. manual ProjectRule create form
12. archive/restore action
13. selected memory context panel
14. memory event timeline
```

### 10.2 Memory View tab 구조

```text
Memory
  - Search
  - Decisions
  - Rules
  - Failures
  - Sessions
  - Selected Context
```

### 10.3 UX 원칙

```text
- Memory item은 source와 생성 경로가 보여야 한다.
- archived item은 기본 검색에서 숨긴다.
- Project Rule은 수동 생성/수정이 가능해야 한다.
- Decision memory는 DecisionRecord 원문으로 이동할 수 있어야 한다.
- Failure Memory는 실패 상태와 recovery action이 먼저 보여야 한다.
- selected memory context는 사용자가 명시적으로 추가/제거할 수 있어야 한다.
```

---

## 11. Agent Backend / Memory Candidate

### 11.1 Memory candidate graph

```text
START
  -> load_memory_source
  -> collect_source_context
  -> classify_memory_type
  -> draft_memory_candidate
  -> validate_memory_candidate
  -> emit_result
  -> END
```

### 11.2 Node 책임

#### load_memory_source

```text
- source_type/source_id 검증
- source artifact 조회 요청 payload 구성
```

#### collect_source_context

Control Plane이 전달한 source snapshot과 read-only project context를 사용한다.

#### classify_memory_type

source 상태를 보고 memory type을 고른다.

```text
DecisionRecord -> decision
Session -> session_summary
ReviewResult needs_changes/blocked -> failure
VerificationRun failed/blocked -> failure
WorkPackage/ImplementationRun -> work_note
```

#### draft_memory_candidate

MemoryCandidate schema로 후보를 만든다.

#### validate_memory_candidate

필수 필드, source link, type/status policy를 검증한다.

---

## 12. Search / Index

### 12.1 SQLite FTS5

MVP 5는 local SQLite FTS5를 기본 검색 엔진으로 사용한다.

index 대상:

```text
- title
- summary
- body
- tags
- source labels
```

### 12.2 Index update

```text
memory.item.created -> upsert FTS row
memory.item.updated -> update FTS row
memory.item.archived -> keep row but status filter excludes by default
memory.item.restored -> include in default search again
```

### 12.3 Ranking

MVP 5 ranking은 단순하다.

```text
1. active item 우선
2. exact title/tag match 우선
3. FTS rank
4. updated_at 최신순
```

---

## 13. Safety / Policy

### 13.1 Memory write policy

```text
- source link 없는 memory 생성 금지
- hidden memory 생성 금지
- rejected DecisionBrief에서 decision memory 생성 금지
- passed verification에서 failure memory 생성 금지
- archived memory는 context selection 불가
```

### 13.2 Prompt injection 방어

Memory body도 untrusted context로 취급한다.

정책:

```text
- Memory item 안의 지시는 system/developer policy를 덮어쓸 수 없다.
- Memory를 command로 실행하지 않는다.
- Memory가 file write, shell command, external request를 요구해도 실행하지 않는다.
- selected memory는 요약된 snapshot으로만 Agent Backend에 전달한다.
```

### 13.3 PII / secret redaction

MVP 5는 최소 redaction hook을 둔다.

```text
- API key 형태 문자열 탐지
- token/password/secret 키워드 탐지
- redaction warning 표시
- 사용자가 override하지 않으면 memory 저장 차단
```

MVP 5에서 완전한 secret scanner를 구현할 필요는 없다.

### 13.4 No automatic RAG

```text
- search 결과를 자동 prompt에 넣지 않는다.
- selected memory만 다음 request context에 들어간다.
- selected memory 사용 event를 남긴다.
```

---

## 14. Event / Trace

### 14.1 Event type

```text
memory.extraction_run.created
memory.extraction_run.started
memory.extraction_run.completed
memory.extraction_run.failed

memory.candidate.created
memory.candidate.accepted
memory.candidate.rejected

memory.item.created
memory.item.updated
memory.item.archived
memory.item.restored
memory.item.selected
memory.item.unselected

memory.search.performed
memory.index.updated

decision_record.promoted_to_memory
session_summary.created
failure_memory.created
project_rule.created

artifact.created
trace.step_recorded
```

### 14.2 Local trace step

```text
- load_memory_source
- collect_source_context
- classify_memory_type
- draft_memory_candidate
- validate_memory_candidate
- store_memory_item
- update_search_index
- select_memory_context
```

---

## 15. 테스트 전략

### 15.1 Backend contract tests

```text
1. manual ProjectRule 생성
2. ProjectMemoryItem schema validation
3. MemorySourceLink 필수 검증
4. accepted DecisionRecord -> decision memory 승격
5. 같은 DecisionRecord 중복 promote idempotency
6. rejected DecisionBrief는 decision memory 승격 차단
7. SessionSummary candidate 생성
8. failed/blocked ReviewResult -> failure memory 승격
9. passed ReviewResult -> failure memory 승격 차단
10. FTS search가 title/summary/body/tag를 검색
11. type/status/tag filter 동작
12. archive된 memory는 기본 검색에서 제외
13. selected memory context 추가/삭제
14. selected memory snapshot이 다음 request payload에 포함 가능
15. memory event와 trace 저장
```

### 15.2 GUI e2e smoke

```text
1. Project open
2. Session create
3. Brainstorming DecisionRecord 생성 또는 fixture 사용
4. DecisionRecord를 Memory로 promote
5. Memory tab open
6. Decision Log에 promoted memory 표시
7. ProjectRule 수동 생성
8. Memory search 수행
9. memory detail/source link 표시
10. memory item을 selected context에 추가
11. selected context에서 제거
12. archive/restore 동작 확인
```

### 15.3 Safety tests

```text
1. source link 없는 memory 생성 차단
2. hidden auto memory 생성 경로 없음
3. archived memory context selection 차단
4. secret-like body 저장 warning 또는 차단
5. GUI가 Agent Backend를 직접 호출하지 않음
6. Memory 생성 중 user project file write가 발생하지 않음
7. Memory 생성 중 shell command 실행이 발생하지 않음
```

---

## 16. MVP 5 완료 조건

MVP 5는 다음 조건을 만족하면 완료로 본다.

```text
1. ProjectMemoryItem을 생성/조회할 수 있다.
2. 모든 memory item은 source link를 가진다.
3. accepted DecisionRecord를 decision memory로 승격할 수 있다.
4. 같은 DecisionRecord promote는 idempotent하다.
5. ProjectRule을 수동 생성/수정/archive/restore할 수 있다.
6. failed/blocked ReviewResult 또는 VerificationRun을 Failure Memory로 승격할 수 있다.
7. passed result는 Failure Memory 승격이 차단된다.
8. SessionSummary memory candidate를 생성할 수 있다.
9. local FTS search가 title/summary/body/tag를 검색한다.
10. type/status/tag/source filter가 동작한다.
11. GUI Memory View에서 Decision Log, Rules, Failures, Session Summary를 볼 수 있다.
12. GUI에서 memory detail과 source link를 볼 수 있다.
13. GUI에서 selected memory context를 추가/제거할 수 있다.
14. selected memory만 다음 planning request context로 전달 가능하다.
15. Memory 생성/검색/선택 event와 local trace가 저장된다.
16. MVP 5에서는 vector DB, external embedding API, automatic RAG가 기본 동작하지 않는다.
17. user project file write와 shell command execution이 발생하지 않는다.
18. backend contract tests가 통과한다.
19. GUI build가 통과한다.
20. GUI e2e smoke가 통과한다.
```

---

## 17. MVP 6와의 경계

MVP 5는 memory를 저장하고 찾는 단계다.
MVP 6는 memory를 포함한 프로젝트 품질/위험 상태를 분석하는 Risk Radar / Quality Center로 넘어갈 수 있다.

MVP 6 후보:

```text
- Risk Radar
- Quality Center
- architecture map
- test/verification history dashboard
- repeated failure detection
- codebase hotspot memory
- richer memory-based context recommendation
- optional vector/semantic search evaluation
```

MVP 5에서 Vector DB를 바로 기본값으로 넣지 않는다.

---

## 18. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
Artemis MVP 5를 구현한다.

MVP 1, MVP 2, MVP 3, MVP 4는 완료된 것으로 본다. 기존 Control Plane, Agent Backend, React GUI 구조를 유지한다.

MVP 5의 목표는 Memory / Decision Log vertical slice다. MVP 4에서 생성된 accepted DecisionRecord를 decision memory로 승격하고, session summary, project rule, failure memory를 ProjectMemoryItem으로 저장/검색/관리할 수 있게 만든다.

Memory는 Control Plane이 canonical state를 소유한다. Agent Backend는 MemoryCandidate 생성만 담당하며, canonical ProjectMemoryItem을 직접 저장하지 않는다. GUI는 Control Plane만 호출해야 한다.

MVP 5는 local-first다. SQLite + FTS5 기반 검색을 기본으로 사용하고, vector DB, external embedding API, automatic RAG, hidden context injection은 포함하지 않는다.

Memory는 반드시 source link를 가져야 한다. selected memory context는 사용자가 명시적으로 선택한 항목만 다음 Work Package request나 Brainstorming request에 연결할 수 있다.

MVP 5는 user project repository에 파일을 쓰지 않는다. shell command 실행, patch 생성/적용, git commit/push, package install, deployment는 범위 밖이다.

필수 구현:
- ProjectMemoryItem / MemorySourceLink / MemoryExtractionRun / MemoryCandidate 모델
- Control Plane Memory API
- DecisionRecord -> decision memory promote
- manual ProjectRule create/update/archive/restore
- SessionSummary memory candidate
- Failure Memory promote from failed/blocked ReviewResult or VerificationRun
- SQLite FTS5 local search
- selected memory context API
- GUI Memory View
- Decision Log / Rules / Failures / Session Summary tabs
- memory detail/source link display
- backend contract tests
- GUI e2e smoke script `scripts/smoke_mvp5_gui.py`

완료 전 검증:
- `.venv` compileall
- full unittest
- FastAPI smoke
- GUI build
- npm audit
- MVP 5 GUI e2e smoke
```
