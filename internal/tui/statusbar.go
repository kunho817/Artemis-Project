package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// StatusBar displays model info, tokens, and keybindings.
type StatusBar struct {
	model            string
	tokens           int
	cost             float64
	width            int
	mode             string // "single" or "multi"
	tier             string // "premium" or "budget"
	keyHints         []KeyHint
	startedAt        time.Time
	pipelineProgress string // "Step 2/5" or ""
}

// KeyHint represents a keyboard shortcut hint.
type KeyHint struct {
	Key  string
	Desc string
}

// DefaultKeyHints returns the default keybindings.
func DefaultKeyHints() []KeyHint {
	return []KeyHint{
		{Key: "^↵", Desc: "Send"},
		{Key: "^S", Desc: "Settings"},
		{Key: "^L", Desc: "Clear"},
		{Key: "/help", Desc: "Cmds"},
		{Key: "^C", Desc: "Quit"},
	}
}

// NewStatusBar creates a new status bar.
func NewStatusBar() StatusBar {
	return StatusBar{
		model:     "Claude",
		tokens:    0,
		cost:      0,
		mode:      "single",
		tier:      "premium",
		keyHints:  DefaultKeyHints(),
		startedAt: time.Now(),
	}
}

// SetSize updates the width.
func (s *StatusBar) SetSize(w int) {
	s.width = w
}

// SetModel sets the active model name.
func (s *StatusBar) SetModel(model string) {
	s.model = model
}

// SetTokens sets the token count.
func (s *StatusBar) SetTokens(tokens int) {
	s.tokens = tokens
}

// SetCost sets the accumulated cost.
func (s *StatusBar) SetCost(cost float64) {
	s.cost = cost
}

// SetMode sets the agent mode ("single" or "multi").
func (s *StatusBar) SetMode(mode string) {
	s.mode = mode
}

// SetTier sets the active tier ("premium" or "budget").
func (s *StatusBar) SetTier(tier string) {
	s.tier = tier
}

// SetKeyHints overrides the default key hints.
func (s *StatusBar) SetKeyHints(hints []KeyHint) {
	s.keyHints = hints
}

func (s *StatusBar) SetPipelineProgress(current, total int) {
	if current > 0 && total > 0 {
		s.pipelineProgress = fmt.Sprintf("Step %d/%d", current, total)
	} else {
		s.pipelineProgress = ""
	}
}

// View renders the status bar.
func (s *StatusBar) View() string {
	// Left section: model + mode/tier + stats
	modelLabel := StatusModelStyle.Render(s.model)

	// Mode/tier badge
	var tierBadge string
	if s.mode == "multi" {
		if s.tier == "premium" {
			tierBadge = StatusTierStyle.Copy().Foreground(ColorAccent).Render("Premium")
		} else {
			tierBadge = StatusTierStyle.Copy().Foreground(ColorWarning).Render("Budget")
		}
	} else {
		tierBadge = StatusTierStyle.Copy().Foreground(ColorDimText).Render("Single")
	}

	// Token count with optional cost
	tokenLabel := StatusTokenStyle.Render(
		fmt.Sprintf("Tk:%s", formatTokens(s.tokens)),
	)
	var costLabel string
	if s.cost > 0 {
		costLabel = StatusTokenStyle.Copy().Foreground(ColorMuted).Render(
			fmt.Sprintf("$%.2f", s.cost),
		)
	}

	// Session duration
	elapsed := time.Since(s.startedAt)
	durationStr := formatDuration(elapsed)
	durationLabel := StatusTokenStyle.Copy().Foreground(ColorDimText).Render(durationStr)

	// Build left section with cleaner separators
	left := lipgloss.JoinHorizontal(lipgloss.Center,
		modelLabel,
		StatusBarStyle.Render(" · "),
		tierBadge,
		StatusBarStyle.Render(" · "),
		tokenLabel,
	)
	if costLabel != "" {
		left = lipgloss.JoinHorizontal(lipgloss.Center,
			left,
			StatusBarStyle.Render(" · "),
			costLabel,
		)
	}
	if s.pipelineProgress != "" {
		left = lipgloss.JoinHorizontal(lipgloss.Center,
			left,
			StatusBarStyle.Render(" · "),
			StatusTokenStyle.Copy().Foreground(ColorAccent).Render(s.pipelineProgress),
		)
	}
	left = lipgloss.JoinHorizontal(lipgloss.Center,
		left,
		StatusBarStyle.Render(" · "),
		durationLabel,
	)

	// Right section: key hints
	var hints []string
	for _, kh := range s.keyHints {
		hint := fmt.Sprintf("%s %s",
			StatusKeyStyle.Render("["+kh.Key+"]"),
			StatusValueStyle.Render(kh.Desc),
		)
		hints = append(hints, hint)
	}
	right := lipgloss.JoinHorizontal(lipgloss.Center, joinWithSpace(hints)...)

	// Fill space between left and right
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := s.width - leftWidth - rightWidth
	if gap < 1 {
		gap = 1
	}

	bar := left + lipgloss.NewStyle().Width(gap).Render("") + right

	return StatusBarStyle.
		Width(s.width).
		Render(bar)
}

// formatTokens formats token count with k/M suffix.
func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// formatDuration formats elapsed time as "12m" or "1h23m".
func formatDuration(d time.Duration) string {
	totalSeconds := int(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// joinWithSpace joins strings with a space separator for lipgloss.
func joinWithSpace(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items)*2-1)
	for i, item := range items {
		if i > 0 {
			result = append(result, " ")
		}
		result = append(result, item)
	}
	return result
}
