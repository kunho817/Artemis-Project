package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/artemis-project/artemis/internal/llm"
)

// ArchiveEntry represents a single JSONL line in the cold archive.
// Each entry is a raw conversation message or a consolidation event.
type ArchiveEntry struct {
	Type      string    `json:"type"` // "message", "consolidation", "fact", "decision"
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`

	// For type="message"
	Role      string `json:"role,omitempty"`       // user, assistant, system
	Content   string `json:"content,omitempty"`    // message content
	AgentRole string `json:"agent_role,omitempty"` // which agent produced this

	// For type="consolidation"
	Summary       string   `json:"summary,omitempty"`
	FilesTouched  []string `json:"files_touched,omitempty"`
	FactCount     int      `json:"fact_count,omitempty"`
	DecisionCount int      `json:"decision_count,omitempty"`

	// For type="fact"
	FactContent string   `json:"fact_content,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	// For type="decision"
	Decision  string `json:"decision,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

// Archiver writes raw session data to JSONL files in the cold storage tier.
// Each session gets its own file: {archivePath}/{sessionID}.jsonl
// Files are append-only — safe for concurrent writes within a session.
type Archiver struct {
	baseDir string
}

// NewArchiver creates an archiver that writes to the given base directory.
// Creates the directory if it doesn't exist.
func NewArchiver(baseDir string) (*Archiver, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("archive: create dir: %w", err)
	}
	return &Archiver{baseDir: baseDir}, nil
}

// ArchiveMessages writes raw conversation messages to the session's JSONL file.
// This preserves the full conversation before consolidation summarizes it.
func (a *Archiver) ArchiveMessages(sessionID string, messages []llm.Message) error {
	f, err := a.openSessionFile(sessionID)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	for _, m := range messages {
		entry := ArchiveEntry{
			Type:      "message",
			SessionID: sessionID,
			Timestamp: time.Now(),
			Role:      m.Role,
			Content:   m.Content,
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("archive: encode message: %w", err)
		}
	}
	return nil
}

// ArchiveConsolidation writes the consolidation result to the session's JSONL file.
// Called after LLM consolidation completes.
func (a *Archiver) ArchiveConsolidation(result *ConsolidateResult) error {
	if result == nil {
		return nil
	}

	f, err := a.openSessionFile(result.SessionID)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)

	// Write the consolidation summary entry
	summary := ArchiveEntry{
		Type:          "consolidation",
		SessionID:     result.SessionID,
		Timestamp:     time.Now(),
		Summary:       result.Summary,
		FilesTouched:  result.FilesTouched,
		FactCount:     len(result.Facts),
		DecisionCount: len(result.Decisions),
	}
	if err := enc.Encode(summary); err != nil {
		return fmt.Errorf("archive: encode consolidation: %w", err)
	}

	// Write individual facts
	for _, f := range result.Facts {
		entry := ArchiveEntry{
			Type:        "fact",
			SessionID:   result.SessionID,
			Timestamp:   time.Now(),
			FactContent: f.Content,
			Tags:        f.Tags,
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("archive: encode fact: %w", err)
		}
	}

	// Write individual decisions
	for _, d := range result.Decisions {
		entry := ArchiveEntry{
			Type:      "decision",
			SessionID: result.SessionID,
			Timestamp: time.Now(),
			Decision:  d.Decision,
			Rationale: d.Rationale,
		}
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("archive: encode decision: %w", err)
		}
	}

	return nil
}

// openSessionFile opens or creates the JSONL file for a session (append mode).
func (a *Archiver) openSessionFile(sessionID string) (*os.File, error) {
	// Sanitize sessionID for filesystem safety
	safe := sanitizeFileName(sessionID)
	path := filepath.Join(a.baseDir, safe+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("archive: open %s: %w", path, err)
	}
	return f, nil
}

// sanitizeFileName removes characters that are invalid in file names.
func sanitizeFileName(name string) string {
	var result []byte
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			result = append(result, c)
		default:
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "unknown"
	}
	return string(result)
}
