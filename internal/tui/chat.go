package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// MessageRole represents who sent the message.
type MessageRole int

const (
	RoleUser MessageRole = iota
	RoleAssistant
	RoleSystem
	RoleAgent // agent output with structured formatting
)

// ChatMessage represents a single message in the chat.
type ChatMessage struct {
	Role      MessageRole
	Content   string
	AgentName string // populated for RoleAgent messages
	AgentRole string // agent's role category (analysis, planning, code, verify)
}

// ChatPanel displays the conversation history.
type ChatPanel struct {
	viewport      viewport.Model
	messages      []ChatMessage
	renderer      *glamour.TermRenderer
	streamingMsgs map[int]bool
	renderedCache []string
	width         int
	height        int
	focused       bool
}

var toolUseBlockRe = regexp.MustCompile(`(?s)<tool_use>.*?</tool_use>`)

// NewChatPanel creates a new chat panel.
func NewChatPanel() ChatPanel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)

	return ChatPanel{
		viewport:      vp,
		messages:      []ChatMessage{},
		renderer:      r,
		renderedCache: []string{},
		streamingMsgs: make(map[int]bool),
	}
}

// SetSize updates the panel dimensions.
func (c *ChatPanel) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.Width = w
	c.viewport.Height = h

	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(max(w-4, 0)),
	)
	if r != nil {
		c.renderer = r
	}
	c.invalidateCache()
	c.refreshContent()
}

// SetFocused sets the focus state.
func (c *ChatPanel) SetFocused(focused bool) {
	c.focused = focused
}

// Update forwards messages to the viewport for scrolling (mouse wheel, arrow keys, etc.).
func (c *ChatPanel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	return cmd
}

// AddMessage appends a message and scrolls to bottom.
func (c *ChatPanel) AddMessage(msg ChatMessage) {
	c.messages = append(c.messages, msg)
	c.renderedCache = append(c.renderedCache, "")
	c.refreshContent()
	c.viewport.GotoBottom()
}

// AppendToLast appends content to the last message (for streaming — single-mode compatibility).
func (c *ChatPanel) AppendToLast(content string) {
	if len(c.messages) == 0 {
		return
	}
	c.AppendToMessage(len(c.messages)-1, content)
}

// FinishStreaming marks all streaming messages complete and applies markdown rendering.
func (c *ChatPanel) FinishStreaming() {
	for idx := range c.streamingMsgs {
		c.finishStreamAt(idx)
	}
	c.refreshContent()
}

// MessageCount returns the number of messages in the chat.
func (c *ChatPanel) MessageCount() int {
	return len(c.messages)
}

// AppendToMessage appends content to a specific message by index (for parallel agent streaming).
func (c *ChatPanel) AppendToMessage(idx int, content string) {
	if idx < 0 || idx >= len(c.messages) {
		return
	}
	if c.streamingMsgs == nil {
		c.streamingMsgs = make(map[int]bool)
	}
	c.streamingMsgs[idx] = true
	c.ensureCacheLength()
	c.renderedCache[idx] = ""
	c.messages[idx].Content += content
	c.refreshContent()
	c.viewport.GotoBottom()
}

// FinishStreamingAt marks a specific message's stream as complete and applies markdown rendering.
func (c *ChatPanel) FinishStreamingAt(idx int) {
	c.finishStreamAt(idx)
	c.refreshContent()
}

func (c *ChatPanel) finishStreamAt(idx int) {
	if idx < 0 || idx >= len(c.messages) {
		return
	}
	msg := c.messages[idx]
	if msg.Role == RoleAssistant || msg.Role == RoleAgent {
		c.ensureCacheLength()
		wrapWidth := c.width - 4
		if msg.Role == RoleAgent {
			wrapWidth = c.width - 6
		}
		c.renderedCache[idx] = c.renderMarkdown(msg.Content, wrapWidth)
	}
	delete(c.streamingMsgs, idx)
}

func (c *ChatPanel) refreshContent() {
	if c.width <= 0 {
		return
	}

	var sb strings.Builder
	contentWidth := c.width - 2 // padding

	for i, msg := range c.messages {
		if i > 0 {
			sb.WriteString("\n")
		}

		isStreaming := c.streamingMsgs[i]

		switch msg.Role {
		case RoleAgent:
			c.renderAgentMessage(&sb, msg, contentWidth, i, isStreaming)
		default:
			c.renderStandardMessage(&sb, msg, contentWidth, i, isStreaming)
		}
	}

	c.viewport.SetContent(sb.String())
}

