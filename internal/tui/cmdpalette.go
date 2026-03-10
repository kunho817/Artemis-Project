package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaletteItem is a single command/action entry in the command palette.
type PaletteItem struct {
	Label       string
	Description string
	Shortcut    string
	Action      string
	Value       string
}

// Len implements fuzzy.Source.
func (p PaletteItem) Len() int { return len(p.searchString()) }

// String implements fuzzy.Source.
func (p PaletteItem) String(i int) string {
	s := p.searchString()
	if i < 0 || i >= len(s) {
		return ""
	}
	return string(s[i])
}

func (p PaletteItem) searchString() string {
	return strings.ToLower(strings.TrimSpace(p.Label + " " + p.Description))
}

// CommandPalette is an overlay with fuzzy search over available commands/actions.
type CommandPalette struct {
	input    textinput.Model
	items    []PaletteItem
	filtered []PaletteItem
	cursor   int
	width    int
	height   int
}

// NewCommandPalette creates the command palette overlay.
func NewCommandPalette(termWidth, termHeight int) *CommandPalette {
	in := textinput.New()
	in.Placeholder = "Search..."
	in.Prompt = "│ "
	in.CharLimit = 128
	in.Focus()
	in.PromptStyle = overlayAccentStyle()
	in.TextStyle = overlayItemStyle()

	cp := &CommandPalette{
		input: in,
		items: []PaletteItem{
			{Label: "Sessions", Description: "List previous sessions", Shortcut: "—", Action: "command", Value: "/sessions"},
			{Label: "Load Session", Description: "Load a previous session", Shortcut: "—", Action: "command", Value: "/load"},
			{Label: "Help", Description: "Show available commands", Shortcut: "—", Action: "command", Value: "/help"},
			{Label: "Clear Chat", Description: "Reset conversation", Shortcut: "Ctrl+L", Action: "clear"},
			{Label: "Settings", Description: "Open settings view", Shortcut: "Ctrl+S", Action: "settings"},
			{Label: "Toggle Agents", Description: "Enable/disable multi-agent", Shortcut: "—", Action: "toggle_agents"},
			{Label: "Switch Tier", Description: "Toggle premium/budget", Shortcut: "—", Action: "switch_tier"},
			{Label: "Switch Theme", Description: "Cycle to next theme", Shortcut: "—", Action: "switch_theme"},
			{Label: "Export Theme", Description: "Save current theme to ~/.artemis/themes/", Shortcut: "—", Action: "export_theme"},
		},
	}
	cp.SetSize(termWidth, termHeight)
	cp.filter()
	return cp
}

// Init returns the initial command for the palette input.
func (cp *CommandPalette) Init() tea.Cmd {
	return textinput.Blink
}

// SetSize updates dimensions.
func (cp *CommandPalette) SetSize(w, h int) {
	cp.width = clampInt(overlayPaletteMin(60, w-10), 36, 60)
	cp.height = clampInt(overlayPaletteMin(16, h-6), 10, 16)
	cp.input.Width = cp.width - 8
}

// Update handles palette input.
func (cp *CommandPalette) Update(msg tea.Msg) (bool, OverlayResult, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return true, OverlayResult{}, nil
		case "up":
			if cp.cursor > 0 {
				cp.cursor--
			}
			return false, OverlayResult{}, nil
		case "down":
			if cp.cursor < len(cp.filtered)-1 {
				cp.cursor++
			}
			return false, OverlayResult{}, nil
		case "enter":
			if len(cp.filtered) == 0 {
				return true, OverlayResult{}, nil
			}
			it := cp.filtered[cp.cursor]
			return true, OverlayResult{Action: it.Action, Value: it.Value}, nil
		}
	}

	var cmd tea.Cmd
	cp.input, cmd = cp.input.Update(msg)
	cp.filter()
	return false, OverlayResult{}, cmd
}

// View renders the command palette.
func (cp *CommandPalette) View() string {
	visible := 8
	if max := cp.height - 8; max > 0 && max < visible {
		visible = max
	}
	if visible < 1 {
		visible = 1
	}

	start := 0
	if cp.cursor >= visible {
		start = cp.cursor - visible + 1
	}
	end := overlayPaletteMin(len(cp.filtered), start+visible)

	var sb strings.Builder
	sb.WriteString(overlayTitleStyle().Render("Command Palette"))
	sb.WriteString("\n")
	sb.WriteString(cp.input.View())
	sb.WriteString("\n")
	sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", cp.width-6)))
	sb.WriteString("\n")

	for i := start; i < end; i++ {
		it := cp.filtered[i]
		prefix := "  "
		lineStyle := overlayItemStyle()
		if i == cp.cursor {
			prefix = "> "
			lineStyle = overlaySelectedStyle()
		}

		right := strings.TrimSpace(it.Shortcut)
		if right == "" || right == "—" {
			right = it.Description
		}
		left := it.Label
		gap := cp.width - 8 - lipgloss.Width(prefix) - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 1 {
			gap = 1
		}

		line := prefix + left + strings.Repeat(" ", gap) + right
		sb.WriteString(lineStyle.Width(cp.width - 6).Render(line))
		sb.WriteString("\n")
	}

	for i := end - start; i < visible; i++ {
		sb.WriteString(strings.Repeat(" ", cp.width-6))
		sb.WriteString("\n")
	}

	sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", cp.width-6)))
	sb.WriteString("\n")
	sb.WriteString(overlayDimStyle().Render("↑↓ navigate  ↵ select  esc close"))

	return overlayBoxStyle(cp.width, cp.height).Render(sb.String())
}

func (cp *CommandPalette) filter() {
	query := strings.TrimSpace(cp.input.Value())
	if query == "" {
		cp.filtered = append([]PaletteItem(nil), cp.items...)
		if cp.cursor >= len(cp.filtered) {
			cp.cursor = len(cp.filtered) - 1
		}
		if cp.cursor < 0 {
			cp.cursor = 0
		}
		return
	}

	type scored struct {
		item  PaletteItem
		score int
	}

	q := strings.ToLower(query)
	scoredItems := make([]scored, 0, len(cp.items))
	for _, it := range cp.items {
		s := fuzzySubsequenceScore(q, it.searchString())
		if s >= 0 {
			scoredItems = append(scoredItems, scored{item: it, score: s})
		}
	}

	sort.SliceStable(scoredItems, func(i, j int) bool {
		return scoredItems[i].score > scoredItems[j].score
	})

	cp.filtered = cp.filtered[:0]
	for _, si := range scoredItems {
		cp.filtered = append(cp.filtered, si.item)
	}

	if cp.cursor >= len(cp.filtered) {
		cp.cursor = len(cp.filtered) - 1
	}
	if cp.cursor < 0 {
		cp.cursor = 0
	}
}

func overlayPaletteMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fuzzySubsequenceScore returns -1 when query doesn't match target as subsequence.
// Higher score means better match.
func fuzzySubsequenceScore(query, target string) int {
	if query == "" {
		return 0
	}
	qi := 0
	score := 0
	streak := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			qi++
			streak++
			score += 10 + streak*2
		} else {
			streak = 0
		}
	}
	if qi != len(query) {
		return -1
	}
	return score - (len(target) - len(query))
}

var _ Overlay = (*CommandPalette)(nil)
