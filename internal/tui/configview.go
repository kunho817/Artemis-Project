package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/config"
)

const (
	tabAgents = 5 // 6th tab index (after 5 providers)
	tabSystem = 6 // 7th tab: system settings sub-tabs
	tabCount  = 7
)

const (
	sysSubMemory     = 0
	sysSubTools      = 1
	sysSubVector     = 2
	sysSubRepoMap    = 3
	sysSubGitHub     = 4
	sysSubAppearance = 5
	sysSubLSP        = 6
	sysSubSkills     = 7
	sysSubMCP        = 8
	sysSubCount      = 9
)

var sysSubLabels = []string{"Memory", "Tools", "Vector", "RepoMap", "GitHub", "Appearance", "LSP", "Skills", "MCP"}

// ConfigView is the settings screen for API and agent configuration.
type ConfigView struct {
	cfg       config.Config
	tabIdx    int // 0-3 = providers, 4 = agents
	fieldIdx  int // field within current tab
	inputs    []textinput.Model
	width     int
	height    int
	saved     bool
	statusMsg string
	sysSubTab int
}

// tab labels: 4 providers + Agents + System
var tabLabels = []string{"Claude", "Gemini", "GPT", "GLM", "VLLM", "Agents", "System"}

// NewConfigView creates a new config view from current config.
func NewConfigView(cfg config.Config) ConfigView {
	cv := ConfigView{
		cfg: cfg,
	}
	cv.buildInputs()
	return cv
}

// fieldCountForTab returns how many fields the current tab has.
func (cv *ConfigView) fieldCountForTab() int {
	switch cv.tabIdx {
	case tabAgents:
		return 2 // Agents toggle, Tier toggle
	case tabSystem:
		switch cv.sysSubTab {
		case sysSubMemory:
			return 4
		case sysSubTools:
			return 1
		case sysSubVector:
			return 3
		case sysSubRepoMap:
			return 2
		case sysSubGitHub:
			return 8
		case sysSubAppearance:
			return 1
		case sysSubLSP:
			return 2
		case sysSubSkills:
			return 2
		case sysSubMCP:
			return 1
		default:
			return 4
		}
	default:
		return 3 // API Key, Endpoint, Model
	}
}

// isOnAgentsTab returns true if viewing the Agents tab.
func (cv *ConfigView) isOnAgentsTab() bool {
	return cv.tabIdx == tabAgents
}

// isOnSystemTab returns true if viewing the System tab.
func (cv *ConfigView) isOnSystemTab() bool {
	return cv.tabIdx == tabSystem
}

// buildInputs creates text inputs for the current provider tab.
func (cv *ConfigView) buildInputs() {
	if cv.isOnAgentsTab() {
		cv.inputs = nil
		if cv.fieldIdx >= cv.fieldCountForTab() {
			cv.fieldIdx = 0
		}
		return
	}

	if cv.isOnSystemTab() {
		cv.buildSystemInputs()
		return
	}

	// Provider tabs — delegated to configview_providers.go
	cv.buildProviderInputs()
}

func (cv *ConfigView) focusField() {
	for i := range cv.inputs {
		cv.inputs[i].Blur()
	}
	if cv.isOnAgentsTab() {
		return
	}
	if cv.isOnSystemTab() {
		inputIdx := cv.systemFieldToInputIdx(cv.fieldIdx)
		if inputIdx >= 0 && inputIdx < len(cv.inputs) {
			cv.inputs[inputIdx].Focus()
			cv.inputs[inputIdx].PromptStyle = InputPromptStyle
			cv.inputs[inputIdx].TextStyle = InputTextStyle
		}
		return
	}
	if cv.fieldIdx < len(cv.inputs) {
		cv.inputs[cv.fieldIdx].Focus()
		cv.inputs[cv.fieldIdx].PromptStyle = InputPromptStyle
		cv.inputs[cv.fieldIdx].TextStyle = InputTextStyle
	}
}

// applyInputsToConfig writes text input values back to config.
func (cv *ConfigView) applyInputsToConfig() {
	if cv.isOnAgentsTab() {
		return
	}
	if cv.isOnSystemTab() {
		cv.applySystemInputs()
		return
	}
	// Provider tabs — delegated to configview_providers.go
	cv.applyProviderInputs()
}

// toggleAgentField handles Enter on agent toggle fields.
func (cv *ConfigView) toggleAgentField() {
	switch cv.fieldIdx {
	case 0: // Agent enabled
		cv.cfg.Agents.Enabled = !cv.cfg.Agents.Enabled
	case 1: // Tier
		if cv.cfg.Agents.Tier == "premium" {
			cv.cfg.Agents.Tier = "budget"
		} else {
			cv.cfg.Agents.Tier = "premium"
		}
	}
}

// SetSize updates dimensions.
func (cv *ConfigView) SetSize(w, h int) {
	cv.width = w
	cv.height = h
	inputWidth := w - 30
	if inputWidth < 20 {
		inputWidth = 20
	}
	for i := range cv.inputs {
		cv.inputs[i].Width = inputWidth
	}
}

