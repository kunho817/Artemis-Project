package tui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (a App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Close overlay on Esc if active (highest priority)
	if msg.String() == "esc" && a.overlayKind != OverlayNone {
		a.overlayKind = OverlayNone
		a.overlay = nil
		a.syncKeyHints()
		return a, nil
	}

	switch msg.String() {
	case "ctrl+k":
		if a.overlayKind == OverlayNone {
			cp := NewCommandPalette(a.width, a.height)
			a.overlayKind = OverlayCommandPalette
			a.overlay = cp
			a.statusBar.SetKeyHints(OverlayKeyHints())
			return a, cp.Init()
		}
		return a, nil
	case "ctrl+a":
		if a.overlayKind == OverlayNone {
			as := NewAgentSelector(a.cfg, a.width, a.height)
			a.overlayKind = OverlayAgentSelector
			a.overlay = as
			a.statusBar.SetKeyHints(OverlayKeyHints())
		}
		return a, nil
	case "ctrl+o":
		if a.overlayKind == OverlayNone {
			cwd, _ := os.Getwd()
			fp := NewFilePicker(cwd, a.width, a.height)
			a.overlayKind = OverlayFilePicker
			a.overlay = fp
			a.statusBar.SetKeyHints(OverlayKeyHints())
		}
		return a, nil
	case "ctrl+d":
		if a.overlayKind == OverlayNone {
			cwd, _ := os.Getwd()
			diffCtx, diffCancel := context.WithTimeout(context.Background(), 10*time.Second)
			cmd := exec.CommandContext(diffCtx, "git", "diff")
			cmd.Dir = cwd
			out, _ := cmd.CombinedOutput()
			diffCancel()
			if len(out) > 0 {
				return a, func() tea.Msg {
					return DiffViewMsg{FileName: "Working Directory", Diff: string(out)}
				}
			}
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: "No uncommitted changes found."})
		}
		return a, nil
	case "ctrl+c":
		a.shutdown()
		return a, tea.Quit
	case "ctrl+s":
		a.viewMode = ViewConfig
		a.configView = NewConfigView(a.cfg)
		a.configView.SetSize(a.width, a.height)
		a.syncKeyHints()
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
	case "enter":
		// Enter = send message (Shift+Enter for newline is handled by textarea)
		if strings.TrimSpace(a.input.Value()) != "" {
			return a.handleSubmit()
		}
		return a, nil // Empty input: do nothing
	case "ctrl+enter":
		// Legacy keybinding — also sends
		if strings.TrimSpace(a.input.Value()) != "" {
			return a.handleSubmit()
		}
		return a, nil
	case "up", "down", "pgup", "pgdown":
		// Forward scroll keys to chat viewport
		scrollCmd := a.chat.Update(msg)
		if scrollCmd != nil {
			cmds = append(cmds, scrollCmd)
		}
		return a, tea.Batch(cmds...)
	}

	return a, nil
}

func (a App) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	scrollCmd := a.chat.Update(msg)
	var cmds []tea.Cmd
	if scrollCmd != nil {
		cmds = append(cmds, scrollCmd)
	}
	return a, tea.Batch(cmds...)
}

func (a App) handleInputUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
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
