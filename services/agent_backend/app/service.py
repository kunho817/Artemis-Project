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
    ArchitectureMapSnapshotDraft,
    MemoryCandidateBackendRequest,
    MemoryCandidateDraft,
    MemoryCandidateRunResult,
    PatchFileDraft,
    PatchSetDraft,
    ProjectHealthSnapshotDraft,
    QualitySignalDraft,
    RiskAnalysisCandidateResult,
    RiskFindingDraft,
    RiskScanBackendRequest,
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

    def create_memory_candidate(
        self,
        request: MemoryCandidateBackendRequest,
    ) -> MemoryCandidateRunResult:
        trace_id = f"trace_{uuid.uuid4().hex[:16]}"
        events = [
            AgentBackendEvent(
                "trace.linked",
                {
                    "trace_id": trace_id,
                    "provider": "local",
                    "extraction_run_id": request.extraction_run_id,
                },
            ),
            AgentBackendEvent("memory.extraction_run.started", {"trace_id": trace_id}),
            AgentBackendEvent(
                "memory.extraction_run.phase_changed",
                {"phase": "load_memory_source"},
            ),
        ]
        try:
            events.append(
                AgentBackendEvent(
                    "memory.extraction_run.phase_changed",
                    {"phase": "draft_memory_candidate"},
                )
            )
            candidate = _build_memory_candidate(request)
            errors = candidate.validate()
            if errors:
                events.append(AgentBackendEvent("memory.extraction_run.failed", {"errors": errors}))
                return MemoryCandidateRunResult(
                    status="failed",
                    trace_id=trace_id,
                    candidate=candidate,
                    events=events,
                    errors=errors,
                )
            events.extend(
                [
                    AgentBackendEvent(
                        "memory.candidate.created",
                        {"type": candidate.type, "title": candidate.title},
                    ),
                    AgentBackendEvent(
                        "memory.extraction_run.phase_changed",
                        {"phase": "validate_memory_candidate"},
                    ),
                    AgentBackendEvent("memory.extraction_run.completed", {}),
                    AgentBackendEvent(
                        "trace.step_recorded",
                        {"trace_id": trace_id, "step": "draft_memory_candidate"},
                    ),
                ]
            )
            return MemoryCandidateRunResult(
                status="completed",
                trace_id=trace_id,
                candidate=candidate,
                events=events,
            )
        except Exception as exc:  # pragma: no cover - defensive service boundary
            events.append(AgentBackendEvent("memory.extraction_run.failed", {"error": str(exc)}))
            return MemoryCandidateRunResult(
                status="failed",
                trace_id=trace_id,
                candidate=None,
                events=events,
                errors=[str(exc)],
            )

    def create_risk_analysis(
        self,
        request: RiskScanBackendRequest,
    ) -> RiskAnalysisCandidateResult:
        trace_id = f"trace_{uuid.uuid4().hex[:16]}"
        events = [
            AgentBackendEvent(
                "trace.linked",
                {
                    "trace_id": trace_id,
                    "provider": "local",
                    "risk_scan_run_id": request.risk_scan_run_id,
                },
            ),
            AgentBackendEvent("risk_scan.started", {"trace_id": trace_id}),
        ]
        try:
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "load_project_context_bundle"},
                )
            )
            context = dict(request.source_context or {})
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "collect_repository_signals"},
                )
            )
            repository_signals = _collect_repository_signals(request.project_root, context)
            events.append(
                AgentBackendEvent(
                    "risk_scan.repository_signals_collected",
                    {
                        "file_count": repository_signals["metrics"]["file_count"],
                        "todo_count": len(repository_signals["todo_matches"]),
                    },
                )
            )

            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "collect_memory_signals"},
                )
            )
            memory_signals = _collect_memory_signals(context)
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "collect_execution_signals"},
                )
            )
            execution_signals = _collect_execution_signals(context)

            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "draft_risk_findings"},
                )
            )
            findings = _build_risk_findings(
                request=request,
                repository_signals=repository_signals,
                memory_signals=memory_signals,
                execution_signals=execution_signals,
            )
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "draft_quality_signals"},
                )
            )
            quality_signals = _build_quality_signals(
                repository_signals=repository_signals,
                memory_signals=memory_signals,
                execution_signals=execution_signals,
            )
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "draft_architecture_map_lite"},
                )
            )
            architecture_map = _build_architecture_map(repository_signals)
            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "rank_and_dedupe_findings"},
                )
            )
            findings = _rank_and_dedupe_findings(findings)
            health_snapshot = _build_project_health_snapshot(findings, quality_signals)

            events.append(
                AgentBackendEvent(
                    "risk_scan.phase_changed",
                    {"phase": "validate_analysis"},
                )
            )
            errors = _validate_risk_analysis(findings, quality_signals, health_snapshot, architecture_map)
            if errors:
                events.append(AgentBackendEvent("risk_scan.failed", {"errors": errors}))
                return RiskAnalysisCandidateResult(
                    status="failed",
                    trace_id=trace_id,
                    findings=findings,
                    quality_signals=quality_signals,
                    project_health_snapshot=health_snapshot,
                    architecture_map_snapshot=architecture_map,
                    source_context=context,
                    events=events,
                    errors=errors,
                )

            events.extend(
                [
                    AgentBackendEvent(
                        "risk_finding.created",
                        {"count": len(findings)},
                    ),
                    AgentBackendEvent(
                        "quality_signal.created",
                        {"count": len(quality_signals)},
                    ),
                    AgentBackendEvent("quality_snapshot.created", {}),
                    AgentBackendEvent("architecture_map.created", {}),
                    AgentBackendEvent("project_health_snapshot.created", {}),
                    AgentBackendEvent(
                        "trace.step_recorded",
                        {"trace_id": trace_id, "step": "validate_analysis"},
                    ),
                    AgentBackendEvent("risk_scan.completed", {}),
                ]
            )
            return RiskAnalysisCandidateResult(
                status="completed",
                trace_id=trace_id,
                findings=findings,
                quality_signals=quality_signals,
                project_health_snapshot=health_snapshot,
                architecture_map_snapshot=architecture_map,
                source_context=context,
                events=events,
            )
        except Exception as exc:  # pragma: no cover - defensive service boundary
            events.append(AgentBackendEvent("risk_scan.failed", {"error": str(exc)}))
            return RiskAnalysisCandidateResult(
                status="failed",
                trace_id=trace_id,
                findings=[],
                quality_signals=[],
                project_health_snapshot=None,
                architecture_map_snapshot=None,
                source_context=request.source_context,
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


def _build_memory_candidate(request: MemoryCandidateBackendRequest) -> MemoryCandidateDraft:
    snapshot = request.source_snapshot
    source_link = {
        "source_type": request.source_type,
        "source_id": request.source_id,
        "relation": "derived_from",
    }
    if request.source_type == "decision_record":
        record = snapshot.get("decision_record", snapshot)
        title = str(record.get("title") or record.get("decision") or "Decision memory")
        decision = str(record.get("decision") or title)
        rationale = str(record.get("rationale") or "No rationale recorded.")
        consequences = _string_list(record.get("consequences"))
        follow_up = _string_list(record.get("follow_up_actions"))
        body = (
            f"Decision: {decision}\n\n"
            f"Rationale: {rationale}\n\n"
            f"Consequences:\n{_bullet_list(consequences)}\n\n"
            f"Follow-up actions:\n{_bullet_list(follow_up)}"
        )
        links = [source_link]
        brainstorming_session_id = record.get("brainstorming_session_id")
        if brainstorming_session_id:
            links.append(
                {
                    "source_type": "brainstorming_session",
                    "source_id": str(brainstorming_session_id),
                    "relation": "supports",
                }
            )
        return MemoryCandidateDraft(
            type="decision",
            title=f"Decision: {title}",
            summary=decision[:260],
            body=body,
            tags=["decision", "adr"],
            importance="high",
            confidence=0.92,
            source_links=links,
        )
    if request.source_type == "session":
        session = snapshot.get("session", {})
        event_count = len(snapshot.get("events", []))
        decision_count = len(snapshot.get("decision_records", []))
        work_package_count = len(snapshot.get("work_packages", []))
        title = f"Session summary: {session.get('title', request.source_id)}"
        body = (
            f"Session: {session.get('title', request.source_id)}\n"
            f"Events recorded: {event_count}\n"
            f"Decision records: {decision_count}\n"
            f"Work packages: {work_package_count}\n\n"
            "Next action: review selected decisions, pending approvals, and implementation results before starting the next planning request."
        )
        return MemoryCandidateDraft(
            type="session_summary",
            title=title,
            summary=f"{event_count} events, {decision_count} decisions, and {work_package_count} work packages were recorded.",
            body=body,
            tags=["session", "summary"],
            importance="medium",
            confidence=0.78,
            source_links=[source_link],
        )
    if request.source_type == "review_result":
        review = snapshot.get("review_result", snapshot)
        status = str(review.get("status", "needs_changes"))
        findings = _string_list(review.get("findings"))
        residual = _string_list(review.get("residual_risks"))
        recommendation = str(review.get("recommendation") or "Review failed or was blocked.")
        return MemoryCandidateDraft(
            type="failure",
            title=f"Failure memory: review {status}",
            summary=recommendation[:260],
            body=(
                f"Symptom: Review result ended as {status}.\n\n"
                f"Findings:\n{_bullet_list(findings)}\n\n"
                f"Residual risks:\n{_bullet_list(residual)}\n\n"
                f"Recovery action: {recommendation}"
            ),
            tags=["failure", "review", status],
            importance="high",
            confidence=0.86,
            source_links=[source_link],
        )
    if request.source_type == "verification_run":
        verification = snapshot.get("verification_run", snapshot)
        command = str(verification.get("command") or "verification")
        status = str(verification.get("status") or "failed")
        stderr = str(verification.get("stderr") or "")
        stdout = str(verification.get("stdout") or "")
        output = stderr or stdout or "No output captured."
        return MemoryCandidateDraft(
            type="failure",
            title=f"Failure memory: {command or status}",
            summary=f"Verification ended as {status}.",
            body=(
                f"Symptom: Verification run ended as {status}.\n\n"
                f"Affected surface: {command or 'unknown command'}\n\n"
                f"Observed output:\n{output[:1200]}\n\n"
                "Prevention hint: run an allowlisted verification command and inspect failures before closing the implementation."
            ),
            tags=["failure", "verification", status],
            importance="high",
            confidence=0.84,
            source_links=[source_link],
        )
    title = str(snapshot.get("title") or request.source_id)
    return MemoryCandidateDraft(
        type="work_note",
        title=f"Work note: {title}",
        summary=f"Memory note extracted from {request.source_type}.",
        body=f"Source {request.source_type}:{request.source_id} should be reviewed before related planning work.",
        tags=["work_note", request.source_type],
        importance="medium",
        confidence=0.7,
        source_links=[source_link],
    )


def _string_list(value: object) -> list[str]:
    if isinstance(value, list):
        return [str(item) for item in value if str(item).strip()]
    if value:
        return [str(value)]
    return []


def _bullet_list(values: list[str]) -> str:
    if not values:
        return "- None recorded"
    return "\n".join(f"- {value}" for value in values)


def _collect_repository_signals(project_root: str, context: dict[str, object]) -> dict[str, object]:
    tools = ReadOnlyToolRouter(project_root)
    file_lines = [line for line in tools.list_files(max_files=800).output.splitlines() if line]
    metrics = _repository_metrics_from_context(file_lines, context)
    todo_matches = _grep_many(tools, ["TODO", "FIXME", "HACK"], max_matches=24)
    secret_hints = _grep_many(tools, ["password", "secret", "token"], max_matches=18)
    boundary_hints = _grep_many(
        tools,
        ["ARTEMIS_AGENT_BACKEND_URL", "/internal/", "127.0.0.1:8765"],
        max_matches=18,
    )
    return {
        "files": file_lines,
        "metrics": metrics,
        "todo_matches": todo_matches,
        "secret_hints": secret_hints,
        "boundary_hints": boundary_hints,
        "git_status": tools.git_status().output,
    }


def _repository_metrics_from_context(
    files: list[str],
    context: dict[str, object],
) -> dict[str, object]:
    provided = context.get("repository_metrics")
    if isinstance(provided, dict):
        return provided
    top_level_counts: dict[str, int] = {}
    extension_counts: dict[str, int] = {}
    kind_counts = {"source": 0, "test": 0, "doc": 0, "script": 0, "config": 0}
    for file_path in files:
        top_level = file_path.split("/", 1)[0]
        top_level_counts[top_level] = top_level_counts.get(top_level, 0) + 1
        suffix = Path(file_path).suffix.lower() or "(none)"
        extension_counts[suffix] = extension_counts.get(suffix, 0) + 1
        lowered = file_path.lower()
        if lowered.startswith("tests/") or "/test" in lowered or lowered.endswith(".spec.ts"):
            kind_counts["test"] += 1
        elif lowered.startswith("docs/") or lowered.endswith(".md"):
            kind_counts["doc"] += 1
        elif lowered.startswith("scripts/"):
            kind_counts["script"] += 1
        elif lowered.endswith((".toml", ".json", ".yaml", ".yml", ".ini", ".cfg")):
            kind_counts["config"] += 1
        else:
            kind_counts["source"] += 1
    return {
        "file_count": len(files),
        "top_level_counts": top_level_counts,
        "extension_counts": extension_counts,
        "kind_counts": kind_counts,
        "has_tests": any(path.startswith("tests/") or "/tests/" in path for path in files),
        "has_gui": any(path.startswith("apps/gui/") for path in files),
        "has_control_plane": any(path.startswith("services/control_plane/") for path in files),
        "has_agent_backend": any(path.startswith("services/agent_backend/") for path in files),
        "has_docs": any(path.startswith("docs/") for path in files),
    }


def _grep_many(
    tools: ReadOnlyToolRouter,
    patterns: list[str],
    *,
    max_matches: int,
) -> list[str]:
    matches: list[str] = []
    for pattern in patterns:
        for line in tools.grep(pattern, max_matches=max_matches).output.splitlines():
            if line not in matches:
                matches.append(line)
            if len(matches) >= max_matches:
                return matches
    return matches


def _collect_memory_signals(context: dict[str, object]) -> dict[str, object]:
    selected = _list_dicts(context.get("selected_memory_snapshots"))
    failure_memories = _list_dicts(context.get("failure_memories"))
    project_rules = _list_dicts(context.get("project_rules"))
    decision_records = _list_dicts(context.get("decision_records"))
    return {
        "selected_memory": selected,
        "failure_memories": failure_memories,
        "project_rules": project_rules,
        "decision_records": decision_records,
        "selected_count": len(selected),
        "failure_count": len(failure_memories),
        "rule_count": len(project_rules),
    }


def _collect_execution_signals(context: dict[str, object]) -> dict[str, object]:
    implementation_runs = _list_dicts(context.get("implementation_runs"))
    verification_runs = _list_dicts(context.get("verification_runs"))
    review_results = _list_dicts(context.get("review_results"))
    work_packages = _list_dicts(context.get("recent_work_packages"))
    pending_approvals = _list_dicts(context.get("pending_approvals"))
    failed_verification = [
        run for run in verification_runs if str(run.get("status")) in {"failed", "blocked"}
    ]
    residual_reviews = [
        review for review in review_results if _string_list(review.get("residual_risks"))
    ]
    return {
        "implementation_runs": implementation_runs,
        "verification_runs": verification_runs,
        "review_results": review_results,
        "work_packages": work_packages,
        "pending_approvals": pending_approvals,
        "failed_verification": failed_verification,
        "residual_reviews": residual_reviews,
    }


def _build_risk_findings(
    *,
    request: RiskScanBackendRequest,
    repository_signals: dict[str, object],
    memory_signals: dict[str, object],
    execution_signals: dict[str, object],
) -> list[RiskFindingDraft]:
    findings: list[RiskFindingDraft] = []
    metrics = repository_signals["metrics"]
    metric_link = _source_link("repository_metric", "repository_metrics", "derived_from", "Repository metrics")

    failed_verification = _list_dicts(execution_signals.get("failed_verification"))
    failure_memories = _list_dicts(memory_signals.get("failure_memories"))
    if failed_verification or failure_memories:
        evidence = [
            f"{len(failed_verification)} failed or blocked verification runs are recorded.",
            f"{len(failure_memories)} failure memory items are active.",
        ]
        source_links = [metric_link]
        for run in failed_verification[:3]:
            source_links.append(
                _source_link(
                    "verification_run",
                    str(run.get("id", "unknown")),
                    "derived_from",
                    str(run.get("command", "verification run")),
                )
            )
        for memory in failure_memories[:3]:
            source_links.append(
                _source_link(
                    "memory_item",
                    str(memory.get("id", "unknown")),
                    "derived_from",
                    str(memory.get("title", "failure memory")),
                )
            )
        findings.append(
            RiskFindingDraft(
                category="verification",
                severity="high" if failed_verification else "medium",
                title="Recorded failures need follow-up before health is treated as stable",
                summary="Failure memory or failed verification records indicate unresolved quality risk.",
                evidence=evidence,
                recommendation="Accept this finding and convert it into a focused follow-up Work Package if the risk is still current.",
                confidence=0.86,
                source_links=source_links,
            )
        )

    residual_reviews = _list_dicts(execution_signals.get("residual_reviews"))
    if residual_reviews:
        review = residual_reviews[0]
        findings.append(
            RiskFindingDraft(
                category="implementation",
                severity="medium",
                title="Review residual risks are still visible in project history",
                summary="At least one ReviewResult records residual risks that should remain visible in Quality Center.",
                evidence=_string_list(review.get("residual_risks"))[:4] or ["Residual risk recorded."],
                recommendation="Review the linked ReviewResult and decide whether the residual risk needs a Work Package.",
                confidence=0.78,
                source_links=[
                    _source_link(
                        "review_result",
                        str(review.get("id", "unknown")),
                        "derived_from",
                        "Review residual risk",
                    )
                ],
            )
        )

    todo_matches = [str(item) for item in repository_signals.get("todo_matches", [])]
    if todo_matches:
        findings.append(
            RiskFindingDraft(
                category="implementation",
                severity="low",
                title="Repository contains TODO/FIXME markers",
                summary="Read-only grep found implementation markers that can hide deferred work.",
                evidence=todo_matches[:5],
                recommendation="Triage the markers and convert only actionable items into Work Packages.",
                confidence=0.72,
                source_links=[metric_link],
            )
        )

    boundary_hints = [str(item) for item in repository_signals.get("boundary_hints", [])]
    suspicious_boundary = [
        line for line in boundary_hints if line.startswith("apps/gui/") and "/internal/" in line
    ]
    if suspicious_boundary:
        findings.append(
            RiskFindingDraft(
                category="architecture",
                severity="high",
                title="GUI may be reaching across the Agent Backend boundary",
                summary="Risk Radar found GUI-side references to internal Agent Backend paths.",
                evidence=suspicious_boundary[:5],
                recommendation="Keep GUI calls routed through Control Plane APIs only.",
                confidence=0.82,
                source_links=[metric_link],
            )
        )

    pending_approvals = _list_dicts(execution_signals.get("pending_approvals"))
    if pending_approvals:
        findings.append(
            RiskFindingDraft(
                category="process",
                severity="medium",
                title="Pending approvals may delay implementation flow",
                summary="ApprovalRequest records are still pending and can hold Work Packages or patch proposals.",
                evidence=[f"{len(pending_approvals)} pending approvals are recorded."],
                recommendation="Review pending approvals before starting new implementation work.",
                confidence=0.8,
                source_links=[
                    _source_link(
                        "work_package",
                        str(pending_approvals[0].get("target_id", "pending_approval")),
                        "supports",
                        "Pending approval",
                    )
                ],
            )
        )

    if not bool(metrics.get("has_tests")):
        findings.append(
            RiskFindingDraft(
                category="verification",
                severity="medium",
                title="Repository-level tests were not detected",
                summary="The scan did not find a top-level tests directory in the read-only file list.",
                evidence=["repository_metrics.has_tests is false"],
                recommendation="Add or point Artemis at recorded verification coverage before relying on project health.",
                confidence=0.74,
                source_links=[metric_link],
            )
        )

    if not findings:
        findings.append(
            RiskFindingDraft(
                category="process",
                severity="info",
                title="No immediate high-risk source-linked issue detected",
                summary=f"The {request.scope_type} scan found no failed verification, residual review, or boundary risk.",
                evidence=["Repository and stored project signals were scanned read-only."],
                recommendation="Keep using Risk Radar after implementation or verification records are added.",
                confidence=0.66,
                source_links=[metric_link],
            )
        )
    return findings


def _build_quality_signals(
    *,
    repository_signals: dict[str, object],
    memory_signals: dict[str, object],
    execution_signals: dict[str, object],
) -> list[QualitySignalDraft]:
    metrics = repository_signals["metrics"]
    metric_link = _source_link("repository_metric", "repository_metrics", "derived_from", "Repository metrics")
    failed_verification = _list_dicts(execution_signals.get("failed_verification"))
    verification_runs = _list_dicts(execution_signals.get("verification_runs"))
    review_results = _list_dicts(execution_signals.get("review_results"))
    failure_memories = _list_dicts(memory_signals.get("failure_memories"))
    project_rules = _list_dicts(memory_signals.get("project_rules"))
    pending_approvals = _list_dicts(execution_signals.get("pending_approvals"))
    kind_counts = metrics.get("kind_counts", {}) if isinstance(metrics, dict) else {}
    return [
        QualitySignalDraft(
            kind="verification",
            status="at_risk" if failed_verification else ("healthy" if verification_runs else "unknown"),
            title="Recorded verification history",
            summary=(
                f"{len(verification_runs)} verification runs are recorded; "
                f"{len(failed_verification)} are failed or blocked."
            ),
            value={"total": len(verification_runs), "failed_or_blocked": len(failed_verification)},
            target={"failed_or_blocked": 0},
            evidence=[
                f"Verification runs: {len(verification_runs)}",
                f"Review results: {len(review_results)}",
            ],
            source_links=[metric_link],
        ),
        QualitySignalDraft(
            kind="coverage_hint",
            status="healthy" if bool(metrics.get("has_tests")) else "watch",
            title="Test artifact coverage hint",
            summary="Read-only repository metadata was used to identify test and smoke artifacts.",
            value={"has_tests": bool(metrics.get("has_tests")), "kind_counts": kind_counts},
            target={"has_tests": True},
            evidence=[f"kind_counts={kind_counts}"],
            source_links=[metric_link],
        ),
        QualitySignalDraft(
            kind="code_size",
            status="watch" if int(metrics.get("file_count", 0)) > 300 else "healthy",
            title="Repository size and module concentration",
            summary="File counts are grouped by top-level module to reveal hotspots.",
            value={
                "file_count": metrics.get("file_count", 0),
                "top_level_counts": metrics.get("top_level_counts", {}),
            },
            target={"file_count": "review manually above 300 files"},
            evidence=[f"file_count={metrics.get('file_count', 0)}"],
            source_links=[metric_link],
        ),
        QualitySignalDraft(
            kind="memory",
            status="watch" if failure_memories else ("healthy" if project_rules else "unknown"),
            title="Memory-derived quality signal",
            summary=(
                f"{len(project_rules)} active project rules and "
                f"{len(failure_memories)} active failure memories were included."
            ),
            value={"project_rules": len(project_rules), "failure_memories": len(failure_memories)},
            target={"failure_memories": 0},
            evidence=[
                f"Selected memory: {memory_signals.get('selected_count', 0)}",
                f"Project rules: {len(project_rules)}",
            ],
            source_links=[
                _source_link(
                    "memory_item",
                    str((project_rules or failure_memories or [{"id": "memory_summary"}])[0].get("id")),
                    "derived_from",
                    "Memory summary",
                )
            ]
            if (project_rules or failure_memories)
            else [metric_link],
        ),
        QualitySignalDraft(
            kind="process",
            status="watch" if pending_approvals else "healthy",
            title="Approval and process signal",
            summary=f"{len(pending_approvals)} pending approvals are currently visible.",
            value={"pending_approvals": len(pending_approvals)},
            target={"pending_approvals": 0},
            evidence=[f"pending_approvals={len(pending_approvals)}"],
            source_links=[metric_link],
        ),
    ]


def _build_architecture_map(repository_signals: dict[str, object]) -> ArchitectureMapSnapshotDraft:
    metrics = repository_signals["metrics"]
    top_level_counts = metrics.get("top_level_counts", {}) if isinstance(metrics, dict) else {}
    expected_modules = [
        "apps/gui",
        "services/control_plane",
        "services/agent_backend",
        "tests",
        "scripts",
        "docs",
    ]
    files = [str(item) for item in repository_signals.get("files", [])]
    nodes = []
    for module in expected_modules:
        count = sum(1 for file_path in files if file_path == module or file_path.startswith(f"{module}/"))
        nodes.append(
            {
                "id": module,
                "label": module,
                "kind": "module",
                "file_count": count,
                "present": count > 0,
            }
        )
    for top_level, count in sorted(top_level_counts.items()):
        if not any(node["id"] == top_level for node in nodes):
            nodes.append(
                {
                    "id": top_level,
                    "label": top_level,
                    "kind": "top_level",
                    "file_count": count,
                    "present": True,
                }
            )
    edges = [
        {"from": "apps/gui", "to": "services/control_plane", "relation": "HTTP API"},
        {"from": "services/control_plane", "to": "services/agent_backend", "relation": "internal API"},
        {"from": "services/control_plane", "to": "SQLite store", "relation": "canonical state"},
        {"from": "services/agent_backend", "to": "read-only tools", "relation": "analysis input"},
    ]
    hotspots = sorted(
        [
            {"module": str(module), "file_count": int(count)}
            for module, count in top_level_counts.items()
        ],
        key=lambda item: item["file_count"],
        reverse=True,
    )[:5]
    boundary_notes = [
        "GUI must call Control Plane only.",
        "Control Plane owns canonical product state.",
        "Agent Backend returns structured analysis candidates and does not persist findings.",
    ]
    return ArchitectureMapSnapshotDraft(
        nodes=nodes,
        edges=edges,
        hotspots=hotspots,
        boundary_notes=boundary_notes,
    )


def _build_project_health_snapshot(
    findings: list[RiskFindingDraft],
    quality_signals: list[QualitySignalDraft],
) -> ProjectHealthSnapshotDraft:
    severity_weights = {"critical": 32, "high": 22, "medium": 12, "low": 5, "info": 1}
    risk_counts = {severity: 0 for severity in severity_weights}
    penalty = 0
    for finding in findings:
        risk_counts[finding.severity] += 1
        penalty += severity_weights[finding.severity]
    score = max(0.0, min(100.0, 100.0 - penalty))
    if risk_counts["critical"]:
        status = "blocked"
    elif risk_counts["high"] or score < 60:
        status = "at_risk"
    elif risk_counts["medium"] or any(signal.status == "watch" for signal in quality_signals):
        status = "watch"
    else:
        status = "healthy"
    quality_summary: dict[str, Any] = {}
    for signal in quality_signals:
        quality_summary[signal.kind] = signal.status
    top_findings = [
        {
            "title": finding.title,
            "severity": finding.severity,
            "category": finding.category,
            "confidence": finding.confidence,
        }
        for finding in findings[:5]
    ]
    return ProjectHealthSnapshotDraft(
        overall_status=status,  # type: ignore[arg-type]
        overall_score=score,
        risk_counts=risk_counts,
        top_findings=top_findings,
        quality_summary=quality_summary,
        recommendation=_health_recommendation(status),
    )


def _rank_and_dedupe_findings(findings: list[RiskFindingDraft]) -> list[RiskFindingDraft]:
    severity_rank = {"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
    seen: set[tuple[str, str]] = set()
    deduped: list[RiskFindingDraft] = []
    for finding in sorted(findings, key=lambda item: (severity_rank[item.severity], item.title)):
        key = (finding.category, finding.title.lower())
        if key in seen:
            continue
        seen.add(key)
        deduped.append(finding)
    return deduped


def _validate_risk_analysis(
    findings: list[RiskFindingDraft],
    quality_signals: list[QualitySignalDraft],
    health_snapshot: ProjectHealthSnapshotDraft,
    architecture_map: ArchitectureMapSnapshotDraft,
) -> list[str]:
    errors: list[str] = []
    if not findings:
        errors.append("findings must be non-empty")
    if not quality_signals:
        errors.append("quality_signals must be non-empty")
    for finding in findings:
        errors.extend(finding.validate())
    for signal in quality_signals:
        errors.extend(signal.validate())
    if not architecture_map.nodes:
        errors.append("architecture_map.nodes must be non-empty")
    if not health_snapshot.recommendation.strip():
        errors.append("project_health_snapshot.recommendation is required")
    return errors


def _health_recommendation(status: str) -> str:
    if status == "blocked":
        return "Resolve critical findings before expanding implementation scope."
    if status == "at_risk":
        return "Accept and convert the highest severity finding that still applies."
    if status == "watch":
        return "Review medium findings and keep verification records current."
    if status == "healthy":
        return "No immediate action is required; rescan after new implementation work."
    return "Project health is unknown until more signals are recorded."


def _source_link(
    source_type: str,
    source_id: str,
    relation: str,
    label: str,
) -> dict[str, str]:
    return {
        "source_type": source_type,
        "source_id": source_id,
        "relation": relation,
        "label": label,
    }


def _list_dicts(value: object) -> list[dict[str, object]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, dict)]
