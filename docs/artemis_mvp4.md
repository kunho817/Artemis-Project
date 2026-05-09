# Artemis MVP 4 설계 문서

> MVP 4의 목표는 여러 Agent 관점으로 기능, 아키텍처, 구현 전략을 토의하고, 그 결과를 구조화된 Decision Record와 Work Package 후보로 연결하는 첫 Brainstorming Room을 만드는 것이다.

---

## 0. 목적

MVP 1은 자연어 요청을 Work Package로 구조화했다.
MVP 2는 그 흐름을 GUI와 event stream으로 조작 가능하게 만들었다.
MVP 3는 승인된 Work Package를 PatchSet, 검증, ReviewResult까지 이어지는 첫 구현 파이프라인으로 연결했다.

MVP 4는 구현 전에 더 나은 방향을 고르는 단계를 만든다.

```text
사용자 주제 또는 기존 산출물
-> BrainstormingSession 생성
-> 역할 기반 Agent 의견 생성
-> 상호 비판과 리스크 검토
-> 대안/추천안 합성
-> DecisionBrief 생성
-> 사용자 승인
-> DecisionRecord 저장
-> 필요 시 WorkPackage 후보 생성
```

MVP 4는 "자율 구현 Agent"를 확장하는 단계가 아니다.
MVP 4는 개인 개발자에게 팀 회의, 아키텍처 리뷰, 반대 의견, 제품 기획 검토를 제공하는 planning slice이다.

---

## 1. 전제

### 1.1 MVP 3 완료 상태

MVP 4는 다음 MVP 3 결과 위에서 시작한다.

```text
- GUI는 Control Plane만 호출한다.
- Control Plane과 Agent Backend는 분리되어 있다.
- Work Package 생성/승인 흐름이 있다.
- ImplementationRun, PatchSet, VerificationRun, ReviewResult가 있다.
- event polling/SSE와 local trace viewer가 있다.
- patch apply와 command execution은 policy gate를 통과한다.
- MVP 3 GUI e2e smoke가 통과한다.
```

### 1.2 MVP 4에서 처음 다루는 문제

MVP 3까지는 사용자가 이미 해야 할 일을 어느 정도 알고 있다는 전제가 강했다.

MVP 4는 다음 질문을 다룬다.

```text
- 이 기능을 정말 해야 하는가?
- 어떤 설계 대안이 있는가?
- 어떤 위험을 먼저 봐야 하는가?
- 구현 순서는 무엇이 현실적인가?
- 반대 의견을 반영하면 Work Package가 어떻게 바뀌어야 하는가?
```

### 1.3 Decision Record는 MVP 5 Memory의 전 단계다

MVP 4에서 `DecisionRecord`를 추가하지만, 이것은 전체 Project Memory가 아니다.

MVP 4의 Decision Record는 다음만 담당한다.

```text
- Brainstorming 결과를 저장한다.
- 사용자가 승인한 결정만 canonical decision으로 남긴다.
- Work Package 변환의 근거를 제공한다.
```

MVP 5에서 다룰 장기 Memory/RAG/Vector DB, 과거 결정 검색, 실패 메모리, 사용자 선호 메모리는 MVP 4 범위가 아니다.

---

## 2. MVP 4 한 문장 정의

**MVP 4는 사용자의 주제나 기존 Artemis 산출물을 여러 Agent 관점으로 토의하고, 대안/리스크/추천안을 구조화해 Decision Record와 Work Package 후보로 연결하는 Brainstorming Room slice이다.**

---

## 3. 핵심 원칙

### 3.1 Brainstorming Room은 채팅방이 아니다

MVP 4의 목표는 자유 채팅 UI가 아니다.

Brainstorming 결과는 반드시 구조화되어야 한다.

```text
- role contribution
- critique
- option
- tradeoff
- risk
- recommendation
- decision brief
```

raw prose만 저장하는 기능은 MVP 4 완료로 보지 않는다.

### 3.2 Control Plane은 canonical state만 소유한다

Control Plane은 BrainstormingSession, DecisionRecord, WorkPackage 변환 상태를 저장한다.

Control Plane이 하지 않는 일:

```text
- Agent 역할별 추론
- 최종 추천안 생성
- 모델 prompt 구성
- 토론 내용 자체 판단
```

### 3.3 Agent Backend는 의견과 추천안을 만들지만 제품 상태를 소유하지 않는다

Agent Backend는 다음을 생성한다.

