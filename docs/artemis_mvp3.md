# Artemis MVP 3 설계 문서

> MVP 3의 목표는 승인된 Work Package를 실제 코드 변경 후보로 변환하고, diff 검토, 승인 후 적용, 검증, 리뷰 결과까지 이어지는 첫 Implementation Pipeline을 만드는 것이다.

---

## 0. 목적

MVP 1은 사용자 요청을 Work Package로 구조화했다.  
MVP 2는 그 흐름을 GUI와 event stream으로 조작 가능하게 만들었다.

MVP 3는 승인된 Work Package를 실제 구현 흐름으로 연결한다.

```text
승인된 Work Package
→ Implementation Plan 생성
→ Patch Proposal 생성
→ Diff 표시
→ 사용자 승인
→ Patch 적용
→ 검증 명령 실행
→ Review Result 생성
→ 결과/이벤트/trace 저장
```

MVP 3는 “완전 자율 구현 Agent”가 아니다.  
MVP 3는 안전한 구현 파이프라인의 최소 vertical slice이다.

---

## 1. 전제

### 1.1 MVP 2 완료 상태

MVP 3는 다음 MVP 2 결과 위에서 시작한다.

```text
- GUI는 Control Plane만 호출한다.
- Control Plane과 Agent Backend는 분리되어 있다.
- Work Package 생성/조회/승인 흐름이 있다.
- AgentRun 이벤트가 GUI timeline에 표시된다.
- local trace/event viewer가 있다.
- Approval approve/reject 조작이 backend state에 반영된다.
- GUI e2e smoke가 통과한다.
```

### 1.2 MVP 3에서 처음 허용되는 위험

MVP 1/2는 read-only였다.  
MVP 3에서는 최초로 프로젝트 파일 변경이 가능해진다.

따라서 MVP 3의 핵심은 코드 생성 능력이 아니라 다음 세 가지이다.

```text
1. Patch를 적용 전 diff로 고정한다.
2. Patch 적용은 별도 사용자 승인 후에만 수행한다.
3. 적용 후 검증 결과와 리뷰 결과를 남긴다.
```

---

## 2. MVP 3 한 문장 정의

**MVP 3는 승인된 Work Package에서 Implementation Plan과 Patch Proposal을 만들고, 사용자가 diff를 승인하면 patch를 적용한 뒤 검증과 리뷰 결과를 저장하는 Implementation Pipeline slice이다.**

---

## 3. 핵심 원칙

### 3.1 Work Package 승인 없이는 구현을 시작하지 않는다

MVP 3의 시작점은 `approved` 상태의 Work Package이다.

```text
pending_approval WorkPackage
→ 구현 시작 불가

approved WorkPackage
→ ImplementationRun 생성 가능
```

### 3.2 Patch 생성과 Patch 적용은 다른 단계이다

Agent Backend는 Patch Proposal을 생성할 수 있다.  
하지만 Control Plane은 이를 즉시 적용하지 않는다.

```text
Patch Proposal 생성
→ Diff artifact 저장
→ 사용자 승인 요청
→ 승인 후 apply
```

### 3.3 Control Plane은 적용 가능한 산출물만 저장한다

Agent Backend의 자유 텍스트를 그대로 실행하지 않는다.  
Patch는 구조화된 `PatchSet`으로 반환되어야 한다.

```text
- file path
- operation
- unified diff 또는 replacement content
- risk
- rationale
```

### 3.4 Tool 실행은 policy를 통과해야 한다

MVP 3에서 새로 허용되는 tool은 제한적이다.

```text
허용:
- read_file
- list_files
- grep
- git_status
- propose_patch
- apply_patch_after_approval
- run_verification_command

금지:
- git commit
- git push
- git reset/rebase
- package install
- DB migration
- deployment
- destructive shell command
- project root 밖 파일 수정
```

### 3.5 검증 없는 완료는 금지한다

Patch 적용 후에는 반드시 VerificationRun이 생성되어야 한다.  
검증 명령을 실행할 수 없으면 `not_run` 상태와 이유를 남긴다.

---

## 4. MVP 3 범위

### 4.1 포함 범위

