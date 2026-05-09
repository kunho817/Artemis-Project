# Artemis 기획 문서

> 개인 개발자가 대규모 프로젝트를 팀처럼 운영할 수 있도록 돕는 GUI 기반 멀티 Agent 개발 운영 시스템

---

## 0. 문서 목적

이 문서는 Artemis의 초기 기획과 시스템 설계를 정리한 문서이다.  
이후 Codex 또는 기타 AI 개발 도구에서 구현 방향을 이어가기 위한 기준 문서로 사용한다.

Artemis는 단순한 AI 코딩 도구가 아니다.  
Artemis의 목표는 개인 개발자가 혼자서는 감당하기 어려운 대규모 프로젝트의 기획, 설계, 구현, 검증, 리뷰, 문서화, 일정 관리, 기술 의사결정, 리스크 관리를 보조하는 것이다.

---

## 0.1 현재 구현 상태

2026-05-10 기준으로 MVP 1부터 MVP 6까지의 vertical slice는 구현과 계획측 재검증이 완료된 것으로 본다.

완료된 기준 문서:

```text
- docs/artemis_mvp1.md
- docs/artemis_mvp2.md
- docs/artemis_mvp3.md
- docs/artemis_mvp4.md
- docs/artemis_mvp5.md
- docs/artemis_mvp6.md
```

완료된 기능 축:

```text
MVP 1: Work Package backend foundation
MVP 2: GUI + Event Stream
MVP 3: Implementation Pipeline
MVP 4: Brainstorming Room + Decision Record
MVP 5: Memory / Decision Log
MVP 6: Risk Radar / Quality Center
```

현재 `main` 브랜치는 새 Artemis 구현을 기준으로 한다.
기존 Go TUI 구현은 `legacy/go-tui` 브랜치에 보존한다.

이 문서는 초기 장기 기획 문서이므로 일부 예시는 아직 미래형 표현을 유지한다.
다만 MVP 1~6에 해당하는 기반 기능은 완료된 현행 baseline으로 취급한다.

---

## 1. Artemis의 핵심 정의

### 1.1 한 문장 정의

**Artemis는 개인 개발자를 위한 가상 개발 조직이다.**

또는 다음과 같이 정의할 수 있다.

**Artemis는 개인 개발자가 팀 없이도 대규모 프로젝트를 설계, 구현, 검증, 운영할 수 있도록 돕는 GUI 기반 멀티 Agent 개발 운영 시스템이다.**

---

### 1.2 Artemis가 해결하려는 문제

개인 개발자는 대규모 프로젝트를 진행할 때 다음 한계를 겪는다.

- 전체 프로젝트 맥락을 장기간 유지하기 어렵다.
- 기능 우선순위와 범위 조절이 어렵다.
- 설계 결정을 검토해줄 사람이 없다.
- 구현 후 코드 리뷰와 QA가 부족하다.
- 테스트, 문서화, 리팩토링이 뒤로 밀린다.
- 기술 부채와 리스크를 체계적으로 추적하기 어렵다.
- 장기 프로젝트에서 “왜 이렇게 만들었는지”를 잊기 쉽다.
- 혼자서는 브레인스토밍, 반론, 대안 검토가 부족하다.

Artemis는 이러한 팀/기업 환경에서 제공되는 기능을 AI Agent와 시스템 구조를 통해 개인에게 제공한다.

---

## 2. 핵심 방향성

Artemis의 핵심 방향은 다음과 같다.

1. GUI는 단순 채팅창이 아니라 개발 지휘실이어야 한다.
2. GUI Backend와 Python Agent Backend를 분리한다.
3. Python Agent Backend는 LangChain, LangGraph, local observability를 활용한다.
4. 모든 작업은 Work Package로 구조화한다.
5. Brainstorming Room으로 팀 회의의 부재를 보완한다.
6. Architecture Review Board로 설계 편향을 줄인다.
7. QA, Review, Risk Radar로 개인의 검증 한계를 보완한다.
8. Project Memory로 장기 프로젝트의 맥락 손실을 막는다.
9. Tool Permission과 Approval Gate로 자동화의 위험을 통제한다.
10. Artemis는 코딩 도구가 아니라 개인용 개발 조직이다.

---

## 3. 전체 시스템 아키텍처

### 3.1 상위 구조

```text
┌──────────────────────────────────────────────┐
│                 Artemis GUI                  │
│  Dashboard, Agent Room, Brainstorming Room   │
│  Diff Viewer, Memory View, Risk Radar        │
└──────────────────────┬───────────────────────┘
                       │ HTTP / WebSocket / SSE
                       ▼
┌──────────────────────────────────────────────┐
│           GUI Backend / Control Plane         │
│  Session, Project, API Gateway, Event Relay   │
│  Approval, Task Status, GUI State             │
└──────────────────────┬───────────────────────┘
                       │ Internal API / Queue
                       ▼
┌──────────────────────────────────────────────┐
│        Python Agent Backend / Brain           │
│  LangGraph Orchestrator                       │
│  LangChain Tools, Models, RAG                 │
│  Local Trace / Observability                  │
└──────────────────────┬───────────────────────┘
                       │ Tool Calls
                       ▼
┌──────────────────────────────────────────────┐
│             Project Runtime Layer             │
│  File, Git, Shell, Test, LSP, MCP, Browser    │
└──────────────────────┬───────────────────────┘
                       ▼
┌──────────────────────────────────────────────┐
│               User Project Repo               │
└──────────────────────────────────────────────┘
```

---

## 4. 시스템 분리 원칙

### 4.1 GUI Client

사용자가 직접 조작하는 인터페이스이다.

