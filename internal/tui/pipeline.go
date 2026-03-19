package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/agent/roles"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/state"
)

// OrchestratorPlanMsg carries the Orchestrator's routing decision back to the Update loop.
type OrchestratorPlanMsg struct {
	Response *orchestrator.OrchestratorResponse
	UserText string // original user input for context + fallback
	Error    error
}

// handleOrchestratedSubmit sends the user message to the Orchestrator for intent classification.
// The Orchestrator classifies intent (trivial/conversational/exploratory/complex) and
// creates a routing decision. Layout stays single until intent is known.
func (a App) handleOrchestratedSubmit(text string) (tea.Model, tea.Cmd) {
	// Build Orchestrator provider
	orchProvider := a.buildProviderWithFallback("orchestrator")
	if orchProvider == nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "No LLM provider available for Orchestrator. Press Ctrl+S to configure.",
		})
		return a, nil
	}

	// Track orchestrator activity (visible when/if layout switches to split)
	a.activity.AddActivity(ActivityItem{
		Status: StatusRunning,
		Text:   "Orchestrator: classifying intent...",
	})
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	a.activity.SetAgentCount(1)
	a.pipelineRunning = true
	// Layout stays single — handleOrchestratorPlan decides based on intent

	// Fire async Orchestrator LLM call
	userText := text
	history := make([]llm.Message, 0, len(a.history)+1)
	history = append(history, llm.Message{Role: "system", Content: roles.BuildOrchestratorPrompt(a.skillRegistry)})
	history = append(history, a.history...)
	cmd := func() tea.Msg {
		ctx := context.Background() // no timeout — AI tasks take as long as needed
		resp, err := orchProvider.Send(ctx, history)
		if err != nil {
			return OrchestratorPlanMsg{UserText: userText, Error: err}
		}
		orchResp, parseErr := orchestrator.ParseOrchestratorResponse(resp)
		return OrchestratorPlanMsg{Response: orchResp, UserText: userText, Error: parseErr}
	}

	return a, cmd
}

// handleOrchestratorPlan processes the Orchestrator's intent classification and routing.
// Routes based on intent: trivial (direct stream), conversational (single agent with tools),
// exploratory (placeholder → complex), complex (full multi-agent pipeline).
func (a App) handleOrchestratorPlan(msg OrchestratorPlanMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		// Orchestrator failed — fall back to legacy fixed pipeline
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fmt.Sprintf("Orchestrator: %s (falling back to pipeline)", msg.Error),
		})
		a.layoutMode = LayoutSplit
		a.recalcLayout()
		return a.executeLegacyPipeline(msg.UserText)
	}

	resp := msg.Response

	// Show intent classification in activity
	a.activity.AddActivity(ActivityItem{
		Status: StatusDone,
		Text:   fmt.Sprintf("Orchestrator: intent=%s", resp.Intent),
	})

	switch resp.Intent {
	case orchestrator.IntentTrivial:
		// Direct streaming — no tools, no split layout, no Engine
		return a.executeTrivial(resp.DirectAgent, resp.DirectTask, resp.DirectCategory, resp.DirectSkills)

	case orchestrator.IntentConversational:
		// Single agent with tools — use executePlan with 1-step plan, stay single
		plan := resp.ToExecutionPlan()
		if plan == nil {
			a.layoutMode = LayoutSplit
			a.recalcLayout()
			return a.executeLegacyPipeline(msg.UserText)
		}
		a.activity.SetAgentCount(1)
		return a.executePlan(plan, msg.UserText)

	case orchestrator.IntentExploratory:
		// Phase 3 placeholder — treat as complex for now
		plan := resp.ToExecutionPlan()
		if plan == nil {
			a.layoutMode = LayoutSplit
			a.recalcLayout()
			return a.executeLegacyPipeline(msg.UserText)
		}
		a.layoutMode = LayoutSplit
		a.recalcLayout()
		a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
		a.activity.SetAgentCount(plan.TotalTasks())
		if resp.Reasoning != "" {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: resp.Reasoning})
		}
		return a.executePlan(plan, msg.UserText)

	case orchestrator.IntentComplex:
		// Full multi-agent pipeline — switch to split layout
		plan := resp.ToExecutionPlan()
		if plan == nil {
			a.layoutMode = LayoutSplit
			a.recalcLayout()
			return a.executeLegacyPipeline(msg.UserText)
		}
		a.layoutMode = LayoutSplit
		a.recalcLayout()
		a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
		a.activity.SetAgentCount(plan.TotalTasks())
		if resp.Reasoning != "" {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: resp.Reasoning})
		}
		return a.executePlan(plan, msg.UserText)

	default:
		// Unknown intent — fall back to legacy pipeline
		a.layoutMode = LayoutSplit
		a.recalcLayout()
		return a.executeLegacyPipeline(msg.UserText)
	}
}

