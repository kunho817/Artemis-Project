package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads the contents of a file with optional line range support.
// Supports offset/limit for reading specific sections of large files.
type ReadFileTool struct {
	baseDir string
}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file (supports line range for large files)"
}
func (t *ReadFileTool) Parameters() string {
	return `path (string, required) — relative file path; offset (number, optional) — starting line number (1-based, default 1); limit (number, optional) — max number of lines to read (default: all, max 2000)`
}

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

	// Parse offset (1-based line number, default 1)
	offset := 1
	if o, ok := params["offset"].(float64); ok && o >= 1 {
		offset = int(o)
	}

	// Parse limit (max lines to return, default 0 = all up to maxLines)
	limit := 0
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	const maxLines = 2000
	const maxFileSize = 500 * 1024 // 500KB fallback for non-line-range reads

	// Get file info
	info, err := os.Stat(fullPath)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to stat file: %s", err)}, nil
	}

	// If no offset/limit specified and file is small, read entire file (fast path)
	if offset == 1 && limit == 0 && info.Size() <= int64(maxFileSize) {
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("failed to read file: %s", err)}, nil
		}
		content := string(data)
		totalLines := strings.Count(content, "\n") + 1

		if totalLines > maxLines {
			// Truncate to maxLines
			lines := strings.SplitN(content, "\n", maxLines+1)
			content = strings.Join(lines[:maxLines], "\n")
			return ToolResult{
				Content:   fmt.Sprintf("[%s — %d total lines, showing 1-%d]\n\n%s", path, totalLines, maxLines, content),
				FilesRead: []string{path},
			}, nil
		}

		return ToolResult{
			Content:   content,
			FilesRead: []string{path},
		}, nil
	}

	// Line-range read: scan file line by line
	file, err := os.Open(fullPath)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to open file: %s", err)}, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase scanner buffer for long lines
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var sb strings.Builder
	lineNum := 0
	linesRead := 0
	totalLines := 0
	effectiveLimit := maxLines
	if limit > 0 && limit < maxLines {
		effectiveLimit = limit
	}

	for scanner.Scan() {
		lineNum++
		totalLines = lineNum

		if lineNum < offset {
			continue
		}

		if linesRead >= effectiveLimit {
			// Keep counting total lines
			continue
		}

		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
		linesRead++
	}

	// Count remaining lines
	for scanner.Scan() {
		totalLines++
	}

	if err := scanner.Err(); err != nil {
		return ToolResult{Error: fmt.Sprintf("error reading file: %s", err)}, nil
	}

	content := sb.String()
	endLine := offset + linesRead - 1

	header := fmt.Sprintf("[%s — %d total lines, showing %d-%d (%d lines)]",
		path, totalLines, offset, endLine, linesRead)

	if linesRead < totalLines-(offset-1) {
		header += " (truncated)"
	}

	return ToolResult{
		Content:   header + "\n\n" + content,
		FilesRead: []string{path},
	}, nil
}