```text
1. approved WorkPackage에서 ImplementationRun 생성
2. Implementation Plan schema 추가
3. Patch Proposal schema 추가
4. PatchSet/PatchFile domain model 추가
5. Patch diff artifact 저장
6. GUI Diff Viewer 최소 화면
7. Patch apply 승인 요청
8. 승인 후 patch 적용
9. VerificationRun 생성
10. 제한된 검증 명령 실행
11. ReviewResult 생성
12. ImplementationRun event stream 표시
13. local trace에 implementation steps 기록
14. backend contract tests 추가
15. GUI e2e smoke 추가
```

### 4.2 제외 범위

```text
- git commit
- git push
- PR 생성
- 자동 package install
- DB migration 실행
- 배포
- 대량 refactor 자동 실행
- 여러 Work Package 병렬 구현
- autonomous retry loop
- Brainstorming Room
- Memory/RAG/Vector DB
- Risk Radar
- Architecture Map
- 멀티 사용자 권한
```

---

## 5. 시스템 구조

```text
React GUI
  ↓ HTTP / SSE
Control Plane
  ↓ internal HTTP
Agent Backend
  ↓ Tool Router / Patch Engine
User Project Repository
```

MVP 3에서도 GUI는 Agent Backend를 직접 호출하지 않는다.

---

## 6. Implementation Pipeline

### 6.1 전체 흐름

```text
START
  ↓
load_approved_work_package
  ↓
collect_implementation_context
  ↓
create_implementation_plan
  ↓
generate_patch_proposal
  ↓
validate_patch_policy
  ↓
store_patch_set
  ↓
request_patch_approval
  ↓
WAIT_FOR_USER
  ↓
apply_patch
  ↓
run_verification
  ↓
review_result
  ↓
final_report
  ↓
END
```

### 6.2 Agent Backend 책임

```text
- Work Package 해석
- 구현 컨텍스트 수집
- Implementation Plan 생성
- Patch Proposal 생성
- Review Result 생성
- patch risk hint 생성
- 검증 결과 해석
```

### 6.3 Control Plane 책임

```text
- ImplementationRun 상태 관리
- PatchSet canonical state 저장
- Diff artifact 저장
- Patch approval 관리
- Patch apply 실행
- VerificationRun 저장
- ReviewResult 저장
- GUI용 상태 정규화
- event/trace 기록
```

---

## 7. 도메인 모델

### 7.1 ImplementationRun

```text
ImplementationRun
- id
- project_id
- session_id
- work_package_id
- status
- current_phase
- trace_id
- created_at
- updated_at
```

Status:

```text
- queued
- planning
- patch_proposed
- pending_patch_approval
- applying
- verifying
- reviewing
- completed
- failed
- canceled
```

### 7.2 ImplementationPlan

```text
ImplementationPlan
- id
- implementation_run_id
- goal
- context_summary
- target_files
- steps
- verification_strategy
- risks
- created_at
```

### 7.3 PatchSet

```text
PatchSet
- id
- implementation_run_id
- status
- summary
- risk_level
- files
- approval_status
- created_at
- updated_at
```

PatchSet status:

```text
- proposed
- pending_approval
- approved
- applied
- rejected
- failed
```

### 7.4 PatchFile

```text
PatchFile
- id
- patch_set_id
- path
- operation
- diff
- rationale
- risk_level
```

Operation:

```text
- create
- update
- delete
```

MVP 3에서는 `delete`를 기본 금지한다.  
삭제가 필요한 경우 PatchSet은 생성할 수 있지만 apply는 차단하고 사용자에게 범위 재검토를 요구한다.

### 7.5 VerificationRun

```text
VerificationRun
- id
- implementation_run_id
- command
- status
- exit_code
- stdout
- stderr
- started_at
- ended_at
```

Status:

```text
- not_run
- running
- passed
- failed
- blocked
```

### 7.6 ReviewResult

```text
ReviewResult
- id
- implementation_run_id
- status
- findings
- residual_risks
- recommendation
- created_at
```

Status:

```text
- pass
- needs_changes
- blocked
```

---

## 8. API 설계

### 8.1 ImplementationRun

```http
POST /api/implementation-runs
GET  /api/implementation-runs/{implementation_run_id}
GET  /api/implementation-runs/{implementation_run_id}/events
GET  /api/implementation-runs/{implementation_run_id}/events/stream
GET  /api/implementation-runs/{implementation_run_id}/result
POST /api/implementation-runs/{implementation_run_id}/cancel
```

생성 요청:

```json
{
  "work_package_id": "wp_001"
}
```

