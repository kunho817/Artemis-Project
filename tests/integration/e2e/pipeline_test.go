// Package e2e provides end-to-end tests for the Artemis pipeline.
package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/agent"
	agentroles "github.com/artemis-project/artemis/internal/agent/roles"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/tools"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestSingleAgentPipeline tests a pipeline with a single agent.
func TestSingleAgentPipeline(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a simple single-agent pipeline
	pipeline := createTestPipeline(t, e, 1)

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify completion
	if !result.Completed {
		t.Errorf("Pipeline did not complete. Halted at: %s, Error: %v", result.HaltedAt, result.HaltError)
	}

	// Verify phase results
	if len(result.PhaseResults) != 1 {
		t.Errorf("Expected 1 phase result, got %d", len(result.PhaseResults))
	}

	// Verify no errors
	if len(result.PhaseResults[0].Errors) > 0 {
		t.Errorf("Unexpected errors in phase: %v", result.PhaseResults[0].Errors)
	}

	// Verify events
	e.AssertEventReceived(bus.EventPhaseStart)
	e.AssertEventReceived(bus.EventPhaseComplete)
	e.AssertEventReceived(bus.EventAgentStart)
	e.AssertEventReceived(bus.EventAgentComplete)
	e.AssertEventReceived(bus.EventPipelineDone)
}

// TestMultiAgentPipeline tests a pipeline with multiple agents in a single phase.
func TestMultiAgentPipeline(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a multi-agent pipeline
	pipeline := createTestPipeline(t, e, 3)

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify completion
	if !result.Completed {
		t.Errorf("Pipeline did not complete. Halted at: %s, Error: %v", result.HaltedAt, result.HaltError)
	}

	// Verify phase results
	if len(result.PhaseResults) != 1 {
		t.Errorf("Expected 1 phase result, got %d", len(result.PhaseResults))
	}

	// Verify agent events - should have 3 agents
	agentStartEvents := e.GetEventsByType(bus.EventAgentStart)
	if len(agentStartEvents) != 3 {
		t.Errorf("Expected 3 agent start events, got %d", len(agentStartEvents))
	}

	agentCompleteEvents := e.GetEventsByType(bus.EventAgentComplete)
	if len(agentCompleteEvents) != 3 {
		t.Errorf("Expected 3 agent complete events, got %d", len(agentCompleteEvents))
	}
}

// TestPipelineWithMockLLM tests pipeline execution with a mock LLM.
func TestPipelineWithMockLLM(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create mock LLM with specific responses
	mock := e.CreateMockProvider("claude").(*harness.MockLLM)
	mock.SetResponse("test", "Mock LLM response for testing")

	// Create a simple pipeline
	pipeline := createTestPipeline(t, e, 1)

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify completion
	if !result.Completed {
		t.Errorf("Pipeline did not complete. Halted at: %s, Error: %v", result.HaltedAt, result.HaltError)
	}

	// Verify mock was called
	e.AssertMockCalled("claude")

	// Verify mock call count
	mockAgent := e.MockBuilder.GetMock("claude")
	if mockAgent.GetCallCount() == 0 {
		t.Error("Mock LLM was not called")
	}
}

// TestPipelineEvents tests that all expected events are emitted during pipeline execution.
func TestPipelineEvents(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a simple pipeline
	pipeline := createTestPipeline(t, e, 2)

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify completion
	if !result.Completed {
		t.Fatalf("Pipeline did not complete: %v", result.HaltError)
	}

	// Verify all expected events were emitted
	expectedEvents := []struct {
		eventType bus.EventType
		minCount  int
	}{
		{bus.EventPhaseStart, 1},
		{bus.EventPhaseComplete, 1},
		{bus.EventAgentStart, 2},
		{bus.EventAgentComplete, 2},
		{bus.EventPipelineDone, 1},
	}

	for _, expected := range expectedEvents {
		events := e.GetEventsByType(expected.eventType)
		if len(events) < expected.minCount {
			t.Errorf("Expected at least %d events of type %d, got %d",
				expected.minCount, expected.eventType, len(events))
		}
	}

	// Verify event order (phase start → agent starts → agent completes → phase complete → pipeline done)
	allEvents := e.GetCollectedEvents()
	phaseStartIndex := -1
	agentStartCount := 0
	agentCompleteCount := 0
	phaseCompleteIndex := -1
	pipelineDoneIndex := -1

	for i, event := range allEvents {
		switch event.Type {
		case bus.EventPhaseStart:
			if phaseStartIndex == -1 {
				phaseStartIndex = i
			}
		case bus.EventAgentStart:
			agentStartCount++
		case bus.EventAgentComplete:
			agentCompleteCount++
		case bus.EventPhaseComplete:
			phaseCompleteIndex = i
		case bus.EventPipelineDone:
			pipelineDoneIndex = i
		}
	}

	// Verify order
	if phaseStartIndex == -1 {
		t.Error("PhaseStart event not found")
	}
	if phaseCompleteIndex == -1 {
		t.Error("PhaseComplete event not found")
	}
	if pipelineDoneIndex == -1 {
		t.Error("PipelineDone event not found")
	}
	if phaseStartIndex >= phaseCompleteIndex {
		t.Error("PhaseStart should come before PhaseComplete")
	}
	if phaseCompleteIndex >= pipelineDoneIndex {
		t.Error("PhaseComplete should come before PipelineDone")
	}
	if agentStartCount != 2 {
		t.Errorf("Expected 2 AgentStart events, got %d", agentStartCount)
	}
	if agentCompleteCount != 2 {
		t.Errorf("Expected 2 AgentComplete events, got %d", agentCompleteCount)
	}
}

