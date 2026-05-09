"""Agent Backend service boundary."""

from __future__ import annotations

from pathlib import Path
import difflib
import uuid

from .graph import MVP1GraphRunner
from .schemas import (
    AgentBackendEvent,
    AgentBackendRequest,
    FinalAgentRunResult,
    ImplementationBackendRequest,
    ImplementationPlanDraft,
    ImplementationProposalResult,
    PatchFileDraft,
    PatchSetDraft,
    ReviewBackendRequest,
    ReviewResultDraft,
)


class AgentBackendService:
    def __init__(self, runner: MVP1GraphRunner | None = None) -> None:
        self.runner = runner or MVP1GraphRunner()

    def run_agent(self, request: AgentBackendRequest) -> FinalAgentRunResult:
        return self.runner.run(request)

    def create_implementation_proposal(
        self,
        request: ImplementationBackendRequest,
    ) -> ImplementationProposalResult:
        trace_id = f"trace_{uuid.uuid4().hex[:16]}"
        events = [
            AgentBackendEvent("implementation_run.started", {"trace_id": trace_id}),
            AgentBackendEvent(
                "implementation_run.phase_changed",
                {"phase": "collect_implementation_context"},
            ),
        ]

        try:
            project_root = Path(request.project_root).resolve()
            work_package = request.work_package
            target_file = "docs/artemis_implementation_log.md"
            target_path = project_root / target_file
            old_content = target_path.read_text(encoding="utf-8") if target_path.exists() else ""
            replacement_content = _build_implementation_log(old_content, work_package)
            operation = "update" if target_path.exists() else "create"
            diff = "\n".join(
                difflib.unified_diff(
                    old_content.splitlines(),
                    replacement_content.splitlines(),
                    fromfile=f"a/{target_file}",
                    tofile=f"b/{target_file}",
                    lineterm="",
                )
            )

            events.extend(
                [
                    AgentBackendEvent(
                        "implementation_run.phase_changed",
                        {"phase": "create_implementation_plan"},
                    ),
                    AgentBackendEvent(
                        "implementation_run.phase_changed",
                        {"phase": "generate_patch_proposal"},
                    ),
                ]
            )

            plan = ImplementationPlanDraft(
                goal=f"Implement approved Work Package: {work_package['title']}",
                context_summary=(
                    "MVP 3 deterministic implementation slice records the approved "
                    "Work Package into a project-local implementation log before broader "
                    "LLM-authored code generation is enabled."
                ),
                target_files=[target_file],
                steps=[
                    "Load the approved Work Package and project root.",
                    "Create a structured implementation log entry for the approved scope.",
                    "Prepare a PatchSet with a reviewable unified diff.",
                    "Require patch approval before applying the file change.",
                    "Run allowlisted verification or record why verification was not run.",
                    "Create a review result from the verification outcome.",
                ],
                verification_strategy=[
                    "Apply only after PatchSet approval.",
                    "Run an allowlisted verification command when one is available.",
                    "Store ReviewResult with residual risks even when verification is blocked.",
                ],
                risks=[
                    {
                        "level": "low",
                        "description": "This MVP 3 slice writes a bounded implementation log file only.",
                    }
                ],
            )
            patch_set = PatchSetDraft(
                summary=f"Record implementation plan for {work_package['title']}",
                risk_level="low",
                files=[
                    PatchFileDraft(
                        path=target_file,
                        operation=operation,
                        diff=diff,
                        rationale="Create a durable implementation record for the approved Work Package.",
                        risk_level="low",
                        replacement_content=replacement_content,
                    )
                ],
            )
            events.extend(
                [
                    AgentBackendEvent("implementation_plan.created", {"target_files": [target_file]}),
                    AgentBackendEvent("patch_set.proposed", {"files": [target_file]}),
                    AgentBackendEvent("patch_set.validation_passed", {"policy": "mvp3"}),
                    AgentBackendEvent("patch_set.pending_approval", {}),
                    AgentBackendEvent(
                        "trace.step_recorded",
                        {"trace_id": trace_id, "step": "generate_patch_proposal"},
                    ),
                ]
            )
            return ImplementationProposalResult(
                status="completed",
                implementation_plan=plan,
                patch_set=patch_set,
                trace_id=trace_id,
                events=events,
            )
        except Exception as exc:  # pragma: no cover - defensive service boundary
            events.append(AgentBackendEvent("implementation_run.failed", {"error": str(exc)}))
            return ImplementationProposalResult(
                status="failed",
                implementation_plan=None,
                patch_set=None,
                trace_id=trace_id,
                events=events,
                errors=[str(exc)],
            )

    def create_review_result(self, request: ReviewBackendRequest) -> ReviewResultDraft:
        verification_runs = request.verification_runs
        if not verification_runs:
            return ReviewResultDraft(
                status="blocked",
                findings=["No verification run was recorded."],
                residual_risks=["Patch effects have not been checked by an allowlisted command."],
                recommendation="Run an allowlisted verification command before treating this as complete.",
            )

        blocked = [run for run in verification_runs if run["status"] in {"blocked", "not_run"}]
        failed = [run for run in verification_runs if run["status"] == "failed"]
        if blocked:
            return ReviewResultDraft(
                status="blocked",
                findings=[f"Verification was {run['status']}: {run['command']}" for run in blocked],
                residual_risks=["Verification did not execute to completion."],
                recommendation="Select an allowlisted command or inspect the blocked reason.",
            )
        if failed:
            return ReviewResultDraft(
                status="needs_changes",
                findings=[f"Verification failed: {run['command']}" for run in failed],
                residual_risks=["Applied files may need follow-up fixes."],
                recommendation="Review stderr/stdout and create a follow-up Work Package if needed.",
            )
        return ReviewResultDraft(
            status="pass",
            findings=["All recorded verification runs passed."],
            residual_risks=[],
            recommendation="Patch is ready for human review outside MVP 3 git operations.",
        )


def _build_implementation_log(existing_content: str, work_package: dict[str, object]) -> str:
    package_id = str(work_package.get("id", "unknown"))
    marker = f"<!-- artemis-work-package:{package_id} -->"
    if marker in existing_content:
        return existing_content

    def lines_for(values: object) -> list[str]:
        if isinstance(values, list) and values:
            return [f"- {value}" for value in values]
        return ["- None recorded"]

    title = str(work_package.get("title") or "Untitled Work Package")
    goal = str(work_package.get("goal") or "No goal recorded")
    scope = "\n".join(lines_for(work_package.get("scope")))
    verification = "\n".join(lines_for(work_package.get("verification")))
    entry = (
        f"{marker}\n"
        f"## {title}\n\n"
        f"- Work package: `{package_id}`\n"
        f"- Goal: {goal}\n\n"
        "### Scope\n\n"
        f"{scope}\n\n"
        "### Verification\n\n"
        f"{verification}\n"
    )
    if not existing_content.strip():
        return f"# Artemis Implementation Log\n\n{entry}"
    separator = "\n\n" if not existing_content.endswith("\n\n") else ""
    if existing_content.endswith("\n") and separator:
        separator = "\n"
    return f"{existing_content}{separator}{entry}"
