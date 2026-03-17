package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// --- JSON-RPC 2.0 types ---

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// --- LSP Protocol Types (subset we actually use) ---

// Position in a text document (0-based line and character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location links a range to a URI.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic represents a compiler error or warning.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// DiagnosticSeverityName returns a human-readable severity name.
func DiagnosticSeverityName(sev int) string {
	switch sev {
	case 1:
		return "Error"
	case 2:
		return "Warning"
	case 3:
		return "Info"
	case 4:
		return "Hint"
	default:
		return "Unknown"
	}
}

// SymbolInformation represents a symbol in the workspace.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// SymbolKindName returns a human-readable symbol kind name.
func SymbolKindName(kind int) string {
	names := map[int]string{
		1: "File", 2: "Module", 3: "Namespace", 4: "Package",
		5: "Class", 6: "Method", 7: "Property", 8: "Field",
		9: "Constructor", 10: "Enum", 11: "Interface", 12: "Function",
		13: "Variable", 14: "Constant", 15: "String", 16: "Number",
		17: "Boolean", 18: "Array", 19: "Object", 20: "Key",
		21: "Null", 22: "EnumMember", 23: "Struct", 24: "Event",
		25: "Operator", 26: "TypeParameter",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return fmt.Sprintf("Kind(%d)", kind)
}

// DocumentSymbol represents a symbol in a document (hierarchical).
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// HoverResult contains hover information.
type HoverResult struct {
	Contents interface{} `json:"contents"` // string | MarkupContent | []MarkedString
}

// TextEdit represents a change to a text document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit represents changes across multiple documents.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"` // URI → edits
}

// --- LSP Client ---

// Client communicates with a single LSP server process via JSON-RPC over stdin/stdout.
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	rootURI string
	lang    string

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan *jsonRPCResponse
	closed   int32
	initDone bool

	// Published diagnostics from the server (URI → diagnostics)
	diagMu      sync.RWMutex
	diagnostics map[string][]Diagnostic
}

// NewClient starts an LSP server process and establishes JSON-RPC communication.
func NewClient(ctx context.Context, lang, command string, args []string, rootDir string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = rootDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	absRoot, _ := filepath.Abs(rootDir)
	rootURI := pathToURI(absRoot)

	c := &Client{
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewReaderSize(stdout, 64*1024),
		stderr:      stderr,
		rootURI:     rootURI,
		lang:        lang,
		pending:     make(map[int64]chan *jsonRPCResponse),
		diagnostics: make(map[string][]Diagnostic),
	}

	// Start response reader goroutine
	go c.readLoop()
	// Drain stderr to prevent blocking
	go io.Copy(io.Discard, stderr)

	return c, nil
}

// Initialize performs the LSP initialize/initialized handshake.
func (c *Client) Initialize(ctx context.Context) error {
	if c.initDone {
		return nil
	}

	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   c.rootURI,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"publishDiagnostics": map[string]interface{}{},
				"hover":              map[string]interface{}{},
				"definition":         map[string]interface{}{},
				"references":         map[string]interface{}{},
				"documentSymbol":     map[string]interface{}{},
				"rename": map[string]interface{}{
					"prepareSupport": true,
				},
			},
			"workspace": map[string]interface{}{
				"symbol":           map[string]interface{}{},
				"workspaceFolders": true,
				"applyEdit":        true,
			},
		},
		"workspaceFolders": []map[string]interface{}{
			{"uri": c.rootURI, "name": filepath.Base(c.rootURI)},
		},
	}

	var result json.RawMessage
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Send initialized notification
	if err := c.notify(ctx, "initialized", map[string]interface{}{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}

	c.initDone = true
	return nil
}

// Shutdown gracefully shuts down the LSP server.
func (c *Client) Shutdown(ctx context.Context) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil
	}
	atomic.StoreInt32(&c.closed, 1)

	// Try graceful shutdown first
	var result json.RawMessage
	_ = c.call(ctx, "shutdown", nil, &result)
	_ = c.notify(ctx, "exit", nil)

	c.stdin.Close()
	err := c.cmd.Wait()

	// Force kill if still running (prevents orphaned processes)
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return err
}