// executePlan runs a dynamic ExecutionPlan from the Orchestrator.
func (a App) executePlan(plan *orchestrator.ExecutionPlan, userText string) (tea.Model, tea.Cmd) {
	eb := bus.NewEventBus(eventBusBuffer)
	a.eventBus = eb

	ctx, cancel := context.WithCancel(context.Background()) // no timeout — pipelines run to completion
	a.cancelPipeline = cancel

	// Phase 5: Generate pipeline run ID and persist
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())
	a.activePipelineRunID = runID
	a.savePipelineRunWithPlan(runID, plan, "complex")

	// Prepare session state with user request + conversation history
	ss := state.NewSessionStateWithID(runID, a.sessionID, "")
	ss.SetHistory(a.buildHistorySummary())
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactUserRequest,
		Source:  "user",
		Content: userText,
	})

	// Store the plan itself as an artifact
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactOrchestratorPlan,
		Source:  "orchestrator",
		Content: plan.Reasoning,
	})

	// Agent builder: creates fully-configured agents from Orchestrator plan tasks.
	// Handles category-based provider override, skills injection, memory, and repo-map.
	buildAgent := func(task orchestrator.AgentTask) agent.Agent {
		provider := a.buildProviderForTask(task.Agent, task.Category)
		if provider == nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, task.Agent, "plan", "skipped: no API key configured"))
			return nil
		}
		ag := roles.NewRoleAgent(agent.Role(task.Agent), provider, eb, a.toolExecutor)
		if a.memStore != nil {
			ag.SetMemory(a.memStore)
		}
		if a.repoMapStore != nil {
			ag.SetRepoMap(a.repoMapStore)
		}
		ag.SetMaxToolIter(a.cfg.MaxToolIter)
		// Apply task, criticality, category, skills, context
		ag.SetTask(task.Task)
		ag.SetCritical(task.Critical)
		if a.projectRules != "" {
			ag.SetProjectRules(a.projectRules)
		}
		if a.codeIndex != nil {
			ag.SetCodeIndex(a.codeIndex)
		}
		if task.Category != "" {
			ag.SetCategory(agent.TaskCategory(task.Category))
		}
		if len(task.Skills) > 0 && a.skillRegistry != nil {
			ag.SetSkills(a.skillRegistry.Resolve(task.Skills))
		}
		if a.historyWindow != nil {
			ag.SetHistoryWindow(a.historyWindow)
		}
		// Phase E-2: Autonomous mode
		if task.Autonomous {
			cwd, _ := os.Getwd()
			verifyFn := agent.ResolveVerifyFunc(task.VerifyWith, cwd)
			ag.SetAutonomous(verifyFn, task.MaxRetries)
		}
		return ag
	}

	// Create engine (nil pipeline — using RunPlan instead)
	// Create recovery bridge for failure recovery (Phase 6)
	bridge := NewRecoveryBridge()
	a.recoveryBridge = bridge

	// Create engine with recovery support
	engine := orchestrator.NewEngine(nil, eb, bridge, buildAgent)

	// Phase C-5: Wire checkpoint store for step-level persistence
	if a.checkpointStore != nil {
		engine.SetCheckpointStore(a.checkpointStore)
	}

	// Phase C-7: Wire replanner for conditional re-planning on failure
	if orchProvider := a.buildProviderWithFallback("orchestrator"); orchProvider != nil {
		engine.SetReplanner(NewOrchestratorReplanner(orchProvider))
	}

	// Create background task manager for parallel background tasks
	bgMgr := orchestrator.NewBackgroundTaskManager(eb)

	// Spawn background tasks (run parallel to main pipeline)
	for _, bgDef := range plan.BackgroundTasks {
		bgMgr.Spawn(ctx, bgDef, buildAgent, userText)
	}

	memStore := a.memStore
	capturedRunID := runID
	go func() {
		result := engine.RunPlan(ctx, plan, ss, buildAgent)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "plan", result.HaltError.Error()))
		}

		// Phase 5: Save background task results to DB
		for _, bt := range bgMgr.AllTasks() {
			if bt.Result != "" && memStore != nil {
				_ = memStore.SaveMessage(context.Background(), &memory.SessionMessage{
					SessionID:     ss.SessionID(),
					Role:          "assistant",
					Content:       bt.Result,
					AgentRole:     bt.AgentRole,
					PipelineRunID: capturedRunID,
				})
			}
		}

		// Phase 5: Update pipeline run status
		if memStore != nil {
			status := "completed"
			if !result.Completed {
				status = "failed"
			}
			_ = memStore.UpdatePipelineRun(context.Background(), capturedRunID, status)
		}

		// Wait for all background tasks before closing EventBus
		bgMgr.WaitAll()
		eb.Close()
	}()

	return a, tea.Batch(a.waitForEvent(eb), waitForRecoveryRequest(bridge))
}