주요 역할:

- 프로젝트 대시보드 표시
- Agent 활동 표시
- Brainstorming Room 제공
- Work Package 표시 및 승인
- Diff 및 코드 리뷰 화면 제공
- 테스트/빌드 결과 표시
- 프로젝트 메모리 탐색
- 리스크와 품질 상태 시각화
- 승인/거절/일시정지/재개 조작 제공

권장 기술:

- Tauri + React
- 또는 Next.js + React

개인 로컬 개발 도구라면 Tauri 기반 GUI가 적합하다.  
웹 기반 협업 가능성을 열어두려면 Next.js 기반도 가능하다.

---

### 4.2 GUI Backend / Control Plane

GUI Backend는 Agent 판단을 직접 수행하지 않는다.  
대신 사용자 인터페이스와 Python Agent Backend 사이의 제어 계층 역할을 한다.

주요 역할:

- 프로젝트 목록 관리
- 프로젝트 열기/닫기
- 세션 관리
- 사용자 설정 관리
- Agent 작업 요청 생성
- 작업 상태 조회
- 승인 요청 관리
- 이벤트 스트림 중계
- Diff 캐싱
- 작업 히스토리 저장
- GUI용 데이터 정규화

GUI Backend는 Artemis의 Control Plane이다.

---

### 4.3 Python Agent Backend / Intelligence Plane

Python Agent Backend는 Artemis의 두뇌 역할을 한다.

주요 역할:

- 사용자 요청 해석
- Intent 분류
- 작업 분해
- Work Package 생성
- 관련 컨텍스트 수집
- Agent 선택
- LangGraph 기반 실행 흐름 제어
- LangChain 기반 모델/툴/RAG 호출
- 브레인스토밍 실행
- 구현 계획 생성
- 코드 수정 제안
- 테스트/검증 계획 생성
- 리뷰 및 리스크 분석
- 장기 메모리 업데이트
- Local trace 기반 추적/디버깅/평가

---

## 5. LangChain, LangGraph, Observability 활용 전략

### 5.1 역할 분리

```text
LangChain  = 모델, 도구, RAG, Retriever, Prompt, Output Parser
LangGraph  = Agent 실행 흐름, 상태, 분기, 승인, 재시도, 장기 작업 제어
Local Trace = 실행 추적, 디버깅, 평가, 비용/성능 관찰
```

---

### 5.2 LangChain의 역할

LangChain은 Artemis에서 Agent 구성 부품으로 사용한다.

담당 영역:

- LLM 호출 추상화
- Tool 정의
- RAG / Retriever
- 코드베이스 검색
- 문서 검색
- Prompt Template
- Output Parser
- Model Adapter
- Embedding
- Vector Store 연동

LangChain은 중앙 오케스트레이터로 쓰기보다는, LangGraph node 내부에서 사용하는 구성 요소로 보는 것이 적절하다.

---

### 5.3 LangGraph의 역할

LangGraph는 Artemis의 핵심 실행 엔진이다.

담당 영역:

- Agent workflow graph 구성
- 상태 기반 실행
- 조건 분기
- 재시도
- 실패 복구
- human-in-the-loop
- 장기 실행 작업
- checkpoint
- event streaming
- node 단위 실행 추적

Artemis의 주요 흐름은 LangGraph graph로 표현한다.

```text
START
  ↓
classify_intent
  ↓
collect_context
  ↓
create_work_package
  ↓
route_by_intent
  ├─ brainstorming_flow
  ├─ architecture_review_flow
  ├─ implementation_flow
  ├─ debugging_flow
  └─ documentation_flow
  ↓
verify_result
  ↓
human_approval_if_needed
  ↓
update_memory
  ↓
END
```

---

### 5.4 Observability의 역할

Artemis는 관제/디버깅/평가 계층을 기본 제공한다.

기본 backend는 LangSmith Cloud가 아니라 Artemis 내부 local trace store이다. LangSmith Cloud는 사용량 비용이 발생할 수 있으므로 기본값으로 사용하지 않는다.

담당 영역:

- Agent 실행 trace 확인
- prompt 입력/출력 추적
- tool call 추적
- 실패 원인 분석
- token 사용량 분석
- latency 분석
- 비용 추적
- Agent별 성능 비교
- prompt 변경 전후 평가
- regression evaluation

Observability는 개발 초기부터 붙이는 것이 좋다.  
다만 개인 코드베이스의 프라이버시와 비용을 고려해야 하므로 다음 선택지를 둔다.

1. 기본: Artemis local trace store
2. 고급 로컬/사내 환경: self-hosted LangSmith endpoint 연동
3. 명시적 opt-in: LangSmith Cloud 연동
4. 모든 모드: trace redaction policy 적용

---

## 6. Artemis의 주요 기능

---

## 6.1 Project Command Center

GUI의 메인 화면이다.  
단순 홈 화면이 아니라 프로젝트 전체 상태를 보여주는 개발 지휘실이다.

표시 요소:

```text
Project Command Center
├─ 현재 프로젝트 상태
│  ├─ 진행 중인 목표
│  ├─ 최근 변경 파일
│  ├─ 실패한 테스트
│  ├─ 열린 TODO
│  └─ 위험 영역
│
├─ Agent Activity
│  ├─ 어떤 Agent가 무엇을 하는지
│  ├─ 현재 단계
│  ├─ 실행한 도구
│  └─ 대기 중인 승인
│
├─ Roadmap / Milestone
│  ├─ 이번 버전 목표
│  ├─ 다음 작업 후보
│  ├─ 미완성 기능
│  └─ 기술 부채
│
├─ Memory / Decisions
│  ├─ 아키텍처 결정
│  ├─ 프로젝트 규칙
│  ├─ 과거 버그 원인
│  └─ 반복되는 작업 패턴
│
└─ Quality Panel
   ├─ 테스트 상태
   ├─ 빌드 상태
   ├─ 타입체크 상태
   ├─ 코드 품질
   └─ 보안 경고
```

