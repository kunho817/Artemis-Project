package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/orchestrator"
)

// AgentEventMsg wraps a bus.AgentEvent for the bubbletea Update loop.
type AgentEventMsg struct {
	Event bus.AgentEvent
}

// PipelineCompleteMsg signals the pipeline has finished.
type PipelineCompleteMsg struct {
	Result orchestrator.EngineResult
}

// waitForEvent returns a tea.Cmd that waits for the next event from the bus.
func (a App) waitForEvent(eb *bus.EventBus) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eb.Chan()
		if !ok {
			// Channel closed — pipeline done
			return PipelineCompleteMsg{}
		}
		return AgentEventMsg{Event: event}
	}
}

// handleAgentEvent processes an agent event and updates the activity panel.
func (a App) handleAgentEvent(msg AgentEventMsg) (tea.Model, tea.Cmd) {
	event := msg.Event

	switch event.Type {
	case bus.EventPhaseStart:
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("Phase: %s", event.Phase),
		})

	case bus.EventStepStart:
		var current, total int
		if n, err := fmt.Sscanf(event.Message, "Step %d of %d", &current, &total); err == nil && n == 2 {
			a.activity.SetPipelineProgress(current, total)
			a.statusBar.SetPipelineProgress(current, total)
		}
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   event.Message,
		})

	case bus.EventAgentStart:
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  %s: starting...", event.AgentName),
		})

	case bus.EventAgentProgress:
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  %s: %s", event.AgentName, event.Message),
		})

	case bus.EventAgentOutput:
		displayName := agentDisplayName(event.AgentName)
		a.chat.AddMessage(ChatMessage{
			Role:      RoleAgent,
			Content:   event.Message,
			AgentName: displayName,
			AgentRole: event.AgentName, // role string used for styling
		})
		// Accumulate for conversation history
		a.pipelineOutputs = append(a.pipelineOutputs,
			fmt.Sprintf("[%s]\n%s", displayName, event.Message))

	case bus.EventFileChanged:
		a.activity.AddFileChange(FileChange{
			Path:  event.Message,
			IsNew: false, // we don't track new vs modified from event
		})
		// Track in persistent memory
		if a.memStore != nil {
			a.memStore.TrackFile(context.Background(), &memory.FileRecord{
				Path:     event.Message,
				LastRole: event.AgentName,
			})
		}
	case bus.EventAgentComplete:
		tokens := a.activity.GetAgentTokens(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusDone,
			Text:   fmt.Sprintf("  %s: done", event.AgentName),
			Tokens: tokens,
		})

	case bus.EventAgentFail:
		errText := event.Message
		// Classify engine errors for user-friendly display
		if event.AgentName == "engine" {
			// Engine errors are LLM/provider errors — classify them
			providerName := a.cfg.ProviderForRole(event.Phase)
			if providerName == "" {
				providerName = a.cfg.ActiveProvider
			}
			fe := llm.ClassifyError(fmt.Errorf("%s", event.Message), providerName)
			errText = fe.Error()
		}
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fmt.Sprintf("  %s: %s", event.AgentName, errText),
		})

	case bus.EventPhaseComplete:
		a.activity.AddActivity(ActivityItem{
			Status: StatusDone,
			Text:   fmt.Sprintf("Phase: %s - %s", event.Phase, event.Message),
		})

	case bus.EventStepComplete:
		a.activity.AddActivity(ActivityItem{
			Status: StatusDone,
			Text:   event.Message,
		})

	case bus.EventPipelineDone:
		a.activity.ClearPipelineProgress()
		a.statusBar.SetPipelineProgress(0, 0)
		a.activity.AddActivity(ActivityItem{
			Status: StatusDone,
			Text:   "Execution complete",
		})

	case bus.EventAgentStreamStart:
		// Agent is starting to stream — add an empty agent message for live updates
		displayName := agentDisplayName(event.AgentName)
		msgIdx := a.chat.MessageCount()
		a.chat.AddMessage(ChatMessage{
			Role:      RoleAgent,
			Content:   "",
			AgentName: displayName,
			AgentRole: event.AgentName,
		})
		if a.agentStreams == nil {
			a.agentStreams = make(map[string]*agentStreamInfo)
		}
		a.agentStreams[event.AgentName] = &agentStreamInfo{
			name:     displayName,
			role:     event.AgentName,
			content:  "",
			msgIndex: msgIdx,
		}
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  %s: streaming...", displayName),
		})

	case bus.EventAgentStreamChunk:
		// Append streaming chunk to the correct agent's chat message
		info := a.agentStreams[event.AgentName]
		if info != nil {
			info.content += event.Message
			a.chat.AppendToMessage(info.msgIndex, event.Message)
		}

	case bus.EventAgentStreamDone:
		// Streaming complete — save accumulated content to pipeline outputs
		info := a.agentStreams[event.AgentName]
		if info != nil {
			a.chat.FinishStreamingAt(info.msgIndex)
			if info.content != "" {
				a.pipelineOutputs = append(a.pipelineOutputs,
					fmt.Sprintf("[%s]\n%s", info.name, info.content))
			}
			delete(a.agentStreams, event.AgentName)
		}

	case bus.EventAgentUsage:
		if usage, ok := event.Data.(*llm.TokenUsage); ok && usage != nil {
			model := a.modelForRole(event.AgentName)
			a.addUsage(usage, model)
			a.activity.SetAgentTokens(event.AgentName, usage.TotalTokens)
		}

	case bus.EventBackgroundTaskStart:
		displayName := agentDisplayName(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  ⚡ %s [bg]: %s", displayName, event.Message),
		})

	case bus.EventBackgroundTaskComplete:
		displayName := agentDisplayName(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusDone,
			Text:   fmt.Sprintf("  ⚡ %s [bg]: %s", displayName, event.Message),
		})

	case bus.EventBackgroundTaskFail:
		displayName := agentDisplayName(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fmt.Sprintf("  ⚡ %s [bg]: %s", displayName, event.Message),
		})

	case bus.EventAgentWarn:
		displayName := agentDisplayName(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fmt.Sprintf("  ⚠ %s: %s", displayName, event.Message),
		})

	case bus.EventRecoveryAttempt:
		displayName := agentDisplayName(event.AgentName)
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  🔄 %s: %s", displayName, event.Message),
		})

	case bus.EventReviewLoop:
		a.activity.AddActivity(ActivityItem{
			Status: StatusRunning,
			Text:   fmt.Sprintf("  🔁 %s", event.Message),
		})

	case bus.EventTestResults:
		if tr, ok := event.Data.(*TestResult); ok {
			a.activity.SetTestResults(tr)
		}
	}

	// Continue listening for more events
	if a.eventBus != nil {
		return a, a.waitForEvent(a.eventBus)
	}
	return a, nil
}

