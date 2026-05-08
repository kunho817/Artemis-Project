"""Start local Artemis MVP 2 backend services.

The script runs Agent Backend and Control Plane in one foreground process for
GUI development. Stop it with Ctrl+C.
"""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import sys
import threading
import time
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def require_uvicorn() -> Any:
    try:
        import uvicorn
    except Exception as exc:  # pragma: no cover - startup diagnostic path
        raise RuntimeError(
            "uvicorn is required. Install the project .venv dependencies before starting services."
        ) from exc
    return uvicorn


def start_server(uvicorn: Any, app: Any, host: str, port: int, name: str) -> tuple[Any, threading.Thread]:
    config = uvicorn.Config(
        app,
        host=host,
        port=port,
        log_level="info",
        lifespan="off",
        access_log=False,
    )
    server = uvicorn.Server(config)
    thread = threading.Thread(target=server.run, name=name, daemon=True)
    thread.start()
    return server, thread


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--agent-port", type=int, default=8765)
    parser.add_argument("--control-port", type=int, default=8000)
    parser.add_argument("--db-path", default=str(ROOT / "data" / "artemis.db"))
    args = parser.parse_args()

    uvicorn = require_uvicorn()

    import services
    from services.agent_backend.app.api import create_app as create_agent_app
    from services.control_plane.app.api import create_app as create_control_app

    services.load_project_env(ROOT / ".env")
    os.environ["ARTEMIS_AGENT_BACKEND_URL"] = f"http://{args.host}:{args.agent_port}"

    agent_server, agent_thread = start_server(
        uvicorn,
        create_agent_app(),
        args.host,
        args.agent_port,
        "artemis-agent-backend",
    )
    control_server, control_thread = start_server(
        uvicorn,
        create_control_app(args.db_path),
        args.host,
        args.control_port,
        "artemis-control-plane",
    )

    print(f"Agent Backend: http://{args.host}:{args.agent_port}")
    print(f"Control Plane: http://{args.host}:{args.control_port}")
    print("Press Ctrl+C to stop.")

    try:
        while agent_thread.is_alive() and control_thread.is_alive():
            time.sleep(0.5)
    except KeyboardInterrupt:
        pass
    finally:
        agent_server.should_exit = True
        control_server.should_exit = True
        agent_thread.join(timeout=5)
        control_thread.join(timeout=5)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
