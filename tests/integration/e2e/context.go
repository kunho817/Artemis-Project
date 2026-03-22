// Package e2e provides end-to-end testing infrastructure for Artemis.
// It extends the integration test harness with TUI, EventBus, and pipeline testing support.
package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tui"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// E2ETestContext provides a complete environment for end-to-end testing.
// It extends the integration test harness with TUI simulation and event tracking.
type E2ETestContext struct {
	T                  *testing.T
	Ctx                context.Context
	Cancel             context.CancelFunc
	TempDir            string
	Config             config.Config
	MockBuilder        *harness.MockLLMBuilder
	MemoryStore        memory.MemoryStore
	CheckpointStore    state.CheckpointStore

	// E2E-specific components
	EventBus           *bus.EventBus
	EventCollector     *EventCollector
	TUIApp             *tui.App

	// Test control
	mu                 sync.Mutex
	cleanupFuncs       []func()
}

// Setup creates a new E2E test context with all necessary components.
func Setup(t *testing.T) *E2ETestContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "artemis-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	e2eCtx := &E2ETestContext{
		T:              t,
		Ctx:            ctx,
		Cancel:         cancel,
		TempDir:        tempDir,
		Config:         createE2ETestConfig(tempDir),
		MockBuilder:    harness.NewMockLLMBuilder(),
		EventBus:       bus.NewEventBus(64),
		EventCollector: NewEventCollector(),
		cleanupFuncs:   make([]func(), 0),
	}

	// Register cleanup
	t.Cleanup(func() {
		e2eCtx.Teardown()
	})

	// Initialize memory store
	e2eCtx.MemoryStore = e2eCtx.setupMemoryStore()

	// Initialize checkpoint store
	e2eCtx.CheckpointStore = e2eCtx.setupCheckpointStore()

	// Start event collection
	e2eCtx.startEventCollection()

	return e2eCtx
}

// createE2ETestConfig creates a test configuration optimized for E2E testing.
func createE2ETestConfig(tempDir string) config.Config {
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
			DBPath:  tempDir + "/artemis.db",
		},
	}
}

// setupMemoryStore initializes the memory store for E2E testing.
func (e *E2ETestContext) setupMemoryStore() memory.MemoryStore {
	store, err := memory.NewSQLiteStore(e.Config.Memory.DBPath)
	if err != nil {
		e.T.Fatalf("Failed to create memory store: %v", err)
	}
	return store
}

// setupCheckpointStore initializes the checkpoint store for E2E testing.
func (e *E2ETestContext) setupCheckpointStore() state.CheckpointStore {
	// Return nil for now - checkpoint store is optional in orchestrator
	return nil
}

// startEventCollection starts collecting events from the EventBus.
func (e *E2ETestContext) startEventCollection() {
	e.EventCollector.Start(e.EventBus)
	e.AddCleanup(func() {
		e.EventCollector.Stop()
	})
}

// CreateMockProvider creates a mock LLM provider for testing.
func (e *E2ETestContext) CreateMockProvider(provider string) llm.Provider {
	mock := e.MockBuilder.Build(provider)
	if mockLLM, ok := mock.(*harness.MockLLM); ok {
		mockLLM.SetDefaultResponses()
	}
	return mock
}

// CreateTUIApp creates a new TUI application for testing.
// This is a lightweight version suitable for automated testing.
func (e *E2ETestContext) CreateTUIApp() *tui.App {
	// For testing, we'll create a minimal app configuration
	// The actual TUI initialization is complex and may need simplification for testing
	app := tui.NewApp()
	e.TUIApp = &app
	return &app
}

// SimulateKeyPress simulates a key press in the TUI.
func (e *E2ETestContext) SimulateKeyPress(key tea.KeyMsg) {
	if e.TUIApp == nil {
		e.T.Fatal("TUI app not initialized. Call CreateTUIApp() first.")
	}

	// Update the TUI model with the key message
	// Note: This is a simplified version. The actual implementation may need
	// to handle the model update loop properly.
	_, _ = (*e.TUIApp).Update(key)
}

// EmitTestEvent emits a test event to the EventBus.
func (e *E2ETestContext) EmitTestEvent(eventType bus.EventType, agentName, phase, message string) {
	event := bus.NewEvent(eventType, agentName, phase, message)
	e.EventBus.Emit(event)
}

// GetCollectedEvents returns all collected events.
func (e *E2ETestContext) GetCollectedEvents() []bus.AgentEvent {
	return e.EventCollector.GetEvents()
}

// GetEventsByType returns events of a specific type.
func (e *E2ETestContext) GetEventsByType(eventType bus.EventType) []bus.AgentEvent {
	return e.EventCollector.GetEventsByType(eventType)
}

// AssertEventReceived verifies that an event of the given type was received.
func (e *E2ETestContext) AssertEventReceived(eventType bus.EventType) {
	events := e.GetEventsByType(eventType)
	if len(events) == 0 {
		e.T.Errorf("Expected event type %d, but none was received", eventType)
	}
}

// AssertEventCount verifies the number of events of a specific type.
func (e *E2ETestContext) AssertEventCount(eventType bus.EventType, expected int) {
	events := e.GetEventsByType(eventType)
	actual := len(events)
	if actual != expected {
		e.T.Errorf("Expected %d events of type %d, got %d", expected, eventType, actual)
	}
}