// handlePipelineComplete processes the pipeline completion signal.
// Note: PipelineCompleteMsg may have an empty Result (from channel close).
// The EventPipelineDone event already handles activity panel notification.
func (a App) handlePipelineComplete(msg PipelineCompleteMsg) (tea.Model, tea.Cmd) {
	a.pipelineRunning = false
	a.activity.ClearPipelineProgress()
	a.statusBar.SetPipelineProgress(0, 0)
	a.layoutMode = LayoutSingle
	a.focused = FocusChat
	a.cancelPipeline = nil
	a.pipelineWg = nil
	a.eventBus = nil
	a.recoveryBridge = nil
	a.recoveryQueue = nil
	a.recalcLayout()

	// Save accumulated pipeline output to conversation history
	agentCount := len(a.pipelineOutputs)
	if agentCount > 0 {
		combined := strings.Join(a.pipelineOutputs, "\n\n")
		a.addToHistory(llm.Message{
			Role:    "assistant",
			Content: combined,
		})
		a.pipelineOutputs = nil
		a.saveMessageToDB("assistant", combined, "pipeline")
	}

	result := msg.Result
	if result.Completed {
		summary := "Execution completed successfully."
		if agentCount > 0 {
			summary = fmt.Sprintf("Execution completed successfully (%d agent outputs).", agentCount)
		}
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: summary,
		})
	} else if result.HaltedAt != "" {
		// Only show error when there's an actual halt (not empty channel-close result)
		errMsg := "unknown error"
		if result.HaltError != nil {
			errMsg = result.HaltError.Error()
		}
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Execution halted at %q: %s", result.HaltedAt, errMsg),
		})
	}
	// else: empty result from channel close — EventPipelineDone already handled notification

	return a, nil
}

// agentDisplayName converts a role string to a display-friendly name.
func agentDisplayName(role string) string {
	switch role {
	case "analyzer":
		return "Analyzer"
	case "searcher":
		return "Searcher"
	case "explorer":
		return "Explorer"
	case "planner":
		return "Planner"
	case "architect":
		return "Architect"
	case "coder":
		return "Coder"
	case "designer":
		return "Designer"
	case "engineer":
		return "Engineer"
	case "qa":
		return "QA"
	case "tester":
		return "Tester"
	case "scout":
		return "Scout"
	case "consultant":
		return "Consultant"
	default:
		return cases.Title(language.English).String(role)
	}
}

func (a *App) modelForRole(role string) string {
	// Check for per-role model override first
	if override := a.cfg.ModelForRole(role); override != "" {
		return override
	}
	providerName := a.cfg.ProviderForRole(role)
	if providerName == "" {
		return a.cfg.GetActiveModel()
	}
	if providerName == "glm" {
		return a.cfg.GLM.Model
	}
	if p := a.cfg.GetProvider(providerName); p != nil {
		return p.Model
	}
	return a.cfg.GetActiveModel()
}
