package llm

import (
	"context"
	"fmt"
	"strings"
)

// ErrorKind categorizes LLM errors for user-friendly display.
type ErrorKind int

const (
	ErrUnknown         ErrorKind = iota
	ErrNoAPIKey                  // API key not configured
	ErrAuth                      // 401/403 — invalid or expired key
	ErrRateLimit                 // 429 — too many requests
	ErrTimeout                   // context deadline exceeded
	ErrCancelled                 // user or context cancelled
	ErrConnection                // network unreachable, DNS failure, connection refused
	ErrServerError               // 5xx from provider
	ErrBadRequest                // 400 — malformed request
	ErrModelNotFound             // model not available
	ErrContentFiltered           // safety filter triggered
	ErrQuotaExceeded             // billing/quota limit
)

// FriendlyError wraps a raw LLM error with a user-facing message.
type FriendlyError struct {
	Kind     ErrorKind
	Provider string // which provider produced the error
	Original error  // underlying error
	Message  string // user-facing message
	Hint     string // actionable suggestion
}

func (e *FriendlyError) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s (%s)", e.Message, e.Hint)
	}
	return e.Message
}

func (e *FriendlyError) Unwrap() error {
	return e.Original
}

// ClassifyError analyzes a raw error and returns a FriendlyError with
// a user-facing message and actionable hint.
func ClassifyError(err error, provider string) *FriendlyError {
	if err == nil {
		return nil
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	fe := &FriendlyError{
		Provider: provider,
		Original: err,
	}

	switch {
	// Context cancellation / timeout
	case err == context.DeadlineExceeded || strings.Contains(lower, "deadline exceeded"):
		fe.Kind = ErrTimeout
		fe.Message = fmt.Sprintf("%s request timed out", providerLabel(provider))
		fe.Hint = "try a shorter prompt or check your network"

	case err == context.Canceled || strings.Contains(lower, "context canceled"):
		fe.Kind = ErrCancelled
		fe.Message = "Request cancelled"

	// API key missing
	case strings.Contains(lower, "api key not configured") || strings.Contains(lower, "api key not set"):
		fe.Kind = ErrNoAPIKey
		fe.Message = fmt.Sprintf("No API key for %s", providerLabel(provider))
		fe.Hint = "Ctrl+S → set API key"

	// Auth errors (401/403)
	case strings.Contains(msg, "status 401"):
		fe.Kind = ErrAuth
		fe.Message = fmt.Sprintf("%s: invalid API key", providerLabel(provider))
		fe.Hint = "Ctrl+S → check API key"

	case strings.Contains(msg, "status 403"):
		fe.Kind = ErrAuth
		fe.Message = fmt.Sprintf("%s: access denied", providerLabel(provider))
		fe.Hint = "check API key permissions or billing status"

	// Rate limiting (429)
	case strings.Contains(msg, "status 429") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		fe.Kind = ErrRateLimit
		fe.Message = fmt.Sprintf("%s: rate limited", providerLabel(provider))
		fe.Hint = "wait a moment and try again"

	// Quota / billing
	case strings.Contains(lower, "quota") || strings.Contains(lower, "billing") || strings.Contains(lower, "insufficient"):
		fe.Kind = ErrQuotaExceeded
		fe.Message = fmt.Sprintf("%s: quota exceeded", providerLabel(provider))
		fe.Hint = "check your billing/usage limits"

	// Content filter
	case strings.Contains(lower, "content filter") || strings.Contains(lower, "safety") || strings.Contains(lower, "blocked"):
		fe.Kind = ErrContentFiltered
		fe.Message = fmt.Sprintf("%s: content filtered", providerLabel(provider))
		fe.Hint = "rephrase your request"

	// Bad request (400)
	case strings.Contains(msg, "status 400"):
		fe.Kind = ErrBadRequest
		fe.Message = fmt.Sprintf("%s: bad request", providerLabel(provider))
		fe.Hint = "prompt may be too long or malformed"

	// Model not found (404 from model endpoint)
	case strings.Contains(msg, "status 404") || strings.Contains(lower, "model not found"):
		fe.Kind = ErrModelNotFound
		fe.Message = fmt.Sprintf("%s: model not available", providerLabel(provider))
		fe.Hint = "Ctrl+S → check model name"

	// Server errors (5xx)
	case strings.Contains(msg, "status 500") || strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") || strings.Contains(msg, "status 504"):
		fe.Kind = ErrServerError
		fe.Message = fmt.Sprintf("%s: server error", providerLabel(provider))
		fe.Hint = "provider may be experiencing issues — retry later"

	// Connection errors
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") ||
		strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "dial tcp") ||
		strings.Contains(lower, "tls handshake") || strings.Contains(lower, "eof") ||
		strings.Contains(lower, "connection reset"):
		fe.Kind = ErrConnection
		fe.Message = fmt.Sprintf("Cannot reach %s", providerLabel(provider))
		fe.Hint = "check your internet connection"

	// Retry exhaustion (from RetryProvider)
	case strings.Contains(lower, "after") && strings.Contains(lower, "retries"):
		// Unwrap the inner error for classification
		// Format: "after N retries: <inner error>"
		if idx := strings.Index(msg, ": "); idx != -1 {
			inner := ClassifyError(fmt.Errorf("%s", msg[idx+2:]), provider)
			if inner.Kind != ErrUnknown {
				inner.Message = inner.Message + " (after retries)"
				return inner
			}
		}
		fe.Kind = ErrUnknown
		fe.Message = fmt.Sprintf("%s: request failed after retries", providerLabel(provider))
		fe.Hint = "check connection and API key"

	default:
		fe.Kind = ErrUnknown
		fe.Message = fmt.Sprintf("%s error: %s", providerLabel(provider), truncate(msg, 120))
	}

	return fe
}

// providerLabel returns a display-friendly name for a provider.
func providerLabel(name string) string {
	switch strings.ToLower(name) {
	case "claude":
		return "Claude"
	case "gemini":
		return "Gemini"
	case "gpt":
		return "GPT"
	case "glm":
		return "GLM"
	default:
		if name == "" {
			return "LLM"
		}
		return name
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
