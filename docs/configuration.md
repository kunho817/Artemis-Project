# Configuration

Artemis reads project-level `.env` settings during service import/startup. The
default configuration is local-first and does not require cloud tracing.

## GLM Coding Plan

```text
ZAI_API_KEY=
ZAI_BASE_URL=https://api.z.ai/api/coding/paas/v4
ZAI_MODEL=glm-5.1
```

Supported models:

- `glm-5.1`
- `glm-5`
- `glm-4.7`
- `glm-4.6`
- `glm-4.5`
- `glm-4.5-air`
- `glm-4.5-flash`

Role-level overrides:

```text
ARTEMIS_GLM_MODEL_ORCHESTRATOR=glm-5.1
ARTEMIS_GLM_MODEL_ARCHITECT=glm-5.1
ARTEMIS_GLM_MODEL_PLANNER=glm-5
ARTEMIS_GLM_MODEL_WORK_PACKAGE_WRITER=glm-5
ARTEMIS_GLM_MODEL_CONTEXT_COLLECTOR=glm-4.7
ARTEMIS_GLM_MODEL_VALIDATOR=glm-4.6
ARTEMIS_GLM_MODEL_QA=glm-4.7
```

When `ZAI_API_KEY` is present, Work Package generation first attempts GLM
structured JSON output. When the key is absent or the model output is malformed,
the deterministic fallback path is used and the reason is recorded in
`work_package.generation_path`.

## Observability

Local trace storage is the default. LangSmith is an explicit opt-in integration.

```text
LANGSMITH_TRACING=false
LANGSMITH_API_KEY=
LANGSMITH_PROJECT=artemis-alpha
```

Use `LANGSMITH_TRACING=true` only when a self-hosted or Cloud LangSmith endpoint
is intentionally configured for the current process.

## Service URLs

```text
ARTEMIS_AGENT_BACKEND_URL=http://127.0.0.1:8765
ARTEMIS_CONTROL_PLANE_ALLOW_ORIGINS=http://127.0.0.1:5173,http://localhost:5173
VITE_CONTROL_PLANE_URL=http://127.0.0.1:8000
VITE_DEFAULT_PROJECT_ROOT=D:\Artemis_Project
```

`scripts/start_mvp2_services.py` sets `ARTEMIS_AGENT_BACKEND_URL` for the local
Control Plane process when both backend services are started together.

## Storage

The Control Plane uses SQLite plus JSONL events by default:

```text
data/artemis.db
data/artemis.events.jsonl
```

Alpha adds schema migration metadata:

- `schema_migrations`
- `schema_metadata`
- `GET /api/storage/schema`

The current migration is an Alpha baseline marker for the MVP 1-6 schema. New
schema changes should be added as idempotent migrations instead of silent table
changes.

## GUI

The GUI is a Vite app. Runtime Control Plane URL is read from:

```text
VITE_CONTROL_PLANE_URL=http://127.0.0.1:8000
```

For local smoke scripts, the value is injected per process so tests can run on
isolated ports.

## Safety Defaults

These behaviors are intentionally not configurable as hidden automation:

- WorkPackage approval is required before ImplementationRun.
- PatchSet approval is required before apply.
- Selected memory must be explicit request/session context.
- External scanners, CI ingestion, vector DB, git commit/push, package install,
  and deployment are not enabled by default.
