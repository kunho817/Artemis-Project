package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/agent/roles"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/llm"
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
	history = append(history, llm.Message{Role: "system", Content: roles.OrchestratorPrompt})
	history = append(history, a.history...)
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
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
		return a.executeTrivial(resp.DirectAgent, resp.DirectTask)

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
	// pipelineRunning already set in handleOrchestratedSubmit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.cancelPipeline = cancel

	// Prepare session state with user request + conversation history
	ss := state.NewSessionState()
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

	// Agent builder: creates agents on demand from the plan
	buildAgent := func(role string) agent.Agent {
		provider := a.buildProviderWithFallback(role)
		if provider == nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, role, "plan", "skipped: no API key configured"))
			return nil
		}
		ag := roles.NewRoleAgent(agent.Role(role), provider, eb, a.toolExecutor)
		if a.memStore != nil {
			ag.SetMemory(a.memStore)
		}
		if a.repoMapStore != nil {
			ag.SetRepoMap(a.repoMapStore)
		}
		ag.SetMaxToolIter(a.cfg.MaxToolIter)
		return ag
	}

	// Create engine (nil pipeline — using RunPlan instead)
	engine := orchestrator.NewEngine(nil, eb)

	// Create background task manager for parallel background tasks
	bgMgr := orchestrator.NewBackgroundTaskManager(eb)

	// Spawn background tasks (run parallel to main pipeline)
	for _, bgDef := range plan.BackgroundTasks {
		bgMgr.Spawn(ctx, bgDef, buildAgent, userText)
	}

	go func() {
		result := engine.RunPlan(ctx, plan, ss, buildAgent)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "plan", result.HaltError.Error()))
		}
		// Wait for all background tasks before closing EventBus
		bgMgr.WaitAll()
		eb.Close()
	}()

	return a, a.waitForEvent(eb)
}

// executeTrivial handles trivial intent — direct LLM streaming without Engine or tools.
// Stays in single layout for a lightweight, fast response path.
func (a App) executeTrivial(agentRole, task string) (tea.Model, tea.Cmd) {
	provider := a.buildProviderWithFallback(agentRole)
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
	messages := make([]llm.Message, 0, len(a.history)+1)
	if systemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, a.history...)

	// Start streaming (reuses single-mode streaming path)
	a.chat.AddMessage(ChatMessage{Role: RoleAssistant, Content: ""})
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.cancelPipeline = cancel

	ss := state.NewSessionState()
	ss.SetHistory(a.buildHistorySummary())
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactUserRequest,
		Source:  "user",
		Content: text,
	})

	engine := orchestrator.NewEngine(pipeline, eb)

	go func() {
		result := engine.Run(ctx, ss)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "pipeline", result.HaltError.Error()))
		}
		eb.Close()
	}()

	return a, a.waitForEvent(eb)
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
