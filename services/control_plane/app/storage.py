"""SQLite and JSONL persistence for MVP 1."""

from __future__ import annotations

from contextlib import contextmanager
from pathlib import Path
import json
import sqlite3
from typing import Any, Iterator

from .models import (
    AgentRun,
    ApprovalRequest,
    Artifact,
    Event,
    Project,
    Session,
    WorkPackage,
    new_id,
    utc_now,
)


class SQLiteStore:
    def __init__(self, db_path: str | Path, event_log_path: str | Path | None = None) -> None:
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self.event_log_path = Path(event_log_path) if event_log_path else self.db_path.with_suffix(".events.jsonl")
        self.event_log_path.parent.mkdir(parents=True, exist_ok=True)
        self._init_schema()

    @contextmanager
    def _connect(self) -> Iterator[sqlite3.Connection]:
        connection = sqlite3.connect(self.db_path)
        connection.row_factory = sqlite3.Row
        try:
            yield connection
            connection.commit()
        finally:
            connection.close()

    def _init_schema(self) -> None:
        with self._connect() as db:
            db.executescript(
                """
                CREATE TABLE IF NOT EXISTS projects (
                  id TEXT PRIMARY KEY,
                  name TEXT NOT NULL,
                  root_path TEXT NOT NULL,
                  status TEXT NOT NULL,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS sessions (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  title TEXT NOT NULL,
                  status TEXT NOT NULL,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS agent_runs (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  user_request TEXT NOT NULL,
                  status TEXT NOT NULL,
                  intent TEXT,
                  current_phase TEXT,
                  langsmith_trace_id TEXT,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS work_packages (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  source_agent_run_id TEXT NOT NULL,
                  title TEXT NOT NULL,
                  goal TEXT NOT NULL,
                  background TEXT NOT NULL,
                  scope TEXT NOT NULL,
                  out_of_scope TEXT NOT NULL,
                  related_files TEXT NOT NULL,
                  required_agents TEXT NOT NULL,
                  implementation_steps TEXT NOT NULL,
                  verification TEXT NOT NULL,
                  risks TEXT NOT NULL,
                  approval_required INTEGER NOT NULL,
                  approval_status TEXT NOT NULL,
                  completion_criteria TEXT NOT NULL,
                  status TEXT NOT NULL,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS approval_requests (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  target_type TEXT NOT NULL,
                  target_id TEXT NOT NULL,
                  reason TEXT NOT NULL,
                  risk_level TEXT NOT NULL,
                  status TEXT NOT NULL,
                  created_at TEXT NOT NULL,
                  resolved_at TEXT
                );
                CREATE TABLE IF NOT EXISTS artifacts (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  source_agent_run_id TEXT NOT NULL,
                  type TEXT NOT NULL,
                  title TEXT NOT NULL,
                  payload TEXT NOT NULL,
                  created_at TEXT NOT NULL
                );
                """
            )

    def create_project(self, name: str, root_path: str) -> Project:
        now = utc_now()
        project = Project(new_id("proj"), name, root_path, "active", now, now)
        with self._connect() as db:
            db.execute(
                "INSERT INTO projects VALUES (?, ?, ?, ?, ?, ?)",
                (project.id, project.name, project.root_path, project.status, project.created_at, project.updated_at),
            )
        return project

    def create_session(self, project_id: str, title: str) -> Session:
        now = utc_now()
        session = Session(new_id("sess"), project_id, title, "active", now, now)
        with self._connect() as db:
            db.execute(
                "INSERT INTO sessions VALUES (?, ?, ?, ?, ?, ?)",
                (session.id, session.project_id, session.title, session.status, session.created_at, session.updated_at),
            )
        return session

    def create_agent_run(self, project_id: str, session_id: str, user_request: str) -> AgentRun:
        now = utc_now()
        run = AgentRun(
            id=new_id("run"),
            project_id=project_id,
            session_id=session_id,
            user_request=user_request,
            status="queued",
            intent=None,
            current_phase=None,
            langsmith_trace_id=None,
            created_at=now,
            updated_at=now,
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO agent_runs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    run.id,
                    run.project_id,
                    run.session_id,
                    run.user_request,
                    run.status,
                    run.intent,
                    run.current_phase,
                    run.langsmith_trace_id,
                    run.created_at,
                    run.updated_at,
                ),
            )
        return run

    def update_agent_run(
        self,
        run_id: str,
        *,
        status: str | None = None,
        intent: str | None = None,
        current_phase: str | None = None,
        langsmith_trace_id: str | None = None,
    ) -> None:
        updates: dict[str, Any] = {"updated_at": utc_now()}
        if status is not None:
            updates["status"] = status
        if intent is not None:
            updates["intent"] = intent
        if current_phase is not None:
            updates["current_phase"] = current_phase
        if langsmith_trace_id is not None:
            updates["langsmith_trace_id"] = langsmith_trace_id
        assignments = ", ".join(f"{key}=?" for key in updates)
        with self._connect() as db:
            db.execute(
                f"UPDATE agent_runs SET {assignments} WHERE id=?",
                (*updates.values(), run_id),
            )

    def create_work_package(
        self,
        *,
        project_id: str,
        session_id: str,
        source_agent_run_id: str,
        draft: dict[str, Any],
    ) -> WorkPackage:
        now = utc_now()
        package = WorkPackage(
            id=new_id("wp"),
            project_id=project_id,
            session_id=session_id,
            source_agent_run_id=source_agent_run_id,
            title=draft["title"],
            goal=draft["goal"],
            background=draft["background"],
            scope=list(draft["scope"]),
            out_of_scope=list(draft["out_of_scope"]),
            related_files=list(draft["related_files"]),
            required_agents=list(draft["required_agents"]),
            implementation_steps=list(draft["implementation_steps"]),
            verification=list(draft["verification"]),
            risks=list(draft["risks"]),
            approval_required=bool(draft["approval_required"]),
            approval_status="pending" if draft["approval_required"] else "not_required",
            completion_criteria=list(draft["completion_criteria"]),
            status="pending_approval" if draft["approval_required"] else "approved",
            created_at=now,
            updated_at=now,
        )
        with self._connect() as db:
            db.execute(
                """
                INSERT INTO work_packages VALUES (
                  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
                )
                """,
                (
                    package.id,
                    package.project_id,
                    package.session_id,
                    package.source_agent_run_id,
                    package.title,
                    package.goal,
                    package.background,
                    json.dumps(package.scope, ensure_ascii=False),
                    json.dumps(package.out_of_scope, ensure_ascii=False),
                    json.dumps(package.related_files, ensure_ascii=False),
                    json.dumps(package.required_agents, ensure_ascii=False),
                    json.dumps(package.implementation_steps, ensure_ascii=False),
                    json.dumps(package.verification, ensure_ascii=False),
                    json.dumps(package.risks, ensure_ascii=False),
                    int(package.approval_required),
                    package.approval_status,
                    json.dumps(package.completion_criteria, ensure_ascii=False),
                    package.status,
                    package.created_at,
                    package.updated_at,
                ),
            )
        return package

    def create_approval_request(
        self,
        *,
        project_id: str,
        session_id: str,
        target_type: str,
        target_id: str,
        reason: str,
        risk_level: str,
    ) -> ApprovalRequest:
        now = utc_now()
        approval = ApprovalRequest(
            id=new_id("approval"),
            project_id=project_id,
            session_id=session_id,
            target_type=target_type,
            target_id=target_id,
            reason=reason,
            risk_level=risk_level,
            status="pending",
            created_at=now,
            resolved_at=None,
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO approval_requests VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    approval.id,
                    approval.project_id,
                    approval.session_id,
                    approval.target_type,
                    approval.target_id,
                    approval.reason,
                    approval.risk_level,
                    approval.status,
                    approval.created_at,
                    approval.resolved_at,
                ),
            )
        return approval

    def resolve_approval(self, approval_id: str, status: str) -> dict[str, Any]:
        if status not in {"approved", "rejected"}:
            raise ValueError("approval status must be approved or rejected")
        resolved_at = utc_now()
        with self._connect() as db:
            row = db.execute(
                "SELECT * FROM approval_requests WHERE id=?",
                (approval_id,),
            ).fetchone()
            if row is None:
                raise KeyError(approval_id)
            db.execute(
                "UPDATE approval_requests SET status=?, resolved_at=? WHERE id=?",
                (status, resolved_at, approval_id),
            )
            approval = dict(row)
            approval["status"] = status
            approval["resolved_at"] = resolved_at
            if approval["target_type"] == "work_package":
                package_status = "approved" if status == "approved" else "rejected"
                db.execute(
                    """
                    UPDATE work_packages
                    SET approval_status=?, status=?, updated_at=?
                    WHERE id=?
                    """,
                    (status, package_status, resolved_at, approval["target_id"]),
                )
        return approval

    def create_artifact(
        self,
        *,
        project_id: str,
        session_id: str,
        source_agent_run_id: str,
        artifact_type: str,
        title: str,
        payload: dict[str, Any],
    ) -> Artifact:
        artifact = Artifact(
            id=new_id("artifact"),
            project_id=project_id,
            session_id=session_id,
            source_agent_run_id=source_agent_run_id,
            type=artifact_type,
            title=title,
            payload=payload,
            created_at=utc_now(),
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO artifacts VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    artifact.id,
                    artifact.project_id,
                    artifact.session_id,
                    artifact.source_agent_run_id,
                    artifact.type,
                    artifact.title,
                    json.dumps(artifact.payload, ensure_ascii=False),
                    artifact.created_at,
                ),
            )
        return artifact

    def append_event(
        self,
        *,
        project_id: str,
        session_id: str,
        agent_run_id: str | None,
        event_type: str,
        payload: dict[str, Any],
    ) -> Event:
        event = Event(new_id("evt"), project_id, session_id, agent_run_id, event_type, payload, utc_now())
        with self.event_log_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(event.to_dict(), ensure_ascii=False) + "\n")
        return event

    def list_events(self, agent_run_id: str) -> list[dict[str, Any]]:
        if not self.event_log_path.exists():
            return []
        events: list[dict[str, Any]] = []
        with self.event_log_path.open("r", encoding="utf-8") as fh:
            for line in fh:
                event = json.loads(line)
                if event.get("agent_run_id") == agent_run_id:
                    events.append(event)
        return events

    def get_agent_run(self, run_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM agent_runs WHERE id=?", (run_id,)).fetchone()
        if row is None:
            raise KeyError(run_id)
        return dict(row)

    def get_work_package(self, package_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM work_packages WHERE id=?", (package_id,)).fetchone()
        if row is None:
            raise KeyError(package_id)
        data = dict(row)
        for key in (
            "scope",
            "out_of_scope",
            "related_files",
            "required_agents",
            "implementation_steps",
            "verification",
            "risks",
            "completion_criteria",
        ):
            data[key] = json.loads(data[key])
        data["approval_required"] = bool(data["approval_required"])
        return data
