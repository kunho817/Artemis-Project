package llm

import (
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// TokenCounter provides tiktoken-based token counting for prompt budget management.
// Uses cl100k_base encoding as a universal proxy — accurate for GPT models,
// ~95% accurate for Claude/Gemini/GLM (sufficient for budget allocation).
type TokenCounter struct {
	enc *tiktoken.Tiktoken
}

// singleton — encoding initialization is expensive; reuse across calls.
var (
	globalCounter     *TokenCounter
	globalCounterOnce sync.Once
	globalCounterErr  error
)

// GetTokenCounter returns the shared TokenCounter instance (singleton).
// Thread-safe. The encoding is initialized once on first call.
func GetTokenCounter() (*TokenCounter, error) {
	globalCounterOnce.Do(func() {
		enc, err := tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			globalCounterErr = err
			return
		}
		globalCounter = &TokenCounter{enc: enc}
	})
	return globalCounter, globalCounterErr
}

// NewTokenCounter creates a fresh TokenCounter (for testing or isolation).
func NewTokenCounter() (*TokenCounter, error) {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, err
	}
	return &TokenCounter{enc: enc}, nil
}

// Count returns the number of tokens in a text string.
func (tc *TokenCounter) Count(text string) int {
	if tc.enc == nil || text == "" {
		return 0
	}
	tokens := tc.enc.Encode(text, nil, nil)
	return len(tokens)
}

// CountMessages estimates total tokens for a slice of Messages.
// Accounts for per-message overhead (role markers, delimiters).
// Overhead estimation follows OpenAI's token counting guidelines:
//   - 4 tokens per message (role, separators)
//   - 2 tokens for reply priming
func (tc *TokenCounter) CountMessages(messages []Message) int {
	if tc.enc == nil {
		return 0
	}

	const perMessage = 4 // <|im_start|>{role}\n ... <|im_end|>\n
	const replyPriming = 2

	total := 0
	for _, msg := range messages {
		total += perMessage
		total += tc.Count(msg.Role)
		total += tc.Count(msg.Content)
	}
	total += replyPriming
	return total
}

// Fits checks if text fits within a token budget.
func (tc *TokenCounter) Fits(text string, budget int) bool {
	return tc.Count(text) <= budget
}

// Truncate truncates text to fit within maxTokens, preserving complete tokens.
// Returns the truncated text and whether truncation occurred.
func (tc *TokenCounter) Truncate(text string, maxTokens int) (string, bool) {
	if tc.enc == nil || text == "" {
		return text, false
	}

	tokens := tc.enc.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text, false
	}

	truncated := tokens[:maxTokens]
	result := tc.enc.Decode(truncated)
	return result, true
}

// TruncateFromEnd truncates text from the beginning, keeping the last maxTokens.
// Useful for keeping recent content when history exceeds budget.
// Returns the truncated text and whether truncation occurred.
func (tc *TokenCounter) TruncateFromEnd(text string, maxTokens int) (string, bool) {
	if tc.enc == nil || text == "" {
		return text, false
	}

	tokens := tc.enc.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text, false
	}

	start := len(tokens) - maxTokens
	truncated := tokens[start:]
	result := tc.enc.Decode(truncated)
	return result, true
}

// CountTokens is a package-level convenience function.
// Returns 0 if the counter fails to initialize (graceful degradation).
func CountTokens(text string) int {
	tc, err := GetTokenCounter()
	if err != nil || tc == nil {
		// Graceful fallback: rough estimate at 4 chars per token
		return len(text) / 4
	}
	return tc.Count(text)
}

// FitsInBudget is a package-level convenience function.
func FitsInBudget(text string, budget int) bool {
	tc, err := GetTokenCounter()
	if err != nil || tc == nil {
		return len(text)/4 <= budget
	}
	return tc.Fits(text, budget)
}
