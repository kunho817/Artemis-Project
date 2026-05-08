"""LangSmith trace helpers.

Tests run without network access or LangSmith credentials, so tracing remains
optional. When `LANGSMITH_TRACING=true` and the LangSmith SDK is installed, the
Agent Backend opens a real root run using the trace context manager.
"""

from __future__ import annotations

from contextlib import contextmanager
from dataclasses import dataclass
import os
from typing import Any, Iterator
import uuid


@dataclass(frozen=True)
class TraceContext:
    trace_id: str
    external_trace_id: str | None
    run_id: str
    project_name: str
    enabled: bool
    requested: bool
    api_key_available: bool
    live_tracing_available: bool = False


class LangSmithTracer:
    def __init__(self, env: dict[str, str] | None = None) -> None:
        self._env = env if env is not None else os.environ

    def start_trace(self, *, project_id: str, session_id: str, agent_run_id: str) -> TraceContext:
        requested = self._env.get("LANGSMITH_TRACING", "false").lower() in {"1", "true", "yes"}
        api_key_available = bool(self._env.get("LANGSMITH_API_KEY"))
        enabled = requested and api_key_available
        project_name = self._env.get("LANGSMITH_PROJECT", "artemis-mvp1")
        run_id = str(uuid.uuid4())
        trace_id = f"trace_{uuid.uuid4().hex[:16]}"
        external_trace_id = f"langsmith_{run_id}" if enabled else None
        return TraceContext(
            trace_id=trace_id,
            external_trace_id=external_trace_id,
            run_id=run_id,
            project_name=project_name,
            enabled=enabled,
            requested=requested,
            api_key_available=api_key_available,
            live_tracing_available=self._has_langsmith(),
        )

    @contextmanager
    def trace_run(
        self,
        trace_context: TraceContext,
        *,
        name: str,
        inputs: dict[str, Any],
        metadata: dict[str, Any],
    ) -> Iterator[Any | None]:
        if not trace_context.enabled:
            yield None
            return

        try:
            from langsmith import trace
        except ImportError:
            yield None
            return

        with trace(
            name,
            run_type="chain",
            inputs=inputs,
            project_name=trace_context.project_name,
            tags=["artemis", "mvp1"],
            metadata=metadata,
            run_id=trace_context.run_id,
        ) as run:
            yield run

    def end_trace(self, run: Any | None, *, outputs: dict[str, Any]) -> None:
        if run is not None and hasattr(run, "end"):
            run.end(outputs=outputs)

    def _has_langsmith(self) -> bool:
        try:
            import langsmith  # noqa: F401
        except ImportError:
            return False
        return True
