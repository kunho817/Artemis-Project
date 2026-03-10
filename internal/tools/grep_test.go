package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "package main\n\nfunc hello() {\n\tfmt.Println(\"hello\")\n}\n")

	tool := &GrepTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "hello",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("tool error: %s", result.Error)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Errorf("expected match content, got: %s", result.Content)
	}
}

func TestGrep_RegexPattern(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "func foo() {}\nfunc bar() {}\nvar baz = 1\n")

	tool := &GrepTool{baseDir: dir}
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "func \\w+\\(",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should find both func declarations
	if !strings.Contains(result.Content, "foo") || !strings.Contains(result.Content, "bar") {
		t.Errorf("expected both func matches, got: %s", result.Content)
	}
	// Should not match "var baz"
	if strings.Contains(result.Content, "baz") {
		t.Errorf("unexpected match for 'baz'")
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "Hello World\nhello world\nHELLO WORLD\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern":     "hello",
		"ignore_case": true,
	})

	if !strings.Contains(result.Content, "3 matches") {
		t.Errorf("expected 3 matches with case-insensitive, got: %s", result.Content)
	}
}

func TestGrep_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "foo bar baz\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "nonexistent",
	})

	if !strings.Contains(result.Content, "No matches") {
		t.Errorf("expected no matches message, got: %s", result.Content)
	}
}

func TestGrep_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "foo\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "[invalid",
	})

	if result.Error == "" {
		t.Error("expected error for invalid regex")
	}
}

func TestGrep_ContextLines(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.txt", "line1\nline2\nMATCH\nline4\nline5\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern":       "MATCH",
		"context_lines": float64(1),
	})

	if !strings.Contains(result.Content, "line2") || !strings.Contains(result.Content, "line4") {
		t.Errorf("expected context lines, got: %s", result.Content)
	}
}

func TestGrep_ExtensionFilter(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "test.go", "findme\n")
	writeTestFile(t, dir, "test.txt", "findme\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "findme",
		"include": ".go",
	})

	if !strings.Contains(result.Content, "test.go") {
		t.Errorf("expected test.go match, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "test.txt") {
		t.Errorf("should not match test.txt with .go filter")
	}
}

func TestGrep_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.MkdirAll(gitDir, 0755)
	writeTestFile(t, dir, ".git/config", "findme\n")
	writeTestFile(t, dir, "main.go", "findme\n")

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "findme",
	})

	if strings.Contains(result.Content, ".git") {
		t.Errorf("should skip .git directory")
	}
	if !strings.Contains(result.Content, "main.go") {
		t.Errorf("should find in main.go")
	}
}

func TestGrep_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	tool := &GrepTool{baseDir: dir}
	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "test",
		"path":    "../../../etc",
	})

	if result.Error == "" {
		t.Error("expected path traversal error")
	}
}
