// Package memory provides integration tests for checkpoint persistence.
package memory

import (
	"context"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestCheckpointSaveAndLoad tests basic checkpoint save and load operations.
func TestCheckpointSaveAndLoad(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// First, create a pipeline run (required for foreign key constraint)
	runID := "test-run-123"
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}

	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Create a checkpoint
	cp := &state.StepCheckpoint{
		RunID:         runID,
		StepIndex:     0,
		StepName:      "analysis",
		Status:        "completed",
		ArtifactsJSON: `[{"type":"analysis","source":"test-agent","content":"Test analysis"}]`,
		AgentResultsJSON: `{"test-agent":"success"}`,
	}

	// Save checkpoint
	err = store.SaveCheckpoint(ctx, cp)
	if err != nil {
		h.T.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve checkpoints
	checkpoints, err := store.GetCheckpoints(ctx, "test-run-123")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		h.T.Errorf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	// Verify checkpoint content
	retrieved := checkpoints[0]
	if retrieved.RunID != "test-run-123" {
		h.T.Errorf("Expected run_id 'test-run-123', got %s", retrieved.RunID)
	}
	if retrieved.StepName != "analysis" {
		h.T.Errorf("Expected step_name 'analysis', got %s", retrieved.StepName)
	}
	if retrieved.Status != "completed" {
		h.T.Errorf("Expected status 'completed', got %s", retrieved.Status)
	}
}

// TestMultipleCheckpoints tests saving and retrieving multiple checkpoints.
func TestMultipleCheckpoints(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_multiple_checkpoints.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	runID := "test-run-multi"

	// First, create a pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}

	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Create multiple checkpoints for the same run
	checkpoints := []*state.StepCheckpoint{
		{
			RunID:         runID,
			StepIndex:     0,
			StepName:      "analysis",
			Status:        "completed",
			ArtifactsJSON: `[{"type":"analysis","content":"Step 1"}]`,
		},
		{
			RunID:         runID,
			StepIndex:     1,
			StepName:      "planning",
			Status:        "completed",
			ArtifactsJSON: `[{"type":"plan","content":"Step 2"}]`,
		},
		{
			RunID:         runID,
			StepIndex:     2,
			StepName:      "execution",
			Status:        "in_progress",
			ArtifactsJSON: `[{"type":"code","content":"Step 3"}]`,
		},
	}

	// Save all checkpoints
	for _, cp := range checkpoints {
		err = store.SaveCheckpoint(ctx, cp)
		if err != nil {
			h.T.Fatalf("Failed to save checkpoint: %v", err)
		}
	}

	// Retrieve all checkpoints
	retrieved, err := store.GetCheckpoints(ctx, "test-run-multi")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints: %v", err)
	}

	if len(retrieved) != 3 {
		h.T.Errorf("Expected 3 checkpoints, got %d", len(retrieved))
	}

	// Verify order (should be ordered by step_index)
	if retrieved[0].StepName != "analysis" {
		h.T.Errorf("Expected first step to be 'analysis', got %s", retrieved[0].StepName)
	}
	if retrieved[1].StepName != "planning" {
		h.T.Errorf("Expected second step to be 'planning', got %s", retrieved[1].StepName)
	}
	if retrieved[2].StepName != "execution" {
		h.T.Errorf("Expected third step to be 'execution', got %s", retrieved[2].StepName)
	}
}

// TestCheckpointByRunID tests that checkpoints are correctly isolated by run ID.
func TestCheckpointByRunID(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint_runid.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Create pipeline runs for each run ID
	for _, runID := range []string{"run-1", "run-2"} {
		pipelineRun := &memory.PipelineRun{
			ID:        runID,
			SessionID: "test-session",
			Intent:    "test",
			Status:    "running",
		}
		err = store.SavePipelineRun(ctx, pipelineRun)
		if err != nil {
			h.T.Fatalf("Failed to save pipeline run %s: %v", runID, err)
		}
	}

	// Save checkpoints for different runs
	checkpoints := []*state.StepCheckpoint{
		{RunID: "run-1", StepIndex: 0, StepName: "step1", Status: "completed"},
		{RunID: "run-1", StepIndex: 1, StepName: "step2", Status: "completed"},
		{RunID: "run-2", StepIndex: 0, StepName: "step1", Status: "completed"},
		{RunID: "run-2", StepIndex: 1, StepName: "step2", Status: "in_progress"},
		{RunID: "run-2", StepIndex: 2, StepName: "step3", Status: "pending"},
	}

	for _, cp := range checkpoints {
		err = store.SaveCheckpoint(ctx, cp)
		if err != nil {
			h.T.Fatalf("Failed to save checkpoint: %v", err)
		}
	}

	// Verify run-1 has 2 checkpoints
	run1Checkpoints, err := store.GetCheckpoints(ctx, "run-1")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints for run-1: %v", err)
	}
	if len(run1Checkpoints) != 2 {
		h.T.Errorf("Expected 2 checkpoints for run-1, got %d", len(run1Checkpoints))
	}

	// Verify run-2 has 3 checkpoints
	run2Checkpoints, err := store.GetCheckpoints(ctx, "run-2")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints for run-2: %v", err)
	}
	if len(run2Checkpoints) != 3 {
		h.T.Errorf("Expected 3 checkpoints for run-2, got %d", len(run2Checkpoints))
	}

	// Verify non-existent run returns empty list
	run3Checkpoints, err := store.GetCheckpoints(ctx, "run-3")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints for run-3: %v", err)
	}
	if len(run3Checkpoints) != 0 {
		h.T.Errorf("Expected 0 checkpoints for non-existent run-3, got %d", len(run3Checkpoints))
	}
}

