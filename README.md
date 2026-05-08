# Artemis

Artemis is being rebuilt as a backend foundation for a personal AI development organization.

MVP 1 focuses on one vertical slice:

```text
user request
-> Control Plane AgentRun
-> Agent Backend LangGraph-style workflow
-> WorkPackageDraft
-> pending approval state
-> event log
-> LangSmith trace correlation id
```

The old Go TUI implementation is preserved on the `legacy/go-tui` branch.

## MVP 1 Scope

- Control Plane owns product state, approvals, events, and artifacts.
- Agent Backend owns intent classification, read-only context collection, Work Package draft creation, and validation.
- LangGraph is used for the root workflow when installed; a sequential fallback keeps local contract tests dependency-light.
- Tool access is read-only: `read_file`, `list_files`, `grep`, `git_status`.
- Z.AI GLM Coding Plan is the model provider for LangChain-backed calls.
- Deterministic fallback behavior is available when no API key is configured.

## Layout

```text
services/
  agent_backend/       # Intelligence plane
  control_plane/       # Product state and API plane
tests/
  contract/            # MVP 1 contract tests
docs/                  # Planning and design documents
```

## Run Contract Tests

```powershell
python -m unittest discover -s tests
```

For a clean local runtime, use the project virtual environment:

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install fastapi annotated-doc uvicorn langchain langchain-openai langgraph langsmith pydantic python-dotenv httpx
.\.venv\Scripts\python.exe -m unittest discover -s tests
.\.venv\Scripts\python.exe scripts\smoke_api.py
```

To verify the live LangSmith path without changing `.env`, run:

```powershell
$env:LANGSMITH_TRACING="true"; .\.venv\Scripts\python.exe scripts\smoke_api.py
```

## GLM Model Routing

Default Coding Plan endpoint:

```text
https://api.z.ai/api/coding/paas/v4
```

Supported MVP profiles:

- `glm-5.1`
- `glm-5`
- `glm-4.7`
- `glm-4.6`
- `glm-4.5`
- `glm-4.5-air`
- `glm-4.5-flash`

Role-level environment overrides use `ARTEMIS_GLM_MODEL_<ROLE>`, for example:

```powershell
$env:ARTEMIS_GLM_MODEL_ARCHITECT="glm-5.1"
$env:ARTEMIS_GLM_MODEL_CONTEXT_COLLECTOR="glm-4.7"
```
