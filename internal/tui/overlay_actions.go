package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/config"
	"github.com/artemis-project/artemis/internal/tui/theme"
)

func (a *App) handleOverlayCommand(result OverlayResult) {
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
}

func (a *App) handleOverlaySettings() {
	a.viewMode = ViewConfig
	a.configView = NewConfigView(a.cfg)
	a.configView.SetSize(a.width, a.height)
}

func (a *App) handleOverlayToggleAgents() {
	a.cfg.Agents.Enabled = !a.cfg.Agents.Enabled
	if err := config.Save(a.cfg); err != nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
		return
	}
	a.initProvider()
}

func (a *App) handleOverlaySwitchTier() {
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
}

func (a *App) handleOverlaySwitchTheme() {
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
}

func (a *App) handleOverlayExportTheme() {
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
}

func (a *App) handleOverlayAgentsChanged() {
	if sel, ok := a.overlay.(*AgentSelector); ok {
		a.cfg = sel.Config()
	}
	if err := config.Save(a.cfg); err != nil {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: fmt.Sprintf("Failed to save config: %v", err)})
		return
	}
	a.initProvider()
}

func (a *App) handleOverlayViewDiff() {
	cwd, _ := os.Getwd()
	diffCtx, diffCancel := context.WithTimeout(context.Background(), 10*time.Second)
	cmd := exec.CommandContext(diffCtx, "git", "diff")
	cmd.Dir = cwd
	out, _ := cmd.CombinedOutput()
	diffCancel()
	if len(out) > 0 {
		overlay := NewDiffOverlay("Working Directory", string(out), a.width, a.height)
		a.overlay = overlay
		a.overlayKind = OverlayDiff
	} else {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "No uncommitted changes found."})
	}
}

func (a *App) handleOverlaySelectFile(result OverlayResult) {
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
