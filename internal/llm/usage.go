package llm

// TokenUsage tracks token consumption for a single LLM call.
type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Add adds another usage to this one (for accumulation).
func (u *TokenUsage) Add(other *TokenUsage) {
	if other == nil {
		return
	}
	u.PromptTokens += other.PromptTokens
	u.CompletionTokens += other.CompletionTokens
	u.TotalTokens += other.TotalTokens
}

// ModelPricing defines per-token pricing for a model.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// DefaultPricing maps model name → pricing.
// Approximate values — user can override via future config.
var DefaultPricing = map[string]ModelPricing{
	// Anthropic Claude
	"claude-sonnet-4-6":        {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-sonnet-4-20250514": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	// Google Gemini
	"gemini-2.5-pro-preview-06-05": {InputPerMillion: 1.25, OutputPerMillion: 10.0},
	"gemini-2.0-flash":             {InputPerMillion: 0.10, OutputPerMillion: 0.40},
	"gemini-3.1-pro-preview":       {InputPerMillion: 1.25, OutputPerMillion: 10.0},
	// OpenAI GPT
	"gpt-4o":      {InputPerMillion: 2.50, OutputPerMillion: 10.0},
	"gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"gpt-5.4":     {InputPerMillion: 2.50, OutputPerMillion: 10.0},
	// ZhipuAI GLM (CNY pricing converted to USD approx)
	"glm-4":       {InputPerMillion: 1.40, OutputPerMillion: 1.40},
	"glm-4-flash": {InputPerMillion: 0.014, OutputPerMillion: 0.014},
	"glm-5":       {InputPerMillion: 1.40, OutputPerMillion: 1.40},
	// Local vLLM (free — runs on user hardware)
	"qwen2.5-coder-7b": {InputPerMillion: 0.0, OutputPerMillion: 0.0},
}

// CalculateCost computes USD cost for a TokenUsage given pricing.
func CalculateCost(usage *TokenUsage, pricing ModelPricing) float64 {
	if usage == nil {
		return 0
	}
	input := float64(usage.PromptTokens) / 1_000_000 * pricing.InputPerMillion
	output := float64(usage.CompletionTokens) / 1_000_000 * pricing.OutputPerMillion
	return input + output
}

// GetPricing returns pricing for a model, with a safe fallback.
func GetPricing(model string) ModelPricing {
	if p, ok := DefaultPricing[model]; ok {
		return p
	}
	// Conservative fallback for unknown models
	return ModelPricing{InputPerMillion: 5.0, OutputPerMillion: 15.0}
}
