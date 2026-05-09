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

export type ImplementationRun = {
  id: string;
  project_id: string;
  session_id: string;
  work_package_id: string;
  status: string;
  current_phase: string | null;
  trace_id: string | null;
  created_at: string;
  updated_at: string;
};

export type ImplementationPlan = {
  id: string;
  implementation_run_id: string;
  goal: string;
  context_summary: string;
  target_files: string[];
  steps: string[];
  verification_strategy: string[];
  risks: RiskHint[];
  created_at: string;
};

export type PatchFile = {
  id: string;
  patch_set_id: string;
  path: string;
  operation: "create" | "update" | "delete";
  diff: string;
  rationale: string;
  risk_level: string;
  replacement_content: string;
};

export type PatchSet = {
  id: string;
  implementation_run_id: string;
  status: string;
  summary: string;
  risk_level: string;
  approval_status: "not_required" | "pending" | "approved" | "rejected";
  applied_files: string[];
  files: PatchFile[];
  created_at: string;
  updated_at: string;
};

export type VerificationRun = {
  id: string;
  implementation_run_id: string;
  command: string;
  status: "not_run" | "running" | "passed" | "failed" | "blocked";
  exit_code: number | null;
  stdout: string;
  stderr: string;
  started_at: string;
  ended_at: string | null;
};

export type ReviewResult = {
  id: string;
  implementation_run_id: string;
  status: "pass" | "needs_changes" | "blocked";
  findings: string[];
  residual_risks: string[];
  recommendation: string;
  created_at: string;
};

export type ImplementationRunResult = {
  implementation_run: ImplementationRun;
  work_package: WorkPackage;
  implementation_plan: ImplementationPlan | null;
  patch_set: PatchSet | null;
  verification_runs: VerificationRun[];
  review_result: ReviewResult | null;
  trace: TraceSummary | null;
  events: EventRecord[];
};

export type BrainstormingMode =
  | "free_ideation"
  | "architecture_debate"
  | "implementation_strategy"
  | "risk_review"
  | "product_planning";

export type BrainstormingSourceType =
  | "topic"
  | "work_package"
  | "implementation_run"
  | "review_result";

export type BrainstormingSession = {
  id: string;
  project_id: string;
  session_id: string;
  source_type: BrainstormingSourceType;
  source_id: string | null;
  topic: string;
  mode: BrainstormingMode;
  status: string;
  current_phase: string | null;
  selected_roles: string[];
  trace_id: string | null;
  created_at: string;
  updated_at: string;
};

export type BrainstormingContribution = {
  id: string;
  brainstorming_session_id: string;
  role: string;
  stance: string;
  summary: string;
  arguments: string[];
  concerns: string[];
  suggested_actions: string[];
  referenced_artifacts: string[];
  created_at: string;
};

export type BrainstormingCritique = {
  id: string;
  brainstorming_session_id: string;
  critic_role: string;
  target_role: string;
  weak_assumptions: string[];
  missing_context: string[];
  risks: string[];
  suggested_revisions: string[];
  created_at: string;
};

export type BrainstormingOption = {
  id: string;
  brainstorming_session_id: string;
  title: string;
  summary: string;
  benefits: string[];
  costs: string[];
  risks: string[];
  required_work: string[];
  verification_hint: string;
  score: number;
  created_at: string;
};

export type DecisionBrief = {
  id: string;
  brainstorming_session_id: string;
  recommendation: string;
  selected_option_id: string;
  rationale: string;
  tradeoffs: string[];
  risks: string[];
  open_questions: string[];
  follow_up_actions: string[];
  work_package_candidate: Record<string, unknown>;
  status: "pending" | "accepted" | "rejected";
  created_at: string;
};

export type DecisionRecord = {
  id: string;
  project_id: string;
  session_id: string;
  brainstorming_session_id: string;
  title: string;
  decision: string;
  rationale: string;
  consequences: string[];
  follow_up_actions: string[];
  linked_work_package_id: string | null;
  created_at: string;
};

export type BrainstormingSessionResponse = {
  project_id: string;
  session_id: string;
  brainstorming_session_id: string;
  status: string;
  events_url: string;
};

export type BrainstormingResult = {
  brainstorming_session: BrainstormingSession;
  contributions: BrainstormingContribution[];
  critiques: BrainstormingCritique[];
  options: BrainstormingOption[];
  decision_brief: DecisionBrief | null;
  decision_record: DecisionRecord | null;
  trace: TraceSummary | null;
  events: EventRecord[];
  artifacts: Artifact[];
};

export type DecisionConversionResult = {
  decision_record: DecisionRecord;
  work_package: WorkPackage;
  approval: Approval | null;
};

export type MemoryType = "decision" | "session_summary" | "project_rule" | "failure" | "work_note";
export type MemoryStatus = "active" | "archived" | "superseded";