// Update handles input for the config view.
func (cv *ConfigView) Update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+s", "esc":
			cv.applyInputsToConfig()
			if err := config.Save(cv.cfg); err != nil {
				cv.statusMsg = fmt.Sprintf("Save error: %v", err)
				return false, nil
			}
			cv.saved = true
			return true, nil

		case "tab", "down":
			cv.applyInputsToConfig()
			cv.fieldIdx = (cv.fieldIdx + 1) % cv.fieldCountForTab()
			cv.focusField()
			return false, nil

		case "shift+tab", "up":
			cv.applyInputsToConfig()
			cv.fieldIdx--
			if cv.fieldIdx < 0 {
				cv.fieldIdx = cv.fieldCountForTab() - 1
			}
			cv.focusField()
			return false, nil

		case "left", "right":
			if cv.isOnSystemTab() && cv.canSwitchSubTab() {
				cv.applyInputsToConfig()
				if msg.String() == "left" {
					cv.sysSubTab--
					if cv.sysSubTab < 0 {
						cv.sysSubTab = sysSubCount - 1
					}
				} else {
					cv.sysSubTab = (cv.sysSubTab + 1) % sysSubCount
				}
				cv.fieldIdx = 0
				cv.buildInputs()
				cv.SetSize(cv.width, cv.height)
				return false, nil
			}

		case "enter":
			if cv.isOnAgentsTab() {
				cv.toggleAgentField()
				return false, nil
			}
			if cv.isOnSystemTab() && cv.isSystemToggleField() {
				cv.toggleSystemField()
				return false, nil
			}

		case "ctrl+right":
			cv.applyInputsToConfig()
			cv.tabIdx = (cv.tabIdx + 1) % tabCount
			cv.fieldIdx = 0
			cv.buildInputs()
			cv.SetSize(cv.width, cv.height)
			return false, nil

		case "ctrl+left":
			cv.applyInputsToConfig()
			cv.tabIdx--
			if cv.tabIdx < 0 {
				cv.tabIdx = tabCount - 1
			}
			cv.fieldIdx = 0
			cv.buildInputs()
			cv.SetSize(cv.width, cv.height)
			return false, nil
		}
	}

	// Forward to active text input (provider tabs and system number fields)
	if !cv.isOnAgentsTab() {
		inputIdx := cv.fieldIdx
		if cv.isOnSystemTab() {
			inputIdx = cv.systemFieldToInputIdx(cv.fieldIdx)
		}
		if inputIdx >= 0 && inputIdx < len(cv.inputs) {
			var cmd tea.Cmd
			cv.inputs[inputIdx], cmd = cv.inputs[inputIdx].Update(msg)
			return false, cmd
		}
	}

	return false, nil
}

// GetConfig returns the current config (after editing).
func (cv *ConfigView) GetConfig() config.Config {
	cv.applyInputsToConfig()
	return cv.cfg
}

// View renders the config screen.
func (cv *ConfigView) View() string {
	var sb strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Padding(1, 2).
		Render("⚙ Settings")

	sb.WriteString(title + "\n\n")

	// Tabs
	sb.WriteString(cv.renderTabs())
	sb.WriteString("\n\n")

	// Divider
	divider := DividerStyle.Render(strings.Repeat("─", cv.width-4))
	sb.WriteString("  " + divider + "\n\n")

	// Content area — switch by tab
	if cv.isOnAgentsTab() {
		cv.renderAgentsContent(&sb)
	} else if cv.isOnSystemTab() {
		sb.WriteString(cv.renderSubTabs())
		sb.WriteString("\n")
		cv.renderSystemContent(&sb)
	} else {
		cv.renderProviderContent(&sb)
	}

	if cv.statusMsg != "" {
		sb.WriteString("\n  " + ActivityErrorStyle.Render(cv.statusMsg) + "\n")
	}

	// Help
	helpText := "[Tab/↑↓] Navigate  [Ctrl+←→] Switch tab  [Ctrl+S/Esc] Save & exit"
	if cv.isOnSystemTab() {
		if cv.canSwitchSubTab() {
			helpText = "[Tab/↑↓] Navigate  [←→] Sub-tab  [Enter] Toggle  [Ctrl+←→] Tab  [Ctrl+S/Esc] Save"
		} else {
			helpText = "[Tab/↑↓] Navigate  [←→] Sub-tab (on toggles)  [Ctrl+←→] Tab  [Ctrl+S/Esc] Save"
		}
	} else if cv.isOnAgentsTab() {
		helpText = "[Tab/↑↓] Navigate  [Enter] Toggle  [Ctrl+←→] Switch tab  [Ctrl+S/Esc] Save & exit"
	}
	help := lipgloss.NewStyle().Foreground(ColorDimText).Padding(1, 2).Render(helpText)
	sb.WriteString("\n" + help)

	return sb.String()
}

