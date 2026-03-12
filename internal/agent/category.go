package agent

// TaskCategory classifies a task's domain, determining model selection and behavioral guidance.
// Categories are assigned by the Orchestrator per task in the ExecutionPlan.
type TaskCategory string

const (
	CategoryVisualEngineering TaskCategory = "visual-engineering"
	CategoryUltrabrain        TaskCategory = "ultrabrain"
	CategoryDeep              TaskCategory = "deep"
	CategoryArtistry          TaskCategory = "artistry"
	CategoryQuick             TaskCategory = "quick"
	CategoryUnspecifiedLow    TaskCategory = "unspecified-low"
	CategoryUnspecifiedHigh   TaskCategory = "unspecified-high"
	CategoryWriting           TaskCategory = "writing"
)

// AllCategories returns all defined task categories.
func AllCategories() []TaskCategory {
	return []TaskCategory{
		CategoryVisualEngineering, CategoryUltrabrain, CategoryDeep,
		CategoryArtistry, CategoryQuick, CategoryUnspecifiedLow,
		CategoryUnspecifiedHigh, CategoryWriting,
	}
}

// IsValidCategory checks if the given string is a valid category name.
func IsValidCategory(name string) bool {
	for _, c := range AllCategories() {
		if string(c) == name {
			return true
		}
	}
	return false
}

// CategoryConfig holds the provider/model override and behavioral prompt for a category.
type CategoryConfig struct {
	// Provider override (e.g., "gemini", "gpt", "claude", "glm").
	// Empty means use the role's default provider.
	PremiumProvider string
	BudgetProvider  string

	// Model override within the provider (e.g., "gemini-3-flash-preview").
	// Empty means use the provider's default model from config.
	PremiumModel string
	BudgetModel  string

	// PromptAppend is injected into the agent's prompt as "## Category Context".
	PromptAppend string
}

// DefaultCategoryConfigs maps each category to its provider/model/prompt configuration.
// Premium tier uses the specified provider; Budget tier uses the budget provider.
var DefaultCategoryConfigs = map[TaskCategory]CategoryConfig{
	CategoryVisualEngineering: {
		PremiumProvider: "gemini",
		BudgetProvider:  "gemini",
		PromptAppend:    visualEngineeringPrompt,
	},
	CategoryUltrabrain: {
		PremiumProvider: "gpt",
		BudgetProvider:  "gemini",
		PromptAppend:    ultrabrainPrompt,
	},
	CategoryDeep: {
		PremiumProvider: "gpt",
		BudgetProvider:  "gemini",
		PromptAppend:    deepPrompt,
	},
	CategoryArtistry: {
		PremiumProvider: "gemini",
		BudgetProvider:  "gemini",
		PromptAppend:    artistryPrompt,
	},
	CategoryQuick: {
		PremiumProvider: "claude",
		BudgetProvider:  "glm",
		PromptAppend:    quickPrompt,
	},
	CategoryUnspecifiedLow: {
		PremiumProvider: "claude",
		BudgetProvider:  "gemini",
		PromptAppend:    unspecifiedLowPrompt,
	},
	CategoryUnspecifiedHigh: {
		PremiumProvider: "claude",
		BudgetProvider:  "gemini",
		PromptAppend:    unspecifiedHighPrompt,
	},
	CategoryWriting: {
		PremiumProvider: "glm",
		BudgetProvider:  "glm",
		PromptAppend:    writingPrompt,
	},
}

// CategoryDescriptions provides short descriptions for each category.
var CategoryDescriptions = map[TaskCategory]string{
	CategoryVisualEngineering: "Frontend, UI/UX, design, styling, animation",
	CategoryUltrabrain:        "Hard, logic-heavy tasks. Give clear goals, not step-by-step instructions.",
	CategoryDeep:              "Goal-oriented autonomous problem-solving. Thorough research before action.",
	CategoryArtistry:          "Complex problem-solving with unconventional, creative approaches.",
	CategoryQuick:             "Trivial tasks — single file changes, typo fixes, simple modifications.",
	CategoryUnspecifiedLow:    "Tasks that don't fit other categories, low effort required.",
	CategoryUnspecifiedHigh:   "Tasks that don't fit other categories, high effort required.",
	CategoryWriting:           "Documentation, prose, technical writing.",
}

// ProviderForCategory returns the preferred provider for a category in the given tier.
func ProviderForCategory(cat TaskCategory, tier string) string {
	cfg, ok := DefaultCategoryConfigs[cat]
	if !ok {
		return ""
	}
	if tier == "premium" {
		return cfg.PremiumProvider
	}
	return cfg.BudgetProvider
}

