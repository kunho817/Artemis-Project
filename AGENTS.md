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

### Pending

- [ ] Replace `langsmith_trace_id` naming with neutral `trace_id` / `external_trace_id` terminology.
- [ ] Implement a first-class local trace store and viewer path for MVP 2.
- [ ] Replace deterministic Work Package fallback with LLM-generated structured output where appropriate.
- [ ] Add persistent service startup scripts.
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

## Session Rules

At session start:

1. Read `AGENTS.md`.
2. Check current branch and dirty worktree state.
3. Review `docs/artemis_mvp1.md`.

At meaningful completion:

1. Update `AGENTS.md`.
2. Run relevant tests.
3. Commit and push to `origin/main` unless the user asks otherwise.
