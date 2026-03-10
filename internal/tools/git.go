package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// GitStatusTool shows the current git status in a safe, read-only manner.
type GitStatusTool struct {
	baseDir string
}

func (t *GitStatusTool) Name() string { return "git_status" }
func (t *GitStatusTool) Description() string {
	return "Show git status (staged, unstaged, untracked files)"
}
func (t *GitStatusTool) Parameters() string {
	return "short (bool, optional, default false) — use short format"
}

func (t *GitStatusTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	args := []string{"status"}

	if short, ok := params["short"].(bool); ok && short {
		args = append(args, "--short")
	}

	output, err := runGitCommand(ctx, t.baseDir, args...)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("git status failed: %s", err)}, nil
	}

	if strings.TrimSpace(output) == "" {
		output = "nothing to commit, working tree clean"
	}

	return ToolResult{Content: output}, nil
}

// GitDiffTool shows git diffs in a safe, read-only manner.
type GitDiffTool struct {
	baseDir string
}

func (t *GitDiffTool) Name() string { return "git_diff" }
func (t *GitDiffTool) Description() string {
	return "Show git diff (unstaged changes, staged changes, or between refs)"
}
func (t *GitDiffTool) Parameters() string {
	return `staged (bool, optional, default false) — show staged changes (--cached); path (string, optional) — limit diff to a specific file/directory; ref (string, optional) — diff against a specific ref (e.g. "HEAD~1", "main")`
}

func (t *GitDiffTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	args := []string{"diff", "--stat"}

	// Add --cached for staged changes
	if staged, ok := params["staged"].(bool); ok && staged {
		args = append(args, "--cached")
	}

	// Add ref comparison
	if ref, ok := params["ref"].(string); ok && ref != "" {
		// Sanitize ref — only allow safe characters
		if !isValidGitRef(ref) {
			return ToolResult{Error: "invalid git ref: must contain only alphanumeric, /, -, _, ~, ^, ."}, nil
		}
		args = append(args, ref)
	}

	// Always include the full diff after stat
	statOutput, err := runGitCommand(ctx, t.baseDir, args...)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("git diff failed: %s", err)}, nil
	}

	// Also get the full diff (without --stat)
	fullArgs := make([]string, 0, len(args))
	for _, a := range args {
		if a != "--stat" {
			fullArgs = append(fullArgs, a)
		}
	}

	// Add path filter
	if path, ok := params["path"].(string); ok && path != "" {
		fullArgs = append(fullArgs, "--", path)
	}

	fullOutput, err := runGitCommand(ctx, t.baseDir, fullArgs...)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("git diff failed: %s", err)}, nil
	}

	if strings.TrimSpace(statOutput) == "" && strings.TrimSpace(fullOutput) == "" {
		return ToolResult{Content: "No changes"}, nil
	}

	// Combine stat summary + full diff
	var sb strings.Builder
	if strings.TrimSpace(statOutput) != "" {
		sb.WriteString("--- Summary ---\n")
		sb.WriteString(statOutput)
		sb.WriteString("\n--- Full Diff ---\n")
	}
	sb.WriteString(fullOutput)

	// Truncate very large diffs
	output := sb.String()
	const maxDiff = 100 * 1024 // 100KB
	if len(output) > maxDiff {
		output = output[:maxDiff] + "\n... (diff truncated, too large)"
	}

	return ToolResult{Content: output}, nil
}

// GitLogTool shows recent commit history.
type GitLogTool struct {
	baseDir string
}

func (t *GitLogTool) Name() string        { return "git_log" }
func (t *GitLogTool) Description() string { return "Show recent git commit history" }
func (t *GitLogTool) Parameters() string {
	return `count (number, optional, default 10) — number of commits to show; oneline (bool, optional, default true) — one-line format; path (string, optional) — limit to commits affecting a specific file`
}

func (t *GitLogTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	count := 10
	if c, ok := params["count"].(float64); ok && c > 0 {
		count = int(c)
		if count > 50 {
			count = 50
		}
	}

	oneline := true
	if ol, ok := params["oneline"].(bool); ok {
		oneline = ol
	}

	args := []string{"log", fmt.Sprintf("-n%d", count)}
	if oneline {
		args = append(args, "--oneline")
	} else {
		args = append(args, "--format=%h %an %ar%n  %s")
	}

	// Path filter
	if path, ok := params["path"].(string); ok && path != "" {
		args = append(args, "--", path)
	}

	output, err := runGitCommand(ctx, t.baseDir, args...)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("git log failed: %s", err)}, nil
	}

	if strings.TrimSpace(output) == "" {
		return ToolResult{Content: "No commits found"}, nil
	}

	return ToolResult{Content: output}, nil
}

// runGitCommand executes a git command with a timeout and returns the output.
func runGitCommand(ctx context.Context, workDir string, args ...string) (string, error) {
	execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "git", args...)
	cmd.Dir = workDir

	// On Windows, suppress console window popup
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr for better diagnostics
		errMsg := err.Error()
		if stderr.Len() > 0 {
			errMsg = strings.TrimSpace(stderr.String())
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	return stdout.String(), nil
}

// isValidGitRef checks that a ref string contains only safe characters.
func isValidGitRef(ref string) bool {
	for _, c := range ref {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '/' || c == '-' || c == '_' || c == '~' || c == '^' || c == '.':
		default:
			return false
		}
	}
	return len(ref) > 0 && len(ref) < 256
}

// setHiddenProcessAttrs sets platform-specific process attributes.
// On Windows, this hides the console window for git subprocess.
// On non-Windows, this is a no-op (see git_unix.go / git_windows.go).
// For now, we use a stub since cross-platform SysProcAttr requires build tags.
func setHiddenProcessAttrs(cmd *exec.Cmd) {
	// Intentionally empty — Windows console hiding requires
	// syscall.SysProcAttr{HideWindow: true} but that needs build tags.
	// The git commands work fine without it.
}
