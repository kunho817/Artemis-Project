package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ProviderConfig holds settings for a single LLM provider.
type ProviderConfig struct {
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Enabled  bool   `json:"enabled"`
}

// GLMConfig is Coding Plan dedicated — no standard chat endpoint.
type GLMConfig struct {
	APIKey   string `json:"api_key"`
	Endpoint string `json:"endpoint"` // Coding Plan endpoint only
	Model    string `json:"model"`
	Enabled  bool   `json:"enabled"`
}

// RoleMapping maps an agent role to a provider name.
type RoleMapping struct {
	Role     string `json:"role"`
	Provider string `json:"provider"` // "claude", "gemini", "gpt", "glm"
}

// AgentConfig holds tiered role-to-provider mappings for the multi-agent pipeline.
// Premium uses all 4 providers (Claude, GPT, Gemini, GLM).
// Budget uses only cost-effective providers (Gemini 종량제, GLM 구독제).
// When Tier="premium", failed calls fall back to the Budget mapping for that role.
type AgentConfig struct {
	Enabled        bool              `json:"enabled"`
	Tier           string            `json:"tier"`                      // "premium" or "budget"
	Premium        []RoleMapping     `json:"premium"`                   // all 4 providers
	Budget         []RoleMapping     `json:"budget"`                    // gemini + glm only
	ModelOverrides map[string]string `json:"model_overrides,omitempty"` // per-role model override (e.g., "scout": "gemini-3-flash-preview")
}

// MemoryConfig holds settings for the persistent memory system.
type MemoryConfig struct {
	Enabled           bool   `json:"enabled"`
	DBPath            string `json:"db_path"`             // SQLite database path (default: ~/.artemis/memory.db)
	ConsolidateOnExit bool   `json:"consolidate_on_exit"` // Run LLM consolidation when session ends
	MaxFactAge        int    `json:"max_fact_age_days"`   // Days before unused facts are pruned (0=never)
	MinFactUseCount   int    `json:"min_fact_use_count"`  // Minimum use_count to survive pruning
	ArchiveEnabled    bool   `json:"archive_enabled"`     // Enable COLD tier JSONL archiving (default: true)
	ArchivePath       string `json:"archive_path"`        // JSONL archive directory (empty = ~/.artemis/archive/)
}

