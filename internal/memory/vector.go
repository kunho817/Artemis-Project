package memory

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

// VectorResult holds a single vector similarity search result.
type VectorResult struct {
	ID         string            // document ID (e.g., "fact_123")
	Content    string            // original text content
	Metadata   map[string]string // stored metadata
	Similarity float32           // cosine similarity score (0.0 - 1.0)
}

// VectorStore wraps chromem-go for embedding storage and similarity search.
// It maintains separate collections for facts, sessions, and decisions.
// Uses Voyage AI's input_type distinction:
//   - "document" EmbeddingFunc for AddDocuments (indexing)
//   - "query" EmbeddingFunc for QueryEmbedding (searching)
type VectorStore struct {
	db             *chromem.DB
	docEmbedFunc   chromem.EmbeddingFunc // input_type: "document"
	queryEmbedFunc chromem.EmbeddingFunc // input_type: "query"

	facts      *chromem.Collection
	sessions   *chromem.Collection
	decisions  *chromem.Collection
	codeChunks *chromem.Collection // semantic code search

	mu sync.RWMutex
}

// NewVectorStore creates a new VectorStore with persistent storage.
// storePath is the directory for chromem-go data (e.g., ~/.artemis/vectors/).
// apiKey and model are for the Voyage AI embedding API.
func NewVectorStore(storePath, apiKey, model string) (*VectorStore, error) {
	db, err := chromem.NewPersistentDB(storePath, true) // gzip compression
	if err != nil {
		return nil, fmt.Errorf("vectorstore: create db: %w", err)
	}

	docFunc := NewVoyageEmbeddingFunc(apiKey, model, "document")
	queryFunc := NewVoyageEmbeddingFunc(apiKey, model, "query")

	vs := &VectorStore{
		db:             db,
		docEmbedFunc:   docFunc,
		queryEmbedFunc: queryFunc,
	}

	// Initialize collections
	if err := vs.initCollections(); err != nil {
		return nil, fmt.Errorf("vectorstore: init collections: %w", err)
	}

	return vs, nil
}

// initCollections creates or retrieves the three collections.
func (vs *VectorStore) initCollections() error {
	var err error

	vs.facts, err = vs.db.GetOrCreateCollection("facts", nil, vs.docEmbedFunc)
	if err != nil {
		return fmt.Errorf("facts collection: %w", err)
	}

	vs.sessions, err = vs.db.GetOrCreateCollection("sessions", nil, vs.docEmbedFunc)
	if err != nil {
		return fmt.Errorf("sessions collection: %w", err)
	}

	vs.decisions, err = vs.db.GetOrCreateCollection("decisions", nil, vs.docEmbedFunc)
	if err != nil {
		return fmt.Errorf("decisions collection: %w", err)
	}

	vs.codeChunks, err = vs.db.GetOrCreateCollection("code_chunks", nil, vs.docEmbedFunc)
	if err != nil {
		return fmt.Errorf("code_chunks collection: %w", err)
	}

	return nil
}

// --- Add Methods (use input_type: "document") ---

// AddFact embeds and stores a fact in the vector store.
func (vs *VectorStore) AddFact(ctx context.Context, factID int64, content string, tags []string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	doc := chromem.Document{
		ID:      fmt.Sprintf("fact_%d", factID),
		Content: content,
		Metadata: map[string]string{
			"fact_id": fmt.Sprintf("%d", factID),
			"tags":    strings.Join(tags, ","),
		},
	}

	return vs.facts.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
}

// AddSession embeds and stores a session summary in the vector store.
func (vs *VectorStore) AddSession(ctx context.Context, sessionID, summary string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	doc := chromem.Document{
		ID:      fmt.Sprintf("session_%s", sessionID),
		Content: summary,
		Metadata: map[string]string{
			"session_id": sessionID,
		},
	}

	return vs.sessions.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
}

// AddDecision embeds and stores a decision in the vector store.
func (vs *VectorStore) AddDecision(ctx context.Context, decisionID int64, content string, tags []string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	doc := chromem.Document{
		ID:      fmt.Sprintf("decision_%d", decisionID),
		Content: content,
		Metadata: map[string]string{
			"decision_id": fmt.Sprintf("%d", decisionID),
			"tags":        strings.Join(tags, ","),
		},
	}

	return vs.decisions.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
}

