package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
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
	"github.com/artemis-project/artemis/internal/lsp"
	"github.com/artemis-project/artemis/internal/mcp"
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
	minTermWidth     = 80
	minTermHeight    = 24
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

type layoutState struct {
	innerWidth    int
	innerHeight   int
	chatWidth     int
	activityWidth int
	inputHeight   int
}

// agentStreamInfo tracks streaming state for a single agent.
type agentStreamInfo struct {
	name     string
	role     string
	content  string
	msgIndex int // index in chat.messages
}

// CostState tracks token and cost totals (value-safe, copied each Update).
type CostState struct {
	TotalTokens int
	TotalCost   float64
}

// SessionState tracks session identity (value-safe).
type SessionState struct {
	ID            string
	ParentID      string
	PipelineRunID string
}

// HistoryState tracks conversation history (value-safe — slices are reference types).
type HistoryState struct {
	Messages         []llm.Message
	Window           *agent.HistoryWindow // pointer, safe
	StreamingContent string
	PipelineOutputs  []string
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
	pipelineWg      *sync.WaitGroup // shared pointer — survives model copies; signals pipeline goroutine exit

	// Tool executor
	toolExecutor  *tools.ToolExecutor
	skillRegistry *agent.SkillRegistry // Phase 4: skill resolution

	// Persistent memory
	memStore         memory.MemoryStore
	vectorStore      memory.VectorSearcher // Phase 2: vector search
	consolidator     *memory.Consolidator
	repoMapStore     *memory.RepoMapStore // Phase 3: repo-map
	projectRules     string               // ARTEMIS.md project rules
	codeIndex        *memory.CodeIndex    // Semantic code search
	lspManager       *lsp.Manager         // Phase D: LSP Control Plane
	mcpManager       *mcp.Manager
	ghSyncer         *ghub.Syncer
	ghProcessor      *ghub.Processor
	checkpointStore  state.CheckpointStore // Phase C-5: step checkpoint persistence
	pendingResumeRun *state.IncompleteRun  // Phase C-5: deferred resume overlay (shown after Init)
	session          SessionState
	focused          FocusedPanel

	layoutMode LayoutMode
	layout     layoutState
	width      int
	height     int
	ready      bool

	// Conversation history and cost tracking
	hist         HistoryState                // multi-turn conversation state + streaming accumulators
	streamCh     <-chan llm.StreamChunk      // active stream channel (single mode)
	agentStreams map[string]*agentStreamInfo // per-agent streaming state (multi-agent mode)
	cost         CostState

	overlayKind OverlayKind
	overlay     Overlay

	// Recovery system (Phase 6)
	recoveryBridge *RecoveryBridge   // shared pointer — survives model copies
	recoveryQueue  []RecoveryRequest // queued recovery requests from concurrent agent failures
}

func computeLayout(width, height, inputHeight int, mode LayoutMode) layoutState {
	innerWidth := width - borderOverhead
	currentInputHeight := inputHeight + 1
	innerHeight := height - borderOverhead - statusBarHeight - currentInputHeight - titleBarHeight

	chatWidth := innerWidth
	activityWidth := 0
	if mode == LayoutSplit {
		chatWidth = int(float64(innerWidth) * chatWidthRatio)
		if chatWidth < minChatWidth {
			chatWidth = minChatWidth
		}
		activityWidth = innerWidth - chatWidth - 1
		if activityWidth < minActivityWidth {
			activityWidth = minActivityWidth
			chatWidth = innerWidth - activityWidth - 1
		}
	}

	return layoutState{
		innerWidth:    innerWidth,
		innerHeight:   innerHeight,
		chatWidth:     chatWidth,
		activityWidth: activityWidth,
		inputHeight:   currentInputHeight,
	}
}

// NewApp creates a new application model.
func NewApp() App {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send, Shift+Enter for newline)"
	ta.Focus()
	ta.CharLimit = 4096
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter") // Shift+Enter = newline
	ta.Prompt = "│ "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorAccent)
	ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(ColorDimText)

	// Load config (fallback to defaults on error)
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Load theme
	theme.Load(cfg.Theme)
	RefreshStyles()

	// Initialize tool executor
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
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
		session: SessionState{
			ID: fmt.Sprintf("ses_%d", time.Now().UnixNano()),
		},
	}
	// Initialize token-aware history window
	tc, _ := llm.GetTokenCounter()
	app.hist.Window = agent.NewHistoryWindow(10, tc) // keep last 10 messages in full
	app.statusBar.SetKeyHints(DefaultKeyHints())

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
	a.activity.SetSessionInfo(a.session.ID, a.statusBar.model)
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
			a.syncKeyHints()
		}
		return a, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKeyMsg(msg)

	case tea.MouseMsg:
		return a.handleMouseMsg(msg)

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
			a.syncKeyHints()
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
			a.syncKeyHints()
		} else {
			a.overlayKind = OverlayNone
			a.overlay = nil
			a.syncKeyHints()
		}
		return a, nil

	case ResumeDecisionMsg:
		// Phase C-5: Handle user's decision on incomplete pipeline run
		a.overlayKind = OverlayNone
		a.overlay = nil
		a.syncKeyHints()
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

	case DiffViewMsg:
		overlay := NewDiffOverlay(msg.FileName, msg.Diff, a.width, a.height)
		a.overlay = overlay
		a.overlayKind = OverlayDiff
		a.syncKeyHints()
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
			a.syncKeyHints()
		}
		return a, nil
	}

	return a.handleInputUpdate(msg)
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
		a.syncKeyHints()
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
	a.hist.Messages = append(a.hist.Messages, msg)
	if a.hist.Window != nil {
		a.hist.Window.Add(msg)
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

	a.layout = computeLayout(a.width, a.height, a.input.Height(), a.layoutMode)
	a.chat.SetSize(a.layout.chatWidth, a.layout.innerHeight)
	a.activity.SetSize(a.layout.activityWidth, a.layout.innerHeight)
	a.statusBar.SetSize(a.width)
	a.input.SetWidth(a.layout.chatWidth - 4) // account for prompt and padding
}

