package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artemis-project/artemis/internal/llm"
)

// GenerateCodeTool delegates code generation to a local vLLM-served model.
// Supports three modes: instruction (ChatML), fim (Fill-in-the-Middle), and file (full file generation).
type GenerateCodeTool struct {
	baseDir  string
	provider llm.Provider // may be nil if local model not configured
}

func (t *GenerateCodeTool) Name() string { return "generate_code" }

func (t *GenerateCodeTool) Description() string {
	return "Generate code using a local fine-tuned model. Modes: instruction (natural language prompt), fim (fill-in-the-middle completion), file (full file generation from description)."
}

func (t *GenerateCodeTool) Parameters() string {
	return `{
  "prompt": "(required) The code generation instruction or description.",
  "language": "(optional) Target programming language (default: go).",
  "mode": "(optional) Generation mode: instruction | fim | file (default: instruction).",
  "prefix": "(optional, fim mode only) Code before the cursor position.",
  "suffix": "(optional, fim mode only) Code after the cursor position.",
  "context_files": "(optional) Comma-separated relative file paths to include as context."
}`
}

func (t *GenerateCodeTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	if t.provider == nil {
		return ToolResult{
			Error: "Local code generation model not configured. Enable vLLM in settings (Ctrl+S) and ensure vLLM server is running.",
		}, nil
	}

	prompt, _ := params["prompt"].(string)
	if prompt == "" {
		return ToolResult{Error: "prompt parameter is required"}, nil
	}

	language, _ := params["language"].(string)
	if language == "" {
		language = "go"
	}

	mode, _ := params["mode"].(string)
	if mode == "" {
		mode = "instruction"
	}

	// Gather optional file context
	contextContent := t.gatherContext(params)

	var messages []llm.Message

	switch mode {
	case "instruction":
		messages = t.buildInstructionMessages(prompt, language, contextContent)
	case "fim":
		messages = t.buildFIMMessages(params, language, contextContent)
	case "file":
		messages = t.buildFileMessages(prompt, language, contextContent)
	default:
		return ToolResult{Error: fmt.Sprintf("unknown mode %q — use instruction, fim, or file", mode)}, nil
	}

	response, err := t.provider.Send(ctx, messages)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("code generation failed: %v", err)}, nil
	}

	// Strip common wrapper artifacts from model output
	response = cleanCodeOutput(response)

	return ToolResult{
		Content: response,
	}, nil
}

// buildInstructionMessages creates ChatML messages for instruction-based code generation.
func (t *GenerateCodeTool) buildInstructionMessages(prompt, language, contextContent string) []llm.Message {
	systemMsg := fmt.Sprintf(
		"You are a code generation assistant. Generate clean, production-quality %s code. "+
			"Output ONLY the code — no explanations, no markdown fences, no comments about your approach. "+
			"Follow existing conventions if context is provided.",
		language,
	)

	userContent := prompt
	if contextContent != "" {
		userContent = fmt.Sprintf("Context:\n%s\n\nTask:\n%s", contextContent, prompt)
	}

	return []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userContent},
	}
}

// buildFIMMessages creates messages for Fill-in-the-Middle code completion.
// Uses Qwen2.5-Coder FIM token format embedded in the user message.
func (t *GenerateCodeTool) buildFIMMessages(params map[string]interface{}, language, contextContent string) []llm.Message {
	prefix, _ := params["prefix"].(string)
	suffix, _ := params["suffix"].(string)

	systemMsg := fmt.Sprintf(
		"You are a code completion assistant for %s. Complete the code at the cursor position. "+
			"Output ONLY the missing code — no explanations, no fences.",
		language,
	)

	// Build FIM prompt using Qwen2.5-Coder special tokens
	var userContent strings.Builder
	if contextContent != "" {
		userContent.WriteString("Context:\n")
		userContent.WriteString(contextContent)
		userContent.WriteString("\n\nComplete the code between prefix and suffix:\n")
	}
	userContent.WriteString("<|fim_prefix|>")
	userContent.WriteString(prefix)
	userContent.WriteString("<|fim_suffix|>")
	userContent.WriteString(suffix)
	userContent.WriteString("<|fim_middle|>")

	return []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userContent.String()},
	}
}

// buildFileMessages creates messages for full file generation.
func (t *GenerateCodeTool) buildFileMessages(prompt, language, contextContent string) []llm.Message {
	systemMsg := fmt.Sprintf(
		"You are a code generation assistant. Generate a complete, production-ready %s source file. "+
			"Output ONLY the file content — no explanations, no markdown fences. "+
			"Include proper package declaration, imports, and all necessary code.",
		language,
	)

	userContent := prompt
	if contextContent != "" {
		userContent = fmt.Sprintf("Reference files:\n%s\n\nGenerate:\n%s", contextContent, prompt)
	}

	return []llm.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userContent},
	}
}

// gatherContext reads optional context files and returns their concatenated content.
func (t *GenerateCodeTool) gatherContext(params map[string]interface{}) string {
	contextFilesRaw, _ := params["context_files"].(string)
	if contextFilesRaw == "" {
		return ""
	}

	var sb strings.Builder
	files := strings.Split(contextFilesRaw, ",")
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		fullPath := filepath.Join(t.baseDir, f)
		if !isInsideDir(t.baseDir, fullPath) {
			continue
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue // skip unreadable files silently
		}

		// Truncate large files to keep context manageable
		text := string(content)
		if len(text) > 8000 {
			text = text[:8000] + "\n... (truncated)"
		}

		sb.WriteString(fmt.Sprintf("// File: %s\n", filepath.ToSlash(f)))
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// cleanCodeOutput strips common wrapper artifacts from model output.
func cleanCodeOutput(s string) string {
	s = strings.TrimSpace(s)

	// Strip markdown code fences if the model wrapped output
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}

	return s
}
