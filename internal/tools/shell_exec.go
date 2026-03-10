package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ShellExecTool executes a shell command with a 30-second timeout.
type ShellExecTool struct {
	baseDir string
}

func (t *ShellExecTool) Name() string        { return "shell_exec" }
func (t *ShellExecTool) Description() string { return "Execute a shell command (30s timeout)" }
func (t *ShellExecTool) Parameters() string {
	return "command (string, required) — command to execute; workdir (string, optional) — working directory relative to project root"
}

func (t *ShellExecTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return ToolResult{Error: "missing required parameter: command"}, nil
	}

	workDir := t.baseDir
	if wd, ok := params["workdir"].(string); ok && wd != "" {
		workDir = filepath.Join(t.baseDir, filepath.Clean(wd))
		if !isInsideDir(t.baseDir, workDir) {
			return ToolResult{Error: "workdir outside project directory"}, nil
		}
	}

	// Create command with timeout
	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", command)
	}
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var sb strings.Builder
	if stdout.Len() > 0 {
		sb.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("[stderr]\n")
		sb.WriteString(stderr.String())
	}

	// Truncate large outputs
	output := sb.String()
	const maxOutput = 50 * 1024 // 50KB
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (truncated)"
	}

	if err != nil {
		exitMsg := fmt.Sprintf("\n[exit: %s]", err)
		return ToolResult{
			Content: output + exitMsg,
			Error:   err.Error(),
		}, nil
	}

	if output == "" {
		output = "(no output)"
	}

	return ToolResult{Content: output + "\n[exit: 0]"}, nil
}