// executeTrivial handles trivial intent — direct LLM streaming without Engine or tools.
// Stays in single layout for a lightweight, fast response path.
// Optional category/skills from OrchestratorResponse are used for provider/prompt selection.
func (a App) executeTrivial(agentRole, _ string, category string, skills []string) (tea.Model, tea.Cmd) {
	// Use category-based provider if specified, otherwise role-based
	provider := a.buildProviderForTask(agentRole, category)
	if provider == nil {
		a.pipelineRunning = false
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("No provider available for %s.", agentRole),
		})
		return a, nil
	}

	a.pipelineRunning = false // No pipeline — just direct streaming

	// Build messages with role-specific system prompt + conversation history
	systemPrompt := roles.SystemPromptForRole(agent.Role(agentRole))

	// Append category behavioral prompt if specified
	if category != "" {
		if catPrompt := agent.PromptForCategory(agent.TaskCategory(category)); catPrompt != "" {
			systemPrompt += "\n\n" + catPrompt
		}
	}

	// Append skill content if specified
	if len(skills) > 0 && a.skillRegistry != nil {
		resolved := a.skillRegistry.Resolve(skills)
		if content := agent.FormatSkillsContent(resolved); content != "" {
			systemPrompt += "\n\n## Skills\n" + content
		}
	}

	// Add project rules to the trivial agent's prompt
	if a.projectRules != "" {
		systemPrompt += "\n\n## Project Rules\n" + a.projectRules
	}

	messages := make([]llm.Message, 0, len(a.history)+1)
	if systemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, a.history...)

	// Start streaming (reuses single-mode streaming path)
	a.chat.AddMessage(ChatMessage{Role: RoleAssistant, Content: ""})
	cmd := func() tea.Msg {
		ctx := context.Background() // no timeout
		ch, err := provider.Stream(ctx, messages)
		if err != nil {
			return LLMResponseMsg{Error: err}
		}
		return streamStartMsg{channel: ch}
	}
	return a, cmd
}