### 8.2 PatchSet

```http
GET  /api/patch-sets/{patch_set_id}
GET  /api/implementation-runs/{implementation_run_id}/patch-set
POST /api/patch-sets/{patch_set_id}/approve
POST /api/patch-sets/{patch_set_id}/reject
POST /api/patch-sets/{patch_set_id}/apply
```

원칙:

```text
- approve는 scope 승인이다.
- apply는 실제 파일 변경이다.
- apply는 approved PatchSet에만 가능하다.
```

### 8.3 Verification

```http
POST /api/implementation-runs/{implementation_run_id}/verification-runs
GET  /api/implementation-runs/{implementation_run_id}/verification-runs
```

명령은 allowlist 기반으로 제한한다.

예시:

```text
python -m unittest discover -s tests
npm run build
npm run test
npm audit --omit=dev
```

### 8.4 Review

```http
GET /api/implementation-runs/{implementation_run_id}/review-result
```

---

## 9. GUI 보강

MVP 3 GUI는 MVP 2 Project Command Center에 Implementation Panel을 추가한다.

### 9.1 필수 화면

```text
1. Approved Work Package 선택
2. ImplementationRun 시작
3. Implementation Plan 표시
4. Patch Diff Viewer
5. Patch 승인/거부
6. Patch 적용 버튼
7. Verification 결과 표시
8. Review Result 표시
9. Implementation event timeline
10. local trace steps 표시
```

### 9.2 Diff Viewer 최소 요구사항

```text
- 파일별 diff 표시
- create/update/delete operation 표시
- risk level 표시
- rationale 표시
- 승인 전 apply 버튼 비활성화
- delete operation은 MVP 3에서 apply 차단 표시
```

### 9.3 Approval UX

MVP 3에는 승인 단계가 두 개 있다.

```text
1. Work Package approval
   - 작업 범위 승인

2. Patch approval
   - 실제 파일 변경 승인
```

GUI는 두 승인을 명확히 구분해야 한다.

---

## 10. Tool / Safety Policy

### 10.1 Patch 적용 정책

```text
- project root 밖 path 금지
- symlink escape 금지
- delete operation apply 금지
- binary file patch 금지
- patch 적용 전 clean/dirty 상태 기록
- patch 적용 후 변경 파일 목록 저장
- apply 실패 시 partial apply 방지
```

MVP 3에서는 가능하면 unified diff를 직접 적용하기보다 안전한 patch engine을 둔다.

권장:

```text
1. PatchSet을 임시 workspace에서 dry-run
2. path policy 검증
3. approval 확인
4. atomic write 또는 rollback 가능한 방식으로 적용
```

### 10.2 검증 명령 정책

검증 명령은 allowlist 기반으로만 실행한다.

```text
허용 예:
- python -m unittest discover -s tests
- pytest
- npm run build
- npm run test
- npm audit --omit=dev

차단 예:
- rm
- del
- git reset
- git clean
- pip install
- npm install
- curl | sh
- Invoke-WebRequest | iex
```

### 10.3 Git 정책

MVP 3에서 Git은 상태 확인까지만 기본 허용한다.

```text
허용:
- git status
- git diff

금지:
- git add
- git commit
- git push
- git reset
- git clean
- git rebase
```

---

## 11. Trace / Event

### 11.1 Event Type

```text
implementation_run.created
implementation_run.started
implementation_run.phase_changed
implementation_run.completed
implementation_run.failed
implementation_run.canceled

implementation_plan.created

patch_set.proposed
patch_set.validation_passed
patch_set.validation_failed
patch_set.pending_approval
patch_set.approved
patch_set.rejected
patch_set.apply_started
patch_set.applied
patch_set.apply_failed

verification.started
verification.completed
verification.failed
verification.blocked

review.started
review.completed

artifact.created
trace.step_recorded
```

### 11.2 Local Trace

MVP 3는 local trace를 implementation pipeline까지 확장한다.

Trace step 예:

```text
- load_approved_work_package
- collect_implementation_context
- create_implementation_plan
- generate_patch_proposal
- validate_patch_policy
- apply_patch
- run_verification
- review_result
```

---

## 12. 테스트 전략

### 12.1 Backend contract tests