// AssertEventMessage verifies that an event with a specific message was received.
func (e *E2ETestContext) AssertEventMessage(eventType bus.EventType, message string) {
	events := e.GetEventsByType(eventType)
	for _, event := range events {
		if event.Message == message {
			return
		}
	}
	e.T.Errorf("Expected event with message %q, but not found in events of type %d", message, eventType)
}

// WaitForEvent waits for a specific event type within the timeout.
func (e *E2ETestContext) WaitForEvent(eventType bus.EventType, timeout time.Duration) bool {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			events := e.GetEventsByType(eventType)
			if len(events) > 0 {
				return true
			}
		case <-e.Ctx.Done():
			return false
		}
	}
}

// AddCleanup registers a cleanup function to be called during Teardown.
func (e *E2ETestContext) AddCleanup(cleanup func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanupFuncs = append(e.cleanupFuncs, cleanup)
}

// Teardown cleans up all resources used by the E2E test context.
func (e *E2ETestContext) Teardown() {
	// Call registered cleanup functions in reverse order
	e.mu.Lock()
	funcs := make([]func(), len(e.cleanupFuncs))
	copy(funcs, e.cleanupFuncs)
	e.mu.Unlock()

	for i := len(funcs) - 1; i >= 0; i-- {
		if funcs[i] != nil {
			funcs[i]()
		}
	}

	// Close EventBus (may be already closed, so we don't call it here)
	// EventBus is closed by EventCollector.Stop()

	// Cancel context
	if e.Cancel != nil {
		e.Cancel()
	}

	// Remove temp directory
	if e.TempDir != "" {
		os.RemoveAll(e.TempDir)
		e.TempDir = "" // Mark as cleaned up
	}
}

// CreateSessionState creates a new session state for testing.
func (e *E2ETestContext) CreateSessionState() *state.SessionState {
	ss := state.NewSessionState()
	ss.SetPhase("test-e2e")
	return ss
}

// CreateTempFile creates a temporary file with the given content.
func (e *E2ETestContext) CreateTempFile(name, content string) string {
	path := fmt.Sprintf("%s/%s", e.TempDir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.T.Fatalf("Failed to create temp file %s: %v", name, err)
	}
	return path
}

// AssertMockCalled verifies that the mock LLM was called at least once.
func (e *E2ETestContext) AssertMockCalled(provider string) {
	mock := e.MockBuilder.GetMock(provider)
	if mock == nil {
		e.T.Errorf("Mock provider %s was not created", provider)
		return
	}

	if mock.GetCallCount() == 0 {
		e.T.Errorf("Mock provider %s was not called", provider)
	}
}

// RunE2ETest runs an E2E test function with proper setup and teardown.
func (e *E2ETestContext) RunE2ETest(testFunc func(*E2ETestContext)) {
	defer e.MockBuilder.ResetAll()
	testFunc(e)
}

// EventCollector collects events from the EventBus for testing.
type EventCollector struct {
	mu     sync.Mutex
	events []bus.AgentEvent
	active bool
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewEventCollector creates a new event collector.
func NewEventCollector() *EventCollector {
	return &EventCollector{
		events: make([]bus.AgentEvent, 0),
		active: false,
	}
}

// Start begins collecting events from the EventBus.
func (ec *EventCollector) Start(eventBus *bus.EventBus) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.active {
		return
	}

	ec.active = true
	ec.events = make([]bus.AgentEvent, 0)

	ctx, cancel := context.WithCancel(context.Background())
	ec.cancel = cancel

	ec.wg.Add(1)
	go func() {
		defer ec.wg.Done()
		ch := eventBus.Chan()
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}
				ec.mu.Lock()
				ec.events = append(ec.events, event)
				ec.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops collecting events.
func (ec *EventCollector) Stop() {
	ec.mu.Lock()
	if !ec.active {
		ec.mu.Unlock()
		return
	}

	ec.active = false
	cancel := ec.cancel
	ec.mu.Unlock()

	// Cancel the goroutine WITHOUT holding the lock
	if cancel != nil {
		cancel()
	}

	// Wait for goroutine to finish
	ec.wg.Wait()
}

// GetEvents returns all collected events.
func (ec *EventCollector) GetEvents() []bus.AgentEvent {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	// Return a copy to prevent race conditions
	events := make([]bus.AgentEvent, len(ec.events))
	copy(events, ec.events)
	return events
}

// GetEventsByType returns events of a specific type.
func (ec *EventCollector) GetEventsByType(eventType bus.EventType) []bus.AgentEvent {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	var filtered []bus.AgentEvent
	for _, event := range ec.events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// GetEventsByAgent returns events from a specific agent.
func (ec *EventCollector) GetEventsByAgent(agentName string) []bus.AgentEvent {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	var filtered []bus.AgentEvent
	for _, event := range ec.events {
		if event.AgentName == agentName {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// Clear clears all collected events.
func (ec *EventCollector) Clear() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.events = make([]bus.AgentEvent, 0)
}

// Count returns the number of collected events.
func (ec *EventCollector) Count() int {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	return len(ec.events)
}
