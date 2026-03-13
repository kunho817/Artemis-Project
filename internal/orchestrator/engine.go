package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/state"
)

// PhaseResult captures the outcome of a single phase execution.
type PhaseResult struct {
	Phase  PhaseName
	Errors map[string]error // agent name → error (only failed agents)
}

// HasCriticalFailure returns true if any critical agent failed.
func (pr *PhaseResult) HasCriticalFailure(phase *Phase) bool {
	for _, a := range phase.CriticalAgents() {
		if _, failed := pr.Errors[a.Name()]; failed {
			return true
		}
	}
	return false
}

// failedCriticalAgents returns the names and errors of critical agents that failed.
func (pr *PhaseResult) failedCriticalAgents(phase *Phase) map[string]error {
	failed := make(map[string]error)
	for _, a := range phase.CriticalAgents() {
		if err, ok := pr.Errors[a.Name()]; ok {
			failed[a.Name()] = err
		}
	}
	return failed
}

// EngineResult captures the outcome of the full pipeline run.
type EngineResult struct {
	PhaseResults []PhaseResult
	Completed    bool // true if all phases finished
	HaltedAt     PhaseName
	HaltError    error // error that caused halt
}

// Engine executes a Pipeline against a SessionState.
// Phases run sequentially; agents within each phase run in parallel.
// Supports 3-stage failure recovery: retry → Consultant diagnosis → user escalation.
type Engine struct {
	pipeline          *Pipeline
	eventBus          *bus.EventBus
	recoveryPrompter  RecoveryPrompter // Stage 3: user escalation (nil = disabled)
	consultantBuilder AgentBuilder     // Stage 2: Consultant agent factory (nil = skip to Stage 3)
}

// NewEngine creates a new pipeline execution engine.
// recoveryPrompter and consultantBuilder may be nil to disable recovery.
func NewEngine(pipeline *Pipeline, eb *bus.EventBus, prompter RecoveryPrompter, consultant AgentBuilder) *Engine {
	return &Engine{
		pipeline:          pipeline,
		eventBus:          eb,
		recoveryPrompter:  prompter,
		consultantBuilder: consultant,
	}
}

// Run executes the entire pipeline. It blocks until all phases complete
// or a critical agent failure halts the pipeline.
// The context can be used to cancel the entire run.
func (e *Engine) Run(ctx context.Context, ss *state.SessionState) EngineResult {
	result := EngineResult{
		PhaseResults: make([]PhaseResult, 0, len(e.pipeline.Phases)),
	}

	for _, phase := range e.pipeline.Phases {
		// Check context cancellation before each phase
		if ctx.Err() != nil {
			result.HaltedAt = phase.Name
			result.HaltError = ctx.Err()
			return result
		}

		// Update shared state with current phase
		ss.SetPhase(string(phase.Name))

		// Emit phase start event
		e.emitPhaseStart(phase.Name)

		// Execute all agents in this phase in parallel
		phaseResult := e.runPhase(ctx, &phase, ss)
		result.PhaseResults = append(result.PhaseResults, phaseResult)

		// Emit phase complete event
		e.emitPhaseComplete(phase.Name, len(phaseResult.Errors))

		// Emit warnings for non-critical failures
		e.emitNonCriticalWarnings(phaseResult, &phase)

		// Check for critical failures — attempt recovery if available
		if phaseResult.HasCriticalFailure(&phase) {
			action := e.attemptRecovery(ctx, &phase, ss, phaseResult, nil)
			switch action {
			case ActionRetry:
				// Recovery succeeded — re-run the failed agents
				retryResult := e.retryFailedAgents(ctx, &phase, ss, phaseResult, nil)
				// Replace last phase result with retry result
				result.PhaseResults[len(result.PhaseResults)-1] = retryResult
				if retryResult.HasCriticalFailure(&phase) {
					// Retry also failed — halt
					result.HaltedAt = phase.Name
					result.HaltError = e.buildHaltError(retryResult, &phase)
					return result
				}
				continue // Retry succeeded, continue pipeline
			case ActionSkip:
				// User chose to skip — continue pipeline without halting
				continue
			default: // ActionAbort or recovery disabled
				result.HaltedAt = phase.Name
				result.HaltError = e.buildHaltError(phaseResult, &phase)
				return result
			}
		}
	}

	result.Completed = true

	// Emit pipeline done event
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventPipelineDone, "engine", "", "Pipeline complete"))
	}

	return result
}

