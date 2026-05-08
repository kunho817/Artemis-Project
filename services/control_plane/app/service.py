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

    def create_work_package_from_request(
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
        self.store.update_agent_run(run.id, status="running", current_phase="agent_backend")
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="agent_run.started",
            payload={},
        )

        backend_result = self.agent_backend.run_agent(
            {
                "project_id": project["id"],
                "session_id": session["id"],
                "agent_run_id": run.id,
                "user_request": user_request,
                "project_root": project["root_path"],
            }
        )
        for event in backend_result["events"]:
            self.store.append_event(
                project_id=project["id"],
                session_id=session["id"],
                agent_run_id=run.id,
                event_type=event["type"],
                payload=event["payload"],
            )

        self.store.update_agent_run(
            run.id,
            status=backend_result["status"],
            intent=backend_result["intent_result"]["intent"],
            current_phase="completed" if backend_result["status"] == "completed" else "failed",
            langsmith_trace_id=backend_result["langsmith_trace_id"],
        )

        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=run.id,
            artifact_type="intent_result",
            title="Intent Result",
            payload=backend_result["intent_result"],
        )
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=run.id,
            artifact_type="context_summary",
            title="Context Summary",
            payload=backend_result["context_summary"],
        )

        if backend_result["status"] != "completed" or backend_result["work_package"] is None:
            return {
                "agent_run_id": run.id,
                "status": backend_result["status"],
                "errors": backend_result["errors"],
                "langsmith_trace_id": backend_result["langsmith_trace_id"],
            }

        draft = backend_result["work_package"]
        self._create_artifact_and_event(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=run.id,
            artifact_type="work_package_draft",
            title=draft["title"],
            payload=draft,
        )
        package = self.store.create_work_package(
            project_id=project["id"],
            session_id=session["id"],
            source_agent_run_id=run.id,
            draft=draft,
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="work_package.created",
            payload={"work_package_id": package.id},
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="work_package.pending_approval",
            payload={"work_package_id": package.id},
        )

        approval = self.store.create_approval_request(
            project_id=project["id"],
            session_id=session["id"],
            target_type="work_package",
            target_id=package.id,
            reason="MVP 1 requires approval before any implementation work.",
            risk_level=package.risks[0]["level"] if package.risks else "medium",
        )
        self.store.append_event(
            project_id=project["id"],
            session_id=session["id"],
            agent_run_id=run.id,
            event_type="approval.requested",
            payload={
                "approval_id": approval.id,
                "target_type": approval.target_type,
                "target_id": approval.target_id,
            },
        )

        return {
            "project_id": project["id"],
            "session_id": session["id"],
            "agent_run_id": run.id,
            "work_package_id": package.id,
            "approval_id": approval.id,
            "status": "pending_approval",
            "langsmith_trace_id": backend_result["langsmith_trace_id"],
        }

    def resolve_approval(self, *, approval_id: str, status: str) -> dict[str, Any]:
        approval = self.store.resolve_approval(approval_id, status)
        event_type = "approval.approved" if status == "approved" else "approval.rejected"
        self.store.append_event(
            project_id=approval["project_id"],
            session_id=approval["session_id"],
            agent_run_id=None,
            event_type=event_type,
            payload={
                "approval_id": approval["id"],
                "target_type": approval["target_type"],
                "target_id": approval["target_id"],
            },
        )
        return approval

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
