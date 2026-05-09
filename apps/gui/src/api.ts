import type {
  AgentRun,
  AgentRunResult,
  Artifact,
  BrainstormingMode,
  BrainstormingResult,
  BrainstormingSession,
  BrainstormingSessionResponse,
  BrainstormingSourceType,
  DecisionConversionResult,
  EventRecord,
  ImplementationRun,
  ImplementationRunResult,
  MemorySearchResult,
  MemoryType,
  PatchSet,
  Project,
  ProjectMemoryItem,
  QualitySnapshot,
  RiskFinding,
  RiskFindingConversionResult,
  RiskFindingStatus,
  RiskRadar,
  RiskScanResponse,
  RiskScanResult,
  RiskScanScopeType,
  SelectedMemoryContext,
  Session,
  TraceSummary,
  VerificationRun,
  WorkPackageRequestResponse
} from "./types";

const DEFAULT_CONTROL_PLANE_URL =
  import.meta.env.VITE_CONTROL_PLANE_URL ?? "http://127.0.0.1:8000";

export class ControlPlaneApi {
  readonly baseUrl: string;

  constructor(baseUrl: string = DEFAULT_CONTROL_PLANE_URL) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  async health(): Promise<{ status: string }> {
    return this.request("/api/health");
  }

  async openProject(name: string, rootPath: string): Promise<Project> {
    return this.request("/api/projects/open", {
      method: "POST",
      body: { name, root_path: rootPath }
    });
  }

  async listProjects(): Promise<Project[]> {
    return this.request("/api/projects");
  }

  async createSession(projectId: string, title: string): Promise<Session> {
    return this.request("/api/sessions", {
      method: "POST",
      body: { project_id: projectId, title }
    });
  }

  async listSessions(projectId: string): Promise<Session[]> {
    return this.request(`/api/sessions?project_id=${encodeURIComponent(projectId)}`);
  }

  async createWorkPackageRequest(
    projectId: string,
    sessionId: string,
    userRequest: string
  ): Promise<WorkPackageRequestResponse> {
    return this.request("/api/work-package-requests", {
      method: "POST",
      body: {
        project_id: projectId,
        session_id: sessionId,
        user_request: userRequest
      }
    });
  }

  async getAgentRun(agentRunId: string): Promise<AgentRun> {
    return this.request(`/api/agent-runs/${agentRunId}`);
  }

  async getAgentRunResult(agentRunId: string): Promise<AgentRunResult> {
    return this.request(`/api/agent-runs/${agentRunId}/result`);
  }

  async listEvents(agentRunId: string, after?: string): Promise<EventRecord[]> {
    const suffix = after ? `?after=${encodeURIComponent(after)}` : "";
    return this.request(`/api/agent-runs/${agentRunId}/events${suffix}`);
  }

  async getTrace(agentRunId: string): Promise<TraceSummary> {
    return this.request(`/api/agent-runs/${agentRunId}/trace`);
  }

  async listArtifacts(agentRunId: string): Promise<Artifact[]> {
    return this.request(`/api/agent-runs/${agentRunId}/artifacts`);
  }

  async resolveApproval(approvalId: string, status: "approve" | "reject") {
    return this.request(`/api/approvals/${approvalId}/${status}`, {
      method: "POST",
      body: {}
    });
  }

  async createImplementationRun(workPackageId: string): Promise<ImplementationRunResult> {
    return this.request("/api/implementation-runs", {
      method: "POST",
      body: { work_package_id: workPackageId }
    });
  }

  async getImplementationRun(implementationRunId: string): Promise<ImplementationRun> {
    return this.request(`/api/implementation-runs/${implementationRunId}`);
  }

  async getImplementationRunResult(
    implementationRunId: string
  ): Promise<ImplementationRunResult> {
    return this.request(`/api/implementation-runs/${implementationRunId}/result`);
  }

  async listImplementationEvents(
    implementationRunId: string,
    after?: string
  ): Promise<EventRecord[]> {
    const suffix = after ? `?after=${encodeURIComponent(after)}` : "";
    return this.request(`/api/implementation-runs/${implementationRunId}/events${suffix}`);
  }

  async resolvePatchSet(patchSetId: string, status: "approve" | "reject"): Promise<PatchSet> {
    return this.request(`/api/patch-sets/${patchSetId}/${status}`, {
      method: "POST",
      body: {}
    });
  }

  async applyPatchSet(patchSetId: string): Promise<PatchSet> {
    return this.request(`/api/patch-sets/${patchSetId}/apply`, {
      method: "POST",
      body: {}
    });
  }

