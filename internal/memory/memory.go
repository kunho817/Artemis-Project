package memory

import (
	"context"
	"time"
)

// VectorSearcher defines the interface for vector-based similarity search.
// Phase 2 uses chromem-go; future migration to sqlite-vec only requires
// a new implementation of this interface.
type VectorSearcher interface {
	// --- Indexing ---
	AddFact(ctx context.Context, factID int64, content string, tags []string) error
	AddSession(ctx context.Context, sessionID, summary string) error
	AddDecision(ctx context.Context, decisionID int64, content string, tags []string) error

	// --- Search ---
	QueryFacts(ctx context.Context, query string, limit int) ([]VectorResult, error)
	QuerySessions(ctx context.Context, query string, limit int) ([]VectorResult, error)
	QueryDecisions(ctx context.Context, query string, limit int) ([]VectorResult, error)

	// --- Utility ---
	SimilarityScore(ctx context.Context, text1, text2 string) (float32, error)
	Close() error
}

// MemoryStore defines the interface for persistent memory.
// Designed to be backend-agnostic: Phase 1 uses SQLite+FTS5,
// Phase 2 can add vector search, Phase 3 can add repo-map indexing.
type MemoryStore interface {
	// --- Facts (Semantic Memory) ---
	SaveFact(ctx context.Context, fact *Fact) error
	QueryFacts(ctx context.Context, opts QueryOpts) ([]Fact, error)
	IncrementFactUsage(ctx context.Context, factID int64) error
	MergeFact(ctx context.Context, factID int64, newContent string) error
	DeleteFact(ctx context.Context, factID int64) error

	// --- Sessions (Episodic Memory) ---
	SaveSession(ctx context.Context, summary *SessionSummary) error
	QuerySessions(ctx context.Context, opts QueryOpts) ([]SessionSummary, error)
	GetSession(ctx context.Context, sessionID string) (*SessionSummary, error)

	// --- Files ---
	TrackFile(ctx context.Context, record *FileRecord) error
	GetTrackedFiles(ctx context.Context) ([]FileRecord, error)

	// --- Decisions ---
	SaveDecision(ctx context.Context, decision *Decision) error
	QueryDecisions(ctx context.Context, opts QueryOpts) ([]Decision, error)

	// --- Messages (Session History) ---
	SaveMessage(ctx context.Context, msg *SessionMessage) error
	GetSessionMessages(ctx context.Context, sessionID string) ([]SessionMessage, error)
	ListSessions(ctx context.Context, limit int) ([]SessionSummary, error)

	// --- Maintenance ---
	DecayFacts(ctx context.Context, maxAge time.Duration, minUseCount int) (int64, error)
	Stats(ctx context.Context) (*StoreStats, error)

	// --- Pipeline Runs (Session Hierarchy) ---
	SavePipelineRun(ctx context.Context, run *PipelineRun) error
	UpdatePipelineRun(ctx context.Context, runID, status string) error
	GetPipelineRuns(ctx context.Context, sessionID string) ([]PipelineRun, error)
	GetChildSessions(ctx context.Context, parentSessionID string) ([]SessionSummary, error)

	// --- Lifecycle ---
	Close() error
	}

// SessionMessage represents a single message in a conversation session.
type SessionMessage struct {
	ID            int64     `json:"id"`
	SessionID     string    `json:"session_id"`
	Role          string    `json:"role"`    // "user", "assistant", "system"
	Content       string    `json:"content"`
	AgentRole     string    `json:"agent_role"` // which agent produced this (empty for user)
	PipelineRunID string    `json:"pipeline_run_id,omitempty"` // Phase 5: links message to pipeline run
	CreatedAt     time.Time `json:"created_at"`
}

// QueryOpts configures memory retrieval.
// Designed for extensibility: Phase 2 adds Embedding field for vector search.
type QueryOpts struct {
	Query  string        // FTS5 text query (Phase 1) / semantic query (Phase 2)
	Tags   []string      // Role-based filtering (e.g., ["arch", "code"])
	Limit  int           // Max results (0 = default 20)
	MaxAge time.Duration // Only results newer than this (0 = no limit)
	MinUse int           // Minimum use_count threshold

	// Phase 2 (vector search)
	Hybrid bool // combine FTS5 + vector results via RRF (requires VectorStore)
}

// Fact represents a learned project fact (Semantic Memory).
type Fact struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"` // "이 프로젝트는 Go 1.24 사용"
	Tags      []string  `json:"tags"`    // ["lang", "build"]
	Source    string    `json:"source"`  // "session:abc123"
	UseCount  int       `json:"use_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SessionSummary represents a consolidated session record (Episodic Memory).
type SessionSummary struct {
	ID              int64     `json:"id"`
	SessionID       string    `json:"session_id"`
	ParentSessionID string    `json:"parent_session_id,omitempty"` // Phase 5: session hierarchy
	Summary         string    `json:"summary"`
	FilesTouched    []string  `json:"files_touched"`
	FactsLearned    int       `json:"facts_learned"` // count of new facts extracted
	Outcome         string    `json:"outcome"`       // "success" / "partial" / "failed"
	MessageCount    int       `json:"message_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// FileRecord tracks files the agent has interacted with.
