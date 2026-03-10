package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Tool defines the interface for agent-executable tools.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// Parameters returns a human-readable description of the tool's parameters.
	Parameters() string

	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error)
}

// ToolResult captures the output of a tool execution.
type ToolResult struct {
	Content      string   // tool output
	Error        string   // error message (if any)
	FilesChanged []string // files that were modified (for Activity panel)
	FilesRead    []string // files that were read
}

// ToolInvocation represents a parsed tool call from an LLM response.
type ToolInvocation struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

// DefaultMaxToolIterations is the safety fallback when no config is set.
const DefaultMaxToolIterations = 50

// ToolExecutor manages and executes agent tools.
type ToolExecutor struct {
	tools   map[string]Tool
	workDir string
}

// NewToolExecutor creates a new tool executor with all built-in tools registered.
func NewToolExecutor(workDir string) *ToolExecutor {
	te := &ToolExecutor{
		tools:   make(map[string]Tool),
		workDir: workDir,
	}
	te.Register(&ReadFileTool{baseDir: workDir})
	te.Register(&WriteFileTool{baseDir: workDir})
	te.Register(&ListDirTool{baseDir: workDir})
	te.Register(&ShellExecTool{baseDir: workDir})
	te.Register(&SearchFilesTool{baseDir: workDir})
	return te
}

// Register adds a tool to the executor.
func (te *ToolExecutor) Register(tool Tool) {
	te.tools[tool.Name()] = tool
}

// Execute runs a tool by name with the given parameters.
func (te *ToolExecutor) Execute(ctx context.Context, name string, params map[string]interface{}) (ToolResult, error) {
	tool, ok := te.tools[name]
	if !ok {
		return ToolResult{Error: "unknown tool: " + name}, fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, params)
}

// ToolDescriptions returns a formatted string describing all available tools
// for injection into agent system prompts.
func (te *ToolExecutor) ToolDescriptions() string {
	var sb strings.Builder
	sb.WriteString("\n\nAVAILABLE TOOLS:\n")
	sb.WriteString("You can use tools by including <tool_use> blocks in your response.\n")
	sb.WriteString("Format: <tool_use>{\"tool\": \"tool_name\", \"params\": {\"key\": \"value\"}}</tool_use>\n\n")

	for _, tool := range te.toolList() {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name(), tool.Description()))
		sb.WriteString(fmt.Sprintf("  Parameters: %s\n", tool.Parameters()))
	}

	sb.WriteString("\nIMPORTANT RULES:\n")
	sb.WriteString("- You may include multiple <tool_use> blocks in a single response.\n")
	sb.WriteString("- After each round of tool use, you will receive results and can continue.\n")
	sb.WriteString("- When your task is COMPLETE, provide your final response WITHOUT any <tool_use> tags.\n")
	sb.WriteString("- All file paths are relative to the project root.\n")
	return sb.String()
}

// toolList returns tools in a stable order for consistent prompt generation.
func (te *ToolExecutor) toolList() []Tool {
	order := []string{"read_file", "write_file", "list_dir", "search_files", "shell_exec"}
	var result []Tool
	for _, name := range order {
		if tool, ok := te.tools[name]; ok {
			result = append(result, tool)
		}
	}
	// Add any tools not in the predefined order
	for name, tool := range te.tools {
		found := false
		for _, n := range order {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			result = append(result, tool)
		}
	}
	return result
}

// ParseToolInvocations extracts tool invocation blocks from an LLM response.
// Returns the invocations and the cleaned response text (without tool_use blocks).
func ParseToolInvocations(response string) ([]ToolInvocation, string) {
	var invocations []ToolInvocation
	cleaned := response

	for {
		const startTag = "<tool_use>"
		const endTag = "</tool_use>"

		startIdx := strings.Index(cleaned, startTag)
		if startIdx < 0 {
			break
		}

		endIdx := strings.Index(cleaned[startIdx:], endTag)
		if endIdx < 0 {
			break
		}
		endIdx += startIdx

		// Extract JSON between tags
		jsonStr := strings.TrimSpace(cleaned[startIdx+len(startTag) : endIdx])

		var inv ToolInvocation
		if err := json.Unmarshal([]byte(jsonStr), &inv); err == nil && inv.Tool != "" {
			invocations = append(invocations, inv)
		}

		// Remove the tool_use block from cleaned text
		cleaned = cleaned[:startIdx] + cleaned[endIdx+len(endTag):]
	}

	return invocations, strings.TrimSpace(cleaned)
}

// FormatToolResult creates a formatted string for feeding tool results back to the LLM.
func FormatToolResult(toolName string, result ToolResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<tool_result name=%q>\n", toolName))
	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("ERROR: %s\n", result.Error))
	} else {
		sb.WriteString(result.Content)
		if !strings.HasSuffix(result.Content, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("</tool_result>")
	return sb.String()
}