```text
- BrainstormingContribution
- BrainstormingCritique
- BrainstormingOption
- DecisionBriefDraft
- WorkPackageCandidateRequest
```

그러나 canonical 저장은 Control Plane이 담당한다.

### 3.4 역할과 turn 수는 제한한다

MVP 4는 무한 토론을 만들지 않는다.

기본 정책:

```text
- 기본 role 수: 4개
- 최대 role 수: 6개
- 기본 critique round: 1회
- 최대 critique round: 2회
- 최종 synthesis는 1회
```

긴 토론이나 자동 반복 회의는 MVP 4 범위가 아니다.

### 3.5 Work Package 변환은 사용자 승인 후에만 수행한다

Brainstorming 결과가 바로 Work Package나 ImplementationRun으로 이어지면 안 된다.

```text
DecisionBrief 생성
-> 사용자 accept/reject
-> accepted DecisionRecord 저장
-> 사용자가 명시적으로 convert 요청
-> Work Package draft 생성
-> 기존 approval 흐름으로 진입
```

### 3.6 MVP 4는 파일 변경을 하지 않는다

MVP 4는 user project repository에 파일을 쓰지 않는다.

허용:

```text
- read-only context collection
- Artemis DB/JSONL에 Brainstorming/Decision state 저장
- Work Package draft 생성
```

금지:

```text
- patch 생성
- patch 적용
- shell command 실행
- git commit/push
- package install
- DB migration 실행
- deployment
```

---

## 4. MVP 4 범위

### 4.1 포함 범위

```text
1. BrainstormingSession domain model 추가
2. BrainstormingContribution schema 추가
3. BrainstormingOption / DecisionBrief schema 추가
4. DecisionRecord 최소 모델 추가
5. topic 기반 BrainstormingSession 생성
6. 기존 산출물 기반 BrainstormingSession 생성
   - WorkPackage
   - ImplementationRun
   - ReviewResult
7. Brainstorming mode 선택
8. Agent role 선택 또는 기본 role 자동 선택
9. Agent Backend Brainstorming graph 추가
10. 역할별 의견 생성
11. cross critique 1회 생성
12. 대안 목록과 tradeoff 생성
13. 최종 recommendation 생성
14. DecisionBrief 저장
15. DecisionBrief accept/reject
16. accepted DecisionBrief에서 DecisionRecord 생성
17. accepted DecisionRecord를 Work Package 후보로 변환
18. Brainstorming event polling/SSE
19. local trace에 brainstorming steps 기록
20. GUI Brainstorming Room 화면 추가
21. GUI role contribution / option / decision view 추가
22. backend contract tests 추가
23. GUI e2e smoke 추가
```

### 4.2 제외 범위

```text
- 실시간 다중 사용자 협업
- 외부 사용자 초대
- 음성/영상 회의
- 장기 Memory/RAG/Vector DB
- 과거 decision semantic search
- 자동 patch retry loop
- richer inline code review comments
- 자동 package install
- git commit/push/PR 생성
- deployment
- user/project permission system
- 조직/팀 workspace
- 외부 web search 기반 research
```

---

## 5. 시스템 구조

```text
React GUI
  -> HTTP / SSE
Control Plane
  -> internal HTTP
Agent Backend
  -> Brainstorming LangGraph
  -> read-only tools
User Project Repository
```

MVP 4에서도 GUI는 Agent Backend를 직접 호출하지 않는다.

---

## 6. Brainstorming Pipeline

### 6.1 전체 흐름

```text
START
  -> load_topic_or_source
  -> collect_brainstorming_context
  -> select_roles
  -> generate_role_contributions
  -> cross_critique
  -> synthesize_options
  -> rank_options
  -> create_decision_brief
  -> validate_brainstorming_result
  -> store_result
  -> WAIT_FOR_USER_DECISION
  -> accept_or_reject_decision
  -> optionally_convert_to_work_package
  -> END
```

### 6.2 Brainstorming mode

MVP 4에서 지원할 mode:

```text
free_ideation
  - 가능한 방향을 넓게 찾는다.

architecture_debate
  - 여러 아키텍처 대안을 비교한다.

implementation_strategy
  - 구현 순서, 작업 분할, 검증 전략을 정리한다.

risk_review
  - 실패 가능성, 보안, 유지보수 리스크를 찾는다.

product_planning
  - 사용자 가치, 범위, 우선순위를 검토한다.
```

MVP 4 기본값은 `architecture_debate`이다.

### 6.3 기본 Agent role