```text
1. approved WorkPackage에서 ImplementationRun 생성 가능
2. pending WorkPackage에서 ImplementationRun 생성 차단
3. PatchSet schema validation
4. path escape patch 차단
5. delete operation apply 차단
6. Patch approval 전 apply 차단
7. Patch approval 후 apply 성공
8. VerificationRun allowlist command 실행
9. blocked command 차단
10. ReviewResult 저장
11. event polling/SSE 확인
12. local trace step 확인
```

### 12.2 GUI e2e smoke

```text
1. WorkPackage 생성
2. WorkPackage 승인
3. ImplementationRun 시작
4. Patch proposal 표시
5. Diff Viewer 표시
6. Patch 승인
7. Patch 적용
8. Verification 결과 표시
9. Review Result 표시
```

### 12.3 Safety tests

```text
1. ../ path patch 차단
2. absolute path patch 차단
3. project root 밖 파일 수정 차단
4. delete operation 차단
5. destructive command 차단
6. unapproved patch apply 차단
```

---

## 13. MVP 3 완료 조건

MVP 3는 다음 조건을 만족하면 완료로 본다.

```text
1. approved WorkPackage에서 ImplementationRun을 시작할 수 있다.
2. pending/rejected WorkPackage에서는 ImplementationRun이 차단된다.
3. Implementation Plan이 생성되고 GUI에 표시된다.
4. Patch Proposal이 구조화된 PatchSet으로 저장된다.
5. GUI에서 파일별 diff를 볼 수 있다.
6. Patch approval 전에는 apply가 불가능하다.
7. Patch approval 후 apply가 가능하다.
8. Patch apply 후 변경 파일 목록이 저장된다.
9. VerificationRun이 생성되고 결과가 저장된다.
10. ReviewResult가 생성되고 GUI에 표시된다.
11. implementation event timeline이 GUI에 표시된다.
12. local trace step이 저장되고 GUI에서 확인된다.
13. path escape, delete, destructive command가 차단된다.
14. backend contract tests가 통과한다.
15. GUI build가 통과한다.
16. GUI e2e smoke가 통과한다.
```

---

## 14. MVP 4와의 경계

MVP 3는 한 Work Package의 단일 구현 흐름만 다룬다.

MVP 4 후보:

```text
- Brainstorming Room
- multi-agent architecture review
- patch retry loop
- richer review comments
- Memory / Decision Log
```

MVP 3에서는 자동 재시도 loop를 넣지 않는다.  
실패하면 실패 원인과 다음 행동을 남기고 멈춘다.

---

## 15. MVP 3 시작 전 Cleanup

MVP 2 검증에서 확인된 보완사항은 MVP 3 구현 전에 먼저 정리한다.

```text
1. project 변경 시 GUI currentSession 재설정
2. GUI e2e에 reject approval path 추가
3. CORS allow_origins="*"를 local/dev policy로 명시하거나 설정화
```

이 cleanup은 MVP 3 본 구현의 일부로 처리해도 된다.

---

## 16. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
새 Artemis MVP 3를 구현한다.

MVP 1과 MVP 2는 완료된 것으로 본다. 기존 Control Plane, Agent Backend, React GUI 구조를 유지한다.

MVP 3의 목표는 approved WorkPackage에서 ImplementationRun을 시작하고, Implementation Plan, Patch Proposal, Diff Viewer, Patch approval, approved patch apply, VerificationRun, ReviewResult까지 이어지는 첫 Implementation Pipeline vertical slice를 구현하는 것이다.

GUI는 Control Plane만 호출해야 하며 Agent Backend를 직접 호출하면 안 된다.

MVP 3에서 처음으로 파일 쓰기가 허용되지만, 반드시 PatchSet 저장 → diff 표시 → patch approval → apply 순서를 거쳐야 한다.

MVP 3는 git commit/push, package install, DB migration, deployment, autonomous retry loop, Brainstorming Room, Memory/RAG UI를 포함하지 않는다.

구현 전 MVP 2 cleanup:
- project 변경 시 currentSession 재설정
- GUI e2e reject approval path 추가
- CORS local/dev policy 설정화

핵심 원칙:
- Work Package 승인 없이는 구현을 시작하지 않는다.
- Patch 생성과 Patch 적용은 별도 단계다.
- Patch approval 전 apply는 불가능해야 한다.
- 검증 없는 완료는 금지한다.
- 모든 file write와 command execution은 policy를 통과해야 한다.
- Observability는 local-first이다.
```
