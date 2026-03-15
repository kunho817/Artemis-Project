package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artemis-project/artemis/internal/mcp"
)

// MCPTool wraps an MCP server's tool as an Artemis Tool.
type MCPTool struct {
	serverID   string
	serverName string
	toolDef    mcp.ToolDef
	manager    *mcp.Manager
}

func (t *MCPTool) Name() string {
	// Prefix with server ID to avoid conflicts: "mcp_github_create_issue"
	return fmt.Sprintf("mcp_%s_%s", t.serverID, t.toolDef.Name)
}

func (t *MCPTool) Description() string {
	desc := t.toolDef.Description
	if desc == "" {
		desc = "MCP tool from " + t.serverName
	}
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, desc)
}

func (t *MCPTool) Parameters() string {
	if len(t.toolDef.InputSchema) == 0 {
		return "See tool description for parameters"
	}
	// Return the JSON schema as a readable format
	var schema map[string]interface{}
	if err := json.Unmarshal(t.toolDef.InputSchema, &schema); err == nil {
		// Extract properties for readable display
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			var parts []string
			for name, prop := range props {
				if pm, ok := prop.(map[string]interface{}); ok {
					desc := ""
					if d, ok := pm["description"].(string); ok {
						desc = " — " + d
					}
					typ := "any"
					if t, ok := pm["type"].(string); ok {
						typ = t
					}
					parts = append(parts, fmt.Sprintf("%s (%s)%s", name, typ, desc))
				}
			}
			return strings.Join(parts, "; ")
		}
	}
	return string(t.toolDef.InputSchema)
}

func (t *MCPTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	result, err := t.manager.CallTool(ctx, t.serverID, t.toolDef.Name, params)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("MCP tool %s failed: %v", t.toolDef.Name, err)}, nil
	}

	// Extract text content from MCP response
	var sb strings.Builder
	for _, block := range result.Content {
		if block.Text != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(block.Text)
		}
	}

	content := sb.String()
	if result.IsError {
		return ToolResult{Error: content}, nil
	}
	return ToolResult{Content: content}, nil
}
