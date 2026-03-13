package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/agent/roles"
	"github.com/artemis-project/artemis/internal/bus"
	ghub "github.com/artemis-project/artemis/internal/github"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
)

func (a *App) initGitHub() {
	if !a.cfg.GitHub.Enabled || a.cfg.GitHub.Token == "" {
		return
	}
	if a.memStore == nil {
		return
	}

	logger := func(msg string) {
		a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: msg})
	}

	store, ok := a.memStore.(ghub.IssueStore)
	if !ok {
		logger("⚠ GitHub issue tracker unavailable: incompatible memory store")
		return
	}

	cwd, _ := os.Getwd()
	wtManager := ghub.NewWorktreeManager(cwd)

	// Shared provider builder for agent roles.
	buildProvider := func(role string) llm.Provider {
		primaryName := a.cfg.ProviderForRole(role)
		var primary llm.Provider
		if a.hasAPIKey(primaryName) {
			primary = llm.NewRetryProvider(llm.NewProvider(primaryName, &a.cfg), 2)
		}

		fbName := a.cfg.FallbackProviderForRole(role)
		var fallback llm.Provider
		if fbName != "" && fbName != primaryName && a.hasAPIKey(fbName) {
			fallback = llm.NewRetryProvider(llm.NewProvider(fbName, &a.cfg), 2)
		}

		if primary == nil && fallback == nil {
			return nil
		}
		if fallback == nil {
			return primary
		}
		return llm.NewFallbackProvider(primary, fallback)
	}

	// LLM triage callback (uses analyzer role's provider).
	var triageLLM ghub.TriageLLM
	if triageProvider := buildProvider("analyzer"); triageProvider != nil {
		triageLLM = func(ctx context.Context, sys, user string) (string, error) {
			return triageProvider.Send(ctx, []llm.Message{
				{Role: "system", Content: sys},
				{Role: "user", Content: user},
			})
		}
	}

	// FixEngine setup.
	var fixEngine ghub.FixEngine
	if a.cfg.Agents.Enabled {
		afe := ghub.NewAgentFixEngine(
			&a.cfg,
			a.eventBus,
			wtManager,
			logger,
		)

		afe.SetRunner(func(ctx context.Context, req ghub.FixRequest, wtPath string) (string, error) {
			toolExec := tools.NewToolExecutor(wtPath)

			issuePrompt := fmt.Sprintf(`Fix GitHub issue #%d in repository %s/%s.

**Issue Title**: %s

**Issue Description**:
%s

Analyze the issue, find the relevant code, implement the fix, and verify it works.
The codebase is available via your tools (read_file, grep, list_dir, etc.).`, req.IssueNumber, req.Owner, req.Repo, req.Title, req.Body)

			orchProvider := buildProvider("orchestrator")
			if orchProvider == nil {
				return "", fmt.Errorf("no orchestrator provider available")
			}

			resp, err := orchProvider.Send(ctx, []llm.Message{
				{Role: "system", Content: roles.OrchestratorPrompt},
				{Role: "user", Content: issuePrompt},
			})
			if err != nil {
				return "", fmt.Errorf("orchestrator LLM call: %w", err)
			}

			plan, err := orchestrator.ParsePlan(resp)
			if err != nil {
				return "", fmt.Errorf("parse plan: %w", err)
			}

			ss := state.NewSessionState()
			ss.AddArtifact(state.Artifact{Type: state.ArtifactUserRequest, Source: "github-issue", Content: issuePrompt})
			ss.AddArtifact(state.Artifact{Type: state.ArtifactOrchestratorPlan, Source: "orchestrator", Content: plan.Reasoning})

			eb := bus.NewEventBus(64)
			buildAgent := func(task orchestrator.AgentTask) agent.Agent {
				provider := buildProvider(task.Agent)
				if provider == nil {
					eb.Emit(bus.NewEvent(bus.EventAgentFail, task.Agent, "fix", "skipped: no API key"))
					return nil
				}
				ag := roles.NewRoleAgent(agent.Role(task.Agent), provider, eb, toolExec)
				if a.memStore != nil {
					ag.SetMemory(a.memStore)
				}
				if a.repoMapStore != nil {
					ag.SetRepoMap(a.repoMapStore)
				}
				ag.SetMaxToolIter(a.cfg.MaxToolIter)
				ag.SetTask(task.Task)
				ag.SetCritical(task.Critical)
				return ag
			}

			result := orchestrator.NewEngine(nil, eb, nil, nil).RunPlan(ctx, plan, ss, buildAgent)
			eb.Close()
			if !result.Completed {
				return "", fmt.Errorf("plan execution failed: %v", result.HaltError)
			}

			var parts []string
			for _, art := range ss.GetByType(state.ArtifactAnalysis) {
				parts = append(parts, art.Content)
			}
			if len(parts) == 0 {
				return strings.TrimSpace(plan.Reasoning), nil
			}
			return strings.Join(parts, "\n\n"), nil
		})

		fixEngine = afe
	}

	a.ghSyncer = ghub.NewSyncer(a.cfg.GitHub, store, logger)
	a.ghProcessor = ghub.NewProcessor(a.cfg.GitHub, store, logger, fixEngine, triageLLM)

	a.ghSyncer.Start(context.Background())

	// Background auto-triage + report.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if a.cfg.GitHub.AutoTriage {
			autoFix, needsHuman, notApplicable, triageErr := a.ghProcessor.TriageAll(ctx)
			if triageErr != nil {
				logger(fmt.Sprintf("GitHub triage error: %v", triageErr))
			} else {
				logger(fmt.Sprintf("GitHub triage complete: auto_fix=%d, needs_human=%d, not_applicable=%d", autoFix, needsHuman, notApplicable))
			}
		}

		report, err := a.ghSyncer.GetPendingReport(ctx)
		if err == nil && report != "No issues require human triage." {
			logger(report)
		}
	}()
}
