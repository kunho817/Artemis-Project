package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/orchestrator"
)

// OrchestratorReplanner implements orchestrator.Replanner by calling the Orchestrator LLM
// with a re-planning prompt that includes failure context.
type OrchestratorReplanner struct {
	provider llm.Provider // Orchestrator's LLM provider
}

// NewOrchestratorReplanner creates a replanner backed by the Orchestrator LLM.
func NewOrchestratorReplanner(provider llm.Provider) *OrchestratorReplanner {
	return &OrchestratorReplanner{provider: provider}
}

// Replan calls the Orchestrator LLM with failure context to produce a revised partial plan.
func (r *OrchestratorReplanner) Replan(ctx context.Context, rctx orchestrator.ReplanContext) (*orchestrator.ExecutionPlan, error) {
	if r.provider == nil {
		return nil, nil
	}

	prompt := r.buildReplanPrompt(rctx)

	messages := []llm.Message{
		{Role: "system", Content: replanSystemPrompt},
		{Role: "user", Content: prompt},
	}

	resp, err := r.provider.Send(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("replanner LLM call failed: %w", err)
	}

	plan, parseErr := orchestrator.ParsePlan(resp)
	if parseErr != nil {
		return nil, fmt.Errorf("replanner response parse failed: %w", parseErr)
	}

	return plan, nil
}

// buildReplanPrompt constructs the re-planning prompt with full failure context.
func (r *OrchestratorReplanner) buildReplanPrompt(rctx orchestrator.ReplanContext) string {
	// Serialize original plan for context
	origPlanJSON := ""
	if rctx.OriginalPlan != nil {
		if b, err := json.Marshal(rctx.OriginalPlan); err == nil {
			origPlanJSON = string(b)
		}
	}

	prompt := fmt.Sprintf(`## Re-planning Request

### Trigger: %s

### Original Plan:
%s

### Execution Progress:
- Completed steps: %d / %d total
- Failed at: step %d (%s)

### Failure Details:
%s
`, rctx.Trigger, origPlanJSON, rctx.CompletedSteps, len(rctx.OriginalPlan.Steps),
		rctx.FailedStepIndex+1, rctx.FailedStepName, rctx.FailureReason)

	if rctx.ReviewIssues != "" {
		prompt += fmt.Sprintf("\n### Unresolved Review Issues:\n%s\n", rctx.ReviewIssues)
	}

	if rctx.Artifacts != "" {
		// Truncate artifacts summary to avoid token overflow
		artifacts := rctx.Artifacts
		if len(artifacts) > 2000 {
			artifacts = artifacts[:2000] + "\n... (truncated)"
		}
		prompt += fmt.Sprintf("\n### Artifacts Produced So Far:\n%s\n", artifacts)
	}

	prompt += `
### Instructions:
Create a NEW partial execution plan that:
1. Acknowledges what has already been completed (do NOT re-do successful work)
2. Addresses the failure/issues with a different approach
3. Completes the remaining goals from the original plan

Respond with a JSON ExecutionPlan containing only the NEW steps needed.`

	return prompt
}

const replanSystemPrompt = `You are the Artemis Orchestrator in RE-PLANNING mode.
A previous execution plan partially failed. You must create a revised plan for the REMAINING work.

CRITICAL RULES:
- Do NOT include steps that already completed successfully.
- Address the specific failure or unresolved issues with a different approach.
- Keep the plan minimal — only include what's needed to finish the job.
- Use the same agent roles and task format as normal plans.
- Output valid JSON only. No markdown, no extra text.

Output format:
{"reasoning":"Why this new approach should work","steps":[{"tasks":[{"agent":"...","task":"...","critical":true}]}]}`
