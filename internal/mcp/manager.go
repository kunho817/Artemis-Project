package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ServerDef defines how to connect to an MCP server.
type ServerDef struct {
	ID      string            `json:"id"`             // unique identifier
	Command string            `json:"command"`        // executable (e.g., "npx", "python")
	Args    []string          `json:"args,omitempty"` // arguments (e.g., ["@modelcontextprotocol/server-github"])
	Env     map[string]string `json:"env,omitempty"`  // environment variables (e.g., {"GITHUB_TOKEN": "..."})
	Enabled bool              `json:"enabled"`        // whether to connect on startup
}

// DiscoveredTool is a tool from an MCP server with its origin tracked.
type DiscoveredTool struct {
	ServerID   string  // which server provides this tool
	ServerName string  // server's self-reported name
	Tool       ToolDef // the tool definition
}

// Manager manages multiple MCP server connections and discovers their tools.
type Manager struct {
	mu      sync.RWMutex
	clients map[string]*Client // serverID → client
	servers []ServerDef        // configured servers
	tools   []DiscoveredTool   // all discovered tools across servers
}

// NewManager creates a new MCP Manager with the given server definitions.
func NewManager(servers []ServerDef) *Manager {
	return &Manager{
		clients: make(map[string]*Client),
		servers: servers,
	}
}

// Connect connects to all enabled MCP servers and discovers their tools.
func (m *Manager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var allTools []DiscoveredTool

	for _, srv := range m.servers {
		if !srv.Enabled {
			continue
		}

		connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

		client, err := NewClient(connCtx, srv.ID, srv.Command, srv.Args, srv.Env)
		if err != nil {
			cancel()
			// Non-fatal — skip this server
			fmt.Printf("MCP: failed to start %s: %v\n", srv.ID, err)
			continue
		}

		if err := client.Initialize(connCtx); err != nil {
			cancel()
			client.Shutdown()
			fmt.Printf("MCP: failed to initialize %s: %v\n", srv.ID, err)
			continue
		}
		cancel()

		m.clients[srv.ID] = client

		// Collect discovered tools
		for _, t := range client.CachedTools() {
			allTools = append(allTools, DiscoveredTool{
				ServerID:   srv.ID,
				ServerName: client.ServerName(),
				Tool:       t,
			})
		}
	}

	m.tools = allTools
	return nil
}

// DiscoveredTools returns all tools discovered from connected servers.
func (m *Manager) DiscoveredTools() []DiscoveredTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tools
}

// CallTool invokes a tool on the appropriate MCP server.
func (m *Manager) CallTool(ctx context.Context, serverID, toolName string, args map[string]interface{}) (*ToolResult, error) {
	m.mu.RLock()
	client, ok := m.clients[serverID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("MCP server %q not connected", serverID)
	}

	return client.CallTool(ctx, toolName, args)
}

// ConnectedServers returns the IDs of connected MCP servers.
func (m *Manager) ConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ids []string
	for id := range m.clients {
		ids = append(ids, id)
	}
	return ids
}

// ServerToolCount returns how many tools a specific server provides.
func (m *Manager) ServerToolCount(serverID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, dt := range m.tools {
		if dt.ServerID == serverID {
			count++
		}
	}
	return count
}

// Shutdown disconnects all MCP servers.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, client := range m.clients {
		client.Shutdown()
		delete(m.clients, id)
	}
	m.tools = nil
}
