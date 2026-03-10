package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/config"
)

type AgentSelector struct {
	cfg     config.Config
	cursor  int
	items   []agentSelectorItem
	width   int
	height  int
	changed bool
}

type agentSelectorItem struct {
	Label    string
	Kind     string
	Provider string
}

func NewAgentSelector(cfg config.Config, termWidth, termHeight int) *AgentSelector {
	as := &AgentSelector{cfg: cfg}
	as.SetSize(termWidth, termHeight)
	as.rebuildItems()
	return as
}

func (as *AgentSelector) SetSize(w, h int) {
	as.width = clampInt(minInt(50, w-10), 38, 50)
	as.height = clampInt(minInt(20, h-4), 14, 20)
}

func (as *AgentSelector) Update(msg tea.Msg) (bool, OverlayResult, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if as.changed {
				return true, OverlayResult{Action: "agents_changed"}, nil
			}
			return true, OverlayResult{}, nil
		case "up":
			if as.cursor > 0 {
				as.cursor--
			}
			return false, OverlayResult{}, nil
		case "down":
			if as.cursor < len(as.items)-1 {
				as.cursor++
			}
			return false, OverlayResult{}, nil
		case "enter":
			switch as.cursor {
			case 0:
				as.cfg.Agents.Enabled = !as.cfg.Agents.Enabled
				as.changed = true
				as.rebuildItems()
			case 1:
				if as.cfg.Agents.Tier == "premium" {
					as.cfg.Agents.Tier = "budget"
				} else {
					as.cfg.Agents.Tier = "premium"
				}
				as.changed = true
				as.rebuildItems()
			}
			return false, OverlayResult{}, nil
		}
	}
	return false, OverlayResult{}, nil
}

func (as *AgentSelector) View() string {
	var sb strings.Builder
	sb.WriteString(overlayTitleStyle().Render("Agent Selector"))
	sb.WriteString("\n\n")

	maxRows := as.height - 8
	if maxRows < 3 {
		maxRows = 3
	}

	for i := 0; i < len(as.items) && i < maxRows; i++ {
		it := as.items[i]
		prefix := "  "
		style := overlayItemStyle()
		if i == as.cursor {
			prefix = "> "
			style = overlaySelectedStyle()
		}

		line := as.renderItemLine(it)
		sb.WriteString(style.Width(as.width - 6).Render(prefix + line))
		sb.WriteString("\n")

		if i == 1 {
			sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", as.width-6)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(overlayDimStyle().Render(strings.Repeat("─", as.width-6)))
	sb.WriteString("\n")
	sb.WriteString(overlayDimStyle().Render("↑↓ navigate  ↵ toggle  esc close"))

	return overlayBoxStyle(as.width, as.height).Render(sb.String())
}

func (as *AgentSelector) Config() config.Config {
	return as.cfg
}

func (as *AgentSelector) rebuildItems() {
	as.items = as.items[:0]

	enabled := "○ Disabled"
	if as.cfg.Agents.Enabled {
		enabled = "● Enabled"
	}
	as.items = append(as.items, agentSelectorItem{Label: "Agents", Kind: "toggle", Provider: enabled})

	tier := "Budget"
	if as.cfg.Agents.Tier == "premium" {
		tier = "Premium"
	}
	as.items = append(as.items, agentSelectorItem{Label: "Tier", Kind: "tier", Provider: tier})

	mappings := as.cfg.Agents.Premium
	if as.cfg.Agents.Tier == "budget" {
		mappings = as.cfg.Agents.Budget
	}
	for _, m := range mappings {
		as.items = append(as.items, agentSelectorItem{Label: m.Role, Kind: "role", Provider: m.Provider})
	}

	if as.cursor >= len(as.items) {
		as.cursor = len(as.items) - 1
	}
	if as.cursor < 0 {
		as.cursor = 0
	}
}

func (as *AgentSelector) renderItemLine(it agentSelectorItem) string {
	switch it.Kind {
	case "toggle":
		return fmt.Sprintf("%-10s %s", it.Label+":", it.Provider)
	case "tier":
		return fmt.Sprintf("%-10s %s", it.Label+":", it.Provider)
	default:
		return fmt.Sprintf("%-12s → %s", it.Label, it.Provider)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ Overlay = (*AgentSelector)(nil)
