package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/lsp"
)

// LSPDiagnosticsTool returns compiler/linter diagnostics for a file.
type LSPDiagnosticsTool struct {
	baseDir string
	manager *lsp.Manager
}

func (t *LSPDiagnosticsTool) Name() string { return "lsp_diagnostics" }
func (t *LSPDiagnosticsTool) Description() string {
	return "Get compiler errors and warnings for a file using the Language Server Protocol"
}
func (t *LSPDiagnosticsTool) Parameters() string {
	return `file (string, required) — relative file path; severity (string, optional, default "all") — one of: "error", "warning", "info", "hint", "all"`
}

func (t *LSPDiagnosticsTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	file, ok := params["file"].(string)
	if !ok || file == "" {
		return ToolResult{Error: "missing required parameter: file"}, nil
	}

	absFile, relFile, errResult := t.resolveFile(file)
	if errResult.Error != "" {
		return errResult, nil
	}

	client, errResult := t.clientForFile(ctx, absFile)
	if errResult.Error != "" {
		return errResult, nil
	}

	diagnostics := client.PublishedDiagnostics(absFile)
	if len(diagnostics) == 0 {
		content, err := os.ReadFile(absFile)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("failed to read file: %s", err)}, nil
		}

		lang := lsp.LanguageForFile(absFile)
		languageID := lsp.LanguageIDForLSP(lang)
		if err := client.DidOpen(ctx, absFile, languageID, string(content)); err != nil {
			return ToolResult{Error: fmt.Sprintf("failed to open file in LSP: %s", err)}, nil
		}
		defer client.DidClose(context.Background(), absFile)

		select {
		case <-ctx.Done():
			return ToolResult{Error: ctx.Err().Error()}, nil
		case <-time.After(2 * time.Second):
		}

		diagnostics = client.PublishedDiagnostics(absFile)
	}

	severityFilter := "all"
	if v, ok := params["severity"].(string); ok && v != "" {
		severityFilter = strings.ToLower(strings.TrimSpace(v))
	}

	sev, filterErr := parseSeverityFilter(severityFilter)
	if filterErr != "" {
		return ToolResult{Error: filterErr}, nil
	}

	var lines []string
	for _, d := range diagnostics {
		if sev != 0 && d.Severity != sev {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s:%d:%d %s: %s",
			relFile,
			d.Range.Start.Line+1,
			d.Range.Start.Character+1,
			lsp.DiagnosticSeverityName(d.Severity),
			d.Message,
		))
	}

	if len(lines) == 0 {
		return ToolResult{Content: "No diagnostics found."}, nil
	}

	return ToolResult{Content: strings.Join(lines, "\n")}, nil
}

// LSPDefinitionTool jumps to symbol definitions.
type LSPDefinitionTool struct {
	baseDir string
	manager *lsp.Manager
}

func (t *LSPDefinitionTool) Name() string { return "lsp_definition" }
func (t *LSPDefinitionTool) Description() string {
	return "Jump to the definition of a symbol at a given position"
}
func (t *LSPDefinitionTool) Parameters() string {
	return "file (string, required) — relative file path; line (number, required, 1-based); character (number, required, 1-based)"
}

func (t *LSPDefinitionTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	file, ok := params["file"].(string)
	if !ok || file == "" {
		return ToolResult{Error: "missing required parameter: file"}, nil
	}

	line, lineErr := getRequiredOneBasedInt(params, "line")
	if lineErr != "" {
		return ToolResult{Error: lineErr}, nil
	}
	character, charErr := getRequiredOneBasedInt(params, "character")
	if charErr != "" {
		return ToolResult{Error: charErr}, nil
	}

	absFile, _, errResult := t.resolveFile(file)
	if errResult.Error != "" {
		return errResult, nil
	}

	client, errResult := t.clientForFile(ctx, absFile)
	if errResult.Error != "" {
		return errResult, nil
	}

	if err := didOpenForQuery(ctx, client, absFile); err != nil {
		return ToolResult{Error: err.Error()}, nil
	}
	defer client.DidClose(context.Background(), absFile)

	locs, err := client.Definition(ctx, absFile, line-1, character-1)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("lsp definition failed: %s", err)}, nil
	}

	if len(locs) == 0 {
		return ToolResult{Content: "No definition found."}, nil
	}

	var lines []string
	for _, loc := range locs {
		lines = append(lines, formatLocation(t.baseDir, loc))
	}
	return ToolResult{Content: strings.Join(lines, "\n")}, nil
}

