package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
)

// Role defines an agent's specialization.
type Role string

const (
	RoleOrchestrator Role = "orchestrator"
	RolePlanner      Role = "planner"
	RoleAnalyzer     Role = "analyzer"
	RoleSearcher     Role = "searcher"
	RoleExplorer     Role = "explorer"
	RoleArchitect    Role = "architect"
	RoleCoder        Role = "coder"
	RoleDesigner     Role = "designer"
	RoleEngineer     Role = "engineer"
	RoleQA           Role = "qa"
	RoleTester       Role = "tester"
)

// AllRoles returns all defined agent roles.
func AllRoles() []Role {
	return []Role{
		RoleOrchestrator, RolePlanner, RoleAnalyzer, RoleSearcher,
		RoleExplorer, RoleArchitect, RoleCoder, RoleDesigner,
		RoleEngineer, RoleQA, RoleTester,
	}
}

// Agent is the interface all specialized agents implement.
type Agent interface {
	// Name returns the agent's display name.
	Name() string

	// Role returns the agent's specialization.
	Role() Role

	// Run executes the agent's task against the shared session state.
	// It reads context from state, calls the LLM, and writes results back.
	Run(ctx context.Context, ss *state.SessionState) error

	// Critical returns whether pipeline should halt on this agent's failure.
	Critical() bool

	// SetTask overrides the agent's default task with an Orchestrator-assigned task.
	SetTask(task string)

	// SetCritical overrides the agent's default criticality.
	SetCritical(critical bool)

	// SetMemory attaches a persistent memory store to this agent.
	SetMemory(store memory.MemoryStore)

	// SetRepoMap attaches a repo-map store for codebase structure awareness.
	SetRepoMap(rm *memory.RepoMapStore)

	// SetMaxToolIter sets the maximum tool iteration count (0 = unlimited).
	SetMaxToolIter(n int)
}

// BaseAgent provides shared logic for all agents.
type BaseAgent struct {
	name         string
	role         Role
	provider     llm.Provider
	system       string // system prompt defining persona
	eventBus     *bus.EventBus
	critical     bool
	overrideTask string               // task assigned by Orchestrator (overrides default)
	toolExec     *tools.ToolExecutor  // tool executor for file/shell operations
	memStore     memory.MemoryStore   // persistent memory (nil if disabled)
	maxToolIter  int                  // max tool iterations (0 = unlimited)
	repoMap      *memory.RepoMapStore // Phase 3: repo-map (nil if disabled)
}

// NewBaseAgent creates a new base agent.
func NewBaseAgent(name string, role Role, provider llm.Provider, systemPrompt string, eb *bus.EventBus, critical bool, toolExec *tools.ToolExecutor) BaseAgent {
	return BaseAgent{
		name:     name,
		role:     role,
		provider: provider,
		system:   systemPrompt,
		eventBus: eb,
		critical: critical,
		toolExec: toolExec,
	}
}

func (b *BaseAgent) Name() string                      { return b.name }
func (b *BaseAgent) Role() Role                        { return b.role }
func (b *BaseAgent) Critical() bool                    { return b.critical }
func (b *BaseAgent) SetTask(task string)               { b.overrideTask = task }
func (b *BaseAgent) SetCritical(c bool)                { b.critical = c }
func (b *BaseAgent) OverrideTask() string              { return b.overrideTask }
func (b *BaseAgent) ToolExecutor() *tools.ToolExecutor { return b.toolExec }

// SetMemory attaches a MemoryStore to this agent for persistent memory access.
// Called during TUI initialization when memory is enabled.
func (b *BaseAgent) SetMemory(store memory.MemoryStore) { b.memStore = store }

// SetMaxToolIter sets the maximum tool iteration count (0 = unlimited).
func (b *BaseAgent) SetMaxToolIter(n int) { b.maxToolIter = n }

// SetRepoMap attaches a RepoMapStore for codebase structure awareness.
func (b *BaseAgent) SetRepoMap(rm *memory.RepoMapStore) { b.repoMap = rm }

// EmitStart notifies the TUI that this agent started.
func (b *BaseAgent) EmitStart(phase string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentStart, b.name, phase, "Starting..."))
	}
}

