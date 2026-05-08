export type BackendStatus = "checking" | "online" | "offline";

export type Project = {
  id: string;
  name: string;
  root_path: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type Session = {
  id: string;
  project_id: string;
  title: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type AgentRunStatus = "idle" | "queued" | "running" | "completed" | "failed" | "canceled";

export type AgentRun = {
  id: string;
  project_id: string;
  session_id: string;
  user_request: string;
  status: AgentRunStatus;
  intent: string | null;
  current_phase: string | null;
  trace_id: string | null;
  external_trace_id: string | null;
  created_at: string;
  updated_at: string;
};

export type EventRecord = {
  id: string;
  project_id: string;
  session_id: string;
  agent_run_id: string | null;
  type: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type RiskHint = {
  level: "low" | "medium" | "high" | "critical";
  description: string;
};

export type WorkPackage = {
  id: string;
  project_id: string;
  session_id: string;
  source_agent_run_id: string;
  title: string;
  goal: string;
  background: string;
  scope: string[];
  out_of_scope: string[];
  related_files: string[];
  required_agents: string[];
  implementation_steps: string[];
  verification: string[];
  risks: RiskHint[];
  approval_required: boolean;
  approval_status: "not_required" | "pending" | "approved" | "rejected";
  completion_criteria: string[];
  status: string;
  created_at: string;
  updated_at: string;
};

export type Approval = {
  id: string;
  project_id: string;
  session_id: string;
  target_type: string;
  target_id: string;
  reason: string;
  risk_level: string;
  status: "not_required" | "pending" | "approved" | "rejected";
  created_at: string;
  resolved_at: string | null;
};

export type Artifact = {
  id: string;
  project_id: string;
  session_id: string;
  source_agent_run_id: string;
  type: string;
  title: string;
  payload: Record<string, unknown>;
  created_at: string;
};

export type TraceSummary = {
  trace: {
    id: string;
    project_id: string;
    session_id: string;
    agent_run_id: string;
    root_name: string;
    status: string;
    started_at: string;
    ended_at: string | null;
    metadata: Record<string, unknown>;
  };
  steps: Array<{
    id: string;
    trace_id: string;
    parent_step_id: string | null;
    name: string;
    type: string;
    status: string;
    inputs_summary: string;
    outputs_summary: string;
    started_at: string;
    ended_at: string | null;
  }>;
};

export type AgentRunResult = {
  agent_run: AgentRun;
  work_package: WorkPackage | null;
  approval: Approval | null;
  artifacts: Artifact[];
  trace: TraceSummary | null;
  events: EventRecord[];
};

export type WorkPackageRequestResponse = {
  project_id: string;
  session_id: string;
  agent_run_id: string;
  status: AgentRunStatus;
  events_url: string;
};