// renderTabs draws the tab row.
func (cv *ConfigView) renderTabs() string {
	var tabs []string
	for i, label := range tabLabels {
		if i == cv.tabIdx {
			// Active tab
			suffix := ""
			if i < len(config.ProviderNames()) {
				provName := config.ProviderNames()[i]
				if provName == "glm" {
					suffix = " (Coding Plan)"
				} else if provName == "vllm" {
					suffix = " (Local)"
				}
			}
			tab := lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBg).
				Background(ColorPrimary).
				Padding(0, 2).
				Render(label + suffix)
			tabs = append(tabs, tab)
		} else {
			// Inactive tab
			style := lipgloss.NewStyle().Padding(0, 2).Foreground(ColorDimText)
			if i < len(config.ProviderNames()) {
				provName := config.ProviderNames()[i]
				if cv.isProviderEnabled(provName) {
					style = style.Foreground(ColorSuccess)
				}
			} else if i == tabAgents {
				if cv.cfg.Agents.Enabled {
					style = style.Foreground(ColorAccent)
				}
			} else if i == tabSystem {
				if cv.cfg.Memory.Enabled || cv.cfg.Vector.Enabled || cv.cfg.RepoMap.Enabled || cv.cfg.GitHub.Enabled {
					style = style.Foreground(ColorAccent)
				}
			}
			tabs = append(tabs, style.Render(label))
		}
	}
	return "  " + lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

// renderAgentsContent draws the agent pipeline settings.
func (cv *ConfigView) renderAgentsContent(sb *strings.Builder) {
	// Agent Mode toggle (fieldIdx 0)
	cv.renderToggleField(sb, 0, "Agents",
		cv.cfg.Agents.Enabled, "Enabled", "Disabled")

	// Tier toggle (fieldIdx 1)
	tierLabel := "Premium"
	tierStyle := TierPremiumStyle
	if cv.cfg.Agents.Tier == "budget" {
		tierLabel = "Budget"
		tierStyle = TierBudgetStyle
	}
	cv.renderCustomToggle(sb, 1, "Tier", tierStyle.Render(tierLabel))

	// Role mapping table
	mappings := cv.cfg.Agents.Premium
	mappingLabel := "Premium"
	if cv.cfg.Agents.Tier == "budget" {
		mappings = cv.cfg.Agents.Budget
		mappingLabel = "Budget"
	}

	sb.WriteString(fmt.Sprintf("  %s\n\n",
		lipgloss.NewStyle().Foreground(ColorDimText).Italic(true).Padding(0, 1).
			Render(fmt.Sprintf("Role mappings (%s):", mappingLabel)),
	))

	for _, m := range mappings {
		roleLabel := RoleLabelStyle.Render(m.Role)
		provLabel := cv.styledProviderName(m.Provider)
		sb.WriteString(fmt.Sprintf("    %s → %s\n", roleLabel, provLabel))
	}
}

// renderToggleField renders a boolean toggle field.
func (cv *ConfigView) renderToggleField(sb *strings.Builder, idx int, label string, value bool, onText, offText string) {
	labelStyle := lipgloss.NewStyle().
		Width(22).
		Foreground(ColorDimText).
		Align(lipgloss.Right).
		Padding(0, 1)

	if cv.fieldIdx == idx {
		labelStyle = labelStyle.Foreground(ColorAccent).Bold(true)
	}

	var valueView string
	if value {
		valueView = ToggleOnStyle.Render("● " + onText)
	} else {
		valueView = ToggleOffStyle.Render("○ " + offText)
	}

	if cv.fieldIdx == idx {
		valueView += lipgloss.NewStyle().Foreground(ColorDimText).Render("  ↵ toggle")
	}

	sb.WriteString(fmt.Sprintf("  %s %s\n\n", labelStyle.Render(label+":"), valueView))
}

// renderCustomToggle renders a toggle with custom styled value.
func (cv *ConfigView) renderCustomToggle(sb *strings.Builder, idx int, label string, styledValue string) {
	labelStyle := lipgloss.NewStyle().
		Width(22).
		Foreground(ColorDimText).
		Align(lipgloss.Right).
		Padding(0, 1)

	if cv.fieldIdx == idx {
		labelStyle = labelStyle.Foreground(ColorAccent).Bold(true)
	}

	valueView := styledValue
	if cv.fieldIdx == idx {
		valueView += lipgloss.NewStyle().Foreground(ColorDimText).Render("  ↵ toggle")
	}

	sb.WriteString(fmt.Sprintf("  %s %s\n\n", labelStyle.Render(label+":"), valueView))
}

// System tab methods (renderSubTabs, buildSystemInputs, applySystemInputs,
// renderSystemContent, renderNumberField, toggleSystemField, isSystemToggleField,
// canSwitchSubTab, systemFieldToInputIdx) are in configview_system.go.
