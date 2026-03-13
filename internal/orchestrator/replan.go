package orchestrator

import "context"

// ReplanContext provides the Replanner with information about the current execution state,
// including what has been completed, what failed, and why re-planning is needed.
type ReplanContext struct {
	OriginalPlan    *ExecutionPlan // The original plan being executed
	CompletedSteps  int            // Number of steps successfully completed
	FailedStepIndex int            // Index of the step that triggered re-planning (-1 if review exhaustion)
	FailedStepName  string         // Name of the failed step
	FailureReason   string         // Description of what went wrong
	ReviewIssues    string         // Remaining review issues (for review exhaustion triggers)
	Artifacts       string         // Summary of artifacts produced so far
	Trigger         ReplanTrigger  // What triggered the re-plan
}

// ReplanTrigger identifies why re-planning was requested.
type ReplanTrigger string

const (
	// ReplanOnFailure triggers when a step fails critically and recovery is exhausted.
	ReplanOnFailure ReplanTrigger = "failure"

	// ReplanOnReviewExhaustion triggers when review loop max iterations are reached
	// but issues remain unresolved.
	ReplanOnReviewExhaustion ReplanTrigger = "review_exhaustion"
)

// Replanner produces a revised partial execution plan when the current plan can't continue.
// The implementation typically calls the Orchestrator LLM with failure context to get a new plan.
// Returns nil plan to signal that re-planning is not possible (fall through to abort/recovery).
type Replanner interface {
	// Replan produces a new partial ExecutionPlan for the remaining work.
	// The returned plan replaces all unexecuted steps in the original plan.
	// Returns (nil, nil) to signal that re-planning should be skipped.
	Replan(ctx context.Context, rctx ReplanContext) (*ExecutionPlan, error)
}
