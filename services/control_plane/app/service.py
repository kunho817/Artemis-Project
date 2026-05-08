"""Control Plane orchestration service."""

from __future__ import annotations

from pathlib import Path
from typing import Any

from .agent_client import AgentBackendClient, HTTPAgentBackendClient
from .storage import SQLiteStore


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
