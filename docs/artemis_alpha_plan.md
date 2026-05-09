# Artemis Alpha 0.1 안정화 계획

> MVP 1~6은 기능 축을 검증했다. Alpha 0.1의 목표는 새 기능을 크게 늘리는 것이 아니라, Artemis를 실제로 계속 실행하고 자기 자신을 관리하는 도구로 안정화하는 것이다.

---

## 0. 목적

MVP 1~6은 다음 vertical slice를 완료했다.

```text
MVP 1: Work Package backend foundation
MVP 2: GUI + Event Stream
MVP 3: Implementation Pipeline
MVP 4: Brainstorming Room + Decision Record
MVP 5: Memory / Decision Log
MVP 6: Risk Radar / Quality Center
```

Alpha 0.1은 이 기능들을 제품형 baseline으로 묶는다.

```text
MVP baseline
-> reproducible local run
-> dogfooding workflow
-> LLM structured output path
-> durable state / migration / checkpoint
-> integrated Command Center UX
-> release-ready docs and verification matrix
```

Alpha 0.1은 MVP 7이 아니다.
MVP 7처럼 새로운 큰 기능을 추가하기 전에, 현재 기능이 실제 사용 흐름에서 버티는지 확인하는 안정화 단계다.

---

## 1. Alpha 0.1 한 문장 정의

**Alpha 0.1은 MVP 1~6의 기능을 하나의 사용 가능한 local-first 개발 운영 도구로 묶고, Artemis가 Artemis 자신의 다음 작업을 end-to-end로 관리할 수 있게 만드는 안정화 단계이다.**

---

## 2. 핵심 원칙

### 2.1 새 기능보다 연결 품질

Alpha 0.1의 핵심은 새 화면을 늘리는 것이 아니다.

우선순위:

```text
1. 이미 있는 기능이 끊기지 않고 이어지는가?
2. 사용자가 다음 행동을 알 수 있는가?
3. 실패했을 때 어디서 왜 실패했는지 보이는가?
4. 재시작 후에도 상태를 다시 이해할 수 있는가?
5. 문서대로 실행하면 같은 결과가 나오는가?
```

### 2.2 Artemis로 Artemis를 운영한다

Alpha 0.1 검증은 dogfooding이 중심이다.

Artemis는 자기 자신의 다음 개선 작업을 다음 흐름으로 처리해야 한다.

```text
RiskFinding 또는 사용자 요청
-> Work Package
-> Brainstorming 또는 Decision Record
-> approval
-> ImplementationRun
-> PatchSet / Diff
-> VerificationRun
-> ReviewResult
-> Memory
-> Risk Radar 재스캔
```

### 2.3 deterministic fallback은 개발 보조 경로다

MVP 단계의 deterministic fallback은 기능 경계 검증에는 유용했다.
하지만 Alpha에서는 실제 LLM structured output 경로가 기본 품질 경로가 되어야 한다.

정책:

```text
- fallback은 API key 없음, 테스트, 장애 대응 용도로 남긴다.
- 실제 사용 경로는 GLM Coding Plan 기반 structured output으로 이동한다.
- fallback 결과가 production UX의 품질 기준이 되지 않게 한다.
```

### 2.4 데이터와 상태는 버전 관리된다

MVP 단계에서는 SQLite schema를 빠르게 확장했다.
Alpha에서는 local-first 저장소도 제품 상태로 취급해야 한다.

```text
- schema version
- migration record
- startup compatibility check
- failed migration recovery note
- trace/event/artifact 정합성 점검
```

### 2.5 hidden automation은 여전히 금지한다

Alpha에서도 다음은 금지한다.

```text
- 승인 없는 patch apply
- 승인 없는 implementation run
- hidden memory injection
- hidden external network research
- 자동 git commit/push
- 자동 package install
- 자동 deployment
```

---

## 3. Alpha 0.1 포함 범위

