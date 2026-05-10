"""Control Plane orchestration service."""

from __future__ import annotations

from pathlib import Path
import shutil
import subprocess
import sys
from typing import Any

from .agent_client import AgentBackendClient, HTTPAgentBackendClient
from .models import utc_now
from .storage import SQLiteStore


ALLOWED_VERIFICATION_COMMANDS = frozenset(
    {
        "python -m unittest discover -s tests",
        "pytest",
        "npm run build",
        "npm run test",
        "npm audit --omit=dev",
    }
)
ALLOWED_BRAINSTORMING_MODES = frozenset(
    {
        "free_ideation",
        "architecture_debate",
        "implementation_strategy",
        "risk_review",
        "product_planning",
    }
)
ALLOWED_BRAINSTORMING_SOURCE_TYPES = frozenset(
    {"topic", "work_package", "implementation_run", "review_result"}
)
ALLOWED_MEMORY_TYPES = frozenset(
    {"decision", "session_summary", "project_rule", "failure", "work_note"}
)
ALLOWED_MEMORY_STATUSES = frozenset({"active", "archived", "superseded"})
ALLOWED_MEMORY_SOURCE_TYPES = frozenset(
    {
        "decision_record",
        "brainstorming_session",
        "work_package",
        "implementation_run",
        "verification_run",
        "review_result",
        "session",
        "manual",
    }
)
ALLOWED_RISK_SCAN_SCOPE_TYPES = frozenset(
    {"project", "session", "work_package", "implementation_run", "review_result", "memory_focus"}
)
ALLOWED_RISK_FINDING_STATUSES = frozenset({"open", "accepted", "dismissed", "mitigated", "converted"})
ALLOWED_RISK_FINDING_CATEGORIES = frozenset(
    {"architecture", "implementation", "verification", "schedule", "product", "security", "process"}
)
ALLOWED_RISK_FINDING_SEVERITIES = frozenset({"info", "low", "medium", "high", "critical"})
ALLOWED_QUALITY_SIGNAL_KINDS = frozenset(
    {"verification", "coverage_hint", "code_size", "memory", "process", "architecture"}
)
ALLOWED_QUALITY_SIGNAL_STATUSES = frozenset({"healthy", "watch", "at_risk", "unknown"})
SECRET_MARKERS = ("api_key", "apikey", "password", "secret", "token", "bearer ")
DEFAULT_BRAINSTORMING_ROLES: dict[str, list[str]] = {
    "free_ideation": ["product_planner", "system_architect", "implementation_planner"],
    "architecture_debate": [
        "product_planner",
        "system_architect",
        "implementation_planner",
        "risk_reviewer",
        "devil_advocate",
    ],
    "implementation_strategy": [
        "system_architect",
        "implementation_planner",
        "risk_reviewer",
        "product_planner",
    ],
    "risk_review": [
        "risk_reviewer",
        "system_architect",
        "implementation_planner",
        "devil_advocate",
    ],
    "product_planning": [
        "product_planner",
        "system_architect",
        "implementation_planner",
        "risk_reviewer",
    ],
}


