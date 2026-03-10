package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	ghub "github.com/artemis-project/artemis/internal/github"
	"github.com/artemis-project/artemis/internal/llm"
)

type fixIssueResultMsg struct {
	issueNumber int
	err         error
}

type issueListStore interface {
	ListAllIssues(ctx context.Context, limit int) ([]*ghub.StoredIssue, error)
}

// handleCommand processes slash commands (e.g., /sessions, /load).
func (a App) handleCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return a, nil
	}

	switch parts[0] {
	case "/sessions":
		return a.cmdListSessions()
	case "/issues":
		return a.cmdListIssues()
	case "/fix":
		if len(parts) < 2 {
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: "Usage: /fix <issue_number>",
			})
			return a, nil
		}
		return a.cmdFixIssue(parts[1])
	case "/load":
		if len(parts) < 2 {
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: "Usage: /load <session_id>",
			})
			return a, nil
		}
		return a.cmdLoadSession(parts[1])
	case "/help":
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Commands:\n  /sessions — List recent sessions\n  /load <id> — Resume a previous session\n  /issues — Sync and list GitHub issues\n  /fix <number> — Create draft fix PR scaffold for issue\n  /help — Show this help",
		})
		return a, nil
	default:
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Unknown command: %s. Type /help for available commands.", parts[0]),
		})
		return a, nil
	}
}

func (a App) cmdListIssues() (tea.Model, tea.Cmd) {
	if a.ghSyncer == nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "GitHub not configured."})
		return a, nil
	}

	store, ok := a.memStore.(issueListStore)
	if !ok || store == nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "Issue store is unavailable."})
		return a, nil
	}

	syncCtx, syncCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := a.ghSyncer.SyncOnce(syncCtx)
	syncCancel()
	if err != nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("GitHub sync failed: %v", err)})
		return a, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	issues, err := store.ListAllIssues(ctx, 50)
	if err != nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to list issues: %v", err)})
		return a, nil
	}

	if len(issues) == 0 {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "No synced issues found."})
		return a, nil
	}

	var sb strings.Builder
	sb.WriteString("GitHub Issues:\n")
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		author := issue.Author
		if strings.TrimSpace(author) == "" {
			author = "unknown"
		}
		title := strings.TrimSpace(issue.Title)
		if title == "" {
			title = "(no title)"
		}
		if len(title) > 80 {
			title = title[:77] + "..."
		}
		sb.WriteString(fmt.Sprintf("#%d  %s [%s]  %s  (%s)\n",
			issue.IssueNumber,
			triageEmoji(issue.TriageStatus),
			issue.TriageStatus,
			title,
			author,
		))
	}

	a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: strings.TrimSpace(sb.String())})
	return a, nil
}

func (a App) cmdFixIssue(numStr string) (tea.Model, tea.Cmd) {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(numStr), "#"))
	issueNumber, err := strconv.Atoi(clean)
	if err != nil || issueNumber <= 0 {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "Invalid issue number. Usage: /fix <issue_number>"})
		return a, nil
	}
	if a.ghProcessor == nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "GitHub processor is not configured."})
		return a, nil
	}

	a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Starting fix for issue #%d...", issueNumber)})

	processor := a.ghProcessor
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return fixIssueResultMsg{issueNumber: issueNumber, err: processor.FixIssue(ctx, issueNumber)}
	}
	return a, cmd
}

func triageEmoji(status ghub.TriageStatus) string {
	switch status {
	case ghub.TriageAutoFix:
		return "🟢"
	case ghub.TriageNeedsHuman:
		return "🟡"
	case ghub.TriageNotApplicable:
		return "🔴"
	case ghub.TriagePending:
		return "⏳"
	case ghub.TriageInProgress:
		return "🔧"
	case ghub.TriageResolved:
		return "✅"
	case ghub.TriageFailed:
		return "❌"
	default:
		return "•"
	}
}

// cmdListSessions shows recent sessions from the memory store.
func (a App) cmdListSessions() (tea.Model, tea.Cmd) {
	if a.memStore == nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Memory system is not enabled. Enable it in settings (Ctrl+S).",
		})
		return a, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessions, err := a.memStore.ListSessions(ctx, 20)
	if err != nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Error listing sessions: %v", err),
		})
		return a, nil
	}

	if len(sessions) == 0 {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "No previous sessions found.",
		})
		return a, nil
	}

	// Format session list
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent sessions (%d):\n", len(sessions)))
	for i, s := range sessions {
		dateStr := s.CreatedAt.Format("2006-01-02 15:04")
		preview := s.Summary
		if preview == "" {
			preview = "(no summary)"
		} else if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		// Mark current session
		marker := "  "
		if s.SessionID == a.sessionID {
			marker = "▸ "
		}
		sb.WriteString(fmt.Sprintf("%s%d. [%s] %s (%d msgs)\n   ID: %s\n",
			marker, i+1, dateStr, preview, s.MessageCount, s.SessionID))
	}
	sb.WriteString("\nUse /load <session_id> to resume a session.")

	a.chat.AddMessage(ChatMessage{
		Role:    RoleSystem,
		Content: sb.String(),
	})
	return a, nil
}

// cmdLoadSession loads a previous session's messages into the current chat.
func (a App) cmdLoadSession(sessionID string) (tea.Model, tea.Cmd) {
	if a.memStore == nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Memory system is not enabled.",
		})
		return a, nil
	}

	// Don't allow loading while we have an active conversation with history
	if len(a.history) > 0 {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Clear the current conversation first (Ctrl+L), then load a session.",
		})
		return a, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := a.memStore.GetSessionMessages(ctx, sessionID)
	if err != nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Error loading session: %v", err),
		})
		return a, nil
	}

	if len(messages) == 0 {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Session %q not found or has no messages.", sessionID),
		})
		return a, nil
	}

	// Switch to the loaded session
	a.sessionID = sessionID
	a.history = nil
	a.chat = NewChatPanel()
	a.recalcLayout()

	a.chat.AddMessage(ChatMessage{
		Role:    RoleSystem,
		Content: fmt.Sprintf("Loaded session %s (%d messages)", sessionID, len(messages)),
	})

	// Replay messages into chat panel and history
	for _, m := range messages {
		switch m.Role {
		case "user":
			a.chat.AddMessage(ChatMessage{
				Role:    RoleUser,
				Content: m.Content,
			})
			a.history = append(a.history, llm.Message{Role: "user", Content: m.Content})
		case "assistant":
			if m.AgentRole != "" && m.AgentRole != "pipeline" {
				a.chat.AddMessage(ChatMessage{
					Role:      RoleAgent,
					Content:   m.Content,
					AgentName: agentDisplayName(m.AgentRole),
					AgentRole: m.AgentRole,
				})
			} else {
				a.chat.AddMessage(ChatMessage{
					Role:    RoleAssistant,
					Content: m.Content,
				})
			}
			a.history = append(a.history, llm.Message{Role: "assistant", Content: m.Content})
		case "system":
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: m.Content,
			})
		}
	}

	a.chat.AddMessage(ChatMessage{
		Role:    RoleSystem,
		Content: "Session resumed. Continue the conversation.",
	})

	return a, nil
}
