package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListDirTool lists the contents of a directory.
type ListDirTool struct {
	baseDir string
}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "List files and directories in a path" }
func (t *ListDirTool) Parameters() string {
	return "path (string, optional, default \".\") — relative directory path"
}

func (t *ListDirTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(path))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read directory: %s", err)}, nil
	}

	var sb strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		info, _ := entry.Info()
		if info != nil {
			sb.WriteString(fmt.Sprintf("%-40s %8d bytes\n", name, info.Size()))
		} else {
			sb.WriteString(name + "\n")
		}
	}

	if sb.Len() == 0 {
		return ToolResult{Content: "(empty directory)"}, nil
	}

	return ToolResult{Content: sb.String()}, nil
}
