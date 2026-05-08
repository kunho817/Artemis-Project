"""FastAPI public API for Control Plane."""

from __future__ import annotations

from pathlib import Path
from typing import Any

from .service import ControlPlaneService
from .storage import SQLiteStore


def create_app(db_path: str = "data/artemis.db") -> object:
    try:
        from fastapi import FastAPI
    except ImportError as exc:
        raise RuntimeError("fastapi is required to run the Control Plane API") from exc

    app = FastAPI(title="Artemis Control Plane", version="0.1.0")
    store = SQLiteStore(db_path)
    service = ControlPlaneService(store)
    cache: dict[str, dict[str, Any]] = {}

    @app.post("/api/projects/open")
    def open_project(payload: dict[str, str]) -> dict[str, Any]:
        project = service.open_project(
            name=payload["name"],
            root_path=payload.get("root_path", str(Path.cwd())),
        )
        cache[project["id"]] = project
        return project

    @app.post("/api/sessions")
    def create_session(payload: dict[str, str]) -> dict[str, Any]:
        return service.create_session(project_id=payload["project_id"], title=payload["title"])

    @app.post("/api/work-packages/from-request")
    def from_request(payload: dict[str, str]) -> dict[str, Any]:
        project = cache[payload["project_id"]]
        session = {"id": payload["session_id"], "project_id": payload["project_id"]}
        return service.create_work_package_from_request(
            project=project,
            session=session,
            user_request=payload["user_request"],
        )

    @app.get("/api/work-packages/{work_package_id}")
    def get_work_package(work_package_id: str) -> dict[str, Any]:
        return store.get_work_package(work_package_id)

    @app.get("/api/agent-runs/{agent_run_id}")
    def get_agent_run(agent_run_id: str) -> dict[str, Any]:
        return store.get_agent_run(agent_run_id)

    @app.get("/api/agent-runs/{agent_run_id}/events")
    def get_events(agent_run_id: str) -> list[dict[str, Any]]:
        return store.list_events(agent_run_id)

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