```text
1. MVP baseline tag/release note 준비
2. README / getting-started / configuration 현재 구현 기준 갱신
3. local startup path 정리
4. end-to-end dogfooding scenario 정의와 실행
5. deterministic Work Package fallback -> LLM structured output 경로 전환
6. deterministic MVP 3 patch/log proposal -> LLM structured PatchSet 경로 전환
7. LangGraph checkpointing 실제 연결
8. SQLite schema version / migration layer 추가
9. event / trace / artifact 정합성 점검
10. background task failure recovery UX
11. Command Center 통합 UX 개선
12. pending approvals / risk findings / memory / runs를 한 화면에서 연결
13. smoke script matrix 정리
14. Alpha acceptance test 추가
15. Risk Radar로 Alpha residual risk 추적
```

---

## 4. Alpha 0.1 제외 범위

```text
- full collaboration / multi-user permission
- hosted web deployment
- external CI provider integration
- full dependency graph parser
- full coverage ingestion
- vector DB 기본 도입
- automatic RAG 기본 도입
- autonomous retry / autonomous remediation loop
- one-click release deployment
- plugin marketplace
- cross-project global memory
```

이 항목들은 Alpha 이후 후보로 남긴다.

---

## 5. 작업 스트림

### A0. Baseline Freeze

목표:

```text
MVP 1~6 완료 상태를 재현 가능한 기준점으로 고정한다.
```

작업:

```text
- `v0.1-mvp-baseline` tag 생성 여부 결정
- MVP 1~6 smoke matrix 정리
- 현재 known limitations 문서화
- README의 MVP 1/2 중심 설명을 MVP 1~6 완료 기준으로 갱신
- docs/getting-started.md의 legacy Go TUI 내용을 새 Artemis 기준으로 교체
```

완료 조건:

```text
- 새 작업자가 README와 getting-started만 보고 backend/gui를 실행할 수 있다.
- MVP 1~6 smoke script 위치와 실행 목적이 문서화되어 있다.
- baseline tag 또는 그에 준하는 release note가 존재한다.
```

### A1. Dogfooding Runbook

목표:

```text
Artemis로 Artemis 자신의 개선 작업 하나를 끝까지 처리한다.
```

대표 시나리오:

```text
1. Risk Radar에서 Alpha 관련 finding 생성
2. finding accept
3. finding -> WorkPackage conversion
4. Brainstorming으로 해결 방향 토론
5. DecisionRecord accept
6. WorkPackage approval
7. ImplementationRun 생성
8. PatchSet 검토와 apply
9. VerificationRun 실행
10. ReviewResult 확인
11. Decision / failure / session summary memory 저장
12. Risk Radar 재스캔
```

완료 조건:

```text
- 실제 dogfooding 로그가 남는다.
- 중간에 사용자가 헷갈린 화면/상태가 기록된다.
- dogfooding에서 나온 마찰이 Work Package 후보로 정리된다.
```

### A2. LLM Work Package Output

목표:

```text
WorkPackage 생성의 실제 사용 경로를 GLM structured output으로 전환한다.
```

작업:

```text
- WorkPackageDraft output parser 강화
- role별 GLM routing 유지
- malformed model output recovery
- deterministic fallback은 명시적 fallback reason과 함께 유지
- live model path contract 또는 smoke 추가
- prompt injection 방어 문구와 schema validation 분리
```

완료 조건:

```text
- API key가 있을 때 LLM-generated WorkPackageDraft가 기본 경로다.
- schema validation 실패 시 fallback 또는 failed state가 명확하다.
- trace/event에서 model path와 fallback path가 구분된다.
```

### A3. LLM PatchSet Output

목표:

```text
MVP 3의 deterministic implementation log patch를 실제 structured PatchSet 제안으로 교체한다.
```

작업:

```text
- ImplementationPlanDraft prompt와 parser 설계
- PatchSetDraft schema 강화
- file write 없이 proposal만 생성
- path traversal / delete / binary / oversized patch 차단
- approval 전 apply 불가 정책 유지
- diff viewer에서 model-generated rationale 표시
```

완료 조건:

```text
- Agent Backend가 LLM-generated ImplementationPlan과 PatchSet을 반환한다.
- Control Plane이 policy validation을 통과한 PatchSet만 pending approval로 저장한다.
- fallback deterministic log patch는 테스트/장애용으로만 남는다.
```

