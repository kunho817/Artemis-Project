"""FastAPI public API for Control Plane."""

import asyncio
import json
from pathlib import Path
from typing import Any

from .service import ControlPlaneService
from .storage import SQLiteStore


TERMINAL_AGENT_RUN_STATES = {"completed", "failed", "canceled"}


def create_app(db_path: str = "data/artemis.db", agent_backend: Any | None = None) -> object:
    try:
        from fastapi import BackgroundTasks, FastAPI
        from fastapi.middleware.cors import CORSMiddleware
        from fastapi.responses import StreamingResponse
    except ImportError as exc:
        raise RuntimeError("fastapi is required to run the Control Plane API") from exc

    app = FastAPI(title="Artemis Control Plane", version="0.2.0")
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=False,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    store = SQLiteStore(db_path)
    service = ControlPlaneService(store, agent_backend=agent_backend)

    @app.post("/api/projects/open")
    def open_project(payload: dict[str, str]) -> dict[str, Any]:
        return service.open_project(
            name=payload["name"],
            root_path=payload.get("root_path", str(Path.cwd())),
        )

    @app.get("/api/projects")
    def list_projects() -> list[dict[str, Any]]:
        return store.list_projects()

    @app.get("/api/projects/{project_id}")
    def get_project(project_id: str) -> dict[str, Any]:
        return store.get_project(project_id)

    @app.post("/api/sessions")
    def create_session(payload: dict[str, str]) -> dict[str, Any]:
        return service.create_session(project_id=payload["project_id"], title=payload["title"])

    @app.get("/api/sessions")
    def list_sessions(project_id: str | None = None) -> list[dict[str, Any]]:
        return store.list_sessions(project_id=project_id)

    @app.get("/api/sessions/{session_id}")
    def get_session(session_id: str) -> dict[str, Any]:
        return store.get_session(session_id)

    @app.post("/api/work-packages/from-request")
    def from_request(payload: dict[str, str]) -> dict[str, Any]:
        project = store.get_project(payload["project_id"])
        session = store.get_session(payload["session_id"])
        return service.create_work_package_from_request(
            project=project,
            session=session,
            user_request=payload["user_request"],
        )

    @app.post("/api/work-package-requests")
    def create_work_package_request(
        payload: dict[str, str],
        background_tasks: BackgroundTasks,
    ) -> dict[str, Any]:
        project = store.get_project(payload["project_id"])
        session = store.get_session(payload["session_id"])
        queued = service.start_work_package_request(
            project=project,
            session=session,
            user_request=payload["user_request"],
        )
        background_tasks.add_task(
            service.execute_work_package_request,
            project=project,
            session=session,
            agent_run_id=queued["agent_run_id"],
            user_request=payload["user_request"],
        )
        return queued

    @app.get("/api/work-packages/{work_package_id}")
    def get_work_package(work_package_id: str) -> dict[str, Any]:
        return store.get_work_package(work_package_id)

    @app.get("/api/agent-runs/{agent_run_id}")
    def get_agent_run(agent_run_id: str) -> dict[str, Any]:
        return store.get_agent_run(agent_run_id)

    @app.get("/api/agent-runs/{agent_run_id}/result")
    def get_agent_run_result(agent_run_id: str) -> dict[str, Any]:
        return service.get_agent_run_result(agent_run_id)

    @app.get("/api/agent-runs/{agent_run_id}/events")
    def get_events(agent_run_id: str, after: str | None = None) -> list[dict[str, Any]]:
        return store.list_events(agent_run_id, after=after)

    @app.get("/api/agent-runs/{agent_run_id}/events/stream")
    def stream_events(agent_run_id: str) -> object:
        async def event_generator() -> Any:
            last_event_id: str | None = None
            while True:
                events = store.list_events(agent_run_id, after=last_event_id)
                for event in events:
                    last_event_id = event["id"]
                    yield (
                        f"id: {event['id']}\n"
                        f"event: {event['type']}\n"
                        f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
                    )

                try:
                    run = store.get_agent_run(agent_run_id)
                except KeyError:
                    run = None
                if run is not None and run["status"] in TERMINAL_AGENT_RUN_STATES and not events:
                    break
                await asyncio.sleep(0.5)

        return StreamingResponse(event_generator(), media_type="text/event-stream")

    @app.get("/api/agent-runs/{agent_run_id}/trace")
    def get_agent_run_trace(agent_run_id: str) -> dict[str, Any]:
        return store.get_trace_summary(agent_run_id)

    @app.get("/api/agent-runs/{agent_run_id}/artifacts")
    def get_agent_run_artifacts(agent_run_id: str) -> list[dict[str, Any]]:
        return store.list_artifacts(agent_run_id)

    @app.post("/api/approvals/{approval_id}/approve")
    def approve(approval_id: str) -> dict[str, Any]:
        return service.resolve_approval(approval_id=approval_id, status="approved")

    @app.post("/api/approvals/{approval_id}/reject")
    def reject(approval_id: str) -> dict[str, Any]:
        return service.resolve_approval(approval_id=approval_id, status="rejected")

    @app.get("/api/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    return app
