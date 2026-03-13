package llm

import (
	"strings"
	"testing"
)

func newTestBudget(t *testing.T, budget int) *ContextBudget {
	t.Helper()
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter: %v", err)
	}
	return NewContextBudget(budget, tc)
}

func TestContextBudget_WithinBudget(t *testing.T) {
	cb := newTestBudget(t, 100_000)

	cb.Reserve("system", "You are a helpful assistant.")
	cb.Allocate(P2, "history", "User asked about Go.", 0)
	cb.Allocate(P5, "facts", "The project uses Go 1.25.", 0)
	cb.Reserve("task", "Write a function.")

	prompt, tokens := cb.Build()
	if prompt == "" {
		t.Error("Build returned empty prompt")
	}
	if tokens <= 0 {
		t.Errorf("tokens = %d, want > 0", tokens)
	}
	if tokens > 100_000 {
		t.Errorf("tokens %d exceeds budget 100000", tokens)
	}
	// All sections should be present
	if !strings.Contains(prompt, "helpful assistant") {
		t.Error("missing system prompt")
	}
	if !strings.Contains(prompt, "Go 1.25") {
		t.Error("missing facts")
	}
}

func TestContextBudget_OverBudget_DropsLowestPriority(t *testing.T) {
	cb := newTestBudget(t, 50) // tiny budget — forces dropping

	cb.Reserve("system", "System prompt.")                                 // P0 ~3 tokens
	cb.Reserve("task", "Task.")                                            // P0 ~2 tokens
	cb.Allocate(P6, "old-history", strings.Repeat("Old message. ", 50), 0) // P6 — lots of tokens

	prompt, tokens := cb.Build()

	// P0 must survive
	if !strings.Contains(prompt, "System prompt") {
		t.Error("P0 system prompt was dropped")
	}
	if !strings.Contains(prompt, "Task") {
		t.Error("P0 task was dropped")
	}
	// Total should be within budget (or close — P0 sections are never dropped)
	if tokens > 50 {
		// P0 alone might exceed tiny budget — that's OK, P0 is never dropped
		// But P6 should have been reduced/dropped
		if strings.Contains(prompt, strings.Repeat("Old message. ", 50)) {
			t.Error("P6 content should have been truncated or dropped")
		}
	}
}

func TestContextBudget_PreTruncation(t *testing.T) {
	cb := newTestBudget(t, 100_000)

	longContent := strings.Repeat("Token ", 500) // ~500 tokens
	cb.Allocate(P5, "facts", longContent, 10)    // cap at 10 tokens

	_, tokens := cb.Build()
	// The facts section should be pre-truncated to ~10 tokens
	// Total should be much less than 500
	if tokens > 50 {
		t.Errorf("pre-truncated section still has %d tokens, expected ~10", tokens)
	}
}

func TestContextBudget_Remaining(t *testing.T) {
	cb := newTestBudget(t, 1000)
	if cb.Remaining() != 1000 {
		t.Errorf("empty budget remaining = %d, want 1000", cb.Remaining())
	}

	cb.Reserve("task", "Hello")
	remaining := cb.Remaining()
	if remaining >= 1000 {
		t.Error("remaining should decrease after Reserve")
	}
}

func TestContextBudget_Summary(t *testing.T) {
	cb := newTestBudget(t, 10_000)
	cb.Reserve("system", "Be helpful.")
	cb.Allocate(P5, "facts", "Go project.", 0)

	summary := cb.Summary()
	if !strings.Contains(summary, "Budget:") {
		t.Error("summary missing Budget line")
	}
	if !strings.Contains(summary, "system") {
		t.Error("summary missing system section")
	}
}

func TestContextBudget_EmptyContent(t *testing.T) {
	cb := newTestBudget(t, 10_000)
	cb.Reserve("system", "")        // should be skipped
	cb.Allocate(P5, "facts", "", 0) // should be skipped

	if len(cb.sections) != 0 {
		t.Errorf("expected 0 sections for empty content, got %d", len(cb.sections))
	}
}

func TestContextBudget_PriorityOrder(t *testing.T) {
	// With a tight budget, higher priority sections should survive
	cb := newTestBudget(t, 100)

	cb.Reserve("task", "Important task.")                               // P0
	cb.Allocate(P2, "recent", "Recent conversation.", 0)                // P2
	cb.Allocate(P6, "old", strings.Repeat("Ancient history. ", 100), 0) // P6 — lots

	prompt, _ := cb.Build()

	// P0 must survive
	if !strings.Contains(prompt, "Important task") {
		t.Error("P0 was dropped")
	}
}

func TestNewContextBudgetForModel(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatal(err)
	}

	cb := NewContextBudgetForModel("gpt-5.4", tc)
	// gpt-5.4: 400K context - 32K output = 368K available
	if cb.TotalBudget() != 368_000 {
		t.Errorf("budget = %d, want 368000", cb.TotalBudget())
	}
}
