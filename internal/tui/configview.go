package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

const (
	tabAgents  = 4 // 5th tab index (after 4 providers)
	tabSystem  = 5 // 6th tab: memory + tool settings
	tabCount   = 6
)

// ConfigView is the settings screen for API and agent configuration.
type ConfigView struct {
	cfg          config.Config
	tabIdx       int // 0-3 = providers, 4 = agents
	fieldIdx     int // field within current tab
	inputs       []textinput.Model
	width        int
	height       int
	saved        bool
	statusMsg    string
	sysViewport  viewport.Model // scrollable viewport for System tab
	sysVpReady   bool
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
		return 11 // Memory(2 toggles) + Facts(2 numbers) + Tools(1 number) + Vector(1 toggle + 2 inputs) + RepoMap(1 toggle + 1 number) + Appearance(1 cycle)
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
// Layout: 0,1=toggles 2,3,4=inputs[0..2] 5=toggle 6,7=inputs[3..4] 8=toggle 9=inputs[5] 10=cycle
func (cv *ConfigView) systemFieldToInputIdx(fieldIdx int) int {
	switch fieldIdx {
	case 2:
		return 0
	case 3:
		return 1
	case 4:
		return 2
	case 6:
		return 3
	case 7:
		return 4
	case 9:
		return 5
	default:
		return -1 // toggle fields
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
	switch cv.fieldIdx {
	case 0: // Memory enabled
		cv.cfg.Memory.Enabled = !cv.cfg.Memory.Enabled
	case 1: // Consolidate on exit
		cv.cfg.Memory.ConsolidateOnExit = !cv.cfg.Memory.ConsolidateOnExit
	case 5: // Vector enabled
		cv.cfg.Vector.Enabled = !cv.cfg.Vector.Enabled
	case 8: // RepoMap enabled
		cv.cfg.RepoMap.Enabled = !cv.cfg.RepoMap.Enabled
	case 10: // Theme cycle
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
	// Size viewport for System tab: content area = height minus title/tabs/divider/help (~10 lines)
	vpHeight := h - 10
	if vpHeight < 4 {
		vpHeight = 4
	}
	cv.sysViewport.Width = w - 4
	cv.sysViewport.Height = vpHeight
	cv.sysVpReady = true
}

// refreshSystemViewport renders System tab content into the viewport.
func (cv *ConfigView) refreshSystemViewport() {
	if !cv.sysVpReady {
		return
	}
	var sb strings.Builder
	cv.renderSystemContent(&sb)
	cv.sysViewport.SetContent(sb.String())
}

// scrollToField auto-scrolls the System tab viewport to keep the focused field visible.
func (cv *ConfigView) scrollToField() {
	if !cv.isOnSystemTab() || !cv.sysVpReady {
		return
	}
	// Approximate line offset per field in System tab content.
	// Each section header ~3 lines, each field ~2 lines + blank line.
	// Layout:  field 0 ~line 3, field 1 ~6, field 2 ~9, field 3 ~12,
	//          field 4 ~18 (section), field 5 ~24, field 6 ~27, field 7 ~30,
	//          field 8 ~36 (section), field 9 ~39, field 10 ~48 (section)
	fieldLineOffsets := []int{3, 6, 9, 12, 18, 24, 27, 30, 36, 39, 48}
	if cv.fieldIdx >= len(fieldLineOffsets) {
		return
	}
	target := fieldLineOffsets[cv.fieldIdx]
	// Ensure target line is visible — scroll so it's roughly in top third
	topThird := cv.sysViewport.Height / 3
	desired := target - topThird
	if desired < 0 {
		desired = 0
	}
	cv.sysViewport.SetYOffset(desired)
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
			cv.scrollToField()
			return false, nil

		case "shift+tab", "up":
			cv.applyInputsToConfig()
			cv.fieldIdx--
			if cv.fieldIdx < 0 {
				cv.fieldIdx = cv.fieldCountForTab() - 1
			}
			cv.focusField()
			cv.scrollToField()
			return false, nil

		case "enter":
			if cv.isOnAgentsTab() {
				cv.toggleAgentField()
				return false, nil
			}
			if cv.isOnSystemTab() && (cv.fieldIdx < 2 || cv.fieldIdx == 5 || cv.fieldIdx == 8 || cv.fieldIdx == 10) {
				cv.toggleSystemField()
				cv.refreshSystemViewport()
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

	// Forward mouse wheel and PgUp/PgDn to viewport on System tab
	if cv.isOnSystemTab() && cv.sysVpReady {
		switch msg := msg.(type) {
		case tea.MouseMsg:
			var cmd tea.Cmd
			cv.sysViewport, cmd = cv.sysViewport.Update(msg)
			return false, cmd
		case tea.KeyMsg:
			switch msg.String() {
			case "pgup", "pgdown":
				var cmd tea.Cmd
				cv.sysViewport, cmd = cv.sysViewport.Update(msg)
				return false, cmd
			}
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
		cv.refreshSystemViewport()
		sb.WriteString(cv.sysViewport.View())
	} else {
		cv.renderProviderContent(&sb)
	}

	if cv.statusMsg != "" {
		sb.WriteString("\n  " + ActivityErrorStyle.Render(cv.statusMsg) + "\n")
	}

	// Help
	helpText := "[Tab/↑↓] Navigate  [Ctrl+←→] Switch tab  [Ctrl+S/Esc] Save & exit"
	if cv.isOnSystemTab() {
		if cv.fieldIdx < 2 || cv.fieldIdx == 5 || cv.fieldIdx == 8 || cv.fieldIdx == 10 {
			helpText = "[Tab/↑↓] Navigate  [Enter] Toggle  [PgUp/PgDn] Scroll  [Ctrl+←→] Tab  [Ctrl+S/Esc] Save"
		} else {
			helpText = "[Tab/↑↓] Navigate  [PgUp/PgDn] Scroll  [Ctrl+←→] Switch tab  [Ctrl+S/Esc] Save & exit"
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
			} else if i == tabSystem {
				suffix = " (Memory, Tools & Repo Map)"
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
				if cv.cfg.Memory.Enabled || cv.cfg.Vector.Enabled || cv.cfg.RepoMap.Enabled {
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

// --- System tab (Memory + Tool settings) ---

// buildSystemInputs creates text inputs for the System tab.
func (cv *ConfigView) buildSystemInputs() {
	// 6 text inputs: Max Fact Age, Min Fact Use, Max Tool Iter, Voyage API Key, Voyage Model, RepoMap Max Tokens
	cv.inputs = make([]textinput.Model, 6)

	// Max Fact Age (days) -> inputs[0]
	cv.inputs[0] = textinput.New()
	cv.inputs[0].Placeholder = "Days (0 = never prune)"
	cv.inputs[0].SetValue(strconv.Itoa(cv.cfg.Memory.MaxFactAge))
	cv.inputs[0].CharLimit = 6

	// Min Fact Use Count -> inputs[1]
	cv.inputs[1] = textinput.New()
	cv.inputs[1].Placeholder = "Minimum use count"
	cv.inputs[1].SetValue(strconv.Itoa(cv.cfg.Memory.MinFactUseCount))
	cv.inputs[1].CharLimit = 4

	// Max Tool Iterations -> inputs[2]
	cv.inputs[2] = textinput.New()
	cv.inputs[2].Placeholder = "0 = unlimited"
	cv.inputs[2].SetValue(strconv.Itoa(cv.cfg.MaxToolIter))
	cv.inputs[2].CharLimit = 6

	// Voyage API Key -> inputs[3]
	cv.inputs[3] = textinput.New()
	cv.inputs[3].Placeholder = "pa-..."
	cv.inputs[3].SetValue(cv.cfg.Vector.APIKey)
	cv.inputs[3].CharLimit = 128
	cv.inputs[3].EchoMode = textinput.EchoPassword

	// Voyage Model -> inputs[4]
	cv.inputs[4] = textinput.New()
	cv.inputs[4].Placeholder = "voyage-code-3"
	cv.inputs[4].SetValue(cv.cfg.Vector.Model)
	cv.inputs[4].CharLimit = 64

	// RepoMap Max Tokens -> inputs[5]
	cv.inputs[5] = textinput.New()
	cv.inputs[5].Placeholder = "2048"
	cv.inputs[5].SetValue(strconv.Itoa(cv.cfg.RepoMap.MaxTokens))
	cv.inputs[5].CharLimit = 6

	if cv.fieldIdx >= cv.fieldCountForTab() {
		cv.fieldIdx = 0
	}
	cv.focusField()
}

// applySystemInputs writes system tab values back to config.
func (cv *ConfigView) applySystemInputs() {
	if len(cv.inputs) < 6 {
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[0].Value())); err == nil && v >= 0 {
		cv.cfg.Memory.MaxFactAge = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[1].Value())); err == nil && v >= 0 {
		cv.cfg.Memory.MinFactUseCount = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[2].Value())); err == nil && v >= 0 {
		cv.cfg.MaxToolIter = v
	}
	cv.cfg.Vector.APIKey = strings.TrimSpace(cv.inputs[3].Value())
	model := strings.TrimSpace(cv.inputs[4].Value())
	if model != "" {
		cv.cfg.Vector.Model = model
	}
	if v, err := strconv.Atoi(strings.TrimSpace(cv.inputs[5].Value())); err == nil && v >= 0 {
		cv.cfg.RepoMap.MaxTokens = v
	}
}

// renderSystemContent draws the System tab with Memory and Tool settings.
func (cv *ConfigView) renderSystemContent(sb *strings.Builder) {
	sb.WriteString(fmt.Sprintf("  %s\n\n",
		SectionHeaderStyle.Render("Memory")))

	// Memory Enabled toggle (fieldIdx 0)
	cv.renderToggleField(sb, 0, "Memory",
		cv.cfg.Memory.Enabled, "Enabled", "Disabled")

	// Consolidate on Exit toggle (fieldIdx 1)
	cv.renderToggleField(sb, 1, "Consolidate",
		cv.cfg.Memory.ConsolidateOnExit, "On Exit", "Off")

	// Max Fact Age (fieldIdx 2 → inputs[0])
	cv.renderNumberField(sb, 2, "Fact Max Age", "days", 0)

	// Min Fact Use Count (fieldIdx 3 → inputs[1])
	cv.renderNumberField(sb, 3, "Fact Min Uses", "times", 1)

	// Divider before tool settings
	sb.WriteString(fmt.Sprintf("\n  %s\n\n",
		SectionHeaderStyle.Render("Tools")))

	// Max Tool Iterations (fieldIdx 4 → inputs[2])
	cv.renderNumberField(sb, 4, "Max Iterations", "0=unlimited", 2)

	// Divider before vector settings
	sb.WriteString(fmt.Sprintf("\n  %s\n\n",
		SectionHeaderStyle.Render("Vector Search")))

	// Vector Enabled toggle (fieldIdx 5)
	cv.renderToggleField(sb, 5, "Vector Search",
		cv.cfg.Vector.Enabled, "Enabled", "Disabled")

	// Voyage API Key (fieldIdx 6 → inputs[3])
	cv.renderNumberField(sb, 6, "Voyage API Key", "", 3)

	// Voyage Model (fieldIdx 7 → inputs[4])
	cv.renderNumberField(sb, 7, "Model", "", 4)


	// Divider before repo-map settings
	sb.WriteString(fmt.Sprintf("\n  %s\n\n",
		SectionHeaderStyle.Render("Repo Map")))

	// RepoMap Enabled toggle (fieldIdx 8)
	cv.renderToggleField(sb, 8, "Repo Map",
		cv.cfg.RepoMap.Enabled, "Enabled", "Disabled")

	// RepoMap Max Tokens (fieldIdx 9 -> inputs[5])
	cv.renderNumberField(sb, 9, "Max Tokens", "tokens", 5)

	// Info display
	if cv.cfg.Memory.Enabled {
		dbPath := cv.cfg.MemoryDBPath()
		sb.WriteString(fmt.Sprintf("\n  %s %s\n",
			lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
				Foreground(ColorDimText).Render("DB Path:"),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(dbPath),
		))
	}
	if cv.cfg.Vector.Enabled {
		vecPath := cv.cfg.VectorStorePath()
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
				Foreground(ColorDimText).Render("Vec Path:"),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(vecPath),
		))
	}

	// Divider before appearance settings
	sb.WriteString(fmt.Sprintf("\n  %s\n\n",
		SectionHeaderStyle.Render("Appearance")))

	// Theme cycle (fieldIdx 10)
	themeName := cv.cfg.Theme
	if themeName == "" {
		themeName = "default"
	}
	themeDisplay := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(themeName)
	cv.renderCustomToggle(sb, 10, "Theme", themeDisplay)

	// Available themes hint
	available := theme.AvailableThemes()
	if len(available) > 0 {
		hint := lipgloss.NewStyle().Foreground(ColorMuted).Render(
			fmt.Sprintf("Available: %s", strings.Join(available, ", ")))
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			lipgloss.NewStyle().Width(22).Render(""),
			hint))
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
