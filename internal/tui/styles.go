package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/tui/theme"
)

// Color palette — delegated to active theme.
// These are kept as vars for backward compatibility with existing code.
// After Phase 1, callers will migrate to theme.S.* direct access.
var (
	ColorPrimary   lipgloss.Color
	ColorSecondary lipgloss.Color
	ColorAccent    lipgloss.Color
	ColorSuccess   lipgloss.Color
	ColorWarning   lipgloss.Color
	ColorError     lipgloss.Color
	ColorMuted     lipgloss.Color
	ColorText      lipgloss.Color
	ColorDimText   lipgloss.Color
	ColorBorder    lipgloss.Color
	ColorBg        lipgloss.Color
	ColorPanelBg   lipgloss.Color
)

// Border styles
var (
	BorderStyle       lipgloss.Style
	ActiveBorderStyle lipgloss.Style
)

// Panel title styles
var (
	TitleStyle       lipgloss.Style
	ActiveTitleStyle lipgloss.Style
	SubTitleStyle    lipgloss.Style
)

// Chat message styles
var (
	UserLabelStyle lipgloss.Style
	BotLabelStyle  lipgloss.Style
	MessageStyle   lipgloss.Style
)

// Activity styles
var (
	ActivityRunningStyle lipgloss.Style
	ActivityDoneStyle    lipgloss.Style
	ActivityErrorStyle   lipgloss.Style
	FileChangedStyle     lipgloss.Style
)

// Status bar styles
var (
	StatusBarStyle   lipgloss.Style
	StatusModelStyle lipgloss.Style
	StatusTokenStyle lipgloss.Style
	StatusKeyStyle   lipgloss.Style
	StatusValueStyle lipgloss.Style
)

// Input styles
var (
	InputPromptStyle lipgloss.Style
	InputTextStyle   lipgloss.Style
)

// Divider
var DividerStyle lipgloss.Style

// Config toggle styles
var (
	ToggleOnStyle      lipgloss.Style
	ToggleOffStyle     lipgloss.Style
	TierPremiumStyle   lipgloss.Style
	TierBudgetStyle    lipgloss.Style
	RoleLabelStyle     lipgloss.Style
	RoleProviderStyle  lipgloss.Style
	SectionHeaderStyle lipgloss.Style
	StatusTierStyle    lipgloss.Style
)

// Agent output styles
var (
	AgentHeaderStyle   lipgloss.Style
	AgentAnalysisStyle lipgloss.Style
	AgentPlanStyle     lipgloss.Style
	AgentCodeStyle     lipgloss.Style
	AgentVerifyStyle   lipgloss.Style
	AgentDividerStyle  lipgloss.Style
)

func init() {
	RefreshStyles()
}

// RefreshStyles rebuilds all style variables from the active theme.
// Called at init and after any theme change (theme.Load).
func RefreshStyles() {
	s := theme.S
	c := &theme.Active.Colors

	// Colors
	ColorPrimary = c.PrimaryColor()
	ColorSecondary = c.SecondaryColor()
	ColorAccent = c.AccentColor()
	ColorSuccess = c.SuccessColor()
	ColorWarning = c.WarningColor()
	ColorError = c.ErrorColor()
	ColorMuted = c.MutedColor()
	ColorText = c.TextColor()
	ColorDimText = c.DimTextColor()
	ColorBorder = c.BorderColor()
	ColorBg = c.BackgroundColor()
	ColorPanelBg = c.PanelColor()

	// Borders
	BorderStyle = s.Border
	ActiveBorderStyle = s.ActiveBorder

	// Titles
	TitleStyle = s.Title
	ActiveTitleStyle = s.ActiveTitle
	SubTitleStyle = s.SubTitle

	// Chat messages
	UserLabelStyle = s.UserLabel
	BotLabelStyle = s.BotLabel
	MessageStyle = s.Message

	// Activity
	ActivityRunningStyle = s.ActivityRunning
	ActivityDoneStyle = s.ActivityDone
	ActivityErrorStyle = s.ActivityError
	FileChangedStyle = s.FileChanged

	// Status bar
	StatusBarStyle = s.StatusBar
	StatusModelStyle = s.StatusModel
	StatusTokenStyle = s.StatusToken
	StatusKeyStyle = s.StatusKey
	StatusValueStyle = s.StatusValue

	// Input
	InputPromptStyle = s.InputPrompt
	InputTextStyle = s.InputText

	// Divider
	DividerStyle = s.Divider

	// Config toggles
	ToggleOnStyle = s.ToggleOn
	ToggleOffStyle = s.ToggleOff
	TierPremiumStyle = s.TierPremium
	TierBudgetStyle = s.TierBudget
	RoleLabelStyle = s.RoleLabel
	RoleProviderStyle = s.RoleProvider
	SectionHeaderStyle = s.SectionHeader
	StatusTierStyle = s.StatusTier

	// Agent output
	AgentHeaderStyle = s.AgentHeader
	AgentAnalysisStyle = s.AgentAnalysis
	AgentPlanStyle = s.AgentPlan
	AgentCodeStyle = s.AgentCode
	AgentVerifyStyle = s.AgentVerify
	AgentDividerStyle = s.AgentDivider
}
