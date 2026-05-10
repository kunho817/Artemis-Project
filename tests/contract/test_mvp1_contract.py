from __future__ import annotations

import json
from pathlib import Path
import tempfile
import time
import unittest

from services.agent_backend.app.config import model_for_role
from services.agent_backend.app.graph import MVP1GraphRunner, build_langgraph
from services.agent_backend.app.llm import GLMResponse
from services.agent_backend.app.schemas import (
    AgentBackendRequest,
    BrainstormingContributionDraft,
    BrainstormingOptionDraft,
    DecisionBriefDraft,
    MemoryCandidateDraft,
    QualitySignalDraft,
    RiskFindingDraft,
    RiskHint,
    WorkPackageCandidateRequest,
    WorkPackageDraft,
)
from services.agent_backend.app.tools import ReadOnlyToolRouter, ToolPermissionError
from services.control_plane.app.agent_client import InProcessAgentBackendClient
from services.control_plane.app.service import ControlPlaneService
from services.control_plane.app.storage import SQLiteStore


class MVP1ContractTests(unittest.TestCase):
    def test_work_package_schema_validation(self) -> None:
        draft = WorkPackageDraft(
            title="",
            goal="Create a work package",
            background="background",
            scope=["scope"],
            out_of_scope=["writes"],
            related_files=["docs/artemis_mvp1.md"],
            required_agents=["Architect"],
            implementation_steps=["review"],
            verification=["schema validation"],
            risks=[RiskHint(level="low", description="low risk")],
            approval_required=True,
            completion_criteria=["pending approval"],
        )

        self.assertIn("title is required", draft.validate())

    def test_glm_role_model_routing(self) -> None:
        env = {
            "ZAI_API_KEY": "test-key",
            "ARTEMIS_GLM_MODEL_ARCHITECT": "glm-5.1",
            "ARTEMIS_GLM_MODEL_VALIDATOR": "glm-4.6",
            "ARTEMIS_GLM_DEFAULT_MODEL": "glm-5.1",
        }

        self.assertEqual(model_for_role("architect", env).model, "glm-5.1")
        self.assertEqual(model_for_role("validator", env).model, "glm-4.6")
        self.assertEqual(model_for_role("context_collector", env).model, "glm-4.7")
        self.assertEqual(model_for_role("unknown_role", env).model, "glm-5.1")

    def test_read_only_tool_permission(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "README.md").write_text("Artemis MVP 1", encoding="utf-8")
            (root / "node_modules").mkdir()
            (root / "node_modules" / "large.js").write_text("Artemis should be ignored", encoding="utf-8")
            tools = ReadOnlyToolRouter(root)

            self.assertTrue(tools.read_file("README.md").ok)
            self.assertNotIn("node_modules", tools.list_files().output)
            self.assertNotIn("node_modules", tools.grep("ignored").output)
            with self.assertRaises(ToolPermissionError):
                tools.assert_allowed("write_file")
            with self.assertRaises(ToolPermissionError):
                tools.read_file("../outside.txt")

    def test_git_status_allows_current_repository_safe_directory(self) -> None:
        root = Path.cwd()
        if not (root / ".git").exists():
            self.skipTest("repository root is not available")

        result = ReadOnlyToolRouter(root).git_status()
        self.assertTrue(result.ok, result.metadata)

    def test_langgraph_builder_is_real_when_dependency_exists(self) -> None:
        runner = MVP1GraphRunner()
        graph = build_langgraph(runner)
        if graph is None:
            self.skipTest("langgraph is not installed")

        self.assertTrue(hasattr(graph, "invoke"))

    def test_langgraph_validation_failure_path(self) -> None:
        class InvalidDraftRunner(MVP1GraphRunner):
            def _create_work_package_with_metadata(self, request, intent, context):
                return (
                    WorkPackageDraft(
                        title="",
                        goal="",
                        background="",
                        scope=[],
                        out_of_scope=[],
                        related_files=[],
                        required_agents=[],
                        implementation_steps=[],
                        verification=[],
                        risks=[],
                        approval_required=True,
                        completion_criteria=[],
                    ),
                    {"path": "test_invalid", "model": "test", "role": "test", "fallback_reason": None},
                )

        result = InvalidDraftRunner().run(
            request=AgentBackendRequest(
                project_id="project",
                session_id="session",
                agent_run_id="run",
                user_request="Create an invalid draft for validation testing",
                project_root=str(Path.cwd()),
            )
        )

        self.assertEqual(result.status, "failed")
        self.assertTrue(result.errors)
        self.assertIn("work_package.validation_failed", {event.type for event in result.events})

    def test_llm_structured_work_package_path_when_configured(self) -> None:
        class FakeSelection:
            model = "glm-5"
            role = "work_package_writer"

        class FakeClient:
            configured = True
            selection = FakeSelection()

            def invoke(self, messages):
                return GLMResponse(
                    content=json.dumps(
                        {
                            "title": "LLM Draft Work Package",
                            "goal": "Use structured model output for Work Package creation.",
                            "background": "Alpha makes the model path the default when configured.",
                            "scope": ["Parse model JSON", "Validate schema"],
                            "out_of_scope": ["Apply patches without approval"],
                            "related_files": ["services/agent_backend/app/graph.py"],
                            "required_agents": ["Planner", "QA"],
                            "implementation_steps": ["Create draft", "Validate draft"],
                            "verification": ["contract test"],
                            "risks": [{"level": "low", "description": "Malformed output can fall back."}],
                            "approval_required": True,
                            "completion_criteria": ["Generation path is recorded"],
                        }
                    ),
                    model="glm-5",
                    role="work_package_writer",
                )

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "README.md").write_text("Artemis Alpha test", encoding="utf-8")
            runner = MVP1GraphRunner(work_package_client_factory=lambda _role: FakeClient())
            result = runner.run(
                request=AgentBackendRequest(
                    project_id="project",
                    session_id="session",
                    agent_run_id="run",
                    user_request="Create an Alpha Work Package with model output",
                    project_root=str(root),
                )
            )

        generation_event = next(
            event for event in result.events if event.type == "work_package.generation_path"
        )
        self.assertEqual(result.status, "completed")
        self.assertEqual(result.work_package.title, "LLM Draft Work Package")
        self.assertEqual(generation_event.payload["path"], "llm_structured")
        self.assertIsNone(generation_event.payload["fallback_reason"])

    def test_malformed_llm_work_package_output_records_fallback_reason(self) -> None:
        class FakeSelection:
            model = "glm-5"
            role = "work_package_writer"

        class FakeClient:
            configured = True
            selection = FakeSelection()

            def invoke(self, messages):
                return GLMResponse(
                    content="not-json",
                    model="glm-5",
                    role="work_package_writer",
                )

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "README.md").write_text("Artemis Alpha fallback test", encoding="utf-8")
            runner = MVP1GraphRunner(work_package_client_factory=lambda _role: FakeClient())
            result = runner.run(
                request=AgentBackendRequest(
                    project_id="project",
                    session_id="session",
                    agent_run_id="run",
                    user_request="Plan the fallback path",
                    project_root=str(root),
                )
            )

        generation_event = next(
            event for event in result.events if event.type == "work_package.generation_path"
        )
        self.assertEqual(result.status, "completed")
        self.assertEqual(generation_event.payload["path"], "deterministic_fallback")
        self.assertIn("ValueError", generation_event.payload["fallback_reason"])

    def test_control_plane_agent_backend_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis test project", encoding="utf-8")
            (project_root / "AGENTS.md").write_text("Project rules", encoding="utf-8")
            (project_root / "docs").mkdir()
            (project_root / "docs" / "artemis_mvp1.md").write_text(
                "MVP 1 Work Package foundation",
                encoding="utf-8",
            )

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP1 test")

            result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Plan a feature request for the MVP 1 control plane",
            )

            self.assertEqual(result["status"], "pending_approval")
            self.assertTrue(result["trace_id"].startswith("trace_"))
            self.assertIsNone(result["external_trace_id"])

            run = store.get_agent_run(result["agent_run_id"])
            self.assertEqual(run["status"], "completed")
            self.assertEqual(run["intent"], "planning_request")
            self.assertEqual(run["trace_id"], result["trace_id"])

            package = store.get_work_package(result["work_package_id"])
            self.assertEqual(package["status"], "pending_approval")
            self.assertEqual(package["approval_status"], "pending")
            self.assertTrue(package["approval_required"])

            events = store.list_events(result["agent_run_id"])
            event_types = {event["type"] for event in events}
            self.assertIn("agent_run.created", event_types)
            self.assertIn("trace.linked", event_types)
            self.assertIn("agent_run.graph_runtime", event_types)
            self.assertIn("artifact.created", event_types)
            self.assertIn("work_package.pending_approval", event_types)
            self.assertIn("approval.requested", event_types)

            trace = store.get_trace_summary(result["agent_run_id"])
            self.assertEqual(trace["trace"]["id"], result["trace_id"])
            self.assertTrue(trace["steps"])

            artifacts = store.list_artifacts(result["agent_run_id"])
            self.assertEqual(
                {artifact["type"] for artifact in artifacts},
                {"intent_result", "context_summary", "work_package_draft"},
            )

            after_first = store.list_events(result["agent_run_id"], after=events[0]["id"])
            self.assertEqual(after_first[0]["id"], events[1]["id"])

            approval = service.resolve_approval(
                approval_id=result["approval_id"],
                status="approved",
            )
            self.assertEqual(approval["status"], "approved")
            approved_package = store.get_work_package(result["work_package_id"])
            self.assertEqual(approved_package["approval_status"], "approved")
            self.assertEqual(approved_package["status"], "approved")

            result_view = service.get_agent_run_result(result["agent_run_id"])
            self.assertEqual(result_view["approval"]["status"], "approved")
            self.assertEqual(result_view["work_package"]["id"], result["work_package_id"])
            self.assertEqual(result_view["trace"]["trace"]["id"], result["trace_id"])

    def test_alpha_schema_status_is_recorded_idempotently(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            db_path = Path(tmp) / "artemis.db"
            first = SQLiteStore(db_path)
            first_status = first.get_schema_status()
            second = SQLiteStore(db_path)
            second_status = second.get_schema_status()

        self.assertEqual(first_status["status"], "ok")
        self.assertEqual(first_status["current_version"], first_status["stored_version"])
        self.assertEqual(len(first_status["migrations"]), 1)
        self.assertEqual(len(second_status["migrations"]), 1)
        self.assertEqual(second_status["migrations"][0]["id"], "0001_alpha_schema_version")

    def test_alpha_command_center_summarizes_next_action(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis Alpha command center", encoding="utf-8")

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="Alpha command center")

            empty_center = service.get_command_center(
                project_id=project["id"],
                session_id=session["id"],
            )
            self.assertEqual(empty_center["next_action"]["kind"], "memory")
            self.assertEqual(empty_center["counts"]["pending_approvals"], 0)

            result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Create an Alpha verification Work Package.",
            )
            center = service.get_command_center(
                project_id=project["id"],
                session_id=session["id"],
            )

        self.assertEqual(center["counts"]["pending_approvals"], 1)
        self.assertEqual(center["next_action"]["kind"], "approval")
        self.assertEqual(center["pending_approvals"][0]["target_id"], result["work_package_id"])

    def test_control_plane_mvp2_async_api_contract(self) -> None:
        try:
            from fastapi.testclient import TestClient
        except Exception as exc:
            self.skipTest(f"fastapi test client is not available: {exc}")

        from services.control_plane.app.api import create_app

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis test project", encoding="utf-8")
            (project_root / "AGENTS.md").write_text("Project rules", encoding="utf-8")

            app = create_app(
                str(root / "artemis.db"),
                agent_backend=InProcessAgentBackendClient(),
            )
            client = TestClient(app)
            project = client.post(
                "/api/projects/open",
                json={"name": "test", "root_path": str(project_root)},
            ).json()
            session = client.post(
                "/api/sessions",
                json={"project_id": project["id"], "title": "MVP2 API test"},
            ).json()
            queued = client.post(
                "/api/work-package-requests",
                json={
                    "project_id": project["id"],
                    "session_id": session["id"],
                    "user_request": "Create an MVP 2 event stream test.",
                },
            ).json()

            self.assertEqual(queued["status"], "queued")
            self.assertIn("/events/stream", queued["events_url"])

            deadline = time.monotonic() + 5
            run = {}
            while time.monotonic() < deadline:
                run = client.get(f"/api/agent-runs/{queued['agent_run_id']}").json()
                if run["status"] in {"completed", "failed", "canceled"}:
                    break
                time.sleep(0.05)

            self.assertEqual(run["status"], "completed")
            result = client.get(f"/api/agent-runs/{queued['agent_run_id']}/result").json()
            trace = client.get(f"/api/agent-runs/{queued['agent_run_id']}/trace").json()
            artifacts = client.get(f"/api/agent-runs/{queued['agent_run_id']}/artifacts").json()
            events = client.get(f"/api/agent-runs/{queued['agent_run_id']}/events").json()
            polled = client.get(
                f"/api/agent-runs/{queued['agent_run_id']}/events",
                params={"after": events[0]["id"]},
            ).json()

            self.assertEqual(result["agent_run"]["trace_id"], trace["trace"]["id"])
            self.assertIsNotNone(result["work_package"])
            self.assertIsNotNone(result["approval"])
            self.assertGreaterEqual(len(artifacts), 3)
            self.assertEqual(polled[0]["id"], events[1]["id"])

    def test_mvp3_implementation_pipeline_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis test project", encoding="utf-8")
            (project_root / "tests").mkdir()
            (project_root / "tests" / "test_smoke.py").write_text(
                "import unittest\n\n"
                "class SmokeTest(unittest.TestCase):\n"
                "    def test_smoke(self):\n"
                "        self.assertTrue(True)\n",
                encoding="utf-8",
            )

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP3 test")
            work_package_result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Create an implementation pipeline test Work Package.",
            )

            with self.assertRaises(ValueError):
                service.create_implementation_run(
                    work_package_id=work_package_result["work_package_id"],
                )

            service.resolve_approval(
                approval_id=work_package_result["approval_id"],
                status="approved",
            )
            implementation_result = service.create_implementation_run(
                work_package_id=work_package_result["work_package_id"],
            )

            implementation_run = implementation_result["implementation_run"]
            patch_set = implementation_result["patch_set"]
            self.assertEqual(implementation_run["status"], "pending_patch_approval")
            self.assertEqual(patch_set["approval_status"], "pending")
            self.assertEqual(patch_set["files"][0]["operation"], "create")

            with self.assertRaises(ValueError):
                service.apply_patch_set(patch_set_id=patch_set["id"])

            approved_patch_set = service.resolve_patch_set(
                patch_set_id=patch_set["id"],
                status="approved",
            )
            self.assertEqual(approved_patch_set["approval_status"], "approved")

            applied_patch_set = service.apply_patch_set(patch_set_id=patch_set["id"])
            self.assertEqual(applied_patch_set["status"], "applied")
            self.assertIn("docs/artemis_implementation_log.md", applied_patch_set["applied_files"])
            self.assertIn(
                work_package_result["work_package_id"],
                (project_root / "docs" / "artemis_implementation_log.md").read_text(
                    encoding="utf-8"
                ),
            )

            final_result = service.get_implementation_run_result(implementation_run["id"])
            event_types = {event["type"] for event in final_result["events"]}
            self.assertEqual(final_result["implementation_run"]["status"], "completed")
            self.assertEqual(final_result["verification_runs"][0]["status"], "passed")
            self.assertEqual(final_result["review_result"]["status"], "pass")
            self.assertIsNotNone(final_result["trace"])
            self.assertIn("implementation_plan.created", event_types)
            self.assertIn("patch_set.applied", event_types)
            self.assertIn("verification.completed", event_types)
            self.assertIn("review.completed", event_types)

    def test_mvp3_patch_and_verification_policy_blocks_unsafe_actions(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis safety project", encoding="utf-8")

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP3 safety")
            work_package_result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Create a safety policy Work Package.",
            )
            service.resolve_approval(
                approval_id=work_package_result["approval_id"],
                status="approved",
            )
            run = store.create_implementation_run(
                project_id=project["id"],
                session_id=session["id"],
                work_package_id=work_package_result["work_package_id"],
            )

            escape_patch = store.create_patch_set(
                implementation_run_id=run.id,
                patch_set={
                    "summary": "Unsafe escape",
                    "risk_level": "high",
                    "files": [
                        {
                            "path": "../escape.txt",
                            "operation": "update",
                            "diff": "",
                            "rationale": "policy test",
                            "risk_level": "high",
                            "replacement_content": "unsafe",
                        }
                    ],
                },
            )
            store.update_patch_set(
                escape_patch.id,
                status="approved",
                approval_status="approved",
            )
            with self.assertRaises(ValueError):
                service.apply_patch_set(patch_set_id=escape_patch.id)

            delete_patch = store.create_patch_set(
                implementation_run_id=run.id,
                patch_set={
                    "summary": "Unsafe delete",
                    "risk_level": "high",
                    "files": [
                        {
                            "path": "README.md",
                            "operation": "delete",
                            "diff": "",
                            "rationale": "policy test",
                            "risk_level": "high",
                            "replacement_content": "",
                        }
                    ],
                },
            )
            store.update_patch_set(
                delete_patch.id,
                status="approved",
                approval_status="approved",
            )
            with self.assertRaises(ValueError):
                service.apply_patch_set(patch_set_id=delete_patch.id)

            verification = service.run_verification(
                implementation_run_id=run.id,
                command="git reset --hard",
            )
            self.assertEqual(verification["status"], "blocked")

    def test_mvp4_brainstorming_schema_validation(self) -> None:
        contribution = BrainstormingContributionDraft(
            role="system_architect",
            stance="cautious",
            summary="Review boundaries.",
            arguments=["Control Plane owns state."],
            concerns=["Scope could grow."],
            suggested_actions=["Keep structured outputs."],
            referenced_artifacts=["docs/artemis_mvp4.md"],
        )
        option = BrainstormingOptionDraft(
            title="Staged slice",
            summary="Ship the vertical slice.",
            benefits=["Covers completion criteria."],
            costs=["Adds API surface."],
            risks=["May need later LLM replacement."],
            required_work=["Add contracts."],
            verification_hint="Run smoke.",
            score=0.9,
        )
        brief = DecisionBriefDraft(
            recommendation="Choose the staged slice.",
            selected_option_index=0,
            rationale="It satisfies MVP4.",
            tradeoffs=["Deterministic now, LLM later."],
            risks=["Scope can expand."],
            open_questions=["Which source should default?"],
            follow_up_actions=["Add GUI smoke."],
            work_package_candidate=WorkPackageCandidateRequest(
                title="Convert decision",
                goal="Create a Work Package candidate.",
                background="Decision accepted.",
                scope=["Convert accepted decision."],
                out_of_scope=["Run implementation."],
                related_files=["docs/artemis_mvp4.md"],
                required_agents=["Architect"],
                implementation_steps=["Review decision."],
                verification=["contract test"],
                risks=[{"level": "medium", "description": "Needs review."}],
                completion_criteria=["Pending approval."],
            ),
        )

        self.assertEqual(contribution.validate(), [])
        self.assertEqual(option.validate(), [])
        self.assertEqual(brief.validate(1), [])

    def test_mvp4_brainstorming_decision_and_conversion_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis MVP 4 project", encoding="utf-8")
            (project_root / "docs").mkdir()
            (project_root / "docs" / "artemis_mvp4.md").write_text(
                "Brainstorming Room and Decision Record",
                encoding="utf-8",
            )

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP4 test")

            with self.assertRaises(ValueError):
                service.start_brainstorming_session(
                    project=project,
                    session=session,
                    topic="Too many roles",
                    roles=[f"role_{index}" for index in range(7)],
                )
            with self.assertRaises(ValueError):
                service.start_brainstorming_session(
                    project=project,
                    session=session,
                    topic="Bad source",
                    source_type="work_package",
                    source_id="missing",
                )

            result = service.create_brainstorming_session(
                project=project,
                session=session,
                topic="Review MVP 4 Brainstorming Room scope",
                roles=[],
            )
            brain = result["brainstorming_session"]
            self.assertEqual(brain["status"], "awaiting_decision")
            self.assertEqual(brain["mode"], "architecture_debate")
            self.assertIn("devil_advocate", brain["selected_roles"])
            self.assertGreaterEqual(len(result["contributions"]), 4)
            self.assertGreaterEqual(len(result["critiques"]), 4)
            self.assertGreaterEqual(len(result["options"]), 3)
            self.assertEqual(result["decision_brief"]["status"], "pending")
            self.assertIsNotNone(result["trace"])

            event_types = {event["type"] for event in result["events"]}
            self.assertIn("brainstorming.roles_selected", event_types)
            self.assertIn("brainstorming.decision_brief_created", event_types)
            self.assertIn("brainstorming.validation_passed", event_types)

            rejected = service.create_brainstorming_session(
                project=project,
                session=session,
                topic="Reject this decision",
            )
            rejected = service.resolve_decision_brief(
                brainstorming_session_id=rejected["brainstorming_session"]["id"],
                decision_brief_id=rejected["decision_brief"]["id"],
                status="rejected",
                note="Too broad.",
            )
            self.assertEqual(rejected["decision_brief"]["status"], "rejected")
            self.assertIsNone(rejected["decision_record"])

            accepted = service.resolve_decision_brief(
                brainstorming_session_id=brain["id"],
                decision_brief_id=result["decision_brief"]["id"],
                status="accepted",
                note="Use the staged API-first path.",
            )
            record = accepted["decision_record"]
            self.assertIsNotNone(record)
            self.assertEqual(accepted["decision_brief"]["status"], "accepted")
            self.assertIsNone(record["linked_work_package_id"])

            converted = service.convert_decision_record_to_work_package(
                decision_record_id=record["id"],
            )
            self.assertEqual(converted["work_package"]["status"], "pending_approval")
            self.assertEqual(converted["work_package"]["approval_status"], "pending")
            self.assertEqual(converted["approval"]["target_type"], "work_package")

            final_result = service.get_brainstorming_result(brain["id"])
            final_events = {event["type"] for event in final_result["events"]}
            self.assertEqual(final_result["brainstorming_session"]["status"], "converted")
            self.assertEqual(
                final_result["decision_record"]["linked_work_package_id"],
                converted["work_package"]["id"],
            )
            self.assertIn("decision_record.created", final_events)
            self.assertIn("work_package.conversion_completed", final_events)

            source_result = service.create_brainstorming_session(
                project=project,
                session=session,
                topic="Review converted Work Package as a source",
                mode="implementation_strategy",
                source_type="work_package",
                source_id=converted["work_package"]["id"],
            )
            self.assertEqual(source_result["brainstorming_session"]["source_type"], "work_package")
            self.assertEqual(
                source_result["brainstorming_session"]["source_id"],
                converted["work_package"]["id"],
            )

    def test_mvp5_memory_schema_manual_search_and_selected_context(self) -> None:
        candidate = MemoryCandidateDraft(
            type="project_rule",
            title="GUI calls Control Plane only",
            summary="The GUI must use Control Plane APIs.",
            body="All GUI actions go through Control Plane APIs.",
            tags=["architecture", "gui"],
            importance="high",
            confidence=1.0,
            source_links=[
                {
                    "source_type": "manual",
                    "source_id": "manual-input",
                    "relation": "derived_from",
                }
            ],
        )
        self.assertEqual(candidate.validate(), [])

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP5 memory")

            item = service.create_manual_memory_item(
                project_id=project["id"],
                payload={
                    "type": "project_rule",
                    "title": "GUI calls Control Plane only",
                    "summary": "The GUI must use Control Plane APIs.",
                    "body": "All GUI actions go through Control Plane APIs.",
                    "tags": ["architecture", "gui", "control-plane"],
                    "importance": "high",
                    "session_id": session["id"],
                },
            )
            self.assertEqual(item["type"], "project_rule")
            self.assertEqual(item["status"], "active")
            self.assertEqual(item["source_links"][0]["source_type"], "manual")

            results = store.search_memory_items(project_id=project["id"], query="Control Plane")
            self.assertEqual(results[0]["item"]["id"], item["id"])
            self.assertIn("source_links", results[0])

            selected = service.select_memory_for_session(
                session_id=session["id"],
                memory_item_id=item["id"],
            )
            self.assertEqual(selected["snapshot"]["id"], item["id"])
            context_payload = service.selected_memory_context_payload(session_id=session["id"])
            self.assertEqual(context_payload["source_context"][0]["id"], item["id"])

            service.unselect_memory_for_session(
                session_id=session["id"],
                memory_item_id=item["id"],
            )
            self.assertEqual(service.selected_memory_context_payload(session_id=session["id"])["source_context"], [])

            archived = service.archive_memory_item(memory_item_id=item["id"])
            self.assertEqual(archived["status"], "archived")
            self.assertEqual(
                store.search_memory_items(project_id=project["id"], query="Control Plane"),
                [],
            )
            with self.assertRaises(ValueError):
                service.select_memory_for_session(
                    session_id=session["id"],
                    memory_item_id=item["id"],
                )
            restored = service.restore_memory_item(memory_item_id=item["id"])
            self.assertEqual(restored["status"], "active")

            with self.assertRaises(ValueError):
                service.create_manual_memory_item(
                    project_id=project["id"],
                    payload={
                        "type": "project_rule",
                        "title": "Bad",
                        "summary": "Contains secret marker",
                        "body": "password should not be stored",
                    },
                )

    def test_mvp5_decision_session_and_failure_memory_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis MVP 5 project", encoding="utf-8")
            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP5 contract")

            brainstorming = service.create_brainstorming_session(
                project=project,
                session=session,
                topic="Decide the MVP 5 Memory slice.",
            )
            accepted = service.resolve_decision_brief(
                brainstorming_session_id=brainstorming["brainstorming_session"]["id"],
                decision_brief_id=brainstorming["decision_brief"]["id"],
                status="accepted",
                note="Promote this decision to memory.",
            )
            record = accepted["decision_record"]
            decision_memory = service.promote_decision_record_to_memory(
                decision_record_id=record["id"],
            )
            duplicate = service.promote_decision_record_to_memory(
                decision_record_id=record["id"],
            )
            self.assertEqual(decision_memory["id"], duplicate["id"])
            self.assertEqual(decision_memory["type"], "decision")
            self.assertEqual(
                store.find_memory_by_source(
                    project_id=project["id"],
                    memory_type="decision",
                    source_type="decision_record",
                    source_id=record["id"],
                )["id"],
                decision_memory["id"],
            )

            summary = service.create_session_memory_summary(session_id=session["id"])
            self.assertEqual(summary["status"], "completed")
            self.assertEqual(summary["memory_item"]["type"], "session_summary")

            work_package_result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Create an MVP 5 failure memory Work Package.",
            )
            service.resolve_approval(
                approval_id=work_package_result["approval_id"],
                status="approved",
            )
            implementation_run = store.create_implementation_run(
                project_id=project["id"],
                session_id=session["id"],
                work_package_id=work_package_result["work_package_id"],
            )
            review = store.create_review_result(
                implementation_run_id=implementation_run.id,
                review={
                    "status": "needs_changes",
                    "findings": ["Verification failed."],
                    "residual_risks": ["Regression risk remains."],
                    "recommendation": "Create a follow-up fix before closing.",
                },
            )
            failure = service.promote_review_result_failure_memory(review_result_id=review.id)
            self.assertEqual(failure["type"], "failure")
            self.assertEqual(failure["source_links"][0]["source_type"], "review_result")

            passed = store.create_review_result(
                implementation_run_id=implementation_run.id,
                review={
                    "status": "pass",
                    "findings": ["All checks passed."],
                    "residual_risks": [],
                    "recommendation": "No failure memory required.",
                },
            )
            with self.assertRaises(ValueError):
                service.promote_review_result_failure_memory(review_result_id=passed.id)

            verification = store.create_verification_run(
                implementation_run_id=implementation_run.id,
                command="python -m unittest discover -s tests",
                status="failed",
                exit_code=1,
                stdout="",
                stderr="failure",
            )
            verification_failure = service.promote_verification_failure_memory(
                verification_run_id=verification.id,
            )
            self.assertEqual(verification_failure["type"], "failure")

            failure_results = store.search_memory_items(
                project_id=project["id"],
                query="failure",
                memory_type="failure",
            )
            self.assertGreaterEqual(len(failure_results), 1)

            extraction_events = [
                event
                for event in store.list_session_events(session["id"])
                if event["type"] == "memory.item.created"
            ]
            self.assertGreaterEqual(len(extraction_events), 3)

    def test_mvp6_risk_schema_validation(self) -> None:
        finding = RiskFindingDraft(
            category="verification",
            severity="high",
            title="Repeated verification failures",
            summary="Failure memory points to repeated verification risk.",
            evidence=["verification_run: failed"],
            recommendation="Create a follow-up Work Package.",
            confidence=0.86,
            source_links=[
                {
                    "source_type": "verification_run",
                    "source_id": "verify_001",
                    "relation": "derived_from",
                }
            ],
        )
        signal = QualitySignalDraft(
            kind="verification",
            status="at_risk",
            title="Recorded verification history",
            summary="A failed run is recorded.",
            value={"failed": 1},
            target={"failed": 0},
            evidence=["1 failed run"],
            source_links=[
                {
                    "source_type": "repository_metric",
                    "source_id": "repository_metrics",
                    "relation": "derived_from",
                }
            ],
        )

        self.assertEqual(finding.validate(), [])
        self.assertEqual(signal.validate(), [])

        missing_source = RiskFindingDraft(
            category="verification",
            severity="medium",
            title="No source",
            summary="Invalid source policy.",
            evidence=["missing"],
            recommendation="Do not store.",
            confidence=0.5,
            source_links=[],
        )
        self.assertIn("source_links must be non-empty", missing_source.validate())

    def test_mvp6_risk_scan_selected_memory_and_conversion_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project_root = root / "project"
            project_root.mkdir()
            (project_root / "README.md").write_text("Artemis MVP 6 project", encoding="utf-8")
            (project_root / "docs").mkdir()
            (project_root / "docs" / "artemis_mvp6.md").write_text(
                "Risk Radar and Quality Center\n",
                encoding="utf-8",
            )

            store = SQLiteStore(root / "artemis.db", root / "events.jsonl")
            service = ControlPlaneService(store, agent_backend=InProcessAgentBackendClient())
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP6 contract")

            rule = service.create_manual_memory_item(
                project_id=project["id"],
                payload={
                    "type": "project_rule",
                    "title": "GUI calls Control Plane only",
                    "summary": "The GUI must use Control Plane APIs.",
                    "body": "All GUI actions go through Control Plane APIs.",
                    "tags": ["architecture", "gui"],
                    "importance": "high",
                    "session_id": session["id"],
                },
            )
            service.select_memory_for_session(session_id=session["id"], memory_item_id=rule["id"])

            work_package_result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Create an MVP 6 verification risk Work Package.",
            )
            service.resolve_approval(
                approval_id=work_package_result["approval_id"],
                status="approved",
            )
            implementation_run = store.create_implementation_run(
                project_id=project["id"],
                session_id=session["id"],
                work_package_id=work_package_result["work_package_id"],
            )
            verification = store.create_verification_run(
                implementation_run_id=implementation_run.id,
                command="python -m unittest discover -s tests",
                status="failed",
                exit_code=1,
                stdout="",
                stderr="contract failure",
            )
            service.promote_verification_failure_memory(verification_run_id=verification.id)

            scan_result = service.create_risk_scan(
                project=project,
                session=session,
                scope_type="project",
                include_selected_memory=True,
                selected_memory_ids=[rule["id"]],
                focus=["verification", "architecture", "process"],
            )
            run = scan_result["risk_scan_run"]
            self.assertEqual(run["status"], "completed")
            self.assertEqual(run["selected_memory_count"], 1)
            self.assertEqual(run["source_context"]["selected_memory_snapshots"][0]["id"], rule["id"])
            self.assertGreaterEqual(len(scan_result["findings"]), 1)
            self.assertTrue(all(finding["source_links"] for finding in scan_result["findings"]))
            self.assertGreaterEqual(len(scan_result["quality_signals"]), 3)
            self.assertIsNotNone(scan_result["project_health_snapshot"])
            self.assertIsNotNone(scan_result["architecture_map_snapshot"])
            self.assertIsNotNone(scan_result["trace"])

            radar = service.get_risk_radar(project_id=project["id"])
            self.assertEqual(radar["latest_scan"]["id"], run["id"])
            self.assertGreaterEqual(len(radar["findings"]), 1)
            quality = service.get_quality_snapshot(project_id=project["id"])
            self.assertEqual(quality["latest_scan"]["id"], run["id"])
            self.assertGreaterEqual(len(quality["signals"]), 3)
            self.assertIsNotNone(quality["architecture_map"])

            finding = next(
                item for item in scan_result["findings"] if item["severity"] in {"high", "medium"}
            )
            with self.assertRaises(ValueError):
                service.convert_risk_finding_to_work_package(risk_finding_id=finding["id"])
            accepted = service.update_risk_finding_status(
                risk_finding_id=finding["id"],
                status="accepted",
            )
            self.assertEqual(accepted["status"], "accepted")
            converted = service.convert_risk_finding_to_work_package(
                risk_finding_id=finding["id"],
            )
            self.assertEqual(converted["risk_finding"]["status"], "converted")
            self.assertEqual(converted["work_package"]["status"], "pending_approval")
            self.assertEqual(converted["approval"]["target_type"], "work_package")
            duplicate = service.convert_risk_finding_to_work_package(
                risk_finding_id=finding["id"],
            )
            self.assertEqual(duplicate["work_package"]["id"], converted["work_package"]["id"])

            event_types = {event["type"] for event in store.list_events(run["id"])}
            self.assertIn("selected_memory.attached_to_risk_scan", event_types)
            self.assertIn("risk_finding.created", event_types)
            self.assertIn("quality_signal.created", event_types)
            self.assertIn("project_health_snapshot.created", event_types)
            self.assertIn("risk_finding.converted_to_work_package", event_types)

            trace = store.get_trace_summary(run["id"])
            self.assertEqual(trace["trace"]["root_name"], "artemis_risk_scan")
            self.assertTrue(trace["steps"])

            service.archive_memory_item(memory_item_id=rule["id"])
            with self.assertRaises(ValueError):
                service.create_risk_scan(
                    project=project,
                    session=session,
                    scope_type="project",
                    include_selected_memory=True,
                    selected_memory_ids=[rule["id"]],
                )


if __name__ == "__main__":
    unittest.main()