export type MemorySourceLink = {
  id: string;
  memory_item_id: string;
  source_type: string;
  source_id: string;
  relation: string;
  created_at: string;
};

export type ProjectMemoryItem = {
  id: string;
  project_id: string;
  type: MemoryType;
  title: string;
  summary: string;
  body: string;
  tags: string[];
  status: MemoryStatus;
  importance: string;
  confidence: number;
  created_by: string;
  source_count: number;
  last_used_at: string | null;
  created_at: string;
  updated_at: string;
  source_links: MemorySourceLink[];
};

export type MemorySearchResult = {
  item: ProjectMemoryItem;
  score: number;
  matched_fields: string[];
  source_links: MemorySourceLink[];
  snippet: string;
};

export type SelectedMemoryItem = {
  session_id: string;
  memory_item_id: string;
  snapshot: Pick<
    ProjectMemoryItem,
    "id" | "type" | "title" | "summary" | "body" | "tags" | "source_links"
  >;
  created_at: string;
};

export type SelectedMemoryContext = {
  session_id: string;
  selected_memory: SelectedMemoryItem[];
  source_context: SelectedMemoryItem["snapshot"][];
};

export type RiskScanStatus = "queued" | "collecting" | "analyzing" | "completed" | "failed" | "canceled";
export type RiskScanScopeType =
  | "project"
  | "session"
  | "work_package"
  | "implementation_run"
  | "review_result"
  | "memory_focus";
export type RiskFindingStatus = "open" | "accepted" | "dismissed" | "mitigated" | "converted";

export type AnalysisSourceLink = {
  source_type: string;
  source_id: string;
  relation: string;
  label?: string;
};

export type RiskScanRun = {
  id: string;
  project_id: string;
  session_id: string;
  scope_type: RiskScanScopeType;
  scope_id: string | null;
  status: RiskScanStatus;
  current_phase: string | null;
  selected_memory_count: number;
  trace_id: string | null;
  source_context: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type RiskFinding = {
  id: string;
  project_id: string;
  risk_scan_run_id: string;
  category: string;
  severity: "info" | "low" | "medium" | "high" | "critical";
  title: string;
  summary: string;
  evidence: string[];
  recommendation: string;
  confidence: number;
  status: RiskFindingStatus;
  source_links: AnalysisSourceLink[];
  converted_work_package_id: string | null;
  created_at: string;
  updated_at: string;
};

export type QualitySignal = {
  id: string;
  project_id: string;
  risk_scan_run_id: string;
  kind: string;
  status: "healthy" | "watch" | "at_risk" | "unknown";
  title: string;
  summary: string;
  value: unknown;
  target: unknown;
  evidence: string[];
  source_links: AnalysisSourceLink[];
  created_at: string;
};

export type ProjectHealthSnapshot = {
  id: string;
  project_id: string;
  risk_scan_run_id: string;
  overall_status: "healthy" | "watch" | "at_risk" | "blocked" | "unknown";
  overall_score: number;
  risk_counts: Record<string, number>;
  top_findings: Array<Record<string, unknown>>;
  quality_summary: Record<string, unknown>;
  recommendation: string;
  created_at: string;
};

export type ArchitectureMapSnapshot = {
  id: string;
  project_id: string;
  risk_scan_run_id: string;
  nodes: Array<Record<string, unknown>>;
  edges: Array<Record<string, unknown>>;
  hotspots: Array<Record<string, unknown>>;
  boundary_notes: string[];
  created_at: string;
};

export type RiskScanResponse = {
  project_id: string;
  session_id: string;
  risk_scan_run_id: string;
  status: RiskScanStatus;
  events_url: string;
};

export type RiskScanResult = {
  risk_scan_run: RiskScanRun;
  findings: RiskFinding[];
  quality_signals: QualitySignal[];
  project_health_snapshot: ProjectHealthSnapshot | null;
  architecture_map_snapshot: ArchitectureMapSnapshot | null;
  trace: TraceSummary | null;
  events: EventRecord[];
  artifacts: Artifact[];
};

export type RiskRadar = {
  project_id: string;
  latest_scan: RiskScanRun | null;
  health: ProjectHealthSnapshot | null;
  findings: RiskFinding[];
  severity_counts: Record<string, number>;
  category_counts: Record<string, number>;
  status_counts: Record<string, number>;
};

export type QualitySnapshot = {
  project_id: string;
  latest_scan: RiskScanRun | null;
  health: ProjectHealthSnapshot | null;
  signals: QualitySignal[];
  architecture_map: ArchitectureMapSnapshot | null;
};

export type RiskFindingConversionResult = {
  risk_finding: RiskFinding;
  work_package: WorkPackage;
  approval: Approval | null;
};
