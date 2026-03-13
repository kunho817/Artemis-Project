package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/artemis-project/artemis/internal/llm"
)

func newTestWindow(t *testing.T, maxRecent int) *HistoryWindow {
	t.Helper()
	tc, err := llm.NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}
	return NewHistoryWindow(maxRecent, tc)
}

func TestHistoryWindow_Add(t *testing.T) {
	hw := newTestWindow(t, 5)
	hw.Add(llm.Message{Role: "user", Content: "Hello"})
	hw.Add(llm.Message{Role: "assistant", Content: "Hi!"})

	if hw.Len() != 2 {
		t.Errorf("Len() = %d, want 2", hw.Len())
	}
}

func TestHistoryWindow_Recent_UnderMax(t *testing.T) {
	hw := newTestWindow(t, 5)
	hw.Add(llm.Message{Role: "user", Content: "msg1"})
	hw.Add(llm.Message{Role: "assistant", Content: "msg2"})

	recent := hw.Recent()
	if len(recent) != 2 {
		t.Errorf("Recent() len = %d, want 2", len(recent))
	}
}

func TestHistoryWindow_Recent_OverMax(t *testing.T) {
	hw := newTestWindow(t, 3)
	for i := 0; i < 10; i++ {
		hw.Add(llm.Message{Role: "user", Content: "msg"})
	}

	recent := hw.Recent()
	if len(recent) != 3 {
		t.Errorf("Recent() len = %d, want 3", len(recent))
	}
}

func TestHistoryWindow_NeedsCompaction(t *testing.T) {
	hw := newTestWindow(t, 3)

	// Under maxRecent — no compaction needed
	hw.Add(llm.Message{Role: "user", Content: "msg"})
	if hw.NeedsCompaction() {
		t.Error("should not need compaction with 1 message")
	}

	// Add enough to exceed maxRecent
	for i := 0; i < 5; i++ {
		hw.Add(llm.Message{Role: "user", Content: "msg"})
	}
	if !hw.NeedsCompaction() {
		t.Error("should need compaction with 6 messages and maxRecent=3")
	}
}

func TestHistoryWindow_Compact(t *testing.T) {
	hw := newTestWindow(t, 3)

	// Add 8 messages
	for i := 0; i < 8; i++ {
		hw.Add(llm.Message{Role: "user", Content: "Message number"})
	}

	// Mock compaction function
	compacted := false
	mockCompact := func(ctx context.Context, existing string, msgs []llm.Message) (string, error) {
		compacted = true
		return "Summary of older messages.", nil
	}

	err := hw.Compact(context.Background(), mockCompact)
	if err != nil {
		t.Fatalf("Compact error: %v", err)
	}
	if !compacted {
		t.Error("compact function was not called")
	}
	if hw.Summarized() == "" {
		t.Error("Summarized() is empty after compaction")
	}
	if hw.Summarized() != "Summary of older messages." {
		t.Errorf("unexpected summary: %s", hw.Summarized())
	}
	// After compaction, messages should be trimmed to maxRecent
	if hw.Len() != 3 {
		t.Errorf("after compaction Len() = %d, want 3", hw.Len())
	}
	if hw.NeedsCompaction() {
		t.Error("should not need compaction after compact")
	}
}

func TestHistoryWindow_CompactWithExistingSummary(t *testing.T) {
	hw := newTestWindow(t, 2)
	hw.Add(llm.Message{Role: "user", Content: "A"})
	hw.Add(llm.Message{Role: "assistant", Content: "B"})
	hw.Add(llm.Message{Role: "user", Content: "C"})
	hw.Add(llm.Message{Role: "assistant", Content: "D"})
	hw.Add(llm.Message{Role: "user", Content: "E"})

	// First compaction
	first := func(ctx context.Context, existing string, msgs []llm.Message) (string, error) {
		return "First batch summary.", nil
	}
	_ = hw.Compact(context.Background(), first)

	// Add more messages
	hw.Add(llm.Message{Role: "assistant", Content: "F"})
	hw.Add(llm.Message{Role: "user", Content: "G"})
	hw.Add(llm.Message{Role: "assistant", Content: "H"})
	hw.Add(llm.Message{Role: "user", Content: "I"})

	// Second compaction — should include existing summary
	var receivedExisting string
	second := func(ctx context.Context, existing string, msgs []llm.Message) (string, error) {
		receivedExisting = existing
		return "Combined summary.", nil
	}
	_ = hw.Compact(context.Background(), second)

	if receivedExisting != "First batch summary." {
		t.Errorf("existing summary = %q, want 'First batch summary.'", receivedExisting)
	}
	if hw.Summarized() != "Combined summary." {
		t.Errorf("final summary = %q", hw.Summarized())
	}
}

