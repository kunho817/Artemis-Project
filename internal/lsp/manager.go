package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ServerConfig defines how to start a specific LSP server.
type ServerConfig struct {
	Command string   `json:"command"`        // executable name (e.g., "gopls")
	Args    []string `json:"args,omitempty"` // command-line arguments (e.g., ["serve"])
	Enabled bool     `json:"enabled"`        // whether this server is active
}

// DefaultServerConfigs returns sensible defaults for known LSP servers.
func DefaultServerConfigs() map[string]ServerConfig {
	return map[string]ServerConfig{
		"go": {
			Command: "gopls",
			Args:    []string{"serve"},
			Enabled: true,
		},
		"python": {
			Command: "pyright-langserver",
			Args:    []string{"--stdio"},
			Enabled: false,
		},
		"typescript": {
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			Enabled: false,
		},
	}
}

// extensionToLanguage maps file extensions to language identifiers.
var extensionToLanguage = map[string]string{
	".go":    "go",
	".py":    "python",
	".pyi":   "python",
	".ts":    "typescript",
	".tsx":   "typescript",
	".js":    "javascript",
	".jsx":   "javascript",
	".rs":    "rust",
	".java":  "java",
	".kt":    "kotlin",
	".rb":    "ruby",
	".c":     "c",
	".cpp":   "cpp",
	".h":     "c",
	".hpp":   "cpp",
	".cs":    "csharp",
	".swift": "swift",
}

// languageIDForLSP maps our language identifiers to LSP languageId strings.
var languageIDForLSP = map[string]string{
	"go":         "go",
	"python":     "python",
	"typescript": "typescriptreact", // covers both .ts and .tsx
	"javascript": "javascriptreact",
	"rust":       "rust",
	"java":       "java",
	"kotlin":     "kotlin",
	"ruby":       "ruby",
	"c":          "c",
	"cpp":        "cpp",
	"csharp":     "csharp",
	"swift":      "swift",
}

// Manager is the LSP Control Plane — manages multiple LSP server clients,
// routes requests based on file type, and handles lazy initialization.
type Manager struct {
	rootDir string
	configs map[string]ServerConfig // language → server config

	mu      sync.RWMutex
	clients map[string]*Client // language → active client
	enabled bool
}

// NewManager creates a new LSP Manager.
func NewManager(rootDir string, configs map[string]ServerConfig) *Manager {
	return &Manager{
		rootDir: rootDir,
		configs: configs,
		clients: make(map[string]*Client),
		enabled: true,
	}
}

// SetEnabled enables or disables the LSP system.
func (m *Manager) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// IsEnabled returns whether the LSP system is enabled.
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// ClientForFile returns (or lazily creates) the LSP client for a given file.
// Returns nil, nil if no LSP server is configured for the file's language.
func (m *Manager) ClientForFile(ctx context.Context, file string) (*Client, error) {
	if !m.enabled {
		return nil, nil
	}

	lang := LanguageForFile(file)
	if lang == "" {
		return nil, nil
	}

	return m.ClientForLanguage(ctx, lang)
}

// ClientForLanguage returns (or lazily creates) the LSP client for a language.
func (m *Manager) ClientForLanguage(ctx context.Context, lang string) (*Client, error) {
	if !m.enabled {
		return nil, nil
	}

	// Check for existing client
	m.mu.RLock()
	client, ok := m.clients[lang]
	m.mu.RUnlock()
	if ok {
		return client, nil
	}

	// Need to create — acquire write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if client, ok := m.clients[lang]; ok {
		return client, nil
	}

	cfg, ok := m.configs[lang]
	if !ok || !cfg.Enabled {
		return nil, nil
	}

	// Start the LSP server
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client, err := NewClient(initCtx, lang, cfg.Command, cfg.Args, m.rootDir)
	if err != nil {
		return nil, fmt.Errorf("start LSP server for %s: %w", lang, err)
	}

	if err := client.Initialize(initCtx); err != nil {
		client.Shutdown(context.Background())
		return nil, fmt.Errorf("initialize LSP server for %s: %w", lang, err)
	}

	m.clients[lang] = client
	return client, nil
}

// Shutdown gracefully shuts down all active LSP servers.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for lang, client := range m.clients {
		client.Shutdown(ctx)
		delete(m.clients, lang)
	}
}

// ActiveLanguages returns the languages with currently running LSP servers.
func (m *Manager) ActiveLanguages() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var langs []string
	for lang := range m.clients {
		langs = append(langs, lang)
	}
	return langs
}

// ConfiguredLanguages returns all languages with server configurations.
func (m *Manager) ConfiguredLanguages() []string {
	var langs []string
	for lang, cfg := range m.configs {
		if cfg.Enabled {
			langs = append(langs, lang)
		}
	}
	return langs
}

// LanguageForFile determines the language from a file path.
func LanguageForFile(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	return extensionToLanguage[ext]
}

// LanguageIDForLSP returns the LSP languageId for a language.
func LanguageIDForLSP(lang string) string {
	if id, ok := languageIDForLSP[lang]; ok {
		return id
	}
	return lang
}
