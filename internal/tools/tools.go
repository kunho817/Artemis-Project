package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/artemis-project/artemis/internal/llm"
	"github.com/artemis-project/artemis/internal/lsp"
	"github.com/artemis-project/artemis/internal/mcp"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Tool defines the interface for agent-executable tools.
type Tool interface {
	// Name returns the tool's identifier.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// Parameters returns a human-readable description of the tool's parameters.
	Parameters() string

	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error)
}

// ToolResult captures the output of a tool execution.
type ToolResult struct {
	Content      string   // tool output
	Error        string   // error message (if any)
	FilesChanged []string // files that were modified (for Activity panel)
	FilesRead    []string // files that were read
}

// ToolInvocation represents a parsed tool call from an LLM response.
type ToolInvocation struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

// DefaultMaxToolIterations is the safety fallback when no config is set.
const DefaultMaxToolIterations = 50

// ToolExecutor manages and executes agent tools.
type ToolExecutor struct {
	tools           map[string]Tool
	workDir         string
	fileLock        *FileLockManager
	autoCommit      bool         // if true, auto-commit after file writes
	commitLog       []string     // stack of auto-commit hashes for undo
	codeGenProvider llm.Provider // local code-gen model (vLLM) for generate_code tool
	lspManager      *lsp.Manager
	mcpManager      *mcp.Manager
	astGrepPath     string
}

// NewToolExecutor creates a new tool executor with all built-in tools registered.
func NewToolExecutor(workDir string) *ToolExecutor {
	fl := NewFileLockManager()
	te := &ToolExecutor{
		tools:    make(map[string]Tool),
		workDir:  workDir,
		fileLock: fl,
	}
	te.Register(&ReadFileTool{baseDir: workDir})
	te.Register(&WriteFileTool{baseDir: workDir, fileLock: fl})
	te.Register(&PatchFileTool{baseDir: workDir, fileLock: fl})
	te.Register(&ListDirTool{baseDir: workDir})
	te.Register(&SearchFilesTool{baseDir: workDir})
	te.Register(&GrepTool{baseDir: workDir})
	te.Register(&ShellExecTool{baseDir: workDir})
	te.Register(&GitStatusTool{baseDir: workDir})
	te.Register(&GitDiffTool{baseDir: workDir})
	te.Register(&GitLogTool{baseDir: workDir})
	te.Register(&RunTestsTool{baseDir: workDir})
	te.Register(&FindDependenciesTool{baseDir: workDir})
	te.Register(&FindDependentsTool{baseDir: workDir})
	return te
}

// Register adds a tool to the executor.
func (te *ToolExecutor) Register(tool Tool) {
	te.tools[tool.Name()] = tool
}

// Execute runs a tool by name with the given parameters.
// If autoCommit is enabled and the tool changes files, a shadow git commit is created.
func (te *ToolExecutor) Execute(ctx context.Context, name string, params map[string]interface{}) (ToolResult, error) {
	tool, ok := te.tools[name]
	if !ok {
		return ToolResult{Error: "unknown tool: " + name}, fmt.Errorf("unknown tool: %s", name)
	}
	result, err := tool.Execute(ctx, params)

	// Auto-commit changed files for safety
	if te.autoCommit && len(result.FilesChanged) > 0 && result.Error == "" {
		if hash, commitErr := te.shadowCommit(ctx, name, result.FilesChanged); commitErr == nil && hash != "" {
			te.commitLog = append(te.commitLog, hash)
		}
	}

	return result, err
}

// SetAutoCommit enables or disables automatic git commits after file writes.
func (te *ToolExecutor) SetAutoCommit(enabled bool) {
	te.autoCommit = enabled
}

// SetCodeGenProvider configures the local code generation provider
// and registers the generate_code tool.
func (te *ToolExecutor) SetCodeGenProvider(provider llm.Provider) {
	te.codeGenProvider = provider
	te.Register(&GenerateCodeTool{baseDir: te.workDir, provider: provider})
}

