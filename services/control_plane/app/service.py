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
