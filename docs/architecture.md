# Architecture

Artemis is a local-first development operations system built from a Control
Plane, an Agent Backend, and a React GUI.

## Runtime Shape

```text
React GUI
  -> Control Plane FastAPI
     -> SQLite / JSONL state
     -> Agent Backend HTTP API
        -> LangGraph / LangChain
        -> read-only project tools
```

## Responsibilities

### Control Plane

The Control Plane owns canonical state:

- Projects and sessions
- AgentRun and WorkPackage records
- ApprovalRequest state
- ImplementationRun, PatchSet, VerificationRun, ReviewResult
- BrainstormingSession, DecisionBrief, DecisionRecord
- ProjectMemoryItem and selected memory context
- RiskScanRun, RiskFinding, QualitySignal, ProjectHealthSnapshot
- Local trace summaries, artifacts, and event logs
- Command Center aggregation
- SQLite schema version and migration metadata

The Control Plane does not reason, prompt models directly, or own agent
workflow logic.

### Agent Backend

The Agent Backend owns structured candidate generation:

- Intent classification
- Read-only context collection
- WorkPackageDraft generation
- Brainstorming contributions and decision brief drafts
- ImplementationPlan and PatchSet proposal drafts
- MemoryCandidate drafts
- Risk analysis candidates
- ReviewResult drafts

The Agent Backend does not persist canonical product state. Model outputs are
treated as untrusted candidates until Control Plane validation and approval
gates pass.

### GUI

The GUI calls the Control Plane only. It surfaces:

- Command Center
- Work Package request and approval flow
- Event, trace, and artifact views
- Brainstorming Room
- Implementation Pipeline and Diff Viewer
- Memory / Decision Log
- Risk Radar / Quality Center

## Alpha Data Flow

```text
RiskFinding or user request
  -> WorkPackage
  -> approval
  -> ImplementationRun
  -> ImplementationPlan
  -> PatchSet
  -> patch approval
  -> apply
  -> VerificationRun
  -> ReviewResult
  -> Memory
  -> RiskScan rescan
```

Every step emits events and, where applicable, a local trace summary. The
Command Center reads the accumulated state and recommends the next visible
action without triggering hidden automation.

## Storage

Primary storage is SQLite. Events are also appended to JSONL for chronological
inspection.

Alpha schema state is visible through:

```http
GET /api/storage/schema
```

The first migration records the MVP 1-6 schema as the Alpha baseline. Future
schema changes should be idempotent and recorded in `schema_migrations`.

## Observability

Artemis local trace storage is the default. LangSmith self-hosted or Cloud
tracing can be enabled explicitly with environment variables, but it is not an
Alpha dependency.

Trace correlation keys:

- `project_id`
- `session_id`
- `agent_run_id` or run-specific id
- `trace_id`
- optional `external_trace_id`

## Safety Boundaries

- No patch apply without PatchSet approval.
- No ImplementationRun without approved WorkPackage.
- No hidden memory injection.
- No automatic git commit or push.
- No automatic package install or deployment.
- No external scanner or CI provider is part of the default local path.
