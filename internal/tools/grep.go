package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepTool searches for regex patterns across project files.
// More powerful than search_files: supports regex, context lines, and case-insensitive matching.
type GrepTool struct {
	baseDir string
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return "Search for a regex pattern across project files (supports regex, context lines, case-insensitive)"
}
func (t *GrepTool) Parameters() string {
	return `pattern (string, required) — regex pattern to search for; path (string, optional, default ".") — directory to search in; include (string, optional) — file extension filter e.g. ".go"; context_lines (number, optional, default 0) — lines of context around matches; ignore_case (bool, optional, default false) — case-insensitive search`
}

func (t *GrepTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
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

	contextLines := 0
	if cl, ok := params["context_lines"].(float64); ok {
		contextLines = int(cl)
		if contextLines < 0 {
			contextLines = 0
		}
		if contextLines > 5 {
			contextLines = 5
		}
	}

	ignoreCase := false
	if ic, ok := params["ignore_case"].(bool); ok {
		ignoreCase = ic
	}

	// Compile regex
	regexPattern := pattern
	if ignoreCase {
		regexPattern = "(?i)" + regexPattern
	}
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid regex pattern: %s", err)}, nil
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(searchPath))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	var sb strings.Builder
	matchCount := 0
	fileCount := 0
	const maxMatches = 200

	walkErr := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(t.baseDir, path)

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
		if info.Size() > 1024*1024 {
			return nil
		}

		if matchCount >= maxMatches {
			return filepath.SkipAll
		}

		matches := grepFile(path, re, contextLines, maxMatches-matchCount)
		if len(matches) == 0 {
			return nil
		}

		relPath, _ := filepath.Rel(t.baseDir, path)
		relPath = filepath.ToSlash(relPath)
		fileCount++

		for _, m := range matches {
			sb.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, m.lineNum, m.line))
			for _, cl := range m.context {
				sb.WriteString(fmt.Sprintf("%s:%d  %s\n", relPath, cl.num, cl.text))
			}
			if len(m.context) > 0 {
				sb.WriteString("--\n")
			}
			matchCount++
		}

		return nil
	})

	if walkErr != nil {
		return ToolResult{Error: fmt.Sprintf("search error: %s", walkErr)}, nil
	}

	if matchCount == 0 {
		return ToolResult{Content: fmt.Sprintf("No matches found for /%s/", pattern)}, nil
	}

	header := fmt.Sprintf("Found %d matches in %d files\n\n", matchCount, fileCount)
	result := header + sb.String()
	if matchCount >= maxMatches {
		result += fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}

	return ToolResult{Content: result}, nil
}

type grepMatch struct {
	lineNum int
	line    string
	context []contextLine
}

type contextLine struct {
	num  int
	text string
}

func grepFile(path string, re *regexp.Regexp, contextLines, maxMatches int) []grepMatch {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Read all lines for context support
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	var matches []grepMatch
	for i, line := range lines {
		if len(matches) >= maxMatches {
			break
		}
		if !re.MatchString(line) {
			continue
		}

		m := grepMatch{
			lineNum: i + 1,
			line:    strings.TrimRight(line, "\r\n"),
		}

		// Add context lines
		if contextLines > 0 {
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			end := i + contextLines + 1
			if end > len(lines) {
				end = len(lines)
			}
			for j := start; j < end; j++ {
				if j == i {
					continue // skip the match line itself
				}
				m.context = append(m.context, contextLine{
					num:  j + 1,
					text: strings.TrimRight(lines[j], "\r\n"),
				})
			}
		}

		matches = append(matches, m)
	}

	return matches
}
