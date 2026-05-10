"""Run an end-to-end FastAPI smoke test for Artemis MVP 1.

This script starts both local API apps with uvicorn, sends a public Control
Plane request, verifies that the request crosses the HTTP Agent Backend
boundary, and approves the resulting work package.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import socket
import sys
import tempfile
import threading
import time
from typing import Any
from urllib import error as urllib_error
from urllib import request as urllib_request


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def require_runtime() -> Any:
    failures: list[str] = []
    try:
        import fastapi  # noqa: F401
    except Exception as exc:  # pragma: no cover - diagnostic path
        failures.append(f"fastapi import failed: {type(exc).__name__}: {exc}")
    try:
        import uvicorn
    except Exception as exc:  # pragma: no cover - diagnostic path
        failures.append(f"uvicorn import failed: {type(exc).__name__}: {exc}")
        uvicorn = None

    if failures:
        raise RuntimeError(
            "FastAPI smoke cannot run because the Python API runtime is unavailable.\n"
            + "\n".join(f"- {failure}" for failure in failures)
        )
    return uvicorn


def find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def request_json(method: str, url: str, payload: dict[str, Any] | None = None) -> Any:
    body = json.dumps(payload).encode("utf-8") if payload is not None else None
    request = urllib_request.Request(
        url,
        data=body,
        headers={"Content-Type": "application/json"},
        method=method,
    )
    with urllib_request.urlopen(request, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))


def wait_for_json(url: str, timeout_seconds: float = 15.0) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            request_json("GET", url)
            return
        except (OSError, urllib_error.URLError) as exc:
            last_error = exc
            time.sleep(0.1)
    raise RuntimeError(f"Timed out waiting for {url}: {last_error}")


def start_server(uvicorn: Any, app: Any, port: int, health_path: str) -> tuple[Any, threading.Thread]:
    config = uvicorn.Config(
        app,
        host="127.0.0.1",
        port=port,
        log_level="warning",
        lifespan="off",
        access_log=False,
    )
    server = uvicorn.Server(config)
    thread = threading.Thread(target=server.run, name=f"uvicorn-{port}", daemon=True)
    thread.start()
    wait_for_json(f"http://127.0.0.1:{port}{health_path}")
    return server, thread


def assert_event(event_types: list[str], expected: str) -> None:
    if expected not in event_types:
        raise AssertionError(f"missing event {expected!r}; got {event_types}")


def run(
    agent_port: int | None = None,
    control_port: int | None = None,
    *,
    live_llm: bool = False,
) -> dict[str, Any]:
    uvicorn = require_runtime()

    if not live_llm:
        os.environ["ZAI_API_KEY"] = ""
        os.environ["ZHIPU_API_KEY"] = ""
        os.environ["GLM_API_KEY"] = ""

    import services
    from services.agent_backend.app.api import create_app as create_agent_app
    from services.control_plane.app.api import create_app as create_control_app

    services.load_project_env(ROOT / ".env")
    agent_port = agent_port or find_free_port()
    control_port = control_port or find_free_port()
    os.environ["ARTEMIS_AGENT_BACKEND_URL"] = f"http://127.0.0.1:{agent_port}"

    with tempfile.TemporaryDirectory(prefix="artemis-api-smoke-") as tmp_dir:
        db_path = str(Path(tmp_dir) / "artemis.db")
        agent_app = create_agent_app()
        control_app = create_control_app(db_path)
        agent_server, agent_thread = start_server(uvicorn, agent_app, agent_port, "/internal/health")
        control_server, control_thread = start_server(uvicorn, control_app, control_port, "/api/health")

        try:
            control_base = f"http://127.0.0.1:{control_port}"
            agent_base = f"http://127.0.0.1:{agent_port}"

            project = request_json(
                "POST",
                f"{control_base}/api/projects/open",
                {"name": "Artemis Smoke", "root_path": str(ROOT)},
            )
            session = request_json(
                "POST",
                f"{control_base}/api/sessions",
                {"project_id": project["id"], "title": "MVP1 API Smoke"},
            )
            work = request_json(
                "POST",
                f"{control_base}/api/work-packages/from-request",
                {
                    "project_id": project["id"],
                    "session_id": session["id"],
                    "user_request": "Validate MVP1 FastAPI API boundary smoke.",
                },
            )

            run_data = request_json("GET", f"{control_base}/api/agent-runs/{work['agent_run_id']}")
            package = request_json("GET", f"{control_base}/api/work-packages/{work['work_package_id']}")
            events = request_json("GET", f"{control_base}/api/agent-runs/{work['agent_run_id']}/events")
            result_view = request_json("GET", f"{control_base}/api/agent-runs/{work['agent_run_id']}/result")
            trace = request_json("GET", f"{control_base}/api/agent-runs/{work['agent_run_id']}/trace")
            artifacts = request_json("GET", f"{control_base}/api/agent-runs/{work['agent_run_id']}/artifacts")
            command_center = request_json(
                "GET",
                f"{control_base}/api/projects/{project['id']}/command-center?session_id={session['id']}",
            )
            schema_status = request_json("GET", f"{control_base}/api/storage/schema")
            backend_events = request_json(
                "GET",
                f"{agent_base}/internal/agent-runs/{work['agent_run_id']}/events",
            )
            approval = request_json("POST", f"{control_base}/api/approvals/{work['approval_id']}/approve")

            event_types = [event["type"] for event in events]
            backend_event_types = [event["type"] for event in backend_events]
            assert run_data["status"] == "completed"
            assert work["status"] == "pending_approval"
            assert package["approval_status"] == "pending"
            assert approval["status"] == "approved"
            assert_event(event_types, "agent_run.graph_runtime")
            assert_event(event_types, "artifact.created")
            assert_event(event_types, "work_package.generation_path")
            assert_event(event_types, "work_package.pending_approval")
            assert_event(event_types, "approval.requested")
            assert_event(backend_event_types, "agent_run.graph_runtime")
            assert command_center["next_action"]["kind"] == "approval"
            assert schema_status["status"] == "ok"
            assert schema_status["migrations"]
            trace_event = next(
                (event for event in backend_events if event["type"] == "trace.linked"),
                None,
            )
            assert result_view["trace"]["trace"]["id"] == work["trace_id"]
            assert trace["trace"]["id"] == work["trace_id"]
            assert len(artifacts) >= 3

            return {
                "status": "ok",
                "agent_backend_url": agent_base,
                "control_plane_url": control_base,
                "agent_run_id": work["agent_run_id"],
                "work_package_id": work["work_package_id"],
                "approval_id": work["approval_id"],
                "trace_id": work["trace_id"],
                "external_trace_id": work["external_trace_id"],
                "trace_event": trace_event["payload"] if trace_event else None,
                "command_center_next_action": command_center["next_action"],
                "schema_status": schema_status,
                "event_types": event_types,
                "backend_event_types": backend_event_types,
            }
        finally:
            control_server.should_exit = True
            agent_server.should_exit = True
            control_thread.join(timeout=5)
            agent_thread.join(timeout=5)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--agent-port", type=int)
    parser.add_argument("--control-port", type=int)
    parser.add_argument(
        "--live-llm",
        action="store_true",
        help="Allow smoke_api to use configured GLM credentials instead of deterministic fallback.",
    )
    args = parser.parse_args()

    try:
        result = run(
            agent_port=args.agent_port,
            control_port=args.control_port,
            live_llm=args.live_llm,
        )
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1

    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
