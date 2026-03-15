// Package mcp implements a lightweight MCP (Model Context Protocol) client.
// Communicates with MCP servers via JSON-RPC 2.0 over stdin/stdout (stdio transport).
// No external dependencies — follows the same pattern as internal/lsp/client.go.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// --- JSON-RPC 2.0 types (shared with LSP) ---

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
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// --- MCP Protocol Types ---

// ServerInfo describes an MCP server's identity.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ToolDef describes a tool offered by an MCP server.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"` // JSON Schema for parameters
}

// ToolResult is the result of calling an MCP tool.
type ToolResult struct {
	Content []ContentBlock `json:"content,omitempty"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock represents a piece of content in an MCP response.
type ContentBlock struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// Resource describes a resource offered by an MCP server.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// --- MCP Client ---

// Client communicates with a single MCP server process via JSON-RPC over stdio.
type Client struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	stderr     io.ReadCloser
	serverName string
	serverID   string // user-assigned ID for this connection

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *jsonRPCResponse
	closed  int32

	// Cached capabilities from initialization
	tools     []ToolDef
	resources []Resource
}

// NewClient starts an MCP server process and establishes communication.
func NewClient(ctx context.Context, id, command string, args []string, env map[string]string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

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
		return nil, fmt.Errorf("start MCP server %s: %w", command, err)
	}

	c := &Client{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   bufio.NewReaderSize(stdout, 64*1024),
		stderr:   stderr,
		serverID: id,
		pending:  make(map[int64]chan *jsonRPCResponse),
	}

	// Start response reader
	go c.readLoop()
	// Drain stderr
	go io.Copy(io.Discard, stderr)

	return c, nil
}

// Initialize performs the MCP initialize handshake.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "Artemis",
			"version": "0.1.0",
		},
	}

	var result struct {
		ProtocolVersion string     `json:"protocolVersion"`
		ServerInfo      ServerInfo `json:"serverInfo"`
		Capabilities    struct {
			Tools     interface{} `json:"tools,omitempty"`
			Resources interface{} `json:"resources,omitempty"`
		} `json:"capabilities"`
	}

	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	c.serverName = result.ServerInfo.Name

	// Send initialized notification
	_ = c.notify(ctx, "notifications/initialized", nil)

	// Discover tools
	if result.Capabilities.Tools != nil {
		tools, err := c.ListTools(ctx)
		if err == nil {
			c.tools = tools
		}
	}

	// Discover resources
	if result.Capabilities.Resources != nil {
		resources, err := c.ListResources(ctx)
		if err == nil {
			c.resources = resources
		}
	}

	return nil
}

// Shutdown gracefully shuts down the MCP server.
func (c *Client) Shutdown() {
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}
	atomic.StoreInt32(&c.closed, 1)
	c.stdin.Close()
	_ = c.cmd.Wait()
}

// ServerName returns the server's self-reported name.
func (c *Client) ServerName() string { return c.serverName }

// ID returns the user-assigned ID for this connection.
func (c *Client) ID() string { return c.serverID }

// --- MCP Method Wrappers ---

// ListTools returns all tools offered by the server.
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	var result ToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListResources returns all resources offered by the server.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	var result struct {
		Resources []Resource `json:"resources"`
	}
	if err := c.call(ctx, "resources/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ReadResource reads a resource by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ContentBlock, error) {
	params := map[string]interface{}{
		"uri": uri,
	}

	var result struct {
		Contents []ContentBlock `json:"contents"`
	}
	if err := c.call(ctx, "resources/read", params, &result); err != nil {
		return nil, err
	}
	return result.Contents, nil
}

// CachedTools returns the tools discovered during initialization.
func (c *Client) CachedTools() []ToolDef {
	return c.tools
}

// CachedResources returns the resources discovered during initialization.
func (c *Client) CachedResources() []Resource {
	return c.resources
}

// --- JSON-RPC internals ---

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
			return json.Unmarshal(resp.Result, result)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

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

func (c *Client) readLoop() {
	for {
		if atomic.LoadInt32(&c.closed) == 1 {
			return
		}

		contentLen := -1
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
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

		body := make([]byte, contentLen)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			continue
		}

		if resp.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*resp.ID]
			c.mu.Unlock()
			if ok {
				ch <- &resp
			}
		}
		// Server notifications silently ignored for now
	}
}