---

## 6.2 Brainstorming Room

개인 개발자가 혼자서는 얻기 어려운 아이디어 발산, 반론, 대안 검토를 제공하는 기능이다.

### 목적

- 기능 아이디어 검토
- 아키텍처 대안 비교
- 구현 전략 논의
- 리스크 분석
- 반대 의견 생성
- Work Package로 변환

### Brainstorming 모드

```text
1. 자유 발산 모드
   - 아이디어를 최대한 많이 생성

2. 설계 토론 모드
   - 여러 아키텍처 대안을 비교

3. 비판 모드
   - 현재 아이디어의 약점만 찾음

4. 제품 기획 모드
   - 사용자 가치, 기능 우선순위 분석

5. 리스크 분석 모드
   - 실패 가능성, 보안, 유지보수 위험 분석

6. 구현 전략 모드
   - 현실적인 개발 순서 제안

7. 회의 시뮬레이션 모드
   - PM, Tech Lead, QA, Designer, DevOps가 토론

8. 반대자 모드
   - 일부러 강하게 반박하는 Agent 투입
```

### Brainstorming LangGraph 흐름

```text
START
  ↓
prepare_topic
  ↓
select_agents
  ↓
parallel_agent_opinions
  ↓
cross_critique
  ↓
synthesize_options
  ↓
rank_options
  ↓
create_recommendation
  ↓
optionally_create_work_package
  ↓
END
```

---

## 6.3 Virtual Team System

Artemis는 상황에 따라 가상 팀을 구성한다.

기본 Agent 역할:

```text
Artemis Virtual Team
├─ Project Manager
├─ Product Planner
├─ System Architect
├─ Backend Engineer
├─ Frontend Engineer
├─ QA Engineer
├─ Security Reviewer
├─ DevOps Engineer
├─ Documentation Writer
├─ UX Critic
├─ Refactor Specialist
├─ Researcher
└─ Devil’s Advocate
```

Agent는 항상 실행되는 것이 아니라, 작업 유형에 따라 필요한 Agent만 선택된다.

예시:

```text
기능 추가 요청:
- Project Manager
- Product Planner
- System Architect
- Backend Engineer
- Frontend Engineer
- QA Engineer
- Reviewer

버그 수정 요청:
- Debugger
- Explorer
- Tester
- Reviewer

기획 요청:
- Product Planner
- UX Critic
- Devil’s Advocate
- Architect

리팩토링 요청:
- Architect
- Refactor Specialist
- Tester
- Risk Reviewer
```

---

## 6.4 Project Memory

대규모 프로젝트에서 가장 중요한 것은 맥락 유지이다.

Artemis는 다음 메모리 계층을 가진다.

```text
Project Memory
├─ Decision Memory
│  ├─ Architecture Decision Record
│  ├─ 기술 선택 이유
│  ├─ 포기한 대안
│  └─ 나중에 다시 볼 논점
│
├─ Work Memory
│  ├─ 진행 중인 작업
│  ├─ 완료된 작업
│  ├─ 중단된 작업
│  ├─ 실패한 시도
│  └─ 다음 행동
│
├─ Codebase Memory
│  ├─ 주요 모듈 설명
│  ├─ 파일별 책임
│  ├─ 의존성 관계
│  ├─ 위험한 레거시 코드
│  └─ 자주 수정되는 영역
│
├─ User Preference Memory
│  ├─ 선호하는 아키텍처
│  ├─ 코딩 스타일
│  ├─ 테스트 기준
│  ├─ 문서 작성 방식
│  └─ 피하고 싶은 방식
│
└─ Failure Memory
   ├─ 과거 빌드 실패
   ├─ 반복된 버그
   ├─ 잘못된 설계 판단
   └─ 복구 방법
```

---

## 6.5 Work Package System

Artemis는 사용자 요청을 바로 코드 수정으로 넘기지 않는다.  
먼저 Work Package로 구조화한다.

### Work Package 구성

```text
Work Package
├─ 목표
├─ 배경
├─ 범위
├─ 제외 범위
├─ 관련 파일
├─ 필요한 Agent
├─ 구현 단계
├─ 검증 방법
├─ 리스크
├─ 승인 필요 여부
└─ 완료 조건
```

### 예시

```json
{
  "title": "Add Brainstorming Room",
  "goal": "사용자가 여러 Agent 관점으로 아이디어를 검토할 수 있는 GUI 기능 추가",
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
  "agents": [
    "ProductPlanner",
    "Architect",
    "SecurityReviewer",
    "DevilAdvocate"
  ],
  "verification": [
    "API contract test",
    "GUI rendering test",
    "session persistence test"
  ],
  "risk": "medium"
}
```

---

## 6.6 Architecture Review Board

개인 개발자의 설계 편향을 보완하는 기능이다.

구성 Agent:

- System Architect
- Backend Engineer
- Frontend Engineer
- Security Reviewer
- Performance Reviewer
- Maintenance Reviewer
- Devil’s Advocate

검토 항목:

- 이 구조가 너무 복잡한가?
- 지금 단계에서 과한 설계인가?
- 나중에 확장 가능한가?
- 테스트하기 쉬운가?
- 장애가 났을 때 복구 가능한가?
- 특정 라이브러리에 과하게 종속되는가?
- 보안상 위험한가?
- 개인 개발자가 유지할 수 있는가?