### A4. LangGraph Checkpointing

목표:

```text
긴 실행과 재시작 후 상태 복원을 위한 checkpoint 기반을 실제 흐름에 연결한다.
```

작업:

```text
- AgentRun / BrainstormingSession / ImplementationRun / RiskScanRun checkpoint key 설계
- local checkpoint store 추가
- interrupted run 상태 표시
- 재개 가능/불가능 상태 구분
- checkpoint와 trace_id correlation
```

완료 조건:

```text
- 장기 run이 중간 실패해도 마지막 phase와 trace를 확인할 수 있다.
- 재시작 후 terminal/non-terminal run 상태가 모순되지 않는다.
- checkpoint 저장 실패가 run 전체를 조용히 성공 처리하지 않는다.
```

### A5. Storage Migration

목표:

```text
SQLite schema가 Alpha 이후에도 안전하게 진화할 수 있게 한다.
```

작업:

```text
- schema_version table
- migration registry
- idempotent migration runner
- startup migration event
- migration failure message
- test fixture DB upgrade test
```

완료 조건:

```text
- 새 schema가 기존 DB를 깨지 않고 확장된다.
- migration 적용 여부를 API 또는 log에서 확인할 수 있다.
- migration 실패 시 사용자가 DB 상태를 알 수 있다.
```

### A6. Command Center UX

목표:

```text
기능별 탭 모음이 아니라 "현재 다음 행동"을 보여주는 지휘실로 만든다.
```

작업:

```text
- pending approvals summary
- active/recent runs summary
- top risk findings
- selected memory summary
- next recommended action
- WorkPackage / Decision / RiskFinding 간 이동 경로
- stale session 상태 정리
```

완료 조건:

```text
- 사용자가 첫 화면에서 다음에 무엇을 해야 하는지 알 수 있다.
- pending approval, high risk finding, failed verification이 묻히지 않는다.
- MVP 1~6의 주요 산출물이 서로 연결되어 보인다.
```

### A7. Verification Matrix

목표:

```text
현재 smoke script와 contract test를 Alpha 기준 검증 세트로 정리한다.
```

필수 검증:

```text
- .venv compileall
- full unittest
- FastAPI smoke
- GUI build
- npm audit
- MVP 2 GUI smoke
- MVP 3 GUI smoke
- MVP 4 GUI smoke
- MVP 5 GUI smoke
- MVP 6 GUI smoke
- Alpha dogfooding smoke
```

완료 조건:

```text
- 한 명령 또는 한 문서 순서로 전체 Alpha 검증을 재현할 수 있다.
- 실패 시 어떤 기능 축이 깨졌는지 바로 알 수 있다.
- smoke가 너무 느려질 경우 quick/full profile을 분리한다.
```

### A8. Documentation Refresh

목표:

```text
문서가 현재 새 Artemis 구현을 설명하게 한다.
```

우선 갱신 대상:

```text
- README.md
- docs/getting-started.md
- docs/configuration.md
- docs/architecture.md
- .env.example
```

완료 조건:

```text
- legacy Go TUI 안내가 현행 시작 문서의 기본 경로가 아니다.
- GLM Coding Plan, local trace, LangSmith opt-in 정책이 명확하다.
- GUI와 backend 실행법이 현재 script 기준으로 맞다.
```

---

## 6. Alpha Data / Safety Policy

### 6.1 Local-first 유지

```text
- 기본 trace는 local store
- 기본 memory는 local SQLite/FTS
- LangSmith Cloud는 explicit opt-in
- external CI / scanner / vector DB는 기본값 아님
```

### 6.2 승인 게이트 유지

```text
- WorkPackage approval 전 ImplementationRun 금지
- PatchSet approval 전 apply 금지
- RiskFinding conversion은 WorkPackage pending approval까지만 진행
- selected memory는 hidden context가 아니라 명시 입력만 허용
```

### 6.3 모델 출력 신뢰 경계

