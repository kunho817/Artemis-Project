// Package vision provides rate limiting for vision API calls.
package vision

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter manages API rate limits for vision providers.
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limits   map[string]int
	window   time.Duration
	enabled  bool
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limits: map[string]int{
			"claude":  50,  // 50 requests/minute
			"gpt":     100, // 100 requests/minute
			"gemini":  60,  // 60 requests/minute
		},
		window:  time.Minute,
		enabled: true,
	}
}

// SetLimit sets the rate limit for a provider.
func (rl *RateLimiter) SetLimit(provider string, limit int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limits[provider] = limit
}

// GetLimit returns the rate limit for a provider.
func (rl *RateLimiter) GetLimit(provider string) (int, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit, ok := rl.limits[provider]
	if !ok {
		return 0, fmt.Errorf("no rate limit set for provider %s", provider)
	}

	return limit, nil
}

// SetWindow sets the time window for rate limiting.
func (rl *RateLimiter) SetWindow(window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.window = window
}

// Enable enables rate limiting.
func (rl *RateLimiter) Enable() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.enabled = true
}

// Disable disables rate limiting.
func (rl *RateLimiter) Disable() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.enabled = false
}

// IsEnabled returns whether rate limiting is enabled.
func (rl *RateLimiter) IsEnabled() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	return rl.enabled
}

// Allow checks if a request is allowed for the provider.
func (rl *RateLimiter) Allow(provider string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.enabled {
		return true
	}

	now := time.Now()

	// Clean up old requests
	if requests, ok := rl.requests[provider]; ok {
		var valid []time.Time
		for _, req := range requests {
			if now.Sub(req) <= rl.window {
				valid = append(valid, req)
			}
		}
		rl.requests[provider] = valid
	}

	// Check limit
	limit, ok := rl.limits[provider]
	if !ok {
		return true // No limit set
	}

	currentCount := len(rl.requests[provider])
	if currentCount >= limit {
		return false
	}

	// Add current request
	rl.requests[provider] = append(rl.requests[provider], now)
	return true
}

// Wait waits until a request is allowed for the provider.
func (rl *RateLimiter) Wait(ctx context.Context, provider string) error {
	for {
		if rl.Allow(provider) {
			return nil
		}

		// Calculate wait time
		rl.mu.Lock()
		requests := rl.requests[provider]
		limit := rl.limits[provider]
		window := rl.window
		rl.mu.Unlock()

		if len(requests) < limit {
			return nil
		}

		// Find the oldest request
		oldest := requests[0]
		waitDuration := window - time.Since(oldest)

		if waitDuration <= 0 {
			continue
		}

		// Wait or check context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Retry
		case <-time.After(100 * time.Millisecond):
			// Retry periodically
		}
	}
}

// GetRemainingRequests returns the number of remaining requests for a provider.
func (rl *RateLimiter) GetRemainingRequests(provider string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limit, ok := rl.limits[provider]
	if !ok {
		return -1 // No limit
	}

	requests := rl.requests[provider]
	now := time.Now()

	// Count valid requests
	validCount := 0
	for _, req := range requests {
		if now.Sub(req) <= rl.window {
			validCount++
		}
	}

	remaining := limit - validCount
	if remaining < 0 {
		remaining = 0
	}

	return remaining
}

// GetResetTime returns when the rate limit will reset for a provider.
func (rl *RateLimiter) GetResetTime(provider string) (time.Time, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	requests, ok := rl.requests[provider]
	if !ok || len(requests) == 0 {
		return time.Time{}, fmt.Errorf("no requests recorded for provider %s", provider)
	}

	// Find the oldest request within the window
	now := time.Now()
	var oldest *time.Time

	for _, req := range requests {
		if now.Sub(req) <= rl.window {
			if oldest == nil || req.Before(*oldest) {
				oldest = &req
			}
		}
	}

	if oldest == nil {
		return time.Time{}, fmt.Errorf("no valid requests found")
	}

	return oldest.Add(rl.window), nil
}

// Reset resets the rate limiter for a provider.
func (rl *RateLimiter) Reset(provider string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.requests[provider] = nil
}

// ResetAll resets all rate limiters.
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.requests = make(map[string][]time.Time)
}

// GetStats returns statistics about rate limiting.
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	stats := make(map[string]interface{})
	now := time.Now()

	for provider, requests := range rl.requests {
		validCount := 0
		for _, req := range requests {
			if now.Sub(req) <= rl.window {
				validCount++
			}
		}

		limit := rl.limits[provider]
		stats[provider] = map[string]interface{}{
			"current":  validCount,
			"limit":    limit,
			"remaining": limit - validCount,
			"window":   rl.window.String(),
		}
	}

	stats["enabled"] = rl.enabled

	return stats
}

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	capacity  int
	tokens    int
	refillRate time.Duration
	lastRefill time.Time
	mu        sync.Mutex
}

