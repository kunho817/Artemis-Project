package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

const (
	tabAgents = 4 // 5th tab index (after 4 providers)
	tabSystem = 5 // 6th tab: system settings sub-tabs
	tabCount  = 6
)

const (
	sysSubMemory     = 0
	sysSubTools      = 1
	sysSubVector     = 2
	sysSubRepoMap    = 3
	sysSubGitHub     = 4
	sysSubAppearance = 5
	sysSubCount      = 6
)

var sysSubLabels = []string{"Memory", "Tools", "Vector", "RepoMap", "GitHub", "Appearance"}

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
var tabLabels = []string{"Claude", "Gemini", "GPT", "GLM", "Agents", "System"}

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

// systemFieldToInputIdx maps System tab fieldIdx to text input array index.
// Returns -1 for toggle fields (no text input).
func (cv *ConfigView) systemFieldToInputIdx(fieldIdx int) int {
	switch cv.sysSubTab {
	case sysSubMemory:
		switch fieldIdx {
		case 2:
			return 0
		case 3:
			return 1
		default:
			return -1
		}
	case sysSubTools:
		if fieldIdx == 0 {
			return 0
		}
		return -1
	case sysSubVector:
		switch fieldIdx {
		case 1:
			return 0
		case 2:
			return 1
		default:
			return -1
		}
	case sysSubRepoMap:
		if fieldIdx == 1 {
			return 0
		}
		return -1
	case sysSubGitHub:
		switch fieldIdx {
		case 1:
			return 0
		case 2:
			return 1
		case 3:
			return 2
		case 4:
			return 3
		case 7:
			return 4
		default:
			return -1
		}
	default:
		return -1
	}
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

	providerName := config.ProviderNames()[cv.tabIdx]

	cv.inputs = make([]textinput.Model, 3)

	var apiKey, endpoint, model string

	if providerName == "glm" {
		glm := cv.cfg.GetGLM()
		apiKey = glm.APIKey
		endpoint = glm.Endpoint
		model = glm.Model
	} else {
		prov := cv.cfg.GetProvider(providerName)
		if prov == nil {
			return
		}
		apiKey = prov.APIKey
		endpoint = prov.Endpoint
		model = prov.Model
	}

	// API Key
	cv.inputs[0] = textinput.New()
	cv.inputs[0].Placeholder = "Enter API key..."
	cv.inputs[0].EchoMode = textinput.EchoPassword
	cv.inputs[0].EchoCharacter = '•'
	cv.inputs[0].SetValue(apiKey)
	cv.inputs[0].CharLimit = 256

	// Endpoint
	cv.inputs[1] = textinput.New()
	if providerName == "glm" {
		cv.inputs[1].Placeholder = "Enter Coding Plan endpoint..."
	} else {
		cv.inputs[1].Placeholder = "Enter endpoint URL..."
	}
	cv.inputs[1].SetValue(endpoint)
	cv.inputs[1].CharLimit = 512

	// Model
	cv.inputs[2] = textinput.New()
	cv.inputs[2].Placeholder = "Enter model name..."
	cv.inputs[2].SetValue(model)
	cv.inputs[2].CharLimit = 128

	if cv.fieldIdx >= cv.fieldCountForTab() {
		cv.fieldIdx = 0
	}
	cv.focusField()
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
	if len(cv.inputs) == 0 {
		return
	}

	providerName := config.ProviderNames()[cv.tabIdx]

	if providerName == "glm" {
		glm := cv.cfg.GetGLM()
		glm.APIKey = cv.inputs[0].Value()
		glm.Endpoint = cv.inputs[1].Value()
		glm.Model = cv.inputs[2].Value()
		glm.Enabled = glm.APIKey != ""
	} else {
		prov := cv.cfg.GetProvider(providerName)
		if prov == nil {
			return
		}
		prov.APIKey = cv.inputs[0].Value()
		prov.Endpoint = cv.inputs[1].Value()
		prov.Model = cv.inputs[2].Value()
		prov.Enabled = prov.APIKey != ""
	}
}