// runPhase executes all agents within a phase in parallel using errgroup.
// Non-critical agent failures are collected but don't halt execution.
// Critical agent failures cancel sibling agents in the same phase.
func (e *Engine) runPhase(ctx context.Context, phase *Phase, ss *state.SessionState) PhaseResult {
	pr := PhaseResult{
		Phase:  phase.Name,
		Errors: make(map[string]error),
	}

	if len(phase.Agents) == 0 {
		return pr
	}

	// Use errgroup with context for cancellation on critical failure
	g, gctx := errgroup.WithContext(ctx)

	// Mutex to protect concurrent writes to pr.Errors
	var mu sync.Mutex

	// Track if a critical agent has failed (to cancel siblings)
	var criticalFailed bool
	var criticalMu sync.Mutex

	for _, a := range phase.Agents {
		a := a // capture loop variable

		g.Go(func() error {
			// Check if a critical sibling already failed
			criticalMu.Lock()
			failed := criticalFailed
			criticalMu.Unlock()
			if failed {
				return nil // skip, phase is already doomed
			}

			err := a.Run(gctx, ss)
			if err != nil {
				mu.Lock()
				pr.Errors[a.Name()] = err
				mu.Unlock()

				if a.Critical() {
					criticalMu.Lock()
					criticalFailed = true
					criticalMu.Unlock()
					// Return error to cancel errgroup context
					return fmt.Errorf("critical agent %s failed: %w", a.Name(), err)
				}
				// Non-critical: log but don't halt
			}
			return nil
		})
	}

	// Wait for all goroutines. The error from errgroup is the first
	// non-nil return, which we only return for critical failures.
	_ = g.Wait()

	return pr
}

// attemptRecovery orchestrates the 3-stage failure recovery for critical agent failures.
// Stage 2: Invoke Consultant for diagnosis + re-run failed agent with correction.
// Stage 3: Prompt user for decision (Retry/Skip/Abort).
// Returns the recovery action taken. Returns ActionAbort if recovery is disabled.
func (e *Engine) attemptRecovery(ctx context.Context, phase *Phase, ss *state.SessionState, pr PhaseResult, buildAgent AgentBuilder) RecoveryAction {
	// No recovery system configured — abort immediately
	if e.recoveryPrompter == nil {
		return ActionAbort
	}

	failedAgents := pr.failedCriticalAgents(phase)
	if len(failedAgents) == 0 {
		return ActionAbort
	}

	// Pick the first failed critical agent for recovery
	var failedName string
	var failedErr error
	var failedRole string
	var failedTask string
	for _, a := range phase.CriticalAgents() {
		if err, ok := failedAgents[a.Name()]; ok {
			failedName = a.Name()
			failedErr = err
			failedRole = string(a.Role())
			if override := a.OverrideTask(); override != "" {
				failedTask = override
			}
			break
		}
	}

	for attempt := 0; attempt < MaxRecoveryAttempts; attempt++ {
		// --- Stage 2: Consultant Diagnosis ---
		diagnosis, suggestion := e.consultAgent(ctx, ss, failedName, failedRole, failedTask, failedErr, attempt)

		// If Consultant provided a suggestion and this is an automatic recovery attempt,
		// try re-running the failed agent (handled by the caller via ActionRetry).
		if suggestion != "" && attempt == 0 {
			e.emitRecoveryAttempt(failedName, fmt.Sprintf("Consultant suggested fix, retrying agent (attempt %d)", attempt+1))
			return ActionRetry
		}

		// --- Stage 3: User Escalation ---
		rc := RecoveryContext{
			FailedAgent:  failedName,
			FailedRole:   failedRole,
			Task:         failedTask,
			Error:        failedErr,
			Diagnosis:    diagnosis,
			Suggestion:   suggestion,
			AttemptCount: attempt + 1,
		}

		e.emitRecoveryAttempt(failedName, "Escalating to user for decision")

		action, err := e.recoveryPrompter.Prompt(ctx, rc)
		if err != nil {
			// Context cancelled or app shutdown — abort
			return ActionAbort
		}

		switch action {
		case ActionSkip:
			return ActionSkip
		case ActionAbort:
			return ActionAbort
		case ActionRetry:
			// User chose retry — loop back to Stage 2 (Consultant)
			continue
		}
	}

	// Max recovery attempts exhausted — abort
	return ActionAbort
}

