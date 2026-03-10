package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileTool writes content to a file (creates parent directories if needed).
type WriteFileTool struct {
	baseDir string
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file (creates or overwrites)"
}
func (t *WriteFileTool) Parameters() string {
	return "path (string, required) — relative file path; content (string, required) — file content"
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Error: "missing required parameter: path"}, nil
	}

	content, ok := params["content"].(string)
	if !ok {
		return ToolResult{Error: "missing required parameter: content"}, nil
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(path))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to create directory: %s", err)}, nil
	}

	// Check if file is new
	_, statErr := os.Stat(fullPath)
	isNew := os.IsNotExist(statErr)

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to write file: %s", err)}, nil
	}

	action := "updated"
	if isNew {
		action = "created"
	}

	return ToolResult{
		Content:      fmt.Sprintf("File %s: %s", action, path),
		FilesChanged: []string{path},
	}, nil
}
