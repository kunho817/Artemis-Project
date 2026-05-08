import {
  Activity,
  Check,
  CircleAlert,
  CircleDot,
  FileText,
  FolderOpen,
  Play,
  RefreshCw,
  Send,
  Server,
  X
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { controlPlaneApi } from "./api";
import type {
  AgentRun,
  AgentRunResult,
  BackendStatus,
  EventRecord,
  Project,
  Session
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
  const [activeTab, setActiveTab] = useState<"timeline" | "trace" | "artifacts">("timeline");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastEventIdRef = useRef<string | undefined>();

  const currentStatus = agentRun?.status ?? "idle";
  const selectedTrace = result?.trace ?? null;
  const selectedArtifacts = result?.artifacts ?? [];

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
        setCurrentSession((existing) => existing ?? loadedSessions[0] ?? null);
      })
      .catch((err) => setError(errorMessage(err)));
    return () => {
      canceled = true;
    };
  }, [currentProject]);

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
    lastEventIdRef.current = undefined;

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
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
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
        </div>

        {activeTab === "timeline" && <Timeline events={events} />}
        {activeTab === "trace" && <TracePanel trace={selectedTrace} />}
        {activeTab === "artifacts" && <ArtifactsPanel artifacts={selectedArtifacts} />}
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

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function statusLabel(status: string) {
  if (status === "idle") return "Ready";
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}
