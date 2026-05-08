"""MVP 1 LangGraph root workflow."""

from __future__ import annotations

import re
from typing import Any, TypedDict

from .config import model_for_role
from .observability import LangSmithTracer
from .schemas import (
    AgentBackendEvent,
    AgentBackendRequest,
    ContextSummary,
    FinalAgentRunResult,
    Intent,
    IntentResult,
    RiskHint,
    WorkPackageDraft,
)
from .tools import ReadOnlyToolRouter


class ArtemisGraphState(TypedDict, total=False):
    request: AgentBackendRequest
    trace_id: str
    trace_enabled: bool
    events: list[AgentBackendEvent]
    intent_result: IntentResult
    context_summary: ContextSummary
    work_package: WorkPackageDraft
    validation_errors: list[str]
    status: str


def build_langgraph(runner: "MVP1GraphRunner") -> object | None:
    """Build the actual LangGraph workflow when the dependency is installed."""

    try:
        from langgraph.graph import END, START, StateGraph
    except ImportError:
        return None

    graph = StateGraph(ArtemisGraphState)
    graph.add_node("classify_intent", runner._node_classify_intent)
    graph.add_node("collect_context", runner._node_collect_context)
    graph.add_node("create_work_package", runner._node_create_work_package)
    graph.add_node("validate_work_package", runner._node_validate_work_package)
    graph.add_edge(START, "classify_intent")
    graph.add_edge("classify_intent", "collect_context")
    graph.add_edge("collect_context", "create_work_package")
    graph.add_edge("create_work_package", "validate_work_package")
    graph.add_edge("validate_work_package", END)
    return graph.compile()