// consultAgent invokes the Consultant agent to diagnose a failure.
// Returns diagnosis and suggestion strings. Both empty if Consultant is unavailable or fails.
func (e *Engine) consultAgent(ctx context.Context, ss *state.SessionState, agentName, agentRole, task string, agentErr error, attempt int) (diagnosis, suggestion string) {
	if e.consultantBuilder == nil {
		return "", ""
	}

	e.emitRecoveryAttempt(agentName, fmt.Sprintf("Consulting diagnosis agent (attempt %d)", attempt+1))

	// Build a diagnostic task for the Consultant
	diagnosticTask := AgentTask{
		Agent: "consultant",
		Task: fmt.Sprintf(
			"DIAGNOSTIC REQUEST: Agent '%s' (role: %s) failed with error:\n%s\n\n"+
				"The agent was working on:\n%s\n\n"+
				"Previous context:\n%s\n\n"+
				"Analyze the failure. Provide:\n"+
				"1. DIAGNOSIS: Why did the agent fail? (one paragraph)\n"+
				"2. SUGGESTION: A specific correction or alternative approach for the agent to try.\n"+
				"Keep your response concise and actionable.",
			agentName, agentRole, agentErr.Error(), task, ss.Summary(),
		),
		Critical: false,
	}

	consultant := e.consultantBuilder(diagnosticTask)
	if consultant == nil {
		return "", ""
	}

	// Create isolated state for Consultant
	consultSS := state.NewSessionState()
	consultSS.SetPhase("recovery")

	// Run Consultant (with retry on first failure)
	err := consultant.Run(ctx, consultSS)
	if err != nil {
		// Retry Consultant once
		e.emitRecoveryAttempt(agentName, "Consultant failed, retrying once...")
		err = consultant.Run(ctx, consultSS)
		if err != nil {
			return "", ""
		}
	}

	// Extract Consultant's output from artifacts
	artifacts := consultSS.GetByType(state.ArtifactConsultation)
	if len(artifacts) == 0 {
		return "", ""
	}

	output := artifacts[len(artifacts)-1].Content
	// Simple heuristic: split on "SUGGESTION:" to separate diagnosis from suggestion
	diagnosis = output
	if idx := findSuggestionIdx(output); idx >= 0 {
		diagnosis = output[:idx]
		suggestion = output[idx:]
	}

	return diagnosis, suggestion
}

// findSuggestionIdx finds the index of the suggestion section in Consultant output.
func findSuggestionIdx(output string) int {
	markers := []string{"SUGGESTION:", "2. SUGGESTION:", "**SUGGESTION**:", "## Suggestion"}
	for _, marker := range markers {
		for i := 0; i+len(marker) <= len(output); i++ {
			if output[i:i+len(marker)] == marker {
				return i
			}
		}
	}
	return -1
}

// retryFailedAgents re-runs only the critical agents that failed in the original phase.
func (e *Engine) retryFailedAgents(ctx context.Context, phase *Phase, ss *state.SessionState, pr PhaseResult, buildAgent AgentBuilder) PhaseResult {
	retryPR := PhaseResult{
		Phase:  pr.Phase,
		Errors: make(map[string]error),
	}

	for _, a := range phase.CriticalAgents() {
		if _, failed := pr.Errors[a.Name()]; !failed {
			continue // only retry failed agents
		}

		e.emitRecoveryAttempt(a.Name(), "Retrying failed agent...")

		err := a.Run(ctx, ss)
		if err != nil {
			retryPR.Errors[a.Name()] = err
		}
	}

	return retryPR
}

// emitNonCriticalWarnings emits warning events for non-critical agent failures.
func (e *Engine) emitNonCriticalWarnings(pr PhaseResult, phase *Phase) {
	if e.eventBus == nil {
		return
	}
	criticals := make(map[string]bool)
	for _, a := range phase.CriticalAgents() {
		criticals[a.Name()] = true
	}
	for name, err := range pr.Errors {
		if !criticals[name] {
			e.eventBus.Emit(bus.AgentEvent{
				Type:      bus.EventAgentWarn,
				AgentName: name,
				Phase:     string(pr.Phase),
				Message:   fmt.Sprintf("Non-critical failure: %s", err.Error()),
				Error:     err,
			})
		}
	}
}

// emitRecoveryAttempt emits a recovery attempt event for Activity panel display.
func (e *Engine) emitRecoveryAttempt(agentName, msg string) {
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventRecoveryAttempt, agentName, "recovery", msg))
	}
}

// buildHaltError constructs a descriptive error for pipeline halt.
func (e *Engine) buildHaltError(pr PhaseResult, phase *Phase) error {
	for _, a := range phase.CriticalAgents() {
		if err, ok := pr.Errors[a.Name()]; ok {
			return fmt.Errorf("pipeline halted at phase %q: critical agent %s failed: %w",
				pr.Phase, a.Name(), err)
		}
	}
	return fmt.Errorf("pipeline halted at phase %q: unknown critical failure", pr.Phase)
}

