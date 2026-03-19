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
	Tokens    int    // token count for this agent (0 = not tracked)
	StepInfo  string // "Step 2/5" progress context
}

// FileChange represents a changed file.
type FileChange struct {
	Path  string
	IsNew bool
}

// TestResult represents the outcome of a test run.
type TestResult struct {
	Passed   int
	Failed   int
	Skipped  int
	Elapsed  time.Duration
	Failures []TestFailure
}

// TestFailure represents a single failed test.
type TestFailure struct {
	Name    string
	Package string
	Output  string
}

// ActivityPanel shows running operations and file changes.
type ActivityPanel struct {
	activityViewport viewport.Model
	activities       []ActivityItem
	files            []FileChange
	testResults      *TestResult
	sessionInfo      string
	modelInfo        string
	agentCount       int
	width            int
	height           int
	focused          bool

	// Pipeline progress tracking
	pipelineSteps       int // total steps
	pipelineCurrentStep int // current step (1-based)
	pipelineRunning     bool
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

	testResultsHeight := 0
	if a.testResults != nil {
		testResultsHeight = 3 // title + divider + summary
		if a.testResults.Failed > 0 {
			maxShow := 3
			if len(a.testResults.Failures) < maxShow {
				maxShow = len(a.testResults.Failures)
			}
			testResultsHeight += maxShow * 3 // name + 2 lines of output
			if len(a.testResults.Failures) > 3 {
				testResultsHeight += 1 // "... and X more failures"
			}
		}
	}

	activityHeight := h - contextHeight - activityHeaderHeight - filesSectionHeight - testResultsHeight
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
	a.testResults = nil
	a.refreshContent()
}

// SetTestResults updates the test results section.
func (a *ActivityPanel) SetTestResults(r *TestResult) {
	a.testResults = r
	a.refreshContent()
}

// ClearTestResults removes the test results section.
func (a *ActivityPanel) ClearTestResults() {
	a.testResults = nil
	a.refreshContent()
}

func (a *ActivityPanel) SetPipelineProgress(current, total int) {
	a.pipelineCurrentStep = current
	a.pipelineSteps = total
	a.pipelineRunning = current < total
}

func (a *ActivityPanel) ClearPipelineProgress() {
	a.pipelineRunning = false
	a.pipelineCurrentStep = 0
	a.pipelineSteps = 0
}

func (a *ActivityPanel) SetAgentTokens(agentName string, tokens int) {
	lowerName := strings.ToLower(agentName)
	for i := len(a.activities) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(a.activities[i].Text), lowerName) {
			a.activities[i].Tokens = tokens
			break
		}
	}
	a.refreshContent()
}

func (a *ActivityPanel) GetAgentTokens(agentName string) int {
	lowerName := strings.ToLower(agentName)
	for i := len(a.activities) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(a.activities[i].Text), lowerName) && a.activities[i].Tokens > 0 {
			return a.activities[i].Tokens
		}
	}
	return 0
}

func (a *ActivityPanel) refreshContent() {
	var sb strings.Builder

	if a.pipelineRunning && a.pipelineSteps > 0 {
		bar := renderProgressBar(a.pipelineCurrentStep, a.pipelineSteps, a.width-4)
		sb.WriteString(fmt.Sprintf("  %s\n", bar))
	}

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
		text := item.Text

		if elapsed != "" {
			sb.WriteString(fmt.Sprintf(" %s %s %s\n", style.Render(icon), text, elapsed))
		} else {
			sb.WriteString(fmt.Sprintf(" %s %s\n", style.Render(icon), text))
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

	// --- Test Results Section ---
	if a.testResults != nil {
		sb.WriteString("\n")
		r := a.testResults
		header := "Test Results"
		headerStyle := SubTitleStyle
		sb.WriteString(fmt.Sprintf("  %s\n", headerStyle.Render(header)))

		// Summary line: "✓ 15 passed  ✗ 2 failed  ○ 1 skipped (3.4s)"
		var parts []string
		if r.Passed > 0 {
			parts = append(parts, fmt.Sprintf("✓ %d passed", r.Passed))
		}
		if r.Failed > 0 {
			parts = append(parts, fmt.Sprintf("✗ %d failed", r.Failed))
		}
		if r.Skipped > 0 {
			parts = append(parts, fmt.Sprintf("○ %d skipped", r.Skipped))
		}
		summary := strings.Join(parts, "  ")
		if r.Elapsed > 0 {
			summary += fmt.Sprintf(" (%s)", r.Elapsed.Truncate(100*time.Millisecond))
		}

		// Color: green if all pass, red if any fail, yellow if skip only
		if r.Failed > 0 {
			// red styling
			sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorError).Render(summary)))
		} else {
			// green styling
			sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorSuccess).Render(summary)))
		}

		// Show failure details (up to 3 failures, 2 lines each)
		if r.Failed > 0 {
			maxShow := 3
			if len(r.Failures) < maxShow {
				maxShow = len(r.Failures)
			}
			for i := 0; i < maxShow; i++ {
				f := r.Failures[i]
				sb.WriteString(fmt.Sprintf("    ✗ %s\n", lipgloss.NewStyle().Foreground(ColorError).Bold(true).Render(f.Name)))
				// Truncate output to first 2 lines
				lines := strings.SplitN(strings.TrimSpace(f.Output), "\n", 3)
				for j, l := range lines {
					if j >= 2 {
						break
					}
					sb.WriteString(fmt.Sprintf("      %s\n", lipgloss.NewStyle().Foreground(ColorDimText).Render(l)))
				}
			}
			if len(r.Failures) > 3 {
				sb.WriteString(fmt.Sprintf("    ... and %d more failures\n", len(r.Failures)-3))
			}
		}
	}

	return sb.String()
}

func (a *ActivityPanel) formatElapsed(item ActivityItem) string {
	var parts []string

	if !item.StartedAt.IsZero() {
		d := time.Since(item.StartedAt)
		if d < 0 {
			d = 0
		}
		seconds := float64(d.Milliseconds()) / 1000.0
		parts = append(parts, fmt.Sprintf("%.1fs", seconds))
	}

	if item.Tokens > 0 {
		parts = append(parts, fmt.Sprintf("%s tk", formatTokensShort(item.Tokens)))
	}

	if len(parts) == 0 {
		return ""
	}

	return lipgloss.NewStyle().Foreground(ColorDimText).Render(fmt.Sprintf("(%s)", strings.Join(parts, " · ")))
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

func renderProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	label := fmt.Sprintf("Step %d/%d", current, total)
	barWidth := width - len(label) - 3 // " [bar] "
	if barWidth < 5 {
		barWidth = 5
	}
	filled := barWidth * current / total
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	// Use lipgloss for coloring
	return fmt.Sprintf("%s [%s]", label, lipgloss.NewStyle().Foreground(ColorAccent).Render(bar))
}

func formatTokensShort(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(tokens)/1_000)
	}
	return fmt.Sprintf("%d", tokens)
}
