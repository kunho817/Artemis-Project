# AGENTS.md - Artemis Project

> This file must be checked at the start and updated at the end of meaningful sessions.

## Project Overview

- **Name**: Artemis Project
- **Current direction**: New Artemis backend foundation
- **Legacy implementation**: preserved on `legacy/go-tui`
- **MVP 1 goal**: Convert natural-language user requests into structured Work Packages, with state, events, approval, and trace correlation.
- **Primary stack**: Python, FastAPI, LangGraph, LangChain, local trace observability
- **Model provider**: Z.AI GLM Coding Plan

## Architecture

```text
D:\Artemis_Project\
├── docs/
│   ├── artemis_planning.md
│   ├── artemis_mvp1.md
│   ├── artemis_mvp2.md
│   ├── artemis_mvp3.md
│   ├── artemis_mvp4.md
│   ├── architecture.md
│   ├── configuration.md
│   ├── getting-started.md
│   └── archive/
│       └── oh-my-claude/
│           └── README.md
├── services/
│   ├── control_plane/
│   │   ├── pyproject.toml
│   │   └── app/
│   │       ├── api.py
│   │       ├── models.py
│   │       ├── service.py
│   │       └── storage.py
│   └── agent_backend/
│       ├── pyproject.toml
│       └── app/
│           ├── api.py
│           ├── config.py
│           ├── graph.py
│           ├── llm.py
│           ├── observability.py
│           ├── schemas.py
│           ├── service.py
│           └── tools.py
├── tests/
│   └── contract/
│       └── test_mvp1_contract.py
├── README.md
├── .env.example
├── .gitignore
└── AGENTS.md
```

## MVP 1 Principles

- Control Plane does not reason.
- Agent Backend does not own canonical product state.
- Agent Backend returns structured schema, not raw prose.
- MVP 1 is read-only against user projects.
- Observability and local trace correlation are part of the default execution model.
- LangSmith Cloud is not a default dependency; self-hosted/Cloud endpoints are explicit opt-in integrations.

## GLM Provider Policy

The LangChain-connected model provider is Z.AI GLM Coding Plan.

Default endpoint:

```text
https://api.z.ai/api/coding/paas/v4
```

Supported model profiles:

```text
glm-5.1
glm-5
glm-4.7
glm-4.6
glm-4.5
glm-4.5-air
glm-4.5-flash
```

Default role mapping:

| Role | Default model | Reason |
|------|---------------|--------|
| orchestrator | glm-5.1 | highest planning and synthesis load |
| architect | glm-5.1 | system design and tradeoff analysis |
| planner | glm-5 | planning and decomposition |
| work_package_writer | glm-5 | structured task drafting |
| context_collector | glm-4.7 | retrieval and summarization |
| validator | glm-4.6 | schema and policy validation |
| qa | glm-4.7 | risk and verification hints |

Each mapping can be overridden with `ARTEMIS_GLM_MODEL_<ROLE>`.

## Current Status

### Completed