// SetLSPManager configures the LSP manager and registers LSP tools.
func (te *ToolExecutor) SetLSPManager(manager *lsp.Manager) {
	te.lspManager = manager
	te.Register(&LSPDiagnosticsTool{baseDir: te.workDir, manager: manager})
	te.Register(&LSPDefinitionTool{baseDir: te.workDir, manager: manager})
	te.Register(&LSPReferencesTool{baseDir: te.workDir, manager: manager})
	te.Register(&LSPHoverTool{baseDir: te.workDir, manager: manager})
	te.Register(&LSPRenameTool{baseDir: te.workDir, manager: manager, fileLock: te.fileLock})
	te.Register(&LSPSymbolsTool{baseDir: te.workDir, manager: manager})
}

// SetMCPManager configures the MCP manager and registers discovered MCP tools.
func (te *ToolExecutor) SetMCPManager(manager *mcp.Manager) {
	te.mcpManager = manager
	// Register all discovered MCP tools
	for _, dt := range manager.DiscoveredTools() {
		te.Register(&MCPTool{
			serverID:   dt.ServerID,
			serverName: dt.ServerName,
			toolDef:    dt.Tool,
			manager:    manager,
		})
	}
}

// SetAstGrep configures ast-grep path and registers AST-aware tools.
func (te *ToolExecutor) SetAstGrep(sgPath string) {
	te.astGrepPath = sgPath
	te.Register(&AstSearchTool{baseDir: te.workDir, sgPath: sgPath})
	te.Register(&AstReplaceTool{baseDir: te.workDir, sgPath: sgPath, fileLock: te.fileLock})
}

// Undo reverts the last auto-committed change using git reset --hard.
// Returns the reverted commit hash, or empty string if nothing to undo.
func (te *ToolExecutor) Undo(ctx context.Context) (string, error) {
	if len(te.commitLog) == 0 {
		return "", fmt.Errorf("nothing to undo")
	}

	hash := te.commitLog[len(te.commitLog)-1]
	te.commitLog = te.commitLog[:len(te.commitLog)-1]

	// git reset --hard HEAD~1
	cmd := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD~1")
	cmd.Dir = te.workDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git reset failed: %s — %s", err, string(output))
	}

	return hash, nil
}

// UndoCount returns the number of auto-commits available for undo.
func (te *ToolExecutor) UndoCount() int {
	return len(te.commitLog)
}

// shadowCommit creates a silent git commit for file changes made by a tool.
func (te *ToolExecutor) shadowCommit(ctx context.Context, toolName string, files []string) (string, error) {
	// git add <files>
	addArgs := append([]string{"add", "--"}, files...)
	addCmd := exec.CommandContext(ctx, "git", addArgs...)
	addCmd.Dir = te.workDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(addCmd)
	}
	if output, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %s — %s", err, string(output))
	}

	// git commit
	msg := fmt.Sprintf("artemis: %s %s", toolName, strings.Join(files, ", "))
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", msg, "--no-verify")
	commitCmd.Dir = te.workDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(commitCmd)
	}
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %s — %s", err, string(output))
	}

	// Get the commit hash
	hashCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	hashCmd.Dir = te.workDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(hashCmd)
	}
	hashOut, err := hashCmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(hashOut)), nil
}

// ToolDescriptions returns a formatted string describing all available tools
// for injection into agent system prompts.
func (te *ToolExecutor) ToolDescriptions() string {
	var sb strings.Builder
	sb.WriteString("\n\nAVAILABLE TOOLS:\n")
	sb.WriteString("You can use tools by including <tool_use> blocks in your response.\n")
	sb.WriteString("Format: <tool_use>{\"tool\": \"tool_name\", \"params\": {\"key\": \"value\"}}</tool_use>\n\n")

	for _, tool := range te.toolList() {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name(), tool.Description()))
		sb.WriteString(fmt.Sprintf("  Parameters: %s\n", tool.Parameters()))
	}

	sb.WriteString("\nIMPORTANT RULES:\n")
	sb.WriteString("- You may include multiple <tool_use> blocks in a single response.\n")
	sb.WriteString("- After each round of tool use, you will receive results and can continue.\n")
	sb.WriteString("- When your task is COMPLETE, provide your final response WITHOUT any <tool_use> tags.\n")
	sb.WriteString("- All file paths are relative to the project root.\n")
	return sb.String()
}

