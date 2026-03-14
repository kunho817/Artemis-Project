package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// EnsureAstGrep resolves an ast-grep binary path using 4-tier fallback:
//  1. Custom path from config (if set)
//  2. System PATH lookup ("sg" then "ast-grep")
//  3. Cached binary in cachePath/sg[.exe]
//  4. Return platform-specific install instructions (no auto-download)
func EnsureAstGrep(customPath, cachePath string) (string, error) {
	var lastErr error

	// Tier 1: custom path
	if customPath != "" {
		if fi, err := os.Stat(customPath); err == nil && !fi.IsDir() {
			return customPath, nil
		}
		lastErr = fmt.Errorf("configured ast-grep path is invalid: %s", customPath)
	}

	// Tier 2: system PATH lookup
	if path, err := exec.LookPath("sg"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("ast-grep"); err == nil {
		return path, nil
	}

	// Tier 3: cached binary
	if cachePath != "" {
		cached := filepath.Join(cachePath, astGrepBinaryName())
		if fi, err := os.Stat(cached); err == nil && !fi.IsDir() {
			return cached, nil
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("ast-grep binary not found")
	}

	return "", fmt.Errorf("failed to resolve ast-grep binary: %w. %s", lastErr, astGrepInstallInstruction())
}

func astGrepBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sg.exe"
	}
	return "sg"
}

func astGrepInstallInstruction() string {
	switch runtime.GOOS {
	case "windows":
		return "Install ast-grep: npm install -g @ast-grep/cli"
	case "darwin":
		return "Install ast-grep: brew install ast-grep"
	default:
		return "Install ast-grep: npm install -g @ast-grep/cli or cargo install ast-grep"
	}
}

// AstSearchTool performs AST-aware code pattern search with ast-grep.
type AstSearchTool struct {
	baseDir string
	sgPath  string
}

func (t *AstSearchTool) Name() string { return "ast_search" }
func (t *AstSearchTool) Description() string {
	return "Search for code patterns using AST-aware matching. More precise than text grep — matches code structure, not just text. Use meta-variables: $VAR matches any single node, $$$ matches multiple nodes."
}
func (t *AstSearchTool) Parameters() string {
	return "pattern (string, required) — AST pattern (e.g., \"fmt.Println($MSG)\", \"func $NAME($$$ARGS) { $$$ }\"); language (string, required) — language (go, python, typescript, javascript, rust, java, etc.); path (string, optional, default \".\") — directory or file to search"
}

func (t *AstSearchTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	if strings.TrimSpace(t.sgPath) == "" {
		return ToolResult{Error: "ast-grep not available. Install: npm install -g @ast-grep/cli"}, nil
	}

	pattern, ok := params["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		return ToolResult{Error: "missing required parameter: pattern"}, nil
	}

	language, ok := params["language"].(string)
	if !ok || strings.TrimSpace(language) == "" {
		return ToolResult{Error: "missing required parameter: language"}, nil
	}

	targetAbs, _, errResult := resolveAstTargetPath(t.baseDir, params)
	if errResult.Error != "" {
		return errResult, nil
	}

	args := []string{"--pattern", pattern, "--lang", language, targetAbs, "--json"}
	out, runErr := runAstGrep(ctx, t.sgPath, t.baseDir, args...)
	if runErr != nil {
		return ToolResult{Error: fmt.Sprintf("ast_search failed: %s", runErr)}, nil
	}

	matches, parseErr := parseAstGrepMatches(out)
	if parseErr != nil {
		return ToolResult{Error: fmt.Sprintf("failed to parse ast-grep output: %s", parseErr)}, nil
	}

	if len(matches) == 0 {
		return ToolResult{Content: fmt.Sprintf("No matches found for pattern: %s", pattern)}, nil
	}

	content := formatAstSearchMatches(t.baseDir, matches)
	content = capToolOutput(content, 100*1024)

	return ToolResult{Content: content}, nil
}

// AstReplaceTool performs AST-aware pattern replacement with ast-grep.
type AstReplaceTool struct {
	baseDir  string
	sgPath   string
	fileLock *FileLockManager
}

func (t *AstReplaceTool) Name() string { return "ast_replace" }
func (t *AstReplaceTool) Description() string {
	return "Replace code patterns using AST-aware matching. Preserves matched content via meta-variables. Example: pattern='console.log($MSG)' rewrite='logger.info($MSG)'. DRY RUN by default — set apply=true to write changes."
}
func (t *AstReplaceTool) Parameters() string {
	return "pattern (string, required) — AST pattern to match; rewrite (string, required) — replacement pattern using same meta-variables; language (string, required) — language; path (string, optional, default \".\") — directory or file; apply (bool, optional, default false) — if true, write changes"
}

