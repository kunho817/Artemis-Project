// Package prompts provides Chain-of-Thought prompting templates.
package prompts

import (
	"fmt"
	"strings"
)

// CoTPromptBuilder builds Chain-of-Thought prompts for step-by-step reasoning.
type CoTPromptBuilder struct {
	steps          []CoTStep
	maxTokens      int
	includeReasoning bool
}

// CoTStep represents a single step in the reasoning chain.
type CoTStep struct {
	Name        string
	Description string
	Prompt      string
	Required    bool
}

// NewCoTPromptBuilder creates a new CoT prompt builder.
func NewCoTPromptBuilder() *CoTPromptBuilder {
	return &CoTPromptBuilder{
		steps:           make([]CoTStep, 0),
		maxTokens:       8192,
		includeReasoning: true,
	}
}

// AddStep adds a reasoning step.
func (cotb *CoTPromptBuilder) AddStep(name, description, prompt string) *CoTPromptBuilder {
	cotb.steps = append(cotb.steps, CoTStep{
		Name:        name,
		Description: description,
		Prompt:      prompt,
		Required:    true,
	})
	return cotb
}

// AddOptionalStep adds an optional reasoning step.
func (cotb *CoTPromptBuilder) AddOptionalStep(name, description, prompt string) *CoTPromptBuilder {
	cotb.steps = append(cotb.steps, CoTStep{
		Name:        name,
		Description: description,
		Prompt:      prompt,
		Required:    false,
	})
	return cotb
}

// SetMaxTokens sets the maximum token limit.
func (cotb *CoTPromptBuilder) SetMaxTokens(maxTokens int) *CoTPromptBuilder {
	cotb.maxTokens = maxTokens
	return cotb
}

// IncludeReasoning controls whether to include reasoning steps in output.
func (cotb *CoTPromptBuilder) IncludeReasoning(include bool) *CoTPromptBuilder {
	cotb.includeReasoning = include
	return cotb
}

// BuildPrompt creates a CoT prompt with step-by-step reasoning guidance.
func (cotb *CoTPromptBuilder) BuildPrompt(task string) string {
	var sb strings.Builder

	sb.WriteString("# Chain-of-Thought Visual Analysis\n\n")
	sb.WriteString(fmt.Sprintf("## Task\n%s\n\n", task))
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("Follow these reasoning steps systematically. ")
	sb.WriteString("Each step builds on the previous one.\n\n")

	sb.WriteString("### Reasoning Steps\n\n")
	for i, step := range cotb.steps {
		prefix := fmt.Sprintf("%d. **%s**", i+1, step.Name)
		if !step.Required {
			prefix += " (Optional)"
		}
		sb.WriteString(fmt.Sprintf("%s\n", prefix))
		sb.WriteString(fmt.Sprintf("- %s\n", step.Description))
		sb.WriteString(fmt.Sprintf("- Prompt: %s\n\n", step.Prompt))
	}

	sb.WriteString("## Response Format\n\n")
	if cotb.includeReasoning {
		sb.WriteString("Provide your reasoning for each step:\n\n")
		sb.WriteString("```\n")
		sb.WriteString("Step 1 - [Step Name]:\n")
		sb.WriteString("  [Your reasoning and observations]\n")
		sb.WriteString("  Result: [finding]\n\n")
		sb.WriteString("Step 2 - [Step Name]:\n")
		sb.WriteString("  [Your reasoning based on Step 1]\n")
		sb.WriteString("  Result: [finding]\n\n")
		sb.WriteString("...\n\n")
		sb.WriteString("Final Answer:\n")
		sb.WriteString("  [Consolidated conclusion]\n")
		sb.WriteString("```\n")
	} else {
		sb.WriteString("After completing all steps, provide your final answer.\n")
		sb.WriteString("Format: Final Answer: [your conclusion]\n")
	}

	return sb.String()
}

// BuildVisualAnalysisPrompt creates a CoT prompt specifically for visual analysis.
func (cotb *CoTPromptBuilder) BuildVisualAnalysisPrompt(question string) string {
	// Add standard visual analysis steps if none exist
	if len(cotb.steps) == 0 {
		cotb.addDefaultVisualSteps()
	}

	return cotb.BuildPrompt(question)
}

