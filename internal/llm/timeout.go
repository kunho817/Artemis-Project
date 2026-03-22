package llm

import "time"

// HTTP timeout constants for LLM providers and external services.
// These timeouts prevent infinite waits and ensure consistent behavior across the application.

const (
	// LLMRequestTimeout is the maximum time to wait for an LLM API response.
	// This includes streaming responses and should account for slow models.
	LLMRequestTimeout = 120 * time.Second

	// LSPTimeout is the maximum time to wait for LSP server operations.
	// LSP operations should generally complete quickly (< 1s for most operations).
	LSPTimeout = 30 * time.Second

	// MCPTimeout is the maximum time to wait for MCP server operations.
	// MCP tool calls may take longer for complex operations.
	MCPTimeout = 30 * time.Second

	// HTTPClientTimeout is the default timeout for general HTTP clients.
	// Used for GitHub API, file downloads, and other web requests.
	HTTPClientTimeout = 60 * time.Second

	// StreamChunkTimeout is the maximum time to wait between stream chunks.
	// If no chunk arrives within this time, the stream is considered dead.
	StreamChunkTimeout = 30 * time.Second

	// DialTimeout is the maximum time to establish a TCP connection.
	// This is part of the overall HTTPClientTimeout.
	DialTimeout = 10 * time.Second

	// TLSHandshakeTimeout is the maximum time to complete the TLS handshake.
	// This is part of the overall HTTPClientTimeout.
	TLSHandshakeTimeout = 10 * time.Second

	// ResponseHeaderTimeout is the maximum time to wait for response headers.
	// This prevents hanging on servers that accept connections but don't respond.
	ResponseHeaderTimeout = 20 * time.Second
)

// GetRequestTimeout returns the appropriate timeout for LLM requests.
// If provider-specific timeout is needed, this can be extended.
func GetRequestTimeout(provider string) time.Duration {
	switch provider {
	case "claude":
		return LLMRequestTimeout
	case "gemini":
		return LLMRequestTimeout
	case "gpt":
		return LLMRequestTimeout
	case "glm":
		return LLMRequestTimeout
	case "vllm":
		return LLMRequestTimeout
	default:
		return LLMRequestTimeout
	}
}

// GetStreamTimeout returns the timeout for streaming operations.
func GetStreamTimeout() time.Duration {
	return StreamChunkTimeout
}