// isProviderEnabled checks if a provider is enabled.
func (cv *ConfigView) isProviderEnabled(name string) bool {
	if name == "glm" {
		glm := cv.cfg.GetGLM()
		return glm.Enabled || glm.APIKey != ""
	}
	prov := cv.cfg.GetProvider(name)
	if prov == nil {
		return false
	}
	return prov.Enabled || prov.APIKey != ""
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

// toggleSystemField handles Enter on system toggle fields.
func (cv *ConfigView) toggleSystemField() {
	switch cv.sysSubTab {
	case sysSubMemory:
		switch cv.fieldIdx {
		case 0:
			cv.cfg.Memory.Enabled = !cv.cfg.Memory.Enabled
		case 1:
			cv.cfg.Memory.ConsolidateOnExit = !cv.cfg.Memory.ConsolidateOnExit
		}
	case sysSubVector:
		if cv.fieldIdx == 0 {
			cv.cfg.Vector.Enabled = !cv.cfg.Vector.Enabled
		}
	case sysSubRepoMap:
		if cv.fieldIdx == 0 {
			cv.cfg.RepoMap.Enabled = !cv.cfg.RepoMap.Enabled
		}
	case sysSubGitHub:
		switch cv.fieldIdx {
		case 0:
			cv.cfg.GitHub.Enabled = !cv.cfg.GitHub.Enabled
		case 5:
			cv.cfg.GitHub.AutoTriage = !cv.cfg.GitHub.AutoTriage
		case 6:
			cv.cfg.GitHub.AutoFix = !cv.cfg.GitHub.AutoFix
		}
	case sysSubAppearance:
		if cv.fieldIdx != 0 {
			return
		}
		themes := theme.AvailableThemes()
		if len(themes) == 0 {
			return
		}
		current := cv.cfg.Theme
		if current == "" {
			current = "default"
		}
		idx := 0
		for i, t := range themes {
			if t == current {
				idx = i
				break
			}
		}
		cv.cfg.Theme = themes[(idx+1)%len(themes)]
	}
}

func (cv *ConfigView) isSystemToggleField() bool {
	if !cv.isOnSystemTab() {
		return false
	}

	switch cv.sysSubTab {
	case sysSubMemory:
		return cv.fieldIdx == 0 || cv.fieldIdx == 1
	case sysSubTools:
		return false
	case sysSubVector:
		return cv.fieldIdx == 0
	case sysSubRepoMap:
		return cv.fieldIdx == 0
	case sysSubGitHub:
		return cv.fieldIdx == 0 || cv.fieldIdx == 5 || cv.fieldIdx == 6
	case sysSubAppearance:
		return cv.fieldIdx == 0
	default:
		return false
	}
}

// canSwitchSubTab returns true if ←/→ should switch sub-tabs rather than
// move cursor in a text input. True on toggle/cycle fields or sub-tabs with
// no toggle fields at all (e.g. Tools).
func (cv *ConfigView) canSwitchSubTab() bool {
	if !cv.isOnSystemTab() {
		return false
	}
	if cv.isSystemToggleField() {
		return true
	}
	// Sub-tabs with no toggles: always allow sub-tab switch
	switch cv.sysSubTab {
	case sysSubTools:
		return true
	}
	return false
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
			if i < 4 {
				provName := config.ProviderNames()[i]
				if provName == "glm" {
					suffix = " (Coding Plan)"
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
			if i < 4 {
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

// renderProviderContent draws the provider settings fields.
func (cv *ConfigView) renderProviderContent(sb *strings.Builder) {
	providerName := config.ProviderNames()[cv.tabIdx]
	fieldLabels := []string{"API Key", "Endpoint", "Model"}
	if providerName == "glm" {
		fieldLabels[1] = "Coding Plan Endpoint"
	}

	for i, label := range fieldLabels {
		labelStyle := lipgloss.NewStyle().
			Width(22).
			Foreground(ColorDimText).
			Align(lipgloss.Right).
			Padding(0, 1)

		if i == cv.fieldIdx {
			labelStyle = labelStyle.Foreground(ColorAccent).Bold(true)
		}

		fieldLabel := labelStyle.Render(label + ":")

		var inputView string
		if i < len(cv.inputs) {
			inputView = cv.inputs[i].View()
		}

		sb.WriteString(fmt.Sprintf("  %s %s\n\n", fieldLabel, inputView))
	}

	// Provider status
	enabled := cv.isProviderEnabled(providerName)
	status := "Disabled"
	statusStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	if enabled {
		status = "Enabled"
		statusStyle = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	}
	sb.WriteString(fmt.Sprintf("  %s %s\n",
		lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
			Foreground(ColorDimText).Render("Status:"),
		statusStyle.Render(status),
	))
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

// styledProviderName returns a provider name with appropriate coloring.
func (cv *ConfigView) styledProviderName(name string) string {
	hasKey := cv.isProviderEnabled(name)
	style := RoleProviderStyle
	if !hasKey {
		style = style.Foreground(ColorMuted).Strikethrough(true)
	}
	return style.Render(name)
}

func (cv *ConfigView) renderSubTabs() string {
	parts := make([]string, 0, len(sysSubLabels))
	for i, label := range sysSubLabels {
		if i == cv.sysSubTab {
			parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(label))
			continue
		}

		style := lipgloss.NewStyle().Foreground(ColorDimText)
		switch i {
		case sysSubMemory:
			if cv.cfg.Memory.Enabled {
				style = style.Foreground(ColorSuccess)
			}
		case sysSubVector:
			if cv.cfg.Vector.Enabled {
				style = style.Foreground(ColorSuccess)
			}
		case sysSubRepoMap:
			if cv.cfg.RepoMap.Enabled {
				style = style.Foreground(ColorSuccess)
			}
		case sysSubGitHub:
			if cv.cfg.GitHub.Enabled {
				style = style.Foreground(ColorSuccess)
			}
		}
		parts = append(parts, style.Render(label))
	}

	row := "  " + strings.Join(parts, lipgloss.NewStyle().Foreground(ColorDimText).Render(" │ "))
	line := "  " + DividerStyle.Render(strings.Repeat("─", 42))
	return row + "\n" + line
}

// --- System tab (sub-tabs) ---

// buildSystemInputs creates text inputs for the System tab.
func (cv *ConfigView) buildSystemInputs() {
	switch cv.sysSubTab {
	case sysSubMemory:
		cv.inputs = make([]textinput.Model, 2)
		cv.inputs[0] = textinput.New()
		cv.inputs[0].Placeholder = "Days (0 = never prune)"
		cv.inputs[0].SetValue(strconv.Itoa(cv.cfg.Memory.MaxFactAge))
		cv.inputs[0].CharLimit = 6

		cv.inputs[1] = textinput.New()
		cv.inputs[1].Placeholder = "Minimum use count"
		cv.inputs[1].SetValue(strconv.Itoa(cv.cfg.Memory.MinFactUseCount))
		cv.inputs[1].CharLimit = 4

	case sysSubTools:
		cv.inputs = make([]textinput.Model, 1)
		cv.inputs[0] = textinput.New()
		cv.inputs[0].Placeholder = "0 = unlimited"
		cv.inputs[0].SetValue(strconv.Itoa(cv.cfg.MaxToolIter))
		cv.inputs[0].CharLimit = 6

	case sysSubVector:
		cv.inputs = make([]textinput.Model, 2)
		cv.inputs[0] = textinput.New()
		cv.inputs[0].Placeholder = "pa-..."
		cv.inputs[0].SetValue(cv.cfg.Vector.APIKey)
		cv.inputs[0].CharLimit = 128
		cv.inputs[0].EchoMode = textinput.EchoPassword
		cv.inputs[0].EchoCharacter = '•'

		cv.inputs[1] = textinput.New()
		cv.inputs[1].Placeholder = "voyage-code-3"
		cv.inputs[1].SetValue(cv.cfg.Vector.Model)
		cv.inputs[1].CharLimit = 64

	case sysSubRepoMap:
		cv.inputs = make([]textinput.Model, 1)
		cv.inputs[0] = textinput.New()
		cv.inputs[0].Placeholder = "2048"
		cv.inputs[0].SetValue(strconv.Itoa(cv.cfg.RepoMap.MaxTokens))
		cv.inputs[0].CharLimit = 6

	case sysSubGitHub:
		cv.inputs = make([]textinput.Model, 5)
		cv.inputs[0] = textinput.New()
		cv.inputs[0].Placeholder = "ghp_..."
		cv.inputs[0].SetValue(cv.cfg.GitHub.Token)
		cv.inputs[0].CharLimit = 256
		cv.inputs[0].EchoMode = textinput.EchoPassword
		cv.inputs[0].EchoCharacter = '•'

		cv.inputs[1] = textinput.New()
		cv.inputs[1].Placeholder = "username or org"
		cv.inputs[1].SetValue(cv.cfg.GitHub.Owner)
		cv.inputs[1].CharLimit = 64

		cv.inputs[2] = textinput.New()
		cv.inputs[2].Placeholder = "repository-name"
		cv.inputs[2].SetValue(cv.cfg.GitHub.Repo)
		cv.inputs[2].CharLimit = 128

		cv.inputs[3] = textinput.New()
		cv.inputs[3].Placeholder = "5"
		cv.inputs[3].SetValue(strconv.Itoa(cv.cfg.GitHub.PollInterval))
		cv.inputs[3].CharLimit = 4

		cv.inputs[4] = textinput.New()
		cv.inputs[4].Placeholder = "main"
		cv.inputs[4].SetValue(cv.cfg.GitHub.BaseBranch)
		cv.inputs[4].CharLimit = 64

	default:
		cv.inputs = nil
	}

	if cv.fieldIdx >= cv.fieldCountForTab() {
		cv.fieldIdx = 0
	}
	cv.focusField()
}

// applySystemInputs writes system tab values back to config.
func (cv *ConfigView) applySystemInputs() {
	switch cv.sysSubTab {
	case sysSubMemory:
		if len(cv.inputs) < 2 {
			return
		}
		if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[0].Value())); err == nil && v >= 0 {
			cv.cfg.Memory.MaxFactAge = v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[1].Value())); err == nil && v >= 0 {
			cv.cfg.Memory.MinFactUseCount = v
		}

	case sysSubTools:
		if len(cv.inputs) < 1 {
			return
		}
		if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[0].Value())); err == nil && v >= 0 {
			cv.cfg.MaxToolIter = v
		}

	case sysSubVector:
		if len(cv.inputs) < 2 {
			return
		}
		cv.cfg.Vector.APIKey = strings.TrimSpace(cv.inputs[0].Value())
		model := strings.TrimSpace(cv.inputs[1].Value())
		if model != "" {
			cv.cfg.Vector.Model = model
		}

	case sysSubRepoMap:
		if len(cv.inputs) < 1 {
			return
		}
		if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[0].Value())); err == nil && v >= 0 {
			cv.cfg.RepoMap.MaxTokens = v
		}

	case sysSubGitHub:
		if len(cv.inputs) < 5 {
			return
		}
		cv.cfg.GitHub.Token = strings.TrimSpace(cv.inputs[0].Value())
		cv.cfg.GitHub.Owner = strings.TrimSpace(cv.inputs[1].Value())
		cv.cfg.GitHub.Repo = strings.TrimSpace(cv.inputs[2].Value())
		if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[3].Value())); err == nil && v >= 0 {
			cv.cfg.GitHub.PollInterval = v
		}
		base := strings.TrimSpace(cv.inputs[4].Value())
		if base != "" {
			cv.cfg.GitHub.BaseBranch = base
		}
	}
}

