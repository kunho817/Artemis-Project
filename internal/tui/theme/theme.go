package theme

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

//go:embed presets/*.json
var presetFS embed.FS

// ThemeColors defines all color slots used throughout the TUI.
type ThemeColors struct {
	// Core palette
	Primary   string `json:"primary"`   // Main accent (violet by default)
	Secondary string `json:"secondary"` // Secondary accent (indigo)
	Accent    string `json:"accent"`    // Highlight / emphasis (cyan)

	// Status colors
	Success string `json:"success"` // green
	Warning string `json:"warning"` // amber
	Error   string `json:"error"`   // red

	// Text hierarchy
	Text    string `json:"text"`     // Primary text (light gray)
	DimText string `json:"dim_text"` // Secondary text (dim gray)
	Muted   string `json:"muted"`    // Disabled / tertiary text (gray)

	// Backgrounds
	Background string `json:"background"` // Main background
	Panel      string `json:"panel"`      // Panel / card background
	Surface    string `json:"surface"`    // Elevated surface (overlays, dialogs)

	// Structure
	Border string `json:"border"` // Borders and dividers

	// Agent role colors (multi-agent pipeline styling)
	AgentAnalysis string `json:"agent_analysis"` // Analyzer/Searcher/Explorer
	AgentPlan     string `json:"agent_plan"`     // Planner
	AgentCode     string `json:"agent_code"`     // Coder/Designer/Engineer/Architect
	AgentVerify   string `json:"agent_verify"`   // QA/Tester
}

// Theme is a named collection of colors for the TUI.
type Theme struct {
	Name   string      `json:"name"`
	Colors ThemeColors `json:"colors"`
}

// Color returns a lipgloss.Color for the given hex string.
func Color(hex string) lipgloss.Color {
	return lipgloss.Color(hex)
}

// Convenience accessors on ThemeColors — return lipgloss.Color.
func (c *ThemeColors) PrimaryColor() lipgloss.Color    { return Color(c.Primary) }
func (c *ThemeColors) SecondaryColor() lipgloss.Color  { return Color(c.Secondary) }
func (c *ThemeColors) AccentColor() lipgloss.Color     { return Color(c.Accent) }
func (c *ThemeColors) SuccessColor() lipgloss.Color    { return Color(c.Success) }
func (c *ThemeColors) WarningColor() lipgloss.Color    { return Color(c.Warning) }
func (c *ThemeColors) ErrorColor() lipgloss.Color      { return Color(c.Error) }
func (c *ThemeColors) TextColor() lipgloss.Color       { return Color(c.Text) }
func (c *ThemeColors) DimTextColor() lipgloss.Color    { return Color(c.DimText) }
func (c *ThemeColors) MutedColor() lipgloss.Color      { return Color(c.Muted) }
func (c *ThemeColors) BackgroundColor() lipgloss.Color { return Color(c.Background) }
func (c *ThemeColors) PanelColor() lipgloss.Color      { return Color(c.Panel) }
func (c *ThemeColors) SurfaceColor() lipgloss.Color    { return Color(c.Surface) }
func (c *ThemeColors) BorderColor() lipgloss.Color     { return Color(c.Border) }

// Styles holds all computed lipgloss styles derived from a Theme.
// Rebuilt whenever the theme changes.
type Styles struct {
	// Borders
	Border       lipgloss.Style
	ActiveBorder lipgloss.Style

	// Titles
	Title       lipgloss.Style
	ActiveTitle lipgloss.Style
	SubTitle    lipgloss.Style

	// Chat messages
	UserLabel lipgloss.Style
	BotLabel  lipgloss.Style
	Message   lipgloss.Style

	// Activity panel
	ActivityRunning lipgloss.Style
	ActivityDone    lipgloss.Style
	ActivityError   lipgloss.Style
	FileChanged     lipgloss.Style

	// Status bar
	StatusBar   lipgloss.Style
	StatusModel lipgloss.Style
	StatusToken lipgloss.Style
	StatusKey   lipgloss.Style
	StatusValue lipgloss.Style
	StatusTier  lipgloss.Style

	// Input
	InputPrompt lipgloss.Style
	InputText   lipgloss.Style

	// Divider
	Divider lipgloss.Style

	// Config view
	ToggleOn      lipgloss.Style
	ToggleOff     lipgloss.Style
	TierPremium   lipgloss.Style
	TierBudget    lipgloss.Style
	RoleLabel     lipgloss.Style
	RoleProvider  lipgloss.Style
	SectionHeader lipgloss.Style

	// Agent output
	AgentHeader   lipgloss.Style
	AgentAnalysis lipgloss.Style
	AgentPlan     lipgloss.Style
	AgentCode     lipgloss.Style
	AgentVerify   lipgloss.Style
	AgentDivider  lipgloss.Style
}

