// Package memory provides integration tests for vector search.
package memory

import (
	"context"
	"testing"

	"github.com/artemis-project/artemis/internal/memory"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestVectorSearchPlaceholder is a placeholder for vector search tests.
// Note: Full vector search testing requires:
// 1. chromem-go dependency
// 2. Embedding API key (Voyage AI, OpenAI, etc.)
// 3. Vector store initialization and indexing
func TestVectorSearchPlaceholder(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	h.T.Log("Vector search testing requires chromem-go dependency and embedding API")
	h.T.Log("Expected behavior:")
	h.T.Log("  - VectorStore.AddFact() should embed and index facts")
	h.T.Log("  - VectorStore.QueryFacts() should return semantically similar results")
	h.T.Log("  - Results should include similarity scores (0.0 - 1.0)")
	h.T.Log("  - Hybrid search: FTS5 for exact match + vector for semantic")
}

// TestVectorHybridSearch documents the expected hybrid search behavior.
func TestVectorHybridSearch(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_vector_hybrid.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save facts
	facts := []*memory.Fact{
		{Content: "JavaScript is used for web development", Tags: []string{"js", "web"}, Source: "test"},
		{Content: "TypeScript adds static typing to JavaScript", Tags: []string{"ts", "web"}, Source: "test"},
		{Content: "React is a popular JavaScript library for UI", Tags: []string{"react", "web"}, Source: "test"},
	}

	for _, fact := range facts {
		err = store.SaveFact(ctx, fact)
		if err != nil {
			h.T.Fatalf("Failed to save fact: %v", err)
		}
	}

	// Hybrid search: FTS5 for exact match + vector for semantic
	opts := memory.QueryOpts{
		Query: "JavaScript frameworks",
		Limit: 10,
	}

	results, err := store.QueryFacts(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query facts: %v", err)
	}

	// Results should include keyword matches (FTS5)
	// With vector store, would also include semantically similar results
	h.T.Logf("Hybrid search results: %d", len(results))
	for i, result := range results {
		h.T.Logf("  %d. %s", i+1, result.Content)
	}

	h.T.Log("Note: With vector store enabled, results would be ranked by semantic similarity")
}

// TestVectorSessionSearch documents session search behavior.
func TestVectorSessionSearch(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	dbPath := h.TempDir + "/test_vector_sessions.db"
	store, err := memory.NewSQLiteStore(dbPath)
	if err != nil {
		h.T.Fatalf("Failed to create SQLite store: %v", err)
	}

	ctx := context.Background()

	// Save sessions
	sessions := []*memory.SessionSummary{
		{
			SessionID:    "session-1",
			Summary:      "Fixed bug in authentication flow",
			FilesTouched: []string{"auth.go"},
			FactsLearned: 1,
			Outcome:      "success",
			MessageCount: 5,
		},
		{
			SessionID:    "session-2",
			Summary:      "Implemented user registration feature",
			FilesTouched: []string{"register.go"},
			FactsLearned: 2,
			Outcome:      "success",
			MessageCount: 8,
		},
		{
			SessionID:    "session-3",
			Summary:      "Added password reset functionality",
			FilesTouched: []string{"reset.go"},
			FactsLearned: 1,
			Outcome:      "success",
			MessageCount: 6,
		},
	}

	for _, session := range sessions {
		err = store.SaveSession(ctx, session)
		if err != nil {
			h.T.Fatalf("Failed to save session: %v", err)
		}
	}

	// Search for sessions
	opts := memory.QueryOpts{
		Query: "user authentication",
		Limit: 10,
	}

	results, err := store.QuerySessions(ctx, opts)
	if err != nil {
		h.T.Fatalf("Failed to query sessions: %v", err)
	}

	// Should find sessions matching the query (FTS5)
	// With vector store, would find semantically similar sessions too
	h.T.Logf("Session search: found %d results", len(results))
	for i, result := range results {
		h.T.Logf("  %d. %s", i+1, result.Summary)
	}

	h.T.Log("Note: With vector store, sessions would be ranked by semantic similarity")
}

// TestVectorEmbeddingAPI documents embedding API requirements.
func TestVectorEmbeddingAPI(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	h.T.Log("Vector store requires embedding API for text vectorization")
	h.T.Log("Supported providers:")
	h.T.Log("  - Voyage AI (voyage-law-2, voyage-code-2)")
	h.T.Log("  - OpenAI (text-embedding-3-small, text-embedding-3-large)")
	h.T.Log("  - Cohere (embed-english-v3.0)")
	h.T.Log("")
	h.T.Log("Implementation:")
	h.T.Log("  1. Install chromem-go: go get github.com/philippgille/chromem-go")
	h.T.Log("  2. Set embedding API key in environment")
	h.T.Log("  3. Initialize VectorStore with embedding function")
	h.T.Log("  4. Facts are automatically embedded on SaveFact()")
	h.T.Log("  5. QueryFacts() uses hybrid search (FTS5 + vector)")
}
