# Artemis

Artemis is a local-first AI development operations tool. The current codebase is the
new Python/FastAPI/React foundation, not the legacy Go TUI. The old Go
implementation is preserved on the `legacy/go-tui` branch for reference only.

## Current Baseline

MVP 1 through MVP 6 are treated as the baseline for Alpha 0.1:

```text
MVP 1: Work Package backend foundation
MVP 2: React GUI + event stream
MVP 3: ImplementationRun, PatchSet, verification, review
MVP 4: Brainstorming Room + Decision Record
MVP 5: Memory / Decision Log
MVP 6: Risk Radar / Quality Center
Alpha: stabilization, dogfooding, schema versioning, Command Center, verification matrix
```

The default operating model is explicit and local:

- Control Plane owns canonical product state, approvals, events, artifacts, and local storage.
- Agent Backend returns structured candidates and does not own canonical product state.
- Work Package and PatchSet execution still require approval gates.
- Local trace storage is the default observability path.
- LangSmith self-hosted or Cloud tracing is opt-in.
- Hidden memory injection, hidden patch apply, automatic git push, package install, and deployment are out of scope.

## Layout

```text
apps/gui/                  React/Vite GUI
services/control_plane/    FastAPI product state and orchestration API
services/agent_backend/    LangGraph/LangChain intelligence plane
scripts/                   Startup, smoke, and verification scripts
tests/contract/            Contract tests for MVP and Alpha behavior
docs/                      Architecture, plans, runbooks, and verification notes
```

## Setup

Use the project virtual environment:

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install fastapi annotated-doc uvicorn langchain langchain-openai langgraph langsmith pydantic python-dotenv httpx
```

Install GUI dependencies:

```powershell
cd apps\gui
npm install
```

Copy `.env.example` to `.env` when you want live GLM calls:

```powershell
Copy-Item .env.example .env
```

`ZAI_API_KEY` enables GLM Coding Plan structured output. Without it, Artemis uses explicit deterministic fallback paths that are marked in events and traces.

## Run Locally

Start Agent Backend and Control Plane:

```powershell
.\.venv\Scripts\python.exe scripts\start_mvp2_services.py
```

Start the GUI in another shell:

```powershell
cd apps\gui
npm run dev
```

Defaults:

- Control Plane: `http://127.0.0.1:8000`
- Agent Backend: `http://127.0.0.1:8765`
- GUI: `http://127.0.0.1:5173`

## Verification

Quick Alpha matrix:

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile quick
```

Full Alpha matrix:

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile full
```

Individual checks:

```powershell
.\.venv\Scripts\python.exe -m compileall services tests scripts
.\.venv\Scripts\python.exe -m unittest discover -s tests
.\.venv\Scripts\python.exe scripts\smoke_api.py
.\.venv\Scripts\python.exe scripts\smoke_alpha_dogfood.py
cd apps\gui
npm run build
npm audit --omit=dev
```

GUI smoke scripts are available at `scripts/smoke_mvp2_gui.py` through
`scripts/smoke_mvp6_gui.py`.

## GLM Model Routing

Default Coding Plan endpoint:

```text
https://api.z.ai/api/coding/paas/v4
```

Supported profiles:

- `glm-5.1`
- `glm-5`
- `glm-4.7`
- `glm-4.6`
- `glm-4.5`
- `glm-4.5-air`
- `glm-4.5-flash`

Role-level overrides use `ARTEMIS_GLM_MODEL_<ROLE>`, for example:

```powershell
$env:ARTEMIS_GLM_MODEL_ARCHITECT="glm-5.1"
$env:ARTEMIS_GLM_MODEL_CONTEXT_COLLECTOR="glm-4.7"
```