결과는 ADR 형태로 저장한다.

---

## 6.7 Implementation Pipeline

구현 작업은 다음 흐름으로 처리한다.

```text
Idea
→ Brainstorming
→ Work Package
→ Architecture Review
→ Implementation Plan
→ Patch
→ Test
→ Review
→ Documentation
→ Memory Update
```

LangGraph 흐름:

```text
START
  ↓
classify_request
  ↓
collect_context
  ↓
create_work_package
  ↓
human_approval_for_scope
  ↓
create_implementation_plan
  ↓
generate_patch
  ↓
apply_patch_safely
  ↓
run_tests
  ↓
review_diff
  ↓
if failed:
      analyze_failure → revise_patch → run_tests
  else:
      update_docs → update_memory → final_report
```

---

## 6.8 Review & QA System

Artemis는 코드 수정 후 반드시 검증과 리뷰를 수행한다.

Review Agent 검토 항목:

- 변경 범위가 요구사항과 일치하는가?
- 불필요하게 큰 수정이 있는가?
- 기존 기능을 깨뜨릴 가능성이 있는가?
- 테스트가 충분한가?
- 예외 처리가 있는가?
- 에러 메시지가 적절한가?
- 성능 문제가 생길 수 있는가?
- 보안상 위험한 입력 경로가 있는가?
- 문서가 갱신되었는가?

### QA Matrix 예시

```text
Feature: Brainstorming Room

Test Matrix:
- 세션 생성 가능
- Agent 역할 선택 가능
- 토론 이벤트 스트리밍 가능
- 중간 취소 가능
- 결과 요약 저장 가능
- 새로고침 후 상태 복원 가능
- 실패한 Agent 응답 복구 가능
- 권한 없는 도구 호출 차단 가능
```

---

## 6.9 Risk Radar

프로젝트 리스크를 시각적으로 보여주는 기능이다.

```text
Risk Radar
├─ Architecture Risk
│  ├─ 순환 의존성
│  ├─ 모듈 경계 불명확
│  └─ 과도한 결합
│
├─ Implementation Risk
│  ├─ 테스트 없는 핵심 코드
│  ├─ 너무 큰 파일
│  ├─ 복잡한 함수
│  └─ 중복 로직
│
├─ Schedule Risk
│  ├─ 오래 방치된 작업
│  ├─ 계속 미뤄진 기능
│  └─ 범위가 커진 작업
│
├─ Product Risk
│  ├─ 목적이 불분명한 기능
│  ├─ 사용 빈도 낮은 기능
│  └─ MVP를 지연시키는 기능
│
└─ Security Risk
   ├─ secret 노출 가능성
   ├─ 위험한 shell command
   ├─ 외부 API key 관리
   └─ 플러그인 권한 문제
```

---

## 6.10 Meeting Simulator

팀 회의를 시뮬레이션하는 기능이다.

회의 유형:

```text
1. 기획 회의
   - 이 기능이 필요한가?
   - MVP에 들어가야 하는가?

2. 설계 회의
   - 어떤 구조가 좋은가?
   - 대안은 무엇인가?

3. 구현 전 리뷰
   - 이 계획으로 바로 구현해도 되는가?

4. 코드 리뷰 회의
   - diff를 보고 문제를 찾음

5. 버그 포스트모템
   - 왜 문제가 발생했는가?
   - 재발 방지는 무엇인가?

6. 릴리즈 회의
   - 이번 버전에 포함할 것
   - 제외할 것
   - 알려진 문제

7. 리팩토링 회의
   - 지금 리팩토링할 가치가 있는가?
   - 어느 범위까지 해야 하는가?
```

회의 결과는 다음 형식으로 남긴다.

```text
Meeting Result
├─ 결정사항
├─ 반려된 대안
├─ 남은 논점
├─ TODO
├─ 리스크
└─ 다음 작업
```

---

## 7. GUI 화면 구성

### 7.1 Home / Project Dashboard

- 최근 프로젝트
- 현재 작업
- Agent 상태
- 실패한 작업
- 오늘의 추천 작업

---

### 7.2 Project Command Center

- 프로젝트 전체 상태
- Roadmap
- Backlog
- Risk Radar
- Quality 상태
- 현재 진행 중인 Work Package

---

### 7.3 Agent Room

- 채팅형 인터페이스
- Agent별 발언 구분
- 실행 중인 도구 표시
- 현재 계획 표시
- 승인 요청 표시
- 실시간 이벤트 스트림

---

### 7.4 Brainstorming Room

- 주제 입력
- 참여 Agent 선택
- 브레인스토밍 모드 선택
- 토론 진행 표시
- Agent별 의견 비교
- 최종 요약
- Work Package로 변환

---

### 7.5 Work Package View

- 목표
- 배경
- 범위
- 제외 범위
- 관련 파일
- 필요한 Agent
- 구현 단계
- 검증 방법
- 리스크
- 승인 상태
- 실행 버튼

---

### 7.6 Diff & Review View

- 파일 변경 diff
- Agent 리뷰 코멘트
- 테스트 결과
- 승인/거절
- 되돌리기
- 추가 수정 요청

---

### 7.7 Memory View

- 아키텍처 결정
- 프로젝트 규칙
- 과거 버그
- 세션 요약
- 검색
- 태그/날짜/작업 기준 필터

---

### 7.8 Architecture Map

- 모듈 관계
- 의존성 그래프
- 주요 파일
- 위험 영역
- 순환 의존성
- 큰 파일/복잡한 파일 표시