// TestCheckpointArtifactSerialization tests artifact JSON serialization.
func TestCheckpointArtifactSerialization(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint_artifacts.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	runID := "test-run-artifacts"

	// Create pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}
	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Create artifacts
	ss := state.NewSessionState()
	ss.SetPhase("test-phase")
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactAnalysis,
		Source:  "agent-1",
		Content: "Analysis content",
	})
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactPlan,
		Source:  "agent-2",
		Content: "Plan content",
	})

	// Serialize artifacts to JSON
	artifactsJSON := state.SerializeArtifacts(ss.GetArtifacts())

	// Create checkpoint with serialized artifacts
	cp := &state.StepCheckpoint{
		RunID:         runID,
		StepIndex:     0,
		StepName:      "test",
		Status:        "completed",
		ArtifactsJSON: artifactsJSON,
	}

	// Save checkpoint
	err = store.SaveCheckpoint(ctx, cp)
	if err != nil {
		h.T.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve checkpoint
	checkpoints, err := store.GetCheckpoints(ctx, "test-run-artifacts")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		h.T.Fatalf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	// Deserialize artifacts
	retrievedArtifacts := state.DeserializeArtifacts(checkpoints[0].ArtifactsJSON)

	// Verify artifacts
	if len(retrievedArtifacts) != 2 {
		h.T.Errorf("Expected 2 artifacts, got %d", len(retrievedArtifacts))
	}

	// Verify first artifact
	if retrievedArtifacts[0].Type != state.ArtifactAnalysis {
		h.T.Errorf("Expected first artifact type 'analysis', got %s", retrievedArtifacts[0].Type)
	}
	if retrievedArtifacts[0].Content != "Analysis content" {
		h.T.Errorf("Expected first artifact content 'Analysis content', got %s", retrievedArtifacts[0].Content)
	}
}

// TestCheckpointPersistenceAcrossRestores tests checkpoint persistence across store restarts.
func TestCheckpointPersistenceAcrossRestores(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint_persistence.db"

	// Create first store
	store1, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create first store: %v", err)
	}

	ctx := context.Background()

	runID := "test-run-persistence"

	// Create pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}
	err = store1.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	cp := &state.StepCheckpoint{
		RunID:         runID,
		StepIndex:     0,
		StepName:      "persistent-step",
		Status:        "completed",
		ArtifactsJSON: `[{"type":"test","content":"Persistent data"}]`,
	}

	err = store1.SaveCheckpoint(ctx, cp)
	if err != nil {
		h.T.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Close first store
	store1.DB().Close()

	// Create second store (simulates restart)
	store2, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create second store: %v", err)
	}

	// Verify checkpoint persisted
	checkpoints, err := store2.GetCheckpoints(ctx, "test-run-persistence")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints after restart: %v", err)
	}

	if len(checkpoints) != 1 {
		h.T.Errorf("Expected 1 checkpoint after restart, got %d", len(checkpoints))
	}

	if checkpoints[0].StepName != "persistent-step" {
		h.T.Errorf("Expected step name 'persistent-step', got %s", checkpoints[0].StepName)
	}
}

