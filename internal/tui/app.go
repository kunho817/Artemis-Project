package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/config"
	ghub "github.com/artemis-project/artemis/internal/github"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

const (
	// Panel width ratio: chat gets ~65%, activity gets ~35%
	chatWidthRatio   = 0.65
	minChatWidth     = 40
	minActivityWidth = 20
	statusBarHeight  = 1
	borderOverhead   = 2 // top + bottom borders
	titleBarHeight   = 1
	eventBusBuffer   = 64
)

// FocusedPanel tracks which panel has focus.
type FocusedPanel int

const (
	FocusChat FocusedPanel = iota
	FocusActivity
)

// ViewMode tracks which view is active.
type ViewMode int

const (
	ViewChat ViewMode = iota
	ViewConfig
)

// LayoutMode tracks whether UI is single-panel or split-panel.
type LayoutMode int

const (
	LayoutSingle LayoutMode = iota
	LayoutSplit
)

// agentStreamInfo tracks streaming state for a single agent.
type agentStreamInfo struct {
	name     string
	role     string
	content  string
	msgIndex int // index in chat.messages
}

// App is the main application model.
type App struct {
	chat      ChatPanel
	activity  ActivityPanel
	statusBar StatusBar
	input     textarea.Model

	// View management
	viewMode   ViewMode
	configView ConfigView

	// LLM
	cfg      config.Config
	provider llm.Provider

	// Pipeline (multi-agent mode)
	eventBus        *bus.EventBus
	cancelPipeline  context.CancelFunc
	pipelineRunning bool

	// Tool executor
	toolExecutor *tools.ToolExecutor
	skillRegistry *agent.SkillRegistry // Phase 4: skill resolution

	// Persistent memory
	memStore     memory.MemoryStore
	vectorStore  memory.VectorSearcher // Phase 2: vector search
	consolidator *memory.Consolidator
	repoMapStore *memory.RepoMapStore // Phase 3: repo-map
	ghSyncer     *ghub.Syncer
	ghProcessor  *ghub.Processor
	checkpointStore state.CheckpointStore // Phase C-5: step checkpoint persistence
	pendingResumeRun *state.IncompleteRun  // Phase C-5: deferred resume overlay (shown after Init)
	sessionID          string // unique ID for current session
	parentSessionID    string // Phase 5: parent session (set when /load creates child)
	activePipelineRunID string // Phase 5: current pipeline run ID (for message linking)
	focused FocusedPanel

	layoutMode LayoutMode
	width      int
	height     int
	ready      bool

	// Conversation history
	history          []llm.Message               // multi-turn conversation (user + assistant)
	historyWindow    *agent.HistoryWindow         // Phase C: token-aware history management
	streamingContent string                      // accumulates streaming response for history (single mode)
	pipelineOutputs  []string                    // accumulates agent output during pipeline run
	streamCh         <-chan llm.StreamChunk      // active stream channel (single mode)
	agentStreams     map[string]*agentStreamInfo // per-agent streaming state (multi-agent mode)

	// Cost tracking
	totalTokens int
	totalCost   float64

	overlayKind OverlayKind
	overlay     Overlay

	// Recovery system (Phase 6)
	recoveryBridge *RecoveryBridge  // shared pointer — survives model copies
	recoveryQueue  []RecoveryRequest // queued recovery requests from concurrent agent failures
}