// BuildStyles creates a Styles struct from a Theme.
func BuildStyles(t *Theme) Styles {
	c := &t.Colors
	return Styles{
		// Borders
		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(c.BorderColor()),
		ActiveBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(c.PrimaryColor()),

		// Titles
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.TextColor()).
			Padding(0, 1),
		ActiveTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.PrimaryColor()).
			Padding(0, 1),
		SubTitle: lipgloss.NewStyle().
			Foreground(c.DimTextColor()).
			Padding(0, 1),

		// Chat messages
		UserLabel: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.AccentColor()),
		BotLabel: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.PrimaryColor()),
		Message: lipgloss.NewStyle().
			Foreground(c.TextColor()),

		// Activity panel
		ActivityRunning: lipgloss.NewStyle().
			Foreground(c.WarningColor()),
		ActivityDone: lipgloss.NewStyle().
			Foreground(c.SuccessColor()),
		ActivityError: lipgloss.NewStyle().
			Foreground(c.ErrorColor()),
		FileChanged: lipgloss.NewStyle().
			Foreground(c.AccentColor()),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(c.DimTextColor()).
			Padding(0, 1),
		StatusModel: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.PrimaryColor()).
			Padding(0, 1),
		StatusToken: lipgloss.NewStyle().
			Foreground(c.MutedColor()).
			Padding(0, 1),
		StatusKey: lipgloss.NewStyle().
			Foreground(c.DimTextColor()),
		StatusValue: lipgloss.NewStyle().
			Foreground(c.TextColor()),
		StatusTier: lipgloss.NewStyle().
			Padding(0, 1),

		// Input
		InputPrompt: lipgloss.NewStyle().
			Foreground(c.PrimaryColor()).
			Bold(true),
		InputText: lipgloss.NewStyle().
			Foreground(c.TextColor()),

		// Divider
		Divider: lipgloss.NewStyle().
			Foreground(c.BorderColor()),

		// Config view
		ToggleOn: lipgloss.NewStyle().
			Foreground(c.SuccessColor()).
			Bold(true),
		ToggleOff: lipgloss.NewStyle().
			Foreground(c.MutedColor()),
		TierPremium: lipgloss.NewStyle().
			Foreground(c.AccentColor()).
			Bold(true),
		TierBudget: lipgloss.NewStyle().
			Foreground(c.WarningColor()).
			Bold(true),
		RoleLabel: lipgloss.NewStyle().
			Foreground(c.DimTextColor()).
			Width(16),
		RoleProvider: lipgloss.NewStyle().
			Foreground(c.TextColor()),
		SectionHeader: lipgloss.NewStyle().
			Foreground(c.PrimaryColor()).
			Bold(true).
			Padding(0, 1),

		// Agent output
		AgentHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(c.SecondaryColor()),
		AgentAnalysis: lipgloss.NewStyle().
			Bold(true).
			Foreground(Color(c.AgentAnalysis)),
		AgentPlan: lipgloss.NewStyle().
			Bold(true).
			Foreground(Color(c.AgentPlan)),
		AgentCode: lipgloss.NewStyle().
			Bold(true).
			Foreground(Color(c.AgentCode)),
		AgentVerify: lipgloss.NewStyle().
			Bold(true).
			Foreground(Color(c.AgentVerify)),
		AgentDivider: lipgloss.NewStyle().
			Foreground(c.BorderColor()),
	}
}

// --- Global State ---

// Active is the currently loaded theme.
var Active Theme

// S holds the computed styles for the active theme.
var S Styles

func init() {
	Active = DefaultTheme()
	S = BuildStyles(&Active)
}

// Load loads a theme by name. Resolution order:
//  1. ~/.artemis/themes/{name}.json   (user override)
//  2. Embedded presets/{name}.json     (built-in)
//
// Falls back to "default" if the requested theme is not found.
func Load(name string) error {
	// 1. Try user override
	userPath, err := userThemePath(name)
	if err == nil {
		data, readErr := os.ReadFile(userPath)
		if readErr == nil {
			var t Theme
			if jsonErr := json.Unmarshal(data, &t); jsonErr == nil {
				Active = t
				S = BuildStyles(&Active)
				return nil
			}
		}
	}

	// 2. Try embedded preset
	data, err := presetFS.ReadFile("presets/" + name + ".json")
	if err == nil {
		var t Theme
		if jsonErr := json.Unmarshal(data, &t); jsonErr == nil {
			Active = t
			S = BuildStyles(&Active)
			return nil
		}
	}

	// 3. Fallback to hardcoded default
	if name != "default" {
		Active = DefaultTheme()
		S = BuildStyles(&Active)
	}
	return nil
}

// Reload rebuilds styles from the current Active theme.
// Call after programmatically modifying Active.Colors.
func Reload() {
	S = BuildStyles(&Active)
}

// AvailableThemes returns all theme names (built-in + user).
func AvailableThemes() []string {
	seen := make(map[string]bool)
	var names []string

	// Built-in presets
	entries, _ := presetFS.ReadDir("presets")
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			n := name[:len(name)-5]
			seen[n] = true
			names = append(names, n)
		}
	}

	// User themes
	dir, err := userThemeDir()
	if err == nil {
		userEntries, _ := os.ReadDir(dir)
		for _, e := range userEntries {
			name := e.Name()
			if filepath.Ext(name) == ".json" {
				n := name[:len(name)-5]
				if !seen[n] {
					names = append(names, n)
				}
			}
		}
	}

	return names
}

// DefaultTheme returns the default Artemis theme (current color scheme).
func DefaultTheme() Theme {
	return Theme{
		Name: "default",
		Colors: ThemeColors{
			Primary:    "#7C3AED",
			Secondary:  "#6366F1",
			Accent:     "#22D3EE",
			Success:    "#22C55E",
			Warning:    "#F59E0B",
			Error:      "#EF4444",
			Text:       "#E5E7EB",
			DimText:    "#9CA3AF",
			Muted:      "#6B7280",
			Background: "#111827",
			Panel:      "#1F2937",
			Surface:    "#374151",
			Border:     "#4B5563",

			AgentAnalysis: "#818CF8",
			AgentPlan:     "#34D399",
			AgentCode:     "#F472B6",
			AgentVerify:   "#FBBF24",
		},
	}
}

// --- Paths ---

func userThemeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".artemis", "themes"), nil
}

func userThemePath(name string) (string, error) {
	dir, err := userThemeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}
