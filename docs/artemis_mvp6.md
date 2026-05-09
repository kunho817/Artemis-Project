# Artemis MVP 6 설계 문서

> MVP 6의 목표는 MVP 5에서 축적된 Memory와 기존 실행 결과를 바탕으로 프로젝트의 위험, 품질, 반복 실패, 구조적 취약점을 한눈에 볼 수 있는 Risk Radar / Quality Center vertical slice를 만드는 것이다.

---

## 0. 목적

MVP 1은 자연어 요청을 Work Package로 구조화했다.
MVP 2는 그 흐름을 GUI와 event stream으로 조작 가능하게 만들었다.
MVP 3는 승인된 Work Package를 구현 파이프라인, PatchSet, VerificationRun, ReviewResult로 연결했다.
MVP 4는 구현 전 토론과 결정을 Brainstorming Room과 DecisionRecord로 구조화했다.
MVP 5는 DecisionRecord, Project Rule, Session Summary, Failure Memory를 local-first Project Memory로 저장하고 검색할 수 있게 했다.

MVP 6는 이 축적된 상태를 "현재 프로젝트가 어디가 위험한가"라는 운영 관점으로 재구성한다.

```text
Project state / Memory / WorkPackage / ImplementationRun / VerificationRun / ReviewResult
-> Risk scan request
-> read-only source collection
-> RiskFinding / QualitySignal / ArchitectureMapSnapshot 후보 생성
-> Control Plane canonical 저장
-> Risk Radar / Quality Center GUI 표시
-> 사용자가 finding을 accept/dismiss/convert-to-WorkPackage 처리
```

MVP 6는 정적 분석 플랫폼 전체를 만드는 단계가 아니다.
MVP 6는 Artemis가 쌓아온 산출물을 읽고 프로젝트 운영 리스크를 구조화해 보여주는 첫 Project Health slice다.

---

## 1. 전제

### 1.1 MVP 5 완료 상태

MVP 6는 다음 MVP 5 결과 위에서 시작한다.

```text
- ProjectMemoryItem / MemorySourceLink / MemoryExtractionRun / MemoryCandidate가 존재한다.
- accepted DecisionRecord를 decision memory로 승격할 수 있다.
- manual ProjectRule을 만들고 archive/restore할 수 있다.
- ReviewResult / VerificationRun failure를 Failure Memory로 승격할 수 있다.
- local SQLite/FTS search로 memory를 조회할 수 있다.
- GUI Memory View에서 decision/rule/failure/session memory를 볼 수 있다.
- selected memory context를 명시적으로 추가/제거할 수 있다.
- GUI는 Control Plane만 호출한다.
```

### 1.2 MVP 6에서 처음 다루는 문제

MVP 5까지는 각 산출물이 개별 화면에 흩어져 있다.

MVP 6는 다음 질문을 다룬다.

```text
- 현재 프로젝트의 가장 큰 위험은 무엇인가?
- 반복되는 실패 패턴은 무엇인가?
- 검증이 부족한 작업이나 영역은 어디인가?
- 최근 구현/리뷰 결과는 프로젝트 품질을 개선했는가?
- 어떤 Project Rule 또는 Decision이 현재 위험 판단의 근거인가?
- 지금 당장 Work Package로 전환해야 할 품질 개선 항목은 무엇인가?
```

### 1.3 Selected Memory 결정

MVP 5 재검증에서 남은 설계 질문은 selected memory를 다음 요청에 어떻게 연결할지였다.

MVP 6에서 이 경계는 다음처럼 확정한다.

```text
허용:
- RiskScanRun 생성 시 사용자가 선택한 memory snapshot을 명시적 source_context로 첨부한다.
- GUI는 현재 session의 selected memory를 scan 입력으로 포함할지 사용자가 볼 수 있게 표시한다.
- WorkPackage 또는 Brainstorming에 memory를 붙이는 기능은 명시적 payload 필드가 있을 때만 허용한다.

금지:
- search 결과를 자동으로 prompt에 넣기
- hidden memory injection
- 사용자가 선택하지 않은 memory를 analysis input으로 자동 확장
- selected memory 때문에 작업 실행을 자동 시작
```

따라서 MVP 6의 기본 방향은 "manual selected context"다.
RAG나 자동 추천 주입이 아니라, 사용자가 선택한 근거를 Risk/Quality 분석에 명시적으로 연결한다.

