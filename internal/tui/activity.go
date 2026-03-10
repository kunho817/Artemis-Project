package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ActivityStatus represents the state of an activity item.
type ActivityStatus int

const (
	StatusRunning ActivityStatus = iota
	StatusDone
	StatusError
)

// ActivityItem represents a single activity entry.
type ActivityItem struct {
	Status    ActivityStatus
	Text      string
	StartedAt time.Time
}

// FileChange represents a changed file.
type FileChange struct {
	Path  string
	IsNew bool
}

// ActivityPanel shows running operations and file changes.
type ActivityPanel struct {
	activityViewport viewport.Model
	activities       []ActivityItem
	files            []FileChange
	sessionInfo      string
	modelInfo        string
	agentCount       int
	width            int
	height           int
	focused          bool
}

// NewActivityPanel creates a new activity panel.
func NewActivityPanel() ActivityPanel {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle()

	return ActivityPanel{
		activityViewport: vp,
		activities:       []ActivityItem{},
		files:            []FileChange{},
	}
}

// SetSize updates the panel dimensions.
func (a *ActivityPanel) SetSize(w, h int) {
	a.width = w
	a.height = h

	// Reserve lines for context + section headers/dividers.
	contextHeight := 6 // Context title, divider, session, model, agents, spacer
	activityHeaderHeight := 2
	filesSectionHeight := 0
	if len(a.files) > 0 {
		filesItems := min(len(a.files), max(h/3, 1))
		filesSectionHeight = 2 + filesItems // title+divider + items
	}
	activityHeight := h - contextHeight - activityHeaderHeight - filesSectionHeight
	if activityHeight < 1 {
		activityHeight = 1
	}

	a.activityViewport.Width = w
	a.activityViewport.Height = activityHeight
	a.refreshContent()
}

// SetFocused sets the focus state.
func (a *ActivityPanel) SetFocused(focused bool) {
	a.focused = focused
}

// AddActivity appends an activity item.
func (a *ActivityPanel) AddActivity(item ActivityItem) {
	if item.Status == StatusRunning && item.StartedAt.IsZero() {
		item.StartedAt = time.Now()
	}
	a.activities = append(a.activities, item)
	a.refreshContent()
	a.activityViewport.GotoBottom()
}

// UpdateLastActivity updates the last activity item's status.
func (a *ActivityPanel) UpdateLastActivity(status ActivityStatus) {
	if len(a.activities) == 0 {
		return
	}
	a.activities[len(a.activities)-1].Status = status
	a.refreshContent()
}

// AddFileChange adds a file change entry.
func (a *ActivityPanel) AddFileChange(fc FileChange) {
	a.files = append(a.files, fc)
	a.refreshContent()
}

// ClearActivities resets all activities.
func (a *ActivityPanel) ClearActivities() {
	a.activities = nil
	a.files = nil
	a.refreshContent()
}

func (a *ActivityPanel) refreshContent() {
	var sb strings.Builder

	for _, item := range a.activities {
		var icon string
		var style lipgloss.Style

		switch item.Status {
		case StatusRunning:
			icon = "●"
			style = ActivityRunningStyle
		case StatusDone:
			icon = "✓"
			style = ActivityDoneStyle
		case StatusError:
			icon = "✗"
			style = ActivityErrorStyle
		}

		elapsed := a.formatElapsed(item)
		if elapsed != "" {
			sb.WriteString(fmt.Sprintf(" %s %s %s\n", style.Render(icon), item.Text, elapsed))
		} else {
			sb.WriteString(fmt.Sprintf(" %s %s\n", style.Render(icon), item.Text))
		}
	}

	a.activityViewport.SetContent(sb.String())
}

// SetSessionInfo updates panel context metadata.
func (a *ActivityPanel) SetSessionInfo(sessionID, model string) {
	if len(sessionID) > 12 {
		sessionID = sessionID[len(sessionID)-8:]
	}
	a.sessionInfo = sessionID
	a.modelInfo = model
	a.refreshContent()
}

// SetAgentCount updates the current pipeline agent count.
func (a *ActivityPanel) SetAgentCount(n int) {
	a.agentCount = n
	a.refreshContent()
}

// View renders the activity panel.
func (a *ActivityPanel) View() string {
	var sb strings.Builder

	// Context section
	contextTitle := SubTitleStyle.Render("Context")
	sb.WriteString(contextTitle + "\n")
	sb.WriteString(DividerStyle.Render(strings.Repeat("─", max(a.width-2, 1))) + "\n")

	sessionText := "-"
	if a.sessionInfo != "" {
		sessionText = a.sessionInfo
	}
	modelText := "-"
	if a.modelInfo != "" {
		modelText = a.modelInfo
	}

	sb.WriteString(fmt.Sprintf(" Session: %s\n", lipgloss.NewStyle().Foreground(ColorDimText).Render(sessionText)))
	sb.WriteString(fmt.Sprintf(" Model: %s\n", lipgloss.NewStyle().Foreground(ColorDimText).Render(modelText)))
	if a.agentCount > 0 {
		sb.WriteString(fmt.Sprintf(" Agents: %s\n", lipgloss.NewStyle().Foreground(ColorDimText).Render(fmt.Sprintf("%d", a.agentCount))))
	}
	sb.WriteString("\n")

	// Activity section
	actTitle := SubTitleStyle.Render("Activity")
	sb.WriteString(actTitle + "\n")
	sb.WriteString(DividerStyle.Render(strings.Repeat("─", max(a.width-2, 1))) + "\n")
	sb.WriteString(a.activityViewport.View())

	// Files section
	if len(a.files) > 0 {
		divider := DividerStyle.Render(strings.Repeat("─", max(a.width-2, 1)))
		sb.WriteString("\n" + divider + "\n")
		filesTitle := SubTitleStyle.Render("Files Changed")
		sb.WriteString(filesTitle + "\n")
		sb.WriteString(DividerStyle.Render(strings.Repeat("─", max(a.width-2, 1))) + "\n")

		for _, f := range a.files {
			label := "~"
			if f.IsNew {
				label = "+"
			}
			sb.WriteString(fmt.Sprintf(" %s %s\n",
				FileChangedStyle.Render(label),
				f.Path,
			))
		}
	}

	return sb.String()
}

func (a *ActivityPanel) formatElapsed(item ActivityItem) string {
	if item.StartedAt.IsZero() {
		return ""
	}
	d := time.Since(item.StartedAt)
	if d < 0 {
		d = 0
	}
	seconds := float64(d.Milliseconds()) / 1000.0
	return lipgloss.NewStyle().Foreground(ColorDimText).Render(fmt.Sprintf("(%.1fs)", seconds))
}

// ScrollUp scrolls the activity viewport up.
func (a *ActivityPanel) ScrollUp(n int) {
	a.activityViewport.LineUp(n)
}

// ScrollDown scrolls the activity viewport down.
func (a *ActivityPanel) ScrollDown(n int) {
	a.activityViewport.LineDown(n)
}

// GetChangedFiles returns the paths of all files changed in this session.
func (a *ActivityPanel) GetChangedFiles() []string {
	paths := make([]string, 0, len(a.files))
	for _, f := range a.files {
		paths = append(paths, f.Path)
	}
	return paths
}
