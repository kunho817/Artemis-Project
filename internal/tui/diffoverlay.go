package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DiffViewMsg requests the diff overlay to open with diff content.
type DiffViewMsg struct {
	FileName string
	Diff     string // unified diff format
}

// DiffOverlay displays file diffs in a scrollable viewport.
type DiffOverlay struct {
	fileName string
	viewport viewport.Model
	width    int
	height   int
	ready    bool
}

func NewDiffOverlay(fileName, diff string, width, height int) *DiffOverlay {
	// Leave margin for overlay borders
	w := width - 8
	h := height - 6
	if w < 40 {
		w = 40
	}
	if h < 10 {
		h = 10
	}

	vp := viewport.New(w, h)
	vp.SetContent(renderDiff(diff, w))

	return &DiffOverlay{
		fileName: fileName,
		viewport: vp,
		width:    w + 4, // add padding
		height:   h + 4,
		ready:    true,
	}
}

func (d *DiffOverlay) Init() tea.Cmd { return nil }

func (d *DiffOverlay) Update(msg tea.Msg) (bool, OverlayResult, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return true, OverlayResult{Action: "diff", Value: "close"}, nil
		}
	}

	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return false, OverlayResult{}, cmd
}

func (d *DiffOverlay) View() string {
	// Title bar
	title := DiffTitleStyle.Render(fmt.Sprintf(" Diff: %s ", d.fileName))

	// Help bar
	help := DiffHintStyle.Render("↑↓/PgUp/PgDn scroll · q/Esc close")

	// Scroll indicator
	scrollInfo := fmt.Sprintf("%d%%", int(d.viewport.ScrollPercent()*100))

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		d.viewport.View(),
		lipgloss.JoinHorizontal(lipgloss.Top, help, "  ", scrollInfo),
	)

	return DiffBorderStyle.
		Padding(0, 1).
		Render(content)
}

func (d *DiffOverlay) SetSize(w, h int) {
	d.width = w
	d.height = h
	vw := w - 8
	vh := h - 6
	if vw < 40 {
		vw = 40
	}
	if vh < 10 {
		vh = 10
	}
	d.viewport.Width = vw
	d.viewport.Height = vh
}

// renderDiff applies syntax highlighting to unified diff text.
func renderDiff(diff string, width int) string {
	var sb strings.Builder

	addStyle := DiffAddStyle
	removeStyle := DiffRemoveStyle
	headerStyle := DiffHeaderStyle
	hunkStyle := DiffHunkStyle
	contextStyle := DiffContextStyle

	for _, line := range strings.Split(diff, "\n") {
		// Truncate long lines
		if len(line) > width-2 {
			line = line[:width-5] + "..."
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(headerStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(removeStyle.Render(line))
		case strings.HasPrefix(line, "diff "):
			sb.WriteString(headerStyle.Render(line))
		default:
			sb.WriteString(contextStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
