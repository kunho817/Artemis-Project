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
BrainstormingMode = Literal[
    "free_ideation",
    "architecture_debate",
    "implementation_strategy",
    "risk_review",
    "product_planning",
]
BrainstormingSourceType = Literal["topic", "work_package", "implementation_run", "review_result"]
BrainstormingStance = Literal["supportive", "cautious", "opposed", "exploratory"]


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


@dataclass
class ImplementationBackendRequest:
    project_id: str
    session_id: str
    implementation_run_id: str
    project_root: str
    work_package: dict[str, Any]


@dataclass
class ImplementationPlanDraft:
    goal: str
    context_summary: str
    target_files: list[str]
    steps: list[str]
    verification_strategy: list[str]
    risks: list[dict[str, Any]]

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PatchFileDraft:
    path: str
    operation: Literal["create", "update", "delete"]
    diff: str
    rationale: str
    risk_level: RiskLevel
    replacement_content: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PatchSetDraft:
    summary: str
    risk_level: RiskLevel
    files: list[PatchFileDraft]

    def to_dict(self) -> dict[str, Any]:
        return {
            "summary": self.summary,
            "risk_level": self.risk_level,
            "files": [file.to_dict() for file in self.files],
        }


@dataclass
class ImplementationProposalResult:
    status: Literal["completed", "failed"]
    implementation_plan: ImplementationPlanDraft | None
    patch_set: PatchSetDraft | None
    trace_id: str
    events: list[AgentBackendEvent]
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "implementation_plan": self.implementation_plan.to_dict()
            if self.implementation_plan
            else None,
            "patch_set": self.patch_set.to_dict() if self.patch_set else None,
            "trace_id": self.trace_id,
            "events": [event.to_dict() for event in self.events],
            "errors": list(self.errors),
        }


@dataclass
class ReviewBackendRequest:
    implementation_run_id: str
    work_package: dict[str, Any]
    patch_set: dict[str, Any] | None
    verification_runs: list[dict[str, Any]]


@dataclass
class ReviewResultDraft:
    status: Literal["pass", "needs_changes", "blocked"]
    findings: list[str]
    residual_risks: list[str]
    recommendation: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingBackendRequest:
    project_id: str
    session_id: str
    brainstorming_session_id: str
    project_root: str
    topic: str
    mode: BrainstormingMode
    source_type: BrainstormingSourceType
    source_id: str | None
    roles: list[str]
    source_context: dict[str, Any] = field(default_factory=dict)


@dataclass
class BrainstormingContributionDraft:
    role: str
    stance: BrainstormingStance
    summary: str
    arguments: list[str]
    concerns: list[str]
    suggested_actions: list[str]
    referenced_artifacts: list[str]

    def validate(self) -> list[str]:
        errors: list[str] = []
        if not self.role.strip():
            errors.append("role is required")
        if self.stance not in {"supportive", "cautious", "opposed", "exploratory"}:
            errors.append("stance is invalid")
        for field_name in ("summary",):
            if not getattr(self, field_name).strip():
                errors.append(f"{field_name} is required")
        for field_name in ("arguments", "concerns", "suggested_actions"):
            if not getattr(self, field_name):
                errors.append(f"{field_name} must be non-empty")
        return errors

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingCritiqueDraft:
    critic_role: str
    target_role: str
    weak_assumptions: list[str]
    missing_context: list[str]
    risks: list[str]
    suggested_revisions: list[str]

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingOptionDraft:
    title: str
    summary: str
    benefits: list[str]
    costs: list[str]
    risks: list[str]
    required_work: list[str]
    verification_hint: str
    score: float

    def validate(self) -> list[str]:
        errors: list[str] = []
        if not self.title.strip():
            errors.append("title is required")
        if not self.summary.strip():
            errors.append("summary is required")
        for field_name in ("benefits", "costs", "risks", "required_work"):
            if not getattr(self, field_name):
                errors.append(f"{field_name} must be non-empty")
        if not 0.0 <= self.score <= 1.0:
            errors.append("score must be between 0.0 and 1.0")
        return errors

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class WorkPackageCandidateRequest:
    title: str
    goal: str
    background: str
    scope: list[str]
    out_of_scope: list[str]
    related_files: list[str]
    required_agents: list[str]
    implementation_steps: list[str]
    verification: list[str]
    risks: list[dict[str, Any]]
    completion_criteria: list[str]

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class DecisionBriefDraft:
    recommendation: str
    selected_option_index: int
    rationale: str
    tradeoffs: list[str]
    risks: list[str]
    open_questions: list[str]
    follow_up_actions: list[str]
    work_package_candidate: WorkPackageCandidateRequest

    def validate(self, option_count: int) -> list[str]:
        errors: list[str] = []
        if not self.recommendation.strip():
            errors.append("recommendation is required")
        if not 0 <= self.selected_option_index < option_count:
            errors.append("selected_option_index points outside options")
        for field_name in ("tradeoffs", "risks", "follow_up_actions"):
            if not getattr(self, field_name):
                errors.append(f"{field_name} must be non-empty")
        return errors

    def to_dict(self) -> dict[str, Any]:
        data = asdict(self)
        data["work_package_candidate"] = self.work_package_candidate.to_dict()
        return data


@dataclass
class BrainstormingRunResult:
    status: Literal["completed", "failed"]
    trace_id: str
    contributions: list[BrainstormingContributionDraft]
    critiques: list[BrainstormingCritiqueDraft]
    options: list[BrainstormingOptionDraft]
    decision_brief: DecisionBriefDraft | None
    events: list[AgentBackendEvent]
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "trace_id": self.trace_id,
            "contributions": [item.to_dict() for item in self.contributions],
            "critiques": [item.to_dict() for item in self.critiques],
            "options": [item.to_dict() for item in self.options],
            "decision_brief": self.decision_brief.to_dict() if self.decision_brief else None,
            "events": [event.to_dict() for event in self.events],
            "errors": list(self.errors),
        }
