package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

// RecoveryDecisionMsg is sent when the user makes a recovery decision.
type RecoveryDecisionMsg struct {
	Action orchestrator.RecoveryAction
}

// RecoveryOverlay displays failure information and recovery options to the user.
// Implements the Overlay interface.
type RecoveryOverlay struct {
	request  RecoveryRequest
	selected int // 0=Retry, 1=Skip, 2=Abort
	width    int
	height   int
}

// NewRecoveryOverlay creates a recovery overlay for a given request.
func NewRecoveryOverlay(req RecoveryRequest, w, h int) *RecoveryOverlay {
	return &RecoveryOverlay{
		request:  req,
		selected: 0,
		width:    w,
		height:   h,
	}
}

// Update handles user input for the recovery overlay.
func (o *RecoveryOverlay) Update(msg tea.Msg) (closed bool, result OverlayResult, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if o.selected > 0 {
				o.selected--
			}
		case "down", "j":
			if o.selected < 2 {
				o.selected++
			}

		case "r", "R":
			o.selected = 0
			return true, OverlayResult{Action: "recovery", Value: "retry"}, func() tea.Msg {
				return RecoveryDecisionMsg{Action: orchestrator.ActionRetry}
			}
		case "s", "S":
			o.selected = 1
			return true, OverlayResult{Action: "recovery", Value: "skip"}, func() tea.Msg {
				return RecoveryDecisionMsg{Action: orchestrator.ActionSkip}
			}
		case "a", "A":
			o.selected = 2
			return true, OverlayResult{Action: "recovery", Value: "abort"}, func() tea.Msg {
				return RecoveryDecisionMsg{Action: orchestrator.ActionAbort}
			}

		case "enter":
			var action orchestrator.RecoveryAction
			switch o.selected {
			case 0:
				action = orchestrator.ActionRetry
			case 1:
				action = orchestrator.ActionSkip
			case 2:
				action = orchestrator.ActionAbort
			}
			return true, OverlayResult{Action: "recovery", Value: string(action)}, func() tea.Msg {
				return RecoveryDecisionMsg{Action: action}
			}

		case "esc":
			// Escape = Abort
			return true, OverlayResult{Action: "recovery", Value: "abort"}, func() tea.Msg {
				return RecoveryDecisionMsg{Action: orchestrator.ActionAbort}
			}
		}
	}
	return false, OverlayResult{}, nil
}

// View renders the recovery overlay.
func (o *RecoveryOverlay) View() string {
	rc := o.request.Context
	t := &theme.Active.Colors

	boxWidth := minInt(60, o.width-8)
	if boxWidth < 30 {
		boxWidth = 30
	}
	contentWidth := boxWidth - 6 // padding + border

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.ErrorColor()).
		Background(t.PanelColor())
	b.WriteString(titleStyle.Render("⚠ Agent Recovery"))
	b.WriteString("\n\n")

	// Error section
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextColor()).
		Background(t.PanelColor())
	dimStyle := lipgloss.NewStyle().
		Foreground(t.DimTextColor()).
		Background(t.PanelColor())

	b.WriteString(labelStyle.Render(fmt.Sprintf("Agent: %s (%s)", rc.FailedAgent, rc.FailedRole)))
	b.WriteString("\n")

	errMsg := rc.Error.Error()
	if len(errMsg) > contentWidth*2 {
		errMsg = errMsg[:contentWidth*2-3] + "..."
	}
	b.WriteString(dimStyle.Render("Error: " + errMsg))
	b.WriteString("\n")

	if rc.AttemptCount > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("Recovery attempts: %d/%d", rc.AttemptCount, orchestrator.MaxRecoveryAttempts)))
		b.WriteString("\n")
	}

	// Consultant diagnosis (if available)
	if rc.Diagnosis != "" {
		b.WriteString("\n")
		diagStyle := lipgloss.NewStyle().
			Foreground(t.AccentColor()).
			Background(t.PanelColor())
		b.WriteString(labelStyle.Render("Diagnosis:"))
		b.WriteString("\n")
		diag := rc.Diagnosis
		if len(diag) > contentWidth*3 {
			diag = diag[:contentWidth*3-3] + "..."
		}
		b.WriteString(diagStyle.Render(diag))
		b.WriteString("\n")
	}

	if rc.Suggestion != "" {
		b.WriteString("\n")
		sugStyle := lipgloss.NewStyle().
			Foreground(t.SuccessColor()).
			Background(t.PanelColor())
		b.WriteString(labelStyle.Render("Suggestion:"))
		b.WriteString("\n")
		sug := rc.Suggestion
		if len(sug) > contentWidth*3 {
			sug = sug[:contentWidth*3-3] + "..."
		}
		b.WriteString(sugStyle.Render(sug))
		b.WriteString("\n")
	}

	// Options
	b.WriteString("\n")
	options := []struct {
		key   string
		label string
		desc  string
	}{
		{"R", "Retry", "Re-run with Consultant diagnosis"},
		{"S", "Skip", "Skip this agent, continue pipeline"},
		{"A", "Abort", "Stop the pipeline"},
	}

	for i, opt := range options {
		prefix := "  "
		style := dimStyle
		if i == o.selected {
			prefix = "▸ "
			style = lipgloss.NewStyle().
				Foreground(t.BackgroundColor()).
				Background(t.PrimaryColor()).
				Bold(true)
		}
		line := fmt.Sprintf("%s[%s] %s — %s", prefix, opt.key, opt.label, opt.desc)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	// Hint
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓ navigate · Enter select · Esc abort"))

	// Wrap in box
	boxStyle := overlayBoxStyle(boxWidth, 0) // auto height
	return boxStyle.Render(b.String())
}

// SetSize updates the available terminal dimensions.
func (o *RecoveryOverlay) SetSize(w, h int) {
	o.width = w
	o.height = h
}