---

## 2. MVP 6 한 문장 정의

**MVP 6는 Project Memory, Work Package, ImplementationRun, VerificationRun, ReviewResult, read-only repository signals를 모아 RiskFinding, QualitySignal, ArchitectureMapSnapshot으로 저장하고 GUI에서 Risk Radar / Quality Center로 관리하는 Project Health slice이다.**

---

## 3. 핵심 원칙

### 3.1 Risk와 Quality는 제품 상태다

Risk/Quality 분석 결과는 일회성 prose가 아니다.

MVP 6에서 분석 결과는 Control Plane이 저장하는 canonical product state가 된다.

```text
- RiskScanRun
- RiskFinding
- QualitySignal
- ArchitectureMapSnapshot
- ProjectHealthSnapshot
```

### 3.2 Control Plane은 상태와 정책을 소유한다

Control Plane 책임:

```text
- scan 요청 접수
- source context bundle 구성
- selected memory 포함 여부 관리
- RiskScanRun 상태 저장
- RiskFinding / QualitySignal canonical 저장
- finding status 관리
- finding -> WorkPackage conversion 관리
- event/trace 저장
- GUI용 aggregation 제공
```

Control Plane이 하지 않는 일:

```text
- LLM risk reasoning 직접 수행
- model prompt 안에서 임의 memory 확장
- finding의 근거 없이 위험을 생성
- 사용자 승인 없이 file write, command execution, patch apply 실행
```

### 3.3 Agent Backend는 분석 후보만 만든다

Agent Backend는 Control Plane이 넘긴 source snapshot과 read-only repository signals를 받아 구조화된 후보를 만든다.

Agent Backend가 하지 않는 일:

```text
- RiskFinding canonical 저장
- finding status 변경
- WorkPackage 직접 생성
- selected memory를 임의로 검색하거나 확장
- shell command 실행
- user project file write
```

### 3.4 모든 finding은 source-linked여야 한다

RiskFinding과 QualitySignal은 반드시 근거를 가져야 한다.

source 예:

```text
- work_package
- implementation_run
- patch_set
- verification_run
- review_result
- decision_record
- memory_item
- session
- repository_file
- repository_metric
```

근거가 없는 추론은 `analysis_note`로만 남기고 RiskFinding으로 저장하지 않는다.

### 3.5 MVP 6는 read-only analysis다

MVP 6는 프로젝트 파일을 수정하지 않는다.

허용:

```text
- repository file list 수집
- read-only file metadata 수집
- read-only grep
- stored VerificationRun / ReviewResult 분석
- stored Memory 분석
- finding -> pending approval WorkPackage 후보 생성
```

금지:

```text
- patch 생성
- patch 적용
- 테스트 실행
- lint 실행
- package install
- migration 실행
- git commit/push
- deployment
```

테스트 결과는 새로 실행해서 얻는 것이 아니라 기존 VerificationRun과 사용자가 저장한 signal을 읽어서 표시한다.

### 3.6 점수는 힌트다

Risk score와 quality score는 자동 우선순위 결정자가 아니다.

```text
- score는 정렬과 시각화 보조 값이다.
- high/critical finding도 사용자가 accept해야 WorkPackage로 전환된다.
- dismissed finding은 기본 Radar에서 숨기지만 기록은 남긴다.
- score 산식과 근거는 GUI에서 확인 가능해야 한다.
```

---

## 4. MVP 6 범위

### 4.1 포함 범위

```text
1. RiskScanRun domain model 추가
2. RiskFinding domain model 추가
3. QualitySignal domain model 추가
4. ProjectHealthSnapshot domain model 추가
5. ArchitectureMapSnapshot lite model 추가
6. selected memory snapshot을 RiskScan source_context에 명시적으로 첨부
7. WorkPackage / ImplementationRun / VerificationRun / ReviewResult source 수집
8. Failure Memory / Project Rule / Decision memory source 수집
9. read-only repository metric 수집
10. Agent Backend risk/quality analysis candidate 생성
11. Control Plane Risk Radar API
12. Control Plane Quality Snapshot API
13. Control Plane Architecture Map lite API
14. finding status update API
15. finding -> pending approval WorkPackage conversion
16. GUI Risk Radar view
17. GUI Quality Center view
18. GUI finding detail/evidence/source link 표시
19. GUI verification history summary
20. GUI repeated failure / hotspot summary
21. risk/quality event와 local trace 저장
22. backend contract tests 추가
23. GUI e2e smoke 추가
```

