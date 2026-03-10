package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads the contents of a file.
type ReadFileTool struct {
	baseDir string
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the contents of a file" }
func (t *ReadFileTool) Parameters() string  { return "path (string, required) — relative file path" }

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Error: "missing required parameter: path"}, nil
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(path))

	// Security: ensure path is within base directory
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read file: %s", err)}, nil
	}

	content := string(data)
	const maxSize = 100 * 1024 // 100KB
	if len(content) > maxSize {
		content = content[:maxSize] + "\n... (truncated, file too large)"
	}

	return ToolResult{
		Content:   content,
		FilesRead: []string{path},
	}, nil
}

// isInsideDir checks whether fullPath is inside baseDir.
func isInsideDir(baseDir, fullPath string) bool {
	absBase, err1 := filepath.Abs(baseDir)
	absPath, err2 := filepath.Abs(fullPath)
	if err1 != nil || err2 != nil {
		return false
	}
	// Normalize separators for comparison
	absBase = filepath.Clean(absBase)
	absPath = filepath.Clean(absPath)

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
