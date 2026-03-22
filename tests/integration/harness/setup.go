package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/state"
)

// TestHarness provides a complete test environment for integration testing.
// It sets up mock LLM providers, temporary directories, and test fixtures.
type TestHarness struct {
	T               *testing.T
	Ctx             context.Context
	Cancel          context.CancelFunc
	TempDir         string
	Config          config.Config
	MockBuilder     *MockLLMBuilder
	MemoryStore     memory.MemoryStore
	CheckpointStore state.CheckpointStore
}

// Setup creates a new test harness with all necessary components.
func Setup(t *testing.T) *TestHarness {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "artemis-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Clean up temp dir on test completion
	t.Cleanup(func() {
		cancel()
		os.RemoveAll(tempDir)
	})

	harness := &TestHarness{
		T:           t,
		Ctx:         ctx,
		Cancel:      cancel,
		TempDir:     tempDir,
		Config:      createTestConfig(tempDir),
		MockBuilder: NewMockLLMBuilder(),
	}

	// Initialize memory store
	harness.MemoryStore = harness.setupMemoryStore()

	// Initialize checkpoint store
	harness.CheckpointStore = harness.setupCheckpointStore()

	return harness
}

// createTestConfig creates a test configuration with all providers enabled.
func createTestConfig(tempDir string) config.Config {
	return config.Config{
		ActiveProvider: "mock",
		Claude: config.ProviderConfig{
			Enabled:  true,
			APIKey:   "test-key",
			Model:    "claude-sonnet-4-6",
			Endpoint: "https://api.anthropic.com/v1/messages",
		},
		Gemini: config.ProviderConfig{
			Enabled:  true,
			APIKey:   "test-key",
			Model:    "gemini-3.1-pro-preview",
			Endpoint: "https://generativelanguage.googleapis.com/v1beta",
		},
		GPT: config.ProviderConfig{
			Enabled:  true,
			APIKey:   "test-key",
			Model:    "gpt-5.4",
			Endpoint: "https://api.openai.com/v1/chat/completions",
		},
		GLM: config.GLMConfig{
			Enabled:  true,
			APIKey:   "test-key",
			Model:    "glm-5",
			Endpoint: "https://api.z.ai/api/coding/paas/v4/chat/completions",
		},
		VLLM: config.ProviderConfig{
			Enabled:  false,
			Model:    "qwen2.5-coder-7b",
			Endpoint: "http://localhost:8000/v1/chat/completions",
		},
		Agents: config.AgentConfig{
			Enabled: true,
			Tier:    "premium",
		},
		Memory: config.MemoryConfig{
			Enabled: true,
			DBPath:  filepath.Join(tempDir, "artemis.db"),
		},
	}
}

// setupMemoryStore initializes the memory store for testing.
func (h *TestHarness) setupMemoryStore() memory.MemoryStore {
	store, err := memory.NewSQLiteStore(h.Config.Memory.DBPath)
	if err != nil {
		h.T.Fatalf("Failed to create memory store: %v", err)
	}
	return store
}

// setupCheckpointStore initializes the checkpoint store for testing.
// For now, we use a no-op checkpoint store since SQLiteStore doesn't fully implement the interface.
func (h *TestHarness) setupCheckpointStore() state.CheckpointStore {
	// Return nil for now - checkpoint store is optional in orchestrator
	// Tests that require checkpoint functionality can create a mock implementation
	return nil
}

// CreateMockProvider creates a mock LLM provider for testing.
func (h *TestHarness) CreateMockProvider(provider string) llm.Provider {
	mock := h.MockBuilder.Build(provider)
	if mockLLM, ok := mock.(*MockLLM); ok {
		mockLLM.SetDefaultResponses()
	}
	return mock
}

// CreateSessionState creates a new session state for testing.
func (h *TestHarness) CreateSessionState() *state.SessionState {
	ss := state.NewSessionState()
	ss.SetPhase("test")
	return ss
}

// CreateTempFile creates a temporary file with the given content.
func (h *TestHarness) CreateTempFile(name, content string) string {
	path := filepath.Join(h.TempDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		h.T.Fatalf("Failed to create temp file %s: %v", name, err)
	}
	return path
}

