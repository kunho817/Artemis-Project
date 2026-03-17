package tools

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

// Worktree represents an isolated git worktree for parallel agent execution.
type Worktree struct {
	Path    string // absolute path to the worktree
	Branch  string // branch name
	cleanup func()
}

// Clean removes the worktree. MUST be called when done (typically via defer).
func (w *Worktree) Clean() {
	if w.cleanup != nil {
		w.cleanup()
	}
}

// ParallelWorktreeManager creates and manages multiple isolated git worktrees
// for parallel agent execution. Each worktree is a full copy of the repo at
// a specific commit, allowing multiple agents to edit files without conflicts.
type ParallelWorktreeManager struct {
	mu       sync.Mutex
	repoRoot string
	active   []*Worktree
}

// NewParallelWorktreeManager creates a manager for the given repository root.
func NewParallelWorktreeManager(repoRoot string) *ParallelWorktreeManager {
	return &ParallelWorktreeManager{repoRoot: repoRoot}
}

// Create creates an isolated worktree branched from the current HEAD.
// The worktree gets its own ToolExecutor for safe parallel file operations.
// Returns the Worktree and a ToolExecutor scoped to that worktree.
func (m *ParallelWorktreeManager) Create(ctx context.Context, name string) (*Worktree, *ToolExecutor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	branch := fmt.Sprintf("artemis-parallel-%s-%d", name, time.Now().UnixNano())

	tmpDir, err := os.MkdirTemp("", "artemis-wt-*")
	if err != nil {
		return nil, nil, fmt.Errorf("worktree: create temp dir: %w", err)
	}

	wtPath := filepath.Join(tmpDir, "wt")

	// Create worktree from current HEAD
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, wtPath, "HEAD")
	cmd.Dir = m.repoRoot
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, nil, fmt.Errorf("worktree: git worktree add failed: %w\n%s", err, string(out))
	}

	cleanup := func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		// Remove worktree
		rmCmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rmCmd.Dir = m.repoRoot
		if runtime.GOOS == "windows" {
			setHiddenProcessAttrs(rmCmd)
		}
		rmCmd.Run()

		// Delete branch
		brCmd := exec.Command("git", "branch", "-D", branch)
		brCmd.Dir = m.repoRoot
		if runtime.GOOS == "windows" {
			setHiddenProcessAttrs(brCmd)
		}
		brCmd.Run()

		os.RemoveAll(tmpDir)

		// Remove from active list
		for i, w := range m.active {
			if w.Path == wtPath {
				m.active = append(m.active[:i], m.active[i+1:]...)
				break
			}
		}
	}

	wt := &Worktree{
		Path:    wtPath,
		Branch:  branch,
		cleanup: cleanup,
	}

	m.active = append(m.active, wt)

	// Create a ToolExecutor scoped to this worktree
	te := NewToolExecutor(wtPath)

	return wt, te, nil
}

// GetDiff returns the git diff of changes made in a worktree (compared to HEAD).
func (m *ParallelWorktreeManager) GetDiff(ctx context.Context, wt *Worktree) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = wt.Path
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("worktree: git diff failed: %w", err)
	}
	return string(out), nil
}

// MergeBack applies changes from a worktree back to the main repo.
// Uses git diff + apply to avoid merge conflicts with branch topology.
func (m *ParallelWorktreeManager) MergeBack(ctx context.Context, wt *Worktree) error {
	// Get the diff
	diff, err := m.GetDiff(ctx, wt)
	if err != nil {
		return err
	}
	if strings.TrimSpace(diff) == "" {
		return nil // no changes
	}

	// Apply the diff to the main repo
	cmd := exec.CommandContext(ctx, "git", "apply", "--allow-empty")
	cmd.Dir = m.repoRoot
	cmd.Stdin = strings.NewReader(diff)
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: git apply failed: %w\n%s", err, string(out))
	}
	return nil
}

// CleanupAll removes all active worktrees.
func (m *ParallelWorktreeManager) CleanupAll() {
	// Copy to avoid mutex issues during cleanup
	m.mu.Lock()
	active := make([]*Worktree, len(m.active))
	copy(active, m.active)
	m.mu.Unlock()

	for _, wt := range active {
		wt.Clean()
	}
}

// ActiveCount returns the number of active worktrees.
func (m *ParallelWorktreeManager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}
