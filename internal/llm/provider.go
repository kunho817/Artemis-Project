package llm

import (
	"context"
	"net/http"
	"time"

	"github.com/artemis-project/artemis/internal/config"
)

// newHTTPClient creates a properly configured HTTP client for LLM API calls.
// Prevents the default zero-timeout http.Client that causes "context deadline exceeded" errors.
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 180 * time.Second, // 3 min hard limit per request
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// StreamChunk represents a chunk of streamed response.
type StreamChunk struct {
	Content   string
	Reasoning string // GLM reasoning_content (thinking mode)
	Done      bool
	Error     error
	Usage     *TokenUsage // populated on Done=true chunk with total usage
}

// Provider defines the interface all LLM providers must implement.
type Provider interface {
	// Name returns the provider identifier (e.g., "claude", "gpt").
	Name() string

	// Model returns the model name currently configured (e.g., "gpt-5.4").
	Model() string

	// Send sends a message and returns the full response.
	Send(ctx context.Context, messages []Message) (string, error)

	// Stream sends a message and streams the response chunk by chunk.
	Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error)
}

// NewProvider creates a provider based on config.
func NewProvider(name string, cfg *config.Config) Provider {
	switch name {
	case "claude":
		return NewClaude(cfg.Claude)
	case "gemini":
		return NewGemini(cfg.Gemini)
	case "gpt":
		return NewGPT(cfg.GPT)
	case "glm":
		return NewGLM(cfg.GLM)
	case "vllm":
		return NewVLLM(cfg.VLLM)
	default:
		return nil
	}
}

// NewProviderWithModel creates a provider with a model override.
// The provider's default model (from config) is replaced with the specified model.
// This enables per-role model selection (e.g., Scout using gemini-3-flash-preview
// while other Gemini roles use gemini-3.1-pro-preview).
func NewProviderWithModel(name string, cfg *config.Config, model string) Provider {
	switch name {
	case "claude":
		override := cfg.Claude
		override.Model = model
		return NewClaude(override)
	case "gemini":
		override := cfg.Gemini
		override.Model = model
		return NewGemini(override)
	case "gpt":
		override := cfg.GPT
		override.Model = model
		return NewGPT(override)
	case "glm":
		override := cfg.GLM
		override.Model = model
		return NewGLM(override)
	case "vllm":
		override := cfg.VLLM
		override.Model = model
		return NewVLLM(override)
	default:
		return nil
	}
}
