package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestMockLLMBasicFunctionality tests the mock LLM provider.
func TestMockLLMBasicFunctionality(t *testing.T) {
	h := harness.Setup(t)
	h.RunTestWithCleanup(func() {
		mock := h.CreateMockProvider("claude").(*harness.MockLLM)

		// Test default responses
		mock.SetDefaultResponses()

		response, err := mock.Send(h.Ctx, []llm.Message{
			{Role: "user", Content: "hello"},
		})

		if err != nil {
			t.Fatalf("Send failed: %v", err)
		}

		if response == "" {
			t.Error("Expected non-empty response")
		}

		// Test call count
		if mock.GetCallCount() != 1 {
			t.Errorf("Expected 1 call, got %d", mock.GetCallCount())
		}
	})
}

// TestMockLLMPatternMatching tests pattern matching in mock responses.
func TestMockLLMPatternMatching(t *testing.T) {
	h := harness.Setup(t)
	h.RunTestWithCleanup(func() {
		mock := h.CreateMockProvider("gemini").(*harness.MockLLM)

		// Set custom responses
		mock.SetResponse("test", "Custom test response")
		mock.SetResponse("code", "Custom code response")

		tests := []struct {
			input    string
			expected string
		}{
			{"test", "Custom test response"},
			{"code", "Custom code response"},
			{"unknown", "Mock response to: unknown"},
		}

		for _, tt := range tests {
			t.Run(tt.input, func(t *testing.T) {
				response, err := mock.Send(h.Ctx, []llm.Message{
					{Role: "user", Content: tt.input},
				})

				if err != nil {
					t.Fatalf("Send failed: %v", err)
				}

				if response != tt.expected {
					t.Errorf("Expected response %q, got %q", tt.expected, response)
				}
			})
		}
	})
}

// TestMockLLMStreaming tests streaming functionality.
func TestMockLLMStreaming(t *testing.T) {
	h := harness.Setup(t)
	h.RunTestWithCleanup(func() {
		mock := h.CreateMockProvider("gpt").(*harness.MockLLM)

		chunkCount := 0
		var fullContent string

		chunk, err := mock.Stream(h.Ctx, []llm.Message{
			{Role: "user", Content: "hello"},
		})

		if err != nil {
			t.Fatalf("Stream failed: %v", err)
		}

		for c := range chunk {
			if c.Done {
				if c.Usage == nil {
					t.Error("Expected usage information on final chunk")
				}
				break
			}
			fullContent += c.Content
			chunkCount++
		}

		if chunkCount == 0 {
			t.Error("Expected at least one chunk")
		}

		if fullContent == "" {
			t.Error("Expected non-empty content from stream")
		}
	})
}

// TestMockBuilder tests the mock builder functionality.
func TestMockBuilder(t *testing.T) {
	h := harness.Setup(t)
	h.RunTestWithCleanup(func() {
		// Create multiple mocks
		providers := []string{"claude", "gemini", "gpt", "glm"}

		for _, provider := range providers {
			mock := h.CreateMockProvider(provider)
			if mock == nil {
				t.Errorf("Failed to create mock for provider %s", provider)
				continue
			}

			if mock.Name() != "mock" {
				t.Errorf("Expected mock name 'mock', got %s", mock.Name())
			}

			// Actually call Send to increment call count
			_, err := mock.Send(h.Ctx, []llm.Message{
				{Role: "user", Content: "test"},
			})
			if err != nil {
				t.Errorf("Send failed for provider %s: %v", provider, err)
			}
		}

		// Verify call counts
		for _, provider := range providers {
			h.AssertMockCalled(provider)
		}
	})
}

// TestFixtureManager tests fixture creation.
func TestFixtureManager(t *testing.T) {
	h := harness.Setup(t)

	fm := harness.NewFixtureManager(h.TempDir)

	// Test Go fixture creation
	goPath, err := fm.CreateGoFixture("test-go")
	if err != nil {
		t.Fatalf("Failed to create Go fixture: %v", err)
	}

	// Verify files exist
	expectedFiles := []string{"main.go", "go.mod"}
	for _, file := range expectedFiles {
		if _, err := os.Stat(filepath.Join(goPath, file)); os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist", file)
		}
	}

	// Test complex fixture
	complexPath, err := fm.CreateComplexFixture("test-complex")
	if err != nil {
		t.Fatalf("Failed to create complex fixture: %v", err)
	}

	// Verify additional files exist
	additionalFiles := []string{"utils.go", "utils_test.go", "README.md"}
	for _, file := range additionalFiles {
		if _, err := os.Stat(filepath.Join(complexPath, file)); os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist in complex fixture", file)
		}
	}
}

// TestAllAgentRoles tests that all 13 agent roles are defined.
func TestAllAgentRoles(t *testing.T) {
	roles := harness.GetAllAgentRoles()

	expectedCount := 13
	if len(roles) != expectedCount {
		t.Errorf("Expected %d agent roles, got %d", expectedCount, len(roles))
	}

	// Verify critical roles exist
	criticalRoles := harness.GetCriticalRoles()
	if len(criticalRoles) != 4 {
		t.Errorf("Expected 4 critical roles, got %d", len(criticalRoles))
	}

	// Verify non-critical roles exist
	nonCriticalRoles := harness.GetNonCriticalRoles()
	if len(nonCriticalRoles) != 9 {
		t.Errorf("Expected 9 non-critical roles, got %d", len(nonCriticalRoles))
	}

	// Verify no overlap between critical and non-critical
	criticalMap := make(map[string]bool)
	for _, role := range criticalRoles {
		criticalMap[role] = true
	}

	for _, role := range nonCriticalRoles {
		if criticalMap[role] {
			t.Errorf("Role %s appears in both critical and non-critical lists", role)
		}
	}
}