// AssertMockCalled verifies that the mock LLM was called at least once.
func (h *TestHarness) AssertMockCalled(provider string) {
	mock := h.MockBuilder.GetMock(provider)
	if mock == nil {
		h.T.Errorf("Mock provider %s was not created", provider)
		return
	}

	if mock.GetCallCount() == 0 {
		h.T.Errorf("Mock provider %s was not called", provider)
	}
}

// AssertMockCallCount verifies that the mock LLM was called a specific number of times.
func (h *TestHarness) AssertMockCallCount(provider string, expected int) {
	mock := h.MockBuilder.GetMock(provider)
	if mock == nil {
		h.T.Errorf("Mock provider %s was not created", provider)
		return
	}

	actual := mock.GetCallCount()
	if actual != expected {
		h.T.Errorf("Mock provider %s was called %d times, expected %d", provider, actual, expected)
	}
}

// WaitForCondition waits for a condition to become true within the timeout.
func (h *TestHarness) WaitForCondition(condition func() bool, message string) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-timeout:
			h.T.Fatalf("Timeout waiting for condition: %s", message)
		case <-ticker.C:
			if condition() {
				return
			}
		case <-h.Ctx.Done():
			h.T.Fatalf("Context cancelled while waiting for condition: %s", message)
		}
	}
}

// RunTestWithCleanup runs a test function with cleanup logic.
func (h *TestHarness) RunTestWithCleanup(testFunc func()) {
	defer h.MockBuilder.ResetAll()
	testFunc()
}

// GetAllAgentRoles returns all 13 agent role names for testing.
func GetAllAgentRoles() []string {
	return []string{
		"scout",
		"consultant",
		"critic",
		"planner",
		"executor",
		"reviewer",
		"fixer",
		"summarizer",
		"explorer",
		"analyzer",
		"optimizer",
		"validator",
		"reporter",
	}
}

// GetCriticalRoles returns the 4 critical agent roles.
func GetCriticalRoles() []string {
	return []string{
		"scout",
		"planner",
		"executor",
		"reviewer",
	}
}

// GetNonCriticalRoles returns the 9 non-critical agent roles.
func GetNonCriticalRoles() []string {
	return []string{
		"consultant",
		"critic",
		"fixer",
		"summarizer",
		"explorer",
		"analyzer",
		"optimizer",
		"validator",
		"reporter",
	}
}

// AssertNoErrors verifies that the error map is empty.
func AssertNoErrors(t *testing.T, errors map[string]error) {
	t.Helper()

	if len(errors) > 0 {
		t.Errorf("Expected no errors, but got %d errors:", len(errors))
		for agent, err := range errors {
			t.Errorf("  %s: %v", agent, err)
		}
	}
}

// AssertErrorsContain verifies that the error map contains specific agent errors.
func AssertErrorsContain(t *testing.T, errors map[string]error, expectedAgents []string) {
	t.Helper()

	for _, agent := range expectedAgents {
		if _, exists := errors[agent]; !exists {
			t.Errorf("Expected error from agent %s, but none found", agent)
		}
	}
}

// CreateTestArtifact creates a test artifact for session state.
func CreateTestArtifact(artifactType, source, content string) state.Artifact {
	return state.Artifact{
		Type:    state.ArtifactType(artifactType),
		Source:  source,
		Content: content,
	}
}

// AssertArtifactExists verifies that an artifact exists in the session state.
func AssertArtifactExists(t *testing.T, ss *state.SessionState, artifactType, source string) {
	t.Helper()

	artifacts := ss.GetBySource(source)
	found := false
	for _, artifact := range artifacts {
		if string(artifact.Type) == artifactType {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected artifact of type %s from source %s, but none found", artifactType, source)
	}
}

// GetTestTimeout returns the timeout for integration tests.
func GetTestTimeout() time.Duration {
	// Check for environment variable override
	if timeout := os.Getenv("ARTEMIS_TEST_TIMEOUT"); timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			return d
		}
	}
	return 5 * time.Minute
}

// SkipIfShort skips the test if -short flag is provided.
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

// PrintTestSeparator prints a visual separator for test output.
func PrintTestSeparator(name string) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("TEST: %s\n", name)
	fmt.Printf("%s\n\n", strings.Repeat("=", 80))
}