### 4.2 제외 범위

```text
- full static analyzer
- full dependency graph engine
- full code coverage integration
- CI provider integration
- external security scanner
- SAST/DAST
- vector DB / embedding 기반 semantic risk search
- automatic RAG
- automatic WorkPackage 생성
- autonomous remediation loop
- patch 생성 또는 적용
- shell command 실행
- 테스트 실행
- git commit/push
- cross-project global quality dashboard
- multi-user permission workflow
```

---

## 5. 시스템 구조

```text
React GUI
  -> HTTP / SSE
Control Plane
  -> Project state / Memory Store / Risk Store / Trace Store
  -> optional internal HTTP
Agent Backend
  -> risk/quality analysis candidate generation
  -> read-only tools
User Project Repository
```

MVP 6에서도 GUI는 Agent Backend를 직접 호출하지 않는다.

---

## 6. Risk / Quality Pipeline

### 6.1 Risk scan 생성

```text
User
  -> Start Risk Scan
  -> scope 선택
  -> selected memory 포함 여부 확인
  -> Control Plane RiskScanRun 생성
  -> Agent Backend analysis 요청
```

scope:

```text
- project
- current_session
- work_package
- implementation_run
- review_result
- memory_focus
```

### 6.2 Source context bundle

Control Plane은 scan scope에 맞춰 source context bundle을 만든다.

```text
ProjectContextBundle
- project
- session
- selected_memory_snapshots
- recent_work_packages
- implementation_runs
- verification_runs
- review_results
- decision_records
- failure_memories
- project_rules
- repository_metrics
- repository_file_samples
```

MVP 6에서 bundle은 저장 가능한 snapshot이어야 한다.
나중에 같은 scan을 다시 열었을 때 당시 근거가 재현되어야 한다.

### 6.3 Agent Backend analysis

```text
ProjectContextBundle
  -> collect_project_signals
  -> classify_risk_findings
  -> summarize_quality_signals
  -> build_architecture_map_lite
  -> rank_findings
  -> validate_analysis
  -> return RiskAnalysisCandidate
```

Agent Backend 결과는 raw prose가 아니라 schema다.

### 6.4 Control Plane 저장

```text
RiskAnalysisCandidate
  -> validate source links
  -> RiskFinding 저장
  -> QualitySignal 저장
  -> ArchitectureMapSnapshot 저장
  -> ProjectHealthSnapshot 저장
  -> local trace summary 저장
```

### 6.5 Finding 처리

사용자는 finding을 다음 상태로 바꿀 수 있다.

```text
- open
- accepted
- dismissed
- mitigated
- converted
```

`accepted`는 "문제로 인정함"이다.
`converted`는 pending approval WorkPackage가 생성되었음을 뜻한다.

### 6.6 Finding -> WorkPackage 전환

```text
RiskFinding
  -> convert_to_work_package 요청
  -> WorkPackageDraft 생성
  -> Control Plane canonical WorkPackage 저장
  -> Approval pending
```

전환된 WorkPackage는 기존 MVP 1/2/3 승인 정책을 따른다.
즉시 구현하지 않는다.

---

## 7. Risk 분류

### 7.1 architecture

구조와 경계의 위험.

예:

```text
- service boundary가 흐림
- GUI가 내부 backend를 직접 호출할 위험
- 한 파일에 과도한 책임 집중
- 의존 방향이 애매함
- 향후 분리해야 할 모듈이 커지고 있음
```

### 7.2 implementation

구현 품질과 유지보수 위험.

예:

```text
- 큰 파일 또는 큰 함수
- 반복 로직
- 테스트 없는 핵심 흐름
- 검증 없이 남은 residual risk
- fallback/deterministic path가 production path로 굳어질 위험
```

### 7.3 verification

검증 상태와 테스트 신뢰도 위험.

예:

```text
- 최근 verification 실패 반복
- smoke는 통과하지만 contract coverage가 부족함
- GUI e2e가 핵심 branch를 덮지 않음
- 허용된 command가 없어 verification이 blocked됨
```

### 7.4 schedule

방치되거나 반복해서 미뤄진 작업.

예:

