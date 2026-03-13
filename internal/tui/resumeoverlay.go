package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

// ResumeDecisionMsg is sent when the user decides on an incomplete pipeline run.
type ResumeDecisionMsg struct {
	Action string              // "resume", "discard", "cancel"
	Run    state.IncompleteRun // the run to resume (if Action == "resume")
}

// ResumeOverlay displays information about an interrupted pipeline run
// and offers the user a choice to resume, discard, or cancel.
type ResumeOverlay struct {
	run      state.IncompleteRun
	selected int // 0=Resume, 1=Discard, 2=Cancel
	width    int
	height   int
}

// NewResumeOverlay creates a resume overlay for a given incomplete run.
func NewResumeOverlay(run state.IncompleteRun, w, h int) *ResumeOverlay {
	return &ResumeOverlay{
		run:      run,
		selected: 0,
		width:    w,
		height:   h,
	}
}

// Update handles user input for the resume overlay.
func (o *ResumeOverlay) Update(msg tea.Msg) (closed bool, result OverlayResult, cmd tea.Cmd) {
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
			return true, OverlayResult{Action: "resume", Value: "resume"}, func() tea.Msg {
				return ResumeDecisionMsg{Action: "resume", Run: o.run}
			}
		case "d", "D":
			o.selected = 1
			return true, OverlayResult{Action: "resume", Value: "discard"}, func() tea.Msg {
				return ResumeDecisionMsg{Action: "discard", Run: o.run}
			}

		case "enter":
			action := "cancel"
			switch o.selected {
			case 0:
				action = "resume"
			case 1:
				action = "discard"
			case 2:
				action = "cancel"
			}
			return true, OverlayResult{Action: "resume", Value: action}, func() tea.Msg {
				return ResumeDecisionMsg{Action: action, Run: o.run}
			}

		case "esc":
			return true, OverlayResult{Action: "resume", Value: "cancel"}, func() tea.Msg {
				return ResumeDecisionMsg{Action: "cancel", Run: o.run}
			}
		}
	}
	return false, OverlayResult{}, nil
}

// View renders the resume overlay.
func (o *ResumeOverlay) View() string {
	t := &theme.Active.Colors

	boxWidth := minInt(65, o.width-8)
	if boxWidth < 35 {
		boxWidth = 35
	}

	contentWidth := boxWidth - 6 // padding + border

	// Title
	title := overlayTitleStyle().Render("⚡ Incomplete Pipeline Detected")

	// Run info
	var info strings.Builder
	info.WriteString(fmt.Sprintf("Run ID:    %s\n", truncateStr(o.run.RunID, contentWidth-11)))
	info.WriteString(fmt.Sprintf("Intent:    %s\n", o.run.Intent))
	info.WriteString(fmt.Sprintf("Started:   %s\n", o.run.CreatedAt.Format("2006-01-02 15:04:05")))
	if o.run.LastStepIndex >= 0 {
		info.WriteString(fmt.Sprintf("Progress:  step %d/%d completed (%s)\n",
			o.run.LastStepIndex+1, o.run.TotalSteps, o.run.LastStepName))
	} else {
		info.WriteString(fmt.Sprintf("Progress:  0/%d steps (no checkpoints)\n", o.run.TotalSteps))
	}

	infoStyle := lipgloss.NewStyle().
		Width(contentWidth).
		Foreground(t.DimTextColor()).
		Background(t.PanelColor())

	// Options
	options := []struct {
		key   string
		label string
		desc  string
	}{
		{"R", "Resume", "Continue from last checkpoint"},
		{"D", "Discard", "Delete checkpoints and start fresh"},
		{"Esc", "Cancel", "Ignore for now"},
	}

	var optLines []string
	for i, opt := range options {
		cursor := "  "
		style := lipgloss.NewStyle().Background(t.PanelColor())
		if i == o.selected {
			cursor = "▸ "
			style = style.Foreground(t.PrimaryColor()).Bold(true)
		} else {
			style = style.Foreground(t.TextColor())
		}
		keyStyle := lipgloss.NewStyle().
			Foreground(t.AccentColor()).
			Bold(true).
			Background(t.PanelColor())
		line := fmt.Sprintf("%s[%s] %s — %s",
			cursor,
			keyStyle.Render(opt.key),
			style.Render(opt.label),
			lipgloss.NewStyle().Foreground(t.DimTextColor()).Background(t.PanelColor()).Render(opt.desc),
		)
		optLines = append(optLines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		infoStyle.Render(info.String()),
		"",
		strings.Join(optLines, "\n"),
	)

	boxHeight := strings.Count(content, "\n") + 4 // padding
	box := overlayBoxStyle(boxWidth, boxHeight)

	return box.Render(content)
}

// SetSize updates the overlay dimensions.
func (o *ResumeOverlay) SetSize(w, h int) {
	o.width = w
	o.height = h
}

// truncateStr truncates a string to maxLen with ellipsis.
func truncateStr(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
