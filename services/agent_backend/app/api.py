"""FastAPI surface for the internal Agent Backend API."""

from __future__ import annotations

from .schemas import AgentBackendRequest, ImplementationBackendRequest, ReviewBackendRequest
from .service import AgentBackendService


def create_app() -> object:
    try:
        from fastapi import FastAPI
    except ImportError as exc:
        raise RuntimeError("fastapi is required to run the Agent Backend API") from exc

    app = FastAPI(title="Artemis Agent Backend", version="0.1.0")
    service = AgentBackendService()
    runs: dict[str, dict[str, object]] = {}

    @app.post("/internal/agent-runs")
    def run_agent(payload: dict[str, str]) -> dict[str, object]:
        request = AgentBackendRequest(**payload)
        result = service.run_agent(request).to_dict()
        runs[request.agent_run_id] = result
        return result

    @app.post("/internal/implementation-runs")
    def create_implementation_proposal(payload: dict[str, object]) -> dict[str, object]:
        request = ImplementationBackendRequest(**payload)
        result = service.create_implementation_proposal(request).to_dict()
        runs[request.implementation_run_id] = result
        return result

    @app.post("/internal/review-results")
    def create_review_result(payload: dict[str, object]) -> dict[str, object]:
        request = ReviewBackendRequest(**payload)
        return service.create_review_result(request).to_dict()

    @app.get("/internal/agent-runs/{agent_run_id}")
    def get_agent_run(agent_run_id: str) -> dict[str, object]:
        return runs[agent_run_id]

    @app.get("/internal/agent-runs/{agent_run_id}/events")
    def get_agent_run_events(agent_run_id: str) -> list[dict[str, object]]:
        return list(runs[agent_run_id].get("events", []))

    @app.post("/internal/agent-runs/{agent_run_id}/cancel")
    def cancel_agent_run(agent_run_id: str) -> dict[str, object]:
        run = runs.get(agent_run_id)
        if run is None:
            run = {"status": "canceled", "events": []}
            runs[agent_run_id] = run
        else:
            run["status"] = "canceled"
        return run

    @app.get("/internal/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    return app