---

### 7.9 Test & Quality Center

- 최근 테스트 결과
- 실패한 테스트
- 커버리지
- 빌드 로그
- 린트/타입체크 상태
- 품질 추세

---

### 7.10 Settings / Policy

- Agent 권한
- Tool 권한
- 위험 명령 차단 규칙
- 모델 설정
- 비용 제한
- 승인 정책
- Observability / Trace 설정
- 프로젝트별 규칙

---

## 8. API 설계 초안

---

## 8.1 Project API

```http
GET  /api/projects
POST /api/projects/open
GET  /api/projects/{project_id}
GET  /api/projects/{project_id}/snapshot
GET  /api/projects/{project_id}/repo-map
GET  /api/projects/{project_id}/architecture-map
GET  /api/projects/{project_id}/risk-radar
GET  /api/projects/{project_id}/quality-snapshot
```

---

## 8.2 Session API

```http
POST /api/sessions
GET  /api/sessions/{session_id}
POST /api/sessions/{session_id}/messages
GET  /api/sessions/{session_id}/events
```

---

## 8.3 Brainstorming API

```http
POST /api/brainstorm
GET  /api/brainstorm/{brainstorm_id}
POST /api/brainstorm/{brainstorm_id}/continue
POST /api/brainstorm/{brainstorm_id}/convert-to-work-package
```

요청 예시:

```json
{
  "project_id": "artemis",
  "topic": "Plugin system design",
  "mode": "architecture_debate",
  "agents": [
    "ProductPlanner",
    "Architect",
    "SecurityReviewer",
    "DevilAdvocate"
  ],
  "output_format": "recommendation"
}
```

---

## 8.4 Work Package API

```http
POST /api/work-packages
GET  /api/work-packages
GET  /api/work-packages/{work_package_id}
POST /api/work-packages/{work_package_id}/approve
POST /api/work-packages/{work_package_id}/execute
POST /api/work-packages/{work_package_id}/pause
POST /api/work-packages/{work_package_id}/cancel
```

---

## 8.5 Agent Task API

```http
POST /api/agent-tasks
GET  /api/agent-tasks/{task_id}
GET  /api/agent-tasks/{task_id}/events
POST /api/agent-tasks/{task_id}/approve-tool-call
POST /api/agent-tasks/{task_id}/reject-tool-call
POST /api/agent-tasks/{task_id}/pause
POST /api/agent-tasks/{task_id}/resume
POST /api/agent-tasks/{task_id}/cancel
```

---

## 8.6 Memory API

```http
GET  /api/memory/search
GET  /api/memory/decisions
POST /api/memory/decisions
GET  /api/memory/session-summaries
GET  /api/memory/project-rules
POST /api/memory/project-rules
```

---

## 9. Event Streaming 설계

Agent 작업은 오래 걸릴 수 있으므로 WebSocket 또는 SSE 기반 이벤트 스트림이 필요하다.

### 이벤트 유형

```text
agent.started
agent.message
agent.completed
agent.failed

tool.call.requested
tool.call.approved
tool.call.rejected
tool.call.started
tool.call.completed
tool.call.failed

work_package.created
work_package.approved
work_package.executing
work_package.completed

verification.started
verification.completed
verification.failed

approval.requested
approval.approved
approval.rejected

memory.updated
decision.created
```

### 이벤트 예시

```json
{
  "type": "agent.started",
  "task_id": "task_001",
  "agent": "Architect",
  "message": "Analyzing plugin system alternatives"
}
```

```json
{
  "type": "tool.call.requested",
  "task_id": "task_001",
  "tool": "patch_file",
  "risk": "medium",
  "requires_approval": true
}
```

```json
{
  "type": "verification.completed",
  "task_id": "task_001",
  "result": "failed",
  "failed_commands": [
    "pnpm test"
  ]
}
```

---

## 10. Python Agent Backend 내부 구조

권장 디렉터리 구조:

```text
python-agent-backend/
├─ app/
│  ├─ main.py
│  ├─ api/
│  │  ├─ sessions.py
│  │  ├─ tasks.py
│  │  ├─ brainstorm.py
│  │  ├─ work_packages.py
│  │  └─ memory.py
│  │
│  ├─ graphs/
│  │  ├─ root_graph.py
│  │  ├─ brainstorming_graph.py
│  │  ├─ implementation_graph.py
│  │  ├─ debugging_graph.py
│  │  └─ review_graph.py
│  │
│  ├─ agents/
│  │  ├─ planner.py
│  │  ├─ architect.py
│  │  ├─ coder.py
│  │  ├─ tester.py
│  │  ├─ reviewer.py
│  │  ├─ product_planner.py
│  │  └─ devil_advocate.py
│  │
│  ├─ tools/
│  │  ├─ file_tools.py
│  │  ├─ git_tools.py
│  │  ├─ shell_tools.py
│  │  ├─ test_tools.py
│  │  ├─ search_tools.py
│  │  └─ lsp_tools.py
│  │
│  ├─ memory/
│  │  ├─ project_memory.py
│  │  ├─ decision_memory.py
│  │  ├─ vector_store.py
│  │  └─ checkpoint_store.py
│  │
│  ├─ policies/
│  │  ├─ tool_policy.py
│  │  ├─ approval_policy.py
│  │  └─ risk_policy.py
│  │
│  ├─ observability/
│  │  ├─ langsmith.py
│  │  └─ event_publisher.py
│  │
│  └─ schemas/
│     ├─ state.py
│     ├─ events.py
│     ├─ work_package.py
│     └─ memory.py
```

---

## 11. LangGraph State 설계 예시

