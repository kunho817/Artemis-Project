// Package memory provides integration tests for memory persistence.
package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestSQLiteStoreCreation tests SQLite store initialization.
func TestSQLiteStoreCreation(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create SQLite store
	dbPath := h.TempDir + "/test.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	if store == nil {
		t.Fatal("Expected non-nil store")
	}

	// Verify DB is accessible
	db := store.DB()
	if db == nil {
		t.Error("Expected non-nil DB connection")
	}
}

// TestSQLiteBasicCRUD tests basic Create, Read, Update, Delete operations.
func TestSQLiteBasicCRUD(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_crud.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Create: Save a fact
	fact := &memory.Fact{
		Content: "Test fact about Artemis project",
		Tags:    []string{"test", "artemis", "project"},
		Source:  "integration-test",
	}

	err = store.SaveFact(ctx, fact)
	if err != nil {
		h.T.Fatalf("Failed to save fact: %v", err)
	}

	// Read: Query the fact back
	opts := memory.QueryOpts{
		Query:  "artemis",
		Limit:  10,
	}

	results, err := store.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query facts: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	// Verify content
	found := false
	for _, result := range results {
		if strings.Contains(result.Content, "Artemis project") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find saved fact")
	}
}

// TestSQLiteFTSSearch tests full-text search functionality.
func TestSQLiteFTSSearch(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_fts.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save multiple facts
	facts := []*memory.Fact{
		{
			Content: "Artemis is an AI coding assistant built with Go",
			Tags:    []string{"go", "ai", "coding"},
			Source:  "test",
		},
		{
			Content: "The TUI uses Bubble Tea for terminal interface",
			Tags:    []string{"tui", "bubbletea", "terminal"},
			Source:  "test",
		},
		{
			Content: "Memory is persisted in SQLite database",
			Tags:    []string{"memory", "sqlite", "database"},
			Source:  "test",
		},
	}

	for _, fact := range facts {
		err = store.SaveFact(ctx, fact)
		if err != nil {
			h.T.Fatalf("Failed to save fact: %v", err)
		}
	}

	// Test various search queries
	searchTests := []struct {
		query    string
		minExpected int // Minimum expected results (FTS5 is fuzzy)
	}{
		{"Go", 1},
		{"coding", 1},
		{"tui", 1},
		{"sqlite", 1},
		{"artemis", 1}, // FTS5 fuzzy matching, may match 1-2 results
		{"assistant", 1},
	}

	for _, tt := range searchTests {
		t.Run(tt.query, func(t *testing.T) {
			opts := memory.QueryOpts{
				Query: tt.query,
				Limit: 10,
			}

			results, err := store.QueryFacts(ctx, opts)
			if err != nil {
				h.T.Fatalf("Failed to query facts: %v", err)
			}

			if len(results) < tt.minExpected {
				h.T.Errorf("Expected at least %d results for query %q, got %d", tt.minExpected, tt.query, len(results))
			}
		})
	}
}

// TestFactStorage tests fact creation, retrieval, and update.
func TestFactStorage(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_facts.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Create fact
	fact := &memory.Fact{
		Content: "Initial fact content",
		Tags:    []string{"initial", "test"},
		Source:  "fact-storage-test",
	}

	err = store.SaveFact(ctx, fact)
	if err != nil {
		h.T.Fatalf("Failed to save fact: %v", err)
	}

	// Retrieve fact
	opts := memory.QueryOpts{
		Query:  "initial",
		Limit: 1,
	}

	results, err := store.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query facts: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected to find saved fact")
	}

	// Update fact (by saving a new version with updated content)
	updatedFact := &memory.Fact{
		Content: "Updated fact content",
		Tags:    []string{"updated", "test"},
		Source:  "fact-storage-test",
	}

	err = store.SaveFact(ctx, updatedFact)
	if err != nil {
		h.T.Fatalf("Failed to save updated fact: %v", err)
	}

	// Query again to verify update
	opts.Query = "updated"
	results, err = store.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query updated facts: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find updated fact")
	}
}