// VectorConfig holds settings for the Phase 2 vector search system.
type VectorConfig struct {
	Enabled   bool   `json:"enabled"`
	Provider  string `json:"provider"` // embedding provider: "voyage" (default)
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`      // embedding model: "voyage-code-3" (default)
	StorePath string `json:"store_path"` // chromem-go data path (empty = ~/.artemis/vectors/)
}

// RepoMapConfig holds settings for the Phase 3 repo-map system.
type RepoMapConfig struct {
	Enabled         bool     `json:"enabled"`
	MaxTokens       int      `json:"max_tokens"`       // max tokens for prompt injection (default: 2048)
	UpdateOnWrite   bool     `json:"update_on_write"`  // auto-reindex after write_file tool
	ExcludePatterns []string `json:"exclude_patterns"` // glob patterns to skip (e.g., vendor/, node_modules/)
	CTagsPath       string   `json:"ctags_path"`       // custom ctags binary path (empty = auto-detect)
}

// LSPConfig holds settings for the Language Server Protocol integration.
type LSPConfig struct {
	Enabled    bool                       `json:"enabled"`
	AutoDetect bool                       `json:"auto_detect"` // auto-detect LSP servers in PATH
	Servers    map[string]LSPServerConfig `json:"servers"`     // language → server config
}

// LSPServerConfig defines how to start a specific LSP server.
type LSPServerConfig struct {
	Command string   `json:"command"`        // executable name (e.g., "gopls")
	Args    []string `json:"args,omitempty"` // command-line arguments (e.g., ["serve"])
	Enabled bool     `json:"enabled"`        // whether this server is active
}

// DefaultLSPConfig returns sensible defaults for the LSP system.
func DefaultLSPConfig() LSPConfig {
	return LSPConfig{
		Enabled:    true, // on by default — degrades gracefully if no LSP servers found
		AutoDetect: true,
		Servers: map[string]LSPServerConfig{
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
		},
	}
}

// SkillsConfig holds settings for the custom skill/plugin system.
type SkillsConfig struct {
	Enabled   bool   `json:"enabled"`
	GlobalDir string `json:"global_dir,omitempty"` // global skills dir (default: ~/.artemis/skills/)
	AutoLoad  bool   `json:"auto_load"`            // auto-load skills from project .artemis/skills/
}

// MCPConfig holds settings for MCP server integrations.
type MCPConfig struct {
	Enabled bool           `json:"enabled"`
	Servers []MCPServerDef `json:"servers,omitempty"`
}

// MCPServerDef defines one MCP server process configuration.
type MCPServerDef struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}

// DefaultMCPConfig returns sensible defaults for MCP integration.
func DefaultMCPConfig() MCPConfig {
	return MCPConfig{
		Enabled: false, // opt-in
	}
}

// DefaultSkillsConfig returns sensible defaults for the skills system.
func DefaultSkillsConfig() SkillsConfig {
	return SkillsConfig{
		Enabled:  true, // on by default
		AutoLoad: true, // auto-load project skills
	}
}

// GitHubConfig holds settings for the GitHub Issue tracker integration.
type GitHubConfig struct {
	Enabled      bool   `json:"enabled"`
	Token        string `json:"token"`         // Personal Access Token
	Owner        string `json:"owner"`         // Repository owner (user or org)
	Repo         string `json:"repo"`          // Repository name
	PollInterval int    `json:"poll_interval"` // Minutes between sync (0 = manual only, default: 5)
	AutoTriage   bool   `json:"auto_triage"`   // Auto-analyze new issues on sync
	AutoFix      bool   `json:"auto_fix"`      // Auto-fix issues classified as auto_fix
	BaseBranch   string `json:"base_branch"`   // Target branch for PRs (default: "main")
}

// DefaultGitHubConfig returns sensible defaults for the GitHub integration.
func DefaultGitHubConfig() GitHubConfig {
	return GitHubConfig{
		Enabled:      false, // opt-in
		PollInterval: 5,
		AutoTriage:   true,
		AutoFix:      false, // safety: manual trigger by default
		BaseBranch:   "main",
	}
}

// Config is the top-level application configuration.
type Config struct {
	ActiveProvider string         `json:"active_provider"`
	Claude         ProviderConfig `json:"claude"`
	Gemini         ProviderConfig `json:"gemini"`
	GPT            ProviderConfig `json:"gpt"`
	GLM            GLMConfig      `json:"glm"`
	VLLM           ProviderConfig `json:"vllm"`
	Agents         AgentConfig    `json:"agents"`
	Memory         MemoryConfig   `json:"memory"`
	Vector         VectorConfig   `json:"vector"`
	RepoMap        RepoMapConfig  `json:"repo_map"`
	LSP            LSPConfig      `json:"lsp"`
	Skills         SkillsConfig   `json:"skills"`
	MCP            MCPConfig      `json:"mcp"`
	GitHub         GitHubConfig   `json:"github"`
	MaxToolIter    int            `json:"max_tool_iterations"`
	Theme          string         `json:"theme"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ActiveProvider: "claude",
		Claude: ProviderConfig{
			Endpoint: "https://api.anthropic.com/v1/messages",
			Model:    "claude-sonnet-4-6",
			Enabled:  true,
		},
		Gemini: ProviderConfig{
			Endpoint: "https://generativelanguage.googleapis.com/v1beta",
			Model:    "gemini-3.1-pro-preview",
			Enabled:  false,
		},
		GPT: ProviderConfig{
			Endpoint: "https://api.openai.com/v1/chat/completions",
			Model:    "gpt-5.4",
			Enabled:  false,
		},
		GLM: GLMConfig{
			Endpoint: "https://api.z.ai/api/coding/paas/v4/chat/completions",
			Model:    "glm-5",
			Enabled:  false,
		},
		VLLM: ProviderConfig{
			Endpoint: "http://localhost:8000/v1/chat/completions",
			Model:    "qwen2.5-coder-7b",
			Enabled:  false,
		},
		Agents:  DefaultAgentConfig(),
		Memory:  DefaultMemoryConfig(),
		Vector:  DefaultVectorConfig(),
		RepoMap: DefaultRepoMapConfig(),
		LSP:     DefaultLSPConfig(),
		Skills:  DefaultSkillsConfig(),
		MCP:     DefaultMCPConfig(),
		GitHub:  DefaultGitHubConfig(),
		Theme:   "default",
	}
}