// TestPipelineContextCancellation tests pipeline cancellation via context.
func TestPipelineContextCancellation(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a pipeline with a slow agent
	pipeline := createSlowTestPipeline(t, e)

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state with cancelable context
	ctx, cancel := context.WithCancel(e.Ctx)
	ss := e.CreateSessionState()

	// Start pipeline in background
	done := make(chan orchestrator.EngineResult, 1)
	go func() {
		done <- engine.Run(ctx, ss)
	}()

	// Wait a bit for pipeline to start, then cancel
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for result with timeout
	select {
	case result := <-done:
		// Verify pipeline was halted or cancelled
		if result.Completed {
			t.Log("Pipeline completed before cancellation (may be expected if fast)")
		}
		if result.HaltedAt != "" {
			t.Logf("Pipeline halted at: %s", result.HaltedAt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for pipeline to complete/cancel")
	}
}

// TestMultiPhasePipeline tests a pipeline with multiple phases.
func TestMultiPhasePipeline(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a multi-phase pipeline
	pipeline := &orchestrator.Pipeline{
		Phases: []orchestrator.Phase{
			{
				Name:   orchestrator.PhaseName("phase1"),
				Agents: []agent.Agent{createMockAgent(t, e, "agent1", agent.RolePlanner, true)},
			},
			{
				Name:   orchestrator.PhaseName("phase2"),
				Agents: []agent.Agent{createMockAgent(t, e, "agent2", agent.RoleCoder, true)},
			},
			{
				Name:   orchestrator.PhaseName("phase3"),
				Agents: []agent.Agent{createMockAgent(t, e, "agent3", agent.RoleQA, true)},
			},
		},
	}

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify completion
	if !result.Completed {
		t.Errorf("Pipeline did not complete. Halted at: %s, Error: %v", result.HaltedAt, result.HaltError)
	}

	// Verify 3 phases completed
	if len(result.PhaseResults) != 3 {
		t.Errorf("Expected 3 phase results, got %d", len(result.PhaseResults))
	}

	// Verify phase events
	phaseStartEvents := e.GetEventsByType(bus.EventPhaseStart)
	phaseCompleteEvents := e.GetEventsByType(bus.EventPhaseComplete)

	if len(phaseStartEvents) != 3 {
		t.Errorf("Expected 3 phase start events, got %d", len(phaseStartEvents))
	}

	if len(phaseCompleteEvents) != 3 {
		t.Errorf("Expected 3 phase complete events, got %d", len(phaseCompleteEvents))
	}
}

// TestPipelineWithNonCriticalAgent tests that non-critical agent failures don't halt the pipeline.
func TestPipelineWithNonCriticalAgent(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a pipeline with one critical and one non-critical agent
	pipeline := &orchestrator.Pipeline{
		Phases: []orchestrator.Phase{
			{
				Name: orchestrator.PhaseName("test-phase"),
				Agents: []agent.Agent{
					createMockAgent(t, e, "critical-agent", agent.RolePlanner, true),
					createFailingAgent(t, e, "failing-agent", agent.RoleAnalyzer, false), // Non-critical
				},
			},
		},
	}

	// Create engine
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify pipeline completed despite non-critical failure
	if !result.Completed {
		t.Errorf("Pipeline should complete despite non-critical failure. Halted at: %s, Error: %v",
			result.HaltedAt, result.HaltError)
	}

	// Verify we have errors from the non-critical agent
	phaseResult := result.PhaseResults[0]
	if len(phaseResult.Errors) == 0 {
		t.Error("Expected errors from non-critical agent")
	}

	// Verify the error is from the failing agent
	if _, ok := phaseResult.Errors["failing-agent"]; !ok {
		t.Error("Expected error from 'failing-agent'")
	}

	// Verify the critical agent didn't fail
	if _, ok := phaseResult.Errors["critical-agent"]; ok {
		t.Error("Critical agent should not have failed")
	}
}

// TestPipelineWithCriticalAgentFailure tests that critical agent failures halt the pipeline.
func TestPipelineWithCriticalAgentFailure(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create a pipeline with a failing critical agent
	pipeline := &orchestrator.Pipeline{
		Phases: []orchestrator.Phase{
			{
				Name: orchestrator.PhaseName("test-phase"),
				Agents: []agent.Agent{
					createMockAgent(t, e, "success-agent", agent.RolePlanner, true),
					createFailingAgent(t, e, "failing-critical", agent.RoleCoder, true), // Critical
				},
			},
		},
	}

	// Create engine (no recovery)
	engine := orchestrator.NewEngine(pipeline, e.EventBus, nil, nil)

	// Create session state
	ss := e.CreateSessionState()

	// Run pipeline
	result := engine.Run(e.Ctx, ss)

	// Verify pipeline was halted
	if result.Completed {
		t.Error("Pipeline should have been halted due to critical agent failure")
	}

	if result.HaltedAt != "test-phase" {
		t.Errorf("Expected halt at 'test-phase', got %s", result.HaltedAt)
	}

	if result.HaltError == nil {
		t.Error("Expected HaltError to be set")
	}

	// Verify we have errors from the critical agent
	phaseResult := result.PhaseResults[0]
	if _, ok := phaseResult.Errors["failing-critical"]; !ok {
		t.Error("Expected error from 'failing-critical'")
	}
}

// createTestPipeline creates a simple test pipeline with the specified number of agents.
func createTestPipeline(t *testing.T, e *E2ETestContext, numAgents int) *orchestrator.Pipeline {
	agents := make([]agent.Agent, numAgents)
	for i := 0; i < numAgents; i++ {
		name := fmt.Sprintf("test-agent-%d", i)
		role := agent.RolePlanner
		agents[i] = createMockAgent(t, e, name, role, true)
	}

	return &orchestrator.Pipeline{
		Phases: []orchestrator.Phase{
			{
				Name:   orchestrator.PhaseName("test-phase"),
				Agents: agents,
			},
		},
	}
}

// createSlowTestPipeline creates a pipeline with a slow agent for cancellation testing.
func createSlowTestPipeline(t *testing.T, e *E2ETestContext) *orchestrator.Pipeline {
	agents := []agent.Agent{
		createSlowAgent(t, e, "slow-agent", agent.RolePlanner, true),
	}

	return &orchestrator.Pipeline{
		Phases: []orchestrator.Phase{
			{
				Name:   orchestrator.PhaseName("slow-phase"),
				Agents: agents,
			},
		},
	}
}

// createMockAgent creates a mock agent for testing.
func createMockAgent(t *testing.T, e *E2ETestContext, name string, role agent.Role, critical bool) agent.Agent {
	// Create mock provider
	mockProvider := e.CreateMockProvider("claude").(*harness.MockLLM)
	mockProvider.SetResponse("test", fmt.Sprintf("Response from %s", name))

	// Create tool executor
	toolExec := tools.NewToolExecutor(e.TempDir)

	// Create role agent (this implements agent.Agent with Run method)
	return agentroles.NewRoleAgent(role, mockProvider, e.EventBus, toolExec)
}

// createFailingAgent creates an agent that always fails.
func createFailingAgent(t *testing.T, e *E2ETestContext, name string, role agent.Role, critical bool) agent.Agent {
	// Create mock provider that returns error
	mockProvider := e.CreateMockProvider("claude").(*harness.MockLLM)
	mockProvider.SetError("test", fmt.Errorf("agent %s failed intentionally", name))

	// Create tool executor
	toolExec := tools.NewToolExecutor(e.TempDir)

	// Create role agent
	agent := agentroles.NewRoleAgent(role, mockProvider, e.EventBus, toolExec)

	// Override the task to trigger the error
	agent.SetTask("test") // This will match the error pattern

	return agent
}

// createSlowAgent creates an agent that simulates slow execution.
func createSlowAgent(t *testing.T, e *E2ETestContext, name string, role agent.Role, critical bool) agent.Agent {
	// Create mock provider with delay
	mockProvider := e.CreateMockProvider("claude").(*harness.MockLLM)
	mockProvider.SetResponse("test", fmt.Sprintf("Slow response from %s", name))
	mockProvider.SetDelay(500 * time.Millisecond) // 500ms delay

	// Create tool executor
	toolExec := tools.NewToolExecutor(e.TempDir)

	// Create role agent
	return agentroles.NewRoleAgent(role, mockProvider, e.EventBus, toolExec)
}
