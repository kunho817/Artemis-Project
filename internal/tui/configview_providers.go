package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/config"
)

// buildProviderInputs creates text inputs for provider tabs (Claude/Gemini/GPT/GLM/VLLM).
func (cv *ConfigView) buildProviderInputs() {
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

// applyProviderInputs writes provider input values back to config.
func (cv *ConfigView) applyProviderInputs() {
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
	} else if providerName == "vllm" {
		prov := cv.cfg.GetProvider(providerName)
		if prov == nil {
			return
		}
		prov.APIKey = cv.inputs[0].Value()
		prov.Endpoint = cv.inputs[1].Value()
		prov.Model = cv.inputs[2].Value()
		prov.Enabled = prov.Endpoint != "" // vLLM: API key optional, endpoint required
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
	if name == "vllm" {
		prov := cv.cfg.GetProvider(name)
		if prov == nil {
			return false
		}
		return prov.Enabled || prov.Endpoint != "" // vLLM: API key optional
	}
	prov := cv.cfg.GetProvider(name)
	if prov == nil {
		return false
	}
	return prov.Enabled || prov.APIKey != ""
}

// renderProviderContent draws the provider settings fields.
func (cv *ConfigView) renderProviderContent(sb *strings.Builder) {
	providerName := config.ProviderNames()[cv.tabIdx]
	fieldLabels := []string{"API Key", "Endpoint", "Model"}
	if providerName == "glm" {
		fieldLabels[1] = "Coding Plan Endpoint"
	} else if providerName == "vllm" {
		fieldLabels[0] = "API Key (optional)"
		fieldLabels[1] = "vLLM Server Endpoint"
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

// styledProviderName returns a provider name with appropriate coloring.
func (cv *ConfigView) styledProviderName(name string) string {
	hasKey := cv.isProviderEnabled(name)
	style := RoleProviderStyle
	if !hasKey {
		style = style.Foreground(ColorMuted).Strikethrough(true)
	}
	return style.Render(name)
}