```python
from typing import TypedDict, Literal, List, Dict, Any, Optional

class ArtemisState(TypedDict):
    project_id: str
    session_id: str
    user_request: str

    intent: Optional[str]
    current_phase: Optional[str]

    context_files: List[str]
    context_summary: Optional[str]

    work_package: Optional[Dict[str, Any]]
    plan: Optional[Dict[str, Any]]

    agent_outputs: Dict[str, Any]
    tool_results: List[Dict[str, Any]]

    proposed_patches: List[Dict[str, Any]]
    test_results: Optional[Dict[str, Any]]
    review_result: Optional[Dict[str, Any]]

    risk_level: Literal["low", "medium", "high", "critical"]
    requires_approval: bool
    approval_status: Optional[Literal["pending", "approved", "rejected"]]

    final_report: Optional[str]
    errors: List[Dict[str, Any]]
```

이 상태는 GUI에서 다음 정보를 표시하기 위해 사용한다.

- 현재 단계
- 실행 중인 Agent
- 읽은 파일
- 제안된 patch
- 테스트 결과
- 승인 대기 상태
- 실패 원인
- 최종 보고서

---

## 12. Agent 권한 정책

Agent마다 도구 권한을 다르게 부여해야 한다.

| Agent | 읽기 | 쓰기 | Shell | Git | 승인 필요 |
|---|---:|---:|---:|---:|---:|
| Product Planner | 가능 | 불가 | 불가 | 불가 | 낮음 |
| Architect | 가능 | 문서만 | 불가 | 불가 | 낮음 |
| Explorer | 가능 | 불가 | 제한 | 불가 | 낮음 |
| Coder | 가능 | 가능 | 제한 | 불가 | 중간 |
| Tester | 가능 | 테스트 파일 가능 | 가능 | 불가 | 중간 |
| Reviewer | 가능 | 불가 | 제한 | 불가 | 낮음 |
| DevOps | 가능 | 가능 | 가능 | 제한 | 높음 |
| Security Reviewer | 가능 | 불가 | 제한 | 불가 | 낮음 |

---

## 13. 위험 작업 승인 정책

다음 작업은 반드시 human approval이 필요하다.

```text
- 대량 파일 수정
- 파일 삭제
- 패키지 설치
- DB migration
- Git reset/rebase
- 배포
- secret 접근
- 외부 API 호출
- shell destructive command
- 프로젝트 root 외부 파일 접근
```

위험 명령 예시:

```text
rm -rf
git reset --hard
git clean -fd
sudo
curl | sh
Invoke-WebRequest | iex
DROP DATABASE
DELETE FROM without WHERE
```

---

## 14. Tool Layer 설계

도구는 권한 기반으로 나눈다.

```text
Read Tools
- read_file
- list_files
- grep
- search_symbols
- semantic_search

Write Tools
- write_file
- patch_file
- create_file
- move_file

Execution Tools
- run_command
- run_test
- run_lint
- run_typecheck
- run_build

Git Tools
- git_status
- git_diff
- git_branch
- git_commit
- git_restore

External Tools
- GitHub issue/PR
- package registry
- documentation search
- MCP servers
```

도구 호출은 반드시 Tool Router를 통해 수행한다.

Tool Router의 책임:

- 권한 확인
- 위험도 분석
- approval 필요 여부 판단
- 실행 전 로그 기록
- 실행 후 결과 저장
- GUI 이벤트 발행
- Local trace 연결

---

## 15. Context Engine 설계

대규모 프로젝트 지원에서 가장 중요한 부분 중 하나이다.

필수 구성:

```text
Context Engine
├─ Repo-map
│  ├─ 파일 트리
│  ├─ 모듈 관계
│  ├─ 클래스/함수/심볼 목록
│  └─ 의존성 그래프
│
├─ LSP / AST 분석
│  ├─ 정의로 이동
│  ├─ 참조 찾기
│  ├─ 타입 오류 확인
│  └─ import 관계 분석
│
├─ Semantic Search
│  ├─ 코드 청크 임베딩
│  ├─ 문서 검색
│  └─ 과거 세션 검색
│
└─ Flow Tracker
   ├─ 이번 작업에서 읽은 파일
   ├─ 수정한 파일
   ├─ 테스트한 파일
   └─ 실패한 명령
```

RAG만으로는 부족하다.  
코드 프로젝트에서는 Repo-map, LSP, AST, grep, semantic search를 함께 사용해야 한다.

---

## 16. Storage 설계

권장 저장소:

```text
SQLite
- 프로젝트 상태
- 세션
- Work Package
- Decision Memory
- 작업 히스토리
- 승인 기록

Vector DB
- 코드 검색
- 문서 검색
- 과거 세션 검색
- 결정사항 검색

JSONL
- event log
- raw tool call log
- recovery log

Git
- 파일 변경 추적
- checkpoint
- rollback
```

초기 MVP에서는 SQLite + JSONL + Git만으로 시작하고, 이후 Vector DB를 추가해도 된다.

---

## 17. 추천 기술 스택

### 17.1 GUI Client

```text
추천:
- Tauri + React
- 또는 Next.js + React

개인용 로컬 앱:
- Tauri 권장

웹 기반:
- Next.js 권장
```

---

### 17.2 GUI Backend

```text
선택지:
- FastAPI
- NestJS

단순 MVP:
- FastAPI

확장성 중시:
- GUI Backend: NestJS
- Agent Backend: FastAPI
```

---

### 17.3 Python Agent Backend

