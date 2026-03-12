package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artemis-project/artemis/internal/agent"
)

// ExecutionPlan represents the Orchestrator's routing decision.
// It defines which agents to invoke and in what order.
type ExecutionPlan struct {
	Reasoning string          `json:"reasoning"`
	Steps     []ExecutionStep `json:"steps"`
}

// ExecutionStep groups tasks that run in parallel.
// Steps themselves run sequentially — step 2 waits for step 1 to finish.
type ExecutionStep struct {
	Tasks []AgentTask `json:"tasks"`
}

// AgentTask is a single agent assignment from the Orchestrator.
type AgentTask struct {
	Agent    string `json:"agent"`    // Role name (e.g., "coder", "analyzer")
	Task     string `json:"task"`     // Specific task description
	Critical bool   `json:"critical"` // If true, failure stops the plan
}

// IntentType classifies the user's request intent.
type IntentType string

const (
	IntentTrivial        IntentType = "trivial"        // Simple greeting or question — no tools needed
	IntentConversational IntentType = "conversational"  // Single agent task — may use tools
	IntentExploratory    IntentType = "exploratory"     // Needs codebase/external exploration first
	IntentComplex        IntentType = "complex"         // Full multi-agent pipeline
)

// ExplorationTask defines a pre-execution exploration query (Phase 3).
type ExplorationTask struct {
	Query string `json:"query"`
	Scope string `json:"scope"` // "codebase" or "external"
}

// OrchestratorResponse wraps the intent classification and routing decision.
type OrchestratorResponse struct {
	Intent    IntentType `json:"intent"`
	Reasoning string     `json:"reasoning"`

	// For trivial/conversational — single-agent routing
	DirectAgent string `json:"direct_agent,omitempty"`
	DirectTask  string `json:"direct_task,omitempty"`

	// For exploratory — pre-execution exploration tasks (Phase 3)
	ExplorationTasks []ExplorationTask `json:"exploration_tasks,omitempty"`

	// For exploratory/complex — full execution plan steps
	Steps []ExecutionStep `json:"steps,omitempty"`
}

// ToExecutionPlan converts an OrchestratorResponse to an ExecutionPlan.
// For trivial/conversational, it creates a minimal 1-step plan.
func (r *OrchestratorResponse) ToExecutionPlan() *ExecutionPlan {
	switch r.Intent {
	case IntentTrivial, IntentConversational:
		if r.DirectAgent == "" {
			return nil
		}
		return &ExecutionPlan{
			Reasoning: r.Reasoning,
			Steps: []ExecutionStep{
				{Tasks: []AgentTask{
					{Agent: r.DirectAgent, Task: r.DirectTask, Critical: true},
				}},
			},
		}
	case IntentExploratory, IntentComplex:
		if len(r.Steps) == 0 {
			return nil
		}
		return &ExecutionPlan{
			Reasoning: r.Reasoning,
			Steps:     r.Steps,
		}
	default:
		return nil
	}
}

// TotalTasks returns the total number of tasks across all steps.
func (p *ExecutionPlan) TotalTasks() int {
	total := 0
	for _, step := range p.Steps {
		total += len(step.Tasks)
	}
	return total
}

// AgentNames returns all unique agent names in the plan.
func (p *ExecutionPlan) AgentNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, step := range p.Steps {
		for _, task := range step.Tasks {
			if !seen[task.Agent] {
				seen[task.Agent] = true
				names = append(names, task.Agent)
			}
		}
	}
	return names
}

// ParsePlan extracts an ExecutionPlan from the Orchestrator's LLM response.
// It handles raw JSON, JSON in markdown code blocks, and JSON embedded in prose.
func ParsePlan(response string) (*ExecutionPlan, error) {
	response = strings.TrimSpace(response)

	// Try direct JSON parse
	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(response), &plan); err == nil {
		if err := validatePlan(&plan); err != nil {
			return nil, err
		}
		return &plan, nil
	}

	// Try extracting JSON from markdown or prose
	extracted := extractJSON(response)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &plan); err == nil {
			if err := validatePlan(&plan); err != nil {
				return nil, err
			}
			return &plan, nil
		}
	}

	return nil, fmt.Errorf("failed to parse execution plan from orchestrator response")
}

