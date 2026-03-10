package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SearchFilesTool searches for text patterns across project files.
type SearchFilesTool struct {
	baseDir string
}

func (t *SearchFilesTool) Name() string { return "search_files" }
func (t *SearchFilesTool) Description() string {
	return "Search for a text pattern across project files"
}
func (t *SearchFilesTool) Parameters() string {
	return "pattern (string, required) — text to search for; path (string, optional, default \".\") — directory to search in; include (string, optional) — file extension filter e.g. \".go\""
}

func (t *SearchFilesTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return ToolResult{Error: "missing required parameter: pattern"}, nil
	}

	searchPath := "."
	if p, ok := params["path"].(string); ok && p != "" {
		searchPath = p
	}

	include := ""
	if inc, ok := params["include"].(string); ok {
		include = inc
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(searchPath))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	var sb strings.Builder
	matchCount := 0
	const maxMatches = 100

	err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(t.baseDir, path)

		// Skip hidden directories and common non-searchable paths
		if info.IsDir() {
			if shouldSkipPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldSkipPath(rel) {
			return nil
		}

		// Filter by extension
		if include != "" && !strings.HasSuffix(path, include) {
			return nil
		}

		// Skip binary/large files
		if info.Size() > 1024*1024 { // 1MB
			return nil
		}

		if matchCount >= maxMatches {
			return filepath.SkipAll
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(line, pattern) {
				relPath, _ := filepath.Rel(t.baseDir, path)
				sb.WriteString(fmt.Sprintf("%s:%d: %s\n", filepath.ToSlash(relPath), lineNum, strings.TrimSpace(line)))
				matchCount++
				if matchCount >= maxMatches {
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Error: fmt.Sprintf("search error: %s", err)}, nil
	}

	if matchCount == 0 {
		return ToolResult{Content: fmt.Sprintf("No matches found for %q", pattern)}, nil
	}

	result := sb.String()
	if matchCount >= maxMatches {
		result += fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}

	return ToolResult{Content: result}, nil
}

// shouldSkipPath returns true for paths that shouldn't be searched.
func shouldSkipPath(rel string) bool {
	skip := []string{".git", "node_modules", "vendor", ".idea", ".vscode", "__pycache__"}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		for _, s := range skip {
			if part == s {
				return true
			}
		}
	}
	return false
}