type FileRecord struct {
	ID          int64     `json:"id"`
	Path        string    `json:"path"`
	LastRole    string    `json:"last_role"` // last agent role that modified it
	ChangeCount int       `json:"change_count"`
	LastSeen    time.Time `json:"last_seen"`
}

// Decision records an architectural or design decision.
type Decision struct {
	ID        int64     `json:"id"`
	Decision  string    `json:"decision"`
	Rationale string    `json:"rationale"`
	Context   string    `json:"context"` // situation when decision was made
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

// StoreStats provides memory store statistics.
type StoreStats struct {
	FactCount     int   `json:"fact_count"`
	SessionCount  int   `json:"session_count"`
	FileCount     int   `json:"file_count"`
	DecisionCount int   `json:"decision_count"`
	DBSizeBytes   int64 `json:"db_size_bytes"`
}

// TokenBudget defines per-section token limits for prompt construction.
// Prevents context window overflow regardless of memory store size.
type TokenBudget struct {
	SystemPrompt   int // ~2000
	ProjectFacts   int // ~2000
	SessionContext int // ~3000
	FileContext    int // ~8000
	Conversation   int // remainder
}

// DefaultTokenBudget returns conservative defaults for prompt construction.
func DefaultTokenBudget() TokenBudget {
	return TokenBudget{
		SystemPrompt:   2000,
		ProjectFacts:   2000,
		SessionContext: 3000,
		FileContext:    8000,
		Conversation:   0, // filled dynamically based on model max
	}
}

// RoleTagMap maps agent roles to their relevant memory tags.
// Used for role-based fact filtering.
var RoleTagMap = map[string][]string{
	"orchestrator": nil, // access to all tags
	"planner":      {"plan", "requirement", "scope", "arch"},
	"analyzer":     {"plan", "requirement", "scope", "analysis"},
	"searcher":     {"code", "impl", "api", "search"},
	"explorer":     {"code", "impl", "arch", "pattern"},
	"architect":    {"arch", "design", "pattern", "decision"},
	"coder":        {"code", "impl", "api", "bug"},
	"designer":     {"ui", "ux", "style", "design"},
	"engineer":     {"code", "impl", "infra", "build"},
	"qa":           {"test", "bug", "quality", "verify"},
	"tester":       {"test", "bug", "quality", "verify"},
	"scout":       {"code", "impl", "arch", "pattern", "search"},
	"consultant":  nil, // access to all tags (like orchestrator)
}

// --- Phase 3: Repo-Map ---

// SymbolKind represents the type of a code symbol.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindType      SymbolKind = "type"
	KindInterface SymbolKind = "interface"
	KindStruct    SymbolKind = "struct"
	KindClass     SymbolKind = "class"
	KindModule    SymbolKind = "module"
	KindConst     SymbolKind = "const"
	KindVar       SymbolKind = "var"
	KindField     SymbolKind = "field"
	KindProperty  SymbolKind = "property"
	KindPackage   SymbolKind = "package"
	KindUnknown   SymbolKind = "unknown"
)

// Symbol represents a code symbol extracted from a source file.
type Symbol struct {
	Name      string     `json:"name"`      // e.g., "BuildPromptWithContext"
	Kind      SymbolKind `json:"kind"`      // function, type, interface, ...
	FilePath  string     `json:"file_path"` // relative path from project root
	Line      int        `json:"line"`      // 1-based line number
	Signature string     `json:"signature"` // full signature (if available)
	Scope     string     `json:"scope"`     // parent scope (e.g., "BaseAgent" for methods)
	Exported  bool       `json:"exported"`  // true if publicly visible
}

// RepoMapRoleFilter defines which symbols each agent role should see.
// Used for role-based prompt injection filtering.
var RepoMapRoleFilter = map[string]func(Symbol) bool{
	"orchestrator": func(s Symbol) bool { return s.Exported },
	"planner":      func(s Symbol) bool { return s.Exported },
	"analyzer":     func(s Symbol) bool { return true },
	"searcher":     func(s Symbol) bool { return true },
	"explorer":     func(s Symbol) bool { return true },
	"architect":    func(s Symbol) bool { return s.Exported },
	"coder":        func(s Symbol) bool { return true },
	"designer":     func(s Symbol) bool { return s.Exported },
	"engineer":     func(s Symbol) bool { return true },
	"qa":           func(s Symbol) bool { return s.Kind == KindFunction || s.Kind == KindMethod },
	"tester":       func(s Symbol) bool { return s.Kind == KindFunction || s.Kind == KindMethod },
	"scout":      func(s Symbol) bool { return true },
	"consultant": func(s Symbol) bool { return s.Exported },
}


// --- Phase 5: Session Hierarchy ---

// PipelineRun tracks a single pipeline execution within a session.
// Multiple pipeline runs can occur within one TUI session.
// Background tasks create child runs linked via ParentRunID.
type PipelineRun struct {
	ID          string    `json:"id"`            // "run_<nano>"
	SessionID   string    `json:"session_id"`    // owning TUI session
	ParentRunID string    `json:"parent_run_id,omitempty"` // parent run (for background tasks)
	Intent      string    `json:"intent,omitempty"`        // trivial/conversational/exploratory/complex
	PlanJSON    string    `json:"plan_json,omitempty"`     // ExecutionPlan JSON (debug)
	Status      string    `json:"status"`        // running/completed/failed
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}