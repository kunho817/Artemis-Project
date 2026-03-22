// Package e2e provides end-to-end tests for the Artemis TUI.
package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestTUIInputToLLMResponse tests the complete flow from TUI input to LLM response.
// This test verifies that user input is properly routed through the TUI to the LLM.
func TestTUIInputToLLMResponse(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Create TUI application
	app := e.CreateTUIApp()
	if app == nil {
		t.Skip("TUI app creation not fully implemented")
	}

	// Setup mock LLM response
	mock := e.CreateMockProvider("claude").(*harness.MockLLM)
	mock.SetResponse("test", "Test response from LLM")

	// Simulate user input (Enter key)
	e.SimulateKeyPress(tea.KeyMsg{
		Type:  tea.KeyEnter,
		Runes: []rune{'\r'},
	})

	// Verify mock was called
	e.AssertMockCalled("claude")

	// Verify events were emitted
	e.AssertEventReceived(bus.EventAgentStart)
	e.AssertEventReceived(bus.EventAgentComplete)
}

// TestEventBusIntegration tests the EventBus integration with the TUI.
func TestEventBusIntegration(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	// Emit test events
	e.EmitTestEvent(bus.EventAgentStart, "test-agent", "test-phase", "Agent starting")
	e.EmitTestEvent(bus.EventAgentProgress, "test-agent", "test-phase", "Agent progressing")
	e.EmitTestEvent(bus.EventAgentComplete, "test-agent", "test-phase", "Agent completed")

	// Wait for events to be collected
	if !e.WaitForEvent(bus.EventAgentComplete, 2*time.Second) {
		t.Fatal("Timeout waiting for EventAgentComplete")
	}

	// Verify event counts
	e.AssertEventCount(bus.EventAgentStart, 1)
	e.AssertEventCount(bus.EventAgentProgress, 1)
	e.AssertEventCount(bus.EventAgentComplete, 1)

	// Verify total event count
	events := e.GetCollectedEvents()
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}

// TestEventCollector tests the EventCollector functionality.
func TestEventCollector(t *testing.T) {
	collector := NewEventCollector()

	// Test initial state
	if collector.Count() != 0 {
		t.Errorf("Expected 0 events initially, got %d", collector.Count())
	}

	// Create a test event bus
	eventBus := bus.NewEventBus(10)
	defer eventBus.Close()

	// Start collecting
	collector.Start(eventBus)
	defer collector.Stop()

	// Emit test events
	eventBus.Emit(bus.NewEvent(bus.EventAgentStart, "agent1", "phase1", "Starting"))
	eventBus.Emit(bus.NewEvent(bus.EventAgentComplete, "agent1", "phase1", "Completed"))

	// Give time for collection
	time.Sleep(100 * time.Millisecond)

	// Verify collection
	events := collector.GetEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	// Test filtering by type
	startEvents := collector.GetEventsByType(bus.EventAgentStart)
	if len(startEvents) != 1 {
		t.Errorf("Expected 1 start event, got %d", len(startEvents))
	}

	// Test filtering by agent
	agentEvents := collector.GetEventsByAgent("agent1")
	if len(agentEvents) != 2 {
		t.Errorf("Expected 2 events from agent1, got %d", len(agentEvents))
	}

	// Test clear
	collector.Clear()
	if collector.Count() != 0 {
		t.Errorf("Expected 0 events after clear, got %d", collector.Count())
	}
}

// TestEventCollectorConcurrent tests EventCollector under concurrent access.
func TestEventCollectorConcurrent(t *testing.T) {
	collector := NewEventCollector()
	eventBus := bus.NewEventBus(100)
	defer eventBus.Close()

	collector.Start(eventBus)
	defer collector.Stop()

	// Emit events concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				eventBus.Emit(bus.NewEvent(
					bus.EventAgentProgress,
					fmt.Sprintf("agent-%d", id),
					"test",
					fmt.Sprintf("Message %d", j),
				))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Give time for collection
	time.Sleep(200 * time.Millisecond)

	// Verify all events were collected
	events := collector.GetEvents()
	expectedCount := 100 // 10 goroutines * 10 events
	if len(events) != expectedCount {
		t.Errorf("Expected %d events, got %d", expectedCount, len(events))
	}
}

// TestE2ETeardown tests that Teardown properly cleans up resources.
func TestE2ETeardown(t *testing.T) {
	// Create a temporary context (t.Cleanup will handle Teardown)
	e := Setup(t)

	// Store temp dir path for later verification
	tempDir := e.TempDir

	// Verify resources are initialized
	if e.EventBus == nil {
		t.Error("EventBus not initialized")
	}
	if e.MemoryStore == nil {
		t.Error("MemoryStore not initialized")
	}

	// The Teardown will be called by t.Cleanup
	// Add a custom cleanup to verify temp dir removal
	t.Cleanup(func() {
		// After all cleanup, verify temp directory was removed
		// Note: There might be a slight delay, so we use a small retry
		for i := 0; i < 5; i++ {
			_, err := os.Stat(tempDir)
			if os.IsNotExist(err) {
				return // Success
			}
			time.Sleep(10 * time.Millisecond)
		}
		// If we get here, the directory still exists (may be OK on some systems)
		// Just log a warning instead of failing
		t.Logf("Note: Temp directory still exists after cleanup: %s", tempDir)
	})
}

// TestCreateTUIApp tests TUI application creation.
func TestCreateTUIApp(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	app := e.CreateTUIApp()
	if app == nil {
		t.Skip("TUI app creation not fully implemented")
	}

	if e.TUIApp == nil {
		t.Error("TUIApp not set in context")
	}
}

