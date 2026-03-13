package llm

import "sync"

// ModelSpec describes the capabilities and limits of an LLM model.
type ModelSpec struct {
	ContextWindow int // total input+output token limit
	MaxOutput     int // maximum output tokens
}

// AvailableInputTokens returns context window minus max output reservation.
// This is the budget available for prompt construction.
func (s ModelSpec) AvailableInputTokens() int {
	avail := s.ContextWindow - s.MaxOutput
	if avail < 0 {
		return 0
	}
	return avail
}

// defaultModelSpecs maps model names to their capabilities.
// Values based on provider documentation as of 2026-03.
var defaultModelSpecs = map[string]ModelSpec{
	// Anthropic Claude
	"claude-sonnet-4-6":        {ContextWindow: 200_000, MaxOutput: 16_000},
	"claude-sonnet-4-20250514": {ContextWindow: 200_000, MaxOutput: 16_000},
	"claude-opus-4-6":          {ContextWindow: 200_000, MaxOutput: 32_000},

	// Google Gemini
	"gemini-3.1-pro-preview":       {ContextWindow: 2_000_000, MaxOutput: 65_000},
	"gemini-3-flash-preview":       {ContextWindow: 1_000_000, MaxOutput: 32_000},
	"gemini-2.5-pro-preview-06-05": {ContextWindow: 1_000_000, MaxOutput: 32_000},
	"gemini-2.0-flash":             {ContextWindow: 1_000_000, MaxOutput: 8_192},

	// OpenAI GPT
	"gpt-5.4":     {ContextWindow: 400_000, MaxOutput: 32_000},
	"gpt-4o":      {ContextWindow: 128_000, MaxOutput: 16_384},
	"gpt-4o-mini": {ContextWindow: 128_000, MaxOutput: 16_384},

	// ZhipuAI GLM
	"glm-5":       {ContextWindow: 200_000, MaxOutput: 8_000},
	"glm-4":       {ContextWindow: 128_000, MaxOutput: 4_096},
	"glm-4-flash": {ContextWindow: 128_000, MaxOutput: 4_096},

	// Local vLLM (Qwen2.5-Coder)
	"qwen2.5-coder-7b": {ContextWindow: 32_768, MaxOutput: 8_192},
}

// fallbackSpec is used when a model is not found in the registry.
// Conservative values to prevent accidental overflow.
var fallbackSpec = ModelSpec{
	ContextWindow: 32_768,
	MaxOutput:     4_096,
}

// modelRegistry provides thread-safe access to model specs.
// Supports runtime registration for user-defined models.
var modelRegistry = struct {
	mu    sync.RWMutex
	specs map[string]ModelSpec
}{
	specs: copySpecs(defaultModelSpecs),
}

// GetModelSpec returns the ModelSpec for a given model name.
// Falls back to conservative defaults for unknown models.
func GetModelSpec(model string) ModelSpec {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	if spec, ok := modelRegistry.specs[model]; ok {
		return spec
	}
	return fallbackSpec
}

// RegisterModelSpec registers or updates a model's spec at runtime.
// This allows users to define specs for custom/local models via config.
func RegisterModelSpec(model string, spec ModelSpec) {
	modelRegistry.mu.Lock()
	defer modelRegistry.mu.Unlock()

	modelRegistry.specs[model] = spec
}

// HasModelSpec checks if a model has a registered spec.
func HasModelSpec(model string) bool {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	_, ok := modelRegistry.specs[model]
	return ok
}

// AllModelSpecs returns a copy of all registered model specs.
func AllModelSpecs() map[string]ModelSpec {
	modelRegistry.mu.RLock()
	defer modelRegistry.mu.RUnlock()

	return copySpecs(modelRegistry.specs)
}

func copySpecs(src map[string]ModelSpec) map[string]ModelSpec {
	dst := make(map[string]ModelSpec, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
