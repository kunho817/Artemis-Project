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
	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/internal/orchestrator"
	"github.com/artemis-project/artemis/internal/state"
	"github.com/artemis-project/artemis/internal/tools"
)

// initMemory initializes the persistent memory store if enabled in config.
func (a *App) initMemory() {
	if !a.cfg.Memory.Enabled {
		return
	}

	dbPath := a.cfg.MemoryDBPath()
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		// Memory failure is non-fatal — log and continue without persistence
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("⚠ Memory system unavailable: %v", err),
		})
		return
	}
	a.memStore = store

	// Phase 2: initialize vector store if enabled
	if a.cfg.Vector.Enabled && a.cfg.Vector.APIKey != "" {
		vecPath := a.cfg.VectorStorePath()
		vs, err := memory.NewVectorStore(vecPath, a.cfg.Vector.APIKey, a.cfg.Vector.Model)
		if err != nil {
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: fmt.Sprintf("⚠ Vector search unavailable: %v", err),
			})
		} else {
			a.vectorStore = vs
			store.SetVectorStore(vs)
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: "Vector search enabled (Voyage AI)",
			})
		}
	}

	// Create consolidator using the active provider (budget tier preferred for cost)
	if a.provider != nil {
		c := memory.NewConsolidator(store, a.provider)
		if a.vectorStore != nil {
			c.SetVectorStore(a.vectorStore)
		}
		a.consolidator = c
	}

	// Phase 3: initialize repo-map if enabled
	if a.cfg.RepoMap.Enabled {
		ctagsPath, ctagsErr := memory.EnsureCTags(a.cfg.RepoMap.CTagsPath, a.cfg.CTagsCachePath())
		if ctagsErr != nil {
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: fmt.Sprintf("\u26a0 Repo-map unavailable: %v", ctagsErr),
			})
		} else {
			parser := memory.NewCtagsParser(ctagsPath)
			cwd, _ := os.Getwd()
			rm := memory.NewRepoMapStore(store.DB(), parser, cwd, a.cfg.RepoMap.ExcludePatterns)
			a.repoMapStore = rm
			store.SetRepoMapStore(rm)
			// Initial indexing in background (non-blocking)
			go func() {
				idxCtx, idxCancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer idxCancel()
				rm.IndexDirectory(idxCtx, cwd)
			}()
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: fmt.Sprintf("Repo-map enabled (ctags: %s)", ctagsPath),
			})
		}
	}

	// Phase 4: GitHub issue tracker
	if a.cfg.GitHub.Enabled && a.cfg.GitHub.Token != "" {
		logger := func(msg string) {
			a.chat.AddMessage(ChatMessage{Role: RoleSystem, Content: msg})
		}

		// Create WorktreeManager + FixEngine
		cwd, _ := os.Getwd()
		wtManager := ghub.NewWorktreeManager(cwd)

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
				buildAgent := func(role string) agent.Agent {
					provider := buildProvider(role)
					if provider == nil {
						eb.Emit(bus.NewEvent(bus.EventAgentFail, role, "fix", "skipped: no API key"))
						return nil
					}
					ag := roles.NewRoleAgent(agent.Role(role), provider, eb, toolExec)
					if a.memStore != nil {
						ag.SetMemory(a.memStore)
					}
					if a.repoMapStore != nil {
						ag.SetRepoMap(a.repoMapStore)
					}
					ag.SetMaxToolIter(a.cfg.MaxToolIter)
					return ag
				}

				result := orchestrator.NewEngine(nil, eb).RunPlan(ctx, plan, ss, buildAgent)
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
		a.ghProcessor = ghub.NewProcessor(a.cfg.GitHub, store, logger, fixEngine)

		a.ghSyncer.Start(context.Background())

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	// Load memory stats for status display
	if stats, err := store.Stats(context.Background()); err == nil {
		vecStatus := ""
		if a.vectorStore != nil {
			vecStatus = " + vector search"
		}
		rmStatus := ""
		if a.repoMapStore != nil {
			rmStatus = " + repo-map"
		}
		a.chat.AddMessage(ChatMessage{
			Role: RoleSystem,
			Content: fmt.Sprintf("Memory loaded: %d facts, %d sessions, %d files tracked%s%s",
				stats.FactCount, stats.SessionCount, stats.FileCount, vecStatus, rmStatus),
		})
	}
}

// shutdownMemory runs consolidation and closes the memory store.
// Called when the application exits.
func (a *App) shutdownMemory() {
	if a.memStore == nil {
		return
	}

	// COLD tier: archive raw messages before consolidation (preserves full conversation)
	var archiver *memory.Archiver
	if a.cfg.Memory.ArchiveEnabled && len(a.history) > 0 {
		arc, err := memory.NewArchiver(a.cfg.ArchivePath())
		if err == nil {
			archiver = arc
			_ = archiver.ArchiveMessages(a.sessionID, a.history)
		}
	}

	// Run consolidation if enabled and we have conversation history
	if a.consolidator != nil && a.cfg.Memory.ConsolidateOnExit && len(a.history) > 2 {
		// Collect files touched during this session from activity panel
		filesTouched := a.activity.GetChangedFiles()

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		result, err := a.consolidator.Consolidate(ctx, a.sessionID, a.history, filesTouched)
		if err == nil && result != nil {
			// COLD tier: archive consolidation results
			if archiver != nil {
				_ = archiver.ArchiveConsolidation(result)
			}
		}
	}

	// Run fact decay if configured
	if a.cfg.Memory.MaxFactAge > 0 {
		maxAge := time.Duration(a.cfg.Memory.MaxFactAge) * 24 * time.Hour
		a.memStore.DecayFacts(context.Background(), maxAge, a.cfg.Memory.MinFactUseCount)
	}
	// Close repo-map store
	if a.repoMapStore != nil {
		a.repoMapStore.Close()
	}

	// Close vector store
	if a.vectorStore != nil {
		a.vectorStore.Close()
	}

	if a.ghSyncer != nil {
		a.ghSyncer.Stop()
	}

	a.memStore.Close()
}

// saveMessageToDB persists a message to the memory store (non-blocking, best-effort).
func (a *App) saveMessageToDB(role, content, agentRole string) {
	if a.memStore == nil {
		return
	}
	msg := &memory.SessionMessage{
		SessionID: a.sessionID,
		Role:      role,
		Content:   content,
		AgentRole: agentRole,
	}
	// Fire and forget — message save failure is non-fatal
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.memStore.SaveMessage(ctx, msg)
	}()
}