// EmitProgress notifies the TUI of intermediate progress.
func (b *BaseAgent) EmitProgress(phase, msg string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentProgress, b.name, phase, msg))
	}
}

// EmitComplete notifies the TUI that this agent finished.
func (b *BaseAgent) EmitComplete(phase, msg string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentComplete, b.name, phase, msg))
	}
}

// EmitFail notifies the TUI that this agent failed.
func (b *BaseAgent) EmitFail(phase string, err error) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.ErrorEvent(b.name, phase, err))
	}
}

// EmitOutput sends the agent's LLM response to the TUI for chat display.
func (b *BaseAgent) EmitOutput(phase, content string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentOutput, b.name, phase, content))
	}
}

// emitFileChanged notifies the TUI that a file was modified by a tool.
func (b *BaseAgent) emitFileChanged(phase, filePath string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventFileChanged, b.name, phase, filePath))
	}
}

// EmitStreamStart notifies the TUI that this agent is beginning to stream a response.
func (b *BaseAgent) EmitStreamStart(phase string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentStreamStart, b.name, phase, ""))
	}
}

// EmitStreamChunk sends a streaming text chunk to the TUI for real-time display.
func (b *BaseAgent) EmitStreamChunk(phase, content string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentStreamChunk, b.name, phase, content))
	}
}

// EmitStreamDone notifies the TUI that streaming is complete.
func (b *BaseAgent) EmitStreamDone(phase string) {
	if b.eventBus != nil {
		b.eventBus.Emit(bus.NewEvent(bus.EventAgentStreamDone, b.name, phase, ""))
	}
}

// EmitUsage sends token usage information to the TUI.
func (b *BaseAgent) EmitUsage(phase string, usage *llm.TokenUsage) {
	if b.eventBus != nil && usage != nil {
		e := bus.NewEvent(bus.EventAgentUsage, b.name, phase, "")
		e.Data = usage
		b.eventBus.Emit(e)
	}
}

// CallLLM sends a prompt to the agent's LLM provider with the system prompt.
func (b *BaseAgent) CallLLM(ctx context.Context, userPrompt string) (string, error) {
	if b.provider == nil {
		return "", fmt.Errorf("agent %s: no LLM provider configured", b.name)
	}

	var messages []llm.Message

	// Add system prompt if set
	if b.system != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: b.system,
		})
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userPrompt,
	})

	return b.provider.Send(ctx, messages)
}

// CallLLMWithTools sends a prompt to the LLM via streaming and handles tool-use loops.
// Each LLM call is streamed: chunks are emitted to the TUI for real-time display,
// while the full response is accumulated internally for tool_use parsing.
// If the response contains <tool_use> blocks, tools are executed and results are
// fed back until the LLM produces a final response without tool calls.
func (b *BaseAgent) CallLLMWithTools(ctx context.Context, userPrompt string, phase string) (string, error) {
	if b.provider == nil {
		return "", fmt.Errorf("agent %s: no LLM provider configured", b.name)
	}

	var messages []llm.Message
	if b.system != "" {
		systemPrompt := b.system
		if b.toolExec != nil {
			systemPrompt += b.toolExec.ToolDescriptions()
		}
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userPrompt,
	})

	maxIter := b.maxToolIter
	if maxIter <= 0 {
		maxIter = 0 // unlimited
	}
	totalUsage := &llm.TokenUsage{}
	for i := 0; maxIter == 0 || i < maxIter; i++ {
		// Stream the LLM response, accumulating the full text
		response, usage, err := b.streamAndAccumulate(ctx, messages, phase)
		if err != nil {
			return "", err
		}
		totalUsage.Add(usage)

		// Parse tool invocations from the accumulated response
		invocations, cleanResponse := tools.ParseToolInvocations(response)
		if len(invocations) == 0 {
			// No tool calls — this is the final response
			if totalUsage.TotalTokens > 0 {
				b.EmitUsage(phase, totalUsage)
			}
			return cleanResponse, nil
		}

		// Execute tools and collect results
		var resultParts []string
		for _, inv := range invocations {
			b.EmitProgress(phase, fmt.Sprintf("Using tool: %s", inv.Tool))

			result, _ := b.toolExec.Execute(ctx, inv.Tool, inv.Params)

			// Emit file change events
			for _, f := range result.FilesChanged {
				b.emitFileChanged(phase, f)
			}

			resultParts = append(resultParts, tools.FormatToolResult(inv.Tool, result))
		}

		// Feed results back to LLM for next iteration
		messages = append(messages, llm.Message{Role: "assistant", Content: response})
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: strings.Join(resultParts, "\n\n"),
		})
	}

	if maxIter > 0 {
		return "", fmt.Errorf("agent %s: max tool iterations (%d) reached", b.name, maxIter)
	}
	return "", fmt.Errorf("agent %s: tool loop did not terminate", b.name)
}

