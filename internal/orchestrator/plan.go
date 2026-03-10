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