// addDefaultVisualSteps adds the standard 5-step visual analysis process.
func (cotb *CoTPromptBuilder) addDefaultVisualSteps() {
	cotb.AddStep(
		"Observation",
		"List all visible UI elements",
		"What do you see? List every UI element (buttons, text, icons, borders, etc.) in order of appearance (top to bottom, left to right).",
	).AddStep(
		"Absolute Positioning",
		"Identify exact locations of each element",
		"For each element, provide: (1) Grid coordinates (row, col), (2) Position keywords (top-left, center, etc.), (3) Approximate size in pixels or grid cells.",
	).AddStep(
		"Relative Positioning",
		"Describe spatial relationships between elements",
		"For key element pairs, describe: (1) Direction (up/down/left/right), (2) Distance (in grid cells), (3) Alignment (aligned left/center/right or top/middle/bottom).",
	).AddStep(
		"Relationship Inference",
		"Infer structural or semantic relationships",
		"Based on positions, what can you infer? (e.g., 'Element A is contained within Element B', 'Elements form a navigation bar', 'This is a form layout').",
	).AddStep(
		"Verification",
		"Cross-check your analysis against the visual",
		"Re-examine the image. Are your conclusions consistent with what you see? Adjust your answer if anything contradicts.",
	)
}

// BuildLayoutValidationPrompt creates a CoT prompt for layout validation.
func (cotb *CoTPromptBuilder) BuildLayoutValidationPrompt(layoutSpec string) string {
	var sb strings.Builder

	sb.WriteString("# Layout Validation with Chain-of-Thought\n\n")
	sb.WriteString(fmt.Sprintf("## Layout Specification\n%s\n\n", layoutSpec))
	sb.WriteString("## Validation Steps\n\n")
	sb.WriteString("1. **Parse Specification**: Extract all element requirements\n")
	sb.WriteString("2. **Visual Scan**: Identify actual elements in the image\n")
	sb.WriteString("3. **Position Check**: For each requirement, verify element exists at specified location\n")
	sb.WriteString("4. **Relationship Check**: Verify spatial relationships between elements\n")
	sb.WriteString("5. **Final Verdict**: Pass/Fail each requirement with explanation\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("For each requirement:\n")
	sb.WriteString("- Requirement: [spec text]\n")
	sb.WriteString("- Step 1: [Your parsing of the requirement]\n")
	sb.WriteString("- Step 2: [What you found in the image]\n")
	sb.WriteString("- Step 3: [Does it match? yes/no + details]\n")
	sb.WriteString("- Step 4: [Are relationships correct? yes/no + details]\n")
	sb.WriteString("- Step 5: [Final verdict: PASS/FAIL + confidence]\n\n")

	sb.WriteString("## Overall Verdict\n")
	sb.WriteString("- Total Requirements: [N]\n")
	sb.WriteString("- Passed: [count]\n")
	sb.WriteString("- Failed: [count]\n")
	sb.WriteString("- Overall: [PASS/FAIL]\n")

	return sb.String()
}

// BuildElementFindingPrompt creates a CoT prompt for finding specific elements.
func (cotb *CoTPromptBuilder) BuildElementFindingPrompt(elementDesc string) string {
	var sb strings.Builder

	sb.WriteString("# Element Finding with Chain-of-Thought\n\n")
	sb.WriteString(fmt.Sprintf("## Target Element\n%s\n\n", elementDesc))
	sb.WriteString("## Search Strategy\n\n")
	sb.WriteString("Follow these steps systematically:\n\n")

	sb.WriteString("### Step 1: Global Scan\n")
	sb.WriteString("- Scan the entire image from top to bottom, left to right\n")
	sb.WriteString("- Look for visual features matching the description\n")
	sb.WriteString("- Note any regions that might contain the element\n\n")

	sb.WriteString("### Step 2: Feature Matching\n")
	sb.WriteString("- Identify key visual features: color, shape, text, size\n")
	sb.WriteString("- Match these features against the element description\n")
	sb.WriteString("- Rate confidence of match (low/medium/high)\n\n")

	sb.WriteString("### Step 3: Location Verification\n")
	sb.WriteString("- If found, note exact grid coordinates\n")
	sb.WriteString("- Verify surrounding context matches expectations\n")
	sb.WriteString("- Confirm element is fully or partially visible\n\n")

	sb.WriteString("### Step 4: Absence Handling\n")
	sb.WriteString("- If not found, consider: (a) Element not present, (b) Outside visible area, (c) Description unclear\n")
	sb.WriteString("- Suggest alternative locations to check\n\n")

	sb.WriteString("## Response Format\n")
	sb.WriteString("- **Found**: [yes/no/partial]\n")
	sb.WriteString("- **Location**: [grid coordinates if found]\n")
	sb.WriteString("- **Confidence**: [0-100%%]\n")
	sb.WriteString("- **Reasoning**: [brief explanation for your conclusion]\n")

	return sb.String()
}

// BuildComparisonPrompt creates a CoT prompt for comparing two images.
func (cotb *CoTPromptBuilder) BuildComparisonPrompt(desc1, desc2 string) string {
	var sb strings.Builder

	sb.WriteString("# Visual Comparison with Chain-of-Thought\n\n")
	sb.WriteString("## Images to Compare\n")
	sb.WriteString(fmt.Sprintf("- Image A: %s\n", desc1))
	sb.WriteString(fmt.Sprintf("- Image B: %s\n\n", desc2))

	sb.WriteString("## Comparison Steps\n\n")
	sb.WriteString("Follow these steps to compare the two images:\n\n")

	sb.WriteString("### Step 1: Element Extraction\n")
	sb.WriteString("- List all UI elements in Image A\n")
	sb.WriteString("- List all UI elements in Image B\n")
	sb.WriteString("- Note any elements present in one but not the other\n\n")

	sb.WriteString("### Step 2: Position Mapping\n")
	sb.WriteString("- For matching elements, map their grid coordinates\n")
	sb.WriteString("- Identify position differences\n")
	sb.WriteString("- Calculate displacement (direction + distance)\n\n")

	sb.WriteString("### Step 3: Relationship Analysis\n")
	sb.WriteString("- Compare spatial relationships between element pairs\n")
	sb.WriteString("- Identify any relationship differences\n")
	sb.WriteString("- Note if alignment is consistent\n\n")

	sb.WriteString("### Step 4: Overall Assessment\n")
	sb.WriteString("- Determine if layouts are identical, similar, or different\n")
	sb.WriteString("- Identify the most significant differences\n")
	sb.WriteString("- Assess impact on user experience\n\n")

	sb.WriteString("## Response Format\n")
	sb.WriteString("- **Overall**: [identical/similar/different]\n")
	sb.WriteString("- **Key Differences**: [list of main differences]\n")
	sb.WriteString("- **Impact**: [low/medium/high impact on UX]\n")
	sb.WriteString("- **Details**: [step-by-step comparison results]\n")

	return sb.String()
}

// String returns the string representation of the builder.
func (cotb *CoTPromptBuilder) String() string {
	return fmt.Sprintf("CoTPromptBuilder{%d steps, maxTokens=%d}", len(cotb.steps), cotb.maxTokens)
}

// DefaultCoTConfig returns default CoT configuration for visual analysis.
func DefaultCoTConfig() *CoTPromptBuilder {
	cotb := NewCoTPromptBuilder()
	cotb.addDefaultVisualSteps()
	return cotb
}

// PresetCoTForLayoutValidation returns a pre-configured CoT for layout validation.
func PresetCoTForLayoutValidation() *CoTPromptBuilder {
	cotb := NewCoTPromptBuilder()
	cotb.AddStep("Parse Requirements", "Extract layout requirements", "Parse the specification and list all required elements and their positions.")
	cotb.AddStep("Visual Inspection", "Inspect the image", "List all visible elements and their positions.")
	cotb.AddStep("Position Verification", "Check element locations", "For each requirement, verify the element exists at the specified location.")
	cotb.AddStep("Relationship Verification", "Check spatial relationships", "Verify that relationships between elements match requirements.")
	cotb.AddStep("Final Assessment", "Provide overall verdict", "Summarize which requirements passed/failed and provide final verdict.")
	return cotb
}

// PresetCoTForElementFinding returns a pre-configured CoT for finding elements.
func PresetCoTForElementFinding() *CoTPromptBuilder {
	cotb := NewCoTPromptBuilder()
	cotb.AddStep("Global Scan", "Scan entire image", "Systematically scan from top-left to bottom-right, looking for the target element.")
	cotb.AddStep("Feature Analysis", "Analyze visual features", "Examine color, shape, text, and size to identify matching features.")
	cotb.AddStep("Location Confirmation", "Verify exact location", "Once found, determine precise grid coordinates and verify surrounding context.")
	cotb.AddStep("Confidence Assessment", "Rate confidence", "Provide confidence score (0-100%%) based on feature match quality.")
	return cotb
}
