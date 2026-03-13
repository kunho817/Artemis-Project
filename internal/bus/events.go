package bus

import "time"

// EventType identifies the kind of agent event.
type EventType int

const (
	EventAgentStart            EventType = iota // Agent began execution
	EventAgentProgress                          // Agent intermediate update
	EventAgentComplete                          // Agent finished successfully
	EventAgentFail                              // Agent encountered an error
	EventAgentOutput                            // Agent produced displayable output
	EventPhaseStart                             // Pipeline phase began
	EventPhaseComplete                          // Pipeline phase finished
	EventPipelineDone                           // Entire pipeline completed
	EventOrchestratorStart                      // Orchestrator began planning
	EventOrchestratorPlanReady                  // Orchestrator produced an execution plan
	EventStepStart                              // Dynamic plan step began
	EventStepComplete                           // Dynamic plan step finished
	EventFileChanged                            // Agent modified a file via tool
	EventAgentStreamStart                       // Agent began streaming a response
	EventAgentStreamChunk                       // Agent streaming chunk (partial text)
	EventAgentStreamDone                        // Agent streaming completed
	EventAgentUsage                             // Agent reported token usage
	EventBackgroundTaskStart            // Background task started running
	EventBackgroundTaskComplete          // Background task finished successfully
	EventBackgroundTaskFail              // Background task encountered an error
	EventAgentWarn                       // Non-critical agent failure (warning)
	EventRecoveryAttempt                 // Recovery system attempting to fix a failure
)

// AgentEvent carries status updates from agents to the TUI.
type AgentEvent struct {
	Type      EventType
	AgentName string
	Phase     string
	Message   string
	Data      interface{} // optional structured payload (e.g., *llm.TokenUsage)
	Error     error
	Timestamp time.Time
}

// NewEvent creates an event with the current timestamp.
func NewEvent(t EventType, agentName, phase, message string) AgentEvent {
	return AgentEvent{
		Type:      t,
		AgentName: agentName,
		Phase:     phase,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// ErrorEvent creates a failure event.
func ErrorEvent(agentName, phase string, err error) AgentEvent {
	return AgentEvent{
		Type:      EventAgentFail,
		AgentName: agentName,
		Phase:     phase,
		Message:   err.Error(),
		Error:     err,
		Timestamp: time.Now(),
	}
}

// EventBus is a channel-based event bus for agent → TUI communication.
type EventBus struct {
	ch chan AgentEvent
}

// NewEventBus creates a new event bus with a buffered channel.
func NewEventBus(bufSize int) *EventBus {
	return &EventBus{
		ch: make(chan AgentEvent, bufSize),
	}
}

// Emit sends an event to the bus (non-blocking if buffer has space).
func (eb *EventBus) Emit(event AgentEvent) {
	select {
	case eb.ch <- event:
	default:
		// Drop event if buffer full — prevents agent goroutine from blocking
	}
}

// Chan returns the receive-only channel for the TUI to listen on.
func (eb *EventBus) Chan() <-chan AgentEvent {
	return eb.ch
}

// Close closes the event bus channel.
func (eb *EventBus) Close() {
	close(eb.ch)
}
