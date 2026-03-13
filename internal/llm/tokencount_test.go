package llm

import (
	"strings"
	"testing"
)

func TestTokenCounter_Count(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter failed: %v", err)
	}

	// Empty string
	if got := tc.Count(""); got != 0 {
		t.Errorf("Count(\"\") = %d, want 0", got)
	}

	// Known string — "Hello, world!" is 4 tokens in cl100k_base
	count := tc.Count("Hello, world!")
	if count < 1 {
		t.Errorf("Count(\"Hello, world!\") = %d, want > 0", count)
	}
}

func TestTokenCounter_CountMessages(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter failed: %v", err)
	}

	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	count := tc.CountMessages(messages)
	// Should be content tokens + per-message overhead (4 each) + reply priming (2)
	// = content + 2*4 + 2 = content + 10
	if count < 10 {
		t.Errorf("CountMessages = %d, expected at least 10 (overhead alone)", count)
	}
}

func TestTokenCounter_Fits(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter failed: %v", err)
	}

	short := "Hello"
	if !tc.Fits(short, 100) {
		t.Error("short text should fit in budget of 100")
	}

	if tc.Fits(short, 0) {
		t.Error("text should not fit in budget of 0")
	}
}

func TestTokenCounter_Truncate(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter failed: %v", err)
	}

	// Long text that definitely exceeds 5 tokens
	long := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)
	truncated, wasTruncated := tc.Truncate(long, 5)

	if !wasTruncated {
		t.Error("expected truncation for long text with maxTokens=5")
	}
	if len(truncated) >= len(long) {
		t.Errorf("truncated len %d should be less than original %d", len(truncated), len(long))
	}

	// Short text that fits
	short := "Hi"
	result, wasTruncated := tc.Truncate(short, 100)
	if wasTruncated {
		t.Error("short text should not be truncated with budget 100")
	}
	if result != short {
		t.Errorf("non-truncated result %q != original %q", result, short)
	}
}

func TestTokenCounter_TruncateFromEnd(t *testing.T) {
	tc, err := NewTokenCounter()
	if err != nil {
		t.Fatalf("NewTokenCounter failed: %v", err)
	}

	long := strings.Repeat("Word ", 200) // ~200+ tokens
	result, wasTruncated := tc.TruncateFromEnd(long, 10)

	if !wasTruncated {
		t.Error("expected truncation from end for long text")
	}
	if len(result) >= len(long) {
		t.Errorf("result len %d should be less than original %d", len(result), len(long))
	}
}

func TestCountTokens_PackageLevel(t *testing.T) {
	count := CountTokens("Hello, world!")
	if count < 1 {
		t.Errorf("CountTokens = %d, want > 0", count)
	}
}

func TestFitsInBudget_PackageLevel(t *testing.T) {
	if !FitsInBudget("Hello", 100) {
		t.Error("short text should fit in budget 100")
	}
}

func TestGetTokenCounter_Singleton(t *testing.T) {
	tc1, err1 := GetTokenCounter()
	tc2, err2 := GetTokenCounter()
	if err1 != nil || err2 != nil {
		t.Fatalf("GetTokenCounter errors: %v, %v", err1, err2)
	}
	if tc1 != tc2 {
		t.Error("GetTokenCounter should return same instance")
	}
}