// NewApp creates a new application model.
func NewApp() App {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Ctrl+Enter to send)"
	ta.Focus()
	ta.CharLimit = 4096
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("enter")
	ta.Prompt = "│ "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorAccent)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(ColorDimText)

	// Load config
	cfg, _ := config.Load()

	// Load theme
	theme.Load(cfg.Theme)
	RefreshStyles()

	// Initialize tool executor
	cwd, _ := os.Getwd()
	te := tools.NewToolExecutor(cwd)
	te.SetAutoCommit(true)

	// Initialize local code generation provider (vLLM) if configured
	if cfg.VLLM.Enabled && cfg.VLLM.Endpoint != "" {
		vllmProvider := llm.NewVLLM(cfg.VLLM)
		te.SetCodeGenProvider(vllmProvider)
	}

	// Initialize skill registry
	skillReg := agent.NewSkillRegistry()
	app := App{
		chat:          NewChatPanel(),
		activity:      NewActivityPanel(),
		statusBar:     NewStatusBar(),
		input:         ta,
		focused:       FocusChat,
		viewMode:      ViewChat,
		layoutMode:    LayoutSingle,
		cfg:           cfg,
		toolExecutor:  te,
		skillRegistry: skillReg,
		sessionID:     fmt.Sprintf("ses_%d", time.Now().UnixNano()),
		totalTokens:   0,
		totalCost:     0,
	}
	// Initialize token-aware history window
	tc, _ := llm.GetTokenCounter()
	app.historyWindow = agent.NewHistoryWindow(10, tc) // keep last 10 messages in full
	app.statusBar.SetKeyHints([]KeyHint{
		{Key: "^↵", Desc: "Send"},
		{Key: "^K", Desc: "Palette"},
		{Key: "^A", Desc: "Agents"},
		{Key: "^O", Desc: "Files"},
		{Key: "^S", Desc: "Settings"},
		{Key: "^L", Desc: "Clear"},
		{Key: "^C", Desc: "Quit"},
	})

	// Initialize LLM provider (single-provider fallback)
	app.initProvider()

	// Initialize persistent memory
	app.initMemory()
	// Determine welcome message based on mode and provider status
	mode := "single-provider"
	if cfg.Agents.Enabled {
		mode = "multi-agent orchestrator"
	}
	if app.provider != nil {
		app.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Welcome to Artemis (%s mode). Type a message to begin.", mode),
		})
	} else {
		app.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "⚠ No LLM provider configured. Press [Ctrl+S] to set up API keys.",
		})
	}

	return app
}

// initProvider creates the LLM provider from config and updates status bar.
func (a *App) initProvider() {
	raw := llm.NewProvider(a.cfg.ActiveProvider, &a.cfg)
	if raw != nil {
		a.provider = llm.NewRetryProvider(raw, 2)
		a.statusBar.SetModel(cases.Title(language.English).String(raw.Name()))
	} else {
		a.provider = nil
		a.statusBar.SetModel("No Provider")
	}

	// Update mode/tier in status bar
	if a.cfg.Agents.Enabled {
		a.statusBar.SetMode("multi")
	} else {
		a.statusBar.SetMode("single")
	}
	a.statusBar.SetTier(a.cfg.Agents.Tier)
	a.activity.SetSessionInfo(a.sessionID, a.statusBar.model)
	if a.cfg.Agents.Enabled {
		a.activity.SetAgentCount(1)
	} else {
		a.activity.SetAgentCount(0)
	}
}

