package llm

import (
	"fmt"
	"strings"
)

// Priority defines the importance level of a prompt section.
// Lower values = higher priority = last to be truncated.
type Priority int

const (
	P0 Priority = iota // System Prompt + Task — NEVER truncated
	P1                 // Active Tool Results (current step)
	P2                 // Recent History (last 3-5 turns)
	P3                 // Artifacts (context from previous agents)
	P4                 // Skills + Category behavioral context
	P5                 // Repo-map + Project Facts
	P6                 // Older/Summarized History — first to drop
)

// BudgetSection represents a single section within the prompt budget.
type BudgetSection struct {
	Priority  Priority
	Label     string // section header (e.g., "## Project Knowledge")
	Content   string // section body
	MaxTokens int    // soft cap: if > 0, content is pre-truncated to this limit
	Tokens    int    // actual token count (set during allocation)
	truncated bool   // true if content was truncated during Build()
	dropped   bool   // true if section was dropped entirely during Build()
}

// ContextBudget manages token allocation across prompt sections
// using priority-based budgeting. Sections are added with priorities,
// and when the total exceeds the model's available input tokens,
// lower-priority sections are truncated or dropped first.
type ContextBudget struct {
	totalBudget int           // available input tokens (ContextWindow - MaxOutput)
	counter     *TokenCounter // tiktoken counter
	sections    []*BudgetSection
}

// NewContextBudget creates a budget for the given available input tokens.
// Pass spec.AvailableInputTokens() as the budget.
func NewContextBudget(availableTokens int, counter *TokenCounter) *ContextBudget {
	return &ContextBudget{
		totalBudget: availableTokens,
		counter:     counter,
		sections:    make([]*BudgetSection, 0, 8),
	}
}

// NewContextBudgetForModel is a convenience constructor that looks up the model spec.
func NewContextBudgetForModel(model string, counter *TokenCounter) *ContextBudget {
	spec := GetModelSpec(model)
	return NewContextBudget(spec.AvailableInputTokens(), counter)
}

// Reserve adds a section that is NEVER truncated or dropped (P0).
// Use for system prompt and task.
func (cb *ContextBudget) Reserve(label, content string) {
	if content == "" {
		return
	}
	tokens := cb.counter.Count(content)
	cb.sections = append(cb.sections, &BudgetSection{
		Priority:  P0,
		Label:     label,
		Content:   content,
		MaxTokens: 0, // no cap — reserved space
		Tokens:    tokens,
	})
}

// Allocate adds a section with a priority and optional max token cap.
// If maxTokens > 0 and content exceeds it, content is pre-truncated.
// During Build(), sections may be further truncated based on total budget.
func (cb *ContextBudget) Allocate(priority Priority, label, content string, maxTokens int) {
	if content == "" {
		return
	}

	actualContent := content
	tokens := cb.counter.Count(content)

	// Pre-truncate if section has its own cap
	if maxTokens > 0 && tokens > maxTokens {
		actualContent, _ = cb.counter.Truncate(content, maxTokens)
		tokens = maxTokens
	}

	cb.sections = append(cb.sections, &BudgetSection{
		Priority:  priority,
		Label:     label,
		Content:   actualContent,
		MaxTokens: maxTokens,
		Tokens:    tokens,
	})
}

// Build assembles the final prompt string within the token budget.
// If total tokens exceed the budget, sections are trimmed from lowest
// priority (P6) upward. P0 sections are never modified.
//
// Returns the assembled prompt and the total token count.
func (cb *ContextBudget) Build() (string, int) {
	totalTokens := cb.usedTokens()

	// If within budget, assemble directly
	if totalTokens <= cb.totalBudget {
		return cb.assemble(), totalTokens
	}

	// Over budget — trim from lowest priority up
	overage := totalTokens - cb.totalBudget

	for pri := P6; pri > P0 && overage > 0; pri-- {
		for i := len(cb.sections) - 1; i >= 0 && overage > 0; i-- {
			s := cb.sections[i]
			if s.Priority != pri || s.dropped {
				continue
			}

			// Try truncating to half first
			halfBudget := s.Tokens / 2
			if halfBudget > 0 && overage < s.Tokens {
				truncated, _ := cb.counter.Truncate(s.Content, s.Tokens-overage)
				newTokens := cb.counter.Count(truncated)
				saved := s.Tokens - newTokens
				s.Content = truncated
				s.Tokens = newTokens
				s.truncated = true
				overage -= saved
			} else {
				// Drop the section entirely
				overage -= s.Tokens
				s.dropped = true
				s.Tokens = 0
			}
		}
	}

	return cb.assemble(), cb.usedTokens()
}

// Remaining returns how many tokens are still available.
func (cb *ContextBudget) Remaining() int {
	remaining := cb.totalBudget - cb.usedTokens()
	if remaining < 0 {
		return 0
	}
	return remaining
}

// UsedTokens returns the total tokens currently allocated.
func (cb *ContextBudget) UsedTokens() int {
	return cb.usedTokens()
}

// TotalBudget returns the total available input tokens.
func (cb *ContextBudget) TotalBudget() int {
	return cb.totalBudget
}

// Summary returns a human-readable breakdown of the budget allocation.
// Useful for debugging and observability.
func (cb *ContextBudget) Summary() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Budget: %d tokens (used: %d, remaining: %d)",
		cb.totalBudget, cb.usedTokens(), cb.Remaining()))

	for _, s := range cb.sections {
		status := ""
		if s.dropped {
			status = " [DROPPED]"
		} else if s.truncated {
			status = " [TRUNCATED]"
		}
		lines = append(lines, fmt.Sprintf("  P%d %-30s %6d tokens%s",
			s.Priority, s.Label, s.Tokens, status))
	}
	return strings.Join(lines, "\n")
}

func (cb *ContextBudget) usedTokens() int {
	total := 0
	for _, s := range cb.sections {
		if !s.dropped {
			total += s.Tokens
		}
	}
	return total
}

func (cb *ContextBudget) assemble() string {
	var parts []string
	for _, s := range cb.sections {
		if s.dropped || s.Content == "" {
			continue
		}
		parts = append(parts, s.Content)
	}
	return strings.Join(parts, "\n\n")
}
