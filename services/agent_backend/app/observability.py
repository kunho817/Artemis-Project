"""LangSmith trace correlation helpers.

The MVP does not require LangSmith network calls in tests. It still creates a
stable correlation id that Control Plane can store next to AgentRun state.
"""

from __future__ import annotations

from dataclasses import dataclass
import os
import uuid


@dataclass(frozen=True)
class TraceContext:
    trace_id: str
    project_name: str
    enabled: bool


class LangSmithTracer:
    def __init__(self, env: dict[str, str] | None = None) -> None:
        self._env = env if env is not None else os.environ

    def start_trace(self, *, project_id: str, session_id: str, agent_run_id: str) -> TraceContext:
        enabled = self._env.get("LANGSMITH_TRACING", "false").lower() in {"1", "true", "yes"}
        project_name = self._env.get("LANGSMITH_PROJECT", "artemis-mvp1")
        prefix = "ls" if enabled else "local"
        trace_id = f"{prefix}_{project_id}_{session_id}_{agent_run_id}_{uuid.uuid4().hex[:12]}"
        return TraceContext(trace_id=trace_id, project_name=project_name, enabled=enabled)