// DefaultAgentConfig returns the standard tiered role-to-provider mappings.
//
// Premium: Claude=Coder/Engineer, GPT=Orchestrator/Architect/Explorer,
//
//	Gemini=Planner/Analyzer/Designer, GLM=Searcher/QA/Tester
//
// Budget:  Gemini=사고 역할(계획/분석/설계/코딩), GLM=실행 역할(탐색/엔지니어링/QA/테스트)
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Enabled: true,
		Tier:    "premium",
		Premium: []RoleMapping{
			{Role: "orchestrator", Provider: "gpt"},
			{Role: "planner", Provider: "gemini"},
			{Role: "analyzer", Provider: "gemini"},
			{Role: "searcher", Provider: "glm"},
			{Role: "explorer", Provider: "gpt"},
			{Role: "architect", Provider: "gpt"},
			{Role: "coder", Provider: "claude"},
			{Role: "designer", Provider: "gemini"},
			{Role: "engineer", Provider: "claude"},
			{Role: "qa", Provider: "glm"},
			{Role: "tester", Provider: "glm"},
			{Role: "scout", Provider: "gemini"},
			{Role: "consultant", Provider: "gpt"},
		},
		Budget: []RoleMapping{
			{Role: "orchestrator", Provider: "gemini"},
			{Role: "planner", Provider: "gemini"},
			{Role: "analyzer", Provider: "gemini"},
			{Role: "searcher", Provider: "glm"},
			{Role: "explorer", Provider: "glm"},
			{Role: "architect", Provider: "gemini"},
			{Role: "coder", Provider: "gemini"},
			{Role: "designer", Provider: "gemini"},
			{Role: "engineer", Provider: "glm"},
			{Role: "qa", Provider: "glm"},
			{Role: "tester", Provider: "glm"},
			{Role: "scout", Provider: "gemini"},
			{Role: "consultant", Provider: "gemini"},
		},
		ModelOverrides: map[string]string{
			"scout": "gemini-3-flash-preview", // fast model for exploration
		},
	}
}

// DefaultMemoryConfig returns sensible defaults for the memory system.
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		Enabled:           true,
		DBPath:            "", // empty = use default (~/.artemis/memory.db)
		ConsolidateOnExit: true,
		MaxFactAge:        90,   // prune facts unused for 90 days
		MinFactUseCount:   2,    // facts used <2 times get pruned
		ArchiveEnabled:    true, // COLD tier on by default
		ArchivePath:       "",   // empty = use default (~/.artemis/archive/)
	}
}

// DefaultVectorConfig returns sensible defaults for the vector search system.
func DefaultVectorConfig() VectorConfig {
	return VectorConfig{
		Enabled:  false, // opt-in — requires Voyage API key
		Provider: "voyage",
		Model:    "voyage-code-3",
	}
}

// DefaultRepoMapConfig returns sensible defaults for the repo-map system.
func DefaultRepoMapConfig() RepoMapConfig {
	return RepoMapConfig{
		Enabled:       false, // opt-in
		MaxTokens:     2048,
		UpdateOnWrite: true,
		ExcludePatterns: []string{
			"vendor/", "node_modules/", ".git/", "__pycache__/",
			"dist/", "build/", ".next/", "target/",
		},
	}
}

// CTagsCachePath returns the directory where auto-downloaded ctags is stored.
func (c *Config) CTagsCachePath() string {
	dir, err := configDir()
	if err != nil {
		return "bin"
	}
	return filepath.Join(dir, "bin")
}

