"""Control Plane domain models."""

from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from typing import Any, Literal
import uuid


AgentRunStatus = Literal["queued", "running", "completed", "failed", "canceled"]
WorkPackageStatus = Literal["draft", "pending_approval", "approved", "rejected", "canceled", "superseded"]
ApprovalStatus = Literal["not_required", "pending", "approved", "rejected"]


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
    langsmith_trace_id: str | None
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