class ControlPlaneService:
    def __init__(
        self,
        store: SQLiteStore,
        agent_backend: AgentBackendClient | None = None,
    ) -> None:
        self.store = store
        self.agent_backend = agent_backend or HTTPAgentBackendClient()

    def open_project(self, *, name: str, root_path: str) -> dict[str, Any]:
        project = self.store.create_project(name=name, root_path=str(Path(root_path).resolve()))
        return project.to_dict()

    def create_session(self, *, project_id: str, title: str) -> dict[str, Any]:
        session = self.store.create_session(project_id=project_id, title=title)
        return session.to_dict()

    def get_command_center(
        self,
        *,
        project_id: str,
        session_id: str | None = None,
    ) -> dict[str, Any]:
        project = self.store.get_project(project_id)
        session = self.store.get_session(session_id) if session_id else None
        pending_approvals = self.store.list_approval_requests(
            project_id=project_id,
            status="pending",
            limit=10,
        )
        agent_runs = self.store.list_agent_runs_by_project(
            project_id=project_id,
            session_id=session_id,
            limit=8,
        )
        implementation_runs = self.store.list_implementation_runs_by_project(
            project_id=project_id,
            session_id=session_id,
            limit=8,
        )
        risk_findings = self.store.list_risk_findings(
            project_id=project_id,
            status="open",
            limit=8,
        )
        selected_memory = self.store.list_selected_memory(session_id) if session_id else []
        quality = self.get_quality_snapshot(project_id=project_id)
        failures = [
            {"type": "agent_run", **run}
            for run in agent_runs
            if run["status"] in {"failed", "canceled"}
        ] + [
            {"type": "implementation_run", **run}
            for run in implementation_runs
            if run["status"] in {"failed", "canceled"}
        ]
        return {
            "project": project,
            "session": session,
            "counts": {
                "pending_approvals": len(pending_approvals),
                "open_risk_findings": len(risk_findings),
                "selected_memory": len(selected_memory),
                "recent_agent_runs": len(agent_runs),
                "recent_implementation_runs": len(implementation_runs),
                "failed_or_canceled_runs": len(failures),
            },
            "pending_approvals": pending_approvals,
            "recent_agent_runs": agent_runs,
            "recent_implementation_runs": implementation_runs,
            "top_risk_findings": risk_findings[:5],
            "selected_memory": selected_memory,
            "quality": quality,
            "next_action": self._command_center_next_action(
                pending_approvals=pending_approvals,
                risk_findings=risk_findings,
                failures=failures,
                selected_memory=selected_memory,
                session=session,
            ),
        }

    def start_work_package_request(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        user_request: str,
    ) -> dict[str, Any]:
        run = self.store.create_agent_run(project["id"], session["id"], user_request)
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="agent_run.created",
            payload={"status": "queued"},
        )
        return {
            "project_id": project["id"],
            "session_id": session["id"],
            "agent_run_id": run.id,
            "status": "queued",
            "events_url": f"/api/agent-runs/{run.id}/events/stream",
        }

    def execute_work_package_request(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        agent_run_id: str,
        user_request: str,
    ) -> dict[str, Any]:
        try:
            return self._execute_work_package_request(
                project=project,
                session=session,
                agent_run_id=agent_run_id,
                user_request=user_request,
            )
        except Exception as exc:  # pragma: no cover - defensive background task path
            self.store.update_agent_run(
                agent_run_id,
                status="failed",
                current_phase="failed",
            )
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=agent_run_id,
                event_type="agent_run.failed",
                payload={"error": str(exc)},
            )
            return {
                "project_id": project["id"],
                "session_id": session["id"],
                "agent_run_id": agent_run_id,
                "status": "failed",
                "errors": [str(exc)],
            }

    def _execute_work_package_request(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        agent_run_id: str,
        user_request: str,
    ) -> dict[str, Any]:
        self.store.update_agent_run(agent_run_id, status="running", current_phase="agent_backend")
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=agent_run_id,
            event_type="agent_run.started",
            payload={},
        )

        backend_result = self.agent_backend.run_agent(
            {
                "project_id": project["id"],
                "session_id": session["id"],
                "agent_run_id": agent_run_id,
                "user_request": user_request,
                "project_root": project["root_path"],
            }
        )
        for event in backend_result["events"]:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=agent_run_id,
                event_type=event["type"],
                payload=event["payload"],
            )

        trace_id = backend_result.get("trace_id")
        external_trace_id = backend_result.get("external_trace_id")
        self.store.update_agent_run(
            agent_run_id,
            status=backend_result["status"],
            intent=backend_result["intent_result"]["intent"],
            current_phase="completed" if backend_result["status"] == "completed" else "failed",
            trace_id=trace_id,
            external_trace_id=external_trace_id,
        )

        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=agent_run_id,
            artifact_type="intent_result",
            title="Intent Result",
            payload=backend_result["intent_result"],
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=agent_run_id,
            artifact_type="context_summary",
            title="Context Summary",
            payload=backend_result["context_summary"],
        )

        if backend_result["status"] != "completed" or backend_result["work_package"] is None:
            result = {
                "agent_run_id": agent_run_id,
                "status": backend_result["status"],
                "errors": backend_result["errors"],
                "trace_id": trace_id,
                "external_trace_id": external_trace_id,
            }
            self._record_trace_from_run(
                project=project,
                session=session,
                agent_run_id=agent_run_id,
                trace_id=trace_id,
                external_trace_id=external_trace_id,
                status=backend_result["status"],
            )
            return result

        draft = backend_result["work_package"]
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=agent_run_id,
            artifact_type="work_package_draft",
            title=draft["title"],
            payload=draft,
        )
        package = self.store.create_work_package(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=agent_run_id,
            draft=draft,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=agent_run_id,
            event_type="work_package.created",
            payload={"work_package_id": package.id},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=agent_run_id,
            event_type="work_package.pending_approval",
            payload={"work_package_id": package.id},
        )

        approval = self.store.create_approval_request(
            project_id=project["id"],
            session_id=session["id"],
            target_type="work_package",
            target_id=package.id,
            reason="MVP 2 requires approval before any implementation work.",
            risk_level=package.risks[0]["level"] if package.risks else "medium",
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=agent_run_id,
            event_type="approval.requested",
            payload={
                "approval_id": approval.id,
                "target_type": approval.target_type,
                "target_id": approval.target_id,
            },
        )
        self._record_trace_from_run(
            project=project,
            session=session,
            agent_run_id=agent_run_id,
            trace_id=trace_id,
            external_trace_id=external_trace_id,
            status=backend_result["status"],
        )

        return {
            "project_id": project["id"],
            "session_id": session["id"],
            "agent_run_id": agent_run_id,
            "work_package_id": package.id,
            "approval_id": approval.id,
            "status": "pending_approval",
            "trace_id": trace_id,
            "external_trace_id": external_trace_id,
        }

    def create_work_package_from_request(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        user_request: str,
    ) -> dict[str, Any]:
        queued = self.start_work_package_request(
            project=project,
            session=session,
            user_request=user_request,
        )
        return self.execute_work_package_request(
            project=project,
            session=session,
            agent_run_id=queued["agent_run_id"],
            user_request=user_request,
        )

    def get_agent_run_result(self, agent_run_id: str) -> dict[str, Any]:
        run = self.store.get_agent_run(agent_run_id)
        work_package = self.store.get_work_package_by_agent_run(agent_run_id)
        approval = None
        if work_package is not None:
            approval = self.store.get_approval_for_target(
                target_type="work_package",
                target_id=work_package["id"],
            )
        try:
            trace = self.store.get_trace_summary(agent_run_id)
        except KeyError:
            trace = None

        return {
            "agent_run": run,
            "work_package": work_package,
            "approval": approval,
            "artifacts": self.store.list_artifacts(agent_run_id),
            "trace": trace,
            "events": self.store.list_events(agent_run_id),
        }

    def resolve_approval(self, *, approval_id: str, status: str) -> dict[str, Any]:
        approval = self.store.resolve_approval(approval_id, status)
        event_type = "approval.approved" if status == "approved" else "approval.rejected"
        self.store.append_event(
            project_id=approval["project_id"],
            session_id=approval["session_id"],
            agent_run_id=approval.get("source_agent_run_id"),
            event_type=event_type,
            payload={
                "approval_id": approval["id"],
                "target_type": approval["target_type"],
                "target_id": approval["target_id"],
            },
        )
        return approval

    def create_implementation_run(self, *, work_package_id: str) -> dict[str, Any]:
        work_package = self.store.get_work_package(work_package_id)
        if work_package["status"] != "approved" or work_package["approval_status"] != "approved":
            raise ValueError("ImplementationRun requires an approved WorkPackage")

        project = self.store.get_project(work_package["project_id"])
        session = self.store.get_session(work_package["session_id"])
        run = self.store.create_implementation_run(
            project_id=project["id"],
            session_id=session["id"],
            work_package_id=work_package["id"],
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="implementation_run.created",
            payload={"implementation_run_id": run.id, "work_package_id": work_package["id"]},
        )
        return self.execute_implementation_run(
            project=project,
            session=session,
            work_package=work_package,
            implementation_run_id=run.id,
        )

    def execute_implementation_run(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        work_package: dict[str, Any],
        implementation_run_id: str,
    ) -> dict[str, Any]:
        self.store.update_implementation_run(
            implementation_run_id,
            status="planning",
            current_phase="agent_backend",
        )
        proposal = self.agent_backend.create_implementation_proposal(
            {
                "project_id": project["id"],
                "session_id": session["id"],
                "implementation_run_id": implementation_run_id,
                "project_root": project["root_path"],
                "work_package": work_package,
            }
        )
        for event in proposal["events"]:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=implementation_run_id,
                event_type=event["type"],
                payload=event["payload"],
            )

        trace_id = proposal.get("trace_id")
        if proposal["status"] != "completed":
            self.store.update_implementation_run(
                implementation_run_id,
                status="failed",
                current_phase="failed",
                trace_id=trace_id,
            )
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=implementation_run_id,
                event_type="implementation_run.failed",
                payload={"errors": proposal.get("errors", [])},
            )
            self._record_trace_from_implementation_run(
                project=project,
                session=session,
                implementation_run_id=implementation_run_id,
                trace_id=trace_id,
                status="failed",
            )
            return self.get_implementation_run_result(implementation_run_id)

        plan = self.store.create_implementation_plan(
            implementation_run_id=implementation_run_id,
            plan=proposal["implementation_plan"],
        )
        patch_set = self.store.create_patch_set(
            implementation_run_id=implementation_run_id,
            patch_set=proposal["patch_set"],
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=implementation_run_id,
            artifact_type="implementation_plan",
            title="Implementation Plan",
            payload=plan.to_dict(),
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=implementation_run_id,
            artifact_type="patch_set",
            title=patch_set.summary,
            payload=self.store.get_patch_set(patch_set.id),
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=implementation_run_id,
            event_type="implementation_plan.created",
            payload={"implementation_plan_id": plan.id},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=implementation_run_id,
            event_type="patch_set.pending_approval",
            payload={"patch_set_id": patch_set.id},
        )
        self.store.update_implementation_run(
            implementation_run_id,
            status="pending_patch_approval",
            current_phase="pending_patch_approval",
            trace_id=trace_id,
        )
        self._record_trace_from_implementation_run(
            project=project,
            session=session,
            implementation_run_id=implementation_run_id,
            trace_id=trace_id,
            status="pending_patch_approval",
        )
        return self.get_implementation_run_result(implementation_run_id)

    def get_implementation_run_result(self, implementation_run_id: str) -> dict[str, Any]:
        run = self.store.get_implementation_run(implementation_run_id)
        try:
            trace = self.store.get_trace_summary(implementation_run_id)
        except KeyError:
            trace = None
        return {
            "implementation_run": run,
            "work_package": self.store.get_work_package(run["work_package_id"]),
            "implementation_plan": self.store.get_implementation_plan(implementation_run_id),
            "patch_set": self.store.get_patch_set_by_implementation_run(implementation_run_id),
            "verification_runs": self.store.list_verification_runs(implementation_run_id),
            "review_result": self.store.get_review_result(implementation_run_id),
            "trace": trace,
            "events": self.store.list_events(implementation_run_id),
        }

    def resolve_patch_set(self, *, patch_set_id: str, status: str) -> dict[str, Any]:
        if status not in {"approved", "rejected"}:
            raise ValueError("patch set status must be approved or rejected")
        patch_set = self.store.get_patch_set(patch_set_id)
        run = self.store.get_implementation_run(patch_set["implementation_run_id"])
        event_type = "patch_set.approved" if status == "approved" else "patch_set.rejected"
        patch_status = "approved" if status == "approved" else "rejected"
        self.store.update_patch_set(
            patch_set_id,
            status=patch_status,
            approval_status=status,
        )
        if status == "rejected":
            self.store.update_implementation_run(
                run["id"],
                status="canceled",
                current_phase="patch_rejected",
            )
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type=event_type,
            payload={"patch_set_id": patch_set_id},
        )
        return self.store.get_patch_set(patch_set_id)

    def apply_patch_set(self, *, patch_set_id: str) -> dict[str, Any]:
        patch_set = self.store.get_patch_set(patch_set_id)
        if patch_set["approval_status"] != "approved" or patch_set["status"] != "approved":
            raise ValueError("PatchSet must be approved before apply")

        run = self.store.get_implementation_run(patch_set["implementation_run_id"])
        project = self.store.get_project(run["project_id"])
        root = Path(project["root_path"]).resolve()
        self.store.update_implementation_run(run["id"], status="applying", current_phase="apply_patch")
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type="patch_set.apply_started",
            payload={"patch_set_id": patch_set_id},
        )
        try:
            applied_files = self._apply_patch_files(root, patch_set["files"])
        except Exception as exc:
            self.store.update_patch_set(patch_set_id, status="failed")
            self.store.update_implementation_run(run["id"], status="failed", current_phase="apply_failed")
            self.store.append_event(
                project_id=run["project_id"],
                session_id=run["session_id"],
                agent_run_id=run["id"],
                event_type="patch_set.apply_failed",
                payload={"patch_set_id": patch_set_id, "error": str(exc)},
            )
            raise

        self.store.update_patch_set(
            patch_set_id,
            status="applied",
            applied_files=applied_files,
        )
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type="patch_set.applied",
            payload={"patch_set_id": patch_set_id, "applied_files": applied_files},
        )
        self.run_verification(implementation_run_id=run["id"], command=None)
        self.create_review_result(implementation_run_id=run["id"])
        return self.store.get_patch_set(patch_set_id)

    def run_verification(
        self,
        *,
        implementation_run_id: str,
        command: str | None,
    ) -> dict[str, Any]:
        run = self.store.get_implementation_run(implementation_run_id)
        project = self.store.get_project(run["project_id"])
        root = Path(project["root_path"]).resolve()
        command = (command or self._infer_verification_command(root) or "").strip()
        self.store.update_implementation_run(
            implementation_run_id,
            status="verifying",
            current_phase="run_verification",
        )

        if not command:
            verification = self.store.create_verification_run(
                implementation_run_id=implementation_run_id,
                command="",
                status="not_run",
                exit_code=None,
                stdout="",
                stderr="No allowlisted verification command could be inferred.",
            )
            self.store.append_event(
                project_id=run["project_id"],
                session_id=run["session_id"],
                agent_run_id=implementation_run_id,
                event_type="verification.blocked",
                payload={"verification_run_id": verification.id, "reason": verification.stderr},
            )
            return verification.to_dict()

        if command not in ALLOWED_VERIFICATION_COMMANDS:
            verification = self.store.create_verification_run(
                implementation_run_id=implementation_run_id,
                command=command,
                status="blocked",
                exit_code=None,
                stdout="",
                stderr="Command is not in the MVP 3 verification allowlist.",
            )
            self.store.append_event(
                project_id=run["project_id"],
                session_id=run["session_id"],
                agent_run_id=implementation_run_id,
                event_type="verification.blocked",
                payload={"verification_run_id": verification.id, "command": command},
            )
            return verification.to_dict()

        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=implementation_run_id,
            event_type="verification.started",
            payload={"command": command},
        )
        started_at = utc_now()
        try:
            completed = subprocess.run(
                self._command_args(command),
                cwd=root,
                check=False,
                capture_output=True,
                text=True,
                encoding="utf-8",
                errors="replace",
                timeout=60,
            )
            status = "passed" if completed.returncode == 0 else "failed"
            verification = self.store.create_verification_run(
                implementation_run_id=implementation_run_id,
                command=command,
                status=status,
                exit_code=completed.returncode,
                stdout=completed.stdout,
                stderr=completed.stderr,
                started_at=started_at,
                ended_at=utc_now(),
            )
        except Exception as exc:
            verification = self.store.create_verification_run(
                implementation_run_id=implementation_run_id,
                command=command,
                status="blocked",
                exit_code=None,
                stdout="",
                stderr=str(exc),
                started_at=started_at,
                ended_at=utc_now(),
            )

        event_type = "verification.completed" if verification.status == "passed" else "verification.failed"
        if verification.status == "blocked":
            event_type = "verification.blocked"
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=implementation_run_id,
            event_type=event_type,
            payload={
                "verification_run_id": verification.id,
                "command": command,
                "status": verification.status,
                "exit_code": verification.exit_code,
            },
        )
        return verification.to_dict()

    def create_review_result(self, *, implementation_run_id: str) -> dict[str, Any]:
        run = self.store.get_implementation_run(implementation_run_id)
        self.store.update_implementation_run(
            implementation_run_id,
            status="reviewing",
            current_phase="review_result",
        )
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=implementation_run_id,
            event_type="review.started",
            payload={},
        )
        review = self.agent_backend.create_review_result(
            {
                "implementation_run_id": implementation_run_id,
                "work_package": self.store.get_work_package(run["work_package_id"]),
                "patch_set": self.store.get_patch_set_by_implementation_run(implementation_run_id),
                "verification_runs": self.store.list_verification_runs(implementation_run_id),
            }
        )
        stored = self.store.create_review_result(
            implementation_run_id=implementation_run_id,
            review=review,
        )
        final_status = "completed" if stored.status == "pass" else "failed"
        self.store.update_implementation_run(
            implementation_run_id,
            status=final_status,
            current_phase="completed" if final_status == "completed" else "review_blocked",
        )
        self.store.append_event(
            project_id=run["project_id"],
            session_id=run["session_id"],
            agent_run_id=implementation_run_id,
            event_type="review.completed",
            payload={"review_result_id": stored.id, "status": stored.status},
        )
        self._record_trace_from_implementation_run(
            project=self.store.get_project(run["project_id"]),
            session=self.store.get_session(run["session_id"]),
            implementation_run_id=implementation_run_id,
            trace_id=run["trace_id"],
            status=final_status,
        )
        return stored.to_dict()

    def start_brainstorming_session(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        topic: str,
        mode: str = "architecture_debate",
        source_type: str = "topic",
        source_id: str | None = None,
        roles: list[str] | None = None,
    ) -> dict[str, Any]:
        topic = " ".join(topic.strip().split())
        if not topic:
            raise ValueError("Brainstorming topic is required")
        if mode not in ALLOWED_BRAINSTORMING_MODES:
            raise ValueError(f"Unsupported brainstorming mode: {mode}")
        if source_type not in ALLOWED_BRAINSTORMING_SOURCE_TYPES:
            raise ValueError(f"Unsupported brainstorming source_type: {source_type}")
        self._source_context_for_brainstorming(source_type=source_type, source_id=source_id)
        selected_roles = self._normalize_brainstorming_roles(roles or [], mode)
        item = self.store.create_brainstorming_session(
            project_id=project["id"],
            session_id=session["id"],
            source_type=source_type,
            source_id=source_id,
            topic=topic,
            mode=mode,
            selected_roles=selected_roles,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=item.id,
            event_type="brainstorming_session.created",
            payload={
                "brainstorming_session_id": item.id,
                "mode": mode,
                "source_type": source_type,
                "roles": selected_roles,
            },
        )
        return {
            "project_id": project["id"],
            "session_id": session["id"],
            "brainstorming_session_id": item.id,
            "status": "queued",
            "events_url": f"/api/brainstorming-sessions/{item.id}/events/stream",
        }

    def execute_brainstorming_session(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        brainstorming_session_id: str,
    ) -> dict[str, Any]:
        try:
            return self._execute_brainstorming_session(
                project=project,
                session=session,
                brainstorming_session_id=brainstorming_session_id,
            )
        except Exception as exc:  # pragma: no cover - defensive background task path
            self.store.update_brainstorming_session(
                brainstorming_session_id,
                status="failed",
                current_phase="failed",
            )
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=brainstorming_session_id,
                event_type="brainstorming_session.failed",
                payload={"error": str(exc)},
            )
            return {
                "brainstorming_session_id": brainstorming_session_id,
                "status": "failed",
                "errors": [str(exc)],
            }

    def _execute_brainstorming_session(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        brainstorming_session_id: str,
    ) -> dict[str, Any]:
        brainstorming_session = self.store.get_brainstorming_session(brainstorming_session_id)
        self.store.update_brainstorming_session(
            brainstorming_session_id,
            status="running",
            current_phase="agent_backend",
        )
        source_context = self._source_context_for_brainstorming(
            source_type=brainstorming_session["source_type"],
            source_id=brainstorming_session["source_id"],
        )
        result = self.agent_backend.run_brainstorming(
            {
                "project_id": project["id"],
                "session_id": session["id"],
                "brainstorming_session_id": brainstorming_session_id,
                "project_root": project["root_path"],
                "topic": brainstorming_session["topic"],
                "mode": brainstorming_session["mode"],
                "source_type": brainstorming_session["source_type"],
                "source_id": brainstorming_session["source_id"],
                "roles": brainstorming_session["selected_roles"],
                "source_context": source_context,
            }
        )
        for event in result["events"]:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=brainstorming_session_id,
                event_type=event["type"],
                payload=event["payload"],
            )

        trace_id = result.get("trace_id")
        if result["status"] != "completed" or result.get("decision_brief") is None:
            self.store.update_brainstorming_session(
                brainstorming_session_id,
                status="failed",
                current_phase="failed",
                trace_id=trace_id,
            )
            self._record_trace_from_brainstorming_session(
                project=project,
                session=session,
                brainstorming_session_id=brainstorming_session_id,
                trace_id=trace_id,
                status="failed",
            )
            return self.get_brainstorming_result(brainstorming_session_id)

        stored_result = self.store.create_brainstorming_result(
            brainstorming_session_id=brainstorming_session_id,
            contributions=result["contributions"],
            critiques=result["critiques"],
            options=result["options"],
            decision_brief=result["decision_brief"],
        )
        brief = stored_result["decision_brief"]
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=brainstorming_session_id,
            artifact_type="brainstorming_result",
            title=brainstorming_session["topic"],
            payload={
                "contribution_count": len(stored_result["contributions"]),
                "option_count": len(stored_result["options"]),
                "decision_brief_id": brief["id"] if brief else None,
            },
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=brainstorming_session_id,
            artifact_type="decision_brief",
            title=brief["recommendation"] if brief else "Decision Brief",
            payload=brief or {},
        )
        self.store.update_brainstorming_session(
            brainstorming_session_id,
            status="awaiting_decision",
            current_phase="awaiting_decision",
            trace_id=trace_id,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=brainstorming_session_id,
            event_type="brainstorming_session.completed",
            payload={"brainstorming_session_id": brainstorming_session_id},
        )
        self._record_trace_from_brainstorming_session(
            project=project,
            session=session,
            brainstorming_session_id=brainstorming_session_id,
            trace_id=trace_id,
            status="awaiting_decision",
        )
        return self.get_brainstorming_result(brainstorming_session_id)

    def create_brainstorming_session(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        topic: str,
        mode: str = "architecture_debate",
        source_type: str = "topic",
        source_id: str | None = None,
        roles: list[str] | None = None,
    ) -> dict[str, Any]:
        queued = self.start_brainstorming_session(
            project=project,
            session=session,
            topic=topic,
            mode=mode,
            source_type=source_type,
            source_id=source_id,
            roles=roles,
        )
        return self.execute_brainstorming_session(
            project=project,
            session=session,
            brainstorming_session_id=queued["brainstorming_session_id"],
        )

    def get_brainstorming_result(self, brainstorming_session_id: str) -> dict[str, Any]:
        result = self.store.get_brainstorming_result(brainstorming_session_id)
        try:
            trace = self.store.get_trace_summary(brainstorming_session_id)
        except KeyError:
            trace = None
        result["trace"] = trace
        result["events"] = self.store.list_events(brainstorming_session_id)
        result["artifacts"] = self.store.list_artifacts(brainstorming_session_id)
        return result

    def resolve_decision_brief(
        self,
        *,
        brainstorming_session_id: str,
        decision_brief_id: str,
        status: str,
        note: str | None = None,
    ) -> dict[str, Any]:
        if status not in {"accepted", "rejected"}:
            raise ValueError("DecisionBrief status must be accepted or rejected")
        brainstorming_session = self.store.get_brainstorming_session(brainstorming_session_id)
        brief = self.store.get_decision_brief(decision_brief_id)
        if brief["brainstorming_session_id"] != brainstorming_session_id:
            raise ValueError("DecisionBrief does not belong to this BrainstormingSession")
        if brief["status"] != "pending":
            raise ValueError("DecisionBrief has already been resolved")

        self.store.update_decision_brief_status(decision_brief_id, status)
        session_status = "accepted" if status == "accepted" else "rejected"
        self.store.update_brainstorming_session(
            brainstorming_session_id,
            status=session_status,
            current_phase=session_status,
        )
        if status == "rejected":
            self.store.append_event(
                project_id=brainstorming_session["project_id"],
                session_id=brainstorming_session["session_id"],
                agent_run_id=brainstorming_session_id,
                event_type="decision_record.rejected",
                payload={"decision_brief_id": decision_brief_id, "reason": note or ""},
            )
            return self.get_brainstorming_result(brainstorming_session_id)

        title = brief["work_package_candidate"].get("title") or brief["recommendation"]
        record = self.store.create_decision_record(
            project_id=brainstorming_session["project_id"],
            session_id=brainstorming_session["session_id"],
            brainstorming_session_id=brainstorming_session_id,
            title=title,
            decision=brief["recommendation"],
            rationale=f"{brief['rationale']}\n\nAcceptance note: {note or 'Accepted.'}",
            consequences=[*brief["tradeoffs"], *brief["risks"]],
            follow_up_actions=brief["follow_up_actions"],
        )
        self.store.append_event(
            project_id=brainstorming_session["project_id"],
            session_id=brainstorming_session["session_id"],
            agent_run_id=brainstorming_session_id,
            event_type="decision_record.accepted",
            payload={"decision_brief_id": decision_brief_id},
        )
        self.store.append_event(
            project_id=brainstorming_session["project_id"],
            session_id=brainstorming_session["session_id"],
            agent_run_id=brainstorming_session_id,
            event_type="decision_record.created",
            payload={"decision_record_id": record.id},
        )
        return self.get_brainstorming_result(brainstorming_session_id)

    def convert_decision_record_to_work_package(
        self,
        *,
        decision_record_id: str,
    ) -> dict[str, Any]:
        record = self.store.get_decision_record(decision_record_id)
        if record["linked_work_package_id"]:
            return {
                "decision_record": record,
                "work_package": self.store.get_work_package(record["linked_work_package_id"]),
                "approval": self.store.get_approval_for_target(
                    target_type="work_package",
                    target_id=record["linked_work_package_id"],
                ),
            }
        brief = self.store.get_decision_brief_by_brainstorming_session(
            record["brainstorming_session_id"]
        )
        if brief is None or brief["status"] != "accepted":
            raise ValueError("Only an accepted DecisionRecord can be converted")

        self.store.append_event(
            project_id=record["project_id"],
            session_id=record["session_id"],
            agent_run_id=record["brainstorming_session_id"],
            event_type="work_package.conversion_requested",
            payload={"decision_record_id": decision_record_id},
        )
        candidate = dict(brief["work_package_candidate"])
        candidate["approval_required"] = True
        package = self.store.create_work_package(
            project_id=record["project_id"],
            session_id=record["session_id"],
            source_agent_run_id=record["brainstorming_session_id"],
            draft=candidate,
        )
        approval = self.store.create_approval_request(
            project_id=record["project_id"],
            session_id=record["session_id"],
            target_type="work_package",
            target_id=package.id,
            reason="MVP 4 converted an accepted DecisionRecord into a Work Package candidate.",
            risk_level=package.risks[0]["level"] if package.risks else "medium",
        )
        self.store.update_decision_record_linked_work_package(decision_record_id, package.id)
        self.store.update_brainstorming_session(
            record["brainstorming_session_id"],
            status="converted",
            current_phase="converted",
        )
        self.store.append_event(
            project_id=record["project_id"],
            session_id=record["session_id"],
            agent_run_id=record["brainstorming_session_id"],
            event_type="work_package.conversion_completed",
            payload={
                "decision_record_id": decision_record_id,
                "work_package_id": package.id,
                "approval_id": approval.id,
            },
        )
        self.store.append_event(
            project_id=record["project_id"],
            session_id=record["session_id"],
            agent_run_id=record["brainstorming_session_id"],
            event_type="work_package.pending_approval",
            payload={"work_package_id": package.id},
        )
        self._record_trace_from_brainstorming_session(
            project=self.store.get_project(record["project_id"]),
            session=self.store.get_session(record["session_id"]),
            brainstorming_session_id=record["brainstorming_session_id"],
            trace_id=self.store.get_brainstorming_session(record["brainstorming_session_id"])[
                "trace_id"
            ],
            status="converted",
        )
        return {
            "decision_record": self.store.get_decision_record(decision_record_id),
            "work_package": package.to_dict(),
            "approval": approval.to_dict(),
        }

    def start_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        scope_type: str = "project",
        scope_id: str | None = None,
        include_selected_memory: bool = False,
        selected_memory_ids: list[str] | None = None,
        focus: list[str] | None = None,
    ) -> dict[str, Any]:
        if scope_type not in ALLOWED_RISK_SCAN_SCOPE_TYPES:
            raise ValueError(f"Unsupported risk scan scope_type: {scope_type}")
        self._validate_risk_scope(project=project, session=session, scope_type=scope_type, scope_id=scope_id)
        source_context = self._source_context_for_risk_scan(
            project=project,
            session=session,
            scope_type=scope_type,
            scope_id=scope_id,
            include_selected_memory=include_selected_memory,
            selected_memory_ids=selected_memory_ids or [],
            focus=focus or [],
        )
        run = self.store.create_risk_scan_run(
            project_id=project["id"],
            session_id=session["id"],
            scope_type=scope_type,
            scope_id=scope_id,
            selected_memory_count=len(source_context["selected_memory_snapshots"]),
            source_context=source_context,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="risk_scan.created",
            payload={
                "risk_scan_run_id": run.id,
                "scope_type": scope_type,
                "scope_id": scope_id,
                "selected_memory_count": run.selected_memory_count,
            },
        )
        if run.selected_memory_count:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=run.id,
                event_type="selected_memory.attached_to_risk_scan",
                payload={
                    "risk_scan_run_id": run.id,
                    "memory_item_ids": [item["id"] for item in source_context["selected_memory_snapshots"]],
                },
            )
        return {
            "project_id": project["id"],
            "session_id": session["id"],
            "risk_scan_run_id": run.id,
            "status": "queued",
            "events_url": f"/api/risk-scans/{run.id}/events/stream",
        }

    def execute_risk_scan(self, *, risk_scan_run_id: str) -> dict[str, Any]:
        run = self.store.get_risk_scan_run(risk_scan_run_id)
        project = self.store.get_project(run["project_id"])
        session = self.store.get_session(run["session_id"])
        try:
            return self._execute_risk_scan(project=project, session=session, risk_scan_run=run)
        except Exception as exc:  # pragma: no cover - defensive background task path
            self.store.update_risk_scan_run(
                risk_scan_run_id,
                status="failed",
                current_phase="failed",
            )
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=risk_scan_run_id,
                event_type="risk_scan.failed",
                payload={"error": str(exc)},
            )
            return self.get_risk_scan_result(risk_scan_run_id)

    def create_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        scope_type: str = "project",
        scope_id: str | None = None,
        include_selected_memory: bool = False,
        selected_memory_ids: list[str] | None = None,
        focus: list[str] | None = None,
    ) -> dict[str, Any]:
        queued = self.start_risk_scan(
            project=project,
            session=session,
            scope_type=scope_type,
            scope_id=scope_id,
            include_selected_memory=include_selected_memory,
            selected_memory_ids=selected_memory_ids,
            focus=focus,
        )
        return self.execute_risk_scan(risk_scan_run_id=queued["risk_scan_run_id"])

    def _execute_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        risk_scan_run: dict[str, Any],
    ) -> dict[str, Any]:
        risk_scan_run_id = risk_scan_run["id"]
        self.store.update_risk_scan_run(
            risk_scan_run_id,
            status="collecting",
            current_phase="collect_source_context",
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            event_type="risk_scan.started",
            payload={},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            event_type="risk_scan.phase_changed",
            payload={"phase": "collect_source_context"},
        )
        self.store.update_risk_scan_run(
            risk_scan_run_id,
            status="analyzing",
            current_phase="agent_backend",
        )
        result = self.agent_backend.create_risk_analysis(
            {
                "project_id": project["id"],
                "session_id": session["id"],
                "risk_scan_run_id": risk_scan_run_id,
                "project_root": project["root_path"],
                "scope_type": risk_scan_run["scope_type"],
                "scope_id": risk_scan_run["scope_id"],
                "focus": risk_scan_run["source_context"].get("focus", []),
                "source_context": risk_scan_run["source_context"],
            }
        )
        for event in result["events"]:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=risk_scan_run_id,
                event_type=event["type"],
                payload=event["payload"],
            )
        trace_id = result.get("trace_id")
        self.store.update_risk_scan_run(risk_scan_run_id, trace_id=trace_id)
        if result["status"] != "completed":
            self.store.update_risk_scan_run(
                risk_scan_run_id,
                status="failed",
                current_phase="failed",
            )
            self._record_trace_from_risk_scan(
                project=project,
                session=session,
                risk_scan_run_id=risk_scan_run_id,
                trace_id=trace_id,
                status="failed",
            )
            return self.get_risk_scan_result(risk_scan_run_id)

        self._validate_risk_analysis_candidate(result)
        findings = [
            self.store.create_risk_finding(
                project_id=project["id"],
                risk_scan_run_id=risk_scan_run_id,
                finding=finding,
            )
            for finding in result["findings"]
        ]
        quality_signals = [
            self.store.create_quality_signal(
                project_id=project["id"],
                risk_scan_run_id=risk_scan_run_id,
                signal=signal,
            )
            for signal in result["quality_signals"]
        ]
        architecture_map = self.store.create_architecture_map_snapshot(
            project_id=project["id"],
            risk_scan_run_id=risk_scan_run_id,
            snapshot=result["architecture_map_snapshot"],
        )
        health = self.store.create_project_health_snapshot(
            project_id=project["id"],
            risk_scan_run_id=risk_scan_run_id,
            snapshot=result["project_health_snapshot"],
        )
        for finding in findings:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=risk_scan_run_id,
                event_type="risk_finding.created",
                payload={
                    "risk_finding_id": finding.id,
                    "severity": finding.severity,
                    "category": finding.category,
                },
            )
        for signal in quality_signals:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=risk_scan_run_id,
                event_type="quality_signal.created",
                payload={"quality_signal_id": signal.id, "kind": signal.kind, "status": signal.status},
            )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            event_type="architecture_map.created",
            payload={"architecture_map_snapshot_id": architecture_map.id},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            event_type="project_health_snapshot.created",
            payload={"project_health_snapshot_id": health.id, "overall_status": health.overall_status},
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=risk_scan_run_id,
            artifact_type="risk_analysis_candidate",
            title="Risk Analysis Candidate",
            payload={
                "finding_count": len(findings),
                "quality_signal_count": len(quality_signals),
                "overall_status": health.overall_status,
                "source_context": risk_scan_run["source_context"],
            },
        )
        self.store.update_risk_scan_run(
            risk_scan_run_id,
            status="completed",
            current_phase="completed",
            trace_id=trace_id,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            event_type="risk_scan.completed",
            payload={"risk_scan_run_id": risk_scan_run_id},
        )
        self._record_trace_from_risk_scan(
            project=project,
            session=session,
            risk_scan_run_id=risk_scan_run_id,
            trace_id=trace_id,
            status="completed",
        )
        return self.get_risk_scan_result(risk_scan_run_id)

    def get_risk_scan_result(self, risk_scan_run_id: str) -> dict[str, Any]:
        run = self.store.get_risk_scan_run(risk_scan_run_id)
        try:
            trace = self.store.get_trace_summary(risk_scan_run_id)
        except KeyError:
            trace = None
        return {
            "risk_scan_run": run,
            "findings": self.store.list_risk_findings(
                project_id=run["project_id"],
                risk_scan_run_id=risk_scan_run_id,
            ),
            "quality_signals": self.store.list_quality_signals(
                project_id=run["project_id"],
                risk_scan_run_id=risk_scan_run_id,
            ),
            "project_health_snapshot": self.store.get_project_health_snapshot_by_risk_scan(risk_scan_run_id),
            "architecture_map_snapshot": self.store.get_architecture_map_by_risk_scan(risk_scan_run_id),
            "trace": trace,
            "events": self.store.list_events(risk_scan_run_id),
            "artifacts": self.store.list_artifacts(risk_scan_run_id),
        }

    def get_risk_radar(
        self,
        *,
        project_id: str,
        category: str | None = None,
        severity: str | None = None,
        status: str | None = None,
    ) -> dict[str, Any]:
        findings = self.store.list_risk_findings(
            project_id=project_id,
            category=category,
            severity=severity,
            status=status,
        )
        severity_counts: dict[str, int] = {severity_key: 0 for severity_key in ALLOWED_RISK_FINDING_SEVERITIES}
        category_counts: dict[str, int] = {category_key: 0 for category_key in ALLOWED_RISK_FINDING_CATEGORIES}
        status_counts: dict[str, int] = {status_key: 0 for status_key in ALLOWED_RISK_FINDING_STATUSES}
        for finding in findings:
            severity_counts[finding["severity"]] = severity_counts.get(finding["severity"], 0) + 1
            category_counts[finding["category"]] = category_counts.get(finding["category"], 0) + 1
            status_counts[finding["status"]] = status_counts.get(finding["status"], 0) + 1
        return {
            "project_id": project_id,
            "latest_scan": (self.store.list_risk_scan_runs(project_id=project_id, limit=1) or [None])[0],
            "health": self.store.get_latest_project_health_snapshot(project_id),
            "findings": findings,
            "severity_counts": severity_counts,
            "category_counts": category_counts,
            "status_counts": status_counts,
        }

    def get_quality_snapshot(self, *, project_id: str) -> dict[str, Any]:
        latest_scan = (self.store.list_risk_scan_runs(project_id=project_id, limit=1) or [None])[0]
        latest_scan_id = latest_scan["id"] if latest_scan else None
        return {
            "project_id": project_id,
            "latest_scan": latest_scan,
            "health": self.store.get_latest_project_health_snapshot(project_id),
            "signals": self.store.list_quality_signals(
                project_id=project_id,
                risk_scan_run_id=latest_scan_id,
            )
            if latest_scan_id
            else [],
            "architecture_map": self.store.get_latest_architecture_map_snapshot(project_id),
        }

    def update_risk_finding_status(self, *, risk_finding_id: str, status: str) -> dict[str, Any]:
        if status not in {"open", "accepted", "dismissed", "mitigated"}:
            raise ValueError("RiskFinding status must be open, accepted, dismissed, or mitigated")
        current = self.store.get_risk_finding(risk_finding_id)
        if current["status"] == "converted" and status != "converted":
            raise ValueError("Converted findings cannot be reopened in MVP 6")
        updated = self.store.update_risk_finding(risk_finding_id, status=status)
        run = self.store.get_risk_scan_run(updated["risk_scan_run_id"])
        event_type = {
            "accepted": "risk_finding.accepted",
            "dismissed": "risk_finding.dismissed",
            "mitigated": "risk_finding.mitigated",
        }.get(status, "risk_finding.updated")
        self.store.append_event(
            project_id=updated["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type=event_type,
            payload={"risk_finding_id": risk_finding_id, "status": status},
        )
        return updated

    def convert_risk_finding_to_work_package(
        self,
        *,
        risk_finding_id: str,
        title: str | None = None,
        goal: str | None = None,
    ) -> dict[str, Any]:
        finding = self.store.get_risk_finding(risk_finding_id)
        if finding["converted_work_package_id"]:
            return {
                "risk_finding": finding,
                "work_package": self.store.get_work_package(finding["converted_work_package_id"]),
                "approval": self.store.get_approval_for_target(
                    target_type="work_package",
                    target_id=finding["converted_work_package_id"],
                ),
            }
        if finding["status"] != "accepted":
            raise ValueError("Only accepted RiskFindings can be converted")
        run = self.store.get_risk_scan_run(finding["risk_scan_run_id"])
        evidence_files = [
            link.get("source_id", "")
            for link in finding["source_links"]
            if link.get("source_type") == "repository_file"
        ]
        draft = {
            "title": title or finding["title"],
            "goal": goal or finding["recommendation"],
            "background": f"Converted from RiskFinding {finding['id']}: {finding['summary']}",
            "scope": [
                "Review the linked risk finding evidence.",
                "Create a bounded remediation plan.",
                "Keep remediation behind existing approval and implementation gates.",
            ],
            "out_of_scope": [
                "Starting implementation automatically.",
                "Applying patches without approval.",
                "Running new verification commands from the risk scan path.",
            ],
            "related_files": evidence_files or ["docs/artemis_mvp6.md"],
            "required_agents": ["Architect", "Planner", "QA"],
            "implementation_steps": [
                "Confirm the risk finding still applies.",
                "Design the smallest follow-up slice that mitigates the risk.",
                "Use the MVP 3 implementation pipeline after Work Package approval.",
            ],
            "verification": [
                "backend contract test",
                "GUI smoke test when user-facing",
                "review result captures residual risk",
            ],
            "risks": [
                {
                    "level": finding["severity"] if finding["severity"] in {"low", "medium", "high", "critical"} else "low",
                    "description": finding["summary"],
                }
            ],
            "approval_required": True,
            "completion_criteria": [
                "Risk evidence has a clear mitigation path.",
                "Converted Work Package is pending approval.",
                "No implementation run starts automatically.",
            ],
        }
        package = self.store.create_work_package(
            project_id=finding["project_id"],
            session_id=run["session_id"],
            source_agent_run_id=run["id"],
            draft=draft,
        )
        approval = self.store.create_approval_request(
            project_id=finding["project_id"],
            session_id=run["session_id"],
            target_type="work_package",
            target_id=package.id,
            reason="MVP 6 converted an accepted RiskFinding into a Work Package candidate.",
            risk_level=package.risks[0]["level"] if package.risks else "medium",
        )
        updated = self.store.update_risk_finding(
            risk_finding_id,
            status="converted",
            converted_work_package_id=package.id,
        )
        self.store.append_event(
            project_id=finding["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type="risk_finding.converted_to_work_package",
            payload={
                "risk_finding_id": risk_finding_id,
                "work_package_id": package.id,
                "approval_id": approval.id,
            },
        )
        self.store.append_event(
            project_id=finding["project_id"],
            session_id=run["session_id"],
            agent_run_id=run["id"],
            event_type="work_package.pending_approval",
            payload={"work_package_id": package.id},
        )
        return {
            "risk_finding": updated,
            "work_package": package.to_dict(),
            "approval": approval.to_dict(),
        }

    def create_manual_memory_item(
        self,
        *,
        project_id: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        memory_type = payload.get("type", "project_rule")
        if memory_type != "project_rule":
            raise ValueError("Manual memory creation is limited to project_rule in MVP 5")
        body = str(payload.get("body") or "").strip()
        if self._contains_secret(body):
            raise ValueError("Memory body looks like it contains a secret or credential")
        title = str(payload.get("title") or "").strip()
        summary = str(payload.get("summary") or "").strip()
        if not title or not summary or not body:
            raise ValueError("title, summary, and body are required")
        project = self.store.get_project(project_id)
        item = self.store.create_memory_item(
            project_id=project["id"],
            memory_type=memory_type,
            title=title,
            summary=summary,
            body=body,
            tags=self._normalize_tags(payload.get("tags") or ["project_rule"]),
            importance=str(payload.get("importance") or "medium"),
            confidence=float(payload.get("confidence") or 1.0),
            created_by="user",
            source_links=[
                {
                    "source_type": "manual",
                    "source_id": "",
                    "relation": "derived_from",
                }
            ],
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=str(payload.get("session_id") or ""),
            agent_run_id=item.id,
            event_type="project_rule.created",
            payload={"memory_item_id": item.id},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=str(payload.get("session_id") or ""),
            agent_run_id=item.id,
            event_type="memory.item.created",
            payload={"memory_item_id": item.id, "type": memory_type},
        )
        return self.store.get_memory_item(item.id)

    def update_memory_item(self, *, memory_item_id: str, payload: dict[str, Any]) -> dict[str, Any]:
        current = self.store.get_memory_item(memory_item_id)
        if "type" in payload and payload["type"] != current["type"]:
            raise ValueError("Memory type cannot be changed")
        if "status" in payload and payload["status"] not in ALLOWED_MEMORY_STATUSES:
            raise ValueError("Unsupported memory status")
        if "body" in payload and self._contains_secret(str(payload["body"])):
            raise ValueError("Memory body looks like it contains a secret or credential")
        updates = dict(payload)
        if "tags" in updates:
            updates["tags"] = self._normalize_tags(updates["tags"])
        item = self.store.update_memory_item(memory_item_id, **updates)
        self.store.append_event(
            project_id=item["project_id"],
            session_id="",
            agent_run_id=item["id"],
            event_type="memory.item.updated",
            payload={"memory_item_id": item["id"]},
        )
        return item

    def archive_memory_item(self, *, memory_item_id: str) -> dict[str, Any]:
        item = self.store.update_memory_item(memory_item_id, status="archived")
        self.store.append_event(
            project_id=item["project_id"],
            session_id="",
            agent_run_id=item["id"],
            event_type="memory.item.archived",
            payload={"memory_item_id": item["id"]},
        )
        return item

    def restore_memory_item(self, *, memory_item_id: str) -> dict[str, Any]:
        item = self.store.update_memory_item(memory_item_id, status="active")
        self.store.append_event(
            project_id=item["project_id"],
            session_id="",
            agent_run_id=item["id"],
            event_type="memory.item.restored",
            payload={"memory_item_id": item["id"]},
        )
        return item

    def promote_decision_record_to_memory(self, *, decision_record_id: str) -> dict[str, Any]:
        record = self.store.get_decision_record(decision_record_id)
        brief = self.store.get_decision_brief_by_brainstorming_session(
            record["brainstorming_session_id"]
        )
        if brief is None or brief["status"] != "accepted":
            raise ValueError("Only accepted DecisionRecords can be promoted to memory")
        existing = self.store.find_memory_by_source(
            project_id=record["project_id"],
            memory_type="decision",
            source_type="decision_record",
            source_id=decision_record_id,
        )
        if existing is not None:
            return existing
        return self._create_memory_from_agent_candidate(
            project_id=record["project_id"],
            session_id=record["session_id"],
            source_type="decision_record",
            source_id=decision_record_id,
            source_snapshot={"decision_record": record, "decision_brief": brief},
            created_by="agent",
            completed_event_type="decision_record.promoted_to_memory",
        )

    def create_session_memory_summary(self, *, session_id: str) -> dict[str, Any]:
        session = self.store.get_session(session_id)
        project = self.store.get_project(session["project_id"])
        existing = self.store.find_memory_by_source(
            project_id=project["id"],
            memory_type="session_summary",
            source_type="session",
            source_id=session_id,
        )
        if existing is not None:
            return {"status": "completed", "memory_item": existing}
        events = self.store.list_session_events(session_id)
        decisions = [
            record
            for record in self.store.list_decision_records(project["id"])
            if record["session_id"] == session_id
        ]
        work_packages = self._list_session_work_packages(session_id)
        if not events and not decisions and not work_packages:
            return {
                "status": "blocked",
                "reason": "Session has no memory-worthy activity.",
                "memory_item": None,
            }
        item = self._create_memory_from_agent_candidate(
            project_id=project["id"],
            session_id=session_id,
            source_type="session",
            source_id=session_id,
            source_snapshot={
                "session": session,
                "events": events,
                "decision_records": decisions,
                "work_packages": work_packages,
            },
            created_by="agent",
            completed_event_type="session_summary.created",
        )
        return {"status": "completed", "memory_item": item}

    def get_session_memory_summary(self, *, session_id: str) -> dict[str, Any]:
        session = self.store.get_session(session_id)
        item = self.store.find_memory_by_source(
            project_id=session["project_id"],
            memory_type="session_summary",
            source_type="session",
            source_id=session_id,
        )
        return {"status": "completed" if item else "missing", "memory_item": item}

    def promote_review_result_failure_memory(self, *, review_result_id: str) -> dict[str, Any]:
        review = self.store.get_review_result_by_id(review_result_id)
        if review["status"] not in {"needs_changes", "blocked"}:
            raise ValueError("Only needs_changes or blocked ReviewResult can create failure memory")
        run = self.store.get_implementation_run(review["implementation_run_id"])
        existing = self.store.find_memory_by_source(
            project_id=run["project_id"],
            memory_type="failure",
            source_type="review_result",
            source_id=review_result_id,
        )
        if existing is not None:
            return existing
        return self._create_memory_from_agent_candidate(
            project_id=run["project_id"],
            session_id=run["session_id"],
            source_type="review_result",
            source_id=review_result_id,
            source_snapshot={"review_result": review, "implementation_run": run},
            created_by="agent",
            completed_event_type="failure_memory.created",
        )

    def promote_verification_failure_memory(self, *, verification_run_id: str) -> dict[str, Any]:
        verification = self.store.get_verification_run_by_id(verification_run_id)
        if verification["status"] not in {"failed", "blocked"}:
            raise ValueError("Only failed or blocked VerificationRun can create failure memory")
        run = self.store.get_implementation_run(verification["implementation_run_id"])
        existing = self.store.find_memory_by_source(
            project_id=run["project_id"],
            memory_type="failure",
            source_type="verification_run",
            source_id=verification_run_id,
        )
        if existing is not None:
            return existing
        return self._create_memory_from_agent_candidate(
            project_id=run["project_id"],
            session_id=run["session_id"],
            source_type="verification_run",
            source_id=verification_run_id,
            source_snapshot={"verification_run": verification, "implementation_run": run},
            created_by="agent",
            completed_event_type="failure_memory.created",
        )

    def select_memory_for_session(self, *, session_id: str, memory_item_id: str) -> dict[str, Any]:
        session = self.store.get_session(session_id)
        item = self.store.get_memory_item(memory_item_id)
        if item["project_id"] != session["project_id"]:
            raise ValueError("Memory item does not belong to this session's project")
        selected = self.store.add_selected_memory(
            session_id=session_id,
            memory_item_id=memory_item_id,
        )
        self.store.append_event(
            project_id=session["project_id"],
            session_id=session_id,
            agent_run_id=session_id,
            event_type="memory.item.selected",
            payload={"memory_item_id": memory_item_id},
        )
        return selected

    def unselect_memory_for_session(self, *, session_id: str, memory_item_id: str) -> None:
        session = self.store.get_session(session_id)
        self.store.remove_selected_memory(session_id=session_id, memory_item_id=memory_item_id)
        self.store.append_event(
            project_id=session["project_id"],
            session_id=session_id,
            agent_run_id=session_id,
            event_type="memory.item.unselected",
            payload={"memory_item_id": memory_item_id},
        )

    def selected_memory_context_payload(self, *, session_id: str) -> dict[str, Any]:
        selected = self.store.list_selected_memory(session_id)
        return {
            "session_id": session_id,
            "selected_memory": selected,
            "source_context": [item["snapshot"] for item in selected],
        }

    def _validate_risk_scope(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        scope_type: str,
        scope_id: str | None,
    ) -> None:
        if session["project_id"] != project["id"]:
            raise ValueError("Session does not belong to project")
        if scope_type in {"project", "session"}:
            return
        if not scope_id:
            raise ValueError(f"scope_id is required for scope_type {scope_type}")
        try:
            if scope_type == "work_package":
                item = self.store.get_work_package(scope_id)
                if item["project_id"] != project["id"]:
                    raise ValueError("WorkPackage does not belong to project")
                return
            if scope_type == "implementation_run":
                item = self.store.get_implementation_run(scope_id)
                if item["project_id"] != project["id"]:
                    raise ValueError("ImplementationRun does not belong to project")
                return
            if scope_type == "review_result":
                review = self.store.get_review_result_by_id(scope_id)
                run = self.store.get_implementation_run(review["implementation_run_id"])
                if run["project_id"] != project["id"]:
                    raise ValueError("ReviewResult does not belong to project")
                return
            if scope_type == "memory_focus":
                memory = self.store.get_memory_item(scope_id)
                if memory["project_id"] != project["id"] or memory["status"] != "active":
                    raise ValueError("Memory focus must be an active item in this project")
                return
        except KeyError as exc:
            raise ValueError(f"Unknown risk scan scope: {scope_type}:{scope_id}") from exc
        raise ValueError(f"Unsupported risk scan scope_type: {scope_type}")

    def _source_context_for_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        scope_type: str,
        scope_id: str | None,
        include_selected_memory: bool,
        selected_memory_ids: list[str],
        focus: list[str],
    ) -> dict[str, Any]:
        selected_memory = self._selected_memory_snapshots_for_risk_scan(
            project=project,
            session=session,
            include_selected_memory=include_selected_memory,
            selected_memory_ids=selected_memory_ids,
        )
        recent_work_packages = self.store.list_work_packages_by_session(session["id"])[:20]
        implementation_runs = self.store.list_implementation_runs_by_project(
            project_id=project["id"],
            limit=30,
        )
        verification_runs = self.store.list_verification_runs_by_project(
            project_id=project["id"],
            limit=50,
        )
        review_results = self.store.list_review_results_by_project(
            project_id=project["id"],
            limit=50,
        )
        decision_records = self.store.list_decision_records(project["id"])[:30]
        failure_memories = self.store.list_memory_items(
            project_id=project["id"],
            memory_type="failure",
            status="active",
            limit=30,
        )
        project_rules = self.store.list_memory_items(
            project_id=project["id"],
            memory_type="project_rule",
            status="active",
            limit=30,
        )
        return {
            "project": project,
            "session": session,
            "scope_type": scope_type,
            "scope_id": scope_id,
            "focus": list(focus),
            "selected_memory_snapshots": selected_memory,
            "recent_work_packages": recent_work_packages,
            "implementation_runs": implementation_runs,
            "verification_runs": verification_runs,
            "review_results": review_results,
            "decision_records": decision_records,
            "failure_memories": failure_memories,
            "project_rules": project_rules,
            "pending_approvals": self.store.list_approval_requests(
                project_id=project["id"],
                status="pending",
                limit=50,
            ),
            "repository_metrics": self._collect_repository_metrics(project["root_path"]),
        }

    def _selected_memory_snapshots_for_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        include_selected_memory: bool,
        selected_memory_ids: list[str],
    ) -> list[dict[str, Any]]:
        if not include_selected_memory:
            if selected_memory_ids:
                raise ValueError("selected_memory_ids require include_selected_memory=true")
            return []
        selected_rows = self.store.list_selected_memory(session["id"])
        selected_by_id = {row["memory_item_id"]: row for row in selected_rows}
        requested_ids = selected_memory_ids or list(selected_by_id)
        snapshots: list[dict[str, Any]] = []
        for memory_item_id in requested_ids:
            if memory_item_id not in selected_by_id:
                raise ValueError("RiskScan can only include memory explicitly selected in the current session")
            memory = self.store.get_memory_item(memory_item_id)
            if memory["project_id"] != project["id"]:
                raise ValueError("Selected memory does not belong to this project")
            if memory["status"] != "active":
                raise ValueError("Archived or superseded memory cannot be attached to a RiskScan")
            snapshots.append(selected_by_id[memory_item_id]["snapshot"])
        return snapshots

    def _collect_repository_metrics(self, root_path: str) -> dict[str, Any]:
        root = Path(root_path).resolve()
        ignored = {".git", ".venv", "__pycache__", ".pytest_cache", "node_modules", "dist", "build", "data"}
        file_count = 0
        total_lines = 0
        top_level_counts: dict[str, int] = {}
        extension_counts: dict[str, int] = {}
        kind_counts: dict[str, int] = {"source": 0, "test": 0, "doc": 0, "script": 0, "config": 0}
        samples: list[dict[str, Any]] = []
        if not root.exists():
            return {
                "file_count": 0,
                "total_lines": 0,
                "top_level_counts": {},
                "extension_counts": {},
                "kind_counts": kind_counts,
                "has_tests": False,
                "repository_file_samples": [],
            }
        for path in root.rglob("*"):
            if any(part in ignored for part in path.relative_to(root).parts):
                continue
            if not path.is_file():
                continue
            file_count += 1
            relative = path.relative_to(root).as_posix()
            top_level = relative.split("/", 1)[0]
            top_level_counts[top_level] = top_level_counts.get(top_level, 0) + 1
            suffix = path.suffix.lower() or "(none)"
            extension_counts[suffix] = extension_counts.get(suffix, 0) + 1
            kind = self._classify_file_kind(relative)
            kind_counts[kind] = kind_counts.get(kind, 0) + 1
            line_count = 0
            try:
                if path.stat().st_size < 256_000:
                    line_count = len(path.read_text(encoding="utf-8", errors="replace").splitlines())
            except OSError:
                line_count = 0
            total_lines += line_count
            if len(samples) < 40:
                samples.append(
                    {
                        "path": relative,
                        "extension": suffix,
                        "top_level": top_level,
                        "kind": kind,
                        "line_count": line_count,
                        "size": path.stat().st_size,
                    }
                )
            if file_count >= 1200:
                break
        return {
            "file_count": file_count,
            "total_lines": total_lines,
            "top_level_counts": top_level_counts,
            "extension_counts": extension_counts,
            "kind_counts": kind_counts,
            "has_tests": kind_counts.get("test", 0) > 0,
            "repository_file_samples": samples,
        }

    def _classify_file_kind(self, relative_path: str) -> str:
        lowered = relative_path.lower()
        if lowered.startswith("tests/") or "/test" in lowered or lowered.endswith(".spec.ts"):
            return "test"
        if lowered.startswith("docs/") or lowered.endswith(".md"):
            return "doc"
        if lowered.startswith("scripts/"):
            return "script"
        if lowered.endswith((".toml", ".json", ".yaml", ".yml", ".ini", ".cfg", ".lock")):
            return "config"
        return "source"

    def _validate_risk_analysis_candidate(self, result: dict[str, Any]) -> None:
        if result.get("project_health_snapshot") is None:
            raise ValueError("RiskAnalysisCandidate requires project_health_snapshot")
        if result.get("architecture_map_snapshot") is None:
            raise ValueError("RiskAnalysisCandidate requires architecture_map_snapshot")
        for finding in result.get("findings") or []:
            if finding.get("category") not in ALLOWED_RISK_FINDING_CATEGORIES:
                raise ValueError(f"Unsupported RiskFinding category: {finding.get('category')}")
            if finding.get("severity") not in ALLOWED_RISK_FINDING_SEVERITIES:
                raise ValueError(f"Unsupported RiskFinding severity: {finding.get('severity')}")
            if not finding.get("source_links"):
                raise ValueError("RiskFinding requires source links")
        for signal in result.get("quality_signals") or []:
            if signal.get("kind") not in ALLOWED_QUALITY_SIGNAL_KINDS:
                raise ValueError(f"Unsupported QualitySignal kind: {signal.get('kind')}")
            if signal.get("status") not in ALLOWED_QUALITY_SIGNAL_STATUSES:
                raise ValueError(f"Unsupported QualitySignal status: {signal.get('status')}")
            if not signal.get("source_links"):
                raise ValueError("QualitySignal requires source links")

    def _create_memory_from_agent_candidate(
        self,
        *,
        project_id: str,
        session_id: str,
        source_type: str,
        source_id: str,
        source_snapshot: dict[str, Any],
        created_by: str,
        completed_event_type: str,
    ) -> dict[str, Any]:
        if source_type not in ALLOWED_MEMORY_SOURCE_TYPES:
            raise ValueError(f"Unsupported memory source_type: {source_type}")
        project = self.store.get_project(project_id)
        extraction = self.store.create_memory_extraction_run(
            project_id=project_id,
            session_id=session_id,
            source_type=source_type,
            source_id=source_id,
        )
        self.store.append_event(
            project_id=project_id,
            session_id=session_id,
            agent_run_id=extraction.id,
            event_type="memory.extraction_run.created",
            payload={"extraction_run_id": extraction.id, "source_type": source_type},
        )
        self.store.update_memory_extraction_run(extraction.id, status="running")
        result = self.agent_backend.create_memory_candidate(
            {
                "project_id": project_id,
                "session_id": session_id,
                "extraction_run_id": extraction.id,
                "project_root": project["root_path"],
                "source_type": source_type,
                "source_id": source_id,
                "source_snapshot": source_snapshot,
            }
        )
        for event in result["events"]:
            self.store.append_event(
                project_id=project_id,
                session_id=session_id,
                agent_run_id=extraction.id,
                event_type=event["type"],
                payload=event["payload"],
            )
        trace_id = result.get("trace_id")
        self.store.update_memory_extraction_run(extraction.id, trace_id=trace_id)
        if result["status"] != "completed" or result.get("candidate") is None:
            self.store.update_memory_extraction_run(extraction.id, status="failed")
            self._record_trace_from_memory_extraction_run(
                project_id=project_id,
                session_id=session_id,
                extraction_run_id=extraction.id,
                trace_id=trace_id,
                status="failed",
            )
            raise ValueError("; ".join(result.get("errors") or ["Memory candidate generation failed"]))
        candidate = self.store.create_memory_candidate(
            extraction_run_id=extraction.id,
            candidate=result["candidate"],
        )
        self.store.update_memory_extraction_run(
            extraction.id,
            status="candidate_ready",
            candidate_count=1,
        )
        memory = self.store.create_memory_item(
            project_id=project_id,
            memory_type=candidate.type,
            title=candidate.title,
            summary=candidate.summary,
            body=candidate.body,
            tags=candidate.tags,
            importance=candidate.importance,
            confidence=candidate.confidence,
            created_by=created_by,
            source_links=candidate.source_links,
        )
        self.store.update_memory_candidate_status(candidate.id, "accepted")
        self.store.update_memory_extraction_run(
            extraction.id,
            status="completed",
            created_memory_count=1,
        )
        self.store.append_event(
            project_id=project_id,
            session_id=session_id,
            agent_run_id=extraction.id,
            event_type="memory.candidate.accepted",
            payload={"candidate_id": candidate.id, "memory_item_id": memory.id},
        )
        self.store.append_event(
            project_id=project_id,
            session_id=session_id,
            agent_run_id=extraction.id,
            event_type="memory.item.created",
            payload={"memory_item_id": memory.id, "type": memory.type},
        )
        self.store.append_event(
            project_id=project_id,
            session_id=session_id,
            agent_run_id=extraction.id,
            event_type=completed_event_type,
            payload={"memory_item_id": memory.id, "source_type": source_type, "source_id": source_id},
        )
        self._record_trace_from_memory_extraction_run(
            project_id=project_id,
            session_id=session_id,
            extraction_run_id=extraction.id,
            trace_id=trace_id,
            status="completed",
        )
        return self.store.get_memory_item(memory.id)

    def _normalize_tags(self, raw_tags: Any) -> list[str]:
        if isinstance(raw_tags, str):
            candidates = [raw_tags]
        else:
            candidates = list(raw_tags or [])
        tags: list[str] = []
        for tag in candidates:
            normalized = str(tag).strip().lower().replace(" ", "-")
            if normalized and normalized not in tags:
                tags.append(normalized)
        return tags or ["memory"]

    def _command_center_next_action(
        self,
        *,
        pending_approvals: list[dict[str, Any]],
        risk_findings: list[dict[str, Any]],
        failures: list[dict[str, Any]],
        selected_memory: list[dict[str, Any]],
        session: dict[str, Any] | None,
    ) -> dict[str, Any]:
        if pending_approvals:
            approval = pending_approvals[0]
            return {
                "kind": "approval",
                "label": "Review pending approval",
                "target_type": approval["target_type"],
                "target_id": approval["target_id"],
                "reason": approval["reason"],
            }
        high_risk = next(
            (finding for finding in risk_findings if finding["severity"] in {"critical", "high"}),
            None,
        )
        if high_risk:
            return {
                "kind": "risk_finding",
                "label": "Triage highest risk finding",
                "target_type": "risk_finding",
                "target_id": high_risk["id"],
                "reason": high_risk["summary"],
            }
        if failures:
            failed = failures[0]
            return {
                "kind": "failed_run",
                "label": "Inspect failed run trace",
                "target_type": failed["type"],
                "target_id": failed["id"],
                "reason": f"{failed['type']} is {failed['status']}.",
            }
        if session is not None and not selected_memory:
            return {
                "kind": "memory",
                "label": "Select memory context",
                "target_type": "session",
                "target_id": session["id"],
                "reason": "No explicit memory context is selected for this session.",
            }
        return {
            "kind": "work_package",
            "label": "Create or continue a Work Package",
            "target_type": "session" if session else "project",
            "target_id": session["id"] if session else "",
            "reason": "No blocking approval, high risk, or failed run is currently visible.",
        }

    def _contains_secret(self, text: str) -> bool:
        lowered = text.lower()
        return any(marker in lowered for marker in SECRET_MARKERS) or "sk-" in lowered

    def _list_session_work_packages(self, session_id: str) -> list[dict[str, Any]]:
        return self.store.list_work_packages_by_session(session_id)

    def _normalize_brainstorming_roles(self, roles: list[str], mode: str) -> list[str]:
        selected: list[str] = []
        for role in roles:
            normalized = role.strip().lower().replace("-", "_")
            if normalized and normalized not in selected:
                selected.append(normalized)
        if not selected:
            selected = list(DEFAULT_BRAINSTORMING_ROLES[mode])
        if len(selected) > 6:
            raise ValueError("Brainstorming role count cannot exceed 6")
        return selected

    def _source_context_for_brainstorming(
        self,
        *,
        source_type: str,
        source_id: str | None,
    ) -> dict[str, Any]:
        if source_type == "topic":
            return {}
        if not source_id:
            raise ValueError(f"source_id is required for source_type {source_type}")
        try:
            if source_type == "work_package":
                return {"work_package": self.store.get_work_package(source_id)}
            if source_type == "implementation_run":
                return {"implementation_run": self.store.get_implementation_run(source_id)}
            if source_type == "review_result":
                return {"review_result": self.store.get_review_result_by_id(source_id)}
        except KeyError as exc:
            raise ValueError(f"Unknown brainstorming source: {source_type}:{source_id}") from exc
        raise ValueError(f"Unsupported brainstorming source_type: {source_type}")

    def _record_trace_from_brainstorming_session(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        brainstorming_session_id: str,
        trace_id: str | None,
        status: str,
    ) -> None:
        if trace_id is None:
            return
        self.store.record_trace_summary(
            trace_id=trace_id,
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=brainstorming_session_id,
            root_name="artemis_brainstorming_session",
            status=status,
            metadata={"event_count": len(self.store.list_events(brainstorming_session_id))},
            events=self.store.list_events(brainstorming_session_id),
        )

    def _record_trace_from_memory_extraction_run(
        self,
        *,
        project_id: str,
        session_id: str,
        extraction_run_id: str,
        trace_id: str | None,
        status: str,
    ) -> None:
        if trace_id is None:
            return
        self.store.record_trace_summary(
            trace_id=trace_id,
            project_id=project_id,
            session_id=session_id,
            agent_run_id=extraction_run_id,
            root_name="artemis_memory_extraction",
            status=status,
            metadata={"event_count": len(self.store.list_events(extraction_run_id))},
            events=self.store.list_events(extraction_run_id),
        )

    def _record_trace_from_risk_scan(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        risk_scan_run_id: str,
        trace_id: str | None,
        status: str,
    ) -> None:
        if trace_id is None:
            return
        run = self.store.get_risk_scan_run(risk_scan_run_id)
        self.store.record_trace_summary(
            trace_id=trace_id,
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=risk_scan_run_id,
            root_name="artemis_risk_scan",
            status=status,
            metadata={
                "event_count": len(self.store.list_events(risk_scan_run_id)),
                "scope_type": run["scope_type"],
                "scope_id": run["scope_id"],
                "selected_memory_count": run["selected_memory_count"],
                "source_context": run["source_context"],
            },
            events=self.store.list_events(risk_scan_run_id),
        )

    def _record_trace_from_run(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        agent_run_id: str,
        trace_id: str | None,
        external_trace_id: str | None,
        status: str,
    ) -> None:
        if trace_id is None:
            return
        self.store.record_trace_summary(
            trace_id=trace_id,
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=agent_run_id,
            root_name="artemis_agent_run",
            status=status,
            metadata={
                "external_trace_id": external_trace_id,
                "event_count": len(self.store.list_events(agent_run_id)),
            },
            events=self.store.list_events(agent_run_id),
        )

    def _record_trace_from_implementation_run(
        self,
        *,
        project: dict[str, Any],
        session: dict[str, Any],
        implementation_run_id: str,
        trace_id: str | None,
        status: str,
    ) -> None:
        if trace_id is None:
            return
        self.store.record_trace_summary(
            trace_id=trace_id,
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=implementation_run_id,
            root_name="artemis_implementation_run",
            status=status,
            metadata={"event_count": len(self.store.list_events(implementation_run_id))},
            events=self.store.list_events(implementation_run_id),
        )

    def _create_artifact_and_event(
        self,
        *,
        project_id: str,
        session_id: str,
        source_agent_run_id: str,
        artifact_type: str,
        title: str,
        payload: dict[str, Any],
    ) -> dict[str, Any]:
        artifact = self.store.create_artifact(
            project_id=project_id,
            session_id=session_id,
            source_agent_run_id=source_agent_run_id,
            artifact_type=artifact_type,
            title=title,
            payload=payload,
        )
        self.store.append_event(
            project_id=project_id,
            session_id=session_id,
            agent_run_id=source_agent_run_id,
            event_type="artifact.created",
            payload={
                "artifact_id": artifact.id,
                "type": artifact.type,
                "title": artifact.title,
            },
        )
        return artifact.to_dict()

    def _apply_patch_files(self, root: Path, files: list[dict[str, Any]]) -> list[str]:
        targets: list[tuple[Path, dict[str, Any], bool, str | None]] = []
        for file in files:
            if file["operation"] == "delete":
                raise ValueError("Delete operation apply is blocked in MVP 3")
            if "\x00" in file.get("replacement_content", ""):
                raise ValueError(f"Binary patch content is blocked: {file['path']}")
            path = self._resolve_patch_path(root, file["path"])
            if path.exists() and not path.is_file():
                raise ValueError(f"Patch target is not a regular file: {file['path']}")
            if path.exists() and path.is_symlink():
                raise ValueError(f"Symlink patch target is blocked: {file['path']}")
            try:
                original = path.read_text(encoding="utf-8") if path.exists() else None
            except UnicodeDecodeError as exc:
                raise ValueError(f"Binary or non-UTF-8 patch target is blocked: {file['path']}") from exc
            targets.append((path, file, path.exists(), original))

        applied: list[str] = []
        try:
            for path, file, _existed, _original in targets:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(file["replacement_content"], encoding="utf-8")
                applied.append(file["path"])
        except Exception:
            for path, _file, existed, original in reversed(targets):
                if existed and original is not None:
                    path.write_text(original, encoding="utf-8")
                elif path.exists():
                    path.unlink()
            raise
        return applied

    def _resolve_patch_path(self, root: Path, raw_path: str) -> Path:
        relative = Path(raw_path)
        if relative.is_absolute() or ".." in relative.parts:
            raise ValueError(f"Patch path escapes project root: {raw_path}")
        candidate = (root / relative).resolve()
        try:
            candidate.relative_to(root)
        except ValueError as exc:
            raise ValueError(f"Patch path escapes project root: {raw_path}") from exc
        return candidate

    def _infer_verification_command(self, root: Path) -> str | None:
        if (root / "tests").is_dir():
            return "python -m unittest discover -s tests"
        if (root / "package.json").is_file():
            return "npm run build"
        return None

    def _command_args(self, command: str) -> list[str]:
        if command == "python -m unittest discover -s tests":
            return [sys.executable, "-m", "unittest", "discover", "-s", "tests"]
        if command == "pytest":
            return [sys.executable, "-m", "pytest"]
        if command.startswith("npm "):
            npm = shutil.which("npm.cmd") or shutil.which("npm")
            if npm is None:
                raise RuntimeError("npm executable was not found")
            return [npm, *command.split()[1:]]
        raise ValueError("Command is not in the MVP 3 verification allowlist")