// LSPReferencesTool finds all symbol references.
type LSPReferencesTool struct {
	baseDir string
	manager *lsp.Manager
}

func (t *LSPReferencesTool) Name() string { return "lsp_references" }
func (t *LSPReferencesTool) Description() string {
	return "Find all references to a symbol at a given position across the codebase"
}
func (t *LSPReferencesTool) Parameters() string {
	return "file (string, required) — relative file path; line (number, required, 1-based); character (number, required, 1-based); include_declaration (bool, optional, default true)"
}

func (t *LSPReferencesTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	file, ok := params["file"].(string)
	if !ok || file == "" {
		return ToolResult{Error: "missing required parameter: file"}, nil
	}

	line, lineErr := getRequiredOneBasedInt(params, "line")
	if lineErr != "" {
		return ToolResult{Error: lineErr}, nil
	}
	character, charErr := getRequiredOneBasedInt(params, "character")
	if charErr != "" {
		return ToolResult{Error: charErr}, nil
	}

	includeDecl := true
	if v, ok := params["include_declaration"].(bool); ok {
		includeDecl = v
	}

	absFile, _, errResult := t.resolveFile(file)
	if errResult.Error != "" {
		return errResult, nil
	}

	client, errResult := t.clientForFile(ctx, absFile)
	if errResult.Error != "" {
		return errResult, nil
	}

	if err := didOpenForQuery(ctx, client, absFile); err != nil {
		return ToolResult{Error: err.Error()}, nil
	}
	defer client.DidClose(context.Background(), absFile)

	locs, err := client.References(ctx, absFile, line-1, character-1, includeDecl)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("lsp references failed: %s", err)}, nil
	}

	if len(locs) == 0 {
		return ToolResult{Content: "No references found."}, nil
	}

	var lines []string
	for _, loc := range locs {
		lines = append(lines, formatLocation(t.baseDir, loc))
	}
	return ToolResult{Content: strings.Join(lines, "\n")}, nil
}

// LSPHoverTool returns hover info for a symbol.
type LSPHoverTool struct {
	baseDir string
	manager *lsp.Manager
}

func (t *LSPHoverTool) Name() string { return "lsp_hover" }
func (t *LSPHoverTool) Description() string {
	return "Get type information and documentation for a symbol at a given position"
}
func (t *LSPHoverTool) Parameters() string {
	return "file (string, required) — relative file path; line (number, required, 1-based); character (number, required, 1-based)"
}

func (t *LSPHoverTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	file, ok := params["file"].(string)
	if !ok || file == "" {
		return ToolResult{Error: "missing required parameter: file"}, nil
	}

	line, lineErr := getRequiredOneBasedInt(params, "line")
	if lineErr != "" {
		return ToolResult{Error: lineErr}, nil
	}
	character, charErr := getRequiredOneBasedInt(params, "character")
	if charErr != "" {
		return ToolResult{Error: charErr}, nil
	}

	absFile, _, errResult := t.resolveFile(file)
	if errResult.Error != "" {
		return errResult, nil
	}

	client, errResult := t.clientForFile(ctx, absFile)
	if errResult.Error != "" {
		return errResult, nil
	}

	if err := didOpenForQuery(ctx, client, absFile); err != nil {
		return ToolResult{Error: err.Error()}, nil
	}
	defer client.DidClose(context.Background(), absFile)

	hover, err := client.Hover(ctx, absFile, line-1, character-1)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("lsp hover failed: %s", err)}, nil
	}
	if strings.TrimSpace(hover) == "" {
		return ToolResult{Content: "No hover information."}, nil
	}

	return ToolResult{Content: hover}, nil
}

// LSPRenameTool renames symbols and applies workspace edits.
type LSPRenameTool struct {
	baseDir  string
	manager  *lsp.Manager
	fileLock *FileLockManager
}