// View implements tea.Model.
func (a App) View() string {
	if !a.ready {
		return "Initializing Artemis..."
	}

	if a.width < minTermWidth || a.height < minTermHeight {
		return lipgloss.NewStyle().
			Foreground(ColorWarning).
			Render(fmt.Sprintf(
				"Terminal too small (%d×%d). Minimum: %d×%d.\nResize your terminal to continue.",
				a.width, a.height, minTermWidth, minTermHeight))
	}

	// Config view takes over the entire screen
	if a.viewMode == ViewConfig {
		return a.configView.View()
	}

	// Title bar
	titleLeft := TitleStyle.Render("Artemis v0.1.0")
	titleRight := ActiveTitleStyle.Render(a.statusBar.model)

	titleGap := a.layout.innerWidth - lipgloss.Width(titleLeft) - lipgloss.Width(titleRight)
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

	chatView := chatBorder.
		Width(a.layout.chatWidth).
		Height(a.layout.innerHeight).
		Render(a.chat.View())

	panels := chatView
	if a.layoutMode == LayoutSplit {
		actView := actBorder.
			Width(a.layout.activityWidth).
			Height(a.layout.innerHeight).
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
	a.pipelineWg = nil
	a.chat = NewChatPanel()
	a.activity.ClearActivities()
	a.hist.Messages = nil
	if a.hist.Window != nil {
		a.hist.Window.Clear()
	}
	a.hist.StreamingContent = ""
	a.hist.PipelineOutputs = nil
	a.agentStreams = nil
	a.recoveryBridge = nil
	a.recoveryQueue = nil
	a.cost.TotalTokens = 0
	a.cost.TotalCost = 0
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
	a.cost.TotalTokens += usage.TotalTokens
	pricing := llm.GetPricing(model)
	a.cost.TotalCost += llm.CalculateCost(usage, pricing)
	a.statusBar.SetTokens(a.cost.TotalTokens)
	a.statusBar.SetCost(a.cost.TotalCost)
}

func (a *App) shutdown() {
	// 1. Cancel pipeline
	if a.cancelPipeline != nil {
		a.cancelPipeline()
	}
	// 2. Wait for pipeline goroutine
	if a.pipelineWg != nil {
		done := make(chan struct{})
		go func() { a.pipelineWg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	// 3. Shutdown LSP
	if a.lspManager != nil {
		a.lspManager.Shutdown()
	}
	// 4. Shutdown MCP
	if a.mcpManager != nil {
		a.mcpManager.Shutdown()
	}
	// 5. Shutdown memory (consolidate + close)
	a.shutdownMemory()
}

func (a *App) syncKeyHints() {
	if a.viewMode == ViewConfig {
		a.statusBar.SetKeyHints(ConfigKeyHints())
		return
	}
	if a.overlayKind != OverlayNone {
		a.statusBar.SetKeyHints(OverlayKeyHints())
		return
	}
	if a.pipelineRunning {
		a.statusBar.SetKeyHints(PipelineKeyHints())
		return
	}
	a.statusBar.SetKeyHints(DefaultKeyHints())
}

func (a *App) handleOverlayResult(result OverlayResult) {
	switch result.Action {
	case "command":
		a.handleOverlayCommand(result)
	case "clear":
		a.clearChatState()
	case "settings":
		a.handleOverlaySettings()
	case "toggle_agents":
		a.handleOverlayToggleAgents()
	case "switch_tier":
		a.handleOverlaySwitchTier()
	case "switch_theme":
		a.handleOverlaySwitchTheme()
	case "export_theme":
		a.handleOverlayExportTheme()
	case "agents_changed":
		a.handleOverlayAgentsChanged()
	case "diff":
		// Just close the overlay
	case "view_diff":
		a.handleOverlayViewDiff()
	case "select_file":
		a.handleOverlaySelectFile(result)
	}
}