// Init implements tea.Model.
func (a App) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle config view if active
	if a.viewMode == ViewConfig {
		return a.updateConfigView(msg)
	}

	// Overlay intercept — overlay captures all input when active
	if a.overlayKind != OverlayNone && a.overlay != nil {
		closed, result, cmd := a.overlay.Update(msg)
		if closed {
			a.handleOverlayResult(result)
			a.overlayKind = OverlayNone
			a.overlay = nil
		}
		return a, cmd
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Close overlay on Esc if active (highest priority)
		if msg.String() == "esc" && a.overlayKind != OverlayNone {
			a.overlayKind = OverlayNone
			a.overlay = nil
			return a, nil
		}

		switch msg.String() {
		case "ctrl+k":
			if a.overlayKind == OverlayNone {
				cp := NewCommandPalette(a.width, a.height)
				a.overlayKind = OverlayCommandPalette
				a.overlay = cp
				return a, cp.Init()
			}
			return a, nil
		case "ctrl+a":
			if a.overlayKind == OverlayNone {
				as := NewAgentSelector(a.cfg, a.width, a.height)
				a.overlayKind = OverlayAgentSelector
				a.overlay = as
			}
			return a, nil
		case "ctrl+o":
			if a.overlayKind == OverlayNone {
				cwd, _ := os.Getwd()
				fp := NewFilePicker(cwd, a.width, a.height)
				a.overlayKind = OverlayFilePicker
				a.overlay = fp
			}
			return a, nil
		case "ctrl+c":
			if a.cancelPipeline != nil {
				a.cancelPipeline()
			}
			a.shutdownMemory()
			return a, tea.Quit
		case "ctrl+s":
			a.viewMode = ViewConfig
			a.configView = NewConfigView(a.cfg)
			a.configView.SetSize(a.width, a.height)
			return a, nil
		case "ctrl+l":
			a.clearChatState()
			a.overlayKind = OverlayNone
			a.overlay = nil
			return a, nil
		case "tab":
			if a.layoutMode == LayoutSingle {
				return a, nil
			}
			a.cycleFocus()
			return a, nil
		case "ctrl+enter":
			return a.handleSubmit()
		case "up", "down", "pgup", "pgdown":
			// Forward scroll keys to chat viewport
			scrollCmd := a.chat.Update(msg)
			if scrollCmd != nil {
				cmds = append(cmds, scrollCmd)
			}
			return a, tea.Batch(cmds...)
		}

	case tea.MouseMsg:
		// Forward all mouse events to chat viewport (scroll wheel)
		scrollCmd := a.chat.Update(msg)
		if scrollCmd != nil {
			cmds = append(cmds, scrollCmd)
		}
		return a, tea.Batch(cmds...)

	case LLMResponseMsg:
		return a.handleLLMResponse(msg)

	case streamStartMsg:
		return a.handleStreamStart(msg)

	case StreamChunkMsg:
		return a.handleStreamChunk(msg)

	case AgentEventMsg:
		return a.handleAgentEvent(msg)

	case PipelineCompleteMsg:
		return a.handlePipelineComplete(msg)

	case RecoveryRequestMsg:
		// Queue the recovery request from Engine goroutine
		a.recoveryQueue = append(a.recoveryQueue, msg.Request)
		// Show overlay if this is the first queued request
		if len(a.recoveryQueue) == 1 {
			overlay := NewRecoveryOverlay(msg.Request, a.width, a.height)
			a.overlayKind = OverlayRecovery
			a.overlay = overlay
		}
		// Re-subscribe ONLY to recovery requests.
		// waitForEvent is still running from pipeline.go's initial tea.Batch;
		// re-subscribing would create duplicate goroutines on the same EventBus channel,
		// causing double PipelineCompleteMsg on close.
		if a.recoveryBridge != nil {
			return a, waitForRecoveryRequest(a.recoveryBridge)
		}
		return a, nil

	case RecoveryDecisionMsg:
		// Send user's decision back to the Engine goroutine
		if len(a.recoveryQueue) > 0 {
			req := a.recoveryQueue[0]
			req.ReplyCh <- msg.Action
			a.recoveryQueue = a.recoveryQueue[1:]
		}
		// Show next queued recovery request, or dismiss overlay
		if len(a.recoveryQueue) > 0 {
			overlay := NewRecoveryOverlay(a.recoveryQueue[0], a.width, a.height)
			a.overlayKind = OverlayRecovery
			a.overlay = overlay
		} else {
			a.overlayKind = OverlayNone
			a.overlay = nil
		}
		return a, nil

	case ResumeDecisionMsg:
		// Phase C-5: Handle user's decision on incomplete pipeline run
		a.overlayKind = OverlayNone
		a.overlay = nil
		switch msg.Action {
		case "resume":
			return a.executeResume(msg.Run)
		case "discard":
			run := msg.Run
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if a.checkpointStore != nil {
					_ = a.checkpointStore.DeleteCheckpoints(ctx, run.RunID)
				}
				if a.memStore != nil {
					_ = a.memStore.UpdatePipelineRun(ctx, run.RunID, "failed")
				}
			}()
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "Incomplete pipeline run discarded."})
		case "cancel":
			// Do nothing — ignore for now
		}
		return a, nil
	case fixIssueResultMsg:
		if msg.err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Fix failed for issue #%d: %v", msg.issueNumber, msg.err)})
		} else {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Fix pipeline scaffold completed for issue #%d.", msg.issueNumber)})
		}
		return a, nil

	case OrchestratorPlanMsg:
		return a.handleOrchestratorPlan(msg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		wasReady := a.ready
		a.ready = true
		a.recalcLayout()
		if a.overlay != nil {
			a.overlay.SetSize(a.width, a.height)
		}
		// Phase C-5: Show deferred resume overlay on first ready
		if !wasReady && a.pendingResumeRun != nil {
			run := a.pendingResumeRun
			a.pendingResumeRun = nil
			overlay := NewResumeOverlay(*run, a.width, a.height)
			a.overlayKind = OverlayResume
			a.overlay = overlay
		}
		return a, nil
	}

	// Forward to textarea input
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Auto-expand textarea height (1 to 8 lines)
	lineCount := a.input.LineCount()
	if lineCount < 1 {
		lineCount = 1
	}
	if lineCount > 8 {
		lineCount = 8
	}
	a.input.SetHeight(lineCount)
	a.recalcLayout()

	return a, tea.Batch(cmds...)
}

// updateConfigView handles updates while in config view.
func (a App) updateConfigView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.configView.SetSize(a.width, a.height)
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	done, cmd := a.configView.Update(msg)
	if done {
		// Return to chat view with updated config
		a.cfg = a.configView.GetConfig()
		a.viewMode = ViewChat
		a.initProvider()
		theme.Load(a.cfg.Theme)
		RefreshStyles()
		a.input.Focus()
		a.recalcLayout()

		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Settings saved.",
		})

		return a, textarea.Blink
	}

	return a, cmd
}

