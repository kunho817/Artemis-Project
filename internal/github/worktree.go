package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// WorktreeManager handles git worktree lifecycle for isolated agent execution.
type WorktreeManager struct {
	mu       sync.Mutex
	repoRoot string
}

// NewWorktreeManager creates a manager for the given repository root.
func NewWorktreeManager(repoRoot string) *WorktreeManager {
	return &WorktreeManager{repoRoot: repoRoot}
}

// Create creates an isolated git worktree for a branch.
// Returns the worktree path and a cleanup function that MUST be called (typically via defer).
// Flow: git fetch origin → git worktree add -b <branch> <path> origin/<baseRef>
func (m *WorktreeManager) Create(ctx context.Context, branch, baseRef string) (string, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create temp parent dir
	tmpDir, err := os.MkdirTemp("", "artemis-fix-*")
	if err != nil {
		return "", nil, fmt.Errorf("worktree: create temp dir: %w", err)
	}

	wtPath := filepath.Join(tmpDir, "wt")

	// Fetch latest from origin
	if _, err := m.gitExec(ctx, "fetch", "origin"); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("worktree: git fetch: %w", err)
	}

	// Create worktree with new branch from origin/<baseRef>
	if _, err := m.gitExec(ctx, "worktree", "add", "-b", branch, wtPath, "origin/"+baseRef); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("worktree: git worktree add: %w", err)
	}

	// Cleanup function
	cleanup := func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Force remove worktree metadata
		_, _ = m.gitExec(context.Background(), "worktree", "remove", "-f", wtPath)

		// Remove physical directory with retry on Windows (file locks)
		m.removeWithRetry(tmpDir)

		// Prune stale worktree entries
		_, _ = m.gitExec(context.Background(), "worktree", "prune")
	}

	return wtPath, cleanup, nil
}

// removeWithRetry removes a directory, retrying on Windows due to potential file locks.
func (m *WorktreeManager) removeWithRetry(path string) {
	for i := 0; i < 3; i++ {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		if runtime.GOOS == "windows" {
			time.Sleep(500 * time.Millisecond)
		}
	}
	// Best effort — log-worthy but not fatal
	_ = os.RemoveAll(path)
}

// gitExec runs a git command in the repository root.
func (m *WorktreeManager) gitExec(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}