func (t *AstReplaceTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	if strings.TrimSpace(t.sgPath) == "" {
		return ToolResult{Error: "ast-grep not available. Install: npm install -g @ast-grep/cli"}, nil
	}

	pattern, ok := params["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		return ToolResult{Error: "missing required parameter: pattern"}, nil
	}

	rewrite, ok := params["rewrite"].(string)
	if !ok || strings.TrimSpace(rewrite) == "" {
		return ToolResult{Error: "missing required parameter: rewrite"}, nil
	}

	language, ok := params["language"].(string)
	if !ok || strings.TrimSpace(language) == "" {
		return ToolResult{Error: "missing required parameter: language"}, nil
	}

	apply := false
	if v, ok := params["apply"].(bool); ok {
		apply = v
	}

	targetAbs, _, errResult := resolveAstTargetPath(t.baseDir, params)
	if errResult.Error != "" {
		return errResult, nil
	}

	previewArgs := []string{"--pattern", pattern, "--rewrite", rewrite, "--lang", language, targetAbs, "--json"}
	previewOut, runErr := runAstGrep(ctx, t.sgPath, t.baseDir, previewArgs...)
	if runErr != nil {
		return ToolResult{Error: fmt.Sprintf("ast_replace preview failed: %s", runErr)}, nil
	}

	matches, parseErr := parseAstGrepMatches(previewOut)
	if parseErr != nil {
		return ToolResult{Error: fmt.Sprintf("failed to parse ast-grep output: %s", parseErr)}, nil
	}

	if len(matches) == 0 {
		return ToolResult{Content: "No replacements found."}, nil
	}

	fileSet := make(map[string]struct{})
	for _, m := range matches {
		rel := astMatchRelPath(t.baseDir, m.File)
		if rel != "" {
			fileSet[rel] = struct{}{}
		}
	}
	files := sortedKeys(fileSet)

	if !apply {
		preview := fmt.Sprintf("Would change %d occurrences in %d files", len(matches), len(files))
		if len(files) > 0 {
			preview += fmt.Sprintf(": %s", strings.Join(files, ", "))
		}
		return ToolResult{Content: capToolOutput(preview, 100*1024)}, nil
	}

	absToLock := make([]string, 0, len(files))
	for _, rel := range files {
		absToLock = append(absToLock, filepath.Join(t.baseDir, filepath.Clean(rel)))
	}
	if t.fileLock != nil {
		for _, p := range absToLock {
			t.fileLock.Lock(p)
		}
		defer func() {
			for i := len(absToLock) - 1; i >= 0; i-- {
				t.fileLock.Unlock(absToLock[i])
			}
		}()
	}

	updateArgs := []string{"--pattern", pattern, "--rewrite", rewrite, "--lang", language, targetAbs, "--update-all"}
	if _, runErr := runAstGrep(ctx, t.sgPath, t.baseDir, updateArgs...); runErr != nil {
		return ToolResult{Error: fmt.Sprintf("ast_replace apply failed: %s", runErr)}, nil
	}

	content := fmt.Sprintf("Applied %d replacements in %d files", len(matches), len(files))
	if len(files) > 0 {
		content += fmt.Sprintf(": %s", strings.Join(files, ", "))
	}

	return ToolResult{
		Content:      capToolOutput(content, 100*1024),
		FilesChanged: files,
	}, nil
}

type astGrepMatch struct {
	File  string       `json:"file"`
	Range astGrepRange `json:"range"`
	Text  string       `json:"text"`
	Lines string       `json:"lines"`
}

type astGrepRange struct {
	Start astGrepPosition `json:"start"`
	End   astGrepPosition `json:"end"`
}

type astGrepPosition struct {
	Line int `json:"line"`
}

func resolveAstTargetPath(baseDir string, params map[string]interface{}) (string, string, ToolResult) {
	target := "."
	if p, ok := params["path"].(string); ok && strings.TrimSpace(p) != "" {
		target = p
	}

	abs := filepath.Join(baseDir, filepath.Clean(target))
	if !isInsideDir(baseDir, abs) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}

	rel, err := filepath.Rel(baseDir, abs)
	if err != nil {
		rel = target
	}

	return abs, filepath.ToSlash(rel), ToolResult{}
}

func runAstGrep(ctx context.Context, sgPath, workDir string, args ...string) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, sgPath, args...)
	cmd.Dir = workDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return stdout.Bytes(), nil
}

func parseAstGrepMatches(raw []byte) ([]astGrepMatch, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return []astGrepMatch{}, nil
	}

	var matches []astGrepMatch
	if err := json.Unmarshal(raw, &matches); err == nil {
		return matches, nil
	}

	// Fallback for versions that wrap matches in an object.
	var wrapped struct {
		Matches []astGrepMatch `json:"matches"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Matches, nil
	}

	return nil, fmt.Errorf("unexpected JSON format")
}

func formatAstSearchMatches(baseDir string, matches []astGrepMatch) string {
	const capMatches = 50
	shown := matches
	if len(shown) > capMatches {
		shown = shown[:capMatches]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matches:\n\n", len(matches)))

	for i, m := range shown {
		line := m.Range.Start.Line + 1
		if line < 1 {
			line = 1
		}
		file := astMatchRelPath(baseDir, m.File)
		if file == "" {
			file = filepath.ToSlash(filepath.Clean(m.File))
		}
		if file == "" {
			file = "(unknown file)"
		}

		snippet := firstSnippetLine(m)
		sb.WriteString(fmt.Sprintf("%s:%d:\n  %s\n", file, line, snippet))
		if i < len(shown)-1 {
			sb.WriteString("\n")
		}
	}

	if len(matches) > capMatches {
		sb.WriteString(fmt.Sprintf("\n... and %d more matches", len(matches)-capMatches))
	}

	return sb.String()
}

func firstSnippetLine(m astGrepMatch) string {
	raw := strings.TrimSpace(m.Text)
	if raw == "" {
		raw = strings.TrimSpace(m.Lines)
	}
	if raw == "" {
		return "(no snippet)"
	}

	lines := strings.Split(raw, "\n")
	snippet := strings.TrimSpace(lines[0])
	if snippet == "" {
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if line != "" {
				snippet = line
				break
			}
		}
	}
	if snippet == "" {
		snippet = "(empty match)"
	}

	if len(snippet) > 160 {
		snippet = snippet[:160] + "..."
	}

	return snippet
}

func astMatchRelPath(baseDir, p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, filepath.Clean(p))
	}
	if !isInsideDir(baseDir, abs) {
		return ""
	}
	rel, err := filepath.Rel(baseDir, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func capToolOutput(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... (truncated to 100KB)"
}
