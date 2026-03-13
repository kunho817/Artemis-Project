package orchestrator

import (
	"context"
)

// RecoveryAction represents a recovery decision.
type RecoveryAction string

const (
	// ActionRetry re-runs the failed agent from Stage 2 (Consultant diagnosis).
	ActionRetry RecoveryAction = "retry"
	// ActionSkip skips the failed agent and continues the pipeline.
	ActionSkip RecoveryAction = "skip"
	// ActionAbort halts the pipeline immediately.
	ActionAbort RecoveryAction = "abort"
)

// RecoveryContext carries failure information and Consultant diagnosis to the user.
type RecoveryContext struct {
	FailedAgent  string // display name of the failed agent
	FailedRole   string // role of the failed agent
	Task         string // the task the agent was executing
	Error        error  // original error from the agent
	Diagnosis    string // Consultant's diagnosis (empty if Consultant also failed)
	Suggestion   string // Consultant's suggested correction
	AttemptCount int    // number of recovery attempts so far
}

// RecoveryPrompter is called by the Engine when automated recovery fails
// and a user decision is needed. Implementations block until a decision is made.
// The context is used to detect app shutdown / pipeline cancellation.
type RecoveryPrompter interface {
	Prompt(ctx context.Context, rc RecoveryContext) (RecoveryAction, error)
}

// MaxRecoveryAttempts is the maximum number of full recovery cycles
// (Consultant diagnosis + agent retry) before forcing user escalation.
const MaxRecoveryAttempts = 3
