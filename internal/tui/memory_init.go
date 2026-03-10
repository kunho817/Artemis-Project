package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/artemis-project/artemis/internal/memory"
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

	// Run consolidation if enabled and we have conversation history
	if a.consolidator != nil && a.cfg.Memory.ConsolidateOnExit && len(a.history) > 2 {
		// Collect files touched during this session from activity panel
		filesTouched := a.activity.GetChangedFiles()

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		result, err := a.consolidator.Consolidate(ctx, a.sessionID, a.history, filesTouched)
		if err == nil && result != nil {
			_ = result // consolidation results are stored in DB automatically
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