func (t *LSPRenameTool) Name() string { return "lsp_rename" }
func (t *LSPRenameTool) Description() string {
	return "Safely rename a symbol across the entire codebase using semantic analysis"
}
func (t *LSPRenameTool) Parameters() string {
	return "file (string, required) — relative file path; line (number, required, 1-based); character (number, required, 1-based); new_name (string, required)"
}

func (t *LSPRenameTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	file, ok := params["file"].(string)
	if !ok || file == "" {
		return ToolResult{Error: "missing required parameter: file"}, nil
	}

	line, lineErr := getRequiredOneBasedInt(params, "line")
	if lineErr != "" {
		return ToolResult{Error: lineErr}, nil
	}
	character, charErr := getRequiredOneBasedInt(params, "character")
	if charErr != "" {
		return ToolResult{Error: charErr}, nil
	}

	newName, ok := params["new_name"].(string)
	if !ok || strings.TrimSpace(newName) == "" {
		return ToolResult{Error: "missing required parameter: new_name"}, nil
	}

	absFile, _, errResult := t.resolveFile(file)
	if errResult.Error != "" {
		return errResult, nil
	}

	client, errResult := t.clientForFile(ctx, absFile)
	if errResult.Error != "" {
		return errResult, nil
	}

	if err := didOpenForQuery(ctx, client, absFile); err != nil {
		return ToolResult{Error: err.Error()}, nil
	}
	defer client.DidClose(context.Background(), absFile)

	workspaceEdit, err := client.Rename(ctx, absFile, line-1, character-1, newName)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("lsp rename failed: %s", err)}, nil
	}

	if workspaceEdit == nil || len(workspaceEdit.Changes) == 0 {
		return ToolResult{Content: "No rename changes generated."}, nil
	}

	type fileChange struct {
		relPath   string
		absPath   string
		editCount int
	}

	var changes []fileChange
	for uri, edits := range workspaceEdit.Changes {
		absPath := uriToPathInline(uri)
		if absPath == "" {
			continue
		}
		if !isInsideDir(t.baseDir, absPath) {
			continue
		}
		rel, err := filepath.Rel(t.baseDir, absPath)
		if err != nil {
			rel = absPath
		}
		changes = append(changes, fileChange{
			relPath:   filepath.ToSlash(rel),
			absPath:   absPath,
			editCount: len(edits),
		})
	}

	if len(changes) == 0 {
		return ToolResult{Content: "No rename changes generated."}, nil
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].relPath < changes[j].relPath })

	for _, change := range changes {
		edits := workspaceEdit.Changes[pathToURIInline(change.absPath)]
		if t.fileLock != nil {
			t.fileLock.Lock(change.absPath)
		}
		if err := applyTextEditsToFile(change.absPath, edits); err != nil {
			if t.fileLock != nil {
				t.fileLock.Unlock(change.absPath)
			}
			return ToolResult{Error: fmt.Sprintf("failed applying rename edits to %s: %s", change.relPath, err)}, nil
		}
		if t.fileLock != nil {
			t.fileLock.Unlock(change.absPath)
		}
	}

	var outLines []string
	filesChanged := make([]string, 0, len(changes))
	for _, ch := range changes {
		outLines = append(outLines, fmt.Sprintf("%s (%d edits)", ch.relPath, ch.editCount))
		filesChanged = append(filesChanged, ch.relPath)
	}

	return ToolResult{
		Content:      fmt.Sprintf("Renamed symbol to %q across %d file(s):\n%s", newName, len(changes), strings.Join(outLines, "\n")),
		FilesChanged: filesChanged,
	}, nil
}

// LSPSymbolsTool lists document symbols or searches workspace symbols.
type LSPSymbolsTool struct {
	baseDir string
	manager *lsp.Manager
}

func (t *LSPSymbolsTool) Name() string { return "lsp_symbols" }
func (t *LSPSymbolsTool) Description() string {
	return "List all symbols in a file or search for symbols across the workspace"
}
func (t *LSPSymbolsTool) Parameters() string {
	return "file (string, optional) — relative file path for document symbols; query (string, optional) — workspace symbol search query"
}