```text
- Pending 항목이 여러 MVP에 걸쳐 남아 있음
- 같은 follow-up이 여러 DecisionRecord에 반복됨
- converted WorkPackage가 승인 대기 상태로 오래 남음
```

### 7.5 product

제품 방향과 범위 위험.

예:

```text
- MVP 범위가 너무 넓음
- 사용자 가치와 직접 연결되지 않는 기능이 선행됨
- planning artifact는 있는데 실행 경로가 없음
```

### 7.6 security

보안과 민감 정보 위험.

예:

```text
- memory body에 secret-like 문자열 저장 시도
- external endpoint나 cloud dependency가 기본값으로 들어갈 위험
- 위험 command를 WorkPackage나 Memory가 요구함
```

### 7.7 process

Artemis 운영 프로세스의 위험.

예:

```text
- 승인 전 실행 경로가 열림
- Agent Backend가 canonical state를 쓰려 함
- event/trace 없는 결과물이 생김
- selected memory가 hidden context로 사용됨
```

---

## 8. Quality Signal

QualitySignal은 점수 그 자체가 아니라 관찰된 품질 신호다.

### 8.1 verification_signal

```text
- 최근 verification run status
- 실패 command
- blocked reason
- review residual risks
```

### 8.2 test_coverage_hint

MVP 6는 coverage tool을 실행하지 않는다.

대신 read-only heuristic만 사용한다.

```text
- tests directory 존재 여부
- contract/e2e smoke 파일 존재 여부
- 최근 변경된 기능과 test artifact 연결 여부
- VerificationRun이 어떤 command를 기록했는지
```

### 8.3 code_size_signal

```text
- 큰 파일
- 특정 extension별 파일 수
- implementation 집중도
- docs/test/source 비율
```

### 8.4 memory_signal

```text
- 반복 failure memory
- high importance project rule
- 최근 decision과 충돌할 수 있는 finding
- archived/superseded memory와의 관계
```

### 8.5 process_signal

```text
- pending approvals
- rejected approvals
- 오래된 open findings
- WorkPackage conversion 후 미진행 상태
```

---

## 9. 도메인 모델

### 9.1 RiskScanRun

```text
RiskScanRun
- id
- project_id
- session_id
- scope_type
- scope_id
- status
- current_phase
- selected_memory_count
- trace_id
- created_at
- updated_at
```

status:

```text
- queued
- collecting
- analyzing
- completed
- failed
- canceled
```

scope_type:

```text
- project
- session
- work_package
- implementation_run
- review_result
- memory_focus
```

### 9.2 RiskFinding

```text
RiskFinding
- id
- project_id
- risk_scan_run_id
- category
- severity
- title
- summary
- evidence
- recommendation
- confidence
- status
- source_links
- converted_work_package_id
- created_at
- updated_at
```

category:

```text
- architecture
- implementation
- verification
- schedule
- product
- security
- process
```

severity:

```text
- info
- low
- medium
- high
- critical
```

status:

```text
- open
- accepted
- dismissed
- mitigated
- converted
```

### 9.3 QualitySignal

```text
QualitySignal
- id
- project_id
- risk_scan_run_id
- kind
- status
- title
- summary
- value
- target
- evidence
- source_links
- created_at
```

kind:

```text
- verification
- coverage_hint
- code_size
- memory
- process
- architecture
```

status:

```text
- healthy
- watch
- at_risk
- unknown
```

### 9.4 ProjectHealthSnapshot

```text
ProjectHealthSnapshot
- id
- project_id
- risk_scan_run_id
- overall_status
- overall_score
- risk_counts
- top_findings
- quality_summary
- recommendation
- created_at
```

overall_status:

```text
- healthy
- watch
- at_risk
- blocked
- unknown
```

### 9.5 ArchitectureMapSnapshot lite

```text
ArchitectureMapSnapshot
- id
- project_id
- risk_scan_run_id
- nodes
- edges
- hotspots
- boundary_notes
- created_at
```

MVP 6의 Architecture Map은 "lite"다.
완전한 언어별 AST/그래프 분석은 하지 않는다.

nodes 예:

```text
- apps/gui
- services/control_plane
- services/agent_backend
- tests
- scripts
- docs
```

edges 예:

```text
- GUI -> Control Plane API
- Control Plane -> Agent Backend internal API
- Control Plane -> SQLite store
- Agent Backend -> read-only tools
```