// renderSystemContent draws the active System sub-tab.
func (cv *ConfigView) renderSystemContent(sb *strings.Builder) {
	switch cv.sysSubTab {
	case sysSubMemory:
		cv.renderToggleField(sb, 0, "Memory", cv.cfg.Memory.Enabled, "Enabled", "Disabled")
		cv.renderToggleField(sb, 1, "Consolidate", cv.cfg.Memory.ConsolidateOnExit, "On Exit", "Off")
		cv.renderNumberField(sb, 2, "Fact Max Age", "days", 0)
		cv.renderNumberField(sb, 3, "Fact Min Uses", "times", 1)
		if cv.cfg.Memory.Enabled {
			sb.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
					Foreground(ColorDimText).Render("DB Path:"),
				lipgloss.NewStyle().Foreground(ColorMuted).Render(cv.cfg.MemoryDBPath()),
			))
		}

	case sysSubTools:
		cv.renderNumberField(sb, 0, "Max Iterations", "0=unlimited", 0)

	case sysSubVector:
		cv.renderToggleField(sb, 0, "Vector Search", cv.cfg.Vector.Enabled, "Enabled", "Disabled")
		cv.renderNumberField(sb, 1, "Voyage API Key", "", 0)
		cv.renderNumberField(sb, 2, "Voyage Model", "", 1)
		if cv.cfg.Vector.Enabled {
			sb.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
					Foreground(ColorDimText).Render("Vec Path:"),
				lipgloss.NewStyle().Foreground(ColorMuted).Render(cv.cfg.VectorStorePath()),
			))
		}

	case sysSubRepoMap:
		cv.renderToggleField(sb, 0, "Repo Map", cv.cfg.RepoMap.Enabled, "Enabled", "Disabled")
		cv.renderNumberField(sb, 1, "Max Tokens", "tokens", 0)

	case sysSubGitHub:
		cv.renderToggleField(sb, 0, "GitHub", cv.cfg.GitHub.Enabled, "Enabled", "Disabled")
		cv.renderNumberField(sb, 1, "Token", "", 0)
		cv.renderNumberField(sb, 2, "Owner", "", 1)
		cv.renderNumberField(sb, 3, "Repo", "", 2)
		cv.renderNumberField(sb, 4, "Poll Interval", "minutes", 3)
		cv.renderToggleField(sb, 5, "Auto Triage", cv.cfg.GitHub.AutoTriage, "Enabled", "Disabled")
		cv.renderToggleField(sb, 6, "Auto Fix", cv.cfg.GitHub.AutoFix, "Enabled", "Disabled")
		cv.renderNumberField(sb, 7, "Base Branch", "", 4)

	case sysSubAppearance:
		themeName := cv.cfg.Theme
		if themeName == "" {
			themeName = "default"
		}
		themeDisplay := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(themeName)
		cv.renderCustomToggle(sb, 0, "Theme", themeDisplay)
		available := theme.AvailableThemes()
		if len(available) > 0 {
			hint := lipgloss.NewStyle().Foreground(ColorMuted).Render(
				fmt.Sprintf("Available: %s", strings.Join(available, ", ")))
			sb.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Width(22).Render(""),
				hint))
		}
	}
}

// renderNumberField renders a number input field with label and unit hint.
func (cv *ConfigView) renderNumberField(sb *strings.Builder, idx int, label, unit string, inputIdx int) {
	labelStyle := lipgloss.NewStyle().
		Width(22).
		Foreground(ColorDimText).
		Align(lipgloss.Right).
		Padding(0, 1)

	if cv.fieldIdx == idx {
		labelStyle = labelStyle.Foreground(ColorAccent).Bold(true)
	}

	fieldLabel := labelStyle.Render(label + ":")

	var inputView string
	if inputIdx >= 0 && inputIdx < len(cv.inputs) {
		inputView = cv.inputs[inputIdx].View()
	}

	unitHint := lipgloss.NewStyle().Foreground(ColorDimText).Render(" " + unit)

	sb.WriteString(fmt.Sprintf("  %s %s%s\n\n", fieldLabel, inputView, unitHint))
}
