# Session #36 — TUI Overhaul Work Plan

> Generated: 2026-03-19
> Scope: Stability fixes, style unification, component decomposition, layout modernization, App decomposition
> Codebase: ~29,000 lines Go, 7,519 lines TUI (22 files + theme)
> Build baseline: `go build ./cmd/artemis/` — PASS

---

## Context

### Original Request
Comprehensive TUI overhaul addressing 5 goroutine leak risks, 13 hardcoded color violations, monolithic App struct (40+ fields), 251-line Update() dispatcher, 1,134-line configview.go, duplicated layout math, and dual streaming paths.

### Research Findings
- **Overlay interface** (Update/View/SetSize) is clean — all 6 overlays implement it. Keep as-is.
- **PlaceOverlayCentered()** compositing works well. No need for external overlay library.
- **Theme system** has 16 ThemeColors slots + 28 Styles categories + 3 presets (default/dracula/tokyonight). Missing: diff-specific color slots.
- **styles.go** uses package-level vars (ColorPrimary, BorderStyle, etc.) delegating to `theme.S` — backward-compat bridge pattern. Works, keep it.
- **RecoveryOverlay** and **ResumeOverlay** already use `overlayBoxStyle()` / `overlayTitleStyle()` helpers correctly. The analysis claim of "building styles inline" is **partially wrong** — they do use helpers, but also create many inline `lipgloss.NewStyle()` calls for labels/dims/options which is acceptable since those are local display logic.
- **DiffOverlay** is the real offender — 9 hardcoded hex colors, zero theme references.
- **activity.go** test results section has 4 hardcoded hex colors (#ff5555 ×2, #50fa7b ×1, #999999 ×1).
- **bgMgr.WaitAll()** is a simple `m.wg.Wait()` — no timeout capability exists.
- **Code Index goroutine** (memory_init.go:97) uses `context.Background()` — confirmed, no timeout.
- **Repo-map goroutine** (memory_init.go:80-84) already has a 2-minute timeout — good.
- **streaming.go:139** — `a.streamCh = nil` without draining. The provider's Stream() returns a channel that the provider's goroutine writes to. If we nil the channel without draining, the provider goroutine may block forever on `ch <- chunk`.
- **View() lines 687-697** duplicates layout math from **recalcLayout() lines 624-648**. Confirmed: both compute `innerHeight`, `chatWidth` with identical formulas.

### Definition of Done
1. All 5 goroutine leak risks eliminated with verified fixes
2. Zero hardcoded hex colors outside `theme/` package
3. ConfigView split into 3 files, each under 400 lines
4. Single source of truth for layout calculations
5. `go build ./cmd/artemis/` passes after every phase
6. No regression in overlay behavior (6 overlays still work)

---

## Work Objectives

### Core Objective
Transform the TUI from a monolithic prototype into a well-structured, leak-free, theme-consistent component system — without breaking any existing functionality.

### Deliverables
1. 5 goroutine leak fixes (Phase 1)
2. Theme-unified diff overlay + test results (Phase 2)
3. Decomposed configview.go, extracted overlay handlers, extracted Update handlers (Phase 3)
4. Unified layout system, dynamic key hints, min terminal size (Phase 4)
5. Sub-struct App decomposition, unified streaming (Phase 5)

---

## Must Have / Must NOT Have

### Must Have
- `go build ./cmd/artemis/` passes after every phase
- Every goroutine has a cancellation path (context or channel close)
- All user-visible colors come from ThemeColors
- Overlay interface remains unchanged (Update/View/SetSize)
- EventBus channel semantics preserved (buffered 64, non-blocking emit)

### Must NOT Have
- No external TUI library additions (no bubbletea-overlay, no lipgloss v2 compositor) — stay with current stack
- No changes to LLM provider code (internal/llm/*)
- No changes to orchestrator/engine logic (internal/orchestrator/engine.go)
- No changes to tool system (internal/tools/*)
- No functional behavior changes — this is purely structural/stability

---

## Phase 1: Stability Fixes

**Risk**: Low (additive fixes, no structural changes)
**Effort**: Small
**Dependencies**: None
**Deploy**: Independent

### Step 1.1: Code Index goroutine timeout

**File**: `internal/tui/memory_init.go`
**Lines**: 97-105
**Change**: Replace `context.Background()` with `context.WithTimeout(context.Background(), 5*time.Minute)` for Code Index indexing goroutine.

**Current code**:
```go
go func() {
    n, err := a.codeIndex.IndexDirectory(context.Background(), cwd)
    // ...
}()
```

**Target code**:
```go
go func() {
    idxCtx, idxCancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer idxCancel()
    n, err := a.codeIndex.IndexDirectory(idxCtx, cwd)
    // ...
}()
```

**Verification**: `go build ./cmd/artemis/` passes. Grep for `context.Background()` in memory_init.go — only repo-map (already has timeout) and consolidator init should remain.

### Step 1.2: BackgroundTaskManager WaitAll timeout

**File**: `internal/orchestrator/background.go`
**Lines**: 178-181
**Change**: Add `WaitAllWithTimeout(d time.Duration) bool` method that returns false if timeout expires. Keep existing `WaitAll()` for backward compat.

**New method**:
```go
// WaitAllWithTimeout blocks until all tasks complete or timeout expires.
// Returns true if all completed, false if timed out.
func (m *BackgroundTaskManager) WaitAllWithTimeout(d time.Duration) bool {
    done := make(chan struct{})
    go func() {
        m.wg.Wait()
        close(done)
    }()
    select {
    case <-done:
        return true
    case <-time.After(d):
        m.CancelAll()
        return false
    }
}
```

**File**: `internal/tui/pipeline.go`
**Lines**: 280 (the ONLY `bgMgr.WaitAll()` call site — in the dynamic plan goroutine)
**Change**: Replace `bgMgr.WaitAll()` with `bgMgr.WaitAllWithTimeout(2 * time.Minute)` at line 280.

**NOTE**: Only the dynamic pipeline goroutine (line 251) uses `BackgroundTaskManager`. The legacy pipeline goroutine (line 403) and resume pipeline goroutine (line 695) do NOT use background tasks — they call `eb.Close()` directly. There is exactly **1** call site to change.

**Verification**: `go build ./cmd/artemis/` passes. `grep -rn "WaitAll()" internal/tui/pipeline.go` returns 0 matches.

### Step 1.3: Ctrl+C pipeline goroutine cleanup

**File**: `internal/tui/app.go`
**Lines**: 336-341
**Change**: Add a `pipelineDone chan struct{}` field to App. Signal it when pipeline goroutine exits. On Ctrl+C, wait up to 3 seconds for pipeline goroutine before quitting.

**App struct change** (add field):
```go
pipelineDone chan struct{} // closed when pipeline goroutine exits
```

**Pipeline goroutine change** (pipeline.go, all 3 go func() blocks):
```go
go func() {
    defer func() {
        if a.pipelineDone != nil {
            close(a.pipelineDone)
        }
    }()
    // ... existing logic ...
}()
```

**Ctrl+C handler change** (app.go):
```go
case "ctrl+c":
    if a.cancelPipeline != nil {
        a.cancelPipeline()
    }
    // Wait briefly for pipeline goroutine to finish
    if a.pipelineDone != nil {
        select {
        case <-a.pipelineDone:
        case <-time.After(3 * time.Second):
        }
    }
    a.shutdownMemory()
    return a, tea.Quit
```

**IMPORTANT**: `pipelineDone` must be a pointer-shared channel (like `recoveryBridge`) or allocated in the pipeline launch function and stored in App. Since bubbletea copies the model, use `*chan struct{}` or allocate in pipeline.go and assign to App before returning.

**Revised approach**: Use a shared `*sync.WaitGroup` stored as a pointer field (like recoveryBridge):
```go
pipelineWg *sync.WaitGroup // shared pointer, survives model copies
```

Pipeline goroutine:
```go
wg := &sync.WaitGroup{}
wg.Add(1)
a.pipelineWg = wg
go func() {
    defer wg.Done()
    // ... existing logic ...
}()
```

Ctrl+C:
```go
case "ctrl+c":
    if a.cancelPipeline != nil {
        a.cancelPipeline()
    }
    if a.pipelineWg != nil {
        done := make(chan struct{})
        go func() { a.pipelineWg.Wait(); close(done) }()
        select {
        case <-done:
        case <-time.After(3 * time.Second):
        }
    }
    a.shutdownMemory()
    return a, tea.Quit
```

**Verification**: `go build ./cmd/artemis/` passes. Manual test: start a pipeline, press Ctrl+C, confirm clean exit without hanging.

### Step 1.4: Stream channel drain on error

**File**: `internal/tui/streaming.go`
**Lines**: 136-146
**Change**: Before setting `a.streamCh = nil` on error, drain the channel in a fire-and-forget goroutine.

**Current code**:
```go
if msg.Error != nil {
    // ...
    a.streamCh = nil
    a.streamingContent = ""
    // ...
}
```

**Target code**:
```go
if msg.Error != nil {
    // ...
    // Drain remaining chunks to unblock provider goroutine
    if a.streamCh != nil {
        ch := a.streamCh
        go func() {
            for range ch {
            }
        }()
    }
    a.streamCh = nil
    a.streamingContent = ""
    // ...
}
```

**Verification**: `go build ./cmd/artemis/` passes. The provider goroutine will no longer block indefinitely when the TUI side encounters a stream error.

### Step 1.5: RecoveryBridge channel lifecycle

**File**: `internal/tui/pipeline.go`
**Change**: Close `bridge.requestCh` when the pipeline goroutine exits (in the deferred cleanup). This ensures `waitForRecoveryRequest()` returns nil instead of blocking forever.

**3 pipeline goroutine instances with different bridge variable names**:

| Location | Line | Bridge Variable | Has bgMgr? |
|----------|------|----------------|------------|
| Dynamic plan | ~251 | `bridge` | YES (add WaitAllWithTimeout before Close) |
| Legacy pipeline | ~403 | `legacyBridge` | NO (just eb.Close) |
| Resume pipeline | ~695 | `bridge` | NO (just eb.Close) |

**Target pattern (dynamic plan goroutine, line ~251)**:
```go
go func() {
    defer close(bridge.requestCh) // unblock waitForRecoveryRequest
    result := engine.RunPlan(ctx, plan, ss, buildAgent)
    // ... save results ...
    bgMgr.WaitAllWithTimeout(2 * time.Minute)
    eb.Close()
}()
```

**Target pattern (legacy pipeline goroutine, line ~403)**:
```go
go func() {
    defer close(legacyBridge.requestCh) // note: legacyBridge, not bridge
    result := engine.Run(ctx, ss)
    // ... save results ...
    eb.Close()
}()
```

**Target pattern (resume pipeline goroutine, line ~695)**:
```go
go func() {
    defer close(bridge.requestCh)
    result := engine.RunPlanFromStep(ctx, plan, ss, buildAgent, startStep)
    // ... save results ...
    eb.Close()
}()
```

**File**: `internal/tui/recoverybridge.go`
**Change**: Make `waitForRecoveryRequest` handle closed channel gracefully (it already does — `ok` check on line 65-66). When the channel is closed, `waitForRecoveryRequest` returns a `tea.Cmd` that returns `nil`. A `tea.Cmd` returning `nil` is safely ignored by bubbletea — this is the expected behavior.

**Verification**: `go build ./cmd/artemis/` passes. After pipeline completes, `waitForRecoveryRequest` goroutine exits cleanly instead of leaking.

### Phase 1 Acceptance Criteria
- [ ] `go build ./cmd/artemis/` passes
- [ ] `go vet ./...` passes
- [ ] No `context.Background()` without timeout in goroutines (except main init paths)
- [ ] All `WaitAll()` calls replaced with `WaitAllWithTimeout()`
- [ ] Ctrl+C waits up to 3s for pipeline
- [ ] Stream channel drained on error
- [ ] RecoveryBridge channel closed on pipeline exit

---

## Phase 2: Style Unification

**Risk**: Low (additive theme slots, mechanical color replacements)
**Effort**: Small
**Dependencies**: None (independent of Phase 1)
**Deploy**: Independent

### Step 2.1: Add diff color slots to ThemeColors

**File**: `internal/tui/theme/theme.go`
**Lines**: Add after line 45 (after `AgentVerify`)

**New fields in ThemeColors struct**:
```go
// Diff viewer colors
DiffAdd     string `json:"diff_add"`     // added lines (green)
DiffRemove  string `json:"diff_remove"`  // removed lines (red)
DiffHeader  string `json:"diff_header"`  // diff headers, file paths (cyan)
DiffHunk    string `json:"diff_hunk"`    // @@ hunk markers (purple)
DiffContext string `json:"diff_context"` // unchanged lines (normal text)
DiffBorder  string `json:"diff_border"`  // diff overlay border
```

**New accessor methods**:
```go
func (c *ThemeColors) DiffAddColor() lipgloss.Color     { return Color(c.DiffAdd) }
func (c *ThemeColors) DiffRemoveColor() lipgloss.Color   { return Color(c.DiffRemove) }
func (c *ThemeColors) DiffHeaderColor() lipgloss.Color   { return Color(c.DiffHeader) }
func (c *ThemeColors) DiffHunkColor() lipgloss.Color     { return Color(c.DiffHunk) }
func (c *ThemeColors) DiffContextColor() lipgloss.Color  { return Color(c.DiffContext) }
func (c *ThemeColors) DiffBorderColor() lipgloss.Color   { return Color(c.DiffBorder) }
```

**Also update `mergeDefaults()` (line 436-488)**: Add 6 new nil-checks for Diff* fields so user custom themes without diff colors inherit from DefaultTheme():
```go
if target.DiffAdd == "" {
    target.DiffAdd = defaults.DiffAdd
}
if target.DiffRemove == "" {
    target.DiffRemove = defaults.DiffRemove
}
if target.DiffHeader == "" {
    target.DiffHeader = defaults.DiffHeader
}
if target.DiffHunk == "" {
    target.DiffHunk = defaults.DiffHunk
}
if target.DiffContext == "" {
    target.DiffContext = defaults.DiffContext
}
if target.DiffBorder == "" {
    target.DiffBorder = defaults.DiffBorder
}
```

**Also update `DefaultTheme()` hardcoded defaults**: Add diff color defaults matching the default.json preset values (#22C55E, #EF4444, #22D3EE, #7C3AED, #E5E7EB, #4B5563).

**Without these changes, user custom themes that omit diff_* fields will get `lipgloss.Color("")` — rendering invisible text. This is a silent regression.**

**Verification**: `go build ./cmd/artemis/` passes (new fields are unused but present).

### Step 2.2: Add diff styles to Styles struct and BuildStyles

**File**: `internal/tui/theme/theme.go`
**Lines**: Add to Styles struct (after AgentDivider, ~line 127)

**New fields in Styles struct**:
```go
// Diff overlay
DiffAdd     lipgloss.Style
DiffRemove  lipgloss.Style
DiffHeader  lipgloss.Style
DiffHunk    lipgloss.Style
DiffContext lipgloss.Style
DiffBorder  lipgloss.Style
DiffTitle   lipgloss.Style
DiffHint    lipgloss.Style
```

**Add to BuildStyles() function**:
```go
// Diff overlay
DiffAdd: lipgloss.NewStyle().
    Foreground(c.DiffAddColor()),
DiffRemove: lipgloss.NewStyle().
    Foreground(c.DiffRemoveColor()),
DiffHeader: lipgloss.NewStyle().
    Foreground(c.DiffHeaderColor()).Bold(true),
DiffHunk: lipgloss.NewStyle().
    Foreground(c.DiffHunkColor()),
DiffContext: lipgloss.NewStyle().
    Foreground(c.DiffContextColor()),
DiffBorder: lipgloss.NewStyle().
    Border(lipgloss.RoundedBorder()).
    BorderForeground(c.DiffBorderColor()),
DiffTitle: lipgloss.NewStyle().
    Bold(true).
    Foreground(c.TextColor()).
    Background(c.SurfaceColor()).
    Padding(0, 1),
DiffHint: lipgloss.NewStyle().
    Foreground(c.DimTextColor()),
```

**Verification**: `go build ./cmd/artemis/` passes.

### Step 2.3: Update all 3 preset JSONs

**Files**: `internal/tui/theme/presets/default.json`, `dracula.json`, `tokyonight.json`
**Change**: Add diff color slots to each preset.

**default.json** additions (Tailwind-inspired palette to match existing default theme):
```json
"diff_add": "#22C55E",
"diff_remove": "#EF4444",
"diff_header": "#22D3EE",
"diff_hunk": "#7C3AED",
"diff_context": "#E5E7EB",
"diff_border": "#4B5563"
```
NOTE: These match the default theme's existing success/error/accent/primary/text/border colors for visual consistency.

**dracula.json** additions (same Dracula palette):
```json
"diff_add": "#50fa7b",
"diff_remove": "#ff5555",
"diff_header": "#8be9fd",
"diff_hunk": "#bd93f9",
"diff_context": "#f8f8f2",
"diff_border": "#6272a4"
```

**tokyonight.json** additions (Tokyo Night palette):
```json
"diff_add": "#9ece6a",
"diff_remove": "#f7768e",
"diff_header": "#7dcfff",
"diff_hunk": "#bb9af7",
"diff_context": "#c0caf5",
"diff_border": "#3b4261"
```

**Verification**: `go build ./cmd/artemis/` passes. Parse each JSON in test or manual check.

### Step 2.4: Add diff style vars to styles.go

**File**: `internal/tui/styles.go`
**Change**: Add package-level vars for diff styles and wire them in RefreshStyles().

**New vars**:
```go
// Diff overlay styles
var (
    DiffAddStyle     lipgloss.Style
    DiffRemoveStyle  lipgloss.Style
    DiffHeaderStyle  lipgloss.Style
    DiffHunkStyle    lipgloss.Style
    DiffContextStyle lipgloss.Style
    DiffBorderStyle  lipgloss.Style
    DiffTitleStyle   lipgloss.Style
    DiffHintStyle    lipgloss.Style
)
```

**RefreshStyles() additions**:
```go
// Diff overlay
DiffAddStyle = s.DiffAdd
DiffRemoveStyle = s.DiffRemove
DiffHeaderStyle = s.DiffHeader
DiffHunkStyle = s.DiffHunk
DiffContextStyle = s.DiffContext
DiffBorderStyle = s.DiffBorder
DiffTitleStyle = s.DiffTitle
DiffHintStyle = s.DiffHint
```

**Verification**: `go build ./cmd/artemis/` passes.

### Step 2.5: Migrate diffoverlay.go to theme styles

**File**: `internal/tui/diffoverlay.go`
**Change**: Replace all 9 hardcoded `lipgloss.Color("#xxxxxx")` with theme style vars.

**View() method** — replace:
```go
// Title bar — was hardcoded #f8f8f2 on #44475a
title := DiffTitleStyle.Render(fmt.Sprintf(" Diff: %s ", d.fileName))

// Help bar — was hardcoded #6272a4
help := DiffHintStyle.Render("...")

// Border — was hardcoded #6272a4
return DiffBorderStyle.Padding(0, 1).Render(content)
```

**renderDiff() function** — replace:
```go
addStyle := DiffAddStyle         // was #50fa7b
removeStyle := DiffRemoveStyle   // was #ff5555
headerStyle := DiffHeaderStyle   // was #8be9fd
hunkStyle := DiffHunkStyle       // was #bd93f9
contextStyle := DiffContextStyle // was #f8f8f2
```

**Verification**: `go build ./cmd/artemis/` passes. `grep -n "lipgloss.Color" internal/tui/diffoverlay.go` returns 0 matches.

### Step 2.6: Migrate activity.go test results to theme colors

**File**: `internal/tui/activity.go`
**Lines**: 334-358 (test results section)
**Change**: Replace 4 hardcoded colors with theme references.

| Line | Current | Replacement |
|------|---------|-------------|
| 336 | `lipgloss.Color("#ff5555")` | `ColorError` |
| 339 | `lipgloss.Color("#50fa7b")` | `ColorSuccess` |
| 350 | `lipgloss.Color("#ff5555")` | `ColorError` |
| 357 | `lipgloss.Color("#999999")` | `ColorDimText` |

These `Color*` vars are already defined in styles.go and refreshed from theme.

**Verification**: `go build ./cmd/artemis/` passes. `grep -n 'lipgloss.Color("#' internal/tui/activity.go` returns 0 matches.

### Phase 2 Acceptance Criteria
- [ ] `go build ./cmd/artemis/` passes
- [ ] `grep -rn 'lipgloss.Color("#' internal/tui/` returns 0 matches (zero hardcoded hex outside theme/)
- [ ] All 3 preset JSONs have diff_* color slots
- [ ] Diff overlay renders correctly with each theme (manual check)
- [ ] Test results section colors match theme

---

## Phase 3: Component Decomposition

**Risk**: Medium (file splits, import changes, method moves)
**Effort**: Large
**Dependencies**: None (independent of Phase 1-2, but recommended after them)
**Deploy**: Independent

### Step 3.1: Split configview.go into 3 files

**File**: `internal/tui/configview.go` (1,134 lines)
**Target**:
- `configview.go` (~350 lines) — ConfigView struct, NewConfigView, Update, View, SetSize, GetConfig, tab switching, field navigation
- `configview_providers.go` (~400 lines) — buildProviderInputs(), renderProviderTab(), applyProviderConfig() for 5 provider tabs (Claude/Gemini/GPT/GLM/VLLM)
- `configview_system.go` (~380 lines) — buildSystemInputs(), renderSystemTab(), applySystemConfig() for 9 system sub-tabs

**Strategy**:
1. Identify all methods/functions that are provider-tab-specific
2. Identify all methods/functions that are system-sub-tab-specific
3. Move them to new files (same package, no import changes needed)
4. Keep shared helpers (fieldCountForTab, buildInputs entry point) in configview.go

**Exact function extraction targets** (from LSP document symbols):

**To `configview_providers.go`** (~400 lines):
- `renderProviderContent()` (line 662-707) — 5-provider tab rendering
- `styledProviderName()` (line 790-797) — provider name styling
- `isProviderEnabled()` (line 295-313) — provider toggle state
- `applyInputsToConfig()` (line 253-293) — provider input → config mapping
- Provider-specific cases from `buildInputs()` (line 164-227) — extract as `buildProviderInputs()`

**To `configview_system.go`** (~380 lines):
- `buildSystemInputs()` (line 837-926) — 9 sub-tab input building
- `applySystemInputs()` (line 928-992) — 9 sub-tab config application
- `renderSystemContent()` (line 994-1111) — 9 sub-tab rendering
- `toggleSystemField()` (line 329-398) — system toggle handler
- `isSystemToggleField()` (line 400-430) — system toggle detection
- `systemFieldToInputIdx()` (line 107-162) — system field → input mapping
- `renderSubTabs()` (line 799-835) — system sub-tab bar rendering
- `canSwitchSubTab()` (line 432-449) — sub-tab navigation guard
- `renderNumberField()` (line 1113-1134) — number field rendering

**Keep in `configview.go`** (~350 lines):
- `ConfigView` struct + constants + `tabLabels`/`sysSubLabels` (lines 1-51)
- `NewConfigView()` (line 54)
- `fieldCountForTab()` (line 63)
- `isOnAgentsTab()`/`isOnSystemTab()` (lines 96-103)
- `buildInputs()` — entry point that dispatches to `buildProviderInputs()`/`buildSystemInputs()` (line 164, slimmed)
- `focusField()` (line 229)
- `SetSize()` (line 452)
- `Update()` (line 465)
- `GetConfig()` (line 558)
- `View()` (line 564)
- `renderTabs()` (line 617)
- `renderAgentsContent()` (line 709)
- `renderToggleField()` (line 744)
- `renderCustomToggle()` (line 770)
- `toggleAgentField()` (line 315)

**Import notes**: All 3 files need `lipgloss`, `config`, `theme`. `configview_providers.go` needs `textinput`. `configview_system.go` needs `textinput`, `strconv`, `fmt`. No new imports required — all are already in configview.go.

**Verification**: `go build ./cmd/artemis/` passes. `wc -l internal/tui/configview*.go` shows no file exceeds 450 lines.

### Step 3.2: Extract overlay action handlers

**File**: `internal/tui/app.go`
**Lines**: 773-910 (handleOverlayResult)
**Target**: Extract each case into a named method on `*App`

**Current pattern** (137-line switch):
```go
func (a *App) handleOverlayResult(result OverlayResult) {
    switch result.Action {
    case "command": // 10 lines
    case "clear": // 1 line
    case "settings": // 3 lines
    case "toggle_agents": // 8 lines
    case "switch_tier": // 10 lines
    case "switch_theme": // 20 lines
    case "export_theme": // 10 lines
    case "agents_changed": // 8 lines
    case "diff": // 1 line
    case "view_diff": // 12 lines
    case "select_file": // 20 lines
    }
}
```

**Target pattern**:
```go
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
        // no-op, just close
    case "view_diff":
        a.handleOverlayViewDiff()
    case "select_file":
        a.handleOverlaySelectFile(result)
    }
}
```

**New file**: `internal/tui/overlay_actions.go` — contains all `handleOverlay*` methods.

**Verification**: `go build ./cmd/artemis/` passes. `handleOverlayResult` is now ≤30 lines.

### Step 3.3: Extract Update() message handlers

**File**: `internal/tui/app.go`
**Lines**: 267-518 (Update method, 251 lines)

**Strategy**: The Update() method already delegates most message types to handler methods (`handleLLMResponse`, `handleStreamChunk`, `handleAgentEvent`, `handlePipelineComplete`). The remaining inline logic is:

1. **Key dispatch** (lines 287-377) — ~90 lines of `case "ctrl+k"`, `case "ctrl+a"`, etc.
2. **Message type dispatch** (lines 387-497) — already clean, delegates to handlers.
3. **Textarea forwarding** (lines 499-517) — ~20 lines.

**Extract**: Move key dispatch into `handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd)` in a new file `internal/tui/keybindings.go`.

This reduces Update() to:
```go
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if a.viewMode == ViewConfig {
        return a.updateConfigView(msg)
    }
    if a.overlayKind != OverlayNone && a.overlay != nil {
        // overlay intercept
    }
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return a.handleKeyMsg(msg)
    case tea.MouseMsg:
        return a.handleMouseMsg(msg)
    // ... message types (already delegated) ...
    }
    return a.handleInputUpdate(msg)
}
```

**Verification**: `go build ./cmd/artemis/` passes. `wc -l internal/tui/app.go` is ≤700 lines (down from 911).

### Phase 3 Acceptance Criteria
- [ ] `go build ./cmd/artemis/` passes
- [ ] `go vet ./...` passes
- [ ] No file in `internal/tui/` exceeds 750 lines (currently configview.go is 1,134)
- [ ] ConfigView split into 3 files
- [ ] Update() is ≤80 lines
- [ ] handleOverlayResult() is ≤30 lines
- [ ] All existing overlay behavior preserved (6 overlays)
- [ ] Config save/load still works (Ctrl+S)

---

## Phase 4: Layout Modernization

**Risk**: Medium (changes to View() rendering, potential visual regression)
**Effort**: Medium
**Dependencies**: Phase 3 (benefits from cleaner App, but not strictly required)
**Deploy**: Independent

### Step 4.1: Unify layout calculations

**File**: `internal/tui/app.go`
**Problem**: View() lines 687-697 and recalcLayout() lines 624-648 compute identical values (`innerWidth`, `innerHeight`, `chatWidth`) independently. If one changes, the other desyncs.

**Change**: Introduce a `layoutState` struct computed once in `recalcLayout()` and stored in App:

```go
type layoutState struct {
    innerWidth    int
    innerHeight   int
    chatWidth     int
    activityWidth int
    inputHeight   int
}
```

- `recalcLayout()` computes and stores `a.layout = computeLayout(a.width, a.height, a.input.Height(), a.layoutMode)`
- `View()` reads from `a.layout` instead of recomputing

**Verification**: `go build ./cmd/artemis/` passes. `grep -n "innerWidth\|innerHeight\|chatWidth" internal/tui/app.go` shows these computed only in `computeLayout()`.

### Step 4.2: Dynamic key hints

**File**: `internal/tui/statusbar.go`
**Current**: Static key hints set once in NewApp() (line 200-208)
**Change**: Make `SetKeyHints()` context-aware. Add hint sets for different states:

```go
func defaultKeyHints() []KeyHint { ... }
func pipelineKeyHints() []KeyHint { ... } // show Ctrl+L=Cancel
func overlayKeyHints() []KeyHint { ... }  // show Esc=Close
func configKeyHints() []KeyHint { ... }   // show Ctrl+S=Save, Esc=Back
```

**Trigger points**:
- Pipeline starts → `a.statusBar.SetKeyHints(pipelineKeyHints())`
- Pipeline ends → `a.statusBar.SetKeyHints(defaultKeyHints())`
- Overlay opens → `a.statusBar.SetKeyHints(overlayKeyHints())`
- Overlay closes → restore previous hints
- Config view enters → `a.statusBar.SetKeyHints(configKeyHints())`

**File**: `internal/tui/keybindings.go` (from Phase 3, if already done) or `internal/tui/app.go` (if Phase 3 not yet applied) — add hint switching at state transitions.

**NOTE**: If Phase 4 is executed before Phase 3, the key dispatch is still in `app.go` (Update method). Place the hint switching calls there instead of `keybindings.go`. Either location works since they're in the same package.

**Verification**: `go build ./cmd/artemis/` passes. Key hints change visually when opening overlay or entering pipeline.

### Step 4.3: Minimum terminal size check

**File**: `internal/tui/app.go` (View method)
**Change**: Add minimum size check at top of View():

```go
const (
    minTermWidth  = 80
    minTermHeight = 24
)

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
    // ... rest of View()
}
```

**Verification**: `go build ./cmd/artemis/` passes. Resize terminal below 80×24 → shows message. Resize back → normal rendering.

### Phase 4 Acceptance Criteria
- [ ] `go build ./cmd/artemis/` passes
- [ ] Layout math exists in exactly one place (computeLayout)
- [ ] Key hints change when entering/leaving pipeline/overlay/config
- [ ] Terminal below 80×24 shows graceful message

---

## Phase 5: App Decomposition

**Risk**: High (structural changes to central type, potential for subtle state bugs due to bubbletea value semantics)
**Effort**: Large
**Dependencies**: Phase 3 (cleaner Update/handleOverlayResult), Phase 4 (unified layout)
**Deploy**: Independent but builds on Phase 3-4

### ⚠ CRITICAL WARNING: Bubbletea Value Semantics

Bubbletea passes the model by **value** through Update(). This means:
- Sub-structs are copied on every Update call
- Pointer fields survive copies (shared state) — used for recoveryBridge, pipelineWg
- Moving state into sub-structs is safe **only if** those sub-structs are value types or their pointer fields are intentionally shared

This constraint limits how far we can decompose. We must NOT put channels, sync primitives, or mutable shared state into value-copied sub-structs without pointer indirection.

### Step 5.1: Extract state sub-structs (value-safe only)

**File**: `internal/tui/app.go`
**Change**: Group related fields into sub-structs. Only group **value-type** or **read-mostly** fields:

```go
// CostState tracks token and cost totals (value-safe, copied each Update).
type CostState struct {
    TotalTokens int
    TotalCost   float64
}

// SessionState tracks session identity (value-safe).
type SessionState struct {
    SessionID           string
    ParentSessionID     string
    ActivePipelineRunID string
}

// HistoryState tracks conversation history (value-safe — slices are reference types).
type HistoryState struct {
    History          []llm.Message
    HistoryWindow    *agent.HistoryWindow // pointer, safe
    StreamingContent string
    PipelineOutputs  []string
}
```

**App struct becomes**:
```go
type App struct {
    // UI components
    chat      ChatPanel
    activity  ActivityPanel
    statusBar StatusBar
    input     textarea.Model
    layout    layoutState // from Phase 4

    // View management
    viewMode   ViewMode
    configView ConfigView

    // LLM
    cfg      config.Config
    provider llm.Provider

    // Pipeline (pointer fields — survive copies)
    eventBus        *bus.EventBus
    cancelPipeline  context.CancelFunc
    pipelineRunning bool
    pipelineWg      *sync.WaitGroup    // from Phase 1
    recoveryBridge  *RecoveryBridge
    recoveryQueue   []RecoveryRequest

    // Tools
    toolExecutor  *tools.ToolExecutor
    skillRegistry *agent.SkillRegistry

    // Memory (all pointers — survive copies)
    memStore        memory.MemoryStore
    vectorStore     memory.VectorSearcher
    consolidator    *memory.Consolidator
    repoMapStore    *memory.RepoMapStore
    codeIndex       *memory.CodeIndex
    lspManager      *lsp.Manager
    mcpManager      *mcp.Manager
    checkpointStore state.CheckpointStore
    pendingResumeRun *state.IncompleteRun
    projectRules    string

    // GitHub (pointers)
    ghSyncer    *ghub.Syncer
    ghProcessor *ghub.Processor

    // Grouped value state
    cost    CostState
    session SessionState
    hist    HistoryState

    // UI state
    focused    FocusedPanel
    layoutMode LayoutMode
    width      int
    height     int
    ready      bool

    overlayKind OverlayKind
    overlay     Overlay

    // Streaming
    streamCh    <-chan llm.StreamChunk
    agentStreams map[string]*agentStreamInfo
}
```

**Mechanical update**: All references to `a.totalTokens` → `a.cost.TotalTokens`, `a.sessionID` → `a.session.SessionID`, `a.history` → `a.hist.History`, etc. Use find-and-replace across all TUI files.

**Verification**: `go build ./cmd/artemis/` passes. `go vet ./...` passes. All functionality preserved.

### Step 5.2: Unify streaming paths (design only — defer implementation)

**Analysis**: Single-mode uses `streamCh + streamingContent` (streaming.go). Multi-mode uses `agentStreams map + EventBus` (events.go). These serve fundamentally different purposes:
- Single mode: Direct LLM streaming to one chat message
- Multi mode: Multiple agents emit events through EventBus, each with their own streaming state

**Decision**: These are **intentionally different patterns** for different use cases. Forcing them into a common StreamManager would add abstraction without benefit. The single-mode path is simple (one channel, one accumulator). The multi-mode path is event-driven (EventBus → agentStreams map).

**Recommendation**: Keep separate. Document the dual-path design in a code comment at the top of streaming.go. This is NOT a defect — it's appropriate architectural divergence.

**Action**: Add explanatory comment to streaming.go:
```go
// streaming.go handles single-provider LLM streaming.
// In single-provider mode, the user's message is sent directly to one LLM,
// and chunks are received via streamCh channel.
//
// Multi-agent streaming uses a completely different path:
// events.go processes AgentEventMsg from the EventBus, tracking per-agent
// streaming state via agentStreams map. This intentional divergence reflects
// the fundamentally different communication patterns (direct channel vs event bus).
```

### Step 5.3: Add component shutdown hooks

**File**: `internal/tui/app.go`
**Change**: Consolidate all shutdown logic into a single `shutdown()` method called from Ctrl+C handler. Currently cleanup is scattered:

```go
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
```

**Ctrl+C handler becomes**:
```go
case "ctrl+c":
    a.shutdown()
    return a, tea.Quit
```

**Verification**: `go build ./cmd/artemis/` passes. Ctrl+C cleanly shuts down all components.

### Phase 5 Acceptance Criteria
- [ ] `go build ./cmd/artemis/` passes
- [ ] `go vet ./...` passes
- [ ] App struct fields grouped into logical sub-structs
- [ ] Dual streaming paths documented (not unified — intentional)
- [ ] Single `shutdown()` method for all cleanup
- [ ] All references to old field names updated

---

## Task Flow and Dependencies

```
Phase 1 (Stability)  ──────────────────────────────────────►  Can deploy independently
Phase 2 (Styles)     ──────────────────────────────────────►  Can deploy independently
                                                              │
Phase 3 (Decompose)  ──────────────────────────────────────►  Can deploy independently
                                                              │
Phase 4 (Layout)     ─── depends on Phase 3 (cleaner code) ─►  Recommended after Phase 3
                                                              │
Phase 5 (App)        ─── depends on Phase 3+4 (cleaner base)─►  Recommended last
```

**Recommended execution order**: 1 → 2 → 3 → 4 → 5

Phases 1 and 2 are independent and can be done in parallel. Phase 3 is independent but provides a cleaner base for 4 and 5.

---

## Commit Strategy

| Phase | Commit Message |
|-------|---------------|
| 1.1-1.5 | `fix: session #36 — fix 5 goroutine leak risks (timeout, drain, cleanup)` |
| 2.1-2.4 | `feat: session #36 — add diff theme color slots and styles` |
| 2.5-2.6 | `fix: session #36 — replace 13 hardcoded hex colors with theme references` |
| 3.1 | `refactor: session #36 — split configview.go into 3 files (1134→3×~370 lines)` |
| 3.2-3.3 | `refactor: session #36 — extract overlay action handlers and keybindings` |
| 4.1-4.3 | `feat: session #36 — unified layout, dynamic key hints, min terminal size` |
| 5.1-5.3 | `refactor: session #36 — decompose App struct, consolidate shutdown` |

---

## Effort Summary

| Phase | Effort | Risk | Files Modified | Lines Changed (est.) |
|-------|--------|------|---------------|---------------------|
| 1: Stability | Small | Low | 4 files | ~60 lines |
| 2: Styles | Small | Low | 7 files | ~120 lines |
| 3: Decompose | Large | Medium | 4 files + 3 new | ~300 lines moved |
| 4: Layout | Medium | Medium | 3 files | ~100 lines |
| 5: App Decompose | Large | High | 10+ files | ~200 lines refactored |

**Total estimated**: ~780 lines changed/moved across 15+ files.

---

## Success Criteria

1. **Zero goroutine leaks**: All goroutines have cancellation/timeout paths
2. **Zero hardcoded colors**: `grep -rn 'lipgloss.Color("#' internal/tui/` returns only theme/ package
3. **No file >750 lines**: `wc -l internal/tui/*.go | sort -rn | head -5` shows all under 750
4. **Clean build**: `go build ./cmd/artemis/` passes
5. **Clean vet**: `go vet ./...` passes
6. **Functional parity**: All 6 overlays, config save/load, streaming, pipeline execution unchanged
