package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 3

// SQLiteStore implements MemoryStore using pure-Go SQLite with FTS5.
// Phase 2 adds optional VectorStore for hybrid search.
// Phase 3 adds optional RepoMapStore for codebase structure indexing.
type SQLiteStore struct {
	db           *sql.DB
	dbPath       string
	vectorStore  *VectorStore  // Phase 2: optional vector search
	repoMapStore *RepoMapStore // Phase 3: optional repo-map indexing
}

// NewSQLiteStore opens (or creates) a SQLite memory database at the given path.
// It runs schema migrations automatically.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("memory: create dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("memory: open db: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: set WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: enable foreign keys: %w", err)
	}

	store := &SQLiteStore{db: db, dbPath: dbPath}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory: migration: %w", err)
	}

	return store, nil
}

// SetVectorStore attaches a VectorStore for hybrid search capability.
// When set, SaveFact/SaveSession/SaveDecision auto-embed content,
// and QueryFacts uses RRF hybrid search (FTS5 + vector).
func (s *SQLiteStore) SetVectorStore(vs *VectorStore) {
	s.vectorStore = vs
}

// SetRepoMapStore attaches a RepoMapStore for codebase structure indexing.
func (s *SQLiteStore) SetRepoMapStore(rm *RepoMapStore) {
	s.repoMapStore = rm
}

// RepoMapStore returns the attached repo-map store (may be nil).
func (s *SQLiteStore) GetRepoMapStore() *RepoMapStore {
	return s.repoMapStore
}

// DB returns the underlying database connection for shared use.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// migrate runs schema migrations up to the current version.
// Designed for incremental upgrades as Artemis evolves.
func (s *SQLiteStore) migrate() error {
	// Create version tracking table
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}

	var version int
	err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return err
	}

	if version < 1 {
		if err := s.migrateV1(); err != nil {
			return fmt.Errorf("v1: %w", err)
		}
	}


	if version < 2 {
		if err := s.migrateV2(); err != nil {
			return fmt.Errorf("v2: %w", err)
		}
	}

	// Phase 3: repo-map symbols table
	if version < 3 {
		if err := s.migrateV3(); err != nil {
			return fmt.Errorf("v3: %w", err)
		}
	}

	// Future migrations go here:
	// if version < 4 { s.migrateV4() }

	return nil
}