- [x] `legacy/go-tui` branch pushed with the old Go TUI state preserved.
- [x] Main branch reset toward the new Artemis MVP 1 direction.
- [x] Control Plane and Agent Backend skeletons created.
- [x] Read-only tool layer created.
- [x] GLM model profile and role routing created.
- [x] MVP 1 contract tests created.
- [x] Verification gaps addressed: real LangGraph path when installed, HTTP Agent Backend client boundary, artifact events, and safe git status.
- [x] Project-level `.env` auto-loading added for service imports, with a no-dependency fallback parser.
- [x] FastAPI HTTP boundary smoke script added at `scripts/smoke_api.py`.
- [x] GLM role routing precedence corrected so role defaults are preserved unless a role-specific env override is set.
- [x] Clean `.venv` dependencies installed and verified.
- [x] FastAPI HTTP boundary smoke passes with real uvicorn Agent Backend and Control Plane servers.
- [x] Live GLM Coding Plan call verified through LangChain using the configured API key.
- [x] Live LangSmith trace path verified when `LANGSMITH_TRACING=true` is present in the process environment.
- [x] LangGraph validation failure path is covered by contract tests.
- [x] Observability direction revised to local-first trace storage with LangSmith self-hosted/Cloud as explicit opt-in.
- [x] MVP 2 GUI + Event Stream design document created at `docs/artemis_mvp2.md`.
- [x] `langsmith_trace_id` naming replaced with neutral `trace_id` / `external_trace_id` terminology.
- [x] MVP 2 Control Plane endpoints added for async Work Package requests, event polling/SSE, result, artifacts, and local trace summary.
- [x] First-class local trace summary tables and GUI viewer path added.
- [x] MVP 2 React/Vite/TypeScript GUI skeleton created at `apps/gui`.
- [x] Persistent MVP 2 backend startup script added at `scripts/start_mvp2_services.py`.
- [x] MVP 2 contract coverage added for async API result, event polling fallback, trace summary, and artifacts.
- [x] MVP 2 Playwright GUI e2e smoke added and verified at `scripts/smoke_mvp2_gui.py`.
- [x] MVP 3 Implementation Pipeline design document created at `docs/artemis_mvp3.md`.
- [x] MVP 3 cleanup completed: GUI session resets on project change, Work Package reject approval e2e added, and CORS now uses local/dev policy instead of wildcard.
- [x] MVP 3 Implementation Pipeline vertical slice added: ImplementationRun, ImplementationPlan, PatchSet/Diff Viewer, patch approval/apply, VerificationRun, ReviewResult, implementation event/trace view, backend contracts, and GUI e2e smoke.
- [x] MVP 3 GUI smoke script added at `scripts/smoke_mvp3_gui.py`.
- [x] MVP 3 verification rerun passed: `.venv` compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp3_gui.py`.
- [x] MVP 3 planning-side revalidation passed: implementation pipeline structure, safety policy, backend contracts, FastAPI smoke, GUI build/audit, and GUI e2e smoke were rechecked.
- [x] MVP 4 Brainstorming Room + Decision Record design document created at `docs/artemis_mvp4.md`.
- [x] MVP 4 Brainstorming Room vertical slice added: BrainstormingSession, structured contributions/critiques/options, DecisionBrief accept/reject, DecisionRecord, Work Package conversion, event/trace view, backend contracts, and GUI smoke.
- [x] MVP 4 verification passed: `.venv` compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp4_gui.py`.
- [x] MVP 4 verification rerun passed: `.venv` compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp4_gui.py`.

### Pending

- [ ] Replace deterministic Work Package fallback with LLM-generated structured output where appropriate.
- [ ] Replace deterministic MVP 3 implementation proposal/log patch with LLM-generated structured PatchSet output when policy and review gates are ready.
- [ ] Add real LangGraph checkpointing after MVP 1 contracts stabilize.

## Session History

| Session | Date | Work |
|---------|------|------|
| #40 | 2026-05-08 | New Artemis redesign direction confirmed. MVP 1 design document created. |
| #41 | 2026-05-08 | `legacy/go-tui` branch pushed, old Go TUI removed from main, MVP 1 Python backend foundation started with GLM Coding Plan routing. |
| #42 | 2026-05-08 | MVP 1 verification run recorded. Follow-up patch added real LangGraph execution path, optional live LangSmith trace context, HTTP Agent Backend boundary, artifact events, and safe git status handling. |
| #43 | 2026-05-08 | MVP 1 re-verification run. Contract tests, compile checks, LangGraph runtime event, and safe git status passed; FastAPI API smoke blocked by missing `annotated_doc` in the current Python environment. |
| #44 | 2026-05-08 | `.env` loading wired into service imports, GLM role-routing precedence corrected, HTTP API smoke runner added. Contract tests and compile checks pass, with the LangGraph test skipped because `langchain_core.messages` is missing in the current runtime. FastAPI smoke is still blocked because the global Python runtime imports `annotated_doc`/`anyio` as broken namespace packages and clean `.venv` install is network-blocked in sandbox. |
| #45 | 2026-05-08 | Clean `.venv` dependency install completed. Contract tests, compile checks, FastAPI HTTP smoke, live GLM LangChain call, and live LangSmith trace path all passed under `.venv`; global Python remains unsuitable for API verification. |
| #46 | 2026-05-08 | MVP 1 re-verification found and fixed a validation failure path bug where empty risk hints crashed before schema validation. Added contract coverage; tests, compile checks, HTTP smoke, live GLM call, and live LangSmith trace path passed under `.venv`. |
| #47 | 2026-05-08 | Planning session updated observability direction: LangSmith Cloud is no longer a default because of cost; Artemis local trace store is the default, with self-hosted/Cloud LangSmith only as explicit opt-in. MVP 2 design document created. |
| #48 | 2026-05-08 | MVP 2 foundation slice started. Added Control Plane async Work Package request API, event polling/SSE, result/artifact/trace endpoints, local trace summary storage, neutral trace naming, React/Vite GUI skeleton, backend startup script, and MVP 2 contract coverage. Contract tests, compile checks, FastAPI smoke, GUI build, and npm audit passed. |
| #49 | 2026-05-08 | MVP 2 verification session added Playwright GUI e2e smoke covering project open, session creation, Work Package request, event timeline, trace/artifact tabs, and approval. Full contract, compile, FastAPI smoke, GUI build, npm audit, and GUI e2e smoke passed. |
| #50 | 2026-05-09 | Planning session created MVP 3 design document. MVP 3 scope fixed as approved WorkPackage → ImplementationRun → Implementation Plan → Patch Proposal → Diff Viewer → patch approval/apply → VerificationRun → ReviewResult, with git commit/push, package install, DB migration, deployment, and autonomous retry loop excluded. |
| #51 | 2026-05-09 | MVP 3 implementation session completed the first Implementation Pipeline slice. Added Agent Backend implementation proposal/review contracts, Control Plane storage/API/service support for ImplementationRun/PatchSet/VerificationRun/ReviewResult, policy-gated patch apply and verification command execution, GUI Diff Viewer and implementation timeline, reject approval e2e, local/dev CORS policy, backend contract tests, and MVP 3 GUI smoke. Verification passed: compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp3_gui.py`. |
| #52 | 2026-05-09 | MVP 3 verification rerun completed. Passed `.venv` compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp3_gui.py`; no implementation fixes were needed. |
| #53 | 2026-05-09 | Planning-side MVP 3 revalidation completed. Rechecked `docs/artemis_mvp3.md` completion conditions against Control Plane, Agent Backend, GUI, contract tests, and smoke scripts; compileall, full unittest, FastAPI smoke, GUI build, npm audit, and GUI e2e smoke passed again. |
| #54 | 2026-05-09 | Planning session created MVP 4 design document. MVP 4 scope fixed as Brainstorming Room plus Decision Record: topic/source-based BrainstormingSession, role contributions, cross critique, options/tradeoffs, DecisionBrief accept/reject, accepted DecisionRecord, and optional conversion to a pending-approval WorkPackage, with project file writes, command execution, patch retry loop, and full Memory/RAG excluded. |
| #55 | 2026-05-09 | MVP 4 implementation session completed the Brainstorming Room vertical slice. Added Agent Backend brainstorming execution, Control Plane Brainstorming/Decision storage and APIs, GUI Brainstorming Room, DecisionBrief accept/reject, accepted DecisionRecord to pending-approval Work Package conversion, backend contracts, and `scripts/smoke_mvp4_gui.py`; verification passed with compileall, full unittest, FastAPI smoke, GUI build, npm audit, and MVP 4 GUI e2e smoke. |
| #56 | 2026-05-09 | MVP 4 verification rerun completed. Rechecked Brainstorming Room and Decision Record implementation coverage against `docs/artemis_mvp4.md`; `.venv` compileall, full unittest, FastAPI smoke, GUI build, npm audit, and `scripts/smoke_mvp4_gui.py` passed. |

## Session Rules

At session start:

1. Read `AGENTS.md`.
2. Check current branch and dirty worktree state.
3. Review `docs/artemis_mvp1.md`.

At meaningful completion:

1. Update `AGENTS.md`.
2. Run relevant tests.
3. Commit and push to `origin/main` unless the user asks otherwise.