class MVP1GraphRunner:
    def __init__(self, tracer: LangSmithTracer | None = None) -> None:
        self.tracer = tracer or LangSmithTracer()

    def run(self, request: AgentBackendRequest) -> FinalAgentRunResult:
        trace = self.tracer.start_trace(
            project_id=request.project_id,
            session_id=request.session_id,
            agent_run_id=request.agent_run_id,
        )
        state: ArtemisGraphState = {
            "request": request,
            "trace_id": trace.trace_id,
            "trace_enabled": trace.enabled,
            "events": [
                AgentBackendEvent(
                    "trace.langsmith_linked",
                    {
                        "trace_id": trace.trace_id,
                        "run_id": trace.run_id,
                        "live": trace.enabled and trace.live_tracing_available,
                        "requested": trace.requested,
                        "api_key_available": trace.api_key_available,
                    },
                )
            ],
        }
        inputs = {
            "project_id": request.project_id,
            "session_id": request.session_id,
            "agent_run_id": request.agent_run_id,
            "user_request": request.user_request,
        }
        metadata = {
            "project_id": request.project_id,
            "session_id": request.session_id,
            "agent_run_id": request.agent_run_id,
        }

        with self.tracer.trace_run(
            trace,
            name="artemis_mvp1_root_graph",
            inputs=inputs,
            metadata=metadata,
        ) as run:
            final_state = self._execute_workflow(state)
            self.tracer.end_trace(
                run,
                outputs={
                    "status": final_state.get("status", "failed"),
                    "intent": final_state.get("intent_result").intent
                    if final_state.get("intent_result")
                    else None,
                },
            )

        return self._result_from_state(final_state)

    def _execute_workflow(self, state: ArtemisGraphState) -> ArtemisGraphState:
        graph = build_langgraph(self)
        if graph is not None:
            state["events"] = [
                *state.get("events", []),
                AgentBackendEvent("agent_run.graph_runtime", {"runtime": "langgraph"}),
            ]
            return graph.invoke(state)

        state["events"] = [
            *state.get("events", []),
            AgentBackendEvent("agent_run.graph_runtime", {"runtime": "sequential_fallback"}),
        ]
        for node in (
            self._node_classify_intent,
            self._node_collect_context,
            self._node_create_work_package,
            self._node_validate_work_package,
        ):
            updates = node(state)
            state = {**state, **updates}
        return state

    def _node_classify_intent(self, state: ArtemisGraphState) -> ArtemisGraphState:
        request = state["request"]
        return {
            "events": [
                *state.get("events", []),
                AgentBackendEvent("agent_run.phase_changed", {"phase": "classify_intent"}),
            ],
            "intent_result": self.classify_intent(request.user_request),
        }

    def _node_collect_context(self, state: ArtemisGraphState) -> ArtemisGraphState:
        request = state["request"]
        intent = state["intent_result"]
        context = self.collect_context(request, intent.intent)
        return {
            "events": [
                *state.get("events", []),
                AgentBackendEvent("agent_run.phase_changed", {"phase": "collect_context"}),
                AgentBackendEvent("context.collection_started", {}),
                AgentBackendEvent(
                    "context.collection_completed",
                    {"files_considered": len(context.files_considered)},
                ),
            ],
            "context_summary": context,
        }

    def _node_create_work_package(self, state: ArtemisGraphState) -> ArtemisGraphState:
        request = state["request"]
        intent = state["intent_result"]
        context = state["context_summary"]
        work_package = self.create_work_package(request, intent, context)
        return {
            "events": [
                *state.get("events", []),
                AgentBackendEvent("agent_run.phase_changed", {"phase": "create_work_package"}),
                AgentBackendEvent(
                    "work_package.draft_created",
                    {"title": work_package.title, "risk_level": work_package.risks[0].level},
                ),
            ],
            "work_package": work_package,
        }

    def _node_validate_work_package(self, state: ArtemisGraphState) -> ArtemisGraphState:
        work_package = state["work_package"]
        errors = work_package.validate()
        events = [
            *state.get("events", []),
            AgentBackendEvent("agent_run.phase_changed", {"phase": "validate_work_package"}),
        ]
        if errors:
            events.append(AgentBackendEvent("work_package.validation_failed", {"errors": errors}))
            return {"events": events, "validation_errors": errors, "status": "failed"}

        events.append(AgentBackendEvent("work_package.validation_passed", {}))
        events.append(AgentBackendEvent("agent_run.completed", {}))
        return {"events": events, "validation_errors": [], "status": "completed"}

    def _result_from_state(self, state: ArtemisGraphState) -> FinalAgentRunResult:
        intent = state["intent_result"]
        context = state["context_summary"]
        work_package = state.get("work_package")
        errors = state.get("validation_errors", [])
        status = "failed" if errors else "completed"
        return FinalAgentRunResult(
            status=status,
            intent_result=intent,
            context_summary=context,
            work_package=work_package,
            risk_hints=work_package.risks if work_package else [],
            langsmith_trace_id=state["trace_id"],
            events=state.get("events", []),
            errors=errors,
        )

    def classify_intent(self, user_request: str) -> IntentResult:
        text = user_request.lower()
        intent: Intent = "unknown"
        rationale = "No strong keyword match."
        patterns: list[tuple[Intent, tuple[str, ...], str]] = [
            ("bug_investigation", ("bug", "error", "fail", "crash", "fix"), "Bug/failure terms found."),
            ("refactor_request", ("refactor", "cleanup", "restructure"), "Refactor terms found."),
            (
                "architecture_question",
                ("architecture", "design", "boundary", "tradeoff"),
                "Architecture terms found.",
            ),
            ("documentation_request", ("doc", "readme", "guide"), "Documentation terms found."),
            ("planning_request", ("plan", "roadmap", "mvp", "scope"), "Planning terms found."),
            ("feature_request", ("add", "create", "implement", "support"), "Feature terms found."),
        ]
        for candidate, keywords, reason in patterns:
            if any(keyword in text for keyword in keywords):
                intent = candidate
                rationale = reason
                break

        selection = model_for_role("validator")
        return IntentResult(
            intent=intent,
            confidence=0.75 if intent != "unknown" else 0.35,
            rationale=rationale,
            model_role="validator",
            model_name=selection.model,
        )

    def collect_context(self, request: AgentBackendRequest, intent: Intent) -> ContextSummary:
        tools = ReadOnlyToolRouter(request.project_root)
        git_status = tools.git_status().output
        files_result = tools.list_files()
        files = [line for line in files_result.output.splitlines() if line]
        keywords = self._keywords(request.user_request)
        related: list[str] = []
        for keyword in keywords[:5]:
            grep_result = tools.grep(keyword, max_matches=10)
            for line in grep_result.output.splitlines():
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
        considered = preferred[:30] or files[:30]
        summary = (
            f"Collected minimal read-only context for intent '{intent}'. "
            f"{len(files)} files listed, {len(related)} related files identified."
        )
        return ContextSummary(
            repository_root=request.project_root,
            git_status=git_status,
            files_considered=considered,
            related_files=related[:20] or considered[:5],
            summary=summary,
        )

    def create_work_package(
        self,
        request: AgentBackendRequest,
        intent: IntentResult,
        context: ContextSummary,
    ) -> WorkPackageDraft:
        title = self._title_from_request(request.user_request)
        required_agents = self._required_agents(intent.intent)
        risk_level = self._risk_level(intent.intent)
        writer_model = model_for_role("work_package_writer").model
        return WorkPackageDraft(
            title=title,
            goal=f"Structure the user request into an approved Artemis Work Package: {request.user_request}",
            background=(
                "MVP 1 converts natural-language requests into tracked, reviewable work. "
                f"The draft was created with the {writer_model} role profile available."
            ),
            scope=[
                "Clarify the requested outcome.",
                "Identify relevant project context using read-only tools.",
                "Create an implementation-ready Work Package draft.",
                "Hold execution until approval is granted.",
            ],
            out_of_scope=[
                "Writing project files.",
                "Applying patches.",
                "Running tests or shell commands.",
                "Committing code changes.",
            ],
            related_files=context.related_files or context.files_considered[:5],
            required_agents=required_agents,
            implementation_steps=[
                "Review this Work Package draft.",
                "Approve or reject the proposed scope.",
                "After approval, hand off to a later implementation MVP.",
            ],
            verification=[
                "schema validation",
                "event log consistency check",
                "approval request creation check",
            ],
            risks=[
                RiskHint(
                    level=risk_level,
                    description="Scope may need refinement before implementation begins.",
                )
            ],
            approval_required=True,
            completion_criteria=[
                "Work Package is stored by Control Plane.",
                "ApprovalRequest is pending.",
                "AgentRun has a trace correlation id.",
            ],
        )

    def _keywords(self, user_request: str) -> list[str]:
        return [word for word in re.findall(r"[A-Za-z0-9_가-힣]{3,}", user_request)][:12]

    def _title_from_request(self, user_request: str) -> str:
        cleaned = " ".join(user_request.strip().split())
        if not cleaned:
            return "Untitled Work Package"
        if len(cleaned) <= 72:
            return cleaned
        return f"{cleaned[:69].rstrip()}..."

    def _required_agents(self, intent: Intent) -> list[str]:
        mapping = {
            "feature_request": ["ProductPlanner", "Architect", "BackendEngineer", "QAEngineer"],
            "bug_investigation": ["Debugger", "Explorer", "QAEngineer", "Reviewer"],
            "refactor_request": ["Architect", "RefactorSpecialist", "QAEngineer", "Reviewer"],
            "architecture_question": ["Architect", "DevilAdvocate", "SecurityReviewer"],
            "documentation_request": ["DocumentationWriter", "Reviewer"],
            "planning_request": ["ProjectManager", "ProductPlanner", "Architect"],
            "unknown": ["ProjectManager", "Architect"],
        }
        return mapping[intent]

    def _risk_level(self, intent: Intent) -> str:
        if intent in {"refactor_request", "bug_investigation"}:
            return "medium"
        if intent == "architecture_question":
            return "high"
        return "low"