// toolList returns tools in a stable order for consistent prompt generation.
func (te *ToolExecutor) toolList() []Tool {
	order := []string{"read_file", "write_file", "patch_file", "list_dir", "search_files", "grep", "ast_search", "ast_replace", "shell_exec", "git_status", "git_diff", "git_log", "run_tests", "find_dependencies", "find_dependents", "generate_code", "lsp_diagnostics", "lsp_definition", "lsp_references", "lsp_hover", "lsp_rename", "lsp_symbols"}
	var result []Tool
	for _, name := range order {
		if tool, ok := te.tools[name]; ok {
			result = append(result, tool)
		}
	}
	// Add any tools not in the predefined order (e.g., MCP tools) — sorted for consistency
	var extra []string
	for name := range te.tools {
		found := false
		for _, n := range order {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range extra {
		result = append(result, te.tools[name])
	}
	return result
}

// ParseToolInvocations extracts tool invocation blocks from an LLM response.
// Returns the invocations and the cleaned response text (without tool_use blocks).
func ParseToolInvocations(response string) ([]ToolInvocation, string) {
	var invocations []ToolInvocation
	cleaned := response

	for {
		const startTag = "<tool_use>"
		const endTag = "</tool_use>"

		startIdx := strings.Index(cleaned, startTag)
		if startIdx < 0 {
			break
		}

		endIdx := strings.Index(cleaned[startIdx:], endTag)
		if endIdx < 0 {
			break
		}
		endIdx += startIdx

		// Extract JSON between tags
		jsonStr := strings.TrimSpace(cleaned[startIdx+len(startTag) : endIdx])

		var inv ToolInvocation
		if err := json.Unmarshal([]byte(jsonStr), &inv); err == nil && inv.Tool != "" {
			invocations = append(invocations, inv)
		}

		// Remove the tool_use block from cleaned text
		cleaned = cleaned[:startIdx] + cleaned[endIdx+len(endTag):]
	}

	return invocations, strings.TrimSpace(cleaned)
}

// FormatToolResult creates a formatted string for feeding tool results back to the LLM.
func FormatToolResult(toolName string, result ToolResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<tool_result name=%q>\n", toolName))
	if result.Error != "" {
		sb.WriteString(fmt.Sprintf("ERROR: %s\n", result.Error))
	} else {
		sb.WriteString(result.Content)
		if !strings.HasSuffix(result.Content, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("</tool_result>")
	return sb.String()
}

// --- FileLockManager: per-file mutex for concurrent write protection ---

// FileLockManager provides per-filepath mutex locking to prevent
// concurrent file modifications from multiple agents.
type FileLockManager struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// NewFileLockManager creates a new file lock manager.
func NewFileLockManager() *FileLockManager {
	return &FileLockManager{
		locks: make(map[string]*sync.Mutex),
	}
}

// Lock acquires the mutex for the given file path.
func (m *FileLockManager) Lock(path string) {
	m.mu.Lock()
	lock, ok := m.locks[path]
	if !ok {
		lock = &sync.Mutex{}
		m.locks[path] = lock
	}
	m.mu.Unlock()
	lock.Lock()
}

// Unlock releases the mutex for the given file path.
func (m *FileLockManager) Unlock(path string) {
	m.mu.Lock()
	lock, ok := m.locks[path]
	m.mu.Unlock()
	if ok {
		lock.Unlock()
	}
}

// --- Atomic file write: temp file + rename for crash safety ---

// atomicWriteFile writes content to a temporary file then renames it to the target path.
// This prevents file corruption if the process crashes mid-write.
func atomicWriteFile(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".artemis-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	// On Windows, os.Rename fails if target exists — remove first
	if runtime.GOOS == "windows" {
		os.Remove(path)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// --- Shared utility functions ---

// isInsideDir checks whether fullPath is inside baseDir.
func isInsideDir(baseDir, fullPath string) bool {
	absBase, err1 := filepath.Abs(baseDir)
	absPath, err2 := filepath.Abs(fullPath)
	if err1 != nil || err2 != nil {
		return false
	}
	// Normalize separators for comparison
	absBase = filepath.Clean(absBase)
	absPath = filepath.Clean(absPath)

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}