// --- Query Methods (use input_type: "query" via QueryEmbedding) ---

// QueryFacts searches for similar facts using the query text.
// Uses Voyage's "query" input_type for optimal retrieval quality.
func (vs *VectorStore) QueryFacts(ctx context.Context, query string, limit int) ([]VectorResult, error) {
	return vs.queryCollection(ctx, vs.facts, query, limit)
}

// QuerySessions searches for similar session summaries.
func (vs *VectorStore) QuerySessions(ctx context.Context, query string, limit int) ([]VectorResult, error) {
	return vs.queryCollection(ctx, vs.sessions, query, limit)
}

// QueryDecisions searches for similar decisions.
func (vs *VectorStore) QueryDecisions(ctx context.Context, query string, limit int) ([]VectorResult, error) {
	return vs.queryCollection(ctx, vs.decisions, query, limit)
}

// queryCollection performs a similarity search on a collection.
// Uses pre-computed query embedding (input_type: "query") + QueryEmbedding.
func (vs *VectorStore) queryCollection(ctx context.Context, collection *chromem.Collection, query string, limit int) ([]VectorResult, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Generate query embedding with input_type: "query"
	embedding, err := vs.queryEmbedFunc(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: embed query: %w", err)
	}

	// Search using pre-computed embedding
	results, err := collection.QueryEmbedding(ctx, embedding, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("vectorstore: query: %w", err)
	}

	var vResults []VectorResult
	for _, r := range results {
		vResults = append(vResults, VectorResult{
			ID:         r.ID,
			Content:    r.Content,
			Metadata:   r.Metadata,
			Similarity: r.Similarity,
		})
	}

	return vResults, nil
}

// --- Utility Methods ---

// SimilarityScore computes the cosine similarity between two texts.
// Useful for semantic deduplication in consolidation.
// Both texts are embedded with input_type: "document".
func (vs *VectorStore) SimilarityScore(ctx context.Context, text1, text2 string) (float32, error) {
	emb1, err := vs.docEmbedFunc(ctx, text1)
	if err != nil {
		return 0, fmt.Errorf("vectorstore: embed text1: %w", err)
	}

	emb2, err := vs.docEmbedFunc(ctx, text2)
	if err != nil {
		return 0, fmt.Errorf("vectorstore: embed text2: %w", err)
	}

	return cosineSimilarity(emb1, emb2), nil
}

// EmbedText generates an embedding for a single text using the document embedding function.
// Useful for manual embedding operations.
func (vs *VectorStore) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return vs.docEmbedFunc(ctx, text)
}

// Close releases resources. chromem-go persistence is synchronous,
// so no explicit flush is needed.
func (vs *VectorStore) Close() error {
	return nil
}

// cosineSimilarity computes the cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// --- Code Chunk Methods (Semantic Context Engine) ---

// CodeChunk represents an indexed code fragment.
type CodeChunk struct {
	ID       string // file:startLine-endLine
	FilePath string
	Content  string
	Score    float32 // similarity score (only set in search results)
}

// AddCodeChunk embeds and stores a code chunk.
func (vs *VectorStore) AddCodeChunk(ctx context.Context, chunk CodeChunk) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	return vs.codeChunks.AddDocument(ctx, chromem.Document{
		ID:       chunk.ID,
		Content:  chunk.Content,
		Metadata: map[string]string{"file": chunk.FilePath},
	})
}

// QueryCodeChunks finds code chunks semantically similar to a query.
func (vs *VectorStore) QueryCodeChunks(ctx context.Context, query string, limit int) ([]CodeChunk, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	count := vs.codeChunks.Count()
	if count == 0 {
		return nil, nil
	}
	if limit > count {
		limit = count
	}

	// Generate query embedding
	embedding, err := vs.queryEmbedFunc(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("code chunk embed query: %w", err)
	}

	results, err := vs.codeChunks.QueryEmbedding(ctx, embedding, limit, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("code chunk query: %w", err)
	}

	var chunks []CodeChunk
	for _, r := range results {
		chunks = append(chunks, CodeChunk{
			ID:       r.ID,
			FilePath: r.Metadata["file"],
			Content:  r.Content,
			Score:    r.Similarity,
		})
	}
	return chunks, nil
}

// CodeChunkCount returns the number of indexed code chunks.
func (vs *VectorStore) CodeChunkCount() int {
	return vs.codeChunks.Count()
}