  async runVerification(
    implementationRunId: string,
    command?: string
  ): Promise<VerificationRun> {
    return this.request(`/api/implementation-runs/${implementationRunId}/verification-runs`, {
      method: "POST",
      body: command ? { command } : {}
    });
  }

  async createBrainstormingSession(payload: {
    projectId: string;
    sessionId: string;
    topic: string;
    mode: BrainstormingMode;
    sourceType: BrainstormingSourceType;
    sourceId?: string | null;
    roles: string[];
  }): Promise<BrainstormingSessionResponse> {
    return this.request("/api/brainstorming-sessions", {
      method: "POST",
      body: {
        project_id: payload.projectId,
        session_id: payload.sessionId,
        topic: payload.topic,
        mode: payload.mode,
        source_type: payload.sourceType,
        source_id: payload.sourceId ?? null,
        roles: payload.roles
      }
    });
  }

  async getBrainstormingSession(brainstormingSessionId: string): Promise<BrainstormingSession> {
    return this.request(`/api/brainstorming-sessions/${brainstormingSessionId}`);
  }

  async getBrainstormingResult(brainstormingSessionId: string): Promise<BrainstormingResult> {
    return this.request(`/api/brainstorming-sessions/${brainstormingSessionId}/result`);
  }

  async listBrainstormingEvents(
    brainstormingSessionId: string,
    after?: string
  ): Promise<EventRecord[]> {
    const suffix = after ? `?after=${encodeURIComponent(after)}` : "";
    return this.request(`/api/brainstorming-sessions/${brainstormingSessionId}/events${suffix}`);
  }

  async resolveDecisionBrief(
    brainstormingSessionId: string,
    decisionBriefId: string,
    status: "accept" | "reject"
  ): Promise<BrainstormingResult> {
    return this.request(`/api/brainstorming-sessions/${brainstormingSessionId}/decision/${status}`, {
      method: "POST",
      body:
        status === "accept"
          ? { decision_brief_id: decisionBriefId, note: "Accepted from GUI." }
          : { decision_brief_id: decisionBriefId, reason: "Rejected from GUI." }
    });
  }

  async convertDecisionRecord(decisionRecordId: string): Promise<DecisionConversionResult> {
    return this.request(`/api/decision-records/${decisionRecordId}/convert-to-work-package`, {
      method: "POST",
      body: {}
    });
  }

  async listMemory(projectId: string, type?: MemoryType, status = "active"): Promise<ProjectMemoryItem[]> {
    const params = new URLSearchParams();
    if (type) params.set("type", type);
    if (status) params.set("status", status);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    return this.request(`/api/projects/${projectId}/memory${suffix}`);
  }

  async searchMemory(payload: {
    projectId: string;
    query: string;
    type?: MemoryType | "";
    status?: string;
  }): Promise<MemorySearchResult[]> {
    const params = new URLSearchParams();
    params.set("q", payload.query);
    if (payload.type) params.set("type", payload.type);
    if (payload.status) params.set("status", payload.status);
    return this.request(`/api/projects/${payload.projectId}/memory/search?${params.toString()}`);
  }

  async createProjectRule(payload: {
    projectId: string;
    sessionId?: string;
    title: string;
    summary: string;
    body: string;
    tags: string[];
    importance: string;
  }): Promise<ProjectMemoryItem> {
    return this.request(`/api/projects/${payload.projectId}/memory`, {
      method: "POST",
      body: {
        type: "project_rule",
        title: payload.title,
        summary: payload.summary,
        body: payload.body,
        tags: payload.tags,
        importance: payload.importance,
        session_id: payload.sessionId
      }
    });
  }

  async updateMemoryItem(
    memoryItemId: string,
    payload: Partial<Pick<ProjectMemoryItem, "title" | "summary" | "body" | "tags" | "importance">>
  ): Promise<ProjectMemoryItem> {
    return this.request(`/api/memory/items/${memoryItemId}`, {
      method: "PATCH",
      body: payload
    });
  }

  async archiveMemoryItem(memoryItemId: string): Promise<ProjectMemoryItem> {
    return this.request(`/api/memory/items/${memoryItemId}/archive`, {
      method: "POST",
      body: {}
    });
  }

  async restoreMemoryItem(memoryItemId: string): Promise<ProjectMemoryItem> {
    return this.request(`/api/memory/items/${memoryItemId}/restore`, {
      method: "POST",
      body: {}
    });
  }

  async promoteDecisionRecord(decisionRecordId: string): Promise<ProjectMemoryItem> {
    return this.request(`/api/decision-records/${decisionRecordId}/promote-to-memory`, {
      method: "POST",
      body: {}
    });
  }

