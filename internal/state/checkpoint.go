package state

import (
	"context"
	"encoding/json"
	"time"
)

// StepCheckpoint captures the outcome of a single step execution for resume support.
// Checkpoints are saved after each step completes in RunPlan(), enabling
// interrupted pipelines to resume from the last successful step.
type StepCheckpoint struct {
	ID               int64     `json:"id,omitempty"` // DB auto-increment (0 for new)
	RunID            string    `json:"run_id"`
	StepIndex        int       `json:"step_index"`
	StepName         string    `json:"step_name"`
	Status           string    `json:"status"`                       // "completed", "failed", "skipped"
	ArtifactsJSON    string    `json:"artifacts_json,omitempty"`     // serialized []Artifact
	AgentResultsJSON string    `json:"agent_results_json,omitempty"` // serialized map[agentName]result
	CreatedAt        time.Time `json:"created_at"`
}

// IncompleteRun contains the minimal info needed to offer a resume prompt.
type IncompleteRun struct {
	RunID         string    `json:"run_id"`
	SessionID     string    `json:"session_id"`
	Intent        string    `json:"intent"`
	PlanJSON      string    `json:"plan_json"`
	Status        string    `json:"status"` // "running" (interrupted)
	CreatedAt     time.Time `json:"created_at"`
	LastStepIndex int       `json:"last_step_index"` // highest completed step (-1 if none)
	LastStepName  string    `json:"last_step_name"`
	TotalSteps    int       `json:"total_steps"` // from plan JSON
}

// CheckpointStore defines the persistence interface for step checkpoints.
// Implemented by SQLiteStore (memory package) to avoid circular imports.
type CheckpointStore interface {
	// SaveCheckpoint persists a step checkpoint after step completion.
	SaveCheckpoint(ctx context.Context, cp *StepCheckpoint) error

	// GetCheckpoints returns all checkpoints for a pipeline run, ordered by step index.
	GetCheckpoints(ctx context.Context, runID string) ([]StepCheckpoint, error)

	// GetIncompleteRuns returns pipeline runs that are still in "running" status
	// for the given session. Used for auto-resume detection on startup.
	GetIncompleteRuns(ctx context.Context, sessionID string) ([]IncompleteRun, error)

	// DeleteCheckpoints removes all checkpoints for a pipeline run (cleanup after completion).
	DeleteCheckpoints(ctx context.Context, runID string) error
}

// SerializeArtifacts converts artifacts to JSON for checkpoint storage.
func SerializeArtifacts(artifacts []Artifact) string {
	data, err := json.Marshal(artifacts)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// DeserializeArtifacts restores artifacts from checkpoint JSON.
func DeserializeArtifacts(jsonStr string) []Artifact {
	if jsonStr == "" || jsonStr == "[]" {
		return nil
	}
	var artifacts []Artifact
	if err := json.Unmarshal([]byte(jsonStr), &artifacts); err != nil {
		return nil
	}
	return artifacts
}

// SerializeAgentResults converts agent results map to JSON.
func SerializeAgentResults(results map[string]string) string {
	data, err := json.Marshal(results)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// DeserializeAgentResults restores agent results from checkpoint JSON.
func DeserializeAgentResults(jsonStr string) map[string]string {
	if jsonStr == "" || jsonStr == "{}" {
		return nil
	}
	var results map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		return nil
	}
	return results
}
