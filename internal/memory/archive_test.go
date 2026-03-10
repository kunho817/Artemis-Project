package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artemis-project/artemis/internal/llm"
)

func TestArchiver_ArchiveMessages(t *testing.T) {
	dir := t.TempDir()

	arc, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver failed: %v", err)
	}

	messages := []llm.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}

	if err := arc.ArchiveMessages("session-123", messages); err != nil {
		t.Fatalf("ArchiveMessages failed: %v", err)
	}

	// Check file exists
	path := filepath.Join(dir, "session-123.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read archive: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var entry1 ArchiveEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry1); err != nil {
		t.Fatalf("failed to parse line 1: %v", err)
	}
	if entry1.Type != "message" || entry1.Role != "user" || entry1.Content != "hello" {
		t.Errorf("unexpected entry1: %+v", entry1)
	}
	if entry1.SessionID != "session-123" {
		t.Errorf("expected session-123, got %q", entry1.SessionID)
	}
}

func TestArchiver_ArchiveConsolidation(t *testing.T) {
	dir := t.TempDir()

	arc, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver failed: %v", err)
	}

	result := &ConsolidateResult{
		SessionID:    "sess-456",
		Summary:      "Did some work",
		FilesTouched: []string{"main.go", "lib.go"},
		Facts: []ExtractedFact{
			{Content: "Uses Go 1.24", Tags: []string{"lang"}},
		},
		Decisions: []ExtractedDecision{
			{Decision: "Use SQLite", Rationale: "Simple and embedded"},
		},
	}

	if err := arc.ArchiveConsolidation(result); err != nil {
		t.Fatalf("ArchiveConsolidation failed: %v", err)
	}

	path := filepath.Join(dir, "sess-456.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read archive: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// 1 consolidation + 1 fact + 1 decision = 3 entries
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	var summary ArchiveEntry
	json.Unmarshal([]byte(lines[0]), &summary)
	if summary.Type != "consolidation" || summary.Summary != "Did some work" {
		t.Errorf("unexpected summary: %+v", summary)
	}
	if summary.FactCount != 1 || summary.DecisionCount != 1 {
		t.Errorf("unexpected counts: facts=%d, decisions=%d", summary.FactCount, summary.DecisionCount)
	}

	var fact ArchiveEntry
	json.Unmarshal([]byte(lines[1]), &fact)
	if fact.Type != "fact" || fact.FactContent != "Uses Go 1.24" {
		t.Errorf("unexpected fact: %+v", fact)
	}

	var decision ArchiveEntry
	json.Unmarshal([]byte(lines[2]), &decision)
	if decision.Type != "decision" || decision.Decision != "Use SQLite" {
		t.Errorf("unexpected decision: %+v", decision)
	}
}

func TestArchiver_AppendMode(t *testing.T) {
	dir := t.TempDir()

	arc, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver failed: %v", err)
	}

	// Write twice to the same session
	arc.ArchiveMessages("sess-1", []llm.Message{{Role: "user", Content: "first"}})
	arc.ArchiveMessages("sess-1", []llm.Message{{Role: "user", Content: "second"}})

	path := filepath.Join(dir, "sess-1.jsonl")
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (appended), got %d", len(lines))
	}
}

func TestArchiver_NilConsolidation(t *testing.T) {
	dir := t.TempDir()
	arc, _ := NewArchiver(dir)

	// Should be a no-op
	if err := arc.ArchiveConsolidation(nil); err != nil {
		t.Errorf("expected nil result to be no-op, got: %v", err)
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := map[string]string{
		"simple":      "simple",
		"with-dash":   "with-dash",
		"with_under":  "with_under",
		"with spaces": "with_spaces",
		"with/slash":  "with_slash",
		"with:colon":  "with_colon",
		"":            "unknown",
	}
	for input, expected := range tests {
		got := sanitizeFileName(input)
		if got != expected {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", input, got, expected)
		}
	}
}
