// Package vision provides configuration for vision providers.
package vision

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// VisionConfig holds configuration for vision providers.
type VisionConfig struct {
	Providers  map[string]ProviderConfig `json:"providers"`
	Active     string                    `json:"active"`
	Fallback   []string                  `json:"fallback"`
	RateLimits map[string]int            `json:"rate_limits"`
	CostLimits map[string]float64        `json:"cost_limits"`
	Budget     float64                   `json:"budget"`
	Debug      bool                      `json:"debug"`

	mu sync.RWMutex
}

// ProviderConfig holds configuration for a single provider.
type ProviderConfig struct {
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Enabled     bool    `json:"enabled"`
	Priority    int     `json:"priority"`
	CostPerCall float64 `json:"cost_per_call"`
	Timeout     int     `json:"timeout"` // in seconds
}

// DefaultVisionConfig returns default configuration optimized for cost (Gemini first).
func DefaultVisionConfig() *VisionConfig {
	return &VisionConfig{
		Providers: make(map[string]ProviderConfig),
		Fallback:  []string{"gemini", "gpt", "claude"}, // Gemini (cheapest) first
		RateLimits: map[string]int{
			"gemini": 60,  // 60 requests/minute
			"gpt":    100, // 100 requests/minute
			"claude": 50,  // 50 requests/minute
		},
		CostLimits: map[string]float64{
			"gemini": 3.0,  // $3 per 1K images (cheapest)
			"gpt":    5.0,  // $5 per 1K images
			"claude": 15.0, // $15 per 1K images
		},
		Budget: 50.0, // $50 total budget (reduced for Gemini-first strategy)
		Debug:  false,
	}
}

// LoadConfigFromEnv loads configuration from environment variables.
func LoadConfigFromEnv() (*VisionConfig, error) {
	cfg := DefaultVisionConfig()

	// Claude
	if key := os.Getenv("ARTEMIS_CLAUDE_API_KEY"); key != "" {
		cfg.Providers["claude"] = ProviderConfig{
			APIKey:      key,
			Model:       "claude-3-5-sonnet-20241022",
			MaxTokens:   8192,
			Enabled:     true,
			Priority:    1,
			CostPerCall: 15.0,
			Timeout:     30,
		}
		cfg.Active = "claude"
	}

	// GPT
	if key := os.Getenv("ARTEMIS_GPT_API_KEY"); key != "" {
		cfg.Providers["gpt"] = ProviderConfig{
			APIKey:      key,
			Model:       "gpt-4o",
			MaxTokens:   4096,
			Enabled:     true,
			Priority:    2,
			CostPerCall: 5.0,
			Timeout:     30,
		}
		if cfg.Active == "" {
			cfg.Active = "gpt"
		}
	}

	// Gemini
	if key := os.Getenv("ARTEMIS_GEMINI_API_KEY"); key != "" {
		cfg.Providers["gemini"] = ProviderConfig{
			APIKey:      key,
			Model:       "gemini-2.0-flash-exp",
			MaxTokens:   4096,
			Enabled:     true,
			Priority:    3,
			CostPerCall: 3.0,
			Timeout:     30,
		}
		if cfg.Active == "" {
			cfg.Active = "gemini"
		}
	}

	if cfg.Active == "" {
		return nil, fmt.Errorf("no vision provider API keys found in environment")
	}

	return cfg, nil
}

// LoadConfigFromFile loads configuration from a JSON file.
func LoadConfigFromFile(path string) (*VisionConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg VisionConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Initialize maps if nil
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	if cfg.RateLimits == nil {
		cfg.RateLimits = make(map[string]int)
	}
	if cfg.CostLimits == nil {
		cfg.CostLimits = make(map[string]float64)
	}
	if cfg.Fallback == nil {
		cfg.Fallback = []string{"gemini", "gpt", "claude"}
	}

	return &cfg, nil
}

// SaveConfigToFile saves configuration to a JSON file.
func (vc *VisionConfig) SaveConfigToFile(path string) error {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(vc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetProvider returns the configuration for a specific provider.
func (vc *VisionConfig) GetProvider(name string) (ProviderConfig, bool) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	provider, ok := vc.Providers[name]
	return provider, ok
}

// SetProvider sets the configuration for a specific provider.
func (vc *VisionConfig) SetProvider(name string, config ProviderConfig) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.Providers[name] = config
}

// SetActive sets the active provider.
func (vc *VisionConfig) SetActive(name string) error {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if _, ok := vc.Providers[name]; !ok {
		return fmt.Errorf("provider %s not found", name)
	}

	vc.Active = name
	return nil
}

// GetActive returns the active provider name.
func (vc *VisionConfig) GetActive() string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	return vc.Active
}

// GetFallback returns the fallback provider list.
func (vc *VisionConfig) GetFallback() []string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	fallback := make([]string, len(vc.Fallback))
	copy(fallback, vc.Fallback)
	return fallback
}

// SetFallback sets the fallback provider list.
func (vc *VisionConfig) SetFallback(fallback []string) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.Fallback = fallback
}

// SetDebug enables/disables debug mode.
func (vc *VisionConfig) SetDebug(debug bool) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.Debug = debug
}

