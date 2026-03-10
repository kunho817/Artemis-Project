package llm

import (
	"context"

	"github.com/artemis-project/artemis/internal/config"
)

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
}

// Provider defines the interface all LLM providers must implement.
type Provider interface {
	// Name returns the provider identifier.
	Name() string

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
	default:
		return nil
	}
}
