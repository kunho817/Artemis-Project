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

// OrchestratorPlanMsg carries the Orchestrator's routing plan back to the Update loop.
type OrchestratorPlanMsg struct {
	Plan     *orchestrator.ExecutionPlan
	UserText string // original user input for context + fallback
	Error    error
}

// handleOrchestratedSubmit sends the user message to the Orchestrator first.
// The Orchestrator analyzes intent and creates an execution plan specifying
// which agents to invoke, with what tasks, in what order.
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

	// Show orchestrator activity
	a.activity.AddActivity(ActivityItem{
		Status: StatusRunning,
		Text:   "Orchestrator: analyzing request...",
	})
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	a.activity.SetAgentCount(1)
	a.pipelineRunning = true
	a.layoutMode = LayoutSplit
	a.recalcLayout()

	// Fire async Orchestrator LLM call
	userText := text
	// Build messages: system prompt + full conversation history
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
		plan, parseErr := orchestrator.ParsePlan(resp)
		return OrchestratorPlanMsg{Plan: plan, UserText: userText, Error: parseErr}
	}

	return a, cmd
}

// handleOrchestratorPlan processes the Orchestrator's routing decision.
// On success, it executes the dynamic plan. On failure, it falls back to the legacy pipeline.
func (a App) handleOrchestratorPlan(msg OrchestratorPlanMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		// Orchestrator failed — fall back to legacy fixed pipeline
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fmt.Sprintf("Orchestrator: %s (falling back to pipeline)", msg.Error),
		})
		return a.executeLegacyPipeline(msg.UserText)
	}

	plan := msg.Plan

	// Show plan summary in activity
	a.activity.AddActivity(ActivityItem{
		Status: StatusDone,
		Text:   fmt.Sprintf("Orchestrator: plan ready (%d steps, %d agents)", len(plan.Steps), plan.TotalTasks()),
	})
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	a.activity.SetAgentCount(plan.TotalTasks())

	// Show orchestrator reasoning in chat
	if plan.Reasoning != "" {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: plan.Reasoning,
		})
	}

	// Execute the dynamic plan
	return a.executePlan(plan, msg.UserText)
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

	go func() {
		result := engine.RunPlan(ctx, plan, ss, buildAgent)
		if !result.Completed && result.HaltError != nil {
			eb.Emit(bus.NewEvent(bus.EventAgentFail, "engine", "plan", result.HaltError.Error()))
		}
		eb.Close()
	}()

	return a, a.waitForEvent(eb)
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

	// Build primary with retry (nil if no API key)
	var primary llm.Provider
	if a.hasAPIKey(primaryName) {
		primary = llm.NewRetryProvider(llm.NewProvider(primaryName, &a.cfg), 2)
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