// emitPhaseStart sends a phase start event.
func (e *Engine) emitPhaseStart(phase PhaseName) {
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventPhaseStart, "engine", string(phase), "Phase started"))
	}
}

// emitPhaseComplete sends a phase complete event.
func (e *Engine) emitPhaseComplete(phase PhaseName, errorCount int) {
	msg := "Phase complete"
	if errorCount > 0 {
		msg = fmt.Sprintf("Phase complete with %d error(s)", errorCount)
	}
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventPhaseComplete, "engine", string(phase), msg))
	}
}

// AgentBuilder is a factory function that creates a fully-configured agent for a task.
// The builder receives the full AgentTask (role, task description, criticality, category, skills)
// and returns an agent ready to run. Provider selection, category overrides, and skill injection
// are handled by the builder implementation.
type AgentBuilder func(task AgentTask) agent.Agent

// RunPlan executes a dynamic ExecutionPlan created by the Orchestrator.
// Steps run sequentially; tasks within each step run in parallel.
// The buildAgent function creates fully-configured agents from AgentTask definitions.
func (e *Engine) RunPlan(ctx context.Context, plan *ExecutionPlan, ss *state.SessionState, buildAgent AgentBuilder) EngineResult {
	result := EngineResult{
		PhaseResults: make([]PhaseResult, 0, len(plan.Steps)),
	}

	for stepIdx, step := range plan.Steps {
		// Check context cancellation before each step
		if ctx.Err() != nil {
			result.HaltedAt = PhaseName(fmt.Sprintf("step-%d", stepIdx+1))
			result.HaltError = ctx.Err()
			return result
		}

		stepName := PhaseName(fmt.Sprintf("step-%d", stepIdx+1))
		ss.SetPhase(string(stepName))

		// Emit step start event
		e.emitStepStart(stepIdx+1, len(step.Tasks))

		// Build agents for this step from the plan's tasks
		var agents []agent.Agent
		for _, task := range step.Tasks {
			a := buildAgent(task)
			if a == nil {
				continue
			}
			agents = append(agents, a)
		}

		if len(agents) == 0 {
			e.emitStepComplete(stepIdx+1, 0)
			continue
		}

		// Reuse the existing parallel execution logic via Phase
		phase := &Phase{
			Name:   stepName,
			Agents: agents,
		}

		phaseResult := e.runPhase(ctx, phase, ss)
		result.PhaseResults = append(result.PhaseResults, phaseResult)

		e.emitStepComplete(stepIdx+1, len(phaseResult.Errors))

		// Emit warnings for non-critical failures
		e.emitNonCriticalWarnings(phaseResult, phase)

		// Check for critical failures — attempt recovery if available
		if phaseResult.HasCriticalFailure(phase) {
			action := e.attemptRecovery(ctx, phase, ss, phaseResult, buildAgent)
			switch action {
			case ActionRetry:
				// Recovery succeeded — re-run the failed agents
				retryResult := e.retryFailedAgents(ctx, phase, ss, phaseResult, buildAgent)
				result.PhaseResults[len(result.PhaseResults)-1] = retryResult
				if retryResult.HasCriticalFailure(phase) {
					result.HaltedAt = stepName
					result.HaltError = e.buildHaltError(retryResult, phase)
					return result
				}
				continue
			case ActionSkip:
				continue
			default: // ActionAbort or recovery disabled
				result.HaltedAt = stepName
				result.HaltError = e.buildHaltError(phaseResult, phase)
				return result
			}
		}
	}

	result.Completed = true

	// Emit pipeline done event
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventPipelineDone, "engine", "", "Plan execution complete"))
	}

	return result
}

// emitStepStart sends a step start event with task count.
func (e *Engine) emitStepStart(stepNum, taskCount int) {
	if e.eventBus != nil {
		msg := fmt.Sprintf("Step %d started (%d tasks)", stepNum, taskCount)
		e.eventBus.Emit(bus.NewEvent(bus.EventStepStart, "engine", fmt.Sprintf("step-%d", stepNum), msg))
	}
}

// emitStepComplete sends a step complete event.
func (e *Engine) emitStepComplete(stepNum, errorCount int) {
	msg := fmt.Sprintf("Step %d complete", stepNum)
	if errorCount > 0 {
		msg = fmt.Sprintf("Step %d complete with %d error(s)", stepNum, errorCount)
	}
	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventStepComplete, "engine", fmt.Sprintf("step-%d", stepNum), msg))
	}
}
