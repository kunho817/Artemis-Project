package github

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/config"
)

type FixEngine interface {
	ExecuteFix(ctx context.Context, req FixRequest) (*FixResult, error)
}

type FixRequest struct {
	IssueNumber int
	Title       string
	Body        string
	Branch      string
	BaseBranch  string
	Owner       string
	Repo        string
}

type FixResult struct {
	Analysis     string
	Summary      string
	FilesChanged []string
	CommitSHA    string
}

type AgentFixEngine struct {
	cfg      *config.Config
	eventBus *bus.EventBus
	worktree *WorktreeManager
	logger   func(string)
	runner   PipelineRunner
}

// PipelineRunner executes the Orchestrator→Engine pipeline in a worktree.
// It should return analysis text for PR summaries.
type PipelineRunner func(ctx context.Context, req FixRequest, wtPath string) (string, error)

func NewAgentFixEngine(
	cfg *config.Config,
	eventBus *bus.EventBus,
	worktree *WorktreeManager,
	logger func(string),
) *AgentFixEngine {
	if logger == nil {
		logger = func(string) {}
	}
	return &AgentFixEngine{cfg: cfg, eventBus: eventBus, worktree: worktree, logger: logger}
}

// SetRunner injects the pipeline runner implementation.
func (e *AgentFixEngine) SetRunner(runner PipelineRunner) {
	e.runner = runner
}

func (e *AgentFixEngine) ExecuteFix(ctx context.Context, req FixRequest) (*FixResult, error) {
	if e.cfg == nil {
		return nil, fmt.Errorf("fixengine: missing config")
	}
	if e.worktree == nil {
		return nil, fmt.Errorf("fixengine: missing worktree manager")
	}

	e.logger(fmt.Sprintf("FixEngine: creating worktree for issue #%d...", req.IssueNumber))
	wtPath, cleanup, err := e.worktree.Create(ctx, req.Branch, req.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("fixengine: create worktree: %w", err)
	}
	defer cleanup()

	e.logger(fmt.Sprintf("FixEngine: worktree ready at %s", filepath.Base(wtPath)))
	if e.runner == nil {
		return nil, fmt.Errorf("fixengine: pipeline runner not configured")
	}
	analysis, err := e.runner(ctx, req, wtPath)
	if err != nil {
		return nil, fmt.Errorf("fixengine: execute pipeline: %w", err)
	}

	if e.eventBus != nil {
		e.eventBus.Emit(bus.NewEvent(bus.EventAgentProgress, "FixEngine", "fix", fmt.Sprintf("Issue #%d pipeline completed", req.IssueNumber)))
	}

	filesChanged, err := e.getChangedFiles(ctx, wtPath)
	if err != nil {
		return nil, fmt.Errorf("fixengine: check changes: %w", err)
	}

	analysis = strings.TrimSpace(analysis)

	if analysis == "" {
		analysis = "No analysis output captured."
	}

	if len(filesChanged) == 0 {
		return &FixResult{Analysis: analysis, Summary: "No code changes were produced by agents."}, nil
	}

	commitSHA, err := e.commitAndPush(ctx, wtPath, req)
	if err != nil {
		return nil, fmt.Errorf("fixengine: commit/push: %w", err)
	}

	return &FixResult{
		Analysis:     analysis,
		Summary:      e.buildSummary(req, filesChanged, analysis),
		FilesChanged: filesChanged,
		CommitSHA:    commitSHA,
	}, nil
}

func (e *AgentFixEngine) getChangedFiles(ctx context.Context, wtPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = wtPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	var files []string
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 3 {
			name := strings.TrimSpace(line[2:])
			if idx := strings.Index(name, " -> "); idx >= 0 {
				name = name[idx+4:]
			}
			files = append(files, name)
		}
	}
	return files, nil
}

func (e *AgentFixEngine) commitAndPush(ctx context.Context, wtPath string, req FixRequest) (string, error) {
	gitCmd := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = wtPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	if err := gitCmd("add", "-A"); err != nil {
		return "", err
	}
	commitMsg := fmt.Sprintf("fix: resolve #%d — %s\n\nAutomated fix by Artemis Agent.", req.IssueNumber, req.Title)
	if err := gitCmd("commit", "-m", commitMsg); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = wtPath
	shaOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	sha := strings.TrimSpace(string(shaOut))
	if err := gitCmd("push", "-u", "origin", req.Branch); err != nil {
		return sha, err
	}
	return sha, nil
}

func (e *AgentFixEngine) buildSummary(req FixRequest, files []string, analysis string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Automated fix for issue #%d.\n\n", req.IssueNumber))
	if analysis != "" && analysis != "No analysis output captured." {
		sb.WriteString("**Analysis**:\n")
		sb.WriteString(analysis)
		sb.WriteString("\n\n")
	}
	if len(files) > 0 {
		sb.WriteString("**Files Changed**:\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	}
	return sb.String()
}
