import {
  Activity,
  Check,
  CircleAlert,
  CircleDot,
  Code2,
  Archive,
  Database,
  FileText,
  FolderOpen,
  GitPullRequest,
  Map as MapIcon,
  Play,
  RefreshCw,
  RotateCcw,
  Search,
  Send,
  Server,
  ShieldCheck,
  X
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { controlPlaneApi } from "./api";
import type {
  AgentRun,
  AgentRunResult,
  BackendStatus,
  BrainstormingMode,
  BrainstormingResult,
  BrainstormingSourceType,
  DecisionRecord,
  EventRecord,
  ImplementationRunResult,
  MemorySearchResult,
  MemoryType,
  Project,
  ProjectMemoryItem,
  QualitySnapshot,
  RiskFinding,
  RiskFindingStatus,
  RiskRadar,
  RiskScanResult,
  SelectedMemoryContext,
  Session,
  WorkPackage
} from "./types";

const DEFAULT_PROJECT_ROOT = import.meta.env.VITE_DEFAULT_PROJECT_ROOT ?? "D:\\Artemis_Project";
const TERMINAL_STATES = new Set(["completed", "failed", "canceled"]);
const SSE_EVENT_TYPES = [
  "agent_run.created",
  "agent_run.started",
  "agent_run.phase_changed",
  "agent_run.completed",
  "agent_run.failed",
  "agent_run.canceled",
  "agent_run.graph_runtime",
  "context.collection_started",
  "context.collection_completed",
  "work_package.draft_created",
  "work_package.validation_passed",
  "work_package.validation_failed",
  "work_package.created",
  "work_package.pending_approval",
  "approval.requested",
  "approval.approved",
  "approval.rejected",
  "artifact.created",
  "trace.linked"
];
const IMPLEMENTATION_SSE_EVENT_TYPES = [
  "implementation_run.created",
  "implementation_run.started",
  "implementation_run.phase_changed",
  "implementation_run.completed",
  "implementation_run.failed",
  "implementation_run.canceled",
  "implementation_plan.created",
  "patch_set.proposed",
  "patch_set.validation_passed",
  "patch_set.validation_failed",
  "patch_set.pending_approval",
  "patch_set.approved",
  "patch_set.rejected",
  "patch_set.apply_started",
  "patch_set.applied",
  "patch_set.apply_failed",
  "verification.started",
  "verification.completed",
  "verification.failed",
  "verification.blocked",
  "review.started",
  "review.completed",
  "artifact.created",
  "trace.step_recorded"
];
const BRAINSTORMING_SSE_EVENT_TYPES = [
  "brainstorming_session.created",
  "brainstorming_session.started",
  "brainstorming_session.phase_changed",
  "brainstorming_session.completed",
  "brainstorming_session.failed",
  "brainstorming_session.canceled",
  "brainstorming.context_collected",
  "brainstorming.roles_selected",
  "brainstorming.role_started",
  "brainstorming.role_completed",
  "brainstorming.role_failed",
  "brainstorming.critique_created",
  "brainstorming.option_created",
  "brainstorming.decision_brief_created",
  "brainstorming.validation_passed",
  "brainstorming.validation_failed",
  "decision_record.accepted",
  "decision_record.rejected",
  "decision_record.created",
  "work_package.conversion_requested",
  "work_package.conversion_completed",
  "work_package.pending_approval",
  "artifact.created",
  "trace.linked",
  "trace.step_recorded"
];
const BRAINSTORMING_TERMINAL_STATES = new Set([
  "awaiting_decision",
  "accepted",
  "rejected",
  "converted",
  "failed",
  "canceled"
]);
const RISK_SCAN_SSE_EVENT_TYPES = [
  "risk_scan.created",
  "risk_scan.started",
  "risk_scan.phase_changed",
  "risk_scan.completed",
  "risk_scan.failed",
  "risk_finding.created",
  "risk_finding.updated",
  "risk_finding.accepted",
  "risk_finding.dismissed",
  "risk_finding.mitigated",
  "risk_finding.converted_to_work_package",
  "quality_signal.created",
  "quality_snapshot.created",
  "architecture_map.created",
  "project_health_snapshot.created",
  "selected_memory.attached_to_risk_scan",
  "artifact.created",
  "trace.linked",
  "trace.step_recorded"
];
const RISK_SCAN_TERMINAL_STATES = new Set(["completed", "failed", "canceled"]);
const BRAINSTORMING_ROLES = [
  "product_planner",
  "system_architect",
  "implementation_planner",
  "risk_reviewer",
  "devil_advocate"
];

export function App() {
  const [backendStatus, setBackendStatus] = useState<BackendStatus>("checking");
  const [projects, setProjects] = useState<Project[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentProject, setCurrentProject] = useState<Project | null>(null);
  const [currentSession, setCurrentSession] = useState<Session | null>(null);
  const [projectName, setProjectName] = useState("Artemis Project");
  const [projectRoot, setProjectRoot] = useState(DEFAULT_PROJECT_ROOT);
  const [sessionTitle, setSessionTitle] = useState("MVP 2 Session");
  const [requestText, setRequestText] = useState(
    "Create an MVP 2 GUI event stream slice for Artemis."
  );
  const [agentRun, setAgentRun] = useState<AgentRun | null>(null);
  const [events, setEvents] = useState<EventRecord[]>([]);
  const [result, setResult] = useState<AgentRunResult | null>(null);
  const [implementationResult, setImplementationResult] = useState<ImplementationRunResult | null>(
    null
  );
  const [implementationEvents, setImplementationEvents] = useState<EventRecord[]>([]);
  const [brainstormingTopic, setBrainstormingTopic] = useState(
    "Review MVP 4 Brainstorming Room scope and conversion path."
  );
  const [brainstormingMode, setBrainstormingMode] =
    useState<BrainstormingMode>("architecture_debate");
  const [brainstormingSourceType, setBrainstormingSourceType] =
    useState<BrainstormingSourceType>("topic");
  const [selectedBrainstormingRoles, setSelectedBrainstormingRoles] = useState<string[]>([
    "product_planner",
    "system_architect",
    "implementation_planner",
    "risk_reviewer"
  ]);
  const [brainstormingResult, setBrainstormingResult] = useState<BrainstormingResult | null>(null);
  const [brainstormingEvents, setBrainstormingEvents] = useState<EventRecord[]>([]);
  const [memoryItems, setMemoryItems] = useState<ProjectMemoryItem[]>([]);
  const [memorySearchResults, setMemorySearchResults] = useState<MemorySearchResult[]>([]);
  const [selectedMemory, setSelectedMemory] = useState<SelectedMemoryContext | null>(null);
  const [memoryDetail, setMemoryDetail] = useState<ProjectMemoryItem | null>(null);
  const [memoryQuery, setMemoryQuery] = useState("Control Plane");
  const [memoryTypeFilter, setMemoryTypeFilter] = useState<MemoryType | "">("");
  const [memoryStatusFilter, setMemoryStatusFilter] = useState("active");
  const [memoryTab, setMemoryTab] = useState<
    "search" | "decisions" | "rules" | "failures" | "sessions" | "selected"
  >("search");
  const [riskScanResult, setRiskScanResult] = useState<RiskScanResult | null>(null);
  const [riskRadar, setRiskRadar] = useState<RiskRadar | null>(null);
  const [qualitySnapshot, setQualitySnapshot] = useState<QualitySnapshot | null>(null);
  const [riskEvents, setRiskEvents] = useState<EventRecord[]>([]);
  const [riskDetail, setRiskDetail] = useState<RiskFinding | null>(null);
  const [includeRiskMemory, setIncludeRiskMemory] = useState(true);
  const [riskScopeType, setRiskScopeType] = useState<"project" | "session">("project");
  const [ruleDraft, setRuleDraft] = useState({
    title: "GUI calls Control Plane only",
    summary: "The GUI must not call Agent Backend directly.",
    body: "All GUI actions go through Control Plane APIs. Agent Backend remains an internal service boundary.",
    tags: "architecture, gui, control-plane"
  });
  const [activeTab, setActiveTab] = useState<
    "timeline" | "trace" | "artifacts" | "implementation" | "brainstorming" | "memory" | "risk"
  >("timeline");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastEventIdRef = useRef<string | undefined>();
  const lastImplementationEventIdRef = useRef<string | undefined>();
  const lastBrainstormingEventIdRef = useRef<string | undefined>();
  const lastRiskEventIdRef = useRef<string | undefined>();

  const currentStatus = agentRun?.status ?? "idle";
  const selectedTrace =
    riskScanResult?.trace ?? brainstormingResult?.trace ?? implementationResult?.trace ?? result?.trace ?? null;
  const selectedArtifacts = result?.artifacts ?? [];
  const memoryEvents = useMemo(() => {
    const allEvents = [...events, ...implementationEvents, ...brainstormingEvents];
    return allEvents.filter((event) => event.type.startsWith("memory.") || event.type.includes("memory"));
  }, [brainstormingEvents, events, implementationEvents]);

  const refreshBackend = useCallback(async () => {
    setBackendStatus("checking");
    try {
      await controlPlaneApi.health();
      const loadedProjects = await controlPlaneApi.listProjects();
      setProjects(loadedProjects);
      setBackendStatus("online");
      if (!currentProject && loadedProjects[0]) {
        setCurrentProject(loadedProjects[0]);
      }
    } catch (err) {
      setBackendStatus("offline");
      setError(errorMessage(err));
    }
  }, [currentProject]);

  useEffect(() => {
    void refreshBackend();
  }, [refreshBackend]);

  useEffect(() => {
    if (!currentProject) {
      setSessions([]);
      setCurrentSession(null);
      return;
    }
    let canceled = false;
    controlPlaneApi
      .listSessions(currentProject.id)
      .then((loadedSessions) => {
        if (canceled) return;
        setSessions(loadedSessions);
        setCurrentSession(loadedSessions[0] ?? null);
      })
      .catch((err) => setError(errorMessage(err)));
    return () => {
      canceled = true;
    };
  }, [currentProject]);

  useEffect(() => {
    setAgentRun(null);
    setEvents([]);
    setResult(null);
    setImplementationResult(null);
    setImplementationEvents([]);
    setBrainstormingResult(null);
    setBrainstormingEvents([]);
    setMemoryItems([]);
    setMemorySearchResults([]);
    setSelectedMemory(null);
    setMemoryDetail(null);
    setRiskScanResult(null);
    setRiskRadar(null);
    setQualitySnapshot(null);
    setRiskEvents([]);
    setRiskDetail(null);
    lastEventIdRef.current = undefined;
    lastImplementationEventIdRef.current = undefined;
    lastBrainstormingEventIdRef.current = undefined;
    lastRiskEventIdRef.current = undefined;
  }, [currentProject?.id]);

  const refreshMemory = useCallback(async () => {
    if (!currentProject) return;
    const [items, searchResults, selected] = await Promise.all([
      controlPlaneApi.listMemory(currentProject.id, undefined, memoryStatusFilter),
      controlPlaneApi.searchMemory({
        projectId: currentProject.id,
        query: memoryQuery,
        type: memoryTypeFilter,
        status: memoryStatusFilter
      }),
      currentSession
        ? controlPlaneApi.listSelectedMemory(currentSession.id)
        : Promise.resolve(null)
    ]);
    setMemoryItems(items);
    setMemorySearchResults(searchResults);
    setSelectedMemory(selected);
    setMemoryDetail((previous) => {
      if (!previous) return searchResults[0]?.item ?? items[0] ?? null;
      return searchResults.find((result) => result.item.id === previous.id)?.item
        ?? items.find((item) => item.id === previous.id)
        ?? previous;
    });
  }, [currentProject, currentSession, memoryQuery, memoryStatusFilter, memoryTypeFilter]);

  useEffect(() => {
    void refreshMemory().catch((err) => setError(errorMessage(err)));
  }, [refreshMemory]);

  const refreshRisk = useCallback(async () => {
    if (!currentProject) return;
    const [radar, quality] = await Promise.all([
      controlPlaneApi.getRiskRadar(currentProject.id),
      controlPlaneApi.getQualitySnapshot(currentProject.id)
    ]);
    setRiskRadar(radar);
    setQualitySnapshot(quality);
    setRiskDetail((previous) => {
      if (!previous) return radar.findings[0] ?? null;
      return radar.findings.find((finding) => finding.id === previous.id) ?? previous;
    });
  }, [currentProject]);

  useEffect(() => {
    void refreshRisk().catch((err) => setError(errorMessage(err)));
  }, [refreshRisk]);

  useEffect(() => {
    if (!agentRun?.id) return;
    let canceled = false;
    let source: EventSource | null = null;

    const mergeEvents = (nextEvents: EventRecord[]) => {
      if (!nextEvents.length) return;
      setEvents((previous) => mergeEventRecords(previous, nextEvents));
      lastEventIdRef.current = nextEvents[nextEvents.length - 1].id;
    };

    const refreshRun = async () => {
      const run = await controlPlaneApi.getAgentRun(agentRun.id);
      if (canceled) return;
      setAgentRun(run);
      if (!source) {
        const nextEvents = await controlPlaneApi.listEvents(agentRun.id, lastEventIdRef.current);
        if (!canceled) mergeEvents(nextEvents);
      }
      if (TERMINAL_STATES.has(run.status)) {
        const nextResult = await controlPlaneApi.getAgentRunResult(agentRun.id);
        if (!canceled) setResult(nextResult);
      }
    };

    if ("EventSource" in window) {
      source = new EventSource(controlPlaneApi.eventStreamUrl(agentRun.id));
      const handleSse = (message: MessageEvent<string>) => {
        try {
          mergeEvents([JSON.parse(message.data) as EventRecord]);
        } catch (err) {
          setError(errorMessage(err));
        }
      };
      for (const type of SSE_EVENT_TYPES) {
        source.addEventListener(type, handleSse as EventListener);
      }
      source.onerror = () => {
        source?.close();
        source = null;
      };
    }

    const interval = window.setInterval(() => {
      void refreshRun().catch((err) => setError(errorMessage(err)));
    }, 900);
    void refreshRun().catch((err) => setError(errorMessage(err)));

    return () => {
      canceled = true;
      window.clearInterval(interval);
      source?.close();
    };
  }, [agentRun?.id]);

  useEffect(() => {
    const implementationRunId = implementationResult?.implementation_run.id;
    if (!implementationRunId) return;
    let canceled = false;
    let source: EventSource | null = null;

    const mergeImplementationEvents = (nextEvents: EventRecord[]) => {
      if (!nextEvents.length) return;
      setImplementationEvents((previous) => mergeEventRecords(previous, nextEvents));
      lastImplementationEventIdRef.current = nextEvents[nextEvents.length - 1].id;
    };

    const refreshImplementationRun = async () => {
      const nextResult = await controlPlaneApi.getImplementationRunResult(implementationRunId);
      if (canceled) return;
      setImplementationResult(nextResult);
      if (!source) {
        const nextEvents = await controlPlaneApi.listImplementationEvents(
          implementationRunId,
          lastImplementationEventIdRef.current
        );
        if (!canceled) mergeImplementationEvents(nextEvents);
      }
    };

    if ("EventSource" in window) {
      source = new EventSource(controlPlaneApi.implementationEventStreamUrl(implementationRunId));
      const handleSse = (message: MessageEvent<string>) => {
        try {
          mergeImplementationEvents([JSON.parse(message.data) as EventRecord]);
        } catch (err) {
          setError(errorMessage(err));
        }
      };
      for (const type of IMPLEMENTATION_SSE_EVENT_TYPES) {
        source.addEventListener(type, handleSse as EventListener);
      }
      source.onerror = () => {
        source?.close();
        source = null;
      };
    }

    const interval = window.setInterval(() => {
      void refreshImplementationRun().catch((err) => setError(errorMessage(err)));
    }, 900);
    void refreshImplementationRun().catch((err) => setError(errorMessage(err)));

    return () => {
      canceled = true;
      window.clearInterval(interval);
      source?.close();
    };
  }, [implementationResult?.implementation_run.id]);

  useEffect(() => {
    const brainstormingSessionId = brainstormingResult?.brainstorming_session.id;
    if (!brainstormingSessionId) return;
    let canceled = false;
    let source: EventSource | null = null;

    const mergeBrainstormingEvents = (nextEvents: EventRecord[]) => {
      if (!nextEvents.length) return;
      setBrainstormingEvents((previous) => mergeEventRecords(previous, nextEvents));
      lastBrainstormingEventIdRef.current = nextEvents[nextEvents.length - 1].id;
    };

    const refreshBrainstorming = async () => {
      const session = await controlPlaneApi.getBrainstormingSession(brainstormingSessionId);
      if (canceled) return;
      if (BRAINSTORMING_TERMINAL_STATES.has(session.status)) {
        const nextResult = await controlPlaneApi.getBrainstormingResult(brainstormingSessionId);
        if (!canceled) setBrainstormingResult(nextResult);
      }
      if (!source) {
        const nextEvents = await controlPlaneApi.listBrainstormingEvents(
          brainstormingSessionId,
          lastBrainstormingEventIdRef.current
        );
        if (!canceled) mergeBrainstormingEvents(nextEvents);
      }
    };

    if ("EventSource" in window) {
      source = new EventSource(controlPlaneApi.brainstormingEventStreamUrl(brainstormingSessionId));
      const handleSse = (message: MessageEvent<string>) => {
        try {
          mergeBrainstormingEvents([JSON.parse(message.data) as EventRecord]);
        } catch (err) {
          setError(errorMessage(err));
        }
      };
      for (const type of BRAINSTORMING_SSE_EVENT_TYPES) {
        source.addEventListener(type, handleSse as EventListener);
      }
      source.onerror = () => {
        source?.close();
        source = null;
      };
    }

    const interval = window.setInterval(() => {
      void refreshBrainstorming().catch((err) => setError(errorMessage(err)));
    }, 900);
    void refreshBrainstorming().catch((err) => setError(errorMessage(err)));

    return () => {
      canceled = true;
      window.clearInterval(interval);
      source?.close();
    };
  }, [brainstormingResult?.brainstorming_session.id]);

  useEffect(() => {
    const riskScanRunId = riskScanResult?.risk_scan_run.id;
    if (!riskScanRunId) return;
    let canceled = false;
    let source: EventSource | null = null;

    const mergeRiskEvents = (nextEvents: EventRecord[]) => {
      if (!nextEvents.length) return;
      setRiskEvents((previous) => mergeEventRecords(previous, nextEvents));
      lastRiskEventIdRef.current = nextEvents[nextEvents.length - 1].id;
    };

    const refreshRiskScan = async () => {
      const run = await controlPlaneApi.getRiskScan(riskScanRunId);
      if (canceled) return;
      if (RISK_SCAN_TERMINAL_STATES.has(run.status)) {
        const nextResult = await controlPlaneApi.getRiskScanResult(riskScanRunId);
        if (!canceled) {
          setRiskScanResult(nextResult);
          setRiskDetail((previous) => previous ?? nextResult.findings[0] ?? null);
          await refreshRisk();
        }
      }
      if (!source) {
        const nextEvents = await controlPlaneApi.listRiskScanEvents(
          riskScanRunId,
          lastRiskEventIdRef.current
        );
        if (!canceled) mergeRiskEvents(nextEvents);
      }
    };

    if ("EventSource" in window) {
      source = new EventSource(controlPlaneApi.riskScanEventStreamUrl(riskScanRunId));
      const handleSse = (message: MessageEvent<string>) => {
        try {
          mergeRiskEvents([JSON.parse(message.data) as EventRecord]);
        } catch (err) {
          setError(errorMessage(err));
        }
      };
      for (const type of RISK_SCAN_SSE_EVENT_TYPES) {
        source.addEventListener(type, handleSse as EventListener);
      }
      source.onerror = () => {
        source?.close();
        source = null;
      };
    }

    const interval = window.setInterval(() => {
      void refreshRiskScan().catch((err) => setError(errorMessage(err)));
    }, 900);
    void refreshRiskScan().catch((err) => setError(errorMessage(err)));

    return () => {
      canceled = true;
      window.clearInterval(interval);
      source?.close();
    };
  }, [refreshRisk, riskScanResult?.risk_scan_run.id]);

  const openProject = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const project = await controlPlaneApi.openProject(projectName, projectRoot);
      setCurrentProject(project);
      setProjects((previous) => [project, ...previous.filter((item) => item.id !== project.id)]);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const createSession = async (event?: FormEvent) => {
    event?.preventDefault();
    if (!currentProject) return null;
    setBusy(true);
    setError(null);
    try {
      const session = await controlPlaneApi.createSession(currentProject.id, sessionTitle);
      setCurrentSession(session);
      setSessions((previous) => [session, ...previous.filter((item) => item.id !== session.id)]);
      return session;
    } catch (err) {
      setError(errorMessage(err));
      return null;
    } finally {
      setBusy(false);
    }
  };

  const submitRequest = async (event: FormEvent) => {
    event.preventDefault();
    if (!currentProject || !requestText.trim()) return;
    setBusy(true);
    setError(null);
    setEvents([]);
    setResult(null);
    setImplementationResult(null);
    setImplementationEvents([]);
    lastEventIdRef.current = undefined;
    lastImplementationEventIdRef.current = undefined;

    try {
      const session = currentSession ?? (await createSession());
      if (!session) return;
      const queued = await controlPlaneApi.createWorkPackageRequest(
        currentProject.id,
        session.id,
        requestText.trim()
      );
      setAgentRun({
        id: queued.agent_run_id,
        project_id: queued.project_id,
        session_id: queued.session_id,
        user_request: requestText.trim(),
        status: queued.status,
        intent: null,
        current_phase: null,
        trace_id: null,
        external_trace_id: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString()
      });
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const resolveApproval = async (status: "approve" | "reject") => {
    if (!result?.approval) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.resolveApproval(result.approval.id, status);
      const nextResult = await controlPlaneApi.getAgentRunResult(result.agent_run.id);
      const nextEvents = await controlPlaneApi.listEvents(result.agent_run.id);
      setResult(nextResult);
      setEvents(nextEvents);
      if (status === "reject") {
        setImplementationResult(null);
        setImplementationEvents([]);
      }
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const startImplementation = async () => {
    const workPackage = result?.work_package;
    if (!workPackage || workPackage.status !== "approved") return;
    setBusy(true);
    setError(null);
    setImplementationEvents([]);
    lastImplementationEventIdRef.current = undefined;
    try {
      const nextResult = await controlPlaneApi.createImplementationRun(workPackage.id);
      setImplementationResult(nextResult);
      setImplementationEvents(nextResult.events);
      lastImplementationEventIdRef.current =
        nextResult.events[nextResult.events.length - 1]?.id ?? undefined;
      setActiveTab("implementation");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const resolvePatchSet = async (status: "approve" | "reject") => {
    const patchSet = implementationResult?.patch_set;
    if (!patchSet) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.resolvePatchSet(patchSet.id, status);
      const nextResult = await controlPlaneApi.getImplementationRunResult(patchSet.implementation_run_id);
      const nextEvents = await controlPlaneApi.listImplementationEvents(patchSet.implementation_run_id);
      setImplementationResult(nextResult);
      setImplementationEvents(nextEvents);
      lastImplementationEventIdRef.current = nextEvents[nextEvents.length - 1]?.id;
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const applyPatchSet = async () => {
    const patchSet = implementationResult?.patch_set;
    if (!patchSet) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.applyPatchSet(patchSet.id);
      const nextResult = await controlPlaneApi.getImplementationRunResult(patchSet.implementation_run_id);
      const nextEvents = await controlPlaneApi.listImplementationEvents(patchSet.implementation_run_id);
      setImplementationResult(nextResult);
      setImplementationEvents(nextEvents);
      lastImplementationEventIdRef.current = nextEvents[nextEvents.length - 1]?.id;
      setActiveTab("implementation");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const startBrainstorming = async () => {
    if (!currentProject || !brainstormingTopic.trim()) return;
    setBusy(true);
    setError(null);
    setBrainstormingEvents([]);
    setBrainstormingResult(null);
    lastBrainstormingEventIdRef.current = undefined;
    try {
      const session = currentSession ?? (await createSession());
      if (!session) return;
      const sourceId =
        brainstormingSourceType === "work_package" ? result?.work_package?.id ?? null : null;
      const queued = await controlPlaneApi.createBrainstormingSession({
        projectId: currentProject.id,
        sessionId: session.id,
        topic: brainstormingTopic.trim(),
        mode: brainstormingMode,
        sourceType: sourceId ? "work_package" : "topic",
        sourceId,
        roles: selectedBrainstormingRoles
      });
      setBrainstormingResult({
        brainstorming_session: {
          id: queued.brainstorming_session_id,
          project_id: queued.project_id,
          session_id: queued.session_id,
          source_type: sourceId ? "work_package" : "topic",
          source_id: sourceId,
          topic: brainstormingTopic.trim(),
          mode: brainstormingMode,
          status: queued.status,
          current_phase: null,
          selected_roles: selectedBrainstormingRoles,
          trace_id: null,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        },
        contributions: [],
        critiques: [],
        options: [],
        decision_brief: null,
        decision_record: null,
        trace: null,
        events: [],
        artifacts: []
      });
      setActiveTab("brainstorming");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const resolveDecisionBrief = async (status: "accept" | "reject") => {
    const brief = brainstormingResult?.decision_brief;
    const brainstormingSessionId = brainstormingResult?.brainstorming_session.id;
    if (!brief || !brainstormingSessionId) return;
    setBusy(true);
    setError(null);
    try {
      const nextResult = await controlPlaneApi.resolveDecisionBrief(
        brainstormingSessionId,
        brief.id,
        status
      );
      const nextEvents = await controlPlaneApi.listBrainstormingEvents(brainstormingSessionId);
      setBrainstormingResult(nextResult);
      setBrainstormingEvents(nextEvents);
      lastBrainstormingEventIdRef.current = nextEvents[nextEvents.length - 1]?.id;
      setActiveTab("brainstorming");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const convertDecisionRecord = async () => {
    const record = brainstormingResult?.decision_record;
    if (!record) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.convertDecisionRecord(record.id);
      const nextResult = await controlPlaneApi.getBrainstormingResult(record.brainstorming_session_id);
      const nextEvents = await controlPlaneApi.listBrainstormingEvents(record.brainstorming_session_id);
      setBrainstormingResult(nextResult);
      setBrainstormingEvents(nextEvents);
      lastBrainstormingEventIdRef.current = nextEvents[nextEvents.length - 1]?.id;
      setActiveTab("brainstorming");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const createProjectRule = async () => {
    if (!currentProject) return;
    setBusy(true);
    setError(null);
    try {
      const item = await controlPlaneApi.createProjectRule({
        projectId: currentProject.id,
        sessionId: currentSession?.id,
        title: ruleDraft.title.trim(),
        summary: ruleDraft.summary.trim(),
        body: ruleDraft.body.trim(),
        tags: parseTags(ruleDraft.tags),
        importance: "high"
      });
      setMemoryDetail(item);
      setMemoryTab("rules");
      await refreshMemory();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const promoteCurrentDecision = async () => {
    const record = brainstormingResult?.decision_record;
    if (!record) return;
    setBusy(true);
    setError(null);
    try {
      const item = await controlPlaneApi.promoteDecisionRecord(record.id);
      setMemoryDetail(item);
      setMemoryTab("decisions");
      await refreshMemory();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const createSessionSummary = async () => {
    if (!currentSession) return;
    setBusy(true);
    setError(null);
    try {
      const result = await controlPlaneApi.createSessionMemorySummary(currentSession.id);
      if (result.memory_item) {
        setMemoryDetail(result.memory_item);
      }
      setMemoryTab("sessions");
      await refreshMemory();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const promoteLatestFailure = async () => {
    const review = implementationResult?.review_result;
    const failedVerification = implementationResult?.verification_runs.find((run) =>
      ["failed", "blocked"].includes(run.status)
    );
    if (!review && !failedVerification) return;
    setBusy(true);
    setError(null);
    try {
      const item =
        review && ["needs_changes", "blocked"].includes(review.status)
          ? await controlPlaneApi.promoteReviewFailureMemory(review.id)
          : failedVerification
            ? await controlPlaneApi.promoteVerificationFailureMemory(failedVerification.id)
            : null;
      if (item) {
        setMemoryDetail(item);
        setMemoryTab("failures");
      }
      await refreshMemory();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const archiveOrRestoreMemory = async (item: ProjectMemoryItem) => {
    setBusy(true);
    setError(null);
    try {
      const next =
        item.status === "active"
          ? await controlPlaneApi.archiveMemoryItem(item.id)
          : await controlPlaneApi.restoreMemoryItem(item.id);
      setMemoryDetail(next);
      await refreshMemory();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const selectMemory = async (item: ProjectMemoryItem) => {
    if (!currentSession) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.selectMemory(currentSession.id, item.id);
      const selected = await controlPlaneApi.listSelectedMemory(currentSession.id);
      setSelectedMemory(selected);
      setMemoryTab("selected");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const unselectMemory = async (memoryItemId: string) => {
    if (!currentSession) return;
    setBusy(true);
    setError(null);
    try {
      await controlPlaneApi.unselectMemory(currentSession.id, memoryItemId);
      const selected = await controlPlaneApi.listSelectedMemory(currentSession.id);
      setSelectedMemory(selected);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const startRiskScan = async () => {
    if (!currentProject) return;
    setBusy(true);
    setError(null);
    setRiskEvents([]);
    setRiskScanResult(null);
    setRiskDetail(null);
    lastRiskEventIdRef.current = undefined;
    try {
      const session = currentSession ?? (await createSession());
      if (!session) return;
      const selectedIds =
        includeRiskMemory && selectedMemory
          ? selectedMemory.selected_memory.map((item) => item.memory_item_id)
          : [];
      const queued = await controlPlaneApi.createRiskScan({
        projectId: currentProject.id,
        sessionId: session.id,
        scopeType: riskScopeType,
        includeSelectedMemory: includeRiskMemory,
        selectedMemoryIds: selectedIds,
        focus: ["verification", "architecture", "process"]
      });
      setRiskScanResult({
        risk_scan_run: {
          id: queued.risk_scan_run_id,
          project_id: queued.project_id,
          session_id: queued.session_id,
          scope_type: riskScopeType,
          scope_id: null,
          status: queued.status,
          current_phase: null,
          selected_memory_count: selectedIds.length,
          trace_id: null,
          source_context: {},
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString()
        },
        findings: [],
        quality_signals: [],
        project_health_snapshot: null,
        architecture_map_snapshot: null,
        trace: null,
        events: [],
        artifacts: []
      });
      setActiveTab("risk");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const updateRiskFinding = async (finding: RiskFinding, status: Exclude<RiskFindingStatus, "converted">) => {
    setBusy(true);
    setError(null);
    try {
      const updated = await controlPlaneApi.updateRiskFindingStatus(finding.id, status);
      setRiskDetail(updated);
      if (riskScanResult) {
        const nextResult = await controlPlaneApi.getRiskScanResult(riskScanResult.risk_scan_run.id);
        setRiskScanResult(nextResult);
        setRiskEvents(nextResult.events);
      }
      await refreshRisk();
      setActiveTab("risk");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const convertRiskFinding = async (finding: RiskFinding) => {
    setBusy(true);
    setError(null);
    try {
      const converted = await controlPlaneApi.convertRiskFinding(finding.id);
      setRiskDetail(converted.risk_finding);
      if (riskScanResult) {
        const nextResult = await controlPlaneApi.getRiskScanResult(riskScanResult.risk_scan_run.id);
        setRiskScanResult(nextResult);
        setRiskEvents(nextResult.events);
      }
      await refreshRisk();
      setActiveTab("risk");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const toggleBrainstormingRole = (role: string) => {
    setSelectedBrainstormingRoles((previous) => {
      if (previous.includes(role)) {
        return previous.filter((item) => item !== role);
      }
      if (previous.length >= 6) return previous;
      return [...previous, role];
    });
  };

  const projectOptions = useMemo(
    () =>
      projects.map((project) => (
        <button
          className={`list-row ${currentProject?.id === project.id ? "selected" : ""}`}
          key={project.id}
          onClick={() => setCurrentProject(project)}
          title={project.root_path}
          type="button"
        >
          <span>{project.name}</span>
          <small>{project.status}</small>
        </button>
      )),
    [currentProject?.id, projects]
  );

  return (
    <div className="app-shell">
      <header className="top-bar">
        <div className="brand-block">
          <CircleDot size={18} />
          <div>
            <strong>Artemis</strong>
            <span>Project Command Center</span>
          </div>
        </div>
        <div className="status-strip">
          <StatusBadge status={backendStatus} />
          <span>{currentProject?.name ?? "No project"}</span>
          <span>{currentSession?.title ?? "No session"}</span>
          <IconButton title="Refresh backend" onClick={() => void refreshBackend()}>
            <RefreshCw size={16} />
          </IconButton>
        </div>
      </header>

      <aside className="sidebar">
        <section className="panel">
          <div className="panel-title">
            <FolderOpen size={16} />
            <span>Project</span>
          </div>
          <form className="stack" onSubmit={openProject}>
            <label>
              <span>Name</span>
              <input value={projectName} onChange={(event) => setProjectName(event.target.value)} />
            </label>
            <label>
              <span>Root Path</span>
              <input value={projectRoot} onChange={(event) => setProjectRoot(event.target.value)} />
            </label>
            <button className="command-button" disabled={busy || backendStatus !== "online"} type="submit">
              <FolderOpen size={16} />
              Open
            </button>
          </form>
          <div className="compact-list">{projectOptions}</div>
        </section>

        <section className="panel">
          <div className="panel-title">
            <FileText size={16} />
            <span>Session</span>
          </div>
          <form className="stack" onSubmit={createSession}>
            <label>
              <span>Title</span>
              <input value={sessionTitle} onChange={(event) => setSessionTitle(event.target.value)} />
            </label>
            <button className="command-button" disabled={busy || !currentProject} type="submit">
              <Play size={16} />
              Create
            </button>
          </form>
          <div className="compact-list">
            {sessions.map((session) => (
              <button
                className={`list-row ${currentSession?.id === session.id ? "selected" : ""}`}
                key={session.id}
                onClick={() => setCurrentSession(session)}
                type="button"
              >
                <span>{session.title}</span>
                <small>{session.status}</small>
              </button>
            ))}
          </div>
        </section>
      </aside>

      <main className="workspace">
        <section className="request-panel">
          <div className="section-heading">
            <div>
              <span>Work Package Request</span>
              <strong>{statusLabel(currentStatus)}</strong>
            </div>
            <RunMeter status={currentStatus} />
          </div>
          <form onSubmit={submitRequest}>
            <textarea
              value={requestText}
              onChange={(event) => setRequestText(event.target.value)}
              placeholder="Describe the work package."
            />
            <div className="action-row">
              <button
                className="primary-button"
                disabled={busy || !currentProject || !requestText.trim()}
                type="submit"
              >
                <Send size={16} />
                Submit
              </button>
              {agentRun && (
                <span className="run-id">
                  {agentRun.id} · {agentRun.current_phase ?? agentRun.status}
                </span>
              )}
            </div>
          </form>
          {error && (
            <div className="error-banner">
              <CircleAlert size={16} />
              <span>{error}</span>
            </div>
          )}
        </section>

        <section className="detail-grid">
          <WorkPackagePanel result={result} />
          <ApprovalPanel result={result} busy={busy} onResolve={resolveApproval} />
        </section>

        <BrainstormingPanel
          busy={busy}
          mode={brainstormingMode}
          result={brainstormingResult}
          roles={selectedBrainstormingRoles}
          sourceType={brainstormingSourceType}
          topic={brainstormingTopic}
          workPackage={result?.work_package ?? null}
          onConvert={convertDecisionRecord}
          onModeChange={setBrainstormingMode}
          onResolveDecision={resolveDecisionBrief}
          onSourceTypeChange={setBrainstormingSourceType}
          onStart={startBrainstorming}
          onToggleRole={toggleBrainstormingRole}
          onTopicChange={setBrainstormingTopic}
        />

        <ImplementationPanel
          busy={busy}
          result={implementationResult}
          workPackage={result?.work_package ?? null}
          onApplyPatch={applyPatchSet}
          onResolvePatch={resolvePatchSet}
          onStart={startImplementation}
        />

        <MemoryPanel
          busy={busy}
          currentSession={currentSession}
          detail={memoryDetail}
          items={memoryItems}
          query={memoryQuery}
          result={implementationResult}
          ruleDraft={ruleDraft}
          searchResults={memorySearchResults}
          selected={selectedMemory}
          tab={memoryTab}
          typeFilter={memoryTypeFilter}
          statusFilter={memoryStatusFilter}
          decisionRecord={brainstormingResult?.decision_record ?? null}
          onArchiveRestore={archiveOrRestoreMemory}
          onCreateProjectRule={createProjectRule}
          onCreateSessionSummary={createSessionSummary}
          onPromoteDecision={promoteCurrentDecision}
          onPromoteFailure={promoteLatestFailure}
          onQueryChange={setMemoryQuery}
          onRefresh={refreshMemory}
          onRuleDraftChange={setRuleDraft}
          onSelect={selectMemory}
          onSelectDetail={setMemoryDetail}
          onStatusFilterChange={setMemoryStatusFilter}
          onTabChange={setMemoryTab}
          onTypeFilterChange={setMemoryTypeFilter}
          onUnselect={unselectMemory}
        />

        <RiskQualityPanel
          busy={busy}
          detail={riskDetail}
          includeSelectedMemory={includeRiskMemory}
          quality={qualitySnapshot}
          radar={riskRadar}
          result={riskScanResult}
          scopeType={riskScopeType}
          selectedMemory={selectedMemory}
          onConvert={convertRiskFinding}
          onDetailChange={setRiskDetail}
          onIncludeSelectedMemoryChange={setIncludeRiskMemory}
          onRefresh={refreshRisk}
          onScopeTypeChange={setRiskScopeType}
          onStart={startRiskScan}
          onUpdateFinding={updateRiskFinding}
        />
      </main>

      <aside className="activity-panel">
        <div className="activity-tabs">
          <TabButton active={activeTab === "timeline"} onClick={() => setActiveTab("timeline")}>
            <Activity size={15} />
            Timeline
          </TabButton>
          <TabButton active={activeTab === "trace"} onClick={() => setActiveTab("trace")}>
            <Server size={15} />
            Trace
          </TabButton>
          <TabButton active={activeTab === "artifacts"} onClick={() => setActiveTab("artifacts")}>
            <FileText size={15} />
            Artifacts
          </TabButton>
          <TabButton
            active={activeTab === "implementation"}
            onClick={() => setActiveTab("implementation")}
          >
            <GitPullRequest size={15} />
            Impl
          </TabButton>
          <TabButton
            active={activeTab === "brainstorming"}
            onClick={() => setActiveTab("brainstorming")}
          >
            <Activity size={15} />
            Brain
          </TabButton>
          <TabButton active={activeTab === "memory"} onClick={() => setActiveTab("memory")}>
            <Database size={15} />
            Memory
          </TabButton>
          <TabButton active={activeTab === "risk"} onClick={() => setActiveTab("risk")}>
            <ShieldCheck size={15} />
            Risk
          </TabButton>
        </div>

        {activeTab === "timeline" && <Timeline events={events} />}
        {activeTab === "trace" && <TracePanel trace={selectedTrace} />}
        {activeTab === "artifacts" && <ArtifactsPanel artifacts={selectedArtifacts} />}
        {activeTab === "implementation" && <Timeline events={implementationEvents} />}
        {activeTab === "brainstorming" && <Timeline events={brainstormingEvents} />}
        {activeTab === "memory" && <Timeline events={memoryEvents} />}
        {activeTab === "risk" && <Timeline events={riskEvents} />}
      </aside>
    </div>
  );
}

function WorkPackagePanel({ result }: { result: AgentRunResult | null }) {
  const workPackage = result?.work_package;
  if (!workPackage) {
    return (
      <section className="panel empty-panel">
        <FileText size={18} />
        <span>No work package</span>
      </section>
    );
  }

  return (
    <section className="panel work-package-panel">
      <div className="panel-title">
        <FileText size={16} />
        <span>Work Package</span>
        <StatusPill label={workPackage.status} />
      </div>
      <h1>{workPackage.title}</h1>
      <p>{workPackage.goal}</p>
      <FieldGroup title="Scope" values={workPackage.scope} />
      <FieldGroup title="Steps" values={workPackage.implementation_steps} />
      <FieldGroup title="Verification" values={workPackage.verification} />
      <FieldGroup title="Related Files" values={workPackage.related_files} />
      <div className="risk-row">
        {workPackage.risks.map((risk) => (
          <span className={`risk-pill ${risk.level}`} key={`${risk.level}-${risk.description}`}>
            {risk.level}: {risk.description}
          </span>
        ))}
      </div>
    </section>
  );
}

function ApprovalPanel({
  result,
  busy,
  onResolve
}: {
  result: AgentRunResult | null;
  busy: boolean;
  onResolve: (status: "approve" | "reject") => void;
}) {
  const approval = result?.approval;
  return (
    <section className="panel approval-panel">
      <div className="panel-title">
        <Check size={16} />
        <span>Approval</span>
        {approval && <StatusPill label={approval.status} />}
      </div>
      {approval ? (
        <>
          <p>{approval.reason}</p>
          <div className="action-row">
            <button
              className="approve-button"
              disabled={busy || approval.status !== "pending"}
              onClick={() => onResolve("approve")}
              title="Approve"
              type="button"
            >
              <Check size={16} />
              Approve
            </button>
            <button
              className="reject-button"
              disabled={busy || approval.status !== "pending"}
              onClick={() => onResolve("reject")}
              title="Reject"
              type="button"
            >
              <X size={16} />
              Reject
            </button>
          </div>
        </>
      ) : (
        <span className="muted">No approval request</span>
      )}
    </section>
  );
}

function RiskQualityPanel({
  busy,
  detail,
  includeSelectedMemory,
  quality,
  radar,
  result,
  scopeType,
  selectedMemory,
  onConvert,
  onDetailChange,
  onIncludeSelectedMemoryChange,
  onRefresh,
  onScopeTypeChange,
  onStart,
  onUpdateFinding
}: {
  busy: boolean;
  detail: RiskFinding | null;
  includeSelectedMemory: boolean;
  quality: QualitySnapshot | null;
  radar: RiskRadar | null;
  result: RiskScanResult | null;
  scopeType: "project" | "session";
  selectedMemory: SelectedMemoryContext | null;
  onConvert: (finding: RiskFinding) => void;
  onDetailChange: (finding: RiskFinding) => void;
  onIncludeSelectedMemoryChange: (value: boolean) => void;
  onRefresh: () => void;
  onScopeTypeChange: (scope: "project" | "session") => void;
  onStart: () => void;
  onUpdateFinding: (
    finding: RiskFinding,
    status: Exclude<RiskFindingStatus, "converted">
  ) => void;
}) {
  const health = result?.project_health_snapshot ?? radar?.health ?? quality?.health ?? null;
  const findings = result?.findings.length ? result.findings : radar?.findings ?? [];
  const signals = result?.quality_signals.length ? result.quality_signals : quality?.signals ?? [];
  const architecture = result?.architecture_map_snapshot ?? quality?.architecture_map ?? null;
  const selectedCount = selectedMemory?.source_context.length ?? 0;
  const canConvert = detail?.status === "accepted" && !detail.converted_work_package_id;

  return (
    <section className="panel risk-panel">
      <div className="panel-title">
        <ShieldCheck size={16} />
        <span>Risk Radar / Quality Center</span>
        {result?.risk_scan_run && <StatusPill label={result.risk_scan_run.status} />}
      </div>

      <div className="risk-toolbar">
        <label>
          <span>Scope</span>
          <select
            value={scopeType}
            onChange={(event) => onScopeTypeChange(event.target.value as "project" | "session")}
          >
            <option value="project">project</option>
            <option value="session">session</option>
          </select>
        </label>
        <label className="check-chip risk-memory-toggle">
          <input
            checked={includeSelectedMemory}
            onChange={(event) => onIncludeSelectedMemoryChange(event.target.checked)}
            type="checkbox"
          />
          <span>Include selected memory ({selectedCount})</span>
        </label>
        <button className="primary-button" disabled={busy} onClick={onStart} type="button">
          <Play size={16} />
          Start Risk Scan
        </button>
        <button className="command-button" disabled={busy} onClick={onRefresh} type="button">
          <RefreshCw size={16} />
          Refresh
        </button>
      </div>

      <div className="risk-health-grid">
        <article className="health-summary">
          <span>Project Health</span>
          <strong>{health ? Math.round(health.overall_score) : "--"}</strong>
          {health && <StatusPill label={health.overall_status} />}
          <p>{health?.recommendation ?? "No risk scan recorded"}</p>
        </article>
        <article className="health-summary">
          <span>Severity Distribution</span>
          <div className="metric-row">
            {Object.entries(radar?.severity_counts ?? health?.risk_counts ?? {}).map(([key, value]) => (
              <span key={key}>
                {key}: {value}
              </span>
            ))}
          </div>
        </article>
        <article className="health-summary">
          <span>Selected Context</span>
          <strong>{result?.risk_scan_run.selected_memory_count ?? selectedCount}</strong>
          <p>
            {selectedMemory?.source_context.map((item) => item.title).join(", ") || "No selected memory"}
          </p>
        </article>
      </div>

      <div className="risk-layout">
        <div className="risk-list">
          <div className="subsection-heading">
            <h2>Risk Findings</h2>
            <span>{findings.length} total</span>
          </div>
          {findings.length ? (
            findings.map((finding) => (
              <button
                className={`risk-row-card ${detail?.id === finding.id ? "selected" : ""}`}
                key={finding.id}
                onClick={() => onDetailChange(finding)}
                type="button"
              >
                <div>
                  <strong>{finding.title}</strong>
                  <span>{finding.summary}</span>
                </div>
                <span className={`risk-pill ${finding.severity}`}>{finding.severity}</span>
                <StatusPill label={finding.status} />
              </button>
            ))
          ) : (
            <EmptyActivity label="No risk findings" />
          )}
        </div>

        <div className="risk-detail">
          {detail ? (
            <>
              <div className="subsection-heading">
                <h2>{detail.title}</h2>
                <StatusPill label={detail.category} />
              </div>
              <p>{detail.summary}</p>
              <p>{detail.recommendation}</p>
              <FieldGroup title="Evidence" values={detail.evidence} />
              <FieldGroup
                title="Source Links"
                values={detail.source_links.map(
                  (link) =>
                    `${link.source_type}:${link.source_id} (${link.relation})${
                      link.label ? ` - ${link.label}` : ""
                    }`
                )}
              />
              <div className="action-row">
                <button
                  className="approve-button"
                  disabled={busy || detail.status === "accepted" || detail.status === "converted"}
                  onClick={() => onUpdateFinding(detail, "accepted")}
                  type="button"
                >
                  <Check size={16} />
                  Accept Finding
                </button>
                <button
                  className="reject-button"
                  disabled={busy || detail.status === "dismissed" || detail.status === "converted"}
                  onClick={() => onUpdateFinding(detail, "dismissed")}
                  type="button"
                >
                  <X size={16} />
                  Dismiss
                </button>
                <button
                  className="command-button"
                  disabled={busy || detail.status === "mitigated" || detail.status === "converted"}
                  onClick={() => onUpdateFinding(detail, "mitigated")}
                  type="button"
                >
                  <Check size={16} />
                  Mitigate
                </button>
                <button
                  className="command-button"
                  disabled={busy || !canConvert}
                  onClick={() => onConvert(detail)}
                  type="button"
                >
                  <FileText size={16} />
                  Convert to Work Package
                </button>
              </div>
              {detail.converted_work_package_id && (
                <span className="run-id">Work Package {detail.converted_work_package_id}</span>
              )}
            </>
          ) : (
            <EmptyActivity label="No finding detail" />
          )}
        </div>
      </div>

      <div className="quality-grid">
        <div className="quality-section">
          <div className="subsection-heading">
            <h2>Quality Center</h2>
            <span>{signals.length} signals</span>
          </div>
          <div className="quality-signal-list">
            {signals.map((signal) => (
              <article className="quality-signal" key={signal.id}>
                <div>
                  <strong>{signal.title}</strong>
                  <StatusPill label={signal.status} />
                </div>
                <p>{signal.summary}</p>
                <code>{formatUnknown(signal.value)}</code>
              </article>
            ))}
            {!signals.length && <EmptyActivity label="No quality signals" />}
          </div>
        </div>

        <div className="quality-section">
          <div className="subsection-heading">
            <h2>Architecture Map Lite</h2>
            <MapIcon size={15} />
          </div>
          {architecture ? (
            <div className="architecture-list">
              <FieldGroup
                title="Nodes"
                values={architecture.nodes.map(
                  (node) => `${String(node.label ?? node.id)} (${String(node.file_count ?? 0)})`
                )}
              />
              <FieldGroup
                title="Edges"
                values={architecture.edges.map(
                  (edge) => `${String(edge.from)} -> ${String(edge.to)} (${String(edge.relation)})`
                )}
              />
              <FieldGroup title="Boundary Notes" values={architecture.boundary_notes} />
            </div>
          ) : (
            <EmptyActivity label="No architecture map" />
          )}
        </div>
      </div>
    </section>
  );
}

function MemoryPanel({
  busy,
  currentSession,
  decisionRecord,
  detail,
  items,
  query,
  result,
  ruleDraft,
  searchResults,
  selected,
  statusFilter,
  tab,
  typeFilter,
  onArchiveRestore,
  onCreateProjectRule,
  onCreateSessionSummary,
  onPromoteDecision,
  onPromoteFailure,
  onQueryChange,
  onRefresh,
  onRuleDraftChange,
  onSelect,
  onSelectDetail,
  onStatusFilterChange,
  onTabChange,
  onTypeFilterChange,
  onUnselect
}: {
  busy: boolean;
  currentSession: Session | null;
  decisionRecord: DecisionRecord | null;
  detail: ProjectMemoryItem | null;
  items: ProjectMemoryItem[];
  query: string;
  result: ImplementationRunResult | null;
  ruleDraft: { title: string; summary: string; body: string; tags: string };
  searchResults: MemorySearchResult[];
  selected: SelectedMemoryContext | null;
  statusFilter: string;
  tab: "search" | "decisions" | "rules" | "failures" | "sessions" | "selected";
  typeFilter: MemoryType | "";
  onArchiveRestore: (item: ProjectMemoryItem) => void;
  onCreateProjectRule: () => void;
  onCreateSessionSummary: () => void;
  onPromoteDecision: () => void;
  onPromoteFailure: () => void;
  onQueryChange: (query: string) => void;
  onRefresh: () => Promise<void>;
  onRuleDraftChange: (draft: { title: string; summary: string; body: string; tags: string }) => void;
  onSelect: (item: ProjectMemoryItem) => void;
  onSelectDetail: (item: ProjectMemoryItem) => void;
  onStatusFilterChange: (status: string) => void;
  onTabChange: (tab: "search" | "decisions" | "rules" | "failures" | "sessions" | "selected") => void;
  onTypeFilterChange: (type: MemoryType | "") => void;
  onUnselect: (memoryItemId: string) => void;
}) {
  const lists = {
    decisions: items.filter((item) => item.type === "decision"),
    rules: items.filter((item) => item.type === "project_rule"),
    failures: items.filter((item) => item.type === "failure"),
    sessions: items.filter((item) => item.type === "session_summary")
  };
  const failureAvailable =
    !!result?.review_result && ["needs_changes", "blocked"].includes(result.review_result.status)
    || !!result?.verification_runs.some((run) => ["failed", "blocked"].includes(run.status));
  const visibleItems =
    tab === "search"
      ? searchResults.map((searchResult) => searchResult.item)
      : tab === "selected"
        ? selected?.source_context
            .map((snapshot) => items.find((item) => item.id === snapshot.id))
            .filter((item): item is ProjectMemoryItem => Boolean(item)) ?? []
        : lists[tab];

  return (
    <section className="panel memory-panel">
      <div className="panel-title">
        <Database size={16} />
        <span>Memory</span>
        <StatusPill label={`${items.length} active`} />
      </div>

      <div className="memory-toolbar">
        <label className="wide-field">
          <span>Search</span>
          <input
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="Search memory"
          />
        </label>
        <label>
          <span>Type</span>
          <select
            value={typeFilter}
            onChange={(event) => onTypeFilterChange(event.target.value as MemoryType | "")}
          >
            <option value="">all</option>
            <option value="decision">decision</option>
            <option value="project_rule">project_rule</option>
            <option value="failure">failure</option>
            <option value="session_summary">session_summary</option>
          </select>
        </label>
        <label>
          <span>Status</span>
          <select value={statusFilter} onChange={(event) => onStatusFilterChange(event.target.value)}>
            <option value="active">active</option>
            <option value="archived">archived</option>
            <option value="">all</option>
          </select>
        </label>
        <button className="command-button" disabled={busy} onClick={() => void onRefresh()} type="button">
          <Search size={16} />
          Search
        </button>
      </div>

      <div className="memory-tabs">
        {(["search", "decisions", "rules", "failures", "sessions", "selected"] as const).map(
          (nextTab) => (
            <button
              className={`tab-button ${tab === nextTab ? "active" : ""}`}
              key={nextTab}
              onClick={() => onTabChange(nextTab)}
              type="button"
            >
              {nextTab}
            </button>
          )
        )}
      </div>

      <div className="memory-actions">
        <button className="command-button" disabled={busy || !decisionRecord} onClick={onPromoteDecision} type="button">
          <Database size={16} />
          Promote Decision
        </button>
        <button className="command-button" disabled={busy || !currentSession} onClick={onCreateSessionSummary} type="button">
          <FileText size={16} />
          Summarize Session
        </button>
        <button className="command-button" disabled={busy || !failureAvailable} onClick={onPromoteFailure} type="button">
          <CircleAlert size={16} />
          Capture Failure
        </button>
      </div>

      {tab === "rules" && (
        <div className="project-rule-form">
          <label>
            <span>Rule Title</span>
            <input
              value={ruleDraft.title}
              onChange={(event) => onRuleDraftChange({ ...ruleDraft, title: event.target.value })}
            />
          </label>
          <label>
            <span>Summary</span>
            <input
              value={ruleDraft.summary}
              onChange={(event) => onRuleDraftChange({ ...ruleDraft, summary: event.target.value })}
            />
          </label>
          <label>
            <span>Rule Body</span>
            <textarea
              value={ruleDraft.body}
              onChange={(event) => onRuleDraftChange({ ...ruleDraft, body: event.target.value })}
            />
          </label>
          <label>
            <span>Tags</span>
            <input
              value={ruleDraft.tags}
              onChange={(event) => onRuleDraftChange({ ...ruleDraft, tags: event.target.value })}
            />
          </label>
          <button
            className="primary-button"
            disabled={busy || !ruleDraft.title.trim() || !ruleDraft.body.trim()}
            onClick={onCreateProjectRule}
            type="button"
          >
            <Check size={16} />
            Create Rule
          </button>
        </div>
      )}

      <div className="memory-layout">
        <div className="memory-list">
          {visibleItems.length ? (
            visibleItems.map((item) => (
              <button
                className={`memory-row ${detail?.id === item.id ? "selected" : ""}`}
                key={item.id}
                onClick={() => onSelectDetail(item)}
                type="button"
              >
                <div>
                  <strong>{item.title}</strong>
                  <span>{item.summary}</span>
                </div>
                <StatusPill label={item.type} />
              </button>
            ))
          ) : (
            <EmptyActivity label="No memory items" />
          )}
        </div>

        <div className="memory-detail">
          {detail ? (
            <>
              <div className="subsection-heading">
                <h2>{detail.title}</h2>
                <StatusPill label={detail.status} />
              </div>
              <p>{detail.summary}</p>
              <pre>{detail.body}</pre>
              <div className="tag-row">
                {detail.tags.map((tag) => (
                  <span key={tag}>{tag}</span>
                ))}
              </div>
              <FieldGroup
                title="Source Links"
                values={detail.source_links.map(
                  (link) => `${link.source_type}:${link.source_id} (${link.relation})`
                )}
              />
              <div className="action-row">
                <button
                  className="command-button"
                  disabled={busy || detail.status !== "active" || !currentSession}
                  onClick={() => onSelect(detail)}
                  type="button"
                >
                  <Check size={16} />
                  Select Context
                </button>
                <button className="command-button" disabled={busy} onClick={() => onArchiveRestore(detail)} type="button">
                  {detail.status === "active" ? <Archive size={16} /> : <RotateCcw size={16} />}
                  {detail.status === "active" ? "Archive" : "Restore"}
                </button>
              </div>
            </>
          ) : (
            <EmptyActivity label="No memory detail" />
          )}
        </div>
      </div>

      <div className="selected-memory-strip">
        <strong>Selected Context</strong>
        {selected?.source_context.length ? (
          selected.source_context.map((snapshot) => (
            <span key={snapshot.id}>
              {snapshot.title}
              <button onClick={() => onUnselect(snapshot.id)} title="Remove" type="button">
                <X size={13} />
              </button>
            </span>
          ))
        ) : (
          <small>No selected memory</small>
        )}
      </div>
    </section>
  );
}

function BrainstormingPanel({
  busy,
  mode,
  result,
  roles,
  sourceType,
  topic,
  workPackage,
  onConvert,
  onModeChange,
  onResolveDecision,
  onSourceTypeChange,
  onStart,
  onToggleRole,
  onTopicChange
}: {
  busy: boolean;
  mode: BrainstormingMode;
  result: BrainstormingResult | null;
  roles: string[];
  sourceType: BrainstormingSourceType;
  topic: string;
  workPackage: WorkPackage | null;
  onConvert: () => void;
  onModeChange: (mode: BrainstormingMode) => void;
  onResolveDecision: (status: "accept" | "reject") => void;
  onSourceTypeChange: (sourceType: BrainstormingSourceType) => void;
  onStart: () => void;
  onToggleRole: (role: string) => void;
  onTopicChange: (topic: string) => void;
}) {
  const session = result?.brainstorming_session;
  const brief = result?.decision_brief;
  const record = result?.decision_record;
  const canUseWorkPackage = !!workPackage;
  const sourceReady = sourceType === "topic" || canUseWorkPackage;
  const canConvert = !!record && !record.linked_work_package_id;

  return (
    <section className="panel brainstorming-panel">
      <div className="panel-title">
        <Activity size={16} />
        <span>Brainstorming Room</span>
        {session && <StatusPill label={session.status} />}
      </div>

      <div className="brainstorming-compose">
        <label className="wide-field">
          <span>Topic</span>
          <textarea
            value={topic}
            onChange={(event) => onTopicChange(event.target.value)}
            placeholder="Frame the decision or design topic."
          />
        </label>
        <label>
          <span>Mode</span>
          <select
            value={mode}
            onChange={(event) => onModeChange(event.target.value as BrainstormingMode)}
          >
            <option value="architecture_debate">architecture_debate</option>
            <option value="implementation_strategy">implementation_strategy</option>
            <option value="risk_review">risk_review</option>
            <option value="product_planning">product_planning</option>
            <option value="free_ideation">free_ideation</option>
          </select>
        </label>
        <label>
          <span>Source</span>
          <select
            value={sourceType}
            onChange={(event) => onSourceTypeChange(event.target.value as BrainstormingSourceType)}
          >
            <option value="topic">topic</option>
            <option value="work_package" disabled={!canUseWorkPackage}>
              current_work_package
            </option>
          </select>
        </label>
      </div>

      <div className="role-selector">
        {BRAINSTORMING_ROLES.map((role) => (
          <label className="check-chip" key={role}>
            <input
              checked={roles.includes(role)}
              onChange={() => onToggleRole(role)}
              type="checkbox"
            />
            <span>{role}</span>
          </label>
        ))}
      </div>

      <div className="action-row">
        <button
          className="primary-button"
          disabled={busy || !topic.trim() || !sourceReady || roles.length === 0}
          onClick={onStart}
          type="button"
        >
          <Play size={16} />
          Start Brainstorming
        </button>
        {session && <span className="run-id">{session.id} - {session.current_phase ?? session.status}</span>}
      </div>

      {result && (
        <div className="brainstorming-grid">
          <div className="brainstorming-section">
            <div className="subsection-heading">
              <h2>Decision Brief</h2>
              {brief && <StatusPill label={brief.status} />}
            </div>
            {brief ? (
              <>
                <p>{brief.recommendation}</p>
                <p>{brief.rationale}</p>
                <FieldGroup title="Tradeoffs" values={brief.tradeoffs} />
                <FieldGroup title="Risks" values={brief.risks} />
                <FieldGroup title="Follow Up" values={brief.follow_up_actions} />
                <div className="action-row">
                  <button
                    className="approve-button"
                    disabled={busy || brief.status !== "pending"}
                    onClick={() => onResolveDecision("accept")}
                    type="button"
                  >
                    <Check size={16} />
                    Accept Decision
                  </button>
                  <button
                    className="reject-button"
                    disabled={busy || brief.status !== "pending"}
                    onClick={() => onResolveDecision("reject")}
                    type="button"
                  >
                    <X size={16} />
                    Reject Decision
                  </button>
                  <button
                    className="command-button"
                    disabled={busy || !canConvert}
                    onClick={onConvert}
                    type="button"
                  >
                    <FileText size={16} />
                    Convert to Work Package
                  </button>
                </div>
              </>
            ) : (
              <span className="muted">No DecisionBrief yet</span>
            )}
          </div>

          <div className="brainstorming-section">
            <h2>Options</h2>
            <div className="option-list">
              {result.options.map((option) => (
                <article
                  className={`option-card ${
                    brief?.selected_option_id === option.id ? "selected" : ""
                  }`}
                  key={option.id}
                >
                  <div>
                    <strong>{option.title}</strong>
                    <span>{Math.round(option.score * 100)}%</span>
                  </div>
                  <p>{option.summary}</p>
                  <FieldGroup title="Benefits" values={option.benefits} />
                  <FieldGroup title="Costs" values={option.costs} />
                </article>
              ))}
            </div>
          </div>

          <div className="brainstorming-section">
            <h2>Contributions</h2>
            <div className="contribution-list">
              {result.contributions.map((contribution) => (
                <article className="contribution-card" key={contribution.id}>
                  <div>
                    <strong>{contribution.role}</strong>
                    <StatusPill label={contribution.stance} />
                  </div>
                  <p>{contribution.summary}</p>
                  <FieldGroup title="Arguments" values={contribution.arguments} />
                  <FieldGroup title="Concerns" values={contribution.concerns} />
                </article>
              ))}
            </div>
          </div>

          <div className="brainstorming-section">
            <h2>Critiques</h2>
            <div className="contribution-list">
              {result.critiques.map((critique) => (
                <article className="contribution-card" key={critique.id}>
                  <div>
                    <strong>{critique.critic_role}</strong>
                    <span className="muted">{critique.target_role}</span>
                  </div>
                  <FieldGroup title="Weak Assumptions" values={critique.weak_assumptions} />
                  <FieldGroup title="Suggested Revisions" values={critique.suggested_revisions} />
                </article>
              ))}
            </div>
          </div>

          {record && (
            <div className="brainstorming-section decision-record-card">
              <div className="subsection-heading">
                <h2>Decision Record</h2>
                {record.linked_work_package_id && <StatusPill label="converted" />}
              </div>
              <p>{record.decision}</p>
              <FieldGroup title="Consequences" values={record.consequences} />
              <FieldGroup title="Follow Up" values={record.follow_up_actions} />
              {record.linked_work_package_id && (
                <span className="run-id">Work Package {record.linked_work_package_id}</span>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ImplementationPanel({
  busy,
  result,
  workPackage,
  onApplyPatch,
  onResolvePatch,
  onStart
}: {
  busy: boolean;
  result: ImplementationRunResult | null;
  workPackage: WorkPackage | null;
  onApplyPatch: () => void;
  onResolvePatch: (status: "approve" | "reject") => void;
  onStart: () => void;
}) {
  const plan = result?.implementation_plan;
  const patchSet = result?.patch_set;
  const canStart = workPackage?.status === "approved" && !result;
  const hasDelete = patchSet?.files.some((file) => file.operation === "delete") ?? false;

  return (
    <section className="panel implementation-panel">
      <div className="panel-title">
        <Code2 size={16} />
        <span>Implementation</span>
        {result && <StatusPill label={result.implementation_run.status} />}
      </div>
      <div className="implementation-toolbar">
        <button className="primary-button" disabled={busy || !canStart} onClick={onStart} type="button">
          <Play size={16} />
          Start
        </button>
        {result && <span className="run-id">{result.implementation_run.id}</span>}
      </div>

      {!result && <span className="muted">Approved Work Package required</span>}

      {plan && (
        <div className="implementation-section">
          <h2>Plan</h2>
          <p>{plan.goal}</p>
          <FieldGroup title="Target Files" values={plan.target_files} />
          <FieldGroup title="Steps" values={plan.steps} />
        </div>
      )}

      {patchSet && (
        <div className="implementation-section">
          <div className="subsection-heading">
            <h2>Patch</h2>
            <StatusPill label={patchSet.approval_status} />
          </div>
          <p>{patchSet.summary}</p>
          <div className="diff-list">
            {patchSet.files.map((file) => (
              <article className="diff-file" key={file.id}>
                <div className="diff-file-head">
                  <strong>{file.path}</strong>
                  <span>{file.operation}</span>
                  <span className={`risk-pill ${file.risk_level}`}>{file.risk_level}</span>
                </div>
                <p>{file.rationale}</p>
                <pre className="diff-block">{file.diff || "No diff"}</pre>
              </article>
            ))}
          </div>
          <div className="action-row">
            <button
              className="approve-button"
              disabled={busy || patchSet.approval_status !== "pending"}
              onClick={() => onResolvePatch("approve")}
              type="button"
            >
              <Check size={16} />
              Approve Patch
            </button>
            <button
              className="reject-button"
              disabled={busy || patchSet.approval_status !== "pending"}
              onClick={() => onResolvePatch("reject")}
              type="button"
            >
              <X size={16} />
              Reject Patch
            </button>
            <button
              className="command-button"
              disabled={busy || patchSet.status !== "approved" || hasDelete}
              onClick={onApplyPatch}
              type="button"
            >
              <GitPullRequest size={16} />
              Apply
            </button>
          </div>
        </div>
      )}

      {!!result?.verification_runs.length && (
        <div className="implementation-section">
          <h2>Verification</h2>
          <div className="verification-list">
            {result.verification_runs.map((run) => (
              <article className="verification-row" key={run.id}>
                <div>
                  <strong>{run.command || "not_run"}</strong>
                  <StatusPill label={run.status} />
                </div>
                <code>{run.stderr || run.stdout || `exit ${run.exit_code ?? "n/a"}`}</code>
              </article>
            ))}
          </div>
        </div>
      )}

      {result?.review_result && (
        <div className="implementation-section">
          <div className="subsection-heading">
            <h2>Review</h2>
            <StatusPill label={result.review_result.status} />
          </div>
          <p>{result.review_result.recommendation}</p>
          <FieldGroup title="Findings" values={result.review_result.findings} />
          <FieldGroup title="Residual Risks" values={result.review_result.residual_risks} />
        </div>
      )}
    </section>
  );
}

function Timeline({ events }: { events: EventRecord[] }) {
  if (!events.length) {
    return <EmptyActivity label="No events" />;
  }
  return (
    <div className="timeline">
      {events.map((event) => (
        <article className="event-card" key={event.id}>
          <div>
            <strong>{event.type}</strong>
            <time>{formatTime(event.created_at)}</time>
          </div>
          <code>{summarizePayload(event.payload)}</code>
        </article>
      ))}
    </div>
  );
}

function TracePanel({ trace }: { trace: AgentRunResult["trace"] }) {
  if (!trace) {
    return <EmptyActivity label="No trace" />;
  }
  return (
    <div className="trace-panel">
      <div className="trace-root">
        <strong>{trace.trace.id}</strong>
        <span>{trace.trace.root_name}</span>
        <StatusPill label={trace.trace.status} />
      </div>
      <div className="timeline">
        {trace.steps.map((step) => (
          <article className="event-card" key={step.id}>
            <div>
              <strong>{step.name}</strong>
              <time>{formatTime(step.started_at)}</time>
            </div>
            <code>{step.type}</code>
          </article>
        ))}
      </div>
    </div>
  );
}

function ArtifactsPanel({ artifacts }: { artifacts: AgentRunResult["artifacts"] }) {
  if (!artifacts.length) {
    return <EmptyActivity label="No artifacts" />;
  }
  return (
    <div className="timeline">
      {artifacts.map((artifact) => (
        <article className="event-card" key={artifact.id}>
          <div>
            <strong>{artifact.title}</strong>
            <time>{artifact.type}</time>
          </div>
          <code>{summarizePayload(artifact.payload)}</code>
        </article>
      ))}
    </div>
  );
}

function FieldGroup({ title, values }: { title: string; values: string[] }) {
  return (
    <div className="field-group">
      <span>{title}</span>
      <ul>
        {values.map((value) => (
          <li key={value}>{value}</li>
        ))}
      </ul>
    </div>
  );
}

function StatusBadge({ status }: { status: BackendStatus }) {
  return (
    <span className={`status-badge ${status}`}>
      <Server size={14} />
      {status}
    </span>
  );
}

function StatusPill({ label }: { label: string }) {
  return <span className={`status-pill ${label}`}>{label}</span>;
}

function RunMeter({ status }: { status: string }) {
  return (
    <div className={`run-meter ${status}`}>
      <span />
      <span />
      <span />
    </div>
  );
}

function IconButton({
  title,
  onClick,
  children
}: {
  title: string;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button className="icon-button" onClick={onClick} title={title} type="button">
      {children}
    </button>
  );
}

function TabButton({
  active,
  onClick,
  children
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button className={`tab-button ${active ? "active" : ""}`} onClick={onClick} type="button">
      {children}
    </button>
  );
}

function EmptyActivity({ label }: { label: string }) {
  return <div className="empty-activity">{label}</div>;
}

function mergeEventRecords(previous: EventRecord[], next: EventRecord[]) {
  const byId = new Map(previous.map((event) => [event.id, event]));
  for (const event of next) {
    byId.set(event.id, event);
  }
  return [...byId.values()].sort((left, right) => left.created_at.localeCompare(right.created_at));
}

function summarizePayload(payload: Record<string, unknown>) {
  const text = JSON.stringify(payload);
  return text.length > 160 ? `${text.slice(0, 157)}...` : text;
}

function formatUnknown(value: unknown) {
  const text = typeof value === "string" ? value : JSON.stringify(value);
  if (!text) return "";
  return text.length > 220 ? `${text.slice(0, 217)}...` : text;
}

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function parseTags(value: string) {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

function statusLabel(status: string) {
  if (status === "idle") return "Ready";
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