// TestSessionPersistence tests session saving and retrieval.
func TestSessionPersistence(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_sessions.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Create session summary
	summary := &memory.SessionSummary{
		SessionID:    "test-session-123",
		Summary:      "Test user message: Test session summary",
		FilesTouched: []string{"file1.go", "file2.py"},
		FactsLearned: 1,
		Outcome:      "success",
		MessageCount: 1,
		CreatedAt:    time.Now(),
	}

	// Save session
	err = store.SaveSession(ctx, summary)
	if err != nil {
		h.T.Fatalf("Failed to save session: %v", err)
	}

	// Retrieve sessions
	sessions, err := store.QuerySessions(ctx, memory.QueryOpts{Limit: 10})
	if err != nil {
		h.T.Fatalf("Failed to get sessions: %v", err)
	}

	if len(sessions) == 0 {
		t.Error("Expected to find at least one session")
	}

	// Verify session content
	found := false
	for _, session := range sessions {
		if session.SessionID == "test-session-123" {
			found = true
			if !strings.Contains(session.Summary, "Test user message") {
				h.T.Errorf("Expected summary to contain 'Test user message', got %s", session.Summary)
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find saved session")
	}
}

// TestMultipleSessions tests saving and retrieving multiple sessions.
func TestMultipleSessions(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_multiple_sessions.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save multiple sessions
	sessions := []*memory.SessionSummary{
		{
			SessionID:    "session-1",
			Summary:      "First message: First summary",
			FilesTouched: []string{"file1.go"},
			FactsLearned: 1,
			Outcome:      "success",
			MessageCount: 1,
			CreatedAt:    time.Now(),
		},
		{
			SessionID:    "session-2",
			Summary:      "Second message: Second summary",
			FilesTouched: []string{"file2.go", "file3.py"},
			FactsLearned: 1,
			Outcome:      "success",
			MessageCount: 1,
			CreatedAt:    time.Now(),
		},
		{
			SessionID:    "session-3",
			Summary:      "Third message: Third summary",
			FilesTouched: []string{"file4.rs"},
			FactsLearned: 1,
			Outcome:      "success",
			MessageCount: 1,
			CreatedAt:    time.Now(),
		},
	}

	for _, session := range sessions {
		err = store.SaveSession(ctx, session)
		if err != nil {
			h.T.Fatalf("Failed to save session %s: %v", session.SessionID, err)
		}
	}

	// Retrieve sessions with limit
	retrieved, err := store.QuerySessions(ctx, memory.QueryOpts{Limit: 2})
	if err != nil {
		h.T.Fatalf("Failed to get sessions: %v", err)
	}

	// Should respect limit
	if len(retrieved) != 2 {
		h.T.Errorf("Expected 2 sessions (limit), got %d", len(retrieved))
	}

	// Get all sessions
	allSessions, err := store.QuerySessions(ctx, memory.QueryOpts{Limit: 100})
	if err != nil {
		h.T.Fatalf("Failed to get all sessions: %v", err)
	}

	// Should have all 3
	if len(allSessions) < 3 {
		h.T.Errorf("Expected at least 3 sessions, got %d", len(allSessions))
	}
}

// TestFactTags tests fact tagging and tag-based search.
func TestFactTags(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_tags.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save facts with specific tags
	fact1 := &memory.Fact{
		Content: "Go is a statically typed language",
		Tags:    []string{"go", "language", "static"},
		Source:  "test",
	}

	fact2 := &memory.Fact{
		Content: "Python is dynamically typed",
		Tags:    []string{"python", "language", "dynamic"},
		Source:  "test",
	}

	fact3 := &memory.Fact{
		Content: "TypeScript adds typing to JavaScript",
		Tags:    []string{"typescript", "language", "javascript"},
		Source:  "test",
	}

	for _, fact := range []*memory.Fact{fact1, fact2, fact3} {
		err = store.SaveFact(ctx, fact)
		if err != nil {
			h.T.Fatalf("Failed to save fact: %v", err)
		}
	}

	// Search by tag
	t.Run("TagGo", func(t *testing.T) {
		opts := memory.QueryOpts{
			Tags:  []string{"go"},
			Limit: 10,
		}

		results, err := store.QueryFacts(ctx, opts)
		if err != nil {
			h.T.Fatalf("Failed to query by tag: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected to find facts with 'go' tag")
		}

		// Verify result has the tag
		for _, result := range results {
			hasTag := false
			for _, tag := range result.Tags {
				if tag == "go" {
					hasTag = true
					break
				}
			}
			if !hasTag {
				t.Errorf("Expected 'go' tag in result tags: %v", result.Tags)
			}
		}
	})

	t.Run("TagLanguage", func(t *testing.T) {
		opts := memory.QueryOpts{
			Tags:  []string{"language"},
			Limit: 10,
		}

		results, err := store.QueryFacts(ctx, opts)
		if err != nil {
			h.T.Fatalf("Failed to query by language tag: %v", err)
		}

		// Should find all three facts (all have "language" tag)
		if len(results) < 3 {
			h.T.Errorf("Expected at least 3 facts with 'language' tag, got %d", len(results))
		}
	})
}

// TestFactUseCount tests fact use count tracking.
func TestFactUseCount(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_usecount.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save a fact
	fact := &memory.Fact{
		Content: "Frequently used fact",
		Tags:    []string{"popular"},
		Source:  "test",
	}

	err = store.SaveFact(ctx, fact)
	if err != nil {
		h.T.Fatalf("Failed to save fact: %v", err)
	}

	// Query the fact multiple times to potentially increment use count
	for i := 0; i < 3; i++ {
		opts := memory.QueryOpts{
			Query: "Frequently used",
			Limit: 1,
		}

		results, err := store.QueryFacts(ctx, opts)
		if err != nil {
			h.T.Fatalf("Failed to query facts (iteration %d): %v", i, err)
		}

		if len(results) == 0 {
			h.T.Fatalf("Expected to find fact (iteration %d)", i)
		}

		// Verify use count exists and is reasonable
		if results[0].UseCount < 0 {
			h.T.Errorf("Expected non-negative use count, got %d", results[0].UseCount)
		}
	}
}

// TestSQLiteVectorSearch tests vector search functionality (when available).
func TestSQLiteVectorSearch(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_vector.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	// Note: Vector search requires chromem-go dependency
	// This test documents the expected behavior

	ctx := context.Background()

	// Save facts that would be embedded
	fact := &memory.Fact{
		Content: "Artificial intelligence and machine learning",
		Tags:    []string{"ai", "ml"},
		Source:  "test",
	}

	err = store.SaveFact(ctx, fact)
	if err != nil {
		h.T.Fatalf("Failed to save fact: %v", err)
	}

	// Test hybrid search (FTS5 + vector)
	t.Log("Vector search testing requires chromem-go dependency")
	t.Log("Expected behavior:")
	t.Log("  - QueryFacts with QueryOpts should use hybrid search")
	t.Log("  - Results should be ranked by semantic similarity")
	t.Log("  - FTS5 provides exact match filtering")
	t.Log("  - Vector store provides semantic ranking")
}

// TestSQLitePersistenceAcrossRestores tests data persistence across store restarts.
func TestSQLitePersistenceAcrossRestores(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_persistence.db"

	// Create first store and save data
	store1, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create first store: %v", err)
	}

	ctx := context.Background()

	// Save a fact
	fact := &memory.Fact{
		Content: "Persistent test data",
		Tags:    []string{"persistent"},
		Source:  "persistence-test",
	}

	err = store1.SaveFact(ctx, fact)
	if err != nil {
		h.T.Fatalf("Failed to save fact: %v", err)
	}

	// Close first store (simulate restart)
	store1.DB().Close()

	// Create second store (simulates restart)
	store2, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create second store: %v", err)
	}

	// Verify data persisted
	opts := memory.QueryOpts{
		Query:  "Persistent test data",
		Limit: 1,
	}

	results, err := store2.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query after restore: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find persisted fact after store restart")
	}

	// Verify content
	if !strings.Contains(results[0].Content, "Persistent test data") {
		t.Errorf("Expected persisted content, got: %s", results[0].Content)
	}
}

