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

func (t *ListDirTool) Name() string { return "list_dir" }
func (t *ListDirTool) Description() string {
	return "List files and directories in a path (max 200 entries)"
}
func (t *ListDirTool) Parameters() string {
	return "path (string, optional, default \".\") — relative directory path; limit (number, optional, default 200) — max entries to show; include (string, optional) — file extension filter e.g. \".go\""
}

func (t *ListDirTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path := "."
	if p, ok := params["path"].(string); ok && p != "" {
		path = p
	}

	// Parse limit parameter
	limit := 200
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Parse include filter parameter
	include := ""
	if inc, ok := params["include"].(string); ok {
		include = inc
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
	shown := 0
	total := len(entries)
	for _, entry := range entries {
		if shown >= limit {
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		// Filter by include extension if specified
		if include != "" && !entry.IsDir() && !strings.HasSuffix(name, include) {
			continue
		}
		info, _ := entry.Info()
		if info != nil {
			sb.WriteString(fmt.Sprintf("%-40s %8d bytes\n", name, info.Size()))
		} else {
			sb.WriteString(name + "\n")
		}
		shown++
	}

	if sb.Len() == 0 {
		return ToolResult{Content: "(empty directory)"}, nil
	}

	result := sb.String()
	if shown >= limit && shown < total {
		result += fmt.Sprintf("\nShowing %d of %d entries\n", shown, total)
	}

	return ToolResult{Content: result}, nil
}