// NewTokenBucket creates a new token bucket.
func NewTokenBucket(capacity int, refillRate time.Duration) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a token is available.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

// Wait waits until a token is available.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		if tb.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(tb.refillRate):
			// Retry
		}
	}
}

// refill refills tokens based on time elapsed.
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	tokensToAdd := int(elapsed / tb.refillRate)
	if tokensToAdd > 0 {
		tb.tokens = min(tb.tokens+tokensToAdd, tb.capacity)
		tb.lastRefill = now
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTokens returns the current number of tokens.
func (tb *TokenBucket) GetTokens() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return tb.tokens
}

// GetCapacity returns the token bucket capacity.
func (tb *TokenBucket) GetCapacity() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	return tb.capacity
}

// SlidingWindowRateLimiter implements a sliding window rate limiter.
type SlidingWindowRateLimiter struct {
	limit    int
	window   time.Duration
	requests []time.Time
	mu       sync.Mutex
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter.
func NewSlidingWindowRateLimiter(limit int, window time.Duration) *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
}

// Allow checks if a request is allowed.
func (sw *SlidingWindowRateLimiter) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()

	// Remove requests outside the window
	var valid []time.Time
	for _, req := range sw.requests {
		if now.Sub(req) <= sw.window {
			valid = append(valid, req)
		}
	}
	sw.requests = valid

	// Check limit
	if len(sw.requests) >= sw.limit {
		return false
	}

	// Add current request
	sw.requests = append(sw.requests, now)
	return true
}

// GetCount returns the number of requests in the current window.
func (sw *SlidingWindowRateLimiter) GetCount() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	count := 0

	for _, req := range sw.requests {
		if now.Sub(req) <= sw.window {
			count++
		}
	}

	return count
}

// FixedWindowRateLimiter implements a fixed window rate limiter.
type FixedWindowRateLimiter struct {
	limit      int
	window     time.Duration
	count      int
	windowStart time.Time
	mu         sync.Mutex
}

// NewFixedWindowRateLimiter creates a new fixed window rate limiter.
func NewFixedWindowRateLimiter(limit int, window time.Duration) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		limit:      limit,
		window:     window,
		count:      0,
		windowStart: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (fw *FixedWindowRateLimiter) Allow() bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()

	// Reset window if expired
	if now.Sub(fw.windowStart) >= fw.window {
		fw.count = 0
		fw.windowStart = now
	}

	// Check limit
	if fw.count >= fw.limit {
		return false
	}

	fw.count++
	return true
}

// GetCount returns the number of requests in the current window.
func (fw *FixedWindowRateLimiter) GetCount() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()

	// Reset window if expired
	if now.Sub(fw.windowStart) >= fw.window {
		fw.count = 0
		fw.windowStart = now
	}

	return fw.count
}

// GetWindowStart returns the start time of the current window.
func (fw *FixedWindowRateLimiter) GetWindowStart() time.Time {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	return fw.windowStart
}

// GetWindowEnd returns the end time of the current window.
func (fw *FixedWindowRateLimiter) GetWindowEnd() time.Time {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	return fw.windowStart.Add(fw.window)
}

// MultiProviderRateLimiter manages rate limiters for multiple providers.
type MultiProviderRateLimiter struct {
	limiters map[string]*RateLimiter
	mu       sync.RWMutex
}

// NewMultiProviderRateLimiter creates a new multi-provider rate limiter.
func NewMultiProviderRateLimiter() *MultiProviderRateLimiter {
	return &MultiProviderRateLimiter{
		limiters: make(map[string]*RateLimiter),
	}
}

// GetLimiter returns the rate limiter for a provider.
func (mprl *MultiProviderRateLimiter) GetLimiter(provider string) *RateLimiter {
	mprl.mu.Lock()
	defer mprl.mu.Unlock()

	if _, ok := mprl.limiters[provider]; !ok {
		mprl.limiters[provider] = NewRateLimiter()
	}

	return mprl.limiters[provider]
}

// RemoveLimiter removes the rate limiter for a provider.
func (mprl *MultiProviderRateLimiter) RemoveLimiter(provider string) {
	mprl.mu.Lock()
	defer mprl.mu.Unlock()

	delete(mprl.limiters, provider)
}

// GetAllLimiters returns all rate limiters.
func (mprl *MultiProviderRateLimiter) GetAllLimiters() map[string]*RateLimiter {
	mprl.mu.RLock()
	defer mprl.mu.RUnlock()

	result := make(map[string]*RateLimiter)
	for k, v := range mprl.limiters {
		result[k] = v
	}

	return result
}