```text
moderator
  - 토론을 구조화하고 최종 DecisionBrief를 합성한다.

product_planner
  - 사용자 가치, 범위, 우선순위를 검토한다.

system_architect
  - 아키텍처 대안과 기술 tradeoff를 검토한다.

implementation_planner
  - 구현 순서, 의존성, 테스트 전략을 검토한다.

risk_reviewer
  - 보안, 데이터 손실, 유지보수, 운영 위험을 검토한다.

devil_advocate
  - 약한 가정과 반대 논리를 제시한다.
```

기본 role set:

```text
product_planner
system_architect
implementation_planner
risk_reviewer
```

`devil_advocate`는 architecture_debate와 risk_review mode에서 기본 포함할 수 있다.

### 6.4 GLM role routing

MVP 4는 기존 GLM Coding Plan provider를 그대로 사용한다.

기본 mapping 제안:

| Brainstorming role | Default model | Reason |
|--------------------|---------------|--------|
| moderator | glm-5.1 | synthesis and decision brief |
| system_architect | glm-5.1 | architecture tradeoff analysis |
| product_planner | glm-5 | product planning |
| implementation_planner | glm-5 | implementation sequencing |
| risk_reviewer | glm-4.6 | policy and risk validation |
| devil_advocate | glm-4.7 | critique and weak assumption discovery |

override 예:

```text
ARTEMIS_GLM_MODEL_BRAINSTORMING_MODERATOR
ARTEMIS_GLM_MODEL_BRAINSTORMING_ARCHITECT
ARTEMIS_GLM_MODEL_BRAINSTORMING_PRODUCT_PLANNER
ARTEMIS_GLM_MODEL_BRAINSTORMING_IMPLEMENTATION_PLANNER
ARTEMIS_GLM_MODEL_BRAINSTORMING_RISK_REVIEWER
ARTEMIS_GLM_MODEL_BRAINSTORMING_DEVIL_ADVOCATE
```

---

## 7. 책임 분리

### 7.1 Control Plane 책임

```text
- BrainstormingSession 생성/상태 관리
- Brainstorming source link 저장
- Agent Backend 실행 요청
- Brainstorming result 저장
- DecisionBrief canonical state 관리
- DecisionRecord accept/reject 관리
- Work Package 변환 요청 관리
- event/trace 저장
- GUI용 상태 정규화
```

Control Plane이 하지 않는 일:

```text
- role별 의견 생성
- critique 생성
- 추천안 합성
- 모델 호출
- prompt 조립
```

### 7.2 Agent Backend 책임

```text
- Brainstorming source 해석
- read-only context collection
- role selection 제안
- role contribution 생성
- cross critique 생성
- option/tradeoff synthesis
- DecisionBriefDraft 생성
- WorkPackageCandidateRequest 생성
- structured schema validation 전 단계 출력
```

Agent Backend가 하지 않는 일:

```text
- BrainstormingSession canonical state 저장
- DecisionRecord 승인 상태 저장
- WorkPackage canonical 저장
- 파일 쓰기
- shell command 실행
- patch 생성/적용
```

---

## 8. 도메인 모델

### 8.1 BrainstormingSession

```text
BrainstormingSession
- id
- project_id
- session_id
- source_type
- source_id
- topic
- mode
- status
- current_phase
- selected_roles
- trace_id
- created_at
- updated_at
```

source_type:

```text
- topic
- work_package
- implementation_run
- review_result
```

status:

```text
- queued
- running
- awaiting_decision
- accepted
- rejected
- converted
- failed
- canceled
```

### 8.2 BrainstormingContribution

```text
BrainstormingContribution
- id
- brainstorming_session_id
- role
- stance
- summary
- arguments
- concerns
- suggested_actions
- referenced_artifacts
- created_at
```

stance:

```text
- supportive
- cautious
- opposed
- exploratory
```

### 8.3 BrainstormingCritique

```text
BrainstormingCritique
- id
- brainstorming_session_id
- critic_role
- target_role
- weak_assumptions
- missing_context
- risks
- suggested_revisions
- created_at
```

### 8.4 BrainstormingOption

```text
BrainstormingOption
- id
- brainstorming_session_id
- title
- summary
- benefits
- costs
- risks
- required_work
- verification_hint
- score
- created_at
```

score는 MVP 4에서 절대 점수가 아니라 정렬 보조 값이다.

```text
0.0 <= score <= 1.0
```

### 8.5 DecisionBrief

```text
DecisionBrief
- id
- brainstorming_session_id
- recommendation
- selected_option_id
- rationale
- tradeoffs
- risks
- open_questions
- follow_up_actions
- work_package_candidate
- status
- created_at
```

