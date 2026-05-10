"""Run the documented Alpha 0.1 verification matrix."""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import shutil
import subprocess
import sys
from typing import NamedTuple


ROOT = Path(__file__).resolve().parents[1]
GUI_DIR = ROOT / "apps" / "gui"


class Check(NamedTuple):
    name: str
    command: list[str]
    cwd: Path = ROOT


def npm_command() -> str:
    executable = shutil.which("npm.cmd") or shutil.which("npm")
    if executable is None:
        raise RuntimeError("npm was not found on PATH")
    return executable


def build_matrix(profile: str) -> list[Check]:
    python = sys.executable
    npm = npm_command()
    checks = [
        Check("compileall", [python, "-m", "compileall", "services", "tests", "scripts"]),
        Check("unittest", [python, "-m", "unittest", "discover", "-s", "tests"]),
        Check("fastapi-smoke", [python, "scripts/smoke_api.py"]),
        Check("gui-build", [npm, "run", "build"], GUI_DIR),
        Check("npm-audit", [npm, "audit", "--omit=dev"], GUI_DIR),
        Check("alpha-dogfood-smoke", [python, "scripts/smoke_alpha_dogfood.py"]),
    ]
    if profile == "full":
        checks.extend(
            [
                Check("mvp2-gui-smoke", [python, "scripts/smoke_mvp2_gui.py"]),
                Check("mvp3-gui-smoke", [python, "scripts/smoke_mvp3_gui.py"]),
                Check("mvp4-gui-smoke", [python, "scripts/smoke_mvp4_gui.py"]),
                Check("mvp5-gui-smoke", [python, "scripts/smoke_mvp5_gui.py"]),
                Check("mvp6-gui-smoke", [python, "scripts/smoke_mvp6_gui.py"]),
            ]
        )
    return checks


def run_check(check: Check) -> None:
    print(f"==> {check.name}", flush=True)
    completed = subprocess.run(
        check.command,
        cwd=check.cwd,
        check=False,
        env=os.environ.copy(),
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    if completed.returncode != 0:
        raise RuntimeError(f"{check.name} failed with exit code {completed.returncode}")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", choices=["quick", "full"], default="quick")
    args = parser.parse_args()

    try:
        for check in build_matrix(args.profile):
            run_check(check)
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print(f"Alpha verification ({args.profile}) passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
