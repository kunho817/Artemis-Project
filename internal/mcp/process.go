package mcp

import "os/exec"

// setHiddenProcessAttrs sets platform-specific process attributes.
// On Windows, this would hide the console window for MCP server subprocesses.
func setHiddenProcessAttrs(cmd *exec.Cmd) {
	// Intentionally empty — see tools/git.go for explanation.
}