// handleSubmit processes user input submission.
func (a App) handleSubmit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(a.input.Value())
	if text == "" {
		return a, nil
	}

	// Don't allow new submissions while pipeline is running
	if a.pipelineRunning {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Pipeline is still running. Please wait or press Ctrl+L to cancel.",
		})
		return a, nil
	}

	// Handle slash commands before treating as LLM message
	if strings.HasPrefix(text, "/") {
		a.input.Reset()
		a.input.SetHeight(1)
		a.recalcLayout()
		return a.handleCommand(text)
	}

	// Add user message
	a.chat.AddMessage(ChatMessage{
		Role:    RoleUser,
		Content: text,
	})

	// Append to conversation history for multi-turn context
	a.addToHistory(llm.Message{Role: "user", Content: text})
	a.saveMessageToDB("user", text, "")

	// Clear input
	a.input.Reset()
	a.input.SetHeight(1)
	a.recalcLayout()

	// Route to orchestrator (multi-agent) or single-provider mode
	if a.cfg.Agents.Enabled {
		return a.handleOrchestratedSubmit(text)
	}
	return a.handleSingleSubmit(text)
}

// addToHistory appends a message to both legacy history and HistoryWindow.
func (a *App) addToHistory(msg llm.Message) {
	a.history = append(a.history, msg)
	if a.historyWindow != nil {
		a.historyWindow.Add(msg)
	}
}

// cycleFocus switches between panels.
func (a *App) cycleFocus() {
	switch a.focused {
	case FocusChat:
		a.focused = FocusActivity
	case FocusActivity:
		a.focused = FocusChat
	}

	a.chat.SetFocused(a.focused == FocusChat)
	a.activity.SetFocused(a.focused == FocusActivity)
}

// recalcLayout recalculates all component sizes.
func (a *App) recalcLayout() {
	if a.width == 0 || a.height == 0 {
		return
	}

	// Available space inside outer border
	innerWidth := a.width - borderOverhead
	currentInputHeight := a.input.Height() + 1 // +1 for input line padding/border
	innerHeight := a.height - borderOverhead - statusBarHeight - currentInputHeight - titleBarHeight

	chatWidth := innerWidth
	activityWidth := 0
	if a.layoutMode == LayoutSplit {
		// Split horizontally
		chatWidth = int(float64(innerWidth) * chatWidthRatio)
		if chatWidth < minChatWidth {
			chatWidth = minChatWidth
		}
		activityWidth = innerWidth - chatWidth - 1 // -1 for vertical divider
		if activityWidth < minActivityWidth {
			activityWidth = minActivityWidth
			chatWidth = innerWidth - activityWidth - 1
		}
	}

	a.chat.SetSize(chatWidth, innerHeight)
	a.activity.SetSize(activityWidth, innerHeight)
	a.statusBar.SetSize(a.width)
	a.input.SetWidth(chatWidth - 4) // account for prompt and padding
}

// View implements tea.Model.
func (a App) View() string {
	if !a.ready {
		return "Initializing Artemis..."
	}

	// Config view takes over the entire screen
	if a.viewMode == ViewConfig {
		return a.configView.View()
	}

	// Title bar
	titleLeft := TitleStyle.Render("Artemis v0.1.0")
	titleRight := ActiveTitleStyle.Render(a.statusBar.model)

	innerWidth := a.width - borderOverhead
	titleGap := innerWidth - lipgloss.Width(titleLeft) - lipgloss.Width(titleRight)
	if titleGap < 0 {
		titleGap = 0
	}
	titleBar := titleLeft + lipgloss.NewStyle().Width(titleGap).Render("") + titleRight

	// Main panels
	chatBorder := BorderStyle
	actBorder := BorderStyle
	if a.layoutMode == LayoutSplit && a.focused == FocusActivity {
		actBorder = ActiveBorderStyle
	} else {
		chatBorder = ActiveBorderStyle
	}

	// Calculate inner heights
	currentInputHeight := a.input.Height() + 1 // +1 for input line padding/border
	innerHeight := a.height - borderOverhead - statusBarHeight - currentInputHeight - titleBarHeight

	chatWidth := innerWidth
	if a.layoutMode == LayoutSplit {
		chatWidth = int(float64(innerWidth) * chatWidthRatio)
		if chatWidth < minChatWidth {
			chatWidth = minChatWidth
		}
	}

	chatView := chatBorder.
		Width(chatWidth).
		Height(innerHeight).
		Render(a.chat.View())

	panels := chatView
	if a.layoutMode == LayoutSplit {
		actWidth := innerWidth - lipgloss.Width(chatView)
		actView := actBorder.
			Width(actWidth).
			Height(innerHeight).
			Render(a.activity.View())
		panels = lipgloss.JoinHorizontal(lipgloss.Top, chatView, actView)
	}

	// Input line
	inputLine := lipgloss.NewStyle().
		Padding(0, 1).
		Render(a.input.View())

	// Status bar
	statusView := a.statusBar.View()

	// Compose full layout
	composed := lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		panels,
		inputLine,
		statusView,
	)

	if a.overlayKind != OverlayNone && a.overlay != nil {
		return PlaceOverlayCentered(a.overlay.View(), composed, a.width, a.height)
	}

	return composed
}

