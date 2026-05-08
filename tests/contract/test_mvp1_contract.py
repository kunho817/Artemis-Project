from __future__ import annotations

from pathlib import Path
import tempfile
import unittest

from services.agent_backend.app.config import model_for_role
from services.agent_backend.app.schemas import RiskHint, WorkPackageDraft
from services.agent_backend.app.tools import ReadOnlyToolRouter, ToolPermissionError
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
        }

        self.assertEqual(model_for_role("architect", env).model, "glm-5.1")
        self.assertEqual(model_for_role("validator", env).model, "glm-4.6")

    def test_read_only_tool_permission(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "README.md").write_text("Artemis MVP 1", encoding="utf-8")
            tools = ReadOnlyToolRouter(root)

            self.assertTrue(tools.read_file("README.md").ok)
            with self.assertRaises(ToolPermissionError):
                tools.assert_allowed("write_file")
            with self.assertRaises(ToolPermissionError):
                tools.read_file("../outside.txt")

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
            service = ControlPlaneService(store)
            project = service.open_project(name="test", root_path=str(project_root))
            session = service.create_session(project_id=project["id"], title="MVP1 test")

            result = service.create_work_package_from_request(
                project=project,
                session=session,
                user_request="Plan a feature request for the MVP 1 control plane",
            )

            self.assertEqual(result["status"], "pending_approval")
            self.assertTrue(result["langsmith_trace_id"].startswith("local_"))

            run = store.get_agent_run(result["agent_run_id"])
            self.assertEqual(run["status"], "completed")
            self.assertEqual(run["intent"], "planning_request")
            self.assertEqual(run["langsmith_trace_id"], result["langsmith_trace_id"])

            package = store.get_work_package(result["work_package_id"])
            self.assertEqual(package["status"], "pending_approval")
            self.assertEqual(package["approval_status"], "pending")
            self.assertTrue(package["approval_required"])

            events = store.list_events(result["agent_run_id"])
            event_types = {event["type"] for event in events}
            self.assertIn("agent_run.created", event_types)
            self.assertIn("trace.langsmith_linked", event_types)
            self.assertIn("work_package.pending_approval", event_types)
            self.assertIn("approval.requested", event_types)

            approval = service.resolve_approval(
                approval_id=result["approval_id"],
                status="approved",
            )
            self.assertEqual(approval["status"], "approved")
            approved_package = store.get_work_package(result["work_package_id"])
            self.assertEqual(approved_package["approval_status"], "approved")
            self.assertEqual(approved_package["status"], "approved")


if __name__ == "__main__":
    unittest.main()