status:

```text
- pending
- accepted
- rejected
```

### 8.6 DecisionRecord

```text
DecisionRecord
- id
- project_id
- session_id
- brainstorming_session_id
- title
- decision
- rationale
- consequences
- follow_up_actions
- linked_work_package_id
- created_at
```

DecisionRecord는 사용자가 DecisionBrief를 accept한 경우에만 생성한다.

### 8.7 WorkPackageCandidateRequest

```text
WorkPackageCandidateRequest
- title
- goal
- background
- scope
- out_of_scope
- related_files
- implementation_steps
- verification
- risks
- completion_criteria
```

이 schema는 기존 WorkPackageDraft와 호환되도록 만든다.

---

## 9. API 설계

### 9.1 BrainstormingSession

```http
POST /api/brainstorming-sessions
GET  /api/brainstorming-sessions/{brainstorming_session_id}
GET  /api/brainstorming-sessions/{brainstorming_session_id}/result
GET  /api/brainstorming-sessions/{brainstorming_session_id}/events
GET  /api/brainstorming-sessions/{brainstorming_session_id}/events/stream
POST /api/brainstorming-sessions/{brainstorming_session_id}/cancel
```

생성 요청:

```json
{
  "project_id": "project_001",
  "session_id": "session_001",
  "topic": "Plugin system design",
  "mode": "architecture_debate",
  "source_type": "topic",
  "source_id": null,
  "roles": [
    "product_planner",
    "system_architect",
    "implementation_planner",
    "risk_reviewer"
  ]
}
```

기존 산출물 기반 요청:

```json
{
  "project_id": "project_001",
  "session_id": "session_001",
  "topic": "Review MVP 3 patch pipeline follow-up",
  "mode": "implementation_strategy",
  "source_type": "review_result",
  "source_id": "review_001",
  "roles": []
}
```

`roles`가 비어 있으면 mode 기준 기본 role을 사용한다.

### 9.2 Decision

```http
POST /api/brainstorming-sessions/{brainstorming_session_id}/decision/accept
POST /api/brainstorming-sessions/{brainstorming_session_id}/decision/reject
GET  /api/decision-records/{decision_record_id}
GET  /api/projects/{project_id}/decision-records
```

accept 요청:

```json
{
  "decision_brief_id": "brief_001",
  "note": "Use the staged API-first design and defer collaboration features."
}
```

reject 요청:

```json
{
  "decision_brief_id": "brief_001",
  "reason": "The recommendation is too broad for the next implementation session."
}
```

### 9.3 Work Package 변환

```http
POST /api/decision-records/{decision_record_id}/convert-to-work-package
```

정책:

```text
- accepted DecisionRecord에서만 변환 가능하다.
- 변환 결과는 pending_approval WorkPackage가 된다.
- 변환 후에도 Work Package approval은 별도 단계로 유지한다.
- 변환은 ImplementationRun을 자동 시작하지 않는다.
```

---

## 10. GUI 설계

MVP 4 GUI는 기존 Project Command Center에 Brainstorming Room을 추가한다.

### 10.1 필수 화면

```text
1. Brainstorming topic 입력
2. mode 선택
3. role 선택
4. source artifact 선택
5. BrainstormingSession 시작 버튼
6. role별 contribution panel
7. critique timeline
8. option/tradeoff list
9. DecisionBrief panel
10. accept/reject 버튼
11. DecisionRecord 표시
12. Work Package 변환 버튼
13. brainstorming event timeline
14. local trace 표시
```

### 10.2 화면 배치

권장 구조:

```text
Left / Main
  - Topic composer
  - Mode selector
  - Role selector
  - DecisionBrief
  - Option cards

Right / Activity
  - Brainstorming timeline
  - Trace
  - Source artifacts

Lower / Detail
  - Role contributions
  - Critiques
  - WorkPackage candidate preview
```

### 10.3 UX 원칙

```text
- 사용자가 현재 토론 phase를 볼 수 있어야 한다.
- role별 의견은 명확히 구분되어야 한다.
- 추천안은 "왜 이 안을 선택했는지"를 보여야 한다.
- accept/reject는 DecisionBrief에만 적용한다.
- Work Package 변환은 accepted DecisionRecord 이후에만 가능하다.
- 변환된 Work Package는 기존 approval UI로 이어진다.
```

---

## 11. Agent Backend / LangGraph

### 11.1 Graph

