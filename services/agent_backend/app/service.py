"""Agent Backend service boundary."""

from __future__ import annotations

from .graph import MVP1GraphRunner
from .schemas import AgentBackendRequest, FinalAgentRunResult


class AgentBackendService:
    def __init__(self, runner: MVP1GraphRunner | None = None) -> None:
        self.runner = runner or MVP1GraphRunner()

    def run_agent(self, request: AgentBackendRequest) -> FinalAgentRunResult:
        return self.runner.run(request)
