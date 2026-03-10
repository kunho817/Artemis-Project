package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchFile_Insert(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nline3")

	tool := &PatchFileTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{
				"op":      "insert",
				"line":    float64(2),
				"content": "inserted",
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}

	content := readTestFile(t, dir, "test.txt")
	lines := strings.Split(content, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
	if lines[1] != "inserted" {
		t.Errorf("expected 'inserted' at line 2, got %q", lines[1])
	}
}

func TestPatchFile_Replace(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nline3")

	tool := &PatchFileTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{
				"op":      "replace",
				"line":    float64(2),
				"content": "replaced",
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}

	content := readTestFile(t, dir, "test.txt")
	lines := strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[1] != "replaced" {
		t.Errorf("expected 'replaced' at line 2, got %q", lines[1])
	}
}

func TestPatchFile_ReplaceRange(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nline3\nline4\nline5")

	tool := &PatchFileTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{
				"op":       "replace",
				"line":     float64(2),
				"end_line": float64(4),
				"content":  "new_content",
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}

	content := readTestFile(t, dir, "test.txt")
	lines := strings.Split(content, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[1] != "new_content" {
		t.Errorf("expected 'new_content', got %q", lines[1])
	}
}

func TestPatchFile_Delete(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nline3")

	tool := &PatchFileTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{
				"op":   "delete",
				"line": float64(2),
			},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}

	content := readTestFile(t, dir, "test.txt")
	lines := strings.Split(content, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" || lines[1] != "line3" {
		t.Errorf("unexpected content: %v", lines)
	}
}

func TestPatchFile_MultipleOps(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nline3\nline4\nline5")

	tool := &PatchFileTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{"op": "delete", "line": float64(2)},
			map[string]interface{}{"op": "replace", "line": float64(4), "content": "replaced4"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}

	content := readTestFile(t, dir, "test.txt")
	lines := strings.Split(content, "\n")
	// Line 4 replaced first (bottom-up), then line 2 deleted
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
}

func TestPatchFile_InvalidOp(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1")

	tool := &PatchFileTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{"op": "invalid", "line": float64(1)},
		},
	})

	if result.Error == "" {
		t.Error("expected error for invalid op")
	}
}

func TestPatchFile_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1")

	tool := &PatchFileTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
		"operations": []interface{}{
			map[string]interface{}{"op": "replace", "line": float64(999), "content": "x"},
		},
	})

	if result.Error == "" {
		t.Error("expected error for out-of-range line")
	}
}

func TestPatchFile_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	tool := &PatchFileTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"path": "../../../etc/passwd",
		"operations": []interface{}{
			map[string]interface{}{"op": "insert", "line": float64(1), "content": "x"},
		},
	})

	if result.Error == "" {
		t.Error("expected path traversal error")
	}
}

func TestSortOpsReverse(t *testing.T) {
	ops := []patchOp{
		{Line: 1}, {Line: 5}, {Line: 3},
	}
	sortOpsReverse(ops)
	if ops[0].Line != 5 || ops[1].Line != 3 || ops[2].Line != 1 {
		t.Errorf("expected descending order, got %v", ops)
	}
}

// --- Helpers ---

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

func readTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	return string(data)
}