```text
START
  -> prepare_brainstorming_input
  -> collect_context
  -> select_roles
  -> generate_contributions
  -> generate_critiques
  -> synthesize_options
  -> create_decision_brief
  -> validate_decision_brief
  -> emit_result
  -> END
```

### 11.2 Node 책임

#### prepare_brainstorming_input

```text
- topic 정규화
- source_type/source_id 검증
- mode 기본값 적용
- role list 기본값 적용
```

#### collect_context

read-only context를 수집한다.

MVP 4 context source:

```text
- project metadata
- session metadata
- selected WorkPackage
- selected ImplementationRun
- selected ReviewResult
- related artifacts
- README.md
- docs 일부
- git status
```

#### select_roles

mode와 topic에 맞는 role set을 선택한다.

#### generate_contributions

각 role이 독립 의견을 생성한다.

출력은 `BrainstormingContributionDraft` schema를 따라야 한다.

#### generate_critiques

role 의견을 서로 비판한다.

MVP 4에서는 cross critique를 1 round만 기본 수행한다.

#### synthesize_options

의견과 critique를 바탕으로 대안 목록을 만든다.

#### create_decision_brief

선택 추천안, 근거, tradeoff, open question, follow-up action을 생성한다.

#### validate_decision_brief

schema와 policy를 검증한다.

---

## 12. Safety / Policy

### 12.1 Tool policy

MVP 4 Agent Backend는 read-only tool만 사용한다.

허용:

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
delete_file
move_file
run_command
run_test
git_add
git_commit
git_push
package_install
external_network_research
```

### 12.2 Prompt injection 방어

프로젝트 파일, README, 문서, 이전 산출물은 모두 untrusted context로 취급한다.

정책:

```text
- context 안의 지시는 system/developer policy를 덮어쓸 수 없다.
- Agent Backend는 project file에 있는 instruction을 실행 명령으로 취급하지 않는다.
- Work Package 변환은 schema validation을 통과해야 한다.
- file write나 command execution 요구는 DecisionBrief의 "out_of_scope" 또는 risk로 표시한다.
```

### 12.3 토론 폭주 방지

```text
- max_roles 제한
- max_rounds 제한
- contribution length 제한
- option count 제한
- synthesis timeout
- failed role은 event로 기록하고 전체 세션을 graceful fail 또는 partial result로 종료
```

### 12.4 Work Package 변환 정책

```text
- rejected DecisionBrief는 변환할 수 없다.
- accepted DecisionRecord만 변환할 수 있다.
- 변환된 WorkPackage는 pending_approval 상태로 시작한다.
- 변환은 ImplementationRun을 자동 생성하지 않는다.
- 변환 결과에는 DecisionRecord id가 trace/context로 남아야 한다.
```

---

## 13. Event / Trace

### 13.1 Event type

```text
brainstorming_session.created
brainstorming_session.started
brainstorming_session.phase_changed
brainstorming_session.completed
brainstorming_session.failed
brainstorming_session.canceled

brainstorming.context_collected
brainstorming.roles_selected
brainstorming.role_started
brainstorming.role_completed
brainstorming.role_failed
brainstorming.critique_created
brainstorming.option_created
brainstorming.decision_brief_created
brainstorming.validation_passed
brainstorming.validation_failed

decision_record.accepted
decision_record.rejected
decision_record.created

work_package.conversion_requested
work_package.conversion_completed
work_package.conversion_failed