func (a *App) clearChatState() {
	if a.cancelPipeline != nil {
		a.cancelPipeline()
		a.cancelPipeline = nil
		a.pipelineRunning = false
	}
	a.chat = NewChatPanel()
	a.activity.ClearActivities()
	a.history = nil
	if a.historyWindow != nil {
		a.historyWindow.Clear()
	}
	a.pipelineOutputs = nil
	a.agentStreams = nil
	a.recoveryBridge = nil
	a.recoveryQueue = nil
	a.totalTokens = 0
	a.totalCost = 0
	a.statusBar.SetTokens(0)
	a.statusBar.SetCost(0)
	a.focused = FocusChat
	a.layoutMode = LayoutSingle
	a.recalcLayout()
}

func (a *App) addUsage(usage *llm.TokenUsage, model string) {
	if usage == nil {
		return
	}
	a.totalTokens += usage.TotalTokens
	pricing := llm.GetPricing(model)
	a.totalCost += llm.CalculateCost(usage, pricing)
	a.statusBar.SetTokens(a.totalTokens)
	a.statusBar.SetCost(a.totalCost)
}

func (a *App) handleOverlayResult(result OverlayResult) {
	switch result.Action {
	case "command":
		if strings.TrimSpace(result.Value) == "" {
			return
		}
		m, cmd := a.handleCommand(result.Value)
		if next, ok := m.(App); ok {
			*a = next
		}
		if cmd != nil {
			// No async command currently used by slash commands; intentionally ignored here.
		}

	case "clear":
		a.clearChatState()

	case "settings":
		a.viewMode = ViewConfig
		a.configView = NewConfigView(a.cfg)
		a.configView.SetSize(a.width, a.height)

	case "toggle_agents":
		a.cfg.Agents.Enabled = !a.cfg.Agents.Enabled
		if err := config.Save(a.cfg); err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
			return
		}
		a.initProvider()

	case "switch_tier":
		if a.cfg.Agents.Tier == "premium" {
			a.cfg.Agents.Tier = "budget"
		} else {
			a.cfg.Agents.Tier = "premium"
		}
		if err := config.Save(a.cfg); err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
			return
		}
		a.initProvider()

	case "switch_theme":
		themes := theme.AvailableThemes()
		if len(themes) == 0 {
			return
		}
		current := a.cfg.Theme
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
		a.cfg.Theme = themes[(idx+1)%len(themes)]
		if err := config.Save(a.cfg); err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
			return
		}
		_ = theme.Load(a.cfg.Theme)
		RefreshStyles()
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Theme switched to %s.", a.cfg.Theme)})

	case "export_theme":
		currentName := a.cfg.Theme
		if currentName == "" {
			currentName = "default"
		}
		path, err := theme.ExportTheme(currentName)
		if err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to export theme: %v", err)})
			return
		}
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Theme exported to %s — edit and restart to apply.", path)})

	case "agents_changed":
		if sel, ok := a.overlay.(*AgentSelector); ok {
			a.cfg = sel.Config()
		}
		if err := config.Save(a.cfg); err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
			return
		}
		a.initProvider()

	case "select_file":
		path := strings.TrimSpace(result.Value)
		if path == "" {
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to read file: %v", err)})
			return
		}
		content := string(b)
		if len(content) > 8000 {
			content = content[:8000] + "\n... (truncated)"
		}
		ctx := fmt.Sprintf("[File Context: %s]\n```\n%s\n```", filepath.ToSlash(path), content)
		existing := strings.TrimSpace(a.input.Value())
		if existing != "" {
			a.input.SetValue(ctx + "\n\n" + existing)
		} else {
			a.input.SetValue(ctx)
		}
		lineCount := a.input.LineCount()
		if lineCount < 1 {
			lineCount = 1
		}
		if lineCount > 8 {
			lineCount = 8
		}
		a.input.SetHeight(lineCount)
		a.recalcLayout()
	}
}