### 9.6 SourceLink

RiskFinding과 QualitySignal은 MemorySourceLink와 유사한 source link를 갖는다.

```text
AnalysisSourceLink
- id
- owner_type
- owner_id
- source_type
- source_id
- relation
- label
- created_at
```

source_type:

```text
- memory_item
- work_package
- implementation_run
- patch_set
- verification_run
- review_result
- decision_record
- repository_file
- repository_metric
- session
```

---

## 10. API 설계

### 10.1 Risk Scan

```http
POST /api/projects/{project_id}/risk-scans
GET  /api/projects/{project_id}/risk-scans
GET  /api/risk-scans/{risk_scan_run_id}
GET  /api/risk-scans/{risk_scan_run_id}/events
GET  /api/risk-scans/{risk_scan_run_id}/events/stream
GET  /api/risk-scans/{risk_scan_run_id}/trace
```

scan 요청 예:

```json
{
  "session_id": "sess_001",
  "scope_type": "project",
  "scope_id": null,
  "include_selected_memory": true,
  "selected_memory_ids": ["mem_001", "mem_002"],
  "focus": ["verification", "architecture", "process"]
}
```

정책:

```text
- include_selected_memory가 true여도 current session에서 명시적으로 선택된 memory만 포함한다.
- selected_memory_ids가 있으면 해당 item이 active인지 확인한다.
- archived/superseded memory는 scan input으로 사용할 수 없다.
- source_context snapshot은 RiskScanRun과 trace에서 재현 가능해야 한다.
```

### 10.2 Risk Radar

```http
GET /api/projects/{project_id}/risk-radar
GET /api/projects/{project_id}/risk-findings
GET /api/risk-findings/{risk_finding_id}
PATCH /api/risk-findings/{risk_finding_id}
POST /api/risk-findings/{risk_finding_id}/convert-to-work-package
```

filters:

```text
category
severity
status
source_type
source_id
limit
```

### 10.3 Quality Center

```http
GET /api/projects/{project_id}/quality-snapshot
GET /api/projects/{project_id}/quality-signals
GET /api/quality-signals/{quality_signal_id}
```

### 10.4 Architecture Map

```http
GET /api/projects/{project_id}/architecture-map
GET /api/risk-scans/{risk_scan_run_id}/architecture-map
```

MVP 6에서는 최신 snapshot을 기본 반환한다.

### 10.5 Finding conversion

finding을 WorkPackage로 전환할 때는 기존 WorkPackage approval flow를 그대로 사용한다.

```json
{
  "title": "Reduce repeated verification failure risk",
  "goal": "Create a focused follow-up for the accepted risk finding.",
  "source_risk_finding_id": "risk_001"
}
```

결과:

```text
- WorkPackage status: pending_approval
- Approval status: pending
- RiskFinding status: converted
- converted_work_package_id 기록
```

---

## 11. GUI 설계

MVP 6 GUI는 기존 Project Command Center에 Risk / Quality 화면을 추가한다.

### 11.1 필수 화면

```text
1. Risk Radar tab
2. Quality Center tab
3. Start Risk Scan action
4. scan scope selector
5. selected memory inclusion toggle
6. Project Health summary
7. risk category distribution
8. severity list
9. finding detail panel
10. evidence/source links
11. finding accept/dismiss/mitigate controls
12. finding -> WorkPackage conversion
13. verification history summary
14. repeated failure summary
15. architecture map lite
16. scan event timeline
17. trace viewer link
```

### 11.2 Risk Radar 구조

```text
Risk Radar
  - Health Summary
  - Severity Distribution
  - Category Distribution
  - Top Findings
  - Open / Accepted / Dismissed filters
  - Finding Detail
```

### 11.3 Quality Center 구조

```text
Quality Center
  - Verification History
  - Review Residual Risks
  - Test Coverage Hints
  - Code Size Signals
  - Process Signals
  - Memory Signals
```

### 11.4 Architecture Map lite 구조

```text
Architecture Map
  - module nodes
  - known service edges
  - hotspots
  - boundary notes
```

### 11.5 UX 원칙