artifact.created
trace.step_recorded
```

### 13.2 Local trace step

```text
- prepare_brainstorming_input
- collect_context
- select_roles
- generate_contributions
- generate_critiques
- synthesize_options
- create_decision_brief
- validate_decision_brief
- accept_decision
- convert_to_work_package
```

---

## 14. 테스트 전략

### 14.1 Backend contract tests

```text
1. topic 기반 BrainstormingSession 생성
2. WorkPackage source 기반 BrainstormingSession 생성
3. invalid source_type/source_id 차단
4. role 기본값 적용
5. role count limit 검증
6. BrainstormingContribution schema validation
7. BrainstormingOption schema validation
8. DecisionBrief schema validation
9. DecisionBrief accept 시 DecisionRecord 생성
10. rejected DecisionBrief는 WorkPackage 변환 차단
11. accepted DecisionRecord에서 WorkPackage 변환 성공
12. 변환된 WorkPackage는 pending_approval 상태
13. event polling/SSE 확인
14. local trace step 확인
15. read-only tool policy 확인
```

### 14.2 GUI e2e smoke

```text
1. Project open
2. Session create
3. Brainstorming tab open
4. topic 입력
5. mode 선택
6. BrainstormingSession 시작
7. role contribution 표시
8. option/tradeoff 표시
9. DecisionBrief 표시
10. DecisionBrief accept
11. DecisionRecord 표시
12. WorkPackage 변환
13. 변환된 WorkPackage pending approval 표시
```

### 14.3 Safety tests

```text
1. Brainstorming 중 file write 요청이 실행되지 않음
2. command execution 요청이 실행되지 않음
3. external network research 요청이 실행되지 않음
4. role 수 제한 초과 차단
5. rejected DecisionBrief 변환 차단
6. GUI가 Agent Backend를 직접 호출하지 않음
```

---

## 15. MVP 4 완료 조건

MVP 4는 다음 조건을 만족하면 완료로 본다.

```text
1. topic 기반 BrainstormingSession을 시작할 수 있다.
2. 기존 WorkPackage 또는 ReviewResult를 source로 BrainstormingSession을 시작할 수 있다.
3. mode와 role selection이 저장된다.
4. Agent Backend가 role별 structured contribution을 반환한다.
5. cross critique가 생성된다.
6. BrainstormingOption 목록이 생성된다.
7. DecisionBrief가 생성되고 GUI에 표시된다.
8. DecisionBrief accept/reject가 가능하다.
9. accept 시 DecisionRecord가 저장된다.
10. rejected DecisionBrief는 WorkPackage 변환이 차단된다.
11. accepted DecisionRecord에서 WorkPackage 후보를 생성할 수 있다.
12. 변환된 WorkPackage는 pending_approval 상태로 기존 approval 흐름에 들어간다.
13. Brainstorming event timeline이 GUI에 표시된다.
14. local trace step이 저장되고 GUI에서 확인된다.
15. MVP 4에서는 user project file write와 command execution이 발생하지 않는다.
16. backend contract tests가 통과한다.
17. GUI build가 통과한다.
18. GUI e2e smoke가 통과한다.
```

---

## 16. MVP 5와의 경계

MVP 4는 DecisionRecord를 만들지만, 장기 Memory 시스템을 만들지는 않는다.

MVP 5 후보:

```text
- Memory / Decision Log view
- ADR 검색
- session summary memory
- project rule memory
- failure memory
- WorkPackage/ImplementationRun history search
- memory 기반 context injection
```

MVP 4에서 만든 DecisionRecord는 MVP 5의 입력 자산이다.

---

## 17. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
Artemis MVP 4를 구현한다.

MVP 1, MVP 2, MVP 3는 완료된 것으로 본다. 기존 Control Plane, Agent Backend, React GUI 구조를 유지한다.

MVP 4의 목표는 Brainstorming Room vertical slice다. 사용자가 topic 또는 기존 WorkPackage/ReviewResult를 source로 BrainstormingSession을 시작하면, Agent Backend가 여러 role의 structured contribution, cross critique, option/tradeoff, DecisionBrief를 생성하고, Control Plane이 이를 저장/중계한다. GUI는 role별 의견, option, DecisionBrief, event timeline, local trace를 표시한다.

DecisionBrief는 사용자가 accept/reject할 수 있어야 한다. accept된 DecisionBrief는 DecisionRecord로 저장된다. accepted DecisionRecord는 사용자가 명시적으로 요청할 때 WorkPackage 후보로 변환할 수 있고, 변환된 WorkPackage는 pending_approval 상태로 기존 approval 흐름에 들어간다.

GUI는 Control Plane만 호출해야 하며 Agent Backend를 직접 호출하면 안 된다.

MVP 4는 user project repository에 파일을 쓰지 않는다. shell command 실행, patch 생성/적용, git commit/push, package install, deployment, external network research는 범위 밖이다.

필수 구현:
- BrainstormingSession / BrainstormingContribution / BrainstormingCritique / BrainstormingOption / DecisionBrief / DecisionRecord 모델
- Control Plane Brainstorming API
- Agent Backend Brainstorming graph 또는 동등한 structured execution path
- role selection과 mode selection
- event polling/SSE
- local trace summary
- GUI Brainstorming Room
- DecisionBrief accept/reject
- accepted DecisionRecord -> pending_approval WorkPackage 변환
- backend contract tests
- GUI e2e smoke script `scripts/smoke_mvp4_gui.py`

완료 전 검증:
- `.venv` compileall
- full unittest
- FastAPI smoke
- GUI build
- npm audit
- MVP 4 GUI e2e smoke
```