// streamAndAccumulate streams an LLM response, emitting chunks to the TUI
// while accumulating the full response text. Returns the complete response.
func (b *BaseAgent) streamAndAccumulate(ctx context.Context, messages []llm.Message, phase string) (string, *llm.TokenUsage, error) {
	ch, err := b.provider.Stream(ctx, messages)
	if err != nil {
		// Fallback to Send() if streaming fails (e.g. provider doesn't support it)
		resp, sendErr := b.provider.Send(ctx, messages)
		if sendErr != nil {
			return "", nil, sendErr
		}
		// Emit the full response as a single output event (non-streaming fallback)
		b.EmitOutput(phase, resp)
		return resp, nil, nil
	}

	b.EmitStreamStart(phase)

	var buf strings.Builder
	var usage *llm.TokenUsage
	for chunk := range ch {
		if chunk.Error != nil {
			b.EmitStreamDone(phase)
			return "", nil, chunk.Error
		}
		if chunk.Done {
			usage = chunk.Usage
			break
		}
		if chunk.Content != "" {
			buf.WriteString(chunk.Content)
			b.EmitStreamChunk(phase, chunk.Content)
		}
	}

	b.EmitStreamDone(phase)
	return buf.String(), usage, nil
}

// BuildPromptWithContext creates a prompt that includes relevant session state,
// persistent memory facts, and conversation history from prior turns.
func (b *BaseAgent) BuildPromptWithContext(ss *state.SessionState, task string) string {
	var parts []string

	// Include relevant persistent memory facts (role-filtered)
	if b.memStore != nil {
		opts := memory.QueryOpts{
			Query: task,
			Tags:  memory.RoleTagMap[string(b.role)],
			Limit: 15,
		}
		if facts, err := b.memStore.QueryFacts(context.Background(), opts); err == nil && len(facts) > 0 {
			var factLines []string
			for _, f := range facts {
				factLines = append(factLines, fmt.Sprintf("- %s", f.Content))
				// Increment usage counter for retrieved facts
				b.memStore.IncrementFactUsage(context.Background(), f.ID)
			}
			parts = append(parts, "## Project Knowledge\n"+strings.Join(factLines, "\n"))
		}
	}

	// Phase 3: Include relevant codebase structure from repo-map
	if b.repoMap != nil {
		filter := memory.RepoMapRoleFilter[string(b.role)]
		if symbols, err := b.repoMap.QuerySymbols(context.Background(), task, 100); err == nil && len(symbols) > 0 {
			// Apply role-based filter
			var filtered []memory.Symbol
			for _, s := range symbols {
				if filter == nil || filter(s) {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > 0 {
				formatted := b.repoMap.FormatRepoMap(filtered, 2048)
				if formatted != "" {
					parts = append(parts, "## Codebase Structure\n"+formatted)
				}
			}
		}
	}

	// Include conversation history if available (multi-turn context)
	history := ss.HistorySummary()
	if history != "" {
		parts = append(parts, "## Conversation History\n"+history)
	}

	// Include artifacts from current pipeline run
	summary := ss.Summary()
	if summary != "" {
		parts = append(parts, "## Context from previous agents\n"+summary)
	}

	// Always include the task
	parts = append(parts, "## Your task\n"+task)

	return strings.Join(parts, "\n")
}