```text
- finding은 반드시 근거와 source link가 먼저 보여야 한다.
- score만 보여주지 않고 이유와 recommended action을 같이 보여준다.
- dismissed finding은 사라지는 것이 아니라 status filter 뒤에 남긴다.
- high/critical finding도 자동 구현되지 않는다.
- selected memory가 scan에 포함되면 어떤 memory가 포함되었는지 보여준다.
- verification history는 새 테스트 실행 결과처럼 오해되지 않게 "recorded runs"로 표시한다.
```

---

## 12. Agent Backend / Risk Analysis

### 12.1 Risk analysis graph

```text
START
  -> load_project_context_bundle
  -> collect_repository_signals
  -> collect_memory_signals
  -> collect_execution_signals
  -> draft_risk_findings
  -> draft_quality_signals
  -> draft_architecture_map_lite
  -> rank_and_dedupe_findings
  -> validate_analysis
  -> emit_result
  -> END
```

### 12.2 Node 책임

#### load_project_context_bundle

```text
- Control Plane이 전달한 bundle을 검증한다.
- selected memory snapshot이 있으면 source link를 보존한다.
- scope_type/scope_id를 검증한다.
```

#### collect_repository_signals

read-only tools만 사용한다.

```text
- list_files
- grep
- read_file
- git_status
```

수집 예:

```text
- 큰 파일 후보
- tests/scripts/docs 존재 여부
- dependency manifest 존재 여부
- TODO/FIXME 패턴
- service boundary 관련 파일
```

#### collect_memory_signals

```text
- Failure Memory 반복 패턴
- Project Rule과 현재 구조의 충돌 가능성
- Decision Memory의 follow-up action
- Session Summary의 pending action
```

#### collect_execution_signals

```text
- VerificationRun status
- ReviewResult residual_risks
- PatchSet risk_level
- pending Approval
- ImplementationRun status
```

#### draft_risk_findings

RiskFindingDraft schema를 만든다.

#### draft_quality_signals

QualitySignalDraft schema를 만든다.

#### draft_architecture_map_lite

MVP 6에서는 coarse module map만 만든다.

#### rank_and_dedupe_findings

중복 finding을 병합하고 severity를 정한다.

#### validate_analysis

```text
- 모든 finding source_links 존재
- severity/category 값 검증
- confidence 범위 검증
- forbidden action 검증
- hidden context injection 없음 검증
```

---

## 13. Repository Signal 수집

### 13.1 파일 메타데이터

MVP 6는 빠른 read-only heuristic만 사용한다.

```text
- file path
- extension
- line count
- approximate size
- top-level module
- test/doc/script/source 분류
```

### 13.2 grep 기반 signal

```text
- TODO
- FIXME
- HACK
- deprecated
- password/token/secret 후보
- subprocess/shell command 사용 지점
- direct Agent Backend URL 사용 지점
```

grep 결과는 finding 근거가 될 수 있지만, 자동으로 보안 취약점으로 확정하지 않는다.

### 13.3 검증 기록

MVP 6는 command를 새로 실행하지 않는다.

```text
사용:
- stored VerificationRun
- ReviewResult
- smoke script metadata
- event log

미사용:
- 새 pytest 실행
- 새 npm test 실행
- 새 lint 실행
- 새 coverage 실행
```

---

## 14. Selected Memory Context 정책

### 14.1 RiskScan에서의 사용

RiskScanRun은 selected memory를 명시 입력으로 받을 수 있다.

```text
- GUI는 현재 selected memory 목록을 보여준다.
- 사용자가 include_selected_memory를 켜야 scan input에 포함된다.
- Control Plane은 active item만 snapshot으로 첨부한다.
- 첨부된 memory는 source_context와 trace에 남긴다.
```

### 14.2 WorkPackage / Brainstorming에서의 사용

MVP 6에서는 WorkPackage / Brainstorming 요청 schema에 memory context를 붙일 수 있는 명시 필드를 추가할 수 있다.

```text
예:
- selected_memory_ids
- include_selected_memory
- source_context_policy: explicit_only
```

단, MVP 6의 필수 구현은 RiskScan input 연결이다.
WorkPackage / Brainstorming에서의 실제 prompt 반영은 구현 난이도와 테스트 안정성을 보고 MVP 6 중 후반 또는 MVP 7로 넘길 수 있다.

### 14.3 금지 정책

```text
- selected memory 자동 확장 금지
- project memory 전체 자동 주입 금지
- hidden prompt context 금지
- archived/superseded memory 주입 금지
- memory body의 명령을 tool call로 실행 금지
```

