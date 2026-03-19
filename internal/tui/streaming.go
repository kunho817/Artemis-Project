package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/llm"
)

// LLMResponseMsg carries an LLM response back to the Update loop.
type LLMResponseMsg struct {
	Content string
	Error   error
}

// StreamChunkMsg carries a streaming chunk from the LLM.
type StreamChunkMsg struct {
	Content string
	Done    bool
	Error   error
	Usage   *llm.TokenUsage
}

// streamStartMsg carries the stream channel from the initial Stream() call.
type streamStartMsg struct {
	channel <-chan llm.StreamChunk
}

// handleSingleSubmit sends a message directly to the active LLM provider.
func (a App) handleSingleSubmit(_ string) (tea.Model, tea.Cmd) {
	// Check if provider is available
	if a.provider == nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "No LLM provider configured. Press Ctrl+S to set up API keys.",
		})
		return a, nil
	}

	// Check API key
	hasKey := false
	if a.cfg.ActiveProvider == "glm" {
		hasKey = a.cfg.GetGLM().APIKey != ""
	} else {
		prov := a.cfg.GetProvider(a.cfg.ActiveProvider)
		hasKey = prov != nil && prov.APIKey != ""
	}
	if !hasKey {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "API key not set for " + a.cfg.ActiveProvider + ". Press Ctrl+S to configure.",
		})
		return a, nil
	}

	// Show activity
	a.activity.AddActivity(ActivityItem{
		Status: StatusRunning,
		Text:   "Sending to " + a.cfg.ActiveProvider + "...",
	})

	// Fire async LLM streaming request
	provider := a.provider
	history := make([]llm.Message, len(a.history))
	copy(history, a.history)
	systemPrompt := ""
	if len(history) > 0 && history[0].Role == "system" {
		systemPrompt = history[0].Content
	}
	if a.projectRules != "" {
		systemPrompt += "\n\n## Project Rules\n" + a.projectRules
	}
	if a.toolExecutor != nil {
		if ft := a.toolExecutor.FlowTracker(); ft != nil {
			flowCtx := ft.FormatFlowContext()
			if flowCtx != "" {
				systemPrompt += "\n\n## Recent Activity\n" + flowCtx
			}
		}
	}
	if systemPrompt != "" {
		if len(history) > 0 && history[0].Role == "system" {
			history[0].Content = systemPrompt
		} else {
			history = append([]llm.Message{{Role: "system", Content: systemPrompt}}, history...)
		}
	}
	a.chat.AddMessage(ChatMessage{Role: RoleAssistant, Content: ""})
	cmd := func() tea.Msg {
		ctx := context.Background() // no timeout
		ch, err := provider.Stream(ctx, history)
		if err != nil {
			return LLMResponseMsg{Error: err}
		}
		// Return the channel reference so Update can continue reading
		return streamStartMsg{channel: ch}
	}
	return a, cmd
}

// handleLLMResponse processes the response from a single-provider LLM call.
func (a App) handleLLMResponse(msg LLMResponseMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		fe := llm.ClassifyError(msg.Error, a.cfg.ActiveProvider)
		a.activity.UpdateLastActivity(StatusError)
		a.activity.AddActivity(ActivityItem{
			Status: StatusError,
			Text:   fe.Message,
		})
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Error: " + fe.Error(),
		})
	} else {
		a.activity.UpdateLastActivity(StatusDone)
		a.chat.AddMessage(ChatMessage{
			Role:    RoleAssistant,
			Content: msg.Content,
		})
		// Append to conversation history
		a.addToHistory(llm.Message{Role: "assistant", Content: msg.Content})
		a.saveMessageToDB("assistant", msg.Content, "")
	}

	return a, nil
}

// handleStreamStart initializes streaming from the LLM channel.
func (a App) handleStreamStart(msg streamStartMsg) (tea.Model, tea.Cmd) {
	a.streamCh = msg.channel
	return a, a.nextStreamChunk()
}

// handleStreamChunk processes a single streaming chunk from the LLM.
func (a App) handleStreamChunk(msg StreamChunkMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		fe := llm.ClassifyError(msg.Error, a.cfg.ActiveProvider)
		// Drain remaining chunks to unblock provider goroutine
		if a.streamCh != nil {
			ch := a.streamCh
			go func() {
				for range ch {
				}
			}()
		}
		a.streamCh = nil
		a.streamingContent = ""
		a.activity.UpdateLastActivity(StatusError)
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Stream error: " + fe.Error(),
		})
		return a, nil
	}

	if msg.Done {
		a.streamCh = nil
		a.chat.FinishStreaming()
		a.activity.UpdateLastActivity(StatusDone)
		a.addUsage(msg.Usage, a.cfg.GetActiveModel())
		if a.streamingContent != "" {
			a.addToHistory(llm.Message{Role: "assistant", Content: a.streamingContent})
			a.saveMessageToDB("assistant", a.streamingContent, "")
			a.streamingContent = ""
		}
		return a, nil
	}

	// Append chunk to chat display and accumulate for history
	a.streamingContent += msg.Content
	a.chat.AppendToLast(msg.Content)
	return a, a.nextStreamChunk()
}

// nextStreamChunk returns a command that reads the next chunk from the stream.
func (a App) nextStreamChunk() tea.Cmd {
	ch := a.streamCh
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return StreamChunkMsg{Done: true}
		}
		return StreamChunkMsg{Content: chunk.Content, Done: chunk.Done, Error: chunk.Error, Usage: chunk.Usage}
	}
}