// executeLegacyPipeline runs the fixed 5-phase pipeline as a fallback
// when the Orchestrator fails to produce a valid plan.
func (a App) executeLegacyPipeline(text string) (tea.Model, tea.Cmd) {
	eb := bus.NewEventBus(eventBusBuffer)
	a.eventBus = eb
	a.pipelineRunning = true
	a.layoutMode = LayoutSplit
	a.recalcLayout()

	// Phase 5: Generate pipeline run ID
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())
	a.activePipelineRunID = runID
	a.savePipelineRun(runID, "", "complex")

	pipeline := orchestrator.DefaultPipeline()

	analysisAgents := a.buildAgentsForPhase(eb,
		agent.RoleAnalyzer, agent.RoleSearcher, agent.RoleExplorer)
	planningAgents := a.buildAgentsForPhase(eb, agent.RolePlanner)
	archAgents := a.buildAgentsForPhase(eb, agent.RoleArchitect)
	implAgents := a.buildAgentsForPhase(eb,
		agent.RoleCoder, agent.RoleDesigner, agent.RoleEngineer)
	verifyAgents := a.buildAgentsForPhase(eb,
		agent.RoleQA, agent.RoleTester)

	_ = pipeline.SetPhaseAgents(orchestrator.PhaseAnalysis, analysisAgents...)
	_ = pipeline.SetPhaseAgents(orchestrator.PhasePlanning, planningAgents...)
	_ = pipeline.SetPhaseAgents(orchestrator.PhaseArchitecture, archAgents...)
	_ = pipeline.SetPhaseAgents(orchestrator.PhaseImplementation, implAgents...)
	_ = pipeline.SetPhaseAgents(orchestrator.PhaseVerification, verifyAgents...)

	a.activity.AddActivity(ActivityItem{
		Status: StatusRunning,
		Text:   "Fallback: legacy pipeline (5 phases)...",
	})
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	a.activity.SetAgentCount(len(analysisAgents) + len(planningAgents) + len(archAgents) + len(implAgents) + len(verifyAgents))

	ctx, cancel := context.WithCancel(context.Background()) // no timeout
	a.cancelPipeline = cancel

	ss := state.NewSessionStateWithID(runID, a.sessionID, "")
	ss.SetHistory(a.buildHistorySummary())
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactUserRequest,
		Source:  "user",
		Content: text,
	})

	// Create recovery bridge for failure recovery (Phase 6)
	legacyBridge := NewRecoveryBridge()
	a.recoveryBridge = legacyBridge

	engine := orchestrator.NewEngine(pipeline, eb, legacyBridge, nil)

	memStore := a.memStore
	capturedRunID := runID
	go func() {
		result := engine.Run(ctx, ss)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "pipeline", result.HaltError.Error()))
		}

		// Phase 5: Update pipeline run status
		if memStore != nil {
			status := "completed"
			if !result.Completed {
				status = "failed"
			}
			_ = memStore.UpdatePipelineRun(context.Background(), capturedRunID, status)
		}

		eb.Close()
	}()

	return a, tea.Batch(a.waitForEvent(eb), waitForRecoveryRequest(legacyBridge))
}

// buildAgentsForPhase creates agents for the given roles.
// Each agent gets a FallbackProvider: primary (from role mapping) + fallback chain.
// If primary has no API key, fallbacks are still tried.
func (a *App) buildAgentsForPhase(eb *bus.EventBus, agentRoles ...agent.Role) []agent.Agent {
	var agents []agent.Agent
	for _, role := range agentRoles {
		provider := a.buildProviderWithFallback(string(role))
		if provider == nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, string(role), "init", "skipped: no API key configured"))
			continue
		}
		ag := roles.NewRoleAgent(role, provider, eb, a.toolExecutor)
		if a.memStore != nil {
			ag.SetMemory(a.memStore)
		}
		if a.repoMapStore != nil {
			ag.SetRepoMap(a.repoMapStore)
		}
		ag.SetMaxToolIter(a.cfg.MaxToolIter)
		if a.projectRules != "" {
			ag.SetProjectRules(a.projectRules)
		}
		if a.codeIndex != nil {
			ag.SetCodeIndex(a.codeIndex)
		}
		if a.historyWindow != nil {
			ag.SetHistoryWindow(a.historyWindow)
		}
		agents = append(agents, ag)
	}
	return agents
}

