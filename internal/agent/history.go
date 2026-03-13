package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/artemis-project/artemis/internal/llm"
)

// CompactFunc summarizes conversation messages into a concise summary.
// Parameters:
//   - existingSummary: previously compacted summary (may be empty)
//   - newMessages: messages to incorporate into the summary
//
// Returns the updated summary text.
type CompactFunc func(ctx context.Context, existingSummary string, newMessages []llm.Message) (string, error)

// HistoryWindow manages conversation history with a sliding window.
// Recent messages are kept in full; older messages are compacted into
// an LLM-generated summary to prevent context overflow.
//
// Structure:
//
//	[Summarized older turns]   ← LLM-compressed (P6 in ContextBudget)
//	[Recent turns 1..N]        ← Full text (P2 in ContextBudget)
//	[Current turn]             ← Always included
type HistoryWindow struct {
	mu             sync.RWMutex
	messages       []llm.Message // all messages (recent at end)
	summarized     string        // LLM-compressed older history
	summarizedUpTo int           // number of old messages already summarized
	maxRecent      int           // keep last N messages in full
	counter        *llm.TokenCounter
}

// NewHistoryWindow creates a history window that keeps the last maxRecent
// messages in full and compacts older messages into summaries.
// If counter is nil, token counting is disabled (not recommended).
func NewHistoryWindow(maxRecent int, counter *llm.TokenCounter) *HistoryWindow {
	if maxRecent < 2 {
		maxRecent = 2
	}
	return &HistoryWindow{
		messages:  make([]llm.Message, 0, 32),
		maxRecent: maxRecent,
		counter:   counter,
	}
}

// Add appends a message to the history window.
func (hw *HistoryWindow) Add(msg llm.Message) {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	hw.messages = append(hw.messages, msg)
}

// SetMessages replaces all messages (used when loading session history).
func (hw *HistoryWindow) SetMessages(messages []llm.Message) {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	hw.messages = make([]llm.Message, len(messages))
	copy(hw.messages, messages)
	hw.summarized = ""
	hw.summarizedUpTo = 0
}

// Recent returns the most recent messages (up to maxRecent).
// These are kept in full text for P2 allocation.
func (hw *HistoryWindow) Recent() []llm.Message {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return hw.recentLocked()
}

func (hw *HistoryWindow) recentLocked() []llm.Message {
	if len(hw.messages) <= hw.maxRecent {
		out := make([]llm.Message, len(hw.messages))
		copy(out, hw.messages)
		return out
	}
	start := len(hw.messages) - hw.maxRecent
	out := make([]llm.Message, hw.maxRecent)
	copy(out, hw.messages[start:])
	return out
}

// RecentFormatted returns recent messages as "role: content" lines.
func (hw *HistoryWindow) RecentFormatted() string {
	recent := hw.Recent()
	if len(recent) == 0 {
		return ""
	}
	var lines []string
	for _, m := range recent {
		lines = append(lines, fmt.Sprintf("%s: %s", m.Role, m.Content))
	}
	return strings.Join(lines, "\n")
}

// Summarized returns the LLM-compressed summary of older messages.
// Returns empty string if no compaction has occurred.
func (hw *HistoryWindow) Summarized() string {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return hw.summarized
}

// NeedsCompaction returns true if there are unsummarized old messages
// beyond the recent window.
func (hw *HistoryWindow) NeedsCompaction() bool {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return hw.needsCompactionLocked()
}

func (hw *HistoryWindow) needsCompactionLocked() bool {
	if len(hw.messages) <= hw.maxRecent {
		return false
	}
	// Old messages that haven't been summarized yet
	cutoff := len(hw.messages) - hw.maxRecent
	return cutoff > hw.summarizedUpTo
}

