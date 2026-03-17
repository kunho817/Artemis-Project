package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/artemis-project/artemis/internal/agent"
	"github.com/artemis-project/artemis/internal/lsp"
	"github.com/artemis-project/artemis/internal/mcp"
	"github.com/artemis-project/artemis/internal/memory"
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

	// Phase C-5: Wire checkpoint store (SQLiteStore implements state.CheckpointStore)
	a.checkpointStore = store

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

	// Load project rules (ARTEMIS.md / .artemis/RULES.md)
	cwd, _ := os.Getwd()
	a.projectRules = agent.LoadProjectRules(cwd)
	if a.projectRules != "" {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: "Project rules loaded (ARTEMIS.md)",
		})
	}

	// Phase E-1: Custom skills
	a.initCustomSkills()

	// Phase D-1: LSP Control Plane
	a.initLSP()

	// Phase D-2: ast-grep (structural search/replace)
	a.initAstGrep()

	// MCP server tools
	a.initMCP()

	// Phase 4: GitHub issue tracker
	a.initGitHub()

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

	// Phase C-5: Check for incomplete pipeline runs (deferred overlay)
	a.checkForIncompleteRuns()
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
			// Phase 5: Link parent session if this was loaded from another session
			if a.parentSessionID != "" {
				summary, _ := a.memStore.GetSession(ctx, a.sessionID)
				if summary != nil && summary.ParentSessionID == "" {
					summary.ParentSessionID = a.parentSessionID
					_ = a.memStore.SaveSession(ctx, summary)
				}
			}

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

	// Phase D: Shutdown LSP servers
	if a.mcpManager != nil {
		a.mcpManager.Shutdown()
	}

	if a.lspManager != nil {
		a.lspManager.Shutdown()
	}

	if a.ghSyncer != nil {
		a.ghSyncer.Stop()
	}

	a.memStore.Close()
}

// saveMessageToDB persists a message to the memory store (non-blocking, best-effort).
// Phase 5: Links message to active pipeline run if available.
func (a *App) saveMessageToDB(role, content, agentRole string) {
	if a.memStore == nil {
		return
	}
	msg := &memory.SessionMessage{
		SessionID:     a.sessionID,
		Role:          role,
		Content:       content,
		AgentRole:     agentRole,
		PipelineRunID: a.activePipelineRunID,
	}
	// Fire and forget — message save failure is non-fatal
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.memStore.SaveMessage(ctx, msg)
	}()
}

// initCustomSkills loads custom skills from global and project directories.
func (a *App) initCustomSkills() {
	if !a.cfg.Skills.Enabled || a.skillRegistry == nil {
		return
	}

	totalLoaded := 0

	// 1. Load global skills (~/.artemis/skills/)
	globalDir := a.cfg.GlobalSkillsDir()
	if n, err := a.skillRegistry.LoadCustomSkills(globalDir, "global"); err != nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Warning: global skills load error: %v", err),
		})
	} else if n > 0 {
		totalLoaded += n
	}

	// 2. Load project-local skills (.artemis/skills/)
	if a.cfg.Skills.AutoLoad {
		cwd, _ := os.Getwd()
		projectDir := cwd + "/.artemis/skills"
		if n, err := a.skillRegistry.LoadCustomSkills(projectDir, "project"); err != nil {
			a.chat.AddMessage(ChatMessage{
				Role:    RoleSystem,
				Content: fmt.Sprintf("Warning: project skills load error: %v", err),
			})
		} else if n > 0 {
			totalLoaded += n
		}
	}

	if totalLoaded > 0 {
		customIDs := a.skillRegistry.CustomSkillIDs()
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Custom skills loaded: %s (%d total)", joinStrings(customIDs), totalLoaded),
		})
	}
}

// initAstGrep tries to find ast-grep and wires it into the tool executor.
func (a *App) initAstGrep() {
	cachePath := ""
	if dir, err := os.UserHomeDir(); err == nil {
		cachePath = dir + "/.artemis/bin"
	}
	sgPath, err := tools.EnsureAstGrep("", cachePath)
	if err != nil {
		// ast-grep is optional — silently skip if not found
		return
	}
	a.toolExecutor.SetAstGrep(sgPath)
	a.chat.AddMessage(ChatMessage{
		Role:    RoleSystem,
		Content: fmt.Sprintf("ast-grep enabled (%s)", sgPath),
	})
}

// initLSP initializes the LSP Control Plane if enabled in config.
func (a *App) initLSP() {
	if !a.cfg.LSP.Enabled {
		return
	}

	// Convert config LSPServerConfigs to lsp.ServerConfig
	serverConfigs := make(map[string]lsp.ServerConfig)
	for lang, sc := range a.cfg.LSP.Servers {
		serverConfigs[lang] = lsp.ServerConfig{
			Command: sc.Command,
			Args:    sc.Args,
			Enabled: sc.Enabled,
		}
	}

	cwd, _ := os.Getwd()
	mgr := lsp.NewManager(cwd, serverConfigs)
	a.lspManager = mgr

	// Wire into tool executor
	a.toolExecutor.SetLSPManager(mgr)

	// Report configured languages
	langs := mgr.ConfiguredLanguages()
	if len(langs) > 0 {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("LSP enabled for: %s (lazy loading)", joinStrings(langs)),
		})
	}
}

// initMCP initializes MCP servers and dynamically registers discovered MCP tools.
func (a *App) initMCP() {
	if !a.cfg.MCP.Enabled || len(a.cfg.MCP.Servers) == 0 {
		return
	}

	// Convert config to mcp.ServerDef
	var servers []mcp.ServerDef
	for _, s := range a.cfg.MCP.Servers {
		servers = append(servers, mcp.ServerDef{
			ID:      s.ID,
			Command: s.Command,
			Args:    s.Args,
			Env:     s.Env,
			Enabled: s.Enabled,
		})
	}

	mgr := mcp.NewManager(servers)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx); err != nil {
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("Warning: MCP connection error: %v", err),
		})
	}

	connected := mgr.ConnectedServers()
	if len(connected) > 0 {
		a.mcpManager = mgr
		a.toolExecutor.SetMCPManager(mgr)
		mcpTools := mgr.DiscoveredTools()
		a.chat.AddMessage(ChatMessage{
			Role:    RoleSystem,
			Content: fmt.Sprintf("MCP enabled: %d servers, %d tools discovered", len(connected), len(mcpTools)),
		})
	}
}

// joinStrings joins a slice with commas.
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// checkForIncompleteRuns queries for interrupted pipeline runs and stores the
// most recent one for deferred overlay display (shown after first WindowSizeMsg).
func (a *App) checkForIncompleteRuns() {
	if a.checkpointStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runs, err := a.checkpointStore.GetIncompleteRuns(ctx, "")
	if err != nil || len(runs) == 0 {
		return
	}
	// Store the most recent incomplete run for deferred overlay display.
	// The overlay is shown in Update() on the first WindowSizeMsg (when a.ready becomes true).
	most := runs[0]
	a.pendingResumeRun = &most
}