// VectorStorePath returns the resolved vector data directory.
// If not set in config, uses the default location in the Artemis config directory.
func (c *Config) VectorStorePath() string {
	if c.Vector.StorePath != "" {
		return c.Vector.StorePath
	}
	dir, err := configDir()
	if err != nil {
		return "vectors" // fallback to current directory
	}
	return filepath.Join(dir, "vectors")
}

// MemoryDBPath returns the resolved database path.
// If not set in config, uses the default location in the Artemis config directory.
func (c *Config) MemoryDBPath() string {
	if c.Memory.DBPath != "" {
		return c.Memory.DBPath
	}
	dir, err := configDir()
	if err != nil {
		return "memory.db" // fallback to current directory
	}
	return filepath.Join(dir, "memory.db")
}

// ArchivePath returns the resolved JSONL archive directory.
// If not set in config, uses the default location in the Artemis config directory.
func (c *Config) ArchivePath() string {
	if c.Memory.ArchivePath != "" {
		return c.Memory.ArchivePath
	}
	dir, err := configDir()
	if err != nil {
		return "archive" // fallback to current directory
	}
	return filepath.Join(dir, "archive")
}

// GlobalSkillsDir returns the resolved global skills directory.
func (c *Config) GlobalSkillsDir() string {
	if c.Skills.GlobalDir != "" {
		return c.Skills.GlobalDir
	}
	dir, err := configDir()
	if err != nil {
		return "skills"
	}
	return filepath.Join(dir, "skills")
}

// configDir returns the configuration directory path.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".artemis"), nil
}

// configPath returns the full config file path.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk, or returns defaults if not found.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return DefaultConfig(), err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return DefaultConfig(), err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}

	return cfg, nil
}

// Save writes the config to disk.
func Save(cfg Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// ProviderNames returns the list of all provider names.
func ProviderNames() []string {
	return []string{"claude", "gemini", "gpt", "glm", "vllm"}
}

// GetProvider returns the ProviderConfig for a given name (Claude, Gemini, GPT, VLLM).
func (c *Config) GetProvider(name string) *ProviderConfig {
	switch name {
	case "claude":
		return &c.Claude
	case "gemini":
		return &c.Gemini
	case "gpt":
		return &c.GPT
	case "vllm":
		return &c.VLLM
	default:
		return nil
	}
}

// GetGLM returns the GLM Coding Plan config.
func (c *Config) GetGLM() *GLMConfig {
	return &c.GLM
}

// GetActiveModel returns model name for currently active single-provider mode.
func (c *Config) GetActiveModel() string {
	if c.ActiveProvider == "glm" {
		return c.GLM.Model
	}
	if p := c.GetProvider(c.ActiveProvider); p != nil {
		return p.Model
	}
	return "unknown"
}

// ProviderForRole returns the provider name for a role based on the active tier.
// Falls back to ActiveProvider if no mapping exists.
func (c *Config) ProviderForRole(role string) string {
	mappings := c.Agents.activeMappings()
	for _, m := range mappings {
		if m.Role == role {
			return m.Provider
		}
	}
	return c.ActiveProvider
}

// FallbackProviderForRole returns the budget-tier provider for a role.
// Used when Tier="premium" and the primary provider fails.
// Returns empty string if no budget mapping exists.
func (c *Config) FallbackProviderForRole(role string) string {
	if c.Agents.Tier != "premium" {
		return "" // budget tier has no further fallback
	}
	for _, m := range c.Agents.Budget {
		if m.Role == role {
			return m.Provider
		}
	}
	return ""
}

// ModelForRole returns the model override for a role, if any.
// Returns empty string if no override exists (use provider default).
func (c *Config) ModelForRole(role string) string {
	if c.Agents.ModelOverrides != nil {
		return c.Agents.ModelOverrides[role]
	}
	return ""
}

// activeMappings returns the role mappings for the current tier.
func (ac *AgentConfig) activeMappings() []RoleMapping {
	switch ac.Tier {
	case "budget":
		return ac.Budget
	default:
		return ac.Premium
	}
}
