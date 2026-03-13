package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RetryProvider wraps a Provider with automatic retry and exponential backoff.
// Non-retryable errors (auth failures, invalid requests) are returned immediately.
type RetryProvider struct {
	inner      Provider
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryProvider creates a provider that retries on transient failures.
// maxRetries=2 means up to 3 total attempts (1 initial + 2 retries).
func NewRetryProvider(inner Provider, maxRetries int) *RetryProvider {
	return &RetryProvider{
		inner:      inner,
		maxRetries: maxRetries,
		baseDelay:  time.Second,
	}
}

func (r *RetryProvider) Name() string { return r.inner.Name() }
func (r *RetryProvider) Model() string { return r.inner.Model() }

// Send retries with exponential backoff (1s, 2s, 4s...) on transient failures.
func (r *RetryProvider) Send(ctx context.Context, messages []Message) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.baseDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := r.inner.Send(ctx, messages)
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if ctx.Err() != nil || isNonRetryable(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("after %d retries: %w", r.maxRetries, lastErr)
}

// Stream retries the initial connection with exponential backoff.
// Once streaming starts, no retry is attempted (the caller handles the channel).
func (r *RetryProvider) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.baseDelay * time.Duration(1<<uint(attempt-1))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := r.inner.Stream(ctx, messages)
		if err == nil {
			return ch, nil
		}
		lastErr = err

		if ctx.Err() != nil || isNonRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d retries: %w", r.maxRetries, lastErr)
}

// isNonRetryable returns true for errors that should not be retried.
func isNonRetryable(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "API key not configured") ||
		strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "status 403") ||
		strings.Contains(msg, "status 400")
}