// Language returns the language this client handles.
func (c *Client) Language() string {
	return c.lang
}

// --- LSP Method Wrappers ---

// Definition returns the definition location(s) for a symbol at the given position.
func (c *Client) Definition(ctx context.Context, file string, line, col int) ([]Location, error) {
	params := c.textDocPosition(file, line, col)

	var result json.RawMessage
	if err := c.call(ctx, "textDocument/definition", params, &result); err != nil {
		return nil, err
	}

	// Response can be Location | []Location | null
	var locs []Location
	if err := json.Unmarshal(result, &locs); err != nil {
		var single Location
		if err2 := json.Unmarshal(result, &single); err2 == nil {
			locs = []Location{single}
		}
	}
	return locs, nil
}

// References returns all reference locations for a symbol.
func (c *Client) References(ctx context.Context, file string, line, col int, includeDecl bool) ([]Location, error) {
	params := c.textDocPosition(file, line, col)
	params["context"] = map[string]interface{}{
		"includeDeclaration": includeDecl,
	}

	var result json.RawMessage
	if err := c.call(ctx, "textDocument/references", params, &result); err != nil {
		return nil, err
	}

	var locs []Location
	if err := json.Unmarshal(result, &locs); err != nil {
		return nil, err
	}
	return locs, nil
}