// buildProviderWithFallback creates a provider for a role with tier-based fallback.
// In premium tier: primary = premium mapping, fallback = budget mapping for same role.
// In budget tier: primary = budget mapping, no fallback.
// Returns nil only if no provider has an API key.
func (a *App) buildProviderWithFallback(role string) llm.Provider {
	primaryName := a.cfg.ProviderForRole(role)
	modelOverride := a.cfg.ModelForRole(role)

	// Build primary with retry (nil if no API key)
	var primary llm.Provider
	if a.hasAPIKey(primaryName) {
		if modelOverride != "" {
			primary = llm.NewRetryProvider(llm.NewProviderWithModel(primaryName, &a.cfg, modelOverride), 2)
		} else {
			primary = llm.NewRetryProvider(llm.NewProvider(primaryName, &a.cfg), 2)
		}
	}

	// Build fallback from budget tier (only when on premium), also with retry
	fbName := a.cfg.FallbackProviderForRole(role)
	var fallback llm.Provider
	if fbName != "" && fbName != primaryName && a.hasAPIKey(fbName) {
		fallback = llm.NewRetryProvider(llm.NewProvider(fbName, &a.cfg), 2)
	}

	// No providers available at all
	if primary == nil && fallback == nil {
		return nil
	}

	// No fallback needed — just return primary (already has retry)
	if fallback == nil {
		return primary
	}

	return llm.NewFallbackProvider(primary, fallback)
}

// buildProviderForTask creates a provider with optional category-based override.
// If a valid category is provided, its provider/model mapping is used instead of the role's.
// Falls back to role-based provider selection if category has no override or no API key.
func (a *App) buildProviderForTask(role, category string) llm.Provider {
	if category != "" && agent.IsValidCategory(category) {
		tier := a.cfg.Agents.Tier
		catProvider := agent.ProviderForCategory(agent.TaskCategory(category), tier)
		catModel := agent.ModelForCategory(agent.TaskCategory(category), tier)
		if catProvider != "" && a.hasAPIKey(catProvider) {
			var p llm.Provider
			if catModel != "" {
				p = llm.NewRetryProvider(llm.NewProviderWithModel(catProvider, &a.cfg, catModel), 2)
			} else {
				p = llm.NewRetryProvider(llm.NewProvider(catProvider, &a.cfg), 2)
			}
			// Add role-based fallback
			fbName := a.cfg.FallbackProviderForRole(role)
			if fbName != "" && fbName != catProvider && a.hasAPIKey(fbName) {
				fallback := llm.NewRetryProvider(llm.NewProvider(fbName, &a.cfg), 2)
				return llm.NewFallbackProvider(p, fallback)
			}
			return p
		}
	}
	// No category or category provider unavailable — fall back to role-based
	return a.buildProviderWithFallback(role)
}

// hasAPIKey checks if a provider has an API key configured.
func (a *App) hasAPIKey(providerName string) bool {
	if providerName == "glm" {
		return a.cfg.GetGLM().APIKey != ""
	}
	prov := a.cfg.GetProvider(providerName)
	return prov != nil && prov.APIKey != ""
}

// buildHistorySummary converts the conversation history into a list of
// "role: content" strings for injection into agent session state.
// Excludes the current (last) user message since it's already in the artifact.
func (a *App) buildHistorySummary() []string {
	if len(a.history) <= 1 {
		return nil // no prior turns to share
	}
	// Exclude the last message (current user request — already an artifact)
	prior := a.history[:len(a.history)-1]
	lines := make([]string, 0, len(prior))
	for _, m := range prior {
		lines = append(lines, fmt.Sprintf("%s: %s", m.Role, m.Content))
	}
	return lines
}

// savePipelineRun persists a new pipeline run record (best-effort, async).
func (a *App) savePipelineRun(runID, planJSON, intent string) {
	if a.memStore == nil {
		return
	}
	run := &memory.PipelineRun{
		ID:          runID,
		SessionID:   a.sessionID,
		ParentRunID: "",
		Intent:      intent,
		PlanJSON:    planJSON,
		Status:      "running",
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.memStore.SavePipelineRun(ctx, run)
	}()
}

// savePipelineRunWithPlan persists a pipeline run with the full plan JSON.
func (a *App) savePipelineRunWithPlan(runID string, plan *orchestrator.ExecutionPlan, intent string) {
	var planJSON string
	if plan != nil {
		if b, err := json.Marshal(plan); err == nil {
			planJSON = string(b)
		}
	}
	a.savePipelineRun(runID, planJSON, intent)
}

