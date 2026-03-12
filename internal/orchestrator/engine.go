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

// EngineResult captures the outcome of the full pipeline run.
type EngineResult struct {
	PhaseResults []PhaseResult
	Completed    bool // true if all phases finished
	HaltedAt     PhaseName
	HaltError    error // error that caused halt
}

// Engine executes a Pipeline against a SessionState.
// Phases run sequentially; agents within each phase run in parallel.
type Engine struct {
	pipeline *Pipeline
	eventBus *bus.EventBus
}

// NewEngine creates a new pipeline execution engine.
func NewEngine(pipeline *Pipeline, eb *bus.EventBus) *Engine {
	return &Engine{
		pipeline: pipeline,
		eventBus: eb,
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

		// Check for critical failures
		if phaseResult.HasCriticalFailure(&phase) {
			result.HaltedAt = phase.Name
			result.HaltError = e.buildHaltError(phaseResult, &phase)
			return result
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

		// Check for critical failures
		if phaseResult.HasCriticalFailure(phase) {
			result.HaltedAt = stepName
			result.HaltError = e.buildHaltError(phaseResult, phase)
			return result
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
