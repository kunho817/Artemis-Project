package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PatchFileTool applies targeted line-level edits to a file.
// More precise than write_file — allows insert, replace, and delete operations
// without needing to rewrite the entire file.
type PatchFileTool struct {
	baseDir string
}

func (t *PatchFileTool) Name() string { return "patch_file" }
func (t *PatchFileTool) Description() string {
	return "Apply targeted line edits to a file (insert, replace, delete lines without rewriting)"
}
func (t *PatchFileTool) Parameters() string {
	return `path (string, required) — relative file path; operations (array, required) — list of edit operations, each with: op ("insert"|"replace"|"delete"), line (number, 1-based line number), content (string, for insert/replace — the new line content), end_line (number, optional for replace/delete — end of range inclusive)`
}

func (t *PatchFileTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Error: "missing required parameter: path"}, nil
	}

	opsRaw, ok := params["operations"]
	if !ok {
		return ToolResult{Error: "missing required parameter: operations"}, nil
	}

	ops, err := parseOperations(opsRaw)
	if err != nil {
		return ToolResult{Error: err.Error()}, nil
	}

	if len(ops) == 0 {
		return ToolResult{Error: "operations array is empty"}, nil
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(path))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	// Read existing file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read file: %s", err)}, nil
	}

	lines := strings.Split(string(data), "\n")

	// Apply operations in reverse order (bottom-up) so line numbers stay valid
	sortOpsReverse(ops)

	for _, op := range ops {
		lines, err = applyOperation(lines, op)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("operation failed: %s", err)}, nil
		}
	}

	// Write back
	result := strings.Join(lines, "\n")
	if err := os.WriteFile(fullPath, []byte(result), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to write file: %s", err)}, nil
	}

	return ToolResult{
		Content:      fmt.Sprintf("Applied %d operation(s) to %s", len(ops), path),
		FilesChanged: []string{path},
	}, nil
}

type patchOp struct {
	Op      string // "insert", "replace", "delete"
	Line    int    // 1-based start line
	EndLine int    // 1-based end line (0 = same as Line)
	Content string // new content (for insert/replace)
}

func parseOperations(raw interface{}) ([]patchOp, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("operations must be an array")
	}

	var ops []patchOp
	for i, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("operation %d: must be an object", i)
		}

		op := patchOp{}

		// Parse op type
		opStr, ok := m["op"].(string)
		if !ok {
			return nil, fmt.Errorf("operation %d: missing 'op' field", i)
		}
		switch opStr {
		case "insert", "replace", "delete":
			op.Op = opStr
		default:
			return nil, fmt.Errorf("operation %d: invalid op %q (must be insert, replace, or delete)", i, opStr)
		}

		// Parse line number
		lineNum, ok := m["line"].(float64)
		if !ok || lineNum < 1 {
			return nil, fmt.Errorf("operation %d: missing or invalid 'line' (must be >= 1)", i)
		}
		op.Line = int(lineNum)

		// Parse optional end_line
		if el, ok := m["end_line"].(float64); ok && el >= 1 {
			op.EndLine = int(el)
		}

		// Parse content (required for insert and replace)
		if op.Op == "insert" || op.Op == "replace" {
			content, ok := m["content"].(string)
			if !ok {
				return nil, fmt.Errorf("operation %d: 'content' required for %s", i, op.Op)
			}
			op.Content = content
		}

		ops = append(ops, op)
	}

	return ops, nil
}

func applyOperation(lines []string, op patchOp) ([]string, error) {
	idx := op.Line - 1 // convert to 0-based

	switch op.Op {
	case "insert":
		// Insert new lines BEFORE the given line
		if idx < 0 {
			idx = 0
		}
		if idx > len(lines) {
			idx = len(lines)
		}
		newLines := strings.Split(op.Content, "\n")
		result := make([]string, 0, len(lines)+len(newLines))
		result = append(result, lines[:idx]...)
		result = append(result, newLines...)
		result = append(result, lines[idx:]...)
		return result, nil

	case "replace":
		if idx < 0 || idx >= len(lines) {
			return nil, fmt.Errorf("line %d out of range (file has %d lines)", op.Line, len(lines))
		}
		endIdx := idx
		if op.EndLine > 0 {
			endIdx = op.EndLine - 1
		}
		if endIdx >= len(lines) {
			endIdx = len(lines) - 1
		}
		if endIdx < idx {
			endIdx = idx
		}
		newLines := strings.Split(op.Content, "\n")
		result := make([]string, 0, len(lines)-((endIdx-idx)+1)+len(newLines))
		result = append(result, lines[:idx]...)
		result = append(result, newLines...)
		result = append(result, lines[endIdx+1:]...)
		return result, nil

	case "delete":
		if idx < 0 || idx >= len(lines) {
			return nil, fmt.Errorf("line %d out of range (file has %d lines)", op.Line, len(lines))
		}
		endIdx := idx
		if op.EndLine > 0 {
			endIdx = op.EndLine - 1
		}
		if endIdx >= len(lines) {
			endIdx = len(lines) - 1
		}
		if endIdx < idx {
			endIdx = idx
		}
		result := make([]string, 0, len(lines)-(endIdx-idx+1))
		result = append(result, lines[:idx]...)
		result = append(result, lines[endIdx+1:]...)
		return result, nil

	default:
		return nil, fmt.Errorf("unknown operation: %s", op.Op)
	}
}

// sortOpsReverse sorts operations by line number descending,
// so applying them bottom-up preserves line numbers.
func sortOpsReverse(ops []patchOp) {
	for i := 0; i < len(ops); i++ {
		for j := i + 1; j < len(ops); j++ {
			if ops[j].Line > ops[i].Line {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}
}
