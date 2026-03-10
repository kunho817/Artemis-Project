package llm

import (
	"context"
	"fmt"
	"testing"
)

func TestClassifyError_Timeout(t *testing.T) {
	fe := ClassifyError(context.DeadlineExceeded, "claude")
	if fe.Kind != ErrTimeout {
		t.Errorf("expected ErrTimeout, got %d", fe.Kind)
	}
	if fe.Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", fe.Provider)
	}
	if fe.Hint == "" {
		t.Error("expected non-empty hint")
	}
}

func TestClassifyError_Cancelled(t *testing.T) {
	fe := ClassifyError(context.Canceled, "gpt")
	if fe.Kind != ErrCancelled {
		t.Errorf("expected ErrCancelled, got %d", fe.Kind)
	}
}

func TestClassifyError_Auth401(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("request failed: status 401"), "claude")
	if fe.Kind != ErrAuth {
		t.Errorf("expected ErrAuth, got %d", fe.Kind)
	}
	if fe.Hint == "" {
		t.Error("expected hint for auth error")
	}
}

func TestClassifyError_Auth403(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("HTTP status 403 Forbidden"), "gemini")
	if fe.Kind != ErrAuth {
		t.Errorf("expected ErrAuth, got %d", fe.Kind)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("status 429 Too Many Requests"), "gpt")
	if fe.Kind != ErrRateLimit {
		t.Errorf("expected ErrRateLimit, got %d", fe.Kind)
	}
}

func TestClassifyError_NoAPIKey(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("API key not configured for claude"), "claude")
	if fe.Kind != ErrNoAPIKey {
		t.Errorf("expected ErrNoAPIKey, got %d", fe.Kind)
	}
}

func TestClassifyError_Connection(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"refused", fmt.Errorf("dial tcp: connection refused")},
		{"no host", fmt.Errorf("dial tcp: no such host")},
		{"eof", fmt.Errorf("unexpected EOF reading response")},
		{"tls", fmt.Errorf("tls handshake timeout")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fe := ClassifyError(tt.err, "gemini")
			if fe.Kind != ErrConnection {
				t.Errorf("expected ErrConnection, got %d for error %q", fe.Kind, tt.err)
			}
		})
	}
}

func TestClassifyError_ServerError(t *testing.T) {
	for _, code := range []string{"500", "502", "503", "504"} {
		fe := ClassifyError(fmt.Errorf("status %s Internal Server Error", code), "glm")
		if fe.Kind != ErrServerError {
			t.Errorf("expected ErrServerError for %s, got %d", code, fe.Kind)
		}
	}
}

func TestClassifyError_BadRequest(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("status 400: context too long"), "claude")
	if fe.Kind != ErrBadRequest {
		t.Errorf("expected ErrBadRequest, got %d", fe.Kind)
	}
}

func TestClassifyError_RetryUnwrap(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("after 2 retries: status 429 rate limit exceeded"), "gpt")
	if fe.Kind != ErrRateLimit {
		t.Errorf("expected retry to unwrap to ErrRateLimit, got %d", fe.Kind)
	}
}

func TestClassifyError_Unknown(t *testing.T) {
	fe := ClassifyError(fmt.Errorf("some random error happened"), "claude")
	if fe.Kind != ErrUnknown {
		t.Errorf("expected ErrUnknown, got %d", fe.Kind)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	fe := ClassifyError(nil, "claude")
	if fe != nil {
		t.Errorf("expected nil for nil error")
	}
}

func TestFriendlyError_ErrorString(t *testing.T) {
	fe := &FriendlyError{Message: "test message", Hint: "try again"}
	s := fe.Error()
	if s != "test message (try again)" {
		t.Errorf("unexpected error string: %q", s)
	}

	fe2 := &FriendlyError{Message: "no hint"}
	if fe2.Error() != "no hint" {
		t.Errorf("expected message only without hint")
	}
}

func TestFriendlyError_Unwrap(t *testing.T) {
	orig := fmt.Errorf("original error")
	fe := &FriendlyError{Original: orig}
	if fe.Unwrap() != orig {
		t.Error("Unwrap should return original error")
	}
}

func TestProviderLabel(t *testing.T) {
	tests := map[string]string{
		"claude": "Claude", "gemini": "Gemini", "gpt": "GPT",
		"glm": "GLM", "": "LLM", "other": "other",
	}
	for input, expected := range tests {
		if got := providerLabel(input); got != expected {
			t.Errorf("providerLabel(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	long := "this is a long string that exceeds the limit"
	got := truncate(long, 20)
	if len(got) != 20 {
		t.Errorf("expected len 20, got %d", len(got))
	}
	if got[len(got)-3:] != "..." {
		t.Errorf("expected ... suffix, got %q", got)
	}
}
