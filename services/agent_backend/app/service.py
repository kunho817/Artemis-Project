"""Agent Backend service boundary."""

from __future__ import annotations

from pathlib import Path
import difflib
import uuid

from .graph import MVP1GraphRunner
from .schemas import (
    AgentBackendEvent,
    AgentBackendRequest,
    BrainstormingBackendRequest,
    BrainstormingContributionDraft,
    BrainstormingCritiqueDraft,
    BrainstormingOptionDraft,
    BrainstormingRunResult,
    DecisionBriefDraft,
    FinalAgentRunResult,
    ImplementationBackendRequest,
    ImplementationPlanDraft,
    ImplementationProposalResult,
    PatchFileDraft,
    PatchSetDraft,
    ReviewBackendRequest,
    ReviewResultDraft,
    WorkPackageCandidateRequest,
)
from .tools import ReadOnlyToolRouter


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

    def run_brainstorming(self, request: BrainstormingBackendRequest) -> BrainstormingRunResult:
        trace_id = f"trace_{uuid.uuid4().hex[:16]}"
        events = [
            AgentBackendEvent(
                "trace.linked",
                {
                    "trace_id": trace_id,
                    "provider": "local",
                    "brainstorming_session_id": request.brainstorming_session_id,
                },
            ),
            AgentBackendEvent("brainstorming_session.started", {"trace_id": trace_id}),
        ]

        try:
            roles = _normalize_brainstorming_roles(request.roles, request.mode)
            events.append(
                AgentBackendEvent(
                    "brainstorming_session.phase_changed",
                    {"phase": "collect_brainstorming_context"},
                )
            )
            context = _collect_brainstorming_context(request)
            events.append(
                AgentBackendEvent(
                    "brainstorming.context_collected",
                    {
                        "files_considered": len(context["files_considered"]),
                        "source_type": request.source_type,
                    },
                )
            )
            events.extend(
                [
                    AgentBackendEvent(
                        "brainstorming_session.phase_changed",
                        {"phase": "select_roles"},
                    ),
                    AgentBackendEvent("brainstorming.roles_selected", {"roles": roles}),
                    AgentBackendEvent(
                        "brainstorming_session.phase_changed",
                        {"phase": "generate_contributions"},
                    ),
                ]
            )
            contributions = [
                _build_contribution(role, request, context, index)
                for index, role in enumerate(roles)
            ]
            for contribution in contributions:
                events.extend(
                    [
                        AgentBackendEvent(
                            "brainstorming.role_started",
                            {"role": contribution.role},
                        ),
                        AgentBackendEvent(
                            "brainstorming.role_completed",
                            {"role": contribution.role, "stance": contribution.stance},
                        ),
                    ]
                )

            events.append(
                AgentBackendEvent(
                    "brainstorming_session.phase_changed",
                    {"phase": "generate_critiques"},
                )
            )
            critiques = _build_critiques(contributions, request.mode)
            for critique in critiques:
                events.append(
                    AgentBackendEvent(
                        "brainstorming.critique_created",
                        {
                            "critic_role": critique.critic_role,
                            "target_role": critique.target_role,
                        },
                    )
                )

            events.append(
                AgentBackendEvent(
                    "brainstorming_session.phase_changed",
                    {"phase": "synthesize_options"},
                )
            )
            options = _build_options(request, context)
            for option in options:
                events.append(
                    AgentBackendEvent(
                        "brainstorming.option_created",
                        {"title": option.title, "score": option.score},
                    )
                )

            events.append(
                AgentBackendEvent(
                    "brainstorming_session.phase_changed",
                    {"phase": "create_decision_brief"},
                )
            )
            decision_brief = _build_decision_brief(request, context, roles)
            errors = _validate_brainstorming_result(contributions, options, decision_brief)
            if errors:
                events.append(AgentBackendEvent("brainstorming.validation_failed", {"errors": errors}))
                return BrainstormingRunResult(
                    status="failed",
                    trace_id=trace_id,
                    contributions=contributions,
                    critiques=critiques,
                    options=options,
                    decision_brief=decision_brief,
                    events=events,
                    errors=errors,
                )

            events.extend(
                [
                    AgentBackendEvent(
                        "brainstorming.decision_brief_created",
                        {"recommendation": decision_brief.recommendation},
                    ),
                    AgentBackendEvent("brainstorming.validation_passed", {}),
                    AgentBackendEvent(
                        "trace.step_recorded",
                        {"trace_id": trace_id, "step": "create_decision_brief"},
                    ),
                    AgentBackendEvent("brainstorming_session.completed", {}),
                ]
            )
            return BrainstormingRunResult(
                status="completed",
                trace_id=trace_id,
                contributions=contributions,
                critiques=critiques,
                options=options,
                decision_brief=decision_brief,
                events=events,
            )
        except Exception as exc:  # pragma: no cover - defensive service boundary
            events.append(AgentBackendEvent("brainstorming_session.failed", {"error": str(exc)}))
            return BrainstormingRunResult(
                status="failed",
                trace_id=trace_id,
                contributions=[],
                critiques=[],
                options=[],
                decision_brief=None,
                events=events,
                errors=[str(exc)],
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


def _normalize_brainstorming_roles(roles: list[str], mode: str) -> list[str]:
    cleaned: list[str] = []
    for role in roles:
        normalized = role.strip().lower().replace("-", "_")
        if normalized and normalized not in cleaned:
            cleaned.append(normalized)
    if cleaned:
        return cleaned[:6]
    defaults = {
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
    return defaults.get(mode, defaults["architecture_debate"])


def _collect_brainstorming_context(request: BrainstormingBackendRequest) -> dict[str, object]:
    tools = ReadOnlyToolRouter(request.project_root)
    files_result = tools.list_files(max_files=160)
    files = [line for line in files_result.output.splitlines() if line]
    keywords = [word for word in request.topic.replace("/", " ").replace("\\", " ").split() if len(word) >= 4]
    related: list[str] = []
    for keyword in keywords[:4]:
        for line in tools.grep(keyword, max_matches=8).output.splitlines():
            file_name = line.split(":", 1)[0]
            if file_name and file_name not in related:
                related.append(file_name)
    preferred = [
        path
        for path in files
        if path in {"README.md", "AGENTS.md"}
        or path.startswith("docs/")
        or path in related
    ]
    return {
        "git_status": tools.git_status().output,
        "files_considered": (preferred or files)[:25],
        "related_files": (related or preferred or files)[:8],
        "source_context": request.source_context,
    }


def _build_contribution(
    role: str,
    request: BrainstormingBackendRequest,
    context: dict[str, object],
    index: int,
) -> BrainstormingContributionDraft:
    stance_by_role = {
        "product_planner": "supportive",
        "system_architect": "cautious",
        "implementation_planner": "exploratory",
        "risk_reviewer": "cautious",
        "devil_advocate": "opposed",
        "moderator": "exploratory",
    }
    focus_by_role = {
        "product_planner": "user value, priority, and scope boundaries",
        "system_architect": "service boundaries, API contracts, and long-term maintainability",
        "implementation_planner": "sequence, testability, and a small vertical slice",
        "risk_reviewer": "policy gates, data loss, and operational failure modes",
        "devil_advocate": "weak assumptions, overreach, and deferral arguments",
        "moderator": "decision framing and synthesis",
    }
    focus = focus_by_role.get(role, "planning quality and implementation risk")
    source_label = request.source_type if request.source_type != "topic" else "topic"
    related_files = [str(item) for item in context.get("related_files", [])]
    return BrainstormingContributionDraft(
        role=role,
        stance=stance_by_role.get(role, "exploratory"),  # type: ignore[arg-type]
        summary=f"{role} frames '{request.topic}' through {focus}.",
        arguments=[
            f"Keep the discussion tied to the {source_label} and the existing Artemis state model.",
            "Preserve Control Plane ownership of canonical product state.",
            "Return structured outputs that can be validated before conversion.",
        ],
        concerns=[
            "The slice can become too broad if collaboration or memory features enter scope.",
            "Generated recommendations need explicit approval before becoming Work Packages.",
            "Project files and commands must remain outside the brainstorming path.",
        ],
        suggested_actions=[
            "Store role contributions, critiques, options, and a DecisionBrief separately.",
            "Gate DecisionRecord creation behind explicit accept/reject actions.",
            "Convert only accepted DecisionRecords into pending-approval Work Packages.",
        ],
        referenced_artifacts=related_files[:3] or [f"{request.source_type}:{request.source_id or request.topic}"],
    )


def _build_critiques(
    contributions: list[BrainstormingContributionDraft],
    mode: str,
) -> list[BrainstormingCritiqueDraft]:
    critiques: list[BrainstormingCritiqueDraft] = []
    if len(contributions) < 2:
        return critiques
    for index, contribution in enumerate(contributions):
        target = contributions[(index + 1) % len(contributions)]
        critiques.append(
            BrainstormingCritiqueDraft(
                critic_role=contribution.role,
                target_role=target.role,
                weak_assumptions=[
                    f"{target.role} may assume enough context exists for {mode}.",
                    "The recommended scope may hide follow-up UX or policy work.",
                ],
                missing_context=[
                    "Concrete user acceptance criteria for the chosen option.",
                    "Verification data from a GUI smoke or backend contract run.",
                ],
                risks=[
                    "Decision output could be accepted without preserving tradeoffs.",
                    "Work Package conversion could bypass approval if not explicitly gated.",
                ],
                suggested_revisions=[
                    "Make the accepted DecisionRecord the only conversion source.",
                    "Include rejected and deferred items in the DecisionBrief risks or tradeoffs.",
                ],
            )
        )
    return critiques


def _build_options(
    request: BrainstormingBackendRequest,
    context: dict[str, object],
) -> list[BrainstormingOptionDraft]:
    related_files = [str(item) for item in context.get("related_files", [])]
    anchor = related_files[0] if related_files else "docs/artemis_mvp4.md"
    return [
        BrainstormingOptionDraft(
            title="Staged Brainstorming Room vertical slice",
            summary=(
                "Add BrainstormingSession, structured role output, DecisionBrief approval, "
                "DecisionRecord storage, and Work Package conversion in one bounded path."
            ),
            benefits=[
                "Covers the MVP 4 completion path end to end.",
                "Keeps GUI, Control Plane, and Agent Backend boundaries aligned.",
                "Provides a reusable DecisionRecord input for MVP 5.",
            ],
            costs=[
                "Uses deterministic structured generation until live LLM policy gates are ready.",
                "Adds several persistence tables and API endpoints.",
            ],
            risks=[
                "The GUI can become dense if brainstorming details are mixed with implementation details.",
                "Source artifact validation must stay strict to avoid orphan decisions.",
            ],
            required_work=[
                "Add Brainstorming and Decision models.",
                "Add internal Agent Backend brainstorming execution.",
                "Render contributions, critiques, options, DecisionBrief, and trace in the GUI.",
            ],
            verification_hint="Run backend contracts plus MVP 4 GUI e2e smoke.",
            score=0.88,
        ),
        BrainstormingOptionDraft(
            title="Backend-first decision ledger",
            summary="Ship only backend APIs and storage, then add the GUI in a later session.",
            benefits=[
                "Reduces immediate frontend risk.",
                f"Can validate source artifact handling around {anchor}.",
            ],
            costs=[
                "Does not satisfy the MVP 4 GUI completion criteria.",
                "Leaves accept/reject and conversion harder to inspect.",
            ],
            risks=["Users cannot review role tradeoffs in the intended workflow."],
            required_work=["Add contracts and API smoke only."],
            verification_hint="Run full unittest and FastAPI smoke.",
            score=0.66,
        ),
        BrainstormingOptionDraft(
            title="Defer conversion until Memory MVP",
            summary="Store brainstorming output but postpone DecisionRecord to Work Package conversion.",
            benefits=["Keeps the current approval model simpler."],
            costs=["Misses the explicit MVP 4 conversion requirement."],
            risks=["Decision output may not connect to implementation planning."],
            required_work=["Add DecisionBrief accept/reject but no conversion endpoint."],
            verification_hint="Check rejected briefs cannot convert.",
            score=0.48,
        ),
    ]


def _build_decision_brief(
    request: BrainstormingBackendRequest,
    context: dict[str, object],
    roles: list[str],
) -> DecisionBriefDraft:
    related_files = [str(item) for item in context.get("related_files", [])]
    title = f"Implement brainstorming decision flow for {request.topic[:48].strip()}"
    return DecisionBriefDraft(
        recommendation="Choose the staged Brainstorming Room vertical slice.",
        selected_option_index=0,
        rationale=(
            "It is the only option that satisfies structured role discussion, DecisionBrief "
            "approval, DecisionRecord persistence, Work Package conversion, event visibility, "
            "and trace visibility without expanding into Memory or autonomous implementation."
        ),
        tradeoffs=[
            "Deterministic generation keeps contracts stable but should later be replaced by GLM structured output.",
            "The GUI gets denser, so Brainstorming details should remain grouped in a dedicated panel.",
            "DecisionRecord storage starts now, while long-term Memory search remains out of scope.",
        ],
        risks=[
            "Role count must remain capped to prevent runaway discussion.",
            "Rejected DecisionBrief conversion must be blocked.",
            "No project file writes or command execution may occur inside brainstorming.",
        ],
        open_questions=[
            "Which source artifact should become the default when both WorkPackage and ReviewResult are available?",
            "How much of the deterministic fallback should be kept after GLM structured generation is enabled?",
        ],
        follow_up_actions=[
            "Add backend contract coverage for topic and WorkPackage source sessions.",
            "Add GUI smoke coverage for accept, DecisionRecord display, and Work Package conversion.",
            "Record local trace steps for each brainstorming phase.",
        ],
        work_package_candidate=WorkPackageCandidateRequest(
            title=title,
            goal=f"Convert the accepted brainstorming decision into an approved planning candidate for: {request.topic}",
            background=(
                "The candidate is generated from a structured Brainstorming DecisionBrief. "
                "It remains pending approval and does not start implementation automatically."
            ),
            scope=[
                "Preserve the accepted DecisionRecord as source context.",
                "Create a pending-approval Work Package candidate from the DecisionBrief.",
                "Show conversion events and trace correlation in the GUI.",
            ],
            out_of_scope=[
                "Writing user project files.",
                "Running shell commands.",
                "Starting ImplementationRun automatically.",
                "Long-term Memory or vector search.",
            ],
            related_files=related_files[:5] or ["docs/artemis_mvp4.md"],
            required_agents=[role.replace("_", " ").title().replace(" ", "") for role in roles[:4]],
            implementation_steps=[
                "Review the DecisionBrief recommendation and tradeoffs.",
                "Accept the brief to create a DecisionRecord.",
                "Convert the accepted DecisionRecord into a pending-approval Work Package.",
                "Approve or reject the converted Work Package through the existing approval flow.",
            ],
            verification=[
                "backend contract test",
                "FastAPI smoke",
                "GUI e2e smoke",
            ],
            risks=[
                {
                    "level": "medium",
                    "description": "The generated candidate may need human narrowing before implementation.",
                }
            ],
            completion_criteria=[
                "DecisionRecord is accepted and stored.",
                "Converted Work Package is pending approval.",
                "Events and trace summary are visible for the brainstorming session.",
            ],
        ),
    )


def _validate_brainstorming_result(
    contributions: list[BrainstormingContributionDraft],
    options: list[BrainstormingOptionDraft],
    decision_brief: DecisionBriefDraft,
) -> list[str]:
    errors: list[str] = []
    if not contributions:
        errors.append("contributions must be non-empty")
    for contribution in contributions:
        errors.extend(contribution.validate())
    if not options:
        errors.append("options must be non-empty")
    for option in options:
        errors.extend(option.validate())
    errors.extend(decision_brief.validate(len(options)))
    return errors
