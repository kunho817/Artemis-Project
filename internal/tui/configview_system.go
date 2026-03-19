package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/tui/theme"
)

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
	case sysSubLSP:
		return -1
	case sysSubSkills:
		return -1
	case sysSubMCP:
		return -1
	default:
		return -1
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

	case sysSubLSP:
		switch cv.fieldIdx {
		case 0:
			cv.cfg.LSP.Enabled = !cv.cfg.LSP.Enabled
		case 1:
			cv.cfg.LSP.AutoDetect = !cv.cfg.LSP.AutoDetect
		}

	case sysSubSkills:
		switch cv.fieldIdx {
		case 0:
			cv.cfg.Skills.Enabled = !cv.cfg.Skills.Enabled
		case 1:
			cv.cfg.Skills.AutoLoad = !cv.cfg.Skills.AutoLoad
		}

	case sysSubMCP:
		switch cv.fieldIdx {
		case 0:
			cv.cfg.MCP.Enabled = !cv.cfg.MCP.Enabled
		}
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
	case sysSubLSP:
		return cv.fieldIdx == 0 || cv.fieldIdx == 1
	case sysSubSkills:
		return true
	case sysSubMCP:
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
	case sysSubSkills:
		return true
	case sysSubMCP:
		return true
	}
	return false
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

	case sysSubLSP:
		cv.inputs = make([]textinput.Model, 0)

	case sysSubSkills:
		cv.inputs = make([]textinput.Model, 0)

	case sysSubMCP:
		cv.inputs = make([]textinput.Model, 0)

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

	case sysSubLSP:
		// No text inputs to apply — LSP only has toggles

	case sysSubSkills:
		// No text inputs

	case sysSubMCP:
		// No text inputs
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

	case sysSubLSP:
		cv.renderToggleField(sb, 0, "LSP", cv.cfg.LSP.Enabled, "Enabled", "Disabled")
		cv.renderToggleField(sb, 1, "Auto-Detect", cv.cfg.LSP.AutoDetect, "Enabled", "Disabled")
		if cv.cfg.LSP.Enabled {
			// Display configured servers
			sb.WriteString("\n")
			for lang, sc := range cv.cfg.LSP.Servers {
				status := "disabled"
				if sc.Enabled {
					status = sc.Command
				}
				sb.WriteString(fmt.Sprintf("  %s %s\n",
					lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
						Foreground(ColorDimText).Render(lang+":"),
					lipgloss.NewStyle().Foreground(ColorMuted).Render(status),
				))
			}
		}

	case sysSubSkills:
		cv.renderToggleField(sb, 0, "Skills", cv.cfg.Skills.Enabled, "Enabled", "Disabled")
		cv.renderToggleField(sb, 1, "Auto-Load Project", cv.cfg.Skills.AutoLoad, "Enabled", "Disabled")
		// Show global skills directory
		globalDir := cv.cfg.GlobalSkillsDir()
		sb.WriteString(fmt.Sprintf("\n  %s %s\n",
			lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
				Foreground(ColorDimText).Render("Global Dir:"),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(globalDir),
		))
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
				Foreground(ColorDimText).Render("Project Dir:"),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(".artemis/skills/"),
		))

	case sysSubMCP:
		cv.renderToggleField(sb, 0, "MCP", cv.cfg.MCP.Enabled, "Enabled", "Disabled")
		if cv.cfg.MCP.Enabled && len(cv.cfg.MCP.Servers) > 0 {
			sb.WriteString("\n")
			for _, srv := range cv.cfg.MCP.Servers {
				status := "disabled"
				if srv.Enabled {
					status = srv.Command
				}
				sb.WriteString(fmt.Sprintf("  %s %s\n",
					lipgloss.NewStyle().Width(22).Align(lipgloss.Right).Padding(0, 1).
						Foreground(ColorDimText).Render(srv.ID+":"),
					lipgloss.NewStyle().Foreground(ColorMuted).Render(status),
				))
			}
		} else if cv.cfg.MCP.Enabled {
			sb.WriteString(fmt.Sprintf("\n  %s\n",
				lipgloss.NewStyle().Foreground(ColorMuted).Render("No MCP servers configured. Edit config.json to add servers."),
			))
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