func (t *LSPSymbolsTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	if file, ok := params["file"].(string); ok && strings.TrimSpace(file) != "" {
		absFile, relFile, errResult := t.resolveFile(file)
		if errResult.Error != "" {
			return errResult, nil
		}

		client, errResult := t.clientForFile(ctx, absFile)
		if errResult.Error != "" {
			return errResult, nil
		}

		syms, err := client.DocumentSymbols(ctx, absFile)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("lsp document symbols failed: %s", err)}, nil
		}

		var lines []string
		flattenDocumentSymbols(relFile, syms, &lines)
		if len(lines) == 0 {
			return ToolResult{Content: "No symbols found."}, nil
		}
		return ToolResult{Content: strings.Join(lines, "\n")}, nil
	}

	query, _ := params["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return ToolResult{Error: "missing required parameter: file or query"}, nil
	}

	client, errResult := t.clientForAnyLanguage(ctx)
	if errResult.Error != "" {
		return errResult, nil
	}

	syms, err := client.WorkspaceSymbols(ctx, query)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("lsp workspace symbols failed: %s", err)}, nil
	}

	if len(syms) == 0 {
		return ToolResult{Content: "No symbols found."}, nil
	}

	var lines []string
	for _, s := range syms {
		absPath := uriToPathInline(s.Location.URI)
		relPath := absPath
		if absPath != "" {
			if rel, err := filepath.Rel(t.baseDir, absPath); err == nil {
				relPath = filepath.ToSlash(rel)
			}
		}
		lines = append(lines, fmt.Sprintf("%s (%s) at %s:%d:%d",
			s.Name,
			lsp.SymbolKindName(s.Kind),
			relPath,
			s.Location.Range.Start.Line+1,
			s.Location.Range.Start.Character+1,
		))
	}

	return ToolResult{Content: strings.Join(lines, "\n")}, nil
}

func (t *LSPDiagnosticsTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPDefinitionTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPReferencesTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPHoverTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPRenameTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPSymbolsTool) resolveFile(file string) (string, string, ToolResult) {
	absFile := filepath.Join(t.baseDir, filepath.Clean(file))
	if !isInsideDir(t.baseDir, absFile) {
		return "", "", ToolResult{Error: "path outside project directory"}
	}
	rel, err := filepath.Rel(t.baseDir, absFile)
	if err != nil {
		rel = file
	}
	return absFile, filepath.ToSlash(rel), ToolResult{}
}

func (t *LSPDiagnosticsTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPDefinitionTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPReferencesTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPHoverTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPRenameTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPSymbolsTool) clientForFile(ctx context.Context, absFile string) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	client, err := t.manager.ClientForFile(ctx, absFile)
	if err != nil {
		return nil, ToolResult{Error: fmt.Sprintf("failed to get LSP client: %s", err)}
	}
	if client == nil {
		langs := t.manager.ConfiguredLanguages()
		sort.Strings(langs)
		return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
	}
	return client, ToolResult{}
}

func (t *LSPSymbolsTool) clientForAnyLanguage(ctx context.Context) (*lsp.Client, ToolResult) {
	if t.manager == nil {
		return nil, ToolResult{Error: "LSP manager is not configured."}
	}
	langs := t.manager.ActiveLanguages()
	if len(langs) == 0 {
		langs = t.manager.ConfiguredLanguages()
	}
	if len(langs) == 0 {
		return nil, ToolResult{Error: "No LSP server available for this file type. Supported languages: []"}
	}
	sort.Strings(langs)

	for _, lang := range langs {
		client, err := t.manager.ClientForLanguage(ctx, lang)
		if err != nil {
			continue
		}
		if client != nil {
			return client, ToolResult{}
		}
	}

	return nil, ToolResult{Error: fmt.Sprintf("No LSP server available for this file type. Supported languages: %v", langs)}
}

func didOpenForQuery(ctx context.Context, client *lsp.Client, absFile string) error {
	content, err := os.ReadFile(absFile)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lang := lsp.LanguageForFile(absFile)
	languageID := lsp.LanguageIDForLSP(lang)
	if err := client.DidOpen(ctx, absFile, languageID, string(content)); err != nil {
		return fmt.Errorf("failed to open file in LSP: %w", err)
	}
	return nil
}