// renderStandardMessage renders user, assistant, and system messages.
func (c *ChatPanel) renderStandardMessage(sb *strings.Builder, msg ChatMessage, contentWidth int, idx int, isStreamingLast bool) {
	var label string
	switch msg.Role {
	case RoleUser:
		label = UserLabelStyle.Render("You")
	case RoleAssistant:
		label = BotLabelStyle.Render("Artemis")
	case RoleSystem:
		label = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("system")
	}

	sb.WriteString(fmt.Sprintf("  %s:", label))

	if msg.Role == RoleAssistant {
		rendered := c.renderAssistantContent(msg.Content, contentWidth, idx, isStreamingLast)
		if strings.TrimSpace(rendered) != "" {
			sb.WriteString("\n")
			sb.WriteString(indentBlock(rendered, "    "))
		}
		sb.WriteString("\n")
		return
	}

	sb.WriteString(" ")

	lines := wrapText(msg.Content, contentWidth-4)
	for j, line := range lines {
		if j == 0 {
			sb.WriteString(MessageStyle.Render(line))
		} else {
			sb.WriteString("\n    " + MessageStyle.Render(line))
		}
	}
	sb.WriteString("\n")
}

// renderAgentMessage renders structured agent output with role-based styling.
func (c *ChatPanel) renderAgentMessage(sb *strings.Builder, msg ChatMessage, contentWidth int, idx int, isStreamingLast bool) {
	// Divider line
	divider := AgentDividerStyle.Render(strings.Repeat("─", contentWidth-2))
	sb.WriteString("  " + divider + "\n")

	// Agent header with role-specific color
	style := agentStyleForRole(msg.AgentRole)
	header := style.Render(fmt.Sprintf("◆ %s", msg.AgentName))
	roleTag := lipgloss.NewStyle().Foreground(ColorDimText).Render(fmt.Sprintf(" (%s)", msg.AgentRole))
	sb.WriteString(fmt.Sprintf("  %s%s\n", header, roleTag))

	rendered := c.renderAgentContent(msg.Content, contentWidth, idx, isStreamingLast)
	for _, line := range strings.Split(rendered, "\n") {
		sb.WriteString("    " + line + "\n")
	}
}

func (c *ChatPanel) renderAssistantContent(content string, width int, idx int, isStreamingLast bool) string {
	if isStreamingLast {
		return strings.Join(wrapText(content, width-4), "\n")
	}
	if idx < len(c.renderedCache) && c.renderedCache[idx] != "" {
		return c.renderedCache[idx]
	}
	rendered := c.renderMarkdown(content, width-4)
	if idx < len(c.renderedCache) {
		c.renderedCache[idx] = rendered
	}
	return rendered
}

func (c *ChatPanel) renderAgentContent(content string, width int, idx int, isStreamingLast bool) string {
	if isStreamingLast {
		return strings.Join(wrapText(content, width-6), "\n")
	}
	if idx < len(c.renderedCache) && c.renderedCache[idx] != "" {
		return c.renderedCache[idx]
	}
	rendered := c.renderMarkdown(content, width-6)
	if idx < len(c.renderedCache) {
		c.renderedCache[idx] = rendered
	}
	return rendered
}

func (c *ChatPanel) renderMarkdown(content string, wrapWidth int) string {
	plainFallback := strings.Join(wrapText(content, max(wrapWidth, 1)), "\n")
	if c.renderer == nil {
		return plainFallback
	}
	rendered, err := c.renderer.Render(preprocessMarkdown(content))
	if err != nil {
		return plainFallback
	}
	return strings.TrimRight(rendered, "\n")
}

func preprocessMarkdown(content string) string {
	return toolUseBlockRe.ReplaceAllStringFunc(content, func(block string) string {
		return "```xml\n" + strings.TrimSpace(block) + "\n```"
	})
}

func indentBlock(content, prefix string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}


func (c *ChatPanel) ensureCacheLength() {
	for len(c.renderedCache) < len(c.messages) {
		c.renderedCache = append(c.renderedCache, "")
	}
}

func (c *ChatPanel) invalidateCache() {
	c.renderedCache = make([]string, len(c.messages))
}

// agentStyleForRole returns a color style based on the agent's role category.
func agentStyleForRole(role string) lipgloss.Style {
	switch role {
	case "analyzer", "searcher", "explorer":
		return AgentAnalysisStyle
	case "planner":
		return AgentPlanStyle
	case "architect", "coder", "designer", "engineer":
		return AgentCodeStyle
	case "qa", "tester":
		return AgentVerifyStyle
	default:
		return AgentHeaderStyle
	}
}

// View renders the chat panel.
func (c *ChatPanel) View() string {
	return c.viewport.View()
}

// ScrollUp scrolls the viewport up.
func (c *ChatPanel) ScrollUp(n int) {
	c.viewport.LineUp(n)
}

// ScrollDown scrolls the viewport down.
func (c *ChatPanel) ScrollDown(n int) {
	c.viewport.LineDown(n)
}

// wrapText wraps text to fit within a given width.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) > maxWidth {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				currentLine += " " + word
			}
		}
		lines = append(lines, currentLine)
	}

	return lines
}