```text
- FastAPI
- LangChain
- LangGraph
- Local trace store / optional self-hosted LangSmith
- Pydantic
- SQLite / PostgreSQL
- Chroma / FAISS / Qdrant
- tree-sitter
- ripgrep
- LSP client
```

---

### 17.4 Runtime Tools

```text
- Git
- ripgrep
- pytest / vitest / pnpm test
- mypy / pyright / ruff
- eslint / tsc
- Docker optional
```

---

## 18. MVP 개발 순서

---

### MVP 1 — Agent Backend 기반 [완료]

목표:

- 사용자 요청 수신
- Intent 분류
- Work Package 생성
- 관련 파일 검색
- 간단한 계획 생성
- Local trace 연결

구성:

- FastAPI
- LangGraph root_graph
- LangChain model/tool
- read_file
- list_files
- grep
- git_status

완료 기준:

- 자연어 요청을 구조화된 Work Package로 변환
- Control Plane / Agent Backend 경계 확립
- local trace, event, approval, artifact 저장
- read-only tool layer와 backend contract test 확보

---

### MVP 2 — GUI + Event Stream [완료]

목표:

- GUI에서 Agent 진행 상황 확인

구성:

- React/Tauri GUI
- GUI Backend
- WebSocket 또는 SSE
- task status
- event log
- approval request view

완료 기준:

- React/Vite GUI skeleton
- async Work Package request
- event polling/SSE
- local trace/artifact viewer
- GUI e2e smoke

---

### MVP 3 — Implementation Pipeline [완료]

목표:

- patch 생성
- diff 표시
- 승인 후 적용
- 테스트 실행
- 실패 로그 분석

LangGraph nodes:

```text
collect_context
plan
generate_patch
request_approval
apply_patch
run_tests
review
```

완료 기준:

- approved WorkPackage -> ImplementationRun
- ImplementationPlan / PatchSet / Diff Viewer
- approval-gated patch apply
- VerificationRun / ReviewResult
- implementation timeline과 GUI smoke

---

### MVP 4 — Brainstorming Room [완료]

목표:

- 여러 Agent 의견 생성
- cross critique
- 최종 추천안 생성
- Work Package 변환

구성:

- topic 입력
- agent role 선택
- mode 선택
- event stream
- final summary
- convert to work package

완료 기준:

- BrainstormingSession
- role contribution / critique / option
- DecisionBrief accept/reject
- accepted DecisionRecord
- DecisionRecord -> pending approval WorkPackage conversion

---

### MVP 5 — Memory / Decision Log [완료]

목표:

- ADR 저장
- 세션 요약
- 프로젝트 규칙 저장
- 과거 결정 검색
- Failure Memory 저장

완료 기준:

- ProjectMemoryItem / MemorySourceLink / MemoryExtractionRun
- DecisionRecord promotion
- Project Rule / Session Summary / Failure Memory
- SQLite/FTS search
- explicit selected memory context
- GUI Memory View

---

### MVP 6 — Risk Radar / Quality Center [완료]

목표:

- 리스크 분석
- 테스트 상태 표시
- 기술 부채 탐지
- 코드 품질 snapshot 제공

완료 기준:

- RiskScanRun / RiskFinding / QualitySignal
- ProjectHealthSnapshot
- ArchitectureMapSnapshot lite
- explicit selected memory RiskScan context
- finding status management
- accepted finding -> pending approval WorkPackage conversion
- GUI Risk Radar / Quality Center
- read-only analysis policy

---

## 19. Codex에서 이어갈 때의 우선 구현 과제

MVP 1~6 완료 이후 Codex에서는 다음 순서로 작업을 이어가는 것이 좋다.

1. deterministic Work Package fallback을 LLM-generated structured output으로 교체한다.
2. deterministic MVP 3 implementation proposal/log patch를 LLM-generated structured PatchSet으로 교체한다.
3. LangGraph checkpointing을 실제 장기 실행 흐름에 연결한다.
4. Architecture Map lite를 더 깊은 dependency / boundary map으로 확장한다.
5. Risk Radar finding trend와 repeated failure clustering을 추가한다.
6. Quality Center에 선택적 coverage / CI result ingestion을 붙인다.
7. Memory 기반 context recommendation을 실험하되 hidden automatic RAG는 기본값으로 넣지 않는다.
8. release readiness dashboard를 추가한다.
9. 협업/웹 사용을 고려한 사용자, 권한, project sharing 모델을 설계한다.
10. plugin/MCP/tool permission policy를 GUI에서 관리할 수 있게 한다.

---

## 20. 초기 Repository 예시

```text
artemis/
├─ apps/
│  ├─ gui/
│  │  ├─ package.json
│  │  ├─ src/
│  │  └─ tauri.conf.json
│  │
│  └─ gui-backend/
│     ├─ src/
│     └─ package.json
│
├─ services/
│  └─ agent-backend/
│     ├─ app/
│     ├─ pyproject.toml
│     └─ README.md
│
├─ packages/
│  ├─ shared-schemas/
│  └─ api-client/
│
├─ docs/
│  ├─ architecture/
│  ├─ adr/
│  └─ planning/
│
├─ .artemis/
│  ├─ memory/
│  ├─ events/
│  └─ config.yaml
│
└─ README.md
```

---

## 21. 중요한 설계 원칙

### 21.1 Agent는 자율 작업자가 아니라 검증 가능한 실행 노드이다

Agent가 무제한으로 생각하고 행동하게 만들면 위험하다.  
Agent는 명확한 입력, 출력, 권한, 책임을 가진 실행 노드여야 한다.

---

### 21.2 Orchestrator가 책임을 가진다