func TestHistoryWindow_CompactNilFunc(t *testing.T) {
	hw := newTestWindow(t, 3)
	for i := 0; i < 10; i++ {
		hw.Add(llm.Message{Role: "user", Content: "msg"})
	}
	// nil CompactFunc should be a no-op
	err := hw.Compact(context.Background(), nil)
	if err != nil {
		t.Errorf("Compact(nil) should not error: %v", err)
	}
}

func TestHistoryWindow_RecentFormatted(t *testing.T) {
	hw := newTestWindow(t, 5)
	hw.Add(llm.Message{Role: "user", Content: "Hello"})
	hw.Add(llm.Message{Role: "assistant", Content: "Hi there!"})

	formatted := hw.RecentFormatted()
	if !strings.Contains(formatted, "user: Hello") {
		t.Errorf("missing user message in formatted: %s", formatted)
	}
	if !strings.Contains(formatted, "assistant: Hi there!") {
		t.Errorf("missing assistant message in formatted: %s", formatted)
	}
}

func TestHistoryWindow_SetMessages(t *testing.T) {
	hw := newTestWindow(t, 3)
	hw.Add(llm.Message{Role: "user", Content: "old"})

	newMsgs := []llm.Message{
		{Role: "user", Content: "loaded1"},
		{Role: "assistant", Content: "loaded2"},
	}
	hw.SetMessages(newMsgs)

	if hw.Len() != 2 {
		t.Errorf("after SetMessages Len() = %d, want 2", hw.Len())
	}
	all := hw.All()
	if all[0].Content != "loaded1" {
		t.Errorf("first message = %q, want 'loaded1'", all[0].Content)
	}
}

func TestHistoryWindow_Clear(t *testing.T) {
	hw := newTestWindow(t, 3)
	hw.Add(llm.Message{Role: "user", Content: "msg"})
	hw.Clear()

	if hw.Len() != 0 {
		t.Errorf("after Clear() Len() = %d, want 0", hw.Len())
	}
	if hw.Summarized() != "" {
		t.Error("after Clear() Summarized() should be empty")
	}
}

func TestHistoryWindow_TotalTokens(t *testing.T) {
	hw := newTestWindow(t, 5)
	hw.Add(llm.Message{Role: "user", Content: "Hello world"})

	tokens := hw.TotalTokens()
	if tokens <= 0 {
		t.Errorf("TotalTokens() = %d, want > 0", tokens)
	}
}

func TestBuildCompactionPrompt_NoExisting(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "What is Go?"},
		{Role: "assistant", Content: "Go is a programming language."},
	}
	prompt := BuildCompactionPrompt("", msgs)

	if !strings.Contains(prompt, "CONVERSATION TO SUMMARIZE") {
		t.Error("missing CONVERSATION TO SUMMARIZE header")
	}
	if strings.Contains(prompt, "PREVIOUS SUMMARY") {
		t.Error("should not have PREVIOUS SUMMARY when no existing")
	}
}

func TestBuildCompactionPrompt_WithExisting(t *testing.T) {
	prompt := BuildCompactionPrompt("Old summary.", []llm.Message{
		{Role: "user", Content: "New question"},
	})

	if !strings.Contains(prompt, "PREVIOUS SUMMARY") {
		t.Error("missing PREVIOUS SUMMARY header")
	}
	if !strings.Contains(prompt, "Old summary.") {
		t.Error("missing existing summary content")
	}
	if !strings.Contains(prompt, "NEW MESSAGES TO INCORPORATE") {
		t.Error("missing NEW MESSAGES header")
	}
}
