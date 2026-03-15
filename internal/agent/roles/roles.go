package roles

import (
	"context"
	"fmt"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
)

// RoleAgent is a concrete agent implementation driven by role-specific system prompts.
type RoleAgent struct {
	agent.BaseAgent
	outputType state.ArtifactType
}

// NewRoleAgent creates an agent for a specific role.
func NewRoleAgent(role agent.Role, provider llm.Provider, eb *bus.EventBus, toolExec *tools.ToolExecutor) agent.Agent {
	name, prompt, artifactType, critical := roleConfig(role)
	base := agent.NewBaseAgent(name, role, provider, prompt, eb, critical, toolExec)
	return &RoleAgent{
		BaseAgent:  base,
		outputType: artifactType,
	}
}

// Run executes the agent's task: reads state, calls LLM, writes result.
// If the Orchestrator assigned a specific task via SetTask(), that task is used.
func (r *RoleAgent) Run(ctx context.Context, ss *state.SessionState) error {
	phase := ss.Phase()
	r.EmitStart(phase)

	// Build prompt from session context
	task := r.taskDescription(ss)
	prompt := r.BuildPromptWithContext(ss, task)

	// Call LLM — use tool-aware call if executor is available
	var response string
	var err error
	streamed := false
	if r.IsAutonomous() && r.ToolExecutor() != nil {
		// Phase E-2: Verify-gated autonomous loop
		r.EmitProgress(phase, "Starting autonomous mode...")
		response, err = r.RunAutonomous(ctx, prompt, phase, nil, 0)
		streamed = true
	} else if r.ToolExecutor() != nil {
		r.EmitProgress(phase, "Calling LLM (with tools)...")
		response, err = r.CallLLMWithTools(ctx, prompt, phase)
		streamed = true // CallLLMWithTools now streams chunks via EventBus
	} else {
		r.EmitProgress(phase, "Calling LLM...")
		response, err = r.CallLLM(ctx, prompt)
	}
	if err != nil {
		r.EmitFail(phase, err)
		return fmt.Errorf("agent %s failed: %w", r.Name(), err)
	}

	// Write result to shared state
	ss.AddArtifact(state.Artifact{
		Type:    r.outputType,
		Source:  r.Name(),
		Content: response,
	})

	// Emit output for chat display (only if not already streamed)
	// When streamed, chunks were already emitted via EventAgentStreamChunk
	if !streamed {
		r.EmitOutput(phase, response)
	}

	r.EmitComplete(phase, "Done")
	return nil
}

// taskDescription generates the task prompt based on the agent's role.
// If the Orchestrator assigned a specific task via SetTask(), it takes precedence.
func (r *RoleAgent) taskDescription(ss *state.SessionState) string {
	// Orchestrator-assigned task overrides the default
	if override := r.OverrideTask(); override != "" {
		return override
	}

	// Default task descriptions (used in legacy fixed-pipeline mode)
	switch r.Role() {
	case agent.RoleAnalyzer:
		reqs := ss.GetByType(state.ArtifactUserRequest)
		if len(reqs) > 0 {
			return "Analyze the following user request:\n" + reqs[len(reqs)-1].Content
		}
		return "No user request found."

	case agent.RoleSearcher:
		reqs := ss.GetByType(state.ArtifactUserRequest)
		if len(reqs) > 0 {
			return "Based on this user request, identify what external information is needed:\n" + reqs[len(reqs)-1].Content
		}
		return "Identify relevant external resources."

	case agent.RoleExplorer:
		reqs := ss.GetByType(state.ArtifactUserRequest)
		if len(reqs) > 0 {
			return "Analyze the project codebase in the context of this request:\n" + reqs[len(reqs)-1].Content
		}
		return "Analyze the project codebase structure."

	case agent.RolePlanner:
		return "Based on the analysis results, create a detailed work plan."

	case agent.RoleArchitect:
		return "Based on the plan, design the technical architecture."

	case agent.RoleCoder:
		return "Implement the code changes described in the architecture."

	case agent.RoleDesigner:
		return "Design the user-facing aspects described in the architecture."

	case agent.RoleEngineer:
		return "Handle infrastructure and integration tasks from the plan."

	case agent.RoleQA:
		return "Review all code changes for quality, security, and correctness."

	case agent.RoleTester:
		return "Create and run tests for the implemented code."

	case agent.RoleScout:
		return "Analyze the codebase and gather information relevant to this request."

	case agent.RoleConsultant:
		return "Provide expert analysis and recommendations for this request."

	default:
		return "Execute your assigned task."
	}
}

// roleConfig returns name, system prompt, artifact type, and criticality for each role.
func roleConfig(role agent.Role) (name string, prompt string, artifactType state.ArtifactType, critical bool) {
	switch role {
	case agent.RoleAnalyzer:
		return "Analyzer", AnalyzerPrompt, state.ArtifactAnalysis, true
	case agent.RoleSearcher:
		return "Searcher", SearcherPrompt, state.ArtifactSearchResult, false
	case agent.RoleExplorer:
		return "Explorer", ExplorerPrompt, state.ArtifactExploration, false
	case agent.RolePlanner:
		return "Planner", PlannerPrompt, state.ArtifactPlan, true
	case agent.RoleArchitect:
		return "Architect", ArchitectPrompt, state.ArtifactArchitecture, true
	case agent.RoleCoder:
		return "Coder", CoderPrompt, state.ArtifactCode, true
	case agent.RoleDesigner:
		return "Designer", DesignerPrompt, state.ArtifactDesign, false
	case agent.RoleEngineer:
		return "Engineer", EngineerPrompt, state.ArtifactCode, false
	case agent.RoleQA:
		return "QA", QAPrompt, state.ArtifactQAReport, true
	case agent.RoleTester:
		return "Tester", TesterPrompt, state.ArtifactTestResult, true
	case agent.RoleScout:
		return "Scout", ScoutPrompt, state.ArtifactExploration, false
	case agent.RoleConsultant:
		return "Consultant", ConsultantPrompt, state.ArtifactConsultation, false
	default:
		return string(role), "", state.ArtifactError, false
	}
}

// SystemPromptForRole returns the system prompt for a given agent role.
// Used by the TUI for direct routing (trivial intent) without creating a full RoleAgent.
func SystemPromptForRole(role agent.Role) string {
	_, prompt, _, _ := roleConfig(role)
	return prompt
}
