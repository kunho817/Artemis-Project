"""Read-only tool layer for MVP 1."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
import subprocess


ALLOWED_TOOLS = frozenset({"read_file", "list_files", "grep", "git_status"})
DENIED_TOOLS = frozenset(
    {
        "write_file",
        "patch_file",
        "move_file",
        "delete_file",
        "run_command",
        "run_test",
        "run_build",
        "git_commit",
        "git_reset",
        "external_network_call",
    }
)


class ToolPermissionError(PermissionError):
    pass


@dataclass
class ToolResult:
    tool: str
    ok: bool
    output: str
    metadata: dict[str, object]


class ReadOnlyToolRouter:
    def __init__(self, repository_root: str | Path) -> None:
        self.repository_root = Path(repository_root).resolve()

    def assert_allowed(self, tool_name: str) -> None:
        if tool_name in DENIED_TOOLS or tool_name not in ALLOWED_TOOLS:
            raise ToolPermissionError(f"Tool '{tool_name}' is not allowed in MVP 1")

    def _resolve_project_path(self, relative_path: str | Path) -> Path:
        candidate = (self.repository_root / relative_path).resolve()
        if candidate != self.repository_root and self.repository_root not in candidate.parents:
            raise ToolPermissionError(f"Path escapes repository root: {relative_path}")
        return candidate

    def read_file(self, relative_path: str, max_bytes: int = 64_000) -> ToolResult:
        self.assert_allowed("read_file")
        path = self._resolve_project_path(relative_path)
        if not path.is_file():
            return ToolResult("read_file", False, "", {"path": relative_path, "error": "not_found"})
        data = path.read_bytes()[:max_bytes]
        return ToolResult(
            "read_file",
            True,
            data.decode("utf-8", errors="replace"),
            {"path": relative_path, "truncated": path.stat().st_size > max_bytes},
        )

    def list_files(self, max_files: int = 400) -> ToolResult:
        self.assert_allowed("list_files")
        ignored = {".git", ".venv", "__pycache__", ".pytest_cache"}
        files: list[str] = []
        for path in self.repository_root.rglob("*"):
            if any(part in ignored for part in path.parts):
                continue
            if path.is_file():
                files.append(path.relative_to(self.repository_root).as_posix())
            if len(files) >= max_files:
                break
        return ToolResult("list_files", True, "\n".join(sorted(files)), {"count": len(files)})

    def grep(self, pattern: str, max_matches: int = 50) -> ToolResult:
        self.assert_allowed("grep")
        pattern_lower = pattern.lower()
        matches: list[str] = []
        for path in self.repository_root.rglob("*"):
            if not path.is_file() or ".git" in path.parts:
                continue
            try:
                text = path.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            for line_number, line in enumerate(text.splitlines(), start=1):
                if pattern_lower in line.lower():
                    rel = path.relative_to(self.repository_root).as_posix()
                    matches.append(f"{rel}:{line_number}:{line.strip()}")
                    if len(matches) >= max_matches:
                        return ToolResult("grep", True, "\n".join(matches), {"count": len(matches)})
        return ToolResult("grep", True, "\n".join(matches), {"count": len(matches)})

    def git_status(self) -> ToolResult:
        self.assert_allowed("git_status")
        try:
            completed = subprocess.run(
                ["git", "-C", str(self.repository_root), "status", "--short"],
                check=False,
                capture_output=True,
                text=True,
                timeout=10,
            )
        except (OSError, subprocess.TimeoutExpired) as exc:
            return ToolResult("git_status", False, str(exc), {"returncode": None})

        return ToolResult(
            "git_status",
            completed.returncode == 0,
            completed.stdout.strip(),
            {"returncode": completed.returncode, "stderr": completed.stderr.strip()},
        )
