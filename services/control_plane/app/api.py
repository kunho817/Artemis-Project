"""FastAPI public API for Control Plane."""

import asyncio
import json
import os
from pathlib import Path
from typing import Any

from .service import ControlPlaneService
from .storage import SQLiteStore


TERMINAL_AGENT_RUN_STATES = {"completed", "failed", "canceled"}
TERMINAL_IMPLEMENTATION_RUN_STATES = {"completed", "failed", "canceled"}
TERMINAL_BRAINSTORMING_STATES = {
    "awaiting_decision",
    "accepted",
    "rejected",
    "converted",
    "failed",
    "canceled",
}


def create_app(db_path: str = "data/artemis.db", agent_backend: Any | None = None) -> object:
    try:
        from fastapi import BackgroundTasks, FastAPI
        from fastapi.middleware.cors import CORSMiddleware
        from fastapi.responses import StreamingResponse
    except ImportError as exc:
        raise RuntimeError("fastapi is required to run the Control Plane API") from exc

    app = FastAPI(title="Artemis Control Plane", version="0.3.0")
    allow_origins = [
        origin.strip()
        for origin in os.environ.get(
            "ARTEMIS_CONTROL_PLANE_ALLOW_ORIGINS",
            "http://127.0.0.1:5173,http://localhost:5173",
        ).split(",")
        if origin.strip()
    ]
    app.add_middleware(
        CORSMiddleware,
        allow_origins=allow_origins,
        allow_origin_regex=r"^http://(127\.0\.0\.1|localhost):\d+$",
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

    @app.post("/api/implementation-runs")
    def create_implementation_run(payload: dict[str, str]) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            return service.create_implementation_run(work_package_id=payload["work_package_id"])
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.get("/api/implementation-runs/{implementation_run_id}")
    def get_implementation_run(implementation_run_id: str) -> dict[str, Any]:
        return store.get_implementation_run(implementation_run_id)

    @app.get("/api/implementation-runs/{implementation_run_id}/result")
    def get_implementation_run_result(implementation_run_id: str) -> dict[str, Any]:
        return service.get_implementation_run_result(implementation_run_id)

    @app.get("/api/implementation-runs/{implementation_run_id}/events")
    def get_implementation_events(
        implementation_run_id: str,
        after: str | None = None,
    ) -> list[dict[str, Any]]:
        return store.list_events(implementation_run_id, after=after)

    @app.get("/api/implementation-runs/{implementation_run_id}/events/stream")
    def stream_implementation_events(implementation_run_id: str) -> object:
        async def event_generator() -> Any:
            last_event_id: str | None = None
            while True:
                events = store.list_events(implementation_run_id, after=last_event_id)
                for event in events:
                    last_event_id = event["id"]
                    yield (
                        f"id: {event['id']}\n"
                        f"event: {event['type']}\n"
                        f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
                    )

                try:
                    run = store.get_implementation_run(implementation_run_id)
                except KeyError:
                    run = None
                if run is not None and run["status"] in TERMINAL_IMPLEMENTATION_RUN_STATES and not events:
                    break
                await asyncio.sleep(0.5)

        return StreamingResponse(event_generator(), media_type="text/event-stream")

    @app.post("/api/implementation-runs/{implementation_run_id}/cancel")
    def cancel_implementation_run(implementation_run_id: str) -> dict[str, Any]:
        run = store.get_implementation_run(implementation_run_id)
        store.update_implementation_run(
            implementation_run_id,
            status="canceled",
            current_phase="canceled",
        )
        store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=implementation_run_id,
            event_type="implementation_run.canceled",
            payload={"implementation_run_id": implementation_run_id},
        )
        return store.get_implementation_run(implementation_run_id)

    @app.get("/api/implementation-runs/{implementation_run_id}/patch-set")
    def get_implementation_patch_set(implementation_run_id: str) -> dict[str, Any]:
        patch_set = store.get_patch_set_by_implementation_run(implementation_run_id)
        if patch_set is None:
            from fastapi import HTTPException

            raise HTTPException(status_code=404, detail="PatchSet not found")
        return patch_set

    @app.get("/api/patch-sets/{patch_set_id}")
    def get_patch_set(patch_set_id: str) -> dict[str, Any]:
        return store.get_patch_set(patch_set_id)

    @app.post("/api/patch-sets/{patch_set_id}/approve")
    def approve_patch_set(patch_set_id: str) -> dict[str, Any]:
        return service.resolve_patch_set(patch_set_id=patch_set_id, status="approved")

    @app.post("/api/patch-sets/{patch_set_id}/reject")
    def reject_patch_set(patch_set_id: str) -> dict[str, Any]:
        return service.resolve_patch_set(patch_set_id=patch_set_id, status="rejected")

    @app.post("/api/patch-sets/{patch_set_id}/apply")
    def apply_patch_set(patch_set_id: str) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            return service.apply_patch_set(patch_set_id=patch_set_id)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.post("/api/implementation-runs/{implementation_run_id}/verification-runs")
    def create_verification_run(
        implementation_run_id: str,
        payload: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        return service.run_verification(
            implementation_run_id=implementation_run_id,
            command=(payload or {}).get("command"),
        )

    @app.get("/api/implementation-runs/{implementation_run_id}/verification-runs")
    def list_verification_runs(implementation_run_id: str) -> list[dict[str, Any]]:
        return store.list_verification_runs(implementation_run_id)

    @app.get("/api/implementation-runs/{implementation_run_id}/review-result")
    def get_review_result(implementation_run_id: str) -> dict[str, Any] | None:
        return store.get_review_result(implementation_run_id)

    @app.get("/api/implementation-runs/{implementation_run_id}/trace")
    def get_implementation_trace(implementation_run_id: str) -> dict[str, Any]:
        return store.get_trace_summary(implementation_run_id)

    @app.post("/api/brainstorming-sessions")
    def create_brainstorming_session(
        payload: dict[str, Any],
        background_tasks: BackgroundTasks,
    ) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            project = store.get_project(payload["project_id"])
            session = store.get_session(payload["session_id"])
            queued = service.start_brainstorming_session(
                project=project,
                session=session,
                topic=payload["topic"],
                mode=payload.get("mode", "architecture_debate"),
                source_type=payload.get("source_type", "topic"),
                source_id=payload.get("source_id"),
                roles=payload.get("roles") or [],
            )
            background_tasks.add_task(
                service.execute_brainstorming_session,
                project=project,
                session=session,
                brainstorming_session_id=queued["brainstorming_session_id"],
            )
            return queued
        except (KeyError, ValueError) as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.get("/api/brainstorming-sessions/{brainstorming_session_id}")
    def get_brainstorming_session(brainstorming_session_id: str) -> dict[str, Any]:
        return store.get_brainstorming_session(brainstorming_session_id)

    @app.get("/api/brainstorming-sessions/{brainstorming_session_id}/result")
    def get_brainstorming_result(brainstorming_session_id: str) -> dict[str, Any]:
        return service.get_brainstorming_result(brainstorming_session_id)

    @app.get("/api/brainstorming-sessions/{brainstorming_session_id}/events")
    def get_brainstorming_events(
        brainstorming_session_id: str,
        after: str | None = None,
    ) -> list[dict[str, Any]]:
        return store.list_events(brainstorming_session_id, after=after)

    @app.get("/api/brainstorming-sessions/{brainstorming_session_id}/events/stream")
    def stream_brainstorming_events(brainstorming_session_id: str) -> object:
        async def event_generator() -> Any:
            last_event_id: str | None = None
            while True:
                events = store.list_events(brainstorming_session_id, after=last_event_id)
                for event in events:
                    last_event_id = event["id"]
                    yield (
                        f"id: {event['id']}\n"
                        f"event: {event['type']}\n"
                        f"data: {json.dumps(event, ensure_ascii=False)}\n\n"
                    )

                try:
                    run = store.get_brainstorming_session(brainstorming_session_id)
                except KeyError:
                    run = None
                if run is not None and run["status"] in TERMINAL_BRAINSTORMING_STATES and not events:
                    break
                await asyncio.sleep(0.5)

        return StreamingResponse(event_generator(), media_type="text/event-stream")

    @app.post("/api/brainstorming-sessions/{brainstorming_session_id}/cancel")
    def cancel_brainstorming_session(brainstorming_session_id: str) -> dict[str, Any]:
        run = store.get_brainstorming_session(brainstorming_session_id)
        store.update_brainstorming_session(
            brainstorming_session_id,
            status="canceled",
            current_phase="canceled",
        )
        store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=brainstorming_session_id,
            event_type="brainstorming_session.canceled",
            payload={"brainstorming_session_id": brainstorming_session_id},
        )
        return store.get_brainstorming_session(brainstorming_session_id)

    @app.get("/api/brainstorming-sessions/{brainstorming_session_id}/trace")
    def get_brainstorming_trace(brainstorming_session_id: str) -> dict[str, Any]:
        return store.get_trace_summary(brainstorming_session_id)

    @app.post("/api/brainstorming-sessions/{brainstorming_session_id}/decision/accept")
    def accept_decision(
        brainstorming_session_id: str,
        payload: dict[str, str],
    ) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            return service.resolve_decision_brief(
                brainstorming_session_id=brainstorming_session_id,
                decision_brief_id=payload["decision_brief_id"],
                status="accepted",
                note=payload.get("note"),
            )
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.post("/api/brainstorming-sessions/{brainstorming_session_id}/decision/reject")
    def reject_decision(
        brainstorming_session_id: str,
        payload: dict[str, str],
    ) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            return service.resolve_decision_brief(
                brainstorming_session_id=brainstorming_session_id,
                decision_brief_id=payload["decision_brief_id"],
                status="rejected",
                note=payload.get("reason"),
            )
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.get("/api/decision-records/{decision_record_id}")
    def get_decision_record(decision_record_id: str) -> dict[str, Any]:
        return store.get_decision_record(decision_record_id)

    @app.get("/api/projects/{project_id}/decision-records")
    def list_decision_records(project_id: str) -> list[dict[str, Any]]:
        return store.list_decision_records(project_id)

    @app.post("/api/decision-records/{decision_record_id}/convert-to-work-package")
    def convert_decision_record(decision_record_id: str) -> dict[str, Any]:
        from fastapi import HTTPException

        try:
            return service.convert_decision_record_to_work_package(
                decision_record_id=decision_record_id,
            )
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

    @app.get("/api/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    return app