// ModelForCategory returns the model override for a category in the given tier.
// Returns empty string if no override (use provider's default model).
func ModelForCategory(cat TaskCategory, tier string) string {
	cfg, ok := DefaultCategoryConfigs[cat]
	if !ok {
		return ""
	}
	if tier == "premium" {
		return cfg.PremiumModel
	}
	return cfg.BudgetModel
}

// PromptForCategory returns the behavioral prompt appendage for a category.
func PromptForCategory(cat TaskCategory) string {
	cfg, ok := DefaultCategoryConfigs[cat]
	if !ok {
		return ""
	}
	return cfg.PromptAppend
}

// --- Category prompt appendages ---

const visualEngineeringPrompt = `<Category_Context>
You are working on VISUAL/UI tasks.

Design-system-first workflow:
1. BEFORE writing any UI code, search for the design system (tokens, theme, shared components)
2. Read 5-10 existing UI components to understand patterns (naming, spacing, colors, typography)
3. If no design system exists, create a minimal one FIRST (palette, type scale, spacing scale)
4. Build with the system — use tokens/variables, NEVER hardcode colors or spacing
5. Verify: every color is a token, every spacing uses the scale, every component follows composition patterns

Bold aesthetic choices over safe defaults. Cohesive palettes. Purposeful animation.
AVOID: generic fonts, cookie-cutter layouts, arbitrary spacing values.
</Category_Context>`

const ultrabrainPrompt = `<Category_Context>
You are working on DEEP LOGICAL REASONING / COMPLEX ARCHITECTURE tasks.

Before writing code, SEARCH existing codebase for similar patterns.
Your code MUST match the project's conventions — blend in seamlessly.

Strategic advisor mindset:
- Bias toward simplicity: least complex solution that fulfills requirements
- Leverage existing code/patterns over new components
- Prioritize maintainability over cleverness
- One clear recommendation with effort estimate
- Signal when an advanced approach is warranted

Response: bottom line (2-3 sentences) → action plan (numbered) → risks/mitigations.
</Category_Context>`

const deepPrompt = `<Category_Context>
You are working on GOAL-ORIENTED AUTONOMOUS tasks.

You are NOT an interactive assistant. You are an autonomous problem-solver.

BEFORE making any changes:
1. Explore the codebase extensively (reading is normal and expected)
2. Read related files, trace dependencies, understand full context
3. Build a complete mental model of the problem space

Autonomous executor mindset:
- You receive a GOAL, not step-by-step instructions
- Figure out HOW to achieve the goal yourself
- Thorough research before any action
- Prefer comprehensive solutions over quick patches
- If unclear, make reasonable assumptions and proceed

Focus on results, not play-by-play progress.
</Category_Context>`

const artistryPrompt = `<Category_Context>
You are working on HIGHLY CREATIVE / ARTISTIC tasks.

Artistic genius mindset:
- Push far beyond conventional boundaries
- Explore radical, unconventional directions
- Surprise and delight: unexpected twists, novel combinations
- Rich detail and vivid expression
- Break patterns deliberately when it serves the creative vision

Balance novelty with coherence. Generate diverse, bold options first.
</Category_Context>`

const quickPrompt = `<Category_Context>
You are working on SMALL / QUICK tasks.

Efficient execution mindset:
- Fast, focused, minimal overhead
- Get to the point immediately
- No over-engineering — simple solutions for simple problems
- Minimal viable implementation
- Skip unnecessary abstractions
</Category_Context>`

const unspecifiedLowPrompt = `<Category_Context>
General-purpose task, moderate effort.

Balanced approach:
- Follow existing patterns
- Clean, readable implementation
- Proportional effort — don't over-engineer
</Category_Context>`

const unspecifiedHighPrompt = `<Category_Context>
General-purpose task, substantial effort.

Thorough approach:
- Understand the full scope before starting
- Consider edge cases and error handling
- Write production-quality code
- Document non-obvious decisions
</Category_Context>`

const writingPrompt = `<Category_Context>
You are working on WRITING / PROSE tasks.

Wordsmith mindset:
- Clear, flowing prose with appropriate tone
- Proper structure and organization
- Engaging and readable

ANTI-AI-SLOP RULES:
- NEVER use em dashes or en dashes. Use commas, periods, or line breaks.
- Remove AI phrases: "delve", "it's important to note", "leverage", "utilize", "facilitate"
- Pick plain words. "Use" not "utilize". "Start" not "commence".
- Use contractions naturally.
- Vary sentence length.
- No filler openings.
- Write like a human, not a corporate template.
</Category_Context>`