// extractJSON finds a JSON object in a response that may be wrapped in markdown
// code blocks or surrounded by prose text.
func extractJSON(s string) string {
	// Try ```json ... ``` blocks
	if idx := strings.Index(s, "```json"); idx >= 0 {
		rest := s[idx+7:]
		if end := strings.Index(rest, "```"); end >= 0 {
			return strings.TrimSpace(rest[:end])
		}
	}

	// Try ``` ... ``` blocks (without language tag)
	if idx := strings.Index(s, "```"); idx >= 0 {
		rest := s[idx+3:]
		if end := strings.Index(rest, "```"); end >= 0 {
			candidate := strings.TrimSpace(rest[:end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	// Try finding raw JSON object: first '{' to last '}'
	first := strings.Index(s, "{")
	last := strings.LastIndex(s, "}")
	if first >= 0 && last > first {
		return s[first : last+1]
	}

	return ""
}

// validatePlan checks that the plan is structurally valid and all referenced
// agent roles exist (excluding Orchestrator, which is a coordinator, not a worker).
func validatePlan(plan *ExecutionPlan) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("execution plan has no steps")
	}
	for i, step := range plan.Steps {
		if len(step.Tasks) == 0 {
			return fmt.Errorf("step %d has no tasks", i+1)
		}
		for _, task := range step.Tasks {
			if task.Agent == "" {
				return fmt.Errorf("step %d: task has empty agent name", i+1)
			}
			if task.Task == "" {
				return fmt.Errorf("step %d: agent %q has empty task description", i+1, task.Agent)
			}
			if !isWorkerRole(task.Agent) {
				return fmt.Errorf("step %d: unknown or invalid agent role %q", i+1, task.Agent)
			}
		}
	}
	return nil
}

// isWorkerRole checks if the role name corresponds to a valid worker agent.
// The Orchestrator role is excluded — it coordinates, it doesn't work.
func isWorkerRole(name string) bool {
	if name == string(agent.RoleOrchestrator) {
		return false
	}
	for _, r := range agent.AllRoles() {
		if string(r) == name {
			return true
		}
	}
	return false
}

// ParseOrchestratorResponse extracts an OrchestratorResponse from the LLM output.
// It supports the new intent-based format and falls back to the legacy ExecutionPlan format.
func ParseOrchestratorResponse(response string) (*OrchestratorResponse, error) {
	response = strings.TrimSpace(response)

	// Try direct JSON parse as OrchestratorResponse
	var resp OrchestratorResponse
	if err := json.Unmarshal([]byte(response), &resp); err == nil {
		if err := validateOrchestratorResponse(&resp); err == nil {
			return &resp, nil
		}
	}

	// Try extracting JSON from markdown or prose
	extracted := extractJSON(response)
	if extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &resp); err == nil {
			if err := validateOrchestratorResponse(&resp); err == nil {
				return &resp, nil
			}
		}
	}

	// Backward compatibility: try parsing as legacy ExecutionPlan format
	plan, err := ParsePlan(response)
	if err == nil {
		return &OrchestratorResponse{
			Intent:    IntentComplex,
			Reasoning: plan.Reasoning,
			Steps:     plan.Steps,
		}, nil
	}

	return nil, fmt.Errorf("failed to parse orchestrator response")
}

// validateOrchestratorResponse checks structural validity of the response.
func validateOrchestratorResponse(resp *OrchestratorResponse) error {
	switch resp.Intent {
	case IntentTrivial, IntentConversational:
		if resp.DirectAgent == "" {
			return fmt.Errorf("%s intent requires direct_agent", resp.Intent)
		}
		if resp.DirectTask == "" {
			return fmt.Errorf("%s intent requires direct_task", resp.Intent)
		}
		if !isWorkerRole(resp.DirectAgent) {
			return fmt.Errorf("unknown agent role %q", resp.DirectAgent)
		}
		return nil

	case IntentExploratory:
		// Exploratory may or may not have steps yet
		if len(resp.Steps) > 0 {
			plan := &ExecutionPlan{Steps: resp.Steps}
			return validatePlan(plan)
		}
		return nil

	case IntentComplex:
		if len(resp.Steps) == 0 {
			return fmt.Errorf("complex intent requires at least one step")
		}
		plan := &ExecutionPlan{Steps: resp.Steps}
		return validatePlan(plan)

	default:
		// If intent is empty but has steps, treat as complex (backward compat)
		if len(resp.Steps) > 0 {
			resp.Intent = IntentComplex
			plan := &ExecutionPlan{Steps: resp.Steps}
			return validatePlan(plan)
		}
		return fmt.Errorf("unknown intent type %q", resp.Intent)
	}
}