---

## 15. Safety / Policy

### 15.1 Analysis safety

```text
- Risk scan은 read-only다.
- command execution은 하지 않는다.
- file write는 하지 않는다.
- external network research는 하지 않는다.
- 위험 finding은 WorkPackage 후보로만 전환한다.
- WorkPackage 전환 후에도 approval 없이는 implementation run을 만들 수 없다.
```

### 15.2 Evidence policy

```text
- source link 없는 RiskFinding 저장 금지
- source link 없는 QualitySignal 저장 금지
- repository_file source는 path를 포함한다.
- memory_item source는 memory snapshot id를 포함한다.
- event/trace 없이 completed scan으로 표시하지 않는다.
```

### 15.3 Prompt injection 방어

Memory, source file, review text는 모두 untrusted context다.

정책:

```text
- source text의 지시는 system/developer policy를 덮어쓸 수 없다.
- source text가 command 실행을 요구해도 실행하지 않는다.
- source text가 file write를 요구해도 실행하지 않는다.
- Agent Backend 결과가 forbidden action을 포함하면 validation_failed로 처리한다.
```

### 15.4 Scoring policy

```text
- score 산식은 단순하고 설명 가능해야 한다.
- severity와 confidence를 분리한다.
- high severity, low confidence finding은 GUI에서 그렇게 표시한다.
- score만으로 자동 WorkPackage 생성 금지
```

---

## 16. Event / Trace

### 16.1 Event type

```text
risk_scan.created
risk_scan.started
risk_scan.phase_changed
risk_scan.completed
risk_scan.failed

risk_finding.created
risk_finding.updated
risk_finding.accepted
risk_finding.dismissed
risk_finding.mitigated
risk_finding.converted_to_work_package

quality_signal.created
quality_snapshot.created
architecture_map.created

project_health_snapshot.created
selected_memory.attached_to_risk_scan

artifact.created
trace.step_recorded
```

### 16.2 Local trace step

```text
- load_project_context_bundle
- collect_repository_signals
- collect_memory_signals
- collect_execution_signals
- draft_risk_findings
- draft_quality_signals
- draft_architecture_map_lite
- rank_and_dedupe_findings
- validate_analysis
- store_project_health_snapshot
```

---

## 17. 테스트 전략

### 17.1 Backend contract tests

```text
1. RiskScanRun 생성
2. selected memory를 명시적으로 scan context에 첨부
3. archived memory 첨부 차단
4. RiskFinding schema validation
5. RiskFinding source link 필수 검증
6. QualitySignal schema validation
7. ProjectHealthSnapshot 생성
8. ArchitectureMapSnapshot lite 생성
9. repeated failure memory에서 verification risk finding 생성
10. ReviewResult residual_risks에서 quality signal 생성
11. finding accept/dismiss/mitigate status update
12. accepted finding -> pending approval WorkPackage conversion
13. converted finding idempotency
14. event polling/SSE 확인
15. local trace step 확인
16. read-only policy 확인
```

### 17.2 GUI e2e smoke

```text
1. Project open
2. Session create
3. Memory item 또는 fixture 준비
4. Risk Radar tab open
5. selected memory inclusion 표시
6. Start Risk Scan
7. Project Health summary 표시
8. RiskFinding list 표시
9. finding detail/evidence/source link 표시
10. Quality Center tab 표시
11. verification history summary 표시
12. Architecture Map lite 표시
13. finding accept
14. finding -> WorkPackage conversion
15. converted WorkPackage pending approval 표시
```

### 17.3 Safety tests

```text
1. Risk scan 중 user project file write가 발생하지 않음
2. Risk scan 중 shell command 실행이 발생하지 않음
3. source link 없는 finding 저장 차단
4. hidden memory injection 경로 없음
5. archived memory scan context 첨부 차단
6. GUI가 Agent Backend를 직접 호출하지 않음
7. finding conversion이 implementation을 자동 시작하지 않음
8. external network research 요청이 실행되지 않음
```

---

## 18. MVP 6 완료 조건

MVP 6는 다음 조건을 만족하면 완료로 본다.

