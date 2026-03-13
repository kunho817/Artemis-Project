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
	RoleScout       Role = "scout"
	RoleConsultant  Role = "consultant"
)

// AllRoles returns all defined agent roles.
func AllRoles() []Role {
	return []Role{
		RoleOrchestrator, RolePlanner, RoleAnalyzer, RoleSearcher,
		RoleExplorer, RoleArchitect, RoleCoder, RoleDesigner,
		RoleEngineer, RoleQA, RoleTester, RoleScout, RoleConsultant,
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

	// SetCategory assigns a task category for model/prompt override.
	SetCategory(cat TaskCategory)

	// SetSkills attaches domain-specific skills to inject into prompts.
	SetSkills(skills []*Skill)

	// SetHistoryWindow attaches a shared history window for token-aware history management.
	SetHistoryWindow(hw *HistoryWindow)

	// OverrideTask returns the Orchestrator-assigned task override (empty if none).
	OverrideTask() string
}

// BaseAgent provides shared logic for all agents.
type BaseAgent struct {
	name           string
	role           Role
	provider       llm.Provider
	system         string // system prompt defining persona
	eventBus       *bus.EventBus
	critical       bool
	overrideTask   string               // task assigned by Orchestrator (overrides default)
	toolExec       *tools.ToolExecutor  // tool executor for file/shell operations
	memStore       memory.MemoryStore   // persistent memory (nil if disabled)
	maxToolIter    int                  // max tool iterations (0 = unlimited)
	repoMap        *memory.RepoMapStore // Phase 3: repo-map (nil if disabled)
	category       TaskCategory         // Phase 4: task category (empty = use role defaults)
	skills         []*Skill             // Phase 4: loaded skill content for prompt injection
	historyWindow  *HistoryWindow       // Phase C: token-aware history management (nil = legacy)
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

// SetCategory assigns a task category that overrides the role's default provider/model.
func (b *BaseAgent) SetCategory(cat TaskCategory) { b.category = cat }

// SetSkills attaches loaded skill content for injection into prompts.
func (b *BaseAgent) SetSkills(skills []*Skill) { b.skills = skills }

// SetHistoryWindow attaches a shared history window for token-aware history management.
// When set, BuildPromptWithContext uses the HistoryWindow instead of SessionState.HistorySummary().
func (b *BaseAgent) SetHistoryWindow(hw *HistoryWindow) { b.historyWindow = hw }

// Category returns the agent's assigned task category (may be empty).
func (b *BaseAgent) Category() TaskCategory { return b.category }

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
// Uses ContextBudget for token-aware assembly when TokenCounter is available,
// falling back to legacy concatenation otherwise.
func (b *BaseAgent) BuildPromptWithContext(ss *state.SessionState, task string) string {
	counter, err := llm.GetTokenCounter()
	if err != nil || counter == nil {
		return b.buildPromptLegacy(ss, task)
	}

	// Determine model for budget allocation
	model := ""
	if b.provider != nil {
		model = b.provider.Model()
	}
	budget := llm.NewContextBudgetForModel(model, counter)

	// P0: Task — always included, never truncated
	budget.Reserve("task", "## Your task\n"+task)

	// P2: Recent conversation history
	if b.historyWindow != nil {
		recent := b.historyWindow.RecentFormatted()
		if recent != "" {
			budget.Allocate(llm.P2, "recent-history", "## Conversation History\n"+recent, 16_000)
		}
	} else {
		// Legacy fallback: use SessionState history (unbounded)
		history := ss.HistorySummary()
		if history != "" {
			budget.Allocate(llm.P2, "history", "## Conversation History\n"+history, 16_000)
		}
	}

	// P3: Artifacts from current pipeline run
	summary := ss.Summary()
	if summary != "" {
		budget.Allocate(llm.P3, "artifacts", "## Context from previous agents\n"+summary, 16_000)
	}

	// P4: Skills + Category behavioral context
	if len(b.skills) > 0 {
		skillContent := FormatSkillsContent(b.skills)
		if skillContent != "" {
			budget.Allocate(llm.P4, "skills", "## Skills\n"+skillContent, 8_000)
		}
	}
	if b.category != "" {
		if catPrompt := PromptForCategory(b.category); catPrompt != "" {
			budget.Allocate(llm.P4, "category", catPrompt, 4_000)
		}
	}

	// P5: Repo-map + Project Facts
	if b.repoMap != nil {
		filter := memory.RepoMapRoleFilter[string(b.role)]
		if symbols, err := b.repoMap.QuerySymbols(context.Background(), task, 100); err == nil && len(symbols) > 0 {
			var filtered []memory.Symbol
			for _, s := range symbols {
				if filter == nil || filter(s) {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) > 0 {
				formatted := b.repoMap.FormatRepoMap(filtered, 2048)
				if formatted != "" {
					budget.Allocate(llm.P5, "repomap", "## Codebase Structure\n"+formatted, 4_000)
				}
			}
		}
	}
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
				b.memStore.IncrementFactUsage(context.Background(), f.ID)
			}
			budget.Allocate(llm.P5, "facts", "## Project Knowledge\n"+strings.Join(factLines, "\n"), 4_000)
		}
	}

	// P6: Summarized older history (from HistoryWindow compaction)
	if b.historyWindow != nil {
		summarized := b.historyWindow.Summarized()
		if summarized != "" {
			budget.Allocate(llm.P6, "older-history", "## Earlier Conversation Summary\n"+summarized, 0)
		}
	}

	prompt, _ := budget.Build()
	return prompt
}

// buildPromptLegacy is the pre-Phase-C concatenation-based prompt builder.
// Used as fallback when TokenCounter is unavailable.
func (b *BaseAgent) buildPromptLegacy(ss *state.SessionState, task string) string {
	var parts []string

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
				b.memStore.IncrementFactUsage(context.Background(), f.ID)
			}
			parts = append(parts, "## Project Knowledge\n"+strings.Join(factLines, "\n"))
		}
	}
	if b.repoMap != nil {
		filter := memory.RepoMapRoleFilter[string(b.role)]
		if symbols, err := b.repoMap.QuerySymbols(context.Background(), task, 100); err == nil && len(symbols) > 0 {
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
	if len(b.skills) > 0 {
		skillContent := FormatSkillsContent(b.skills)
		if skillContent != "" {
			parts = append(parts, "## Skills\n"+skillContent)
		}
	}
	history := ss.HistorySummary()
	if history != "" {
		parts = append(parts, "## Conversation History\n"+history)
	}
	summary := ss.Summary()
	if summary != "" {
		parts = append(parts, "## Context from previous agents\n"+summary)
	}
	if b.category != "" {
		if catPrompt := PromptForCategory(b.category); catPrompt != "" {
			parts = append(parts, catPrompt)
		}
	}
	parts = append(parts, "## Your task\n"+task)
	return strings.Join(parts, "\n")
}