func parseSeverityFilter(severity string) (int, string) {
	switch severity {
	case "all":
		return 0, ""
	case "error":
		return 1, ""
	case "warning":
		return 2, ""
	case "info":
		return 3, ""
	case "hint":
		return 4, ""
	default:
		return 0, "invalid severity: must be one of error, warning, info, hint, all"
	}
}

func getRequiredOneBasedInt(params map[string]interface{}, key string) (int, string) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Sprintf("missing required parameter: %s", key)
	}

	var n int
	switch x := v.(type) {
	case float64:
		n = int(x)
	case int:
		n = x
	default:
		return 0, fmt.Sprintf("invalid parameter %s: must be a number", key)
	}

	if n < 1 {
		return 0, fmt.Sprintf("invalid parameter %s: must be >= 1", key)
	}
	return n, ""
}

func formatLocation(baseDir string, loc lsp.Location) string {
	absPath := uriToPathInline(loc.URI)
	relPath := absPath
	if absPath != "" {
		if rel, err := filepath.Rel(baseDir, absPath); err == nil {
			relPath = filepath.ToSlash(rel)
		}
	}
	return fmt.Sprintf("%s:%d:%d", relPath, loc.Range.Start.Line+1, loc.Range.Start.Character+1)
}

func flattenDocumentSymbols(relFile string, symbols []lsp.DocumentSymbol, out *[]string) {
	for _, s := range symbols {
		*out = append(*out, fmt.Sprintf("%s (%s) at %s:%d:%d",
			s.Name,
			lsp.SymbolKindName(s.Kind),
			relFile,
			s.SelectionRange.Start.Line+1,
			s.SelectionRange.Start.Character+1,
		))
		if len(s.Children) > 0 {
			flattenDocumentSymbols(relFile, s.Children, out)
		}
	}
}

func applyTextEditsToFile(absPath string, edits []lsp.TextEdit) error {
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	sorted := make([]lsp.TextEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		a := sorted[i]
		b := sorted[j]
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line > b.Range.Start.Line
		}
		if a.Range.Start.Character != b.Range.Start.Character {
			return a.Range.Start.Character > b.Range.Start.Character
		}
		if a.Range.End.Line != b.Range.End.Line {
			return a.Range.End.Line > b.Range.End.Line
		}
		return a.Range.End.Character > b.Range.End.Character
	})

	for _, edit := range sorted {
		start, err := positionToOffset(content, edit.Range.Start)
		if err != nil {
			return err
		}
		end, err := positionToOffset(content, edit.Range.End)
		if err != nil {
			return err
		}
		if end < start {
			return fmt.Errorf("invalid edit range")
		}
		content = content[:start] + edit.NewText + content[end:]
	}

	return atomicWriteFile(absPath, []byte(content), 0644)
}

func positionToOffset(content string, pos lsp.Position) (int, error) {
	if pos.Line < 0 || pos.Character < 0 {
		return 0, fmt.Errorf("invalid position: line/character must be non-negative")
	}

	lineStarts := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}

	if pos.Line >= len(lineStarts) {
		return 0, fmt.Errorf("line %d out of range", pos.Line)
	}

	lineStart := lineStarts[pos.Line]
	lineEnd := len(content)
	if pos.Line+1 < len(lineStarts) {
		lineEnd = lineStarts[pos.Line+1]
	}

	lineText := content[lineStart:lineEnd]
	lineText = strings.TrimSuffix(lineText, "\n")
	lineText = strings.TrimSuffix(lineText, "\r")

	byteInLine, err := byteIndexAtRune(lineText, pos.Character)
	if err != nil {
		return 0, err
	}

	return lineStart + byteInLine, nil
}

func byteIndexAtRune(s string, runeIdx int) (int, error) {
	if runeIdx < 0 {
		return 0, fmt.Errorf("invalid character index")
	}
	if runeIdx == 0 {
		return 0, nil
	}

	count := 0
	for i := range s {
		if count == runeIdx {
			return i, nil
		}
		count++
	}
	if count == runeIdx {
		return len(s), nil
	}
	return 0, fmt.Errorf("character %d out of range", runeIdx)
}

func pathToURIInline(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}

func uriToPathInline(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) > 2 && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
