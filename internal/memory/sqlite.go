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

	ghsync "github.com/artemis-project/artemis/internal/github"
	"github.com/artemis-project/artemis/internal/state"
	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 6

// SQLiteStore implements MemoryStore using pure-Go SQLite with FTS5.
// Phase 2 adds optional VectorStore for hybrid search.
// Phase 3 adds optional RepoMapStore for codebase structure indexing.
type SQLiteStore struct {
	db           *sql.DB
	dbPath       string
	vectorStore  VectorSearcher // Phase 2: optional vector search
	repoMapStore *RepoMapStore  // Phase 3: optional repo-map indexing
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
func (s *SQLiteStore) SetVectorStore(vs VectorSearcher) {
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

	// Phase 4: GitHub issue tracker tables
	if version < 4 {
		if err := s.migrateV4(); err != nil {
			return fmt.Errorf("v4: %w", err)
		}
	}

	// Phase 5: Session hierarchy + pipeline runs
	if version < 5 {
		if err := s.migrateV5(); err != nil {
			return fmt.Errorf("v5: %w", err)
		}
	}

	// Phase C-5: Step checkpoints for pipeline resume
	if version < 6 {
		if err := s.migrateV6(); err != nil {
			return fmt.Errorf("v6: %w", err)
		}
	}

	// Future migrations go here:
	// if version < 7 { s.migrateV7() }
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

// migrateV4 adds GitHub issue tracker tables and FTS5 index.
func (s *SQLiteStore) migrateV4() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS github_issues (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			issue_number    INTEGER NOT NULL UNIQUE,
			title           TEXT NOT NULL,
			body            TEXT DEFAULT '',
			state           TEXT DEFAULT 'open',
			labels          TEXT DEFAULT '[]',
			author          TEXT DEFAULT '',
			triage_status   TEXT DEFAULT 'pending',
			triage_reason   TEXT DEFAULT '',
			pr_number       INTEGER DEFAULT 0,
			created_at      DATETIME,
			updated_at      DATETIME,
			synced_at       DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS github_comments (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			issue_number    INTEGER NOT NULL,
			comment_id      INTEGER NOT NULL,
			body            TEXT DEFAULT '',
			author          TEXT DEFAULT '',
			created_at      DATETIME,
			UNIQUE(comment_id)
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS github_issues_fts USING fts5(
			title, body, content=github_issues, content_rowid=id
		)`,

		`CREATE TRIGGER IF NOT EXISTS github_issues_ai AFTER INSERT ON github_issues BEGIN
			INSERT INTO github_issues_fts(rowid, title, body)
			VALUES (new.id, new.title, new.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS github_issues_ad AFTER DELETE ON github_issues BEGIN
			INSERT INTO github_issues_fts(github_issues_fts, rowid, title, body)
			VALUES('delete', old.id, old.title, old.body);
		END`,
		`CREATE TRIGGER IF NOT EXISTS github_issues_au AFTER UPDATE ON github_issues BEGIN
			INSERT INTO github_issues_fts(github_issues_fts, rowid, title, body)
			VALUES('delete', old.id, old.title, old.body);
			INSERT INTO github_issues_fts(rowid, title, body)
			VALUES (new.id, new.title, new.body);
		END`,

		`INSERT INTO schema_version (version) VALUES (4)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// migrateV5 adds session hierarchy support: parent_session_id, pipeline_runs table,
// and pipeline_run_id on session_messages.
func (s *SQLiteStore) migrateV5() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Add parent_session_id to session_summaries
		`ALTER TABLE session_summaries ADD COLUMN parent_session_id TEXT DEFAULT ''`,

		// Pipeline runs — tracks each pipeline/background execution within a session
		`CREATE TABLE IF NOT EXISTS pipeline_runs (
			id             TEXT PRIMARY KEY,
			session_id     TEXT NOT NULL,
			parent_run_id  TEXT DEFAULT '',
			intent         TEXT DEFAULT '',
			plan_json      TEXT DEFAULT '',
			status         TEXT DEFAULT 'running',
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at   DATETIME
		)`,

		`CREATE INDEX IF NOT EXISTS idx_pipeline_runs_session ON pipeline_runs(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pipeline_runs_parent ON pipeline_runs(parent_run_id)`,

		// Add pipeline_run_id to session_messages
		`ALTER TABLE session_messages ADD COLUMN pipeline_run_id TEXT DEFAULT ''`,

		`INSERT INTO schema_version (version) VALUES (5)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// migrateV6 adds step_checkpoints table for pipeline resume support (Phase C-5).
func (s *SQLiteStore) migrateV6() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		// Step checkpoints — captures outcome of each step for resume
		`CREATE TABLE IF NOT EXISTS step_checkpoints (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id            TEXT NOT NULL,
			step_index        INTEGER NOT NULL,
			step_name         TEXT NOT NULL,
			status            TEXT NOT NULL,
			artifacts_json    TEXT DEFAULT '[]',
			agent_results_json TEXT DEFAULT '{}',
			created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (run_id) REFERENCES pipeline_runs(id)
		)`,

		`CREATE INDEX IF NOT EXISTS idx_checkpoints_run ON step_checkpoints(run_id, step_index)`,

		`INSERT INTO schema_version (version) VALUES (6)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}

	return tx.Commit()
}

// --- Checkpoint Store (Phase C-5) ---

// SaveCheckpoint persists a step checkpoint after step completion.
func (s *SQLiteStore) SaveCheckpoint(ctx context.Context, cp *state.StepCheckpoint) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO step_checkpoints (run_id, step_index, step_name, status, artifacts_json, agent_results_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		cp.RunID, cp.StepIndex, cp.StepName, cp.Status, cp.ArtifactsJSON, cp.AgentResultsJSON,
	)
	if err != nil {
		return fmt.Errorf("memory: save checkpoint: %w", err)
	}
	cp.ID, _ = result.LastInsertId()
	return nil
}

// GetCheckpoints returns all checkpoints for a pipeline run, ordered by step index.
func (s *SQLiteStore) GetCheckpoints(ctx context.Context, runID string) ([]state.StepCheckpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, step_index, step_name, status, COALESCE(artifacts_json, '[]'),
		        COALESCE(agent_results_json, '{}'), created_at
		 FROM step_checkpoints WHERE run_id = ?
		 ORDER BY step_index ASC`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []state.StepCheckpoint
	for rows.Next() {
		var cp state.StepCheckpoint
		if err := rows.Scan(&cp.ID, &cp.RunID, &cp.StepIndex, &cp.StepName,
			&cp.Status, &cp.ArtifactsJSON, &cp.AgentResultsJSON, &cp.CreatedAt); err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, rows.Err()
}

// GetIncompleteRuns returns pipeline runs that are still in "running" status.
// These represent interrupted pipelines that may be resumable.
func (s *SQLiteStore) GetIncompleteRuns(ctx context.Context, sessionID string) ([]state.IncompleteRun, error) {
	// Find all "running" pipeline runs (those that were interrupted before completion)
	query := `SELECT pr.id, pr.session_id, COALESCE(pr.intent, ''), COALESCE(pr.plan_json, ''),
	                 pr.status, pr.created_at,
	                 COALESCE(MAX(sc.step_index), -1) AS last_step_index,
	                 COALESCE(MAX(CASE WHEN sc.step_index = (SELECT MAX(step_index) FROM step_checkpoints WHERE run_id = pr.id) THEN sc.step_name END), '') AS last_step_name
	          FROM pipeline_runs pr
	          LEFT JOIN step_checkpoints sc ON sc.run_id = pr.id
	          WHERE pr.status = 'running'`

	var args []interface{}
	if sessionID != "" {
		query += ` AND pr.session_id = ?`
		args = append(args, sessionID)
	}
	query += ` GROUP BY pr.id ORDER BY pr.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("memory: get incomplete runs: %w", err)
	}
	defer rows.Close()

	var runs []state.IncompleteRun
	for rows.Next() {
		var r state.IncompleteRun
		if err := rows.Scan(&r.RunID, &r.SessionID, &r.Intent, &r.PlanJSON,
			&r.Status, &r.CreatedAt, &r.LastStepIndex, &r.LastStepName); err != nil {
			return nil, err
		}
		// Derive total steps from plan JSON
		if r.PlanJSON != "" {
			var planData struct {
				Steps []json.RawMessage `json:"steps"`
			}
			if err := json.Unmarshal([]byte(r.PlanJSON), &planData); err == nil {
				r.TotalSteps = len(planData.Steps)
			}
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// DeleteCheckpoints removes all checkpoints for a pipeline run (cleanup).
func (s *SQLiteStore) DeleteCheckpoints(ctx context.Context, runID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM step_checkpoints WHERE run_id = ?`, runID,
	)
	if err != nil {
		return fmt.Errorf("memory: delete checkpoints: %w", err)
	}
	return nil
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
		 (session_id, parent_session_id, summary, files_touched, facts_learned, outcome, message_count, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		summary.SessionID, summary.ParentSessionID, summary.Summary, string(filesJSON),
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
		`INSERT INTO session_messages (session_id, role, content, agent_role, pipeline_run_id, created_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		msg.SessionID, msg.Role, msg.Content, msg.AgentRole, msg.PipelineRunID,
	)
	if err != nil {
		return fmt.Errorf("memory: save message: %w", err)
	}
	msg.ID, _ = result.LastInsertId()
	return nil
}

func (s *SQLiteStore) GetSessionMessages(ctx context.Context, sessionID string) ([]SessionMessage, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, agent_role, COALESCE(pipeline_run_id, ''), created_at
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
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.AgentRole, &m.PipelineRunID, &m.CreatedAt); err != nil {
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

	// Union consolidated sessions (with summaries) + non-consolidated (messages only).
	// Phase 5: includes parent_session_id from consolidated sessions.
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, COALESCE(summary, ''), COALESCE(parent_session_id, ''), message_count, created_at
		 FROM (
		   SELECT session_id, summary, parent_session_id, message_count, created_at
		   FROM session_summaries
		   UNION ALL
		   SELECT m.session_id, '' AS summary, '' AS parent_session_id, COUNT(*) AS message_count, MIN(m.created_at) AS created_at
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
		if err := rows.Scan(&ss.SessionID, &ss.Summary, &ss.ParentSessionID, &ss.MessageCount, &ss.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// --- Pipeline Runs (Session Hierarchy) ---

// SavePipelineRun persists a new pipeline run record.
func (s *SQLiteStore) SavePipelineRun(ctx context.Context, run *PipelineRun) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pipeline_runs (id, session_id, parent_run_id, intent, plan_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		run.ID, run.SessionID, run.ParentRunID, run.Intent, run.PlanJSON, run.Status,
	)
	if err != nil {
		return fmt.Errorf("memory: save pipeline run: %w", err)
	}
	return nil
}

// UpdatePipelineRun updates the status (and completed_at) of an existing pipeline run.
func (s *SQLiteStore) UpdatePipelineRun(ctx context.Context, runID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pipeline_runs SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, runID,
	)
	if err != nil {
		return fmt.Errorf("memory: update pipeline run %q: %w", runID, err)
	}
	return nil
}

// GetPipelineRuns returns all pipeline runs for a session, ordered by creation time.
func (s *SQLiteStore) GetPipelineRuns(ctx context.Context, sessionID string) ([]PipelineRun, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, COALESCE(parent_run_id, ''), COALESCE(intent, ''),
		        COALESCE(plan_json, ''), status, created_at, completed_at
		 FROM pipeline_runs WHERE session_id = ?
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get pipeline runs: %w", err)
	}
	defer rows.Close()

	var runs []PipelineRun
	for rows.Next() {
		var r PipelineRun
		var completedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.SessionID, &r.ParentRunID, &r.Intent,
			&r.PlanJSON, &r.Status, &r.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			r.CompletedAt = completedAt.Time
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetChildSessions returns sessions whose parent_session_id matches the given parent.
func (s *SQLiteStore) GetChildSessions(ctx context.Context, parentSessionID string) ([]SessionSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, COALESCE(summary, ''), COALESCE(parent_session_id, ''), message_count, created_at
		 FROM session_summaries WHERE parent_session_id = ?
		 ORDER BY created_at DESC`,
		parentSessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: get child sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.SessionID, &ss.Summary, &ss.ParentSessionID, &ss.MessageCount, &ss.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, ss)
	}
	return sessions, rows.Err()
}

// --- GitHub Issues (Issue Tracker) ---

// UpsertIssue inserts or replaces a GitHub issue row by issue_number.
func (s *SQLiteStore) UpsertIssue(ctx context.Context, issue *ghsync.StoredIssue) error {
	if issue == nil {
		return fmt.Errorf("memory: upsert issue: nil issue")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO github_issues
		 (issue_number, title, body, state, labels, author, triage_status, triage_reason, pr_number, created_at, updated_at, synced_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.IssueNumber,
		issue.Title,
		issue.Body,
		issue.State,
		issue.Labels,
		issue.Author,
		string(issue.TriageStatus),
		issue.TriageReason,
		issue.PRNumber,
		issue.CreatedAt,
		issue.UpdatedAt,
		issue.SyncedAt,
	)
	if err != nil {
		return fmt.Errorf("memory: upsert issue #%d: %w", issue.IssueNumber, err)
	}

	return nil
}

// GetIssue returns a stored issue by issue number.
func (s *SQLiteStore) GetIssue(ctx context.Context, issueNumber int) (*ghsync.StoredIssue, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, issue_number, title, body, state, labels, author, triage_status, triage_reason,
		        pr_number, created_at, updated_at, synced_at
		 FROM github_issues
		 WHERE issue_number = ?`,
		issueNumber,
	)

	var issue ghsync.StoredIssue
	var triageStatus string
	err := row.Scan(
		&issue.ID,
		&issue.IssueNumber,
		&issue.Title,
		&issue.Body,
		&issue.State,
		&issue.Labels,
		&issue.Author,
		&triageStatus,
		&issue.TriageReason,
		&issue.PRNumber,
		&issue.CreatedAt,
		&issue.UpdatedAt,
		&issue.SyncedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory: get issue #%d: %w", issueNumber, err)
	}

	issue.TriageStatus = ghsync.TriageStatus(triageStatus)
	return &issue, nil
}

// ListIssues returns issues filtered by triage status.
func (s *SQLiteStore) ListIssues(ctx context.Context, status ghsync.TriageStatus, limit int) ([]*ghsync.StoredIssue, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, issue_number, title, body, state, labels, author, triage_status, triage_reason,
		        pr_number, created_at, updated_at, synced_at
		 FROM github_issues
		 WHERE triage_status = ?
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		string(status),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list issues by status %q: %w", status, err)
	}
	defer rows.Close()

	issues := make([]*ghsync.StoredIssue, 0, limit)
	for rows.Next() {
		issue, err := scanStoredIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: list issues by status %q: %w", status, err)
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: list issues by status %q rows: %w", status, err)
	}

	return issues, nil
}

// ListAllIssues returns issues ordered by update time.
func (s *SQLiteStore) ListAllIssues(ctx context.Context, limit int) ([]*ghsync.StoredIssue, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, issue_number, title, body, state, labels, author, triage_status, triage_reason,
		        pr_number, created_at, updated_at, synced_at
		 FROM github_issues
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory: list all issues: %w", err)
	}
	defer rows.Close()

	issues := make([]*ghsync.StoredIssue, 0, limit)
	for rows.Next() {
		issue, err := scanStoredIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("memory: list all issues: %w", err)
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("memory: list all issues rows: %w", err)
	}

	return issues, nil
}

// UpdateTriageStatus updates triage status and reason for an issue.
func (s *SQLiteStore) UpdateTriageStatus(ctx context.Context, issueNumber int, status ghsync.TriageStatus, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_issues
		 SET triage_status = ?, triage_reason = ?, synced_at = CURRENT_TIMESTAMP
		 WHERE issue_number = ?`,
		string(status), reason, issueNumber,
	)
	if err != nil {
		return fmt.Errorf("memory: update triage status for issue #%d: %w", issueNumber, err)
	}
	return nil
}

// UpdatePRNumber updates linked PR number for an issue.
func (s *SQLiteStore) UpdatePRNumber(ctx context.Context, issueNumber int, prNumber int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_issues
		 SET pr_number = ?, synced_at = CURRENT_TIMESTAMP
		 WHERE issue_number = ?`,
		prNumber, issueNumber,
	)
	if err != nil {
		return fmt.Errorf("memory: update PR number for issue #%d: %w", issueNumber, err)
	}
	return nil
}

// SaveComment persists a GitHub issue comment.
func (s *SQLiteStore) SaveComment(ctx context.Context, issueNumber int, commentID int64, body, author string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO github_comments
		 (issue_number, comment_id, body, author, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		issueNumber, commentID, body, author, createdAt,
	)
	if err != nil {
		return fmt.Errorf("memory: save comment %d for issue #%d: %w", commentID, issueNumber, err)
	}
	return nil
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

func scanStoredIssue(rows *sql.Rows) (*ghsync.StoredIssue, error) {
	issue := &ghsync.StoredIssue{}
	var triageStatus string
	if err := rows.Scan(
		&issue.ID,
		&issue.IssueNumber,
		&issue.Title,
		&issue.Body,
		&issue.State,
		&issue.Labels,
		&issue.Author,
		&triageStatus,
		&issue.TriageReason,
		&issue.PRNumber,
		&issue.CreatedAt,
		&issue.UpdatedAt,
		&issue.SyncedAt,
	); err != nil {
		return nil, err
	}
	issue.TriageStatus = ghsync.TriageStatus(triageStatus)
	return issue, nil
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