// migrateV1 creates the initial schema: facts, sessions, files, decisions + FTS5.
func (s *SQLiteStore) migrateV1() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Semantic facts (learned project knowledge)
		`CREATE TABLE IF NOT EXISTS semantic_facts (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			content     TEXT NOT NULL,
			tags        TEXT DEFAULT '',
			source      TEXT DEFAULT '',
			use_count   INTEGER DEFAULT 0,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// FTS5 index for semantic facts
		`CREATE VIRTUAL TABLE IF NOT EXISTS facts_fts USING fts5(
			content,
			tags,
			content=semantic_facts,
			content_rowid=id
		)`,

		// Triggers to keep FTS5 in sync with semantic_facts
		`CREATE TRIGGER IF NOT EXISTS facts_ai AFTER INSERT ON semantic_facts BEGIN
			INSERT INTO facts_fts(rowid, content, tags) VALUES (new.id, new.content, new.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS facts_ad AFTER DELETE ON semantic_facts BEGIN
			INSERT INTO facts_fts(facts_fts, rowid, content, tags) VALUES('delete', old.id, old.content, old.tags);
		END`,
		`CREATE TRIGGER IF NOT EXISTS facts_au AFTER UPDATE ON semantic_facts BEGIN
			INSERT INTO facts_fts(facts_fts, rowid, content, tags) VALUES('delete', old.id, old.content, old.tags);
			INSERT INTO facts_fts(rowid, content, tags) VALUES (new.id, new.content, new.tags);
		END`,

		// Session summaries (episodic memory)
		`CREATE TABLE IF NOT EXISTS session_summaries (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id    TEXT UNIQUE NOT NULL,
			summary       TEXT NOT NULL,
			files_touched TEXT DEFAULT '[]',
			facts_learned INTEGER DEFAULT 0,
			outcome       TEXT DEFAULT 'success',
			message_count INTEGER DEFAULT 0,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// FTS5 index for session summaries
		`CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
			summary,
			content=session_summaries,
			content_rowid=id
		)`,

		`CREATE TRIGGER IF NOT EXISTS sessions_ai AFTER INSERT ON session_summaries BEGIN
			INSERT INTO sessions_fts(rowid, summary) VALUES (new.id, new.summary);
		END`,
		`CREATE TRIGGER IF NOT EXISTS sessions_ad AFTER DELETE ON session_summaries BEGIN
			INSERT INTO sessions_fts(sessions_fts, rowid, summary) VALUES('delete', old.id, old.summary);
		END`,
		`CREATE TRIGGER IF NOT EXISTS sessions_au AFTER UPDATE ON session_summaries BEGIN
			INSERT INTO sessions_fts(sessions_fts, rowid, summary) VALUES('delete', old.id, old.summary);
			INSERT INTO sessions_fts(rowid, summary) VALUES (new.id, new.summary);
		END`,

		// File tracking
		`CREATE TABLE IF NOT EXISTS file_index (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			path         TEXT UNIQUE NOT NULL,
			last_role    TEXT DEFAULT '',
			change_count INTEGER DEFAULT 0,
			last_seen    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Architectural decisions
		`CREATE TABLE IF NOT EXISTS decisions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			decision   TEXT NOT NULL,
			rationale  TEXT DEFAULT '',
			context    TEXT DEFAULT '',
			tags       TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// FTS5 index for decisions
		`CREATE VIRTUAL TABLE IF NOT EXISTS decisions_fts USING fts5(
			decision,
			rationale,
			content=decisions,
			content_rowid=id
		)`,

		`CREATE TRIGGER IF NOT EXISTS decisions_ai AFTER INSERT ON decisions BEGIN
			INSERT INTO decisions_fts(rowid, decision, rationale) VALUES (new.id, new.decision, new.rationale);
		END`,
		`CREATE TRIGGER IF NOT EXISTS decisions_ad AFTER DELETE ON decisions BEGIN
			INSERT INTO decisions_fts(decisions_fts, rowid, decision, rationale) VALUES('delete', old.id, old.decision, old.rationale);
		END`,
		`CREATE TRIGGER IF NOT EXISTS decisions_au AFTER UPDATE ON decisions BEGIN
			INSERT INTO decisions_fts(decisions_fts, rowid, decision, rationale) VALUES('delete', old.id, old.decision, old.rationale);
			INSERT INTO decisions_fts(rowid, decision, rationale) VALUES (new.id, new.decision, new.rationale);
		END`,

		// Record schema version
		`INSERT INTO schema_version (version) VALUES (1)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// migrateV2 adds the session_messages table for full conversation history storage.
func (s *SQLiteStore) migrateV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Session messages (full conversation replay)
		`CREATE TABLE IF NOT EXISTS session_messages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT NOT NULL,
			role        TEXT NOT NULL,
			content     TEXT NOT NULL,
			agent_role  TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Index for efficient session lookup ordered by time
		`CREATE INDEX IF NOT EXISTS idx_messages_session ON session_messages(session_id, created_at)`,

		// Record schema version
		`INSERT INTO schema_version (version) VALUES (2)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// migrateV3 adds the repo_symbols table for Phase 3 repo-map indexing.
func (s *SQLiteStore) migrateV3() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Code symbols extracted from source files
		`CREATE TABLE IF NOT EXISTS repo_symbols (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path   TEXT NOT NULL,
			name        TEXT NOT NULL,
			kind        TEXT NOT NULL,
			line        INTEGER NOT NULL,
			signature   TEXT DEFAULT '',
			scope       TEXT DEFAULT '',
			exported    BOOLEAN DEFAULT 0,
			file_hash   TEXT DEFAULT '',
			updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(file_path, name, kind, line)
		)`,

		// Indexes for efficient queries
		`CREATE INDEX IF NOT EXISTS idx_symbols_file ON repo_symbols(file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_symbols_name ON repo_symbols(name)`,
		`CREATE INDEX IF NOT EXISTS idx_symbols_kind ON repo_symbols(kind)`,

		// FTS5 index for symbol search
		`CREATE VIRTUAL TABLE IF NOT EXISTS repo_symbols_fts USING fts5(
			name, signature, file_path,
			content='repo_symbols', content_rowid='id'
		)`,

		// FTS5 sync triggers
		`CREATE TRIGGER IF NOT EXISTS symbols_ai AFTER INSERT ON repo_symbols BEGIN
			INSERT INTO repo_symbols_fts(rowid, name, signature, file_path)
			VALUES (new.id, new.name, new.signature, new.file_path);
		END`,
		`CREATE TRIGGER IF NOT EXISTS symbols_ad AFTER DELETE ON repo_symbols BEGIN
			INSERT INTO repo_symbols_fts(repo_symbols_fts, rowid, name, signature, file_path)
			VALUES('delete', old.id, old.name, old.signature, old.file_path);
		END`,
		`CREATE TRIGGER IF NOT EXISTS symbols_au AFTER UPDATE ON repo_symbols BEGIN
			INSERT INTO repo_symbols_fts(repo_symbols_fts, rowid, name, signature, file_path)
			VALUES('delete', old.id, old.name, old.signature, old.file_path);
			INSERT INTO repo_symbols_fts(rowid, name, signature, file_path)
			VALUES (new.id, new.name, new.signature, new.file_path);
		END`,

		// Record schema version
		`INSERT INTO schema_version (version) VALUES (3)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// --- Facts (Semantic Memory) ---

func (s *SQLiteStore) SaveFact(ctx context.Context, fact *Fact) error {
	tags := strings.Join(fact.Tags, ",")
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO semantic_facts (content, tags, source, use_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		fact.Content, tags, fact.Source, fact.UseCount,
	)
	if err != nil {
		return fmt.Errorf("memory: save fact: %w", err)
	}
	fact.ID, _ = result.LastInsertId()

	// Phase 2: auto-embed in vector store (async, fire-and-forget)
	if s.vectorStore != nil {
		go func(id int64, content string, tags []string) {
			bgCtx := context.Background()
			_ = s.vectorStore.AddFact(bgCtx, id, content, tags)
		}(fact.ID, fact.Content, fact.Tags)
	}

	return nil
}

func (s *SQLiteStore) QueryFacts(ctx context.Context, opts QueryOpts) ([]Fact, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	// Phase 2: hybrid search when vectorStore is available and query is non-empty
	if s.vectorStore != nil && opts.Query != "" {
		results, err := s.queryFactsHybrid(ctx, opts, limit)
		if err == nil {
			return results, nil
		}
		// Fall through to FTS5 on vector error
	}

	return s.queryFactsFTS(ctx, opts, limit)
}

// queryFactsFTS performs FTS5-only fact search (Phase 1 behavior).
func (s *SQLiteStore) queryFactsFTS(ctx context.Context, opts QueryOpts, limit int) ([]Fact, error) {
	var rows *sql.Rows
	var err error

	if opts.Query != "" {
		query := buildFTSQuery(opts.Query)
		if len(opts.Tags) > 0 {
			tagFilter := buildTagFilter(opts.Tags)
			rows, err = s.db.QueryContext(ctx,
				`SELECT f.id, f.content, f.tags, f.source, f.use_count, f.created_at, f.updated_at
				 FROM semantic_facts f
				 JOIN facts_fts fts ON f.id = fts.rowid
				 WHERE facts_fts MATCH ? AND (`+tagFilter+`)
				 ORDER BY rank, f.use_count DESC
				 LIMIT ?`,
				query, limit,
			)
		} else {
			rows, err = s.db.QueryContext(ctx,
				`SELECT f.id, f.content, f.tags, f.source, f.use_count, f.created_at, f.updated_at
				 FROM semantic_facts f
				 JOIN facts_fts fts ON f.id = fts.rowid
				 WHERE facts_fts MATCH ?
				 ORDER BY rank, f.use_count DESC
				 LIMIT ?`,
				query, limit,
			)
		}
	} else if len(opts.Tags) > 0 {
		tagFilter := buildTagFilter(opts.Tags)
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, content, tags, source, use_count, created_at, updated_at
			 FROM semantic_facts
			 WHERE `+tagFilter+`
			 ORDER BY use_count DESC, updated_at DESC
			 LIMIT ?`,
			limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, content, tags, source, use_count, created_at, updated_at
			 FROM semantic_facts
			 ORDER BY use_count DESC, updated_at DESC
			 LIMIT ?`,
			limit,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("memory: query facts: %w", err)
	}
	defer rows.Close()

	return scanFacts(rows)
}

func (s *SQLiteStore) IncrementFactUsage(ctx context.Context, factID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE semantic_facts SET use_count = use_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		factID,
	)
	return err
}

func (s *SQLiteStore) MergeFact(ctx context.Context, factID int64, newContent string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE semantic_facts SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newContent, factID,
	)
	return err
}

func (s *SQLiteStore) DeleteFact(ctx context.Context, factID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM semantic_facts WHERE id = ?`, factID,
	)
	return err
}

// --- Sessions (Episodic Memory) ---

func (s *SQLiteStore) SaveSession(ctx context.Context, summary *SessionSummary) error {
	filesJSON, _ := json.Marshal(summary.FilesTouched)
	result, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO session_summaries
		 (session_id, summary, files_touched, facts_learned, outcome, message_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		summary.SessionID, summary.Summary, string(filesJSON),
		summary.FactsLearned, summary.Outcome, summary.MessageCount,
	)
	if err != nil {
		return fmt.Errorf("memory: save session: %w", err)
	}
	summary.ID, _ = result.LastInsertId()

	// Phase 2: auto-embed in vector store (async)
	if s.vectorStore != nil {
		go func(sid, sum string) {
			bgCtx := context.Background()
			_ = s.vectorStore.AddSession(bgCtx, sid, sum)
		}(summary.SessionID, summary.Summary)
	}

	return nil
}

func (s *SQLiteStore) QuerySessions(ctx context.Context, opts QueryOpts) ([]SessionSummary, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	var rows *sql.Rows
	var err error

	if opts.Query != "" {
		query := buildFTSQuery(opts.Query)
		rows, err = s.db.QueryContext(ctx,
			`SELECT s.id, s.session_id, s.summary, s.files_touched, s.facts_learned,
			        s.outcome, s.message_count, s.created_at
			 FROM session_summaries s
			 JOIN sessions_fts fts ON s.id = fts.rowid
			 WHERE sessions_fts MATCH ?
			 ORDER BY rank
			 LIMIT ?`,
			query, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, session_id, summary, files_touched, facts_learned,
			        outcome, message_count, created_at
			 FROM session_summaries
			 ORDER BY created_at DESC
			 LIMIT ?`,
			limit,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("memory: query sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

func (s *SQLiteStore) GetSession(ctx context.Context, sessionID string) (*SessionSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, summary, files_touched, facts_learned,
		        outcome, message_count, created_at
		 FROM session_summaries WHERE session_id = ?`,
		sessionID,
	)

	var ss SessionSummary
	var filesJSON string
	err := row.Scan(&ss.ID, &ss.SessionID, &ss.Summary, &filesJSON,
		&ss.FactsLearned, &ss.Outcome, &ss.MessageCount, &ss.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: get session: %w", err)
	}
	json.Unmarshal([]byte(filesJSON), &ss.FilesTouched)
	return &ss, nil
}

// --- Files ---

func (s *SQLiteStore) TrackFile(ctx context.Context, record *FileRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO file_index (path, last_role, change_count, last_seen)
		 VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		 ON CONFLICT(path) DO UPDATE SET
		   last_role = excluded.last_role,
		   change_count = change_count + 1,
		   last_seen = CURRENT_TIMESTAMP`,
		record.Path, record.LastRole,
	)
	return err
}

func (s *SQLiteStore) GetTrackedFiles(ctx context.Context) ([]FileRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, path, last_role, change_count, last_seen
		 FROM file_index ORDER BY last_seen DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		if err := rows.Scan(&f.ID, &f.Path, &f.LastRole, &f.ChangeCount, &f.LastSeen); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// --- Decisions ---

func (s *SQLiteStore) SaveDecision(ctx context.Context, decision *Decision) error {
	tags := strings.Join(decision.Tags, ",")
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO decisions (decision, rationale, context, tags, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		decision.Decision, decision.Rationale, decision.Context, tags,
	)
	if err != nil {
		return fmt.Errorf("memory: save decision: %w", err)
	}
	decision.ID, _ = result.LastInsertId()

	// Phase 2: auto-embed in vector store (async)
	if s.vectorStore != nil {
		go func(id int64, content string, tags []string) {
			bgCtx := context.Background()
			combined := content
			if decision.Rationale != "" {
				combined += " — " + decision.Rationale
			}
			_ = s.vectorStore.AddDecision(bgCtx, id, combined, tags)
		}(decision.ID, decision.Decision, decision.Tags)
	}

	return nil
}

func (s *SQLiteStore) QueryDecisions(ctx context.Context, opts QueryOpts) ([]Decision, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	var rows *sql.Rows
	var err error

	if opts.Query != "" {
		query := buildFTSQuery(opts.Query)
		rows, err = s.db.QueryContext(ctx,
			`SELECT d.id, d.decision, d.rationale, d.context, d.tags, d.created_at
			 FROM decisions d
			 JOIN decisions_fts fts ON d.id = fts.rowid
			 WHERE decisions_fts MATCH ?
			 ORDER BY rank
			 LIMIT ?`,
			query, limit,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, decision, rationale, context, tags, created_at
			 FROM decisions ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("memory: query decisions: %w", err)
	}
	defer rows.Close()

	var decisions []Decision
	for rows.Next() {
		var d Decision
		var tags string
		if err := rows.Scan(&d.ID, &d.Decision, &d.Rationale, &d.Context, &tags, &d.CreatedAt); err != nil {
			return nil, err
		}
		if tags != "" {
			d.Tags = strings.Split(tags, ",")
		}
		decisions = append(decisions, d)
	}
	return decisions, rows.Err()
}

// --- Messages (Session History) ---

func (s *SQLiteStore) SaveMessage(ctx context.Context, msg *SessionMessage) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO session_messages (session_id, role, content, agent_role, created_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		msg.SessionID, msg.Role, msg.Content, msg.AgentRole,
	)
	if err != nil {
		return fmt.Errorf("memory: save message: %w", err)
	}
	msg.ID, _ = result.LastInsertId()
	return nil
}

func (s *SQLiteStore) GetSessionMessages(ctx context.Context, sessionID string) ([]SessionMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, agent_role, created_at
		 FROM session_messages WHERE session_id = ?
		 ORDER BY created_at ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get session messages: %w", err)
	}
	defer rows.Close()

	var messages []SessionMessage
	for rows.Next() {
		var m SessionMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.AgentRole, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ListSessions returns recent sessions with summary info, ordered by recency.
// Unlike QuerySessions which searches summaries, this returns all sessions that
// have stored messages (including those without consolidation summaries).
func (s *SQLiteStore) ListSessions(ctx context.Context, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	// First check session_summaries (consolidated sessions),
	// then union with session_messages for non-consolidated sessions.
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, COALESCE(summary, ''), message_count, created_at
		 FROM (
		   -- Consolidated sessions (have summaries)
		   SELECT session_id, summary, message_count, created_at
		   FROM session_summaries
		   UNION ALL
		   -- Non-consolidated sessions (messages only, no summary yet)
		   SELECT m.session_id, '' AS summary, COUNT(*) AS message_count, MIN(m.created_at) AS created_at
		   FROM session_messages m
		   WHERE m.session_id NOT IN (SELECT session_id FROM session_summaries)
		   GROUP BY m.session_id
		 ) ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.SessionID, &ss.Summary, &ss.MessageCount, &ss.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// --- Maintenance ---

// DecayFacts removes or deprioritizes stale, low-use facts.
// Returns the number of facts deleted.
func (s *SQLiteStore) DecayFacts(ctx context.Context, maxAge time.Duration, minUseCount int) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM semantic_facts
		 WHERE updated_at < ? AND use_count < ?`,
		cutoff, minUseCount,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) Stats(ctx context.Context) (*StoreStats, error) {
	stats := &StoreStats{}

	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM semantic_facts").Scan(&stats.FactCount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM session_summaries").Scan(&stats.SessionCount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM file_index").Scan(&stats.FileCount)
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions").Scan(&stats.DecisionCount)

	// Get DB file size
	if info, err := os.Stat(s.dbPath); err == nil {
		stats.DBSizeBytes = info.Size()
	}

	return stats, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Helpers ---

// buildFTSQuery converts a natural query into FTS5 syntax.
// Wraps individual terms with * for prefix matching.
func buildFTSQuery(query string) string {
	words := strings.Fields(query)
	if len(words) == 0 {
		return query
	}
	// Use OR-based search with prefix matching for flexibility
	var parts []string
	for _, w := range words {
		// Escape double quotes in the word
		w = strings.ReplaceAll(w, "\"", "")
		if w != "" {
			parts = append(parts, w+"*")
		}
	}
	return strings.Join(parts, " OR ")
}

// buildTagFilter creates a SQL WHERE clause fragment for tag filtering.
// Tags are stored as comma-separated in the tags column.
func buildTagFilter(tags []string) string {
	var clauses []string
	for _, tag := range tags {
		// Match tag anywhere in the comma-separated list
		clauses = append(clauses, fmt.Sprintf("(',' || tags || ',' LIKE '%%,%s,%%')", tag))
	}
	return strings.Join(clauses, " OR ")
}

// scanFacts reads Fact records from sql.Rows.
func scanFacts(rows *sql.Rows) ([]Fact, error) {
	var facts []Fact
	for rows.Next() {
		var f Fact
		var tags string
		if err := rows.Scan(&f.ID, &f.Content, &tags, &f.Source, &f.UseCount, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if tags != "" {
			f.Tags = strings.Split(tags, ",")
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// scanSessions reads SessionSummary records from sql.Rows.
func scanSessions(rows *sql.Rows) ([]SessionSummary, error) {
	var sessions []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		var filesJSON string
		if err := rows.Scan(&ss.ID, &ss.SessionID, &ss.Summary, &filesJSON,
			&ss.FactsLearned, &ss.Outcome, &ss.MessageCount, &ss.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(filesJSON), &ss.FilesTouched)
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- Phase 2: Hybrid Search (RRF) ---

// queryFactsHybrid combines FTS5 and vector search results using
// Reciprocal Rank Fusion (RRF) for improved retrieval quality.
// Weights: Vector 0.7, FTS5 0.3 (semantic similarity prioritized).
func (s *SQLiteStore) queryFactsHybrid(ctx context.Context, opts QueryOpts, limit int) ([]Fact, error) {
	// 1. FTS5 results (keyword matching)
	ftsFacts, ftsErr := s.queryFactsFTS(ctx, opts, limit*2)
	if ftsErr != nil {
		ftsFacts = nil // continue with vector-only if FTS fails
	}

	// 2. Vector results (semantic similarity)
	vecResults, vecErr := s.vectorStore.QueryFacts(ctx, opts.Query, limit*2)
	if vecErr != nil {
		// Vector search failed — fall back to FTS5 only
		if ftsFacts != nil {
			if len(ftsFacts) > limit {
				return ftsFacts[:limit], nil
			}
			return ftsFacts, nil
		}
		return nil, fmt.Errorf("memory: both FTS5 and vector search failed: fts=%v, vec=%v", ftsErr, vecErr)
	}

	// 3. RRF fusion (Reciprocal Rank Fusion)
	const k = 60.0
	const ftsWeight = 0.3
	const vecWeight = 0.7

	type scoredFact struct {
		fact     Fact
		rrfScore float64
	}

	scores := make(map[int64]*scoredFact)

	// Score FTS5 results
	for i, f := range ftsFacts {
		scores[f.ID] = &scoredFact{
			fact:     f,
			rrfScore: ftsWeight / (k + float64(i+1)),
		}
	}

	// Score vector results
	for i, vr := range vecResults {
		factID := parseFactIDFromVectorID(vr.ID)
		if factID <= 0 {
			continue
		}

		if sf, ok := scores[factID]; ok {
			// Fact exists in both FTS5 and vector — boost score
			sf.rrfScore += vecWeight / (k + float64(i+1))
		} else {
			// Vector-only result — load full fact from DB
			fact, err := s.getFactByID(ctx, factID)
			if err == nil && fact != nil {
				scores[factID] = &scoredFact{
					fact:     *fact,
					rrfScore: vecWeight / (k + float64(i+1)),
				}
			}
		}
	}

	// Sort by RRF score descending
	sorted := make([]scoredFact, 0, len(scores))
	for _, sf := range scores {
		sorted = append(sorted, *sf)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].rrfScore > sorted[j].rrfScore
	})

	// Apply tag filter to vector-only results (FTS already filtered)
	results := make([]Fact, 0, limit)
	for _, sf := range sorted {
		if len(results) >= limit {
			break
		}
		// If tags are specified, verify the fact matches
		if len(opts.Tags) > 0 && !factMatchesTags(sf.fact, opts.Tags) {
			continue
		}
		results = append(results, sf.fact)
	}

	return results, nil
}

// getFactByID loads a single fact from the database by ID.
func (s *SQLiteStore) getFactByID(ctx context.Context, factID int64) (*Fact, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, content, tags, source, use_count, created_at, updated_at
		 FROM semantic_facts WHERE id = ?`,
		factID,
	)

	var f Fact
	var tags string
	err := row.Scan(&f.ID, &f.Content, &tags, &f.Source, &f.UseCount, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if tags != "" {
		f.Tags = strings.Split(tags, ",")
	}
	return &f, nil
}

// parseFactIDFromVectorID extracts the numeric fact ID from a vector document ID.
// e.g., "fact_123" -> 123
func parseFactIDFromVectorID(vectorID string) int64 {
	if !strings.HasPrefix(vectorID, "fact_") {
		return 0
	}
	id, err := strconv.ParseInt(vectorID[5:], 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// factMatchesTags checks if a fact has at least one of the specified tags.
func factMatchesTags(f Fact, tags []string) bool {
	for _, ft := range f.Tags {
		for _, t := range tags {
			if ft == t {
				return true
			}
		}
	}
	return false
}