// IsDebug returns whether debug mode is enabled.
func (vc *VisionConfig) IsDebug() bool {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	return vc.Debug
}

// GetEnabledProviders returns a list of enabled providers.
func (vc *VisionConfig) GetEnabledProviders() []string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	var enabled []string
	for name, provider := range vc.Providers {
		if provider.Enabled {
			enabled = append(enabled, name)
		}
	}

	return enabled
}

// GetProviderByPriority returns providers sorted by priority.
func (vc *VisionConfig) GetProviderByPriority() []string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	// Simple bubble sort by priority (low number = high priority)
	type providerPair struct {
		name     string
		priority int
	}

	pairs := make([]providerPair, 0, len(vc.Providers))
	for name, provider := range vc.Providers {
		if provider.Enabled {
			pairs = append(pairs, providerPair{
				name:     name,
				priority: provider.Priority,
			})
		}
	}

	// Sort by priority
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[i].priority > pairs[j].priority {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	result := make([]string, len(pairs))
	for i, pair := range pairs {
		result[i] = pair.name
	}

	return result
}

// Validate validates the configuration.
func (vc *VisionConfig) Validate() error {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	if vc.Active == "" {
		return fmt.Errorf("no active provider set")
	}

	if _, ok := vc.Providers[vc.Active]; !ok {
		return fmt.Errorf("active provider %s not found", vc.Active)
	}

	for name, provider := range vc.Providers {
		if provider.Enabled {
			if provider.APIKey == "" {
				return fmt.Errorf("provider %s is enabled but has no API key", name)
			}
			if provider.Model == "" {
				return fmt.Errorf("provider %s has no model specified", name)
			}
		}
	}

	return nil
}

// GetCostEstimate returns the estimated cost for a number of calls.
func (vc *VisionConfig) GetCostEstimate(provider string, calls int) (float64, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	p, ok := vc.Providers[provider]
	if !ok {
		return 0, fmt.Errorf("provider %s not found", provider)
	}

	return p.CostPerCall * float64(calls) / 1000.0, nil
}

// CheckBudget checks if the estimated cost is within budget.
func (vc *VisionConfig) CheckBudget(estimatedCost float64) error {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	if estimatedCost > vc.Budget {
		return fmt.Errorf("estimated cost $%.2f exceeds budget $%.2f", estimatedCost, vc.Budget)
	}

	return nil
}

// SetBudget sets the total budget.
func (vc *VisionConfig) SetBudget(budget float64) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.Budget = budget
}

// GetBudget returns the total budget.
func (vc *VisionConfig) GetBudget() float64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	return vc.Budget
}

// GetRateLimit returns the rate limit for a provider.
func (vc *VisionConfig) GetRateLimit(provider string) (int, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	limit, ok := vc.RateLimits[provider]
	if !ok {
		return 0, fmt.Errorf("no rate limit set for provider %s", provider)
	}

	return limit, nil
}

// SetRateLimit sets the rate limit for a provider.
func (vc *VisionConfig) SetRateLimit(provider string, limit int) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.RateLimits[provider] = limit
}

// String returns a string representation of the config.
func (vc *VisionConfig) String() string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	return fmt.Sprintf("VisionConfig{active=%s, providers=%d, budget=$%.2f}",
		vc.Active, len(vc.Providers), vc.Budget)
}

// ConfigManager manages vision configuration with persistence.
type ConfigManager struct {
	configPath string
	config     *VisionConfig
	mu         sync.RWMutex
}

// NewConfigManager creates a new config manager.
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// Load loads the configuration from file or environment.
func (cm *ConfigManager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Try to load from file first
	if _, err := os.Stat(cm.configPath); err == nil {
		config, err := LoadConfigFromFile(cm.configPath)
		if err == nil {
			cm.config = config
			return nil
		}
	}

	// Fall back to environment variables
	config, err := LoadConfigFromEnv()
	if err != nil {
		return err
	}

	cm.config = config
	return nil
}

// Save saves the configuration to file.
func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.config == nil {
		return fmt.Errorf("no configuration to save")
	}

	return cm.config.SaveConfigToFile(cm.configPath)
}

// GetConfig returns the current configuration.
func (cm *ConfigManager) GetConfig() *VisionConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.config
}

// SetConfig sets the configuration.
func (cm *ConfigManager) SetConfig(config *VisionConfig) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.config = config
}

// Reload reloads the configuration from file or environment.
func (cm *ConfigManager) Reload() error {
	return cm.Load()
}

// GetConfigDir returns the default configuration directory.
func GetConfigDir() (string, error) {
	// Check for ARTEMIS_CONFIG_DIR environment variable
	if configDir := os.Getenv("ARTEMIS_CONFIG_DIR"); configDir != "" {
		return configDir, nil
	}

	// Use XDG config directory or fallback
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".artemis")
	return configDir, nil
}

// GetDefaultConfigPath returns the default configuration file path.
func GetDefaultConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "vision_config.json"), nil
}

// EnsureConfigDir ensures the configuration directory exists.
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}

	return os.MkdirAll(configDir, 0755)
}