```text
- LLM output은 항상 untrusted structured candidate
- Control Plane policy validation 통과 전 canonical state로 저장하지 않음
- path, command, external request는 별도 allowlist/policy 확인
```

---

## 7. Alpha 검증 시나리오

### 7.1 Happy Path

```text
1. GUI로 Artemis 프로젝트 열기
2. 새 session 생성
3. 사용자 요청으로 WorkPackage 생성
4. WorkPackage 승인
5. ImplementationRun 생성
6. PatchSet diff 확인
7. PatchSet 승인 및 apply
8. VerificationRun 실행
9. ReviewResult 확인
10. Session summary memory 생성
11. Risk scan 실행
12. finding이 없거나 accepted/mitigated 상태로 정리됨
```

### 7.2 Failure Path

```text
1. malformed LLM output 발생
2. schema validation 실패
3. fallback 또는 failed state 기록
4. event/trace/artifact에서 실패 원인 확인
5. Failure Memory 생성
6. Risk Radar가 해당 실패를 표시
```

### 7.3 Restart Path

```text
1. non-terminal run이 있는 상태에서 서비스 재시작
2. GUI가 stale/interrupted 상태를 표시
3. trace와 events가 남아 있음
4. 사용자가 재시도/정리/새 작업 중 선택 가능
```

---

## 8. Alpha 완료 조건

Alpha 0.1은 다음 조건을 만족하면 완료로 본다.

```text
1. README와 getting-started가 현행 Python/FastAPI/React Artemis 기준이다.
2. MVP 1~6 smoke matrix가 문서화되어 있다.
3. Artemis로 Artemis 개선 작업 하나를 dogfooding으로 end-to-end 처리했다.
4. WorkPackage 생성 기본 경로가 LLM structured output이다.
5. PatchSet 제안 기본 경로가 LLM structured output이다.
6. deterministic fallback은 명시적 fallback reason과 함께만 사용된다.
7. LangGraph checkpointing이 적어도 하나의 장기 run 경로에 연결되어 있다.
8. SQLite schema version/migration 기반이 있다.
9. background task 실패가 GUI와 event/trace에서 보인다.
10. Command Center가 pending approval, risk finding, recent run, selected memory를 요약한다.
11. Alpha verification matrix가 통과한다.
12. known limitations와 post-Alpha 후보가 문서화되어 있다.
```

---

## 9. Post-Alpha 후보

```text
- Deep Architecture Map
- release readiness dashboard
- CI result ingestion
- coverage ingestion
- repeated failure clustering
- memory context recommendation
- optional semantic/vector search evaluation
- collaboration / web sharing
- plugin and MCP permission console
- packaging / updater
```

---

## 10. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
Artemis Alpha 0.1 안정화를 진행한다.

MVP 1~6은 완료된 baseline으로 본다. 새 대형 기능을 추가하기보다 현재 기능을 하나의 실제 사용 가능한 local-first 도구로 묶는 것이 목표다.

우선순위는 다음이다.

1. README / getting-started / configuration을 현재 Python/FastAPI/React 구현 기준으로 갱신한다.
2. MVP 1~6 smoke matrix와 Alpha verification matrix를 정리한다.
3. Artemis로 Artemis 자신의 개선 작업 하나를 end-to-end dogfooding할 수 있는 runbook을 만든다.
4. deterministic WorkPackage fallback을 GLM structured output 기본 경로로 교체한다.
5. deterministic MVP 3 implementation log patch를 LLM-generated structured PatchSet 기본 경로로 교체한다.
6. LangGraph checkpointing과 SQLite schema migration 기반을 추가한다.
7. Command Center에서 pending approvals, risk findings, recent runs, selected memory를 한 화면에서 연결한다.

금지:
- 승인 없는 patch apply
- 승인 없는 implementation run
- hidden memory injection
- 자동 git commit/push
- 자동 package install/deployment
- external CI/scanner/vector DB를 기본값으로 추가

완료 전 검증:
- `.venv` compileall
- full unittest
- FastAPI smoke
- GUI build
- npm audit
- MVP 2~6 GUI smoke
- Alpha dogfooding smoke 또는 문서화된 수동 dogfooding run
```
