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
    ImplementationPlan,
    ImplementationRun,
    PatchFile,
    PatchSet,
    Project,
    ReviewResult,
    Session,
    Trace,
    TraceStep,
    VerificationRun,
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
                  trace_id TEXT,
                  external_trace_id TEXT,
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
                CREATE TABLE IF NOT EXISTS traces (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  agent_run_id TEXT NOT NULL,
                  root_name TEXT NOT NULL,
                  status TEXT NOT NULL,
                  started_at TEXT NOT NULL,
                  ended_at TEXT,
                  metadata TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS trace_steps (
                  id TEXT PRIMARY KEY,
                  trace_id TEXT NOT NULL,
                  parent_step_id TEXT,
                  name TEXT NOT NULL,
                  type TEXT NOT NULL,
                  status TEXT NOT NULL,
                  inputs_summary TEXT NOT NULL,
                  outputs_summary TEXT NOT NULL,
                  started_at TEXT NOT NULL,
                  ended_at TEXT
                );
                CREATE TABLE IF NOT EXISTS implementation_runs (
                  id TEXT PRIMARY KEY,
                  project_id TEXT NOT NULL,
                  session_id TEXT NOT NULL,
                  work_package_id TEXT NOT NULL,
                  status TEXT NOT NULL,
                  current_phase TEXT,
                  trace_id TEXT,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS implementation_plans (
                  id TEXT PRIMARY KEY,
                  implementation_run_id TEXT NOT NULL,
                  goal TEXT NOT NULL,
                  context_summary TEXT NOT NULL,
                  target_files TEXT NOT NULL,
                  steps TEXT NOT NULL,
                  verification_strategy TEXT NOT NULL,
                  risks TEXT NOT NULL,
                  created_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS patch_sets (
                  id TEXT PRIMARY KEY,
                  implementation_run_id TEXT NOT NULL,
                  status TEXT NOT NULL,
                  summary TEXT NOT NULL,
                  risk_level TEXT NOT NULL,
                  approval_status TEXT NOT NULL,
                  applied_files TEXT NOT NULL,
                  created_at TEXT NOT NULL,
                  updated_at TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS patch_files (
                  id TEXT PRIMARY KEY,
                  patch_set_id TEXT NOT NULL,
                  path TEXT NOT NULL,
                  operation TEXT NOT NULL,
                  diff TEXT NOT NULL,
                  rationale TEXT NOT NULL,
                  risk_level TEXT NOT NULL,
                  replacement_content TEXT NOT NULL
                );
                CREATE TABLE IF NOT EXISTS verification_runs (
                  id TEXT PRIMARY KEY,
                  implementation_run_id TEXT NOT NULL,
                  command TEXT NOT NULL,
                  status TEXT NOT NULL,
                  exit_code INTEGER,
                  stdout TEXT NOT NULL,
                  stderr TEXT NOT NULL,
                  started_at TEXT NOT NULL,
                  ended_at TEXT
                );
                CREATE TABLE IF NOT EXISTS review_results (
                  id TEXT PRIMARY KEY,
                  implementation_run_id TEXT NOT NULL,
                  status TEXT NOT NULL,
                  findings TEXT NOT NULL,
                  residual_risks TEXT NOT NULL,
                  recommendation TEXT NOT NULL,
                  created_at TEXT NOT NULL
                );
                """
            )
            self._ensure_column(db, "agent_runs", "trace_id", "TEXT")
            self._ensure_column(db, "agent_runs", "external_trace_id", "TEXT")
            self._ensure_column(db, "patch_sets", "applied_files", "TEXT NOT NULL DEFAULT '[]'")

    def _ensure_column(
        self,
        db: sqlite3.Connection,
        table_name: str,
        column_name: str,
        column_type: str,
    ) -> None:
        columns = {row["name"] for row in db.execute(f"PRAGMA table_info({table_name})").fetchall()}
        if column_name not in columns:
            db.execute(f"ALTER TABLE {table_name} ADD COLUMN {column_name} {column_type}")

    def create_project(self, name: str, root_path: str) -> Project:
        now = utc_now()
        project = Project(new_id("proj"), name, root_path, "active", now, now)
        with self._connect() as db:
            db.execute(
                "INSERT INTO projects VALUES (?, ?, ?, ?, ?, ?)",
                (project.id, project.name, project.root_path, project.status, project.created_at, project.updated_at),
            )
        return project

    def list_projects(self) -> list[dict[str, Any]]:
        with self._connect() as db:
            rows = db.execute("SELECT * FROM projects ORDER BY updated_at DESC").fetchall()
        return [dict(row) for row in rows]

    def get_project(self, project_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM projects WHERE id=?", (project_id,)).fetchone()
        if row is None:
            raise KeyError(project_id)
        return dict(row)

    def create_session(self, project_id: str, title: str) -> Session:
        now = utc_now()
        session = Session(new_id("sess"), project_id, title, "active", now, now)
        with self._connect() as db:
            db.execute(
                "INSERT INTO sessions VALUES (?, ?, ?, ?, ?, ?)",
                (session.id, session.project_id, session.title, session.status, session.created_at, session.updated_at),
            )
        return session

    def list_sessions(self, project_id: str | None = None) -> list[dict[str, Any]]:
        with self._connect() as db:
            if project_id is None:
                rows = db.execute("SELECT * FROM sessions ORDER BY updated_at DESC").fetchall()
            else:
                rows = db.execute(
                    "SELECT * FROM sessions WHERE project_id=? ORDER BY updated_at DESC",
                    (project_id,),
                ).fetchall()
        return [dict(row) for row in rows]

    def get_session(self, session_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM sessions WHERE id=?", (session_id,)).fetchone()
        if row is None:
            raise KeyError(session_id)
        return dict(row)

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
            trace_id=None,
            external_trace_id=None,
            created_at=now,
            updated_at=now,
        )
        with self._connect() as db:
            db.execute(
                """
                INSERT INTO agent_runs (
                  id,
                  project_id,
                  session_id,
                  user_request,
                  status,
                  intent,
                  current_phase,
                  trace_id,
                  external_trace_id,
                  created_at,
                  updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    run.id,
                    run.project_id,
                    run.session_id,
                    run.user_request,
                    run.status,
                    run.intent,
                    run.current_phase,
                    run.trace_id,
                    run.external_trace_id,
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
        trace_id: str | None = None,
        external_trace_id: str | None = None,
    ) -> None:
        updates: dict[str, Any] = {"updated_at": utc_now()}
        if status is not None:
            updates["status"] = status
        if intent is not None:
            updates["intent"] = intent
        if current_phase is not None:
            updates["current_phase"] = current_phase
        if trace_id is not None:
            updates["trace_id"] = trace_id
        if external_trace_id is not None:
            updates["external_trace_id"] = external_trace_id
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
                package = db.execute(
                    "SELECT source_agent_run_id FROM work_packages WHERE id=?",
                    (approval["target_id"],),
                ).fetchone()
                approval["source_agent_run_id"] = (
                    package["source_agent_run_id"] if package is not None else None
                )
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

    def list_artifacts(self, agent_run_id: str) -> list[dict[str, Any]]:
        with self._connect() as db:
            rows = db.execute(
                "SELECT * FROM artifacts WHERE source_agent_run_id=? ORDER BY created_at ASC",
                (agent_run_id,),
            ).fetchall()
        artifacts: list[dict[str, Any]] = []
        for row in rows:
            artifact = dict(row)
            artifact["payload"] = json.loads(artifact["payload"])
            artifacts.append(artifact)
        return artifacts

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

    def list_events(self, agent_run_id: str, after: str | None = None) -> list[dict[str, Any]]:
        if not self.event_log_path.exists():
            return []
        events: list[dict[str, Any]] = []
        with self.event_log_path.open("r", encoding="utf-8") as fh:
            for line in fh:
                event = json.loads(line)
                if event.get("agent_run_id") == agent_run_id:
                    events.append(event)
        if after is None:
            return events
        for index, event in enumerate(events):
            if event["id"] == after:
                return events[index + 1 :]
        return events

    def get_work_package_by_agent_run(self, agent_run_id: str) -> dict[str, Any] | None:
        with self._connect() as db:
            row = db.execute(
                "SELECT id FROM work_packages WHERE source_agent_run_id=? ORDER BY created_at DESC LIMIT 1",
                (agent_run_id,),
            ).fetchone()
        if row is None:
            return None
        return self.get_work_package(row["id"])

    def get_approval_for_target(self, *, target_type: str, target_id: str) -> dict[str, Any] | None:
        with self._connect() as db:
            row = db.execute(
                """
                SELECT * FROM approval_requests
                WHERE target_type=? AND target_id=?
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (target_type, target_id),
            ).fetchone()
        if row is None:
            return None
        return dict(row)

    def record_trace_summary(
        self,
        *,
        trace_id: str,
        project_id: str,
        session_id: str,
        agent_run_id: str,
        root_name: str,
        status: str,
        metadata: dict[str, Any],
        events: list[dict[str, Any]],
    ) -> Trace:
        now = utc_now()
        started_at = events[0]["created_at"] if events else now
        ended_at = now if status in {"completed", "failed", "canceled"} else None
        trace = Trace(
            id=trace_id,
            project_id=project_id,
            session_id=session_id,
            agent_run_id=agent_run_id,
            root_name=root_name,
            status=status,
            started_at=started_at,
            ended_at=ended_at,
            metadata=metadata,
        )

        steps: list[TraceStep] = []
        phase_events = [
            event
            for event in events
            if event["type"] in {"agent_run.phase_changed", "implementation_run.phase_changed"}
        ]
        for index, event in enumerate(phase_events, start=1):
            phase = str(event.get("payload", {}).get("phase", f"phase_{index}"))
            steps.append(
                TraceStep(
                    id=new_id("step"),
                    trace_id=trace.id,
                    parent_step_id=None,
                    name=phase,
                    type="graph_node",
                    status="completed",
                    inputs_summary="",
                    outputs_summary=json.dumps(event.get("payload", {}), ensure_ascii=False),
                    started_at=event["created_at"],
                    ended_at=event["created_at"],
                )
            )
        if not steps:
            steps.append(
                TraceStep(
                    id=new_id("step"),
                    trace_id=trace.id,
                    parent_step_id=None,
                    name=root_name,
                    type="root",
                    status=status,
                    inputs_summary="",
                    outputs_summary=json.dumps({"status": status}, ensure_ascii=False),
                    started_at=started_at,
                    ended_at=ended_at,
                )
            )

        with self._connect() as db:
            db.execute(
                """
                INSERT OR REPLACE INTO traces VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    trace.id,
                    trace.project_id,
                    trace.session_id,
                    trace.agent_run_id,
                    trace.root_name,
                    trace.status,
                    trace.started_at,
                    trace.ended_at,
                    json.dumps(trace.metadata, ensure_ascii=False),
                ),
            )
            db.execute("DELETE FROM trace_steps WHERE trace_id=?", (trace.id,))
            db.executemany(
                """
                INSERT INTO trace_steps VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                [
                    (
                        step.id,
                        step.trace_id,
                        step.parent_step_id,
                        step.name,
                        step.type,
                        step.status,
                        step.inputs_summary,
                        step.outputs_summary,
                        step.started_at,
                        step.ended_at,
                    )
                    for step in steps
                ],
            )
        return trace

    def get_trace_summary(self, agent_run_id: str) -> dict[str, Any]:
        with self._connect() as db:
            trace_row = db.execute(
                "SELECT * FROM traces WHERE agent_run_id=? ORDER BY started_at DESC LIMIT 1",
                (agent_run_id,),
            ).fetchone()
            if trace_row is None:
                raise KeyError(agent_run_id)
            step_rows = db.execute(
                "SELECT * FROM trace_steps WHERE trace_id=? ORDER BY started_at ASC, id ASC",
                (trace_row["id"],),
            ).fetchall()
        trace = dict(trace_row)
        trace["metadata"] = json.loads(trace["metadata"])
        return {
            "trace": trace,
            "steps": [dict(row) for row in step_rows],
        }

    def get_agent_run(self, run_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM agent_runs WHERE id=?", (run_id,)).fetchone()
        if row is None:
            raise KeyError(run_id)
        data = dict(row)
        data.setdefault("external_trace_id", None)
        return data

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

    def create_implementation_run(
        self,
        *,
        project_id: str,
        session_id: str,
        work_package_id: str,
    ) -> ImplementationRun:
        now = utc_now()
        run = ImplementationRun(
            id=new_id("impl"),
            project_id=project_id,
            session_id=session_id,
            work_package_id=work_package_id,
            status="queued",
            current_phase=None,
            trace_id=None,
            created_at=now,
            updated_at=now,
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO implementation_runs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    run.id,
                    run.project_id,
                    run.session_id,
                    run.work_package_id,
                    run.status,
                    run.current_phase,
                    run.trace_id,
                    run.created_at,
                    run.updated_at,
                ),
            )
        return run

    def update_implementation_run(
        self,
        implementation_run_id: str,
        *,
        status: str | None = None,
        current_phase: str | None = None,
        trace_id: str | None = None,
    ) -> None:
        updates: dict[str, Any] = {"updated_at": utc_now()}
        if status is not None:
            updates["status"] = status
        if current_phase is not None:
            updates["current_phase"] = current_phase
        if trace_id is not None:
            updates["trace_id"] = trace_id
        assignments = ", ".join(f"{key}=?" for key in updates)
        with self._connect() as db:
            db.execute(
                f"UPDATE implementation_runs SET {assignments} WHERE id=?",
                (*updates.values(), implementation_run_id),
            )

    def get_implementation_run(self, implementation_run_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute(
                "SELECT * FROM implementation_runs WHERE id=?",
                (implementation_run_id,),
            ).fetchone()
        if row is None:
            raise KeyError(implementation_run_id)
        return dict(row)

    def create_implementation_plan(
        self,
        *,
        implementation_run_id: str,
        plan: dict[str, Any],
    ) -> ImplementationPlan:
        item = ImplementationPlan(
            id=new_id("plan"),
            implementation_run_id=implementation_run_id,
            goal=plan["goal"],
            context_summary=plan["context_summary"],
            target_files=list(plan["target_files"]),
            steps=list(plan["steps"]),
            verification_strategy=list(plan["verification_strategy"]),
            risks=list(plan["risks"]),
            created_at=utc_now(),
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO implementation_plans VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    item.id,
                    item.implementation_run_id,
                    item.goal,
                    item.context_summary,
                    json.dumps(item.target_files, ensure_ascii=False),
                    json.dumps(item.steps, ensure_ascii=False),
                    json.dumps(item.verification_strategy, ensure_ascii=False),
                    json.dumps(item.risks, ensure_ascii=False),
                    item.created_at,
                ),
            )
        return item

    def get_implementation_plan(self, implementation_run_id: str) -> dict[str, Any] | None:
        with self._connect() as db:
            row = db.execute(
                """
                SELECT * FROM implementation_plans
                WHERE implementation_run_id=?
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (implementation_run_id,),
            ).fetchone()
        if row is None:
            return None
        data = dict(row)
        for key in ("target_files", "steps", "verification_strategy", "risks"):
            data[key] = json.loads(data[key])
        return data

    def create_patch_set(
        self,
        *,
        implementation_run_id: str,
        patch_set: dict[str, Any],
    ) -> PatchSet:
        now = utc_now()
        item = PatchSet(
            id=new_id("patch"),
            implementation_run_id=implementation_run_id,
            status="pending_approval",
            summary=patch_set["summary"],
            risk_level=patch_set["risk_level"],
            approval_status="pending",
            applied_files=[],
            created_at=now,
            updated_at=now,
        )
        files = [
            PatchFile(
                id=new_id("pfile"),
                patch_set_id=item.id,
                path=file["path"],
                operation=file["operation"],
                diff=file["diff"],
                rationale=file["rationale"],
                risk_level=file["risk_level"],
                replacement_content=file.get("replacement_content", ""),
            )
            for file in patch_set["files"]
        ]
        with self._connect() as db:
            db.execute(
                "INSERT INTO patch_sets VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    item.id,
                    item.implementation_run_id,
                    item.status,
                    item.summary,
                    item.risk_level,
                    item.approval_status,
                    json.dumps(item.applied_files, ensure_ascii=False),
                    item.created_at,
                    item.updated_at,
                ),
            )
            db.executemany(
                "INSERT INTO patch_files VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                [
                    (
                        file.id,
                        file.patch_set_id,
                        file.path,
                        file.operation,
                        file.diff,
                        file.rationale,
                        file.risk_level,
                        file.replacement_content,
                    )
                    for file in files
                ],
            )
        return item

    def get_patch_set(self, patch_set_id: str) -> dict[str, Any]:
        with self._connect() as db:
            row = db.execute("SELECT * FROM patch_sets WHERE id=?", (patch_set_id,)).fetchone()
            if row is None:
                raise KeyError(patch_set_id)
            file_rows = db.execute(
                "SELECT * FROM patch_files WHERE patch_set_id=? ORDER BY id ASC",
                (patch_set_id,),
            ).fetchall()
        data = dict(row)
        data["applied_files"] = json.loads(data["applied_files"])
        data["files"] = [dict(file_row) for file_row in file_rows]
        return data

    def get_patch_set_by_implementation_run(
        self,
        implementation_run_id: str,
    ) -> dict[str, Any] | None:
        with self._connect() as db:
            row = db.execute(
                """
                SELECT id FROM patch_sets
                WHERE implementation_run_id=?
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (implementation_run_id,),
            ).fetchone()
        if row is None:
            return None
        return self.get_patch_set(row["id"])

    def update_patch_set(
        self,
        patch_set_id: str,
        *,
        status: str | None = None,
        approval_status: str | None = None,
        applied_files: list[str] | None = None,
    ) -> None:
        updates: dict[str, Any] = {"updated_at": utc_now()}
        if status is not None:
            updates["status"] = status
        if approval_status is not None:
            updates["approval_status"] = approval_status
        if applied_files is not None:
            updates["applied_files"] = json.dumps(applied_files, ensure_ascii=False)
        assignments = ", ".join(f"{key}=?" for key in updates)
        with self._connect() as db:
            db.execute(
                f"UPDATE patch_sets SET {assignments} WHERE id=?",
                (*updates.values(), patch_set_id),
            )

    def create_verification_run(
        self,
        *,
        implementation_run_id: str,
        command: str,
        status: str,
        exit_code: int | None,
        stdout: str,
        stderr: str,
        started_at: str | None = None,
        ended_at: str | None = None,
    ) -> VerificationRun:
        started = started_at or utc_now()
        item = VerificationRun(
            id=new_id("verify"),
            implementation_run_id=implementation_run_id,
            command=command,
            status=status,
            exit_code=exit_code,
            stdout=stdout,
            stderr=stderr,
            started_at=started,
            ended_at=ended_at or utc_now(),
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO verification_runs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    item.id,
                    item.implementation_run_id,
                    item.command,
                    item.status,
                    item.exit_code,
                    item.stdout,
                    item.stderr,
                    item.started_at,
                    item.ended_at,
                ),
            )
        return item

    def list_verification_runs(self, implementation_run_id: str) -> list[dict[str, Any]]:
        with self._connect() as db:
            rows = db.execute(
                """
                SELECT * FROM verification_runs
                WHERE implementation_run_id=?
                ORDER BY started_at ASC
                """,
                (implementation_run_id,),
            ).fetchall()
        return [dict(row) for row in rows]

    def create_review_result(
        self,
        *,
        implementation_run_id: str,
        review: dict[str, Any],
    ) -> ReviewResult:
        item = ReviewResult(
            id=new_id("review"),
            implementation_run_id=implementation_run_id,
            status=review["status"],
            findings=list(review["findings"]),
            residual_risks=list(review["residual_risks"]),
            recommendation=review["recommendation"],
            created_at=utc_now(),
        )
        with self._connect() as db:
            db.execute(
                "INSERT INTO review_results VALUES (?, ?, ?, ?, ?, ?, ?)",
                (
                    item.id,
                    item.implementation_run_id,
                    item.status,
                    json.dumps(item.findings, ensure_ascii=False),
                    json.dumps(item.residual_risks, ensure_ascii=False),
                    item.recommendation,
                    item.created_at,
                ),
            )
        return item

    def get_review_result(self, implementation_run_id: str) -> dict[str, Any] | None:
        with self._connect() as db:
            row = db.execute(
                """
                SELECT * FROM review_results
                WHERE implementation_run_id=?
                ORDER BY created_at DESC
                LIMIT 1
                """,
                (implementation_run_id,),
            ).fetchone()
        if row is None:
            return None
        data = dict(row)
        data["findings"] = json.loads(data["findings"])
        data["residual_risks"] = json.loads(data["residual_risks"])
        return data