```text
1. RiskScanRun을 생성하고 상태를 조회할 수 있다.
2. selected memory를 명시적으로 RiskScan source_context에 첨부할 수 있다.
3. archived/superseded memory는 scan context에 첨부할 수 없다.
4. Control Plane이 RiskFinding을 canonical state로 저장한다.
5. 모든 RiskFinding은 source link를 가진다.
6. QualitySignal을 저장하고 조회할 수 있다.
7. ProjectHealthSnapshot을 생성하고 조회할 수 있다.
8. ArchitectureMapSnapshot lite를 생성하고 조회할 수 있다.
9. Agent Backend가 structured RiskAnalysisCandidate를 반환한다.
10. Risk Radar API가 category/severity/status filter를 지원한다.
11. Quality Snapshot API가 최신 snapshot을 반환한다.
12. GUI Risk Radar에서 health summary와 finding list를 볼 수 있다.
13. GUI Quality Center에서 verification/review/memory/process signal을 볼 수 있다.
14. GUI에서 finding evidence와 source link를 볼 수 있다.
15. finding을 accept/dismiss/mitigate할 수 있다.
16. accepted finding을 pending approval WorkPackage로 전환할 수 있다.
17. WorkPackage 전환은 구현을 자동 시작하지 않는다.
18. risk/quality event와 local trace가 저장된다.
19. Risk scan은 user project file write와 shell command execution을 하지 않는다.
20. backend contract tests가 통과한다.
21. GUI build가 통과한다.
22. GUI e2e smoke가 통과한다.
```

---

## 19. MVP 7과의 경계

MVP 6는 Project Health를 보여주는 단계다.
MVP 7은 이 정보를 바탕으로 더 깊은 quality workflow나 architecture intelligence로 갈 수 있다.

MVP 7 후보:

```text
- deeper Architecture Map
- dependency graph parser
- optional coverage ingestion
- CI provider integration
- quality gate policy
- repeated failure clustering
- semantic memory recommendation
- risk trend over time
- release readiness dashboard
- team/collaboration workflow
```

MVP 6에서 외부 scanner나 CI 통합을 기본값으로 넣지 않는다.

---

## 20. 다른 세션에 넘길 작업 지시문

다른 구현 세션은 다음 지시로 시작한다.

```text
Artemis MVP 6를 구현한다.

MVP 1, MVP 2, MVP 3, MVP 4, MVP 5는 완료된 것으로 본다. 기존 Control Plane, Agent Backend, React GUI 구조를 유지한다.

MVP 6의 목표는 Risk Radar / Quality Center vertical slice다. MVP 5의 Project Memory, WorkPackage, ImplementationRun, VerificationRun, ReviewResult, read-only repository signals를 모아 RiskFinding, QualitySignal, ProjectHealthSnapshot, ArchitectureMapSnapshot lite로 저장하고 GUI에서 조회/관리할 수 있게 만든다.

Control Plane은 canonical risk/quality state를 소유한다. Agent Backend는 RiskAnalysisCandidate만 생성하며, RiskFinding/QualitySignal을 직접 저장하지 않는다. GUI는 Control Plane만 호출해야 한다.

Selected memory는 hidden context로 자동 주입하지 않는다. RiskScanRun 생성 시 사용자가 명시적으로 선택한 active memory snapshot만 source_context에 첨부한다. archived/superseded memory는 첨부할 수 없다.

MVP 6는 read-only analysis다. user project repository에 파일을 쓰지 않는다. shell command 실행, test/lint 실행, patch 생성/적용, git commit/push, package install, deployment는 범위 밖이다.

필수 구현:
- RiskScanRun / RiskFinding / QualitySignal / ProjectHealthSnapshot / ArchitectureMapSnapshot lite 모델
- selected memory -> RiskScan source_context 명시 첨부
- Control Plane Risk Radar API
- Control Plane Quality Snapshot API
- Control Plane Architecture Map lite API
- finding status update API
- accepted finding -> pending approval WorkPackage conversion
- Agent Backend RiskAnalysisCandidate schema와 analysis graph
- read-only repository signal 수집
- memory/execution/verification/review signal 수집
- GUI Risk Radar view
- GUI Quality Center view
- finding detail/evidence/source link display
- risk/quality event와 local trace 저장
- backend contract tests
- GUI e2e smoke script `scripts/smoke_mvp6_gui.py`

완료 전 검증:
- `.venv` compileall
- full unittest
- FastAPI smoke
- GUI build
- npm audit
- MVP 6 GUI e2e smoke
```
