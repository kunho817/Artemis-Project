package lsp

import "os/exec"

// setHiddenProcessAttrs sets platform-specific process attributes.
// On Windows, this would hide the console window for LSP subprocesses.
// For now, this is a no-op stub (same pattern as tools/git.go).
func setHiddenProcessAttrs(cmd *exec.Cmd) {
	// Intentionally empty — Windows console hiding requires
	// syscall.SysProcAttr{HideWindow: true} with build tags.
	// LSP servers work fine without it.
}