// Hover returns hover information (type, docs) for a position.
func (c *Client) Hover(ctx context.Context, file string, line, col int) (string, error) {
	params := c.textDocPosition(file, line, col)

	var result json.RawMessage
	if err := c.call(ctx, "textDocument/hover", params, &result); err != nil {
		return "", err
	}

	if string(result) == "null" {
		return "", nil
	}

	// Parse hover result — contents can be string, MarkupContent, or []MarkedString
	var hover struct {
		Contents interface{} `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return "", err
	}

	return extractHoverContent(hover.Contents), nil
}

// DocumentSymbols returns the symbols in a document.
func (c *Client) DocumentSymbols(ctx context.Context, file string) ([]DocumentSymbol, error) {
	absFile, _ := filepath.Abs(file)
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": pathToURI(absFile),
		},
	}

	var result json.RawMessage
	if err := c.call(ctx, "textDocument/documentSymbol", params, &result); err != nil {
		return nil, err
	}

	// Response can be DocumentSymbol[] or SymbolInformation[]
	var docSyms []DocumentSymbol
	if err := json.Unmarshal(result, &docSyms); err != nil {
		// Fall back to SymbolInformation format
		var symInfos []SymbolInformation
		if err2 := json.Unmarshal(result, &symInfos); err2 == nil {
			for _, si := range symInfos {
				docSyms = append(docSyms, DocumentSymbol{
					Name:           si.Name,
					Kind:           si.Kind,
					Range:          si.Location.Range,
					SelectionRange: si.Location.Range,
				})
			}
		}
	}
	return docSyms, nil
}

// WorkspaceSymbols searches for symbols across the workspace.
func (c *Client) WorkspaceSymbols(ctx context.Context, query string) ([]SymbolInformation, error) {
	params := map[string]interface{}{
		"query": query,
	}

	var result json.RawMessage
	if err := c.call(ctx, "workspace/symbol", params, &result); err != nil {
		return nil, err
	}

	var symbols []SymbolInformation
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

// Rename renames a symbol across the workspace and returns the edits to apply.
func (c *Client) Rename(ctx context.Context, file string, line, col int, newName string) (*WorkspaceEdit, error) {
	params := c.textDocPosition(file, line, col)
	params["newName"] = newName

	var result json.RawMessage
	if err := c.call(ctx, "textDocument/rename", params, &result); err != nil {
		return nil, err
	}

	var edit WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return nil, err
	}
	return &edit, nil
}

// PublishedDiagnostics returns the latest diagnostics for a file.
func (c *Client) PublishedDiagnostics(file string) []Diagnostic {
	absFile, _ := filepath.Abs(file)
	uri := pathToURI(absFile)

	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	return c.diagnostics[uri]
}

// DidOpen notifies the server that a document was opened.
func (c *Client) DidOpen(ctx context.Context, file, languageID, text string) error {
	absFile, _ := filepath.Abs(file)
	return c.notify(ctx, "textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        pathToURI(absFile),
			"languageId": languageID,
			"version":    1,
			"text":       text,
		},
	})
}

// DidClose notifies the server that a document was closed.
func (c *Client) DidClose(ctx context.Context, file string) error {
	absFile, _ := filepath.Abs(file)
	return c.notify(ctx, "textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": pathToURI(absFile),
		},
	})
}

// --- JSON-RPC internals ---

// call sends a JSON-RPC request and waits for the response.
func (c *Client) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("client is closed")
	}

	id := atomic.AddInt64(&c.nextID, 1)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	respCh := make(chan *jsonRPCResponse, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.send(req); err != nil {
		return fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && resp.Result != nil {
			raw, ok := result.(*json.RawMessage)
			if ok {
				*raw = resp.Result
				return nil
			}
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(ctx context.Context, method string, params interface{}) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil
	}

	msg := struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	return c.send(msg)
}

// send writes a JSON-RPC message with LSP Content-Length header.
func (c *Client) send(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	if _, err := c.stdin.Write(body); err != nil {
		return err
	}
	return nil
}

// readLoop reads JSON-RPC responses from stdout and dispatches them.
func (c *Client) readLoop() {
	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return
		}

		// Read Content-Length header
		contentLen := -1
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break // End of headers
			}
			if strings.HasPrefix(line, "Content-Length:") {
				valStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				if n, err := strconv.Atoi(valStr); err == nil {
					contentLen = n
				}
			}
		}

		if contentLen <= 0 {
			continue
		}

		// Read body
		body := make([]byte, contentLen)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			// Might be a notification from server
			c.handleServerMessage(body)
			continue
		}

		// Dispatch response to caller
		if resp.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*resp.ID]
			c.mu.Unlock()
			if ok {
				ch <- &resp
			}
		} else {
			// Server notification (e.g., diagnostics)
			c.handleServerMessage(body)
		}
	}
}

// handleServerMessage processes notifications from the server.
func (c *Client) handleServerMessage(body []byte) {
	var msg struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return
	}

	switch msg.Method {
	case "textDocument/publishDiagnostics":
		var params struct {
			URI         string       `json:"uri"`
			Diagnostics []Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(msg.Params, &params); err == nil {
			c.diagMu.Lock()
			c.diagnostics[params.URI] = params.Diagnostics
			c.diagMu.Unlock()
		}
	}
}

// --- Helpers ---

// textDocPosition creates a TextDocumentPositionParams map.
func (c *Client) textDocPosition(file string, line, col int) map[string]interface{} {
	absFile, _ := filepath.Abs(file)
	return map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": pathToURI(absFile),
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": col,
		},
	}
}

// pathToURI converts a file path to a file:// URI.
func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path // Windows drive letter: C:/foo → /C:/foo
	}
	return "file://" + path
}

// uriToPath converts a file:// URI back to a file path.
func uriToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) > 2 && path[2] == ':' {
		path = path[1:] // /C:/foo → C:/foo
	}
	return filepath.FromSlash(path)
}

// extractHoverContent extracts displayable text from hover contents.
func extractHoverContent(contents interface{}) string {
	switch v := contents.(type) {
	case string:
		return v
	case map[string]interface{}:
		// MarkupContent: {kind: "markdown", value: "..."}
		if val, ok := v["value"].(string); ok {
			return val
		}
	case []interface{}:
		var parts []string
		for _, item := range v {
			switch m := item.(type) {
			case string:
				parts = append(parts, m)
			case map[string]interface{}:
				if val, ok := m["value"].(string); ok {
					parts = append(parts, val)
				}
			}
		}
		return strings.Join(parts, "\n\n")
	}
	// Last resort: marshal back to JSON
	b, _ := json.Marshal(contents)
	return string(b)
}