Agent끼리 자유롭게 대화만 시키면 책임 경계가 흐려진다.  
Orchestrator가 작업 흐름, 상태, 승인, 실패 복구를 책임져야 한다.

---

### 21.3 Context 품질이 코드 품질을 결정한다

대규모 프로젝트에서 Agent가 실패하는 주된 이유는 코드 작성 능력 부족이 아니라 잘못된 컨텍스트이다.  
Repo-map, LSP, semantic search, grep을 함께 써야 한다.

---

### 21.4 검증 없는 완료는 금지한다

작업 완료는 Agent의 선언이 아니라 다음 근거로 판단해야 한다.

- 테스트 결과
- 빌드 결과
- 타입체크 결과
- lint 결과
- diff review
- 요구사항 충족 여부

---

### 21.5 모든 중요한 결정은 Memory에 남긴다

특히 다음은 반드시 기록한다.

- 아키텍처 결정
- 포기한 대안
- 위험한 임시방편
- 실패한 시도
- 반복된 버그
- 다음 작업

---

## 22. 피해야 할 설계

```text
1. Agent 수를 초반부터 과하게 늘리는 것
2. 모든 Agent에게 모든 도구 권한을 주는 것
3. 대량 파일 수정을 한 번에 허용하는 것
4. 메모리를 단순 대화 로그 덤프로 만드는 것
5. 검증 없이 완료 처리하는 것
6. Orchestrator 없이 Agent끼리만 대화시키는 것
7. RAG만 믿고 Repo-map/LSP를 만들지 않는 것
8. GUI Backend가 LangGraph 내부 구현에 강하게 종속되는 것
9. 위험한 shell/git 명령을 자동 승인하는 것
10. Work Package 없이 바로 구현에 들어가는 것
```

---

## 23. 최종 제품 정체성

Artemis는 다음 정체성을 가진다.

```text
Artemis is a personal AI development organization simulator.

It helps a solo developer operate large-scale projects by providing:
- virtual project management
- architecture review
- multi-agent brainstorming
- implementation planning
- code generation
- testing and QA
- risk analysis
- documentation
- long-term project memory
- decision tracking
- safe tool execution
```

한국어 정의:

**Artemis는 개인 개발자가 팀 없이도 대규모 프로젝트를 설계, 구현, 검증, 운영할 수 있도록 돕는 GUI 기반 멀티 Agent 개발 운영 시스템이다.**

---

## 24. 다음 단계

Codex에서 이어갈 다음 작업은 다음 중 하나로 시작하는 것이 좋다.

### 선택지 A — Backend First

가장 추천한다.

1. `services/agent-backend` 생성
2. FastAPI 서버 생성
3. LangGraph root graph 작성
4. ArtemisState 정의
5. Tool Router skeleton 작성
6. Work Package 생성 API 작성

장점:

- 핵심 로직을 빠르게 검증 가능
- GUI 없이도 API 테스트 가능
- LangGraph 흐름을 먼저 안정화 가능

---

### 선택지 B — GUI First

1. Tauri + React 앱 생성
2. Project Dashboard mock 작성
3. Agent Room mock 작성
4. Brainstorming Room mock 작성
5. API mock 연결

장점:

- 제품 감각을 빨리 확인 가능
- 최종 사용 경험을 먼저 설계 가능

단점:

- Agent Backend가 없으면 실제 기능 검증이 늦어짐

---

### 선택지 C — Vertical Slice

작은 기능 하나를 처음부터 끝까지 구현한다.

예시:

**“사용자 요청 → Work Package 생성 → GUI 표시 → 승인 → 완료 보고”**

구성:

1. GUI 입력
2. GUI Backend API
3. Python Agent Backend
4. LangGraph classify + work_package graph
5. 이벤트 스트림
6. GUI 결과 표시

가장 제품적인 접근이다.

---

## 25. 권장 시작 방향

현재 Artemis는 구조가 크기 때문에 **Vertical Slice** 방식이 가장 좋다.

초기 목표:

```text
사용자가 GUI에서 요청을 입력하면,
Python Agent Backend가 LangGraph를 통해 Work Package를 생성하고,
GUI가 이를 실시간 이벤트와 함께 표시한다.
```

이것이 성공하면 이후 기능은 자연스럽게 확장할 수 있다.

확장 순서:

```text
[완료] Work Package
→ [완료] GUI + Event Stream
→ [완료] Implementation Pipeline / Diff & Review
→ [완료] Brainstorming Room
→ [완료] Memory
→ [완료] Risk Radar / Quality Center
→ [다음 후보] Deep Architecture Map
→ [다음 후보] Release Readiness / Collaboration
```

---

## 26. 핵심 요약

Artemis는 단순 코딩 보조 AI가 아니라, 개인 개발자의 부족한 팀 역량을 보완하는 시스템이다.

핵심 구조:

```text
GUI Client
→ GUI Backend / Control Plane
→ Python Agent Backend / Intelligence Plane
→ LangGraph Orchestrator
→ LangChain Tools/RAG/Models
→ Local Observability
→ Project Runtime
```

핵심 기능:

```text
- Project Command Center
- Brainstorming Room
- Virtual Team System
- Work Package System
- Architecture Review Board
- Implementation Pipeline
- Review & QA System
- Risk Radar
- Meeting Simulator
- Project Memory
```

핵심 원칙:

```text
- 바로 구현하지 말고 Work Package로 구조화한다.
- Agent는 역할과 권한을 분리한다.
- 모든 위험 작업은 승인받는다.
- 모든 중요한 결정은 Memory에 남긴다.
- 모든 구현은 테스트와 리뷰를 거친다.
- Artemis는 개인용 개발 조직이어야 한다.
```