// executeResume reconstructs an execution plan from a stored incomplete run and
// resumes from the last completed step using Engine.RunPlanFromStep().
func (a App) executeResume(run state.IncompleteRun) (tea.Model, tea.Cmd) {
	// Parse the stored plan JSON
	plan, err := orchestrator.ParsePlan(run.PlanJSON)
	if err != nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Failed to parse stored plan: %v", err),
		})
		return a, nil
	}

	startStep := run.LastStepIndex + 1 // resume after last completed step
	if startStep >= len(plan.Steps) {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "All steps were already completed. Nothing to resume.",
		})
		// Clean up checkpoints
		if a.checkpointStore != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = a.checkpointStore.DeleteCheckpoints(ctx, run.RunID)
			}()
		}
		return a, nil
	}

	// Switch to split layout for pipeline execution
	a.layoutMode = LayoutSplit
	a.recalcLayout()
	a.pipelineRunning = true
	a.activePipelineRunID = run.RunID

	a.chat.AddMessage(ChatMessage{
		Role:    RoleSystem,
		Content: fmt.Sprintf("Resuming pipeline from step %d/%d (%s)...", startStep+1, len(plan.Steps), plan.Reasoning),
	})

	eb := bus.NewEventBus(eventBusBuffer)
	a.eventBus = eb

	ctx, cancel := context.WithCancel(context.Background()) // no timeout
	a.cancelPipeline = cancel

	// Restore session state from checkpoints
	ss := state.NewSessionStateWithID(run.RunID, run.SessionID, "")
	ss.SetHistory(a.buildHistorySummary())
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactOrchestratorPlan,
		Source:  "orchestrator",
		Content: plan.Reasoning,
	})

	// Checkpoints are loaded internally by Engine.RunPlanFromStep via checkpointStore

	// Agent builder (reuses same pattern as executePlan)
	buildAgent := func(task orchestrator.AgentTask) agent.Agent {
		provider := a.buildProviderForTask(task.Agent, task.Category)
		if provider == nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, task.Agent, "plan", "skipped: no API key configured"))
			return nil
		}
		ag := roles.NewRoleAgent(agent.Role(task.Agent), provider, eb, a.toolExecutor)
		if a.memStore != nil {
			ag.SetMemory(a.memStore)
		}
		if a.repoMapStore != nil {
			ag.SetRepoMap(a.repoMapStore)
		}
		ag.SetMaxToolIter(a.cfg.MaxToolIter)
		ag.SetTask(task.Task)
		ag.SetCritical(task.Critical)
		if a.projectRules != "" {
			ag.SetProjectRules(a.projectRules)
		}
		if a.codeIndex != nil {
			ag.SetCodeIndex(a.codeIndex)
		}
		if task.Category != "" {
			ag.SetCategory(agent.TaskCategory(task.Category))
		}
		if len(task.Skills) > 0 && a.skillRegistry != nil {
			ag.SetSkills(a.skillRegistry.Resolve(task.Skills))
		}
		if a.historyWindow != nil {
			ag.SetHistoryWindow(a.historyWindow)
		}
		// Phase E-2: Autonomous mode
		if task.Autonomous {
			cwd, _ := os.Getwd()
			verifyFn := agent.ResolveVerifyFunc(task.VerifyWith, cwd)
			ag.SetAutonomous(verifyFn, task.MaxRetries)
		}
		return ag
	}

	// Create engine with recovery support
	bridge := NewRecoveryBridge()
	a.recoveryBridge = bridge
	engine := orchestrator.NewEngine(nil, eb, bridge, buildAgent)
	if a.checkpointStore != nil {
		engine.SetCheckpointStore(a.checkpointStore)
	}

	a.activity.AddActivity(ActivityItem{
		Status: StatusRunning,
		Text:   fmt.Sprintf("Resuming from step %d/%d...", startStep+1, len(plan.Steps)),
	})
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	a.activity.SetAgentCount(plan.TotalTasks())

	memStore := a.memStore
	capturedRunID := run.RunID
	go func() {
		result := engine.RunPlanFromStep(ctx, plan, ss, buildAgent, startStep)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "resume", result.HaltError.Error()))
		}

		// Update pipeline run status
		if memStore != nil {
			status := "completed"
			if !result.Completed {
				status = "failed"
			}
			_ = memStore.UpdatePipelineRun(context.Background(), capturedRunID, status)
		}

		eb.Close()
	}()

	return a, tea.Batch(a.waitForEvent(eb), waitForRecoveryRequest(bridge))
}