  async createSessionMemorySummary(sessionId: string): Promise<{
    status: string;
    memory_item: ProjectMemoryItem | null;
    reason?: string;
  }> {
    return this.request(`/api/sessions/${sessionId}/memory-summary`, {
      method: "POST",
      body: {}
    });
  }

  async promoteReviewFailureMemory(reviewResultId: string): Promise<ProjectMemoryItem> {
    return this.request(`/api/review-results/${reviewResultId}/promote-failure-memory`, {
      method: "POST",
      body: {}
    });
  }

  async promoteVerificationFailureMemory(verificationRunId: string): Promise<ProjectMemoryItem> {
    return this.request(`/api/verification-runs/${verificationRunId}/promote-failure-memory`, {
      method: "POST",
      body: {}
    });
  }

  async listSelectedMemory(sessionId: string): Promise<SelectedMemoryContext> {
    return this.request(`/api/sessions/${sessionId}/selected-memory`);
  }

  async selectMemory(sessionId: string, memoryItemId: string): Promise<SelectedMemoryContext["selected_memory"][number]> {
    return this.request(`/api/sessions/${sessionId}/selected-memory`, {
      method: "POST",
      body: { memory_item_id: memoryItemId }
    });
  }

  async unselectMemory(sessionId: string, memoryItemId: string): Promise<{ status: string }> {
    return this.request(`/api/sessions/${sessionId}/selected-memory/${memoryItemId}`, {
      method: "DELETE"
    });
  }

  async createRiskScan(payload: {
    projectId: string;
    sessionId: string;
    scopeType: RiskScanScopeType;
    includeSelectedMemory: boolean;
    selectedMemoryIds: string[];
    focus: string[];
  }): Promise<RiskScanResponse> {
    return this.request(`/api/projects/${payload.projectId}/risk-scans`, {
      method: "POST",
      body: {
        session_id: payload.sessionId,
        scope_type: payload.scopeType,
        scope_id: null,
        include_selected_memory: payload.includeSelectedMemory,
        selected_memory_ids: payload.selectedMemoryIds,
        focus: payload.focus
      }
    });
  }

  async getRiskScan(riskScanRunId: string): Promise<RiskScanResult["risk_scan_run"]> {
    return this.request(`/api/risk-scans/${riskScanRunId}`);
  }

  async getRiskScanResult(riskScanRunId: string): Promise<RiskScanResult> {
    return this.request(`/api/risk-scans/${riskScanRunId}/result`);
  }

  async listRiskScanEvents(riskScanRunId: string, after?: string): Promise<EventRecord[]> {
    const suffix = after ? `?after=${encodeURIComponent(after)}` : "";
    return this.request(`/api/risk-scans/${riskScanRunId}/events${suffix}`);
  }

  async getRiskRadar(projectId: string): Promise<RiskRadar> {
    return this.request(`/api/projects/${projectId}/risk-radar`);
  }

  async getQualitySnapshot(projectId: string): Promise<QualitySnapshot> {
    return this.request(`/api/projects/${projectId}/quality-snapshot`);
  }

  async updateRiskFindingStatus(
    riskFindingId: string,
    status: Exclude<RiskFindingStatus, "converted">
  ): Promise<RiskFinding> {
    return this.request(`/api/risk-findings/${riskFindingId}`, {
      method: "PATCH",
      body: { status }
    });
  }

  async convertRiskFinding(riskFindingId: string): Promise<RiskFindingConversionResult> {
    return this.request(`/api/risk-findings/${riskFindingId}/convert-to-work-package`, {
      method: "POST",
      body: {}
    });
  }

  eventStreamUrl(agentRunId: string): string {
    return `${this.baseUrl}/api/agent-runs/${agentRunId}/events/stream`;
  }

  implementationEventStreamUrl(implementationRunId: string): string {
    return `${this.baseUrl}/api/implementation-runs/${implementationRunId}/events/stream`;
  }

  brainstormingEventStreamUrl(brainstormingSessionId: string): string {
    return `${this.baseUrl}/api/brainstorming-sessions/${brainstormingSessionId}/events/stream`;
  }

  riskScanEventStreamUrl(riskScanRunId: string): string {
    return `${this.baseUrl}/api/risk-scans/${riskScanRunId}/events/stream`;
  }

  private async request<T>(path: string, init: { method?: string; body?: unknown } = {}): Promise<T> {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: init.method ?? "GET",
      headers: init.body === undefined ? undefined : { "Content-Type": "application/json" },
      body: init.body === undefined ? undefined : JSON.stringify(init.body)
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(`${response.status} ${response.statusText}: ${text}`);
    }

    return response.json() as Promise<T>;
  }
}

export const controlPlaneApi = new ControlPlaneApi();
