"""Artemis service packages.

Importing this package loads the project-level ``.env`` file when present.
The loader never overrides values that are already set by the parent process.
"""

from __future__ import annotations

from pathlib import Path
import os


PROJECT_ROOT = Path(__file__).resolve().parents[1]


def load_project_env(env_path: str | Path | None = None) -> bool:
    """Load root ``.env`` values without exposing secrets in process output."""

    path = Path(env_path) if env_path is not None else PROJECT_ROOT / ".env"
    if not path.exists():
        return False

    try:
        from dotenv import load_dotenv
    except ImportError:
        return _load_project_env_fallback(path)

    return bool(load_dotenv(path, override=False))


def _load_project_env_fallback(path: Path) -> bool:
    loaded = False
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            continue

        key, value = line.split("=", 1)
        key = key.strip()
        if not key or key in os.environ:
            continue

        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {"'", '"'}:
            value = value[1:-1]
        os.environ[key] = value
        loaded = True
    return loaded


load_project_env()
