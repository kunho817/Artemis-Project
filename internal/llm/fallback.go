package llm

import (
	"context"
	"fmt"
)

// FallbackProvider wraps a primary provider with fallback alternatives.
// If the primary's Send or Stream fails, each fallback is tried in order.
// This is transparent to callers — the Agent doesn't know it's using a fallback.
type FallbackProvider struct {
	primary   Provider
	fallbacks []Provider
	lastUsed  string // name of the provider that last served a request
}

// NewFallbackProvider creates a provider with a fallback chain.
// If primary is nil, the first available fallback is promoted to primary.
func NewFallbackProvider(primary Provider, fallbacks ...Provider) *FallbackProvider {
	fp := &FallbackProvider{
		primary:   primary,
		fallbacks: fallbacks,
	}
	// Promote first fallback if no primary
	if fp.primary == nil && len(fp.fallbacks) > 0 {
		fp.primary = fp.fallbacks[0]
		fp.fallbacks = fp.fallbacks[1:]
	}
	return fp
}

// Name returns the primary provider's name.
func (f *FallbackProvider) Name() string {
	if f.primary != nil {
		return f.primary.Name()
	}
	return "fallback(none)"
}

// Model returns the primary provider's model name.
func (f *FallbackProvider) Model() string {
	if f.primary != nil {
		return f.primary.Model()
	}
	return ""
}

// LastUsed returns the name of the provider that actually served the last request.
// Useful for observability — the caller can check if a fallback was used.
func (f *FallbackProvider) LastUsed() string {
	return f.lastUsed
}

// Send tries the primary provider first, then each fallback in order.
func (f *FallbackProvider) Send(ctx context.Context, messages []Message) (string, error) {
	providers := f.chain()
	if len(providers) == 0 {
		return "", fmt.Errorf("no providers available")
	}

	var lastErr error
	for _, p := range providers {
		resp, err := p.Send(ctx, messages)
		if err == nil {
			f.lastUsed = p.Name()
			return resp, nil
		}
		lastErr = fmt.Errorf("%s: %w", p.Name(), err)

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return "", lastErr
		}
	}

	return "", fmt.Errorf("all providers failed, last: %w", lastErr)
}

// Stream tries the primary provider first, then each fallback in order.
func (f *FallbackProvider) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	providers := f.chain()
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers available")
	}

	var lastErr error
	for _, p := range providers {
		ch, err := p.Stream(ctx, messages)
		if err == nil {
			f.lastUsed = p.Name()
			return ch, nil
		}
		lastErr = fmt.Errorf("%s: %w", p.Name(), err)

		if ctx.Err() != nil {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("all providers failed, last: %w", lastErr)
}

// chain returns [primary, fallback1, fallback2, ...] filtering out nils.
func (f *FallbackProvider) chain() []Provider {
	var chain []Provider
	if f.primary != nil {
		chain = append(chain, f.primary)
	}
	chain = append(chain, f.fallbacks...)
	return chain
}