// TestPipelineResumption tests a realistic pipeline resumption scenario.
func TestPipelineResumption(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_pipeline_resumption.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	runID := "pipeline-run-001"

	// Create pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}
	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Simulate a pipeline that completed first 2 phases
	checkpoints := []*state.StepCheckpoint{
		{
			RunID:         runID,
			StepIndex:     0,
			StepName:      "analysis",
			Status:        "completed",
			ArtifactsJSON: `[{"type":"analysis","source":"analyst","content":"Requirements analyzed"}]`,
		},
		{
			RunID:         runID,
			StepIndex:     1,
			StepName:      "planning",
			Status:        "completed",
			ArtifactsJSON: `[{"type":"plan","source":"planner","content":"Implementation plan created"}]`,
		},
		{
			RunID:         runID,
			StepIndex:     2,
			StepName:      "execution",
			Status:        "failed",
			ArtifactsJSON: `[{"type":"error","source":"executor","content":"Execution failed"}]`,
		},
	}

	for _, cp := range checkpoints {
		err = store.SaveCheckpoint(ctx, cp)
		if err != nil {
			h.T.Fatalf("Failed to save checkpoint: %v", err)
		}
	}

	// Simulate resumption: load checkpoints to find last successful state
	retrieved, err := store.GetCheckpoints(ctx, runID)
	if err != nil {
		h.T.Fatalf("Failed to load checkpoints for resumption: %v", err)
	}

	// Find last completed step
	lastCompletedIndex := -1
	for _, cp := range retrieved {
		if cp.Status == "completed" {
			lastCompletedIndex = cp.StepIndex
		}
	}

	if lastCompletedIndex != 1 {
		h.T.Errorf("Expected last completed index to be 1, got %d", lastCompletedIndex)
	}

	// Verify we can resume from the failed step
	resumeFromStep := ""
	for _, cp := range retrieved {
		if cp.Status == "failed" || cp.Status == "in_progress" {
			resumeFromStep = cp.StepName
			break
		}
	}

	if resumeFromStep != "execution" {
		h.T.Errorf("Expected to resume from 'execution', got %s", resumeFromStep)
	}

	h.T.Logf("Pipeline resumption: can resume from step '%s' (index %d)", resumeFromStep, lastCompletedIndex+1)
}

// TestCheckpointTimestamps verifies checkpoint timestamps are recorded correctly.
func TestCheckpointTimestamps(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint_timestamps.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	runID := "test-run-timestamps"

	// Create pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}
	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Save checkpoint
	cp := &state.StepCheckpoint{
		RunID:     runID,
		StepIndex: 0,
		StepName:  "test",
		Status:    "completed",
	}
	err = store.SaveCheckpoint(ctx, cp)
	if err != nil {
		h.T.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve checkpoint
	checkpoints, err := store.GetCheckpoints(ctx, runID)
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		h.T.Fatalf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	// Verify timestamp is set (ignore exact time due to timezone differences)
	retrievedTime := checkpoints[0].CreatedAt
	if retrievedTime.IsZero() {
		h.T.Error("Expected non-zero timestamp")
	}

	// Verify timestamp is recent (within last minute)
	recentThreshold := time.Now().Add(-1 * time.Minute)
	if retrievedTime.Before(recentThreshold) {
		h.T.Errorf("Expected recent timestamp, got %v", retrievedTime)
	}
}

// TestCheckpointWithEmptyArtifacts tests checkpoints with no artifacts.
func TestCheckpointWithEmptyArtifacts(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_checkpoint_empty.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	runID := "test-run-empty"

	// Create pipeline run
	pipelineRun := &memory.PipelineRun{
		ID:        runID,
		SessionID: "test-session",
		Intent:    "test",
		Status:    "running",
	}
	err = store.SavePipelineRun(ctx, pipelineRun)
	if err != nil {
		h.T.Fatalf("Failed to save pipeline run: %v", err)
	}

	// Save checkpoint with empty artifacts
	cp := &state.StepCheckpoint{
		RunID:         runID,
		StepIndex:     0,
		StepName:      "test",
		Status:        "completed",
		ArtifactsJSON: "[]",
	}

	err = store.SaveCheckpoint(ctx, cp)
	if err != nil {
		h.T.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Retrieve checkpoint
	checkpoints, err := store.GetCheckpoints(ctx, "test-run-empty")
	if err != nil {
		h.T.Fatalf("Failed to get checkpoints: %v", err)
	}

	if len(checkpoints) != 1 {
		h.T.Fatalf("Expected 1 checkpoint, got %d", len(checkpoints))
	}

	// Verify artifacts can be deserialized
	artifacts := state.DeserializeArtifacts(checkpoints[0].ArtifactsJSON)

	if len(artifacts) != 0 {
		h.T.Errorf("Expected 0 artifacts, got %d", len(artifacts))
	}
}
