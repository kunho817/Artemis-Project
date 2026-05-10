"""Run the Alpha 0.1 dogfooding flow against in-process services."""

from __future__ import annotations

import json
from pathlib import Path
import sys
import tempfile
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def create_project(root: Path) -> Path:
    project_root = root / "project"
    project_root.mkdir()
    (project_root / "README.md").write_text(
        "Artemis Alpha dogfooding project\n\nTODO: verify Alpha residual risks.\n",
        encoding="utf-8",
    )
    (project_root / "docs").mkdir()
    (project_root / "docs" / "alpha.md").write_text(
        "Alpha dogfooding target.\n",
        encoding="utf-8",
    )
    (project_root / "tests").mkdir()
    (project_root / "tests" / "test_smoke.py").write_text(
        "import unittest\n\n"
        "class AlphaSmokeTest(unittest.TestCase):\n"
        "    def test_smoke(self):\n"
        "        self.assertTrue(True)\n",
        encoding="utf-8",
    )
    return project_root


def run() -> dict[str, Any]:
    from services.agent_backend.app.service import AgentBackendService
    from services.control_plane.app.agent_client import InProcessAgentBackendClient
    from services.control_plane.app.service import ControlPlaneService
    from services.control_plane.app.storage import SQLiteStore

    with tempfile.TemporaryDirectory(prefix="artemis-alpha-dogfood-") as tmp:
        tmp_root = Path(tmp)
        project_root = create_project(tmp_root)
        store = SQLiteStore(tmp_root / "artemis.db", tmp_root / "events.jsonl")
        service = ControlPlaneService(
            store,
            agent_backend=InProcessAgentBackendClient(AgentBackendService()),
        )
        project = service.open_project(name="Alpha Dogfood", root_path=str(project_root))
        session = service.create_session(project_id=project["id"], title="Alpha Dogfooding")

        rule = service.create_manual_memory_item(
            project_id=project["id"],
            payload={
                "type": "project_rule",
                "title": "Keep Alpha local-first",
                "summary": "Do not add hidden automation or external scanners.",
                "body": "Alpha dogfooding must keep approvals explicit and local state inspectable.",
                "tags": ["alpha", "safety"],
                "importance": "high",
                "session_id": session["id"],
            },
        )
        service.select_memory_for_session(session_id=session["id"], memory_item_id=rule["id"])

        scan = service.create_risk_scan(
            project=project,
            session=session,
            scope_type="project",
            include_selected_memory=True,
            selected_memory_ids=[rule["id"]],
            focus=["alpha", "verification", "process"],
        )
        finding = scan["findings"][0]
        service.update_risk_finding_status(risk_finding_id=finding["id"], status="accepted")
        converted = service.convert_risk_finding_to_work_package(risk_finding_id=finding["id"])
        approval = converted["approval"]
        if approval is None:
            raise RuntimeError("Converted WorkPackage did not create an approval")
        service.resolve_approval(approval_id=approval["id"], status="approved")

        implementation = service.create_implementation_run(
            work_package_id=converted["work_package"]["id"],
        )
        patch_set = implementation["patch_set"]
        if patch_set is None:
            raise RuntimeError("ImplementationRun did not produce a PatchSet")
        service.resolve_patch_set(patch_set_id=patch_set["id"], status="approved")
        service.apply_patch_set(patch_set_id=patch_set["id"])
        final_implementation = service.get_implementation_run_result(
            implementation["implementation_run"]["id"],
        )
        summary = service.create_session_memory_summary(session_id=session["id"])
        rescan = service.create_risk_scan(
            project=project,
            session=session,
            scope_type="project",
            include_selected_memory=True,
            selected_memory_ids=[rule["id"]],
            focus=["alpha", "post-implementation"],
        )
        command_center = service.get_command_center(
            project_id=project["id"],
            session_id=session["id"],
        )

        return {
            "status": "ok",
            "project_root": str(project_root),
            "risk_scan_id": scan["risk_scan_run"]["id"],
            "converted_work_package_id": converted["work_package"]["id"],
            "implementation_run_id": final_implementation["implementation_run"]["id"],
            "implementation_status": final_implementation["implementation_run"]["status"],
            "review_status": final_implementation["review_result"]["status"]
            if final_implementation["review_result"]
            else None,
            "memory_summary_status": summary["status"],
            "rescan_id": rescan["risk_scan_run"]["id"],
            "command_center_next_action": command_center["next_action"],
        }


def main() -> int:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    try:
        result = run()
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