// Compact summarizes older messages using the provided CompactFunc.
// Only messages that haven't been previously summarized are sent.
// This is idempotent — calling it when no compaction is needed is a no-op.
//
// The function is synchronous and may take 2-5 seconds (LLM call).
// Call before prompt construction when history is large.
func (hw *HistoryWindow) Compact(ctx context.Context, fn CompactFunc) error {
	if fn == nil {
		return nil
	}

	hw.mu.Lock()
	if !hw.needsCompactionLocked() {
		hw.mu.Unlock()
		return nil
	}

	// Identify messages to summarize
	cutoff := len(hw.messages) - hw.maxRecent
	toSummarize := make([]llm.Message, cutoff-hw.summarizedUpTo)
	copy(toSummarize, hw.messages[hw.summarizedUpTo:cutoff])
	existingSummary := hw.summarized
	hw.mu.Unlock()

	// Call LLM outside of lock (may take seconds)
	newSummary, err := fn(ctx, existingSummary, toSummarize)
	if err != nil {
		return fmt.Errorf("history compaction failed: %w", err)
	}

	// Update state with new summary
	hw.mu.Lock()
	defer hw.mu.Unlock()

	hw.summarized = newSummary

	// Trim summarized messages from the slice to save memory
	newCutoff := len(hw.messages) - hw.maxRecent
	if newCutoff > 0 {
		hw.messages = hw.messages[newCutoff:]
		hw.summarizedUpTo = 0
	}

	return nil
}

// TotalTokens returns the approximate total tokens across all content
// (summarized + all messages).
func (hw *HistoryWindow) TotalTokens() int {
	hw.mu.RLock()
	defer hw.mu.RUnlock()

	if hw.counter == nil {
		return 0
	}

	total := 0
	if hw.summarized != "" {
		total += hw.counter.Count(hw.summarized)
	}
	for _, m := range hw.messages {
		total += hw.counter.Count(m.Content) + 4 // +4 per message overhead
	}
	return total
}

// RecentTokens returns the token count for recent messages only.
func (hw *HistoryWindow) RecentTokens() int {
	hw.mu.RLock()
	defer hw.mu.RUnlock()

	if hw.counter == nil {
		return 0
	}

	recent := hw.recentLocked()
	total := 0
	for _, m := range recent {
		total += hw.counter.Count(m.Content) + 4
	}
	return total
}

// Len returns the total number of messages in the window.
func (hw *HistoryWindow) Len() int {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	return len(hw.messages)
}

// All returns a copy of all messages.
func (hw *HistoryWindow) All() []llm.Message {
	hw.mu.RLock()
	defer hw.mu.RUnlock()
	out := make([]llm.Message, len(hw.messages))
	copy(out, hw.messages)
	return out
}

// Clear resets the history window.
func (hw *HistoryWindow) Clear() {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	hw.messages = hw.messages[:0]
	hw.summarized = ""
	hw.summarizedUpTo = 0
}

// BuildCompactionPrompt creates the prompt sent to LLM for summarization.
// Exported for testing and customization.
func BuildCompactionPrompt(existingSummary string, messages []llm.Message) string {
	var sb strings.Builder

	sb.WriteString("You are a conversation summarizer. Create a concise summary of the following conversation that preserves:\n")
	sb.WriteString("1. Key decisions and their rationale\n")
	sb.WriteString("2. Important technical details and code references\n")
	sb.WriteString("3. Current state of any ongoing tasks\n")
	sb.WriteString("4. Unresolved questions or pending items\n\n")
	sb.WriteString("Be concise but thorough. Use bullet points. Preserve file paths, function names, and specific values.\n\n")

	if existingSummary != "" {
		sb.WriteString("=== PREVIOUS SUMMARY ===\n")
		sb.WriteString(existingSummary)
		sb.WriteString("\n\n")
		sb.WriteString("=== NEW MESSAGES TO INCORPORATE ===\n")
	} else {
		sb.WriteString("=== CONVERSATION TO SUMMARIZE ===\n")
	}

	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	sb.WriteString("\n=== YOUR SUMMARY ===\n")
	return sb.String()
}
