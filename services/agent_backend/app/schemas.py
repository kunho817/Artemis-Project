"""Structured schemas returned by the Agent Backend."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any, Literal


Intent = Literal[
    "feature_request",
    "bug_investigation",
    "refactor_request",
    "architecture_question",
    "documentation_request",
    "planning_request",
    "unknown",
]

RiskLevel = Literal["low", "medium", "high", "critical"]


@dataclass
class RiskHint:
    level: RiskLevel
    description: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class IntentResult:
    intent: Intent
    confidence: float
    rationale: str
    model_role: str = "classifier"
    model_name: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ContextSummary:
    repository_root: str
    git_status: str
    files_considered: list[str]
    related_files: list[str]
    summary: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class WorkPackageDraft:
    title: str
    goal: str
    background: str
    scope: list[str]
    out_of_scope: list[str]
    related_files: list[str]
    required_agents: list[str]
    implementation_steps: list[str]
    verification: list[str]
    risks: list[RiskHint]
    approval_required: bool
    completion_criteria: list[str]

    def validate(self) -> list[str]:
        errors: list[str] = []
        scalar_fields = ("title", "goal", "background")
        for field_name in scalar_fields:
            value = getattr(self, field_name)
            if not isinstance(value, str) or not value.strip():
                errors.append(f"{field_name} is required")

        list_fields = (
            "scope",
            "out_of_scope",
            "related_files",
            "required_agents",
            "implementation_steps",
            "verification",
            "completion_criteria",
        )
        for field_name in list_fields:
            value = getattr(self, field_name)
            if not isinstance(value, list) or not value:
                errors.append(f"{field_name} must be a non-empty list")

        if not isinstance(self.approval_required, bool):
            errors.append("approval_required must be a boolean")

        if not self.risks:
            errors.append("risks must be a non-empty list")

        return errors

    def to_dict(self) -> dict[str, Any]:
        data = asdict(self)
        data["risks"] = [risk.to_dict() for risk in self.risks]
        return data


@dataclass
class AgentBackendRequest:
    project_id: str
    session_id: str
    agent_run_id: str
    user_request: str
    project_root: str


@dataclass
class AgentBackendEvent:
    type: str
    payload: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class FinalAgentRunResult:
    status: Literal["completed", "failed"]
    intent_result: IntentResult
    context_summary: ContextSummary
    work_package: WorkPackageDraft | None
    risk_hints: list[RiskHint]
    trace_id: str
    external_trace_id: str | None
    events: list[AgentBackendEvent]
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "intent_result": self.intent_result.to_dict(),
            "context_summary": self.context_summary.to_dict(),
            "work_package": self.work_package.to_dict() if self.work_package else None,
            "risk_hints": [risk.to_dict() for risk in self.risk_hints],
            "trace_id": self.trace_id,
            "external_trace_id": self.external_trace_id,
            "events": [event.to_dict() for event in self.events],
            "errors": list(self.errors),
        }