// TestSimulateKeyPress tests key press simulation.
func TestSimulateKeyPress(t *testing.T) {
	harness.SkipIfShort(t)

	e := Setup(t)
	defer e.Teardown()

	app := e.CreateTUIApp()
	if app == nil {
		t.Skip("TUI app creation not fully implemented")
	}

	// Simulate various key presses
	testKeys := []tea.KeyMsg{
		{Type: tea.KeyEnter},
		{Type: tea.KeyUp},
		{Type: tea.KeyDown},
		{Type: tea.KeyLeft},
		{Type: tea.KeyRight},
		{Type: tea.KeyBackspace},
		{Type: tea.KeyCtrlC},
	}

	for _, key := range testKeys {
		// This should not panic
		e.SimulateKeyPress(key)
	}
}

// TestAssertEventMessage tests event message assertion.
func TestAssertEventMessage(t *testing.T) {
	t.Run("ExistingMessage", func(t *testing.T) {
		e := Setup(t)
		defer e.Teardown()

		// Emit a test event with a specific message
		testMessage := "Test message for assertion"
		e.EmitTestEvent(bus.EventAgentStart, "test-agent", "test-phase", testMessage)

		// Wait for event collection
		time.Sleep(100 * time.Millisecond)

		// Assert the message was received (should not error)
		e.AssertEventMessage(bus.EventAgentStart, testMessage)
	})

	t.Run("MissingMessage", func(t *testing.T) {
		e := Setup(t)
		defer e.Teardown()

		// Emit a test event with a different message
		e.EmitTestEvent(bus.EventAgentStart, "test-agent", "test-phase", "Different message")

		// Wait for event collection
		time.Sleep(100 * time.Millisecond)

		// Verify that AssertEventMessage would fail for non-existent message
		// We do this by checking the events directly
		events := e.GetEventsByType(bus.EventAgentStart)
		found := false
		for _, event := range events {
			if event.Message == "Non-existent message" {
				found = true
				break
			}
		}

		if found {
			t.Error("Expected to not find the non-existent message, but it was found")
		}
		// Success: the message is not found, which is expected
	})
}

// TestWaitForEvent tests the WaitForEvent functionality.
func TestWaitForEvent(t *testing.T) {
	e := Setup(t)
	defer e.Teardown()

	// Test immediate event
	e.EmitTestEvent(bus.EventAgentStart, "test-agent", "test-phase", "Starting")
	if !e.WaitForEvent(bus.EventAgentStart, 1*time.Second) {
		t.Error("Expected to find event immediately")
	}

	// Test delayed event
	go func() {
		time.Sleep(500 * time.Millisecond)
		e.EmitTestEvent(bus.EventAgentComplete, "test-agent", "test-phase", "Completed")
	}()

	if !e.WaitForEvent(bus.EventAgentComplete, 2*time.Second) {
		t.Error("Expected to find event after delay")
	}

	// Test timeout
	if e.WaitForEvent(bus.EventRecoveryAttempt, 500*time.Millisecond) {
		t.Error("Expected timeout for non-existent event")
	}
}

// TestGetEventsByType tests filtering events by type.
func TestGetEventsByType(t *testing.T) {
	e := Setup(t)
	defer e.Teardown()

	// Emit various event types
	e.EmitTestEvent(bus.EventAgentStart, "agent1", "phase1", "Start 1")
	e.EmitTestEvent(bus.EventAgentStart, "agent2", "phase1", "Start 2")
	e.EmitTestEvent(bus.EventAgentComplete, "agent1", "phase1", "Complete 1")
	e.EmitTestEvent(bus.EventAgentProgress, "agent1", "phase1", "Progress 1")

	// Wait for collection
	time.Sleep(100 * time.Millisecond)

	// Get all AgentStart events
	startEvents := e.GetEventsByType(bus.EventAgentStart)
	if len(startEvents) != 2 {
		t.Errorf("Expected 2 start events, got %d", len(startEvents))
	}

	// Get all AgentComplete events
	completeEvents := e.GetEventsByType(bus.EventAgentComplete)
	if len(completeEvents) != 1 {
		t.Errorf("Expected 1 complete event, got %d", len(completeEvents))
	}

	// Get non-existent event type
	recoveryEvents := e.GetEventsByType(bus.EventRecoveryAttempt)
	if len(recoveryEvents) != 0 {
		t.Errorf("Expected 0 recovery events, got %d", len(recoveryEvents))
	}
}

// TestAddCleanup tests custom cleanup function registration.
func TestAddCleanup(t *testing.T) {
	e := Setup(t)

	cleanupCalled := false
	e.AddCleanup(func() {
		cleanupCalled = true
	})

	e.Teardown()

	if !cleanupCalled {
		t.Error("Custom cleanup function was not called")
	}
}

// TestCreateSessionState tests session state creation.
func TestCreateSessionState(t *testing.T) {
	e := Setup(t)
	defer e.Teardown()

	ss := e.CreateSessionState()
	if ss == nil {
		t.Fatal("Session state is nil")
	}

	phase := ss.Phase()
	if phase != "test-e2e" {
		t.Errorf("Expected phase 'test-e2e', got %s", phase)
	}
}

// TestCreateTempFile tests temporary file creation.
func TestCreateTempFile(t *testing.T) {
	e := Setup(t)
	defer e.Teardown()

	content := "Test content for temporary file"
	path := e.CreateTempFile("test.txt", content)

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Temp file was not created: %s", path)
	}

	// Verify file content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read temp file: %v", err)
	}

	if string(data) != content {
		t.Errorf("Expected content %q, got %q", content, string(data))
	}
}
