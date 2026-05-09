"""Control Plane domain models."""

from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Any, Literal
import uuid


AgentRunStatus = Literal["queued", "running", "completed", "failed", "canceled"]
WorkPackageStatus = Literal["draft", "pending_approval", "approved", "rejected", "canceled", "superseded"]
ApprovalStatus = Literal["not_required", "pending", "approved", "rejected"]
ImplementationRunStatus = Literal[
    "queued",
    "planning",
    "patch_proposed",
    "pending_patch_approval",
    "applying",
    "verifying",
    "reviewing",
    "completed",
    "failed",
    "canceled",
]
PatchSetStatus = Literal["proposed", "pending_approval", "approved", "applied", "rejected", "failed"]
PatchOperation = Literal["create", "update", "delete"]
VerificationStatus = Literal["not_run", "running", "passed", "failed", "blocked"]
ReviewStatus = Literal["pass", "needs_changes", "blocked"]
BrainstormingSourceType = Literal["topic", "work_package", "implementation_run", "review_result"]
BrainstormingMode = Literal[
    "free_ideation",
    "architecture_debate",
    "implementation_strategy",
    "risk_review",
    "product_planning",
]
BrainstormingSessionStatus = Literal[
    "queued",
    "running",
    "awaiting_decision",
    "accepted",
    "rejected",
    "converted",
    "failed",
    "canceled",
]
DecisionBriefStatus = Literal["pending", "accepted", "rejected"]
MemoryType = Literal["decision", "session_summary", "project_rule", "failure", "work_note"]
MemoryStatus = Literal["active", "archived", "superseded"]
MemoryCreatedBy = Literal["user", "system", "agent"]
MemorySourceType = Literal[
    "decision_record",
    "brainstorming_session",
    "work_package",
    "implementation_run",
    "verification_run",
    "review_result",
    "session",
    "manual",
]
MemoryRelation = Literal["derived_from", "supports", "contradicts", "supersedes", "follows_up"]
MemoryExtractionStatus = Literal[
    "queued",
    "running",
    "candidate_ready",
    "completed",
    "failed",
    "canceled",
]
MemoryCandidateStatus = Literal["pending", "accepted", "rejected"]


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def new_id(prefix: str) -> str:
    return f"{prefix}_{uuid.uuid4().hex[:12]}"


@dataclass
class Project:
    id: str
    name: str
    root_path: str
    status: str
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class Session:
    id: str
    project_id: str
    title: str
    status: str
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class AgentRun:
    id: str
    project_id: str
    session_id: str
    user_request: str
    status: AgentRunStatus
    intent: str | None
    current_phase: str | None
    trace_id: str | None
    external_trace_id: str | None
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class WorkPackage:
    id: str
    project_id: str
    session_id: str
    source_agent_run_id: str
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
    approval_required: bool
    approval_status: ApprovalStatus
    completion_criteria: list[str]
    status: WorkPackageStatus
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ApprovalRequest:
    id: str
    project_id: str
    session_id: str
    target_type: str
    target_id: str
    reason: str
    risk_level: str
    status: ApprovalStatus
    created_at: str
    resolved_at: str | None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class Event:
    id: str
    project_id: str
    session_id: str
    agent_run_id: str | None
    type: str
    payload: dict[str, Any]
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class Artifact:
    id: str
    project_id: str
    session_id: str
    source_agent_run_id: str
    type: str
    title: str
    payload: dict[str, Any]
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class Trace:
    id: str
    project_id: str
    session_id: str
    agent_run_id: str
    root_name: str
    status: str
    started_at: str
    ended_at: str | None
    metadata: dict[str, Any]

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class TraceStep:
    id: str
    trace_id: str
    parent_step_id: str | None
    name: str
    type: str
    status: str
    inputs_summary: str
    outputs_summary: str
    started_at: str
    ended_at: str | None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ImplementationRun:
    id: str
    project_id: str
    session_id: str
    work_package_id: str
    status: ImplementationRunStatus
    current_phase: str | None
    trace_id: str | None
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ImplementationPlan:
    id: str
    implementation_run_id: str
    goal: str
    context_summary: str
    target_files: list[str]
    steps: list[str]
    verification_strategy: list[str]
    risks: list[dict[str, Any]]
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PatchSet:
    id: str
    implementation_run_id: str
    status: PatchSetStatus
    summary: str
    risk_level: str
    approval_status: ApprovalStatus
    applied_files: list[str]
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class PatchFile:
    id: str
    patch_set_id: str
    path: str
    operation: PatchOperation
    diff: str
    rationale: str
    risk_level: str
    replacement_content: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class VerificationRun:
    id: str
    implementation_run_id: str
    command: str
    status: VerificationStatus
    exit_code: int | None
    stdout: str
    stderr: str
    started_at: str
    ended_at: str | None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ReviewResult:
    id: str
    implementation_run_id: str
    status: ReviewStatus
    findings: list[str]
    residual_risks: list[str]
    recommendation: str
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingSession:
    id: str
    project_id: str
    session_id: str
    source_type: BrainstormingSourceType
    source_id: str | None
    topic: str
    mode: BrainstormingMode
    status: BrainstormingSessionStatus
    current_phase: str | None
    selected_roles: list[str]
    trace_id: str | None
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingContribution:
    id: str
    brainstorming_session_id: str
    role: str
    stance: str
    summary: str
    arguments: list[str]
    concerns: list[str]
    suggested_actions: list[str]
    referenced_artifacts: list[str]
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingCritique:
    id: str
    brainstorming_session_id: str
    critic_role: str
    target_role: str
    weak_assumptions: list[str]
    missing_context: list[str]
    risks: list[str]
    suggested_revisions: list[str]
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class BrainstormingOption:
    id: str
    brainstorming_session_id: str
    title: str
    summary: str
    benefits: list[str]
    costs: list[str]
    risks: list[str]
    required_work: list[str]
    verification_hint: str
    score: float
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class DecisionBrief:
    id: str
    brainstorming_session_id: str
    recommendation: str
    selected_option_id: str
    rationale: str
    tradeoffs: list[str]
    risks: list[str]
    open_questions: list[str]
    follow_up_actions: list[str]
    work_package_candidate: dict[str, Any]
    status: DecisionBriefStatus
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class DecisionRecord:
    id: str
    project_id: str
    session_id: str
    brainstorming_session_id: str
    title: str
    decision: str
    rationale: str
    consequences: list[str]
    follow_up_actions: list[str]
    linked_work_package_id: str | None
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class ProjectMemoryItem:
    id: str
    project_id: str
    type: MemoryType
    title: str
    summary: str
    body: str
    tags: list[str]
    status: MemoryStatus
    importance: str
    confidence: float
    created_by: MemoryCreatedBy
    source_count: int
    last_used_at: str | None
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class MemorySourceLink:
    id: str
    memory_item_id: str
    source_type: MemorySourceType
    source_id: str
    relation: MemoryRelation
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class MemoryExtractionRun:
    id: str
    project_id: str
    session_id: str
    source_type: MemorySourceType
    source_id: str
    status: MemoryExtractionStatus
    candidate_count: int
    created_memory_count: int
    trace_id: str | None
    created_at: str
    updated_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass
class MemoryCandidate:
    id: str
    extraction_run_id: str
    type: MemoryType
    title: str
    summary: str
    body: str
    tags: list[str]
    importance: str
    confidence: float
    source_links: list[dict[str, Any]]
    status: MemoryCandidateStatus
    created_at: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)