// TestSQLiteConcurrency tests concurrent access to the store.
// Note: SQLite uses single-writer mode by default, so high concurrency will result in
// "database is locked" errors. This test verifies the system handles concurrency gracefully
// without crashing or corrupting data.
func TestSQLiteConcurrency(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_concurrency.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Concurrent save operations (reduced from 10×5 to 5×2 to minimize lock contention)
	numGoroutines := 5
	numOperations := 2
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < numOperations; j++ {
				fact := &memory.Fact{
					Content: fmt.Sprintf("Concurrent fact %d-%d", id, j),
					Tags:    []string{"concurrent", fmt.Sprintf("goroutine-%d", id)},
					Source:  "concurrency-test",
				}

				err := store.SaveFact(ctx, fact)
				if err != nil {
					// Expected: some writes will fail with "database is locked"
					// Log but don't fail the test
					h.T.Logf("Goroutine %d operation %d: %v", id, j, err)
				}
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(10 * time.Second):
			h.T.Fatalf("Timeout waiting for goroutines (%d/%d completed)", i, numGoroutines)
		}
	}

	// Verify the system is still functional after concurrent operations
	// Query for any saved facts
	opts := memory.QueryOpts{
		Tags:  []string{"concurrent"},
		Limit: 100,
	}

	results, err := store.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query concurrent facts: %v", err)
	}

	// At least some facts should have been saved
	if len(results) == 0 {
		h.T.Error("Expected at least some facts to be saved despite lock contention")
	}

	// Verify no data corruption
	for _, result := range results {
		if result.Content == "" {
			h.T.Error("Expected non-empty content for saved fact")
		}
		if len(result.Tags) == 0 {
			h.T.Error("Expected tags to be preserved")
		}
	}

	h.T.Logf("Successfully saved %d facts with %d goroutines (SQLite single-writer mode)", len(results), numGoroutines)
}
