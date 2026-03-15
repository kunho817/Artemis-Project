package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	chAnsi "github.com/charmbracelet/x/ansi"
	museAnsi "github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"

	"github.com/artemis-project/artemis/internal/tui/theme"
)

// OverlayKind identifies which overlay is active.
type OverlayKind int

const (
	OverlayNone OverlayKind = iota
	OverlayCommandPalette
	OverlayAgentSelector
	OverlayFilePicker
	OverlayRecovery
	OverlayResume
	OverlayDiff
)

// OverlayResult carries the outcome when an overlay closes.
type OverlayResult struct {
	Action string // e.g. "command", "toggle_agents", "select_file", "switch_tier", "switch_theme"
	Value  string // associated value (command text, file path, theme name, etc.)
}

// Overlay is the interface for floating dialog components.
type Overlay interface {
	// Update handles input. Returns true when the overlay should close.
	Update(msg tea.Msg) (closed bool, result OverlayResult, cmd tea.Cmd)

	// View renders the overlay content (including its own border/background).
	View() string

	// SetSize updates the available terminal dimensions.
	SetSize(w, h int)
}

// overlayBoxStyle returns the standard overlay box style with border and background.
func overlayBoxStyle(width, height int) lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.PrimaryColor()).
		Background(t.PanelColor()).
		Padding(1, 2)
}

// overlayTitleStyle returns the style for overlay titles.
func overlayTitleStyle() lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(t.PrimaryColor()).
		Background(t.PanelColor())
}

// overlayDimStyle returns the style for dimmed/hint text in overlays.
func overlayDimStyle() lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Foreground(t.DimTextColor()).
		Background(t.PanelColor())
}

// overlayItemStyle returns the style for normal items in overlay lists.
func overlayItemStyle() lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Foreground(t.TextColor()).
		Background(t.PanelColor())
}

// overlaySelectedStyle returns the style for the selected/highlighted item.
func overlaySelectedStyle() lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Foreground(t.BackgroundColor()).
		Background(t.PrimaryColor()).
		Bold(true)
}

// overlayAccentStyle returns the style for accent text in overlays.
func overlayAccentStyle() lipgloss.Style {
	t := &theme.Active.Colors
	return lipgloss.NewStyle().
		Foreground(t.AccentColor()).
		Background(t.PanelColor())
}

// --- PlaceOverlay compositing ---

// PlaceOverlay places fg on top of bg at position (x, y).
// Adapted from opencode-ai/opencode (MIT license).
func PlaceOverlay(x, y int, fg, bg string) string {
	fgLines, fgWidth := getOverlayLines(fg)
	bgLines, bgWidth := getOverlayLines(bg)
	bgHeight := len(bgLines)
	fgHeight := len(fgLines)

	if fgWidth >= bgWidth && fgHeight >= bgHeight {
		return fg
	}

	// Clamp position
	x = clampInt(x, 0, maxInt(0, bgWidth-fgWidth))
	y = clampInt(y, 0, maxInt(0, bgHeight-fgHeight))

	var b strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < y || i >= y+fgHeight {
			b.WriteString(bgLine)
			continue
		}

		pos := 0
		if x > 0 {
			left := truncate.String(bgLine, uint(x))
			pos = museAnsi.PrintableRuneWidth(left)
			b.WriteString(left)
			if pos < x {
				b.WriteString(strings.Repeat(" ", x-pos))
				pos = x
			}
		}

		fgLine := fgLines[i-y]
		b.WriteString(fgLine)
		pos += museAnsi.PrintableRuneWidth(fgLine)

		// Right side of background
		bgLineWidth := museAnsi.PrintableRuneWidth(bgLine)
		right := cutLeft(bgLine, pos)
		rightWidth := museAnsi.PrintableRuneWidth(right)
		if rightWidth <= bgLineWidth-pos {
			b.WriteString(strings.Repeat(" ", bgLineWidth-rightWidth-pos))
		}
		b.WriteString(right)
	}

	return b.String()
}

// PlaceOverlayCentered places fg centered on top of bg.
func PlaceOverlayCentered(fg, bg string, totalWidth, totalHeight int) string {
	fgWidth := lipgloss.Width(fg)
	fgHeight := lipgloss.Height(fg)
	x := (totalWidth - fgWidth) / 2
	y := (totalHeight - fgHeight) / 2
	return PlaceOverlay(x, y, fg, bg)
}

func getOverlayLines(s string) (lines []string, widest int) {
	lines = strings.Split(s, "\n")
	for _, l := range lines {
		w := museAnsi.PrintableRuneWidth(l)
		if widest < w {
			widest = w
		}
	}
	return lines, widest
}

// cutLeft cuts printable characters from the left of a string.
func cutLeft(s string, cutWidth int) string {
	return chAnsi.Cut(s, cutWidth, lipgloss.Width(s))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
