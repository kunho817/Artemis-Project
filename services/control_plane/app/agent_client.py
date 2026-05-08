"""Agent Backend client adapters.

The Control Plane talks to Agent Backend through this boundary. Production uses
HTTP by default; tests can inject the in-process adapter without changing
Control Plane orchestration logic.
"""

from __future__ import annotations

import json
import os
from typing import Any, Protocol
from urllib import request as urllib_request


class AgentBackendClient(Protocol):
    def run_agent(self, payload: dict[str, str]) -> dict[str, Any]:
        """Run an Agent Backend workflow and return the structured result."""


class HTTPAgentBackendClient:
    def __init__(self, base_url: str | None = None, timeout_seconds: float = 120.0) -> None:
        self.base_url = (base_url or os.environ.get("ARTEMIS_AGENT_BACKEND_URL") or "http://127.0.0.1:8765").rstrip("/")
        self.timeout_seconds = timeout_seconds

    def run_agent(self, payload: dict[str, str]) -> dict[str, Any]:
        body = json.dumps(payload).encode("utf-8")
        request = urllib_request.Request(
            f"{self.base_url}/internal/agent-runs",
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib_request.urlopen(request, timeout=self.timeout_seconds) as response:
            return json.loads(response.read().decode("utf-8"))


class InProcessAgentBackendClient:
    def __init__(self, service: Any | None = None) -> None:
        self._service = service

    def run_agent(self, payload: dict[str, str]) -> dict[str, Any]:
        from services.agent_backend.app.schemas import AgentBackendRequest
        from services.agent_backend.app.service import AgentBackendService

        service = self._service or AgentBackendService()
        return service.run_agent(AgentBackendRequest(**payload)).to_dict()
