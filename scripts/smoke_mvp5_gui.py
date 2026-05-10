"""Run the MVP 5 GUI e2e smoke test with an isolated writable project."""

from __future__ import annotations

import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
from typing import Any
from urllib import request as urllib_request


ROOT = Path(__file__).resolve().parents[1]
GUI_DIR = ROOT / "apps" / "gui"


def find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def wait_for_url(url: str, timeout_seconds: float = 20.0) -> None:
    deadline = time.monotonic() + timeout_seconds
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with urllib_request.urlopen(url, timeout=5) as response:
                if response.status < 500:
                    return
        except Exception as exc:  # pragma: no cover - diagnostic wait path
            last_error = exc
        time.sleep(0.25)
    raise RuntimeError(f"Timed out waiting for {url}: {last_error}")


def create_e2e_project(root: Path) -> Path:
    project_root = root / "project"
    project_root.mkdir()
    (project_root / "README.md").write_text("Artemis MVP 5 GUI smoke project\n", encoding="utf-8")
    docs_dir = project_root / "docs"
    docs_dir.mkdir()
    (docs_dir / "artemis_mvp5.md").write_text(
        "Memory and Decision Log\n",
        encoding="utf-8",
    )
    return project_root


def run() -> dict[str, Any]:
    agent_port = find_free_port()
    control_port = find_free_port()
    gui_port = find_free_port()
    control_url = f"http://127.0.0.1:{control_port}"
    agent_url = f"http://127.0.0.1:{agent_port}"
    gui_url = f"http://127.0.0.1:{gui_port}"

    env = os.environ.copy()
    env["ARTEMIS_AGENT_BACKEND_URL"] = agent_url
    env["ARTEMIS_CONTROL_PLANE_ALLOW_ORIGINS"] = gui_url
    env["VITE_CONTROL_PLANE_URL"] = control_url
    env["ARTEMIS_GUI_PORT"] = str(gui_port)
    env["ARTEMIS_GUI_URL"] = gui_url
    env["ZAI_API_KEY"] = ""
    env["ZHIPU_API_KEY"] = ""
    env["GLM_API_KEY"] = ""

    with tempfile.TemporaryDirectory(prefix="artemis-mvp5-gui-") as tmp_dir:
        tmp_root = Path(tmp_dir)
        project_root = create_e2e_project(tmp_root)
        env["ARTEMIS_E2E_PROJECT_ROOT"] = str(project_root)
        db_path = str(tmp_root / "artemis.db")
        backend = subprocess.Popen(
            [
                sys.executable,
                str(ROOT / "scripts" / "start_mvp2_services.py"),
                "--agent-port",
                str(agent_port),
                "--control-port",
                str(control_port),
                "--db-path",
                db_path,
            ],
            cwd=ROOT,
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        try:
            wait_for_url(f"{agent_url}/internal/health")
            wait_for_url(f"{control_url}/api/health")
            completed = subprocess.run(
                ["npm.cmd", "run", "test:e2e", "--", "tests/mvp5-smoke.spec.ts"],
                cwd=GUI_DIR,
                env=env,
                check=False,
                capture_output=True,
                text=True,
                encoding="utf-8",
                errors="replace",
                timeout=180,
            )
            if completed.returncode != 0:
                raise RuntimeError(
                    "GUI e2e smoke failed\n"
                    f"STDOUT:\n{completed.stdout}\n"
                    f"STDERR:\n{completed.stderr}"
                )
            return {
                "status": "ok",
                "agent_backend_url": agent_url,
                "control_plane_url": control_url,
                "gui_url": gui_url,
                "project_root": str(project_root),
                "stdout": completed.stdout or "",
            }
        finally:
            backend.terminate()
            try:
                backend.wait(timeout=10)
            except subprocess.TimeoutExpired:
                backend.kill()


def main() -> int:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    if hasattr(sys.stderr, "reconfigure"):
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    try:
        result = run()
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print(result["stdout"])
    print(f"GUI smoke: {result['status']}")
    print(f"Control Plane: {result['control_plane_url']}")
    print(f"Agent Backend: {result['agent_backend_url']}")
    print(f"GUI: {result['gui_url']}")
    print(f"Project Root: {result['project_root']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
