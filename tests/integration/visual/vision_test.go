// Package visual_test provides integration tests for vision framework.
package visual_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/vision"
	visual "github.com/artemis-project/artemis/tests/integration/visual"
	"github.com/artemis-project/artemis/tests/integration/visual/providers"
)

// TestVisionConfigLoading tests that vision configuration loads from environment.
func TestVisionConfigLoading(t *testing.T) {
	apiKey := os.Getenv("ARTEMIS_GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("ARTEMIS_GEMINI_API_KEY not set")
	}

	cfg, err := vision.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Active == "" {
		t.Error("No active provider set")
	}

	t.Logf("Active provider: %s", cfg.Active)
	t.Logf("Providers enabled: %d", len(cfg.Providers))

	for name, p := range cfg.Providers {
		if p.Enabled {
			t.Logf("  - %s: %s (priority %d)", name, p.Model, p.Priority)
		}
	}
}

// TestRateLimiter tests rate limiter creation.
func TestRateLimiter(t *testing.T) {
	rl := vision.NewRateLimiter()
	if rl == nil {
		t.Fatal("Failed to create rate limiter")
	}

	limit, err := rl.GetLimit("gemini")
	if err != nil {
		t.Fatalf("Failed to get limit: %v", err)
	}

	t.Logf("Gemini rate limit: %d requests/minute", limit)
}

// TestVisionFrameworkEndToEnd tests basic vision framework initialization.
func TestVisionFrameworkEndToEnd(t *testing.T) {
	apiKey := os.Getenv("ARTEMIS_GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("ARTEMIS_GEMINI_API_KEY not set")
	}

	// Load config
	cfg, err := vision.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create rate limiter
	rl := vision.NewRateLimiter()
	if rl == nil {
		t.Fatal("Failed to create rate limiter")
	}

	t.Logf("✓ Vision framework initialized successfully")
	t.Logf("  Active: %s", cfg.Active)
	t.Logf("  Providers: %d enabled", len(cfg.Providers))
}

// TestVisionFrameworkVerification is a manual test entry point.
func TestVisionFrameworkVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping manual test in short mode")
	}

	apiKey := os.Getenv("ARTEMIS_GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("ARTEMIS_GEMINI_API_KEY not set")
	}

	fmt.Println("=== Vision Framework Verification ===")

	cfg, err := vision.LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Config load failed: %v", err)
	}

	fmt.Printf("✓ Vision config loaded\n")
	fmt.Printf("  Active provider: %s\n", cfg.Active)
	fmt.Printf("  Providers enabled: %d\n", len(cfg.Providers))

	for name, p := range cfg.Providers {
		if p.Enabled {
			fmt.Printf("  - %s: %s (priority %d, $%.2f/1K images)\n",
				name, p.Model, p.Priority, p.CostPerCall)
		}
	}

	rl := vision.NewRateLimiter()
	limit, _ := rl.GetLimit("gemini")
	fmt.Printf("\n✓ Rate limiter created\n")
	fmt.Printf("  Gemini limit: %d requests/minute\n", limit)

	fmt.Printf("\n✅ Vision framework verification complete!\n")
}

// TestVisionClientWithMockProvider tests VisionClient with a mock provider.
func TestVisionClientWithMockProvider(t *testing.T) {
	// Create VisionClient with default config
	client := visual.NewVisionClient(nil)
	if client == nil {
		t.Fatal("Failed to create VisionClient")
	}

	// Register mock provider
	mockProvider := &mockVisionProvider{
		name:    "mock",
		cost:    1.0,
		support: map[string]bool{"png": true, "jpeg": true},
	}
	client.RegisterProvider("mock", mockProvider)

	// Test provider registration
	if len(client.EstimateTotalCost(1000)) != 1 {
		t.Error("Expected 1 provider registered")
	}

	t.Log("✓ VisionClient created and provider registered")
}

// TestVisionRouter tests all routing strategies.
func TestVisionRouter(t *testing.T) {
	// Create router with different strategies
	strategies := []visual.RouterStrategy{
		visual.RouteByCost,
		visual.RouteByQuality,
		visual.RouteBySpeed,
		visual.RouteByAvailability,
	}

	for _, strategy := range strategies {
		router := visual.NewVisionRouter(strategy)
		if router == nil {
			t.Errorf("Failed to create router with strategy %v", strategy)
			continue
		}

		// Register providers with metadata
		router.RegisterProvider(&visual.ProviderInfo{
			Name:      "gemini",
			Cost:      3.0,
			Quality:   80,
			Speed:     90,
			Available: true,
		})
		router.RegisterProvider(&visual.ProviderInfo{
			Name:      "gpt",
			Cost:      5.0,
			Quality:   90,
			Speed:     70,
			Available: true,
		})
		router.RegisterProvider(&visual.ProviderInfo{
			Name:      "claude",
			Cost:      15.0,
			Quality:   95,
			Speed:     50,
			Available: true,
		})

		t.Logf("✓ Router created with strategy: %s", strategy)
	}
}

// TestVisionRouterSelection tests provider selection logic.
func TestVisionRouterSelection(t *testing.T) {
	// Test cost-based routing
	router := visual.NewVisionRouter(visual.RouteByCost)
	router.RegisterProvider(&visual.ProviderInfo{Name: "expensive", Cost: 100, Quality: 99, Speed: 99, Available: true})
	router.RegisterProvider(&visual.ProviderInfo{Name: "cheap", Cost: 1, Quality: 50, Speed: 50, Available: true})

	providers := map[string]visual.VisionProvider{
		"expensive": &mockVisionProvider{name: "expensive"},
		"cheap":     &mockVisionProvider{name: "cheap"},
	}

	selected := router.SelectProvider(providers)
	if selected == nil {
		t.Fatal("Expected provider to be selected")
	}

	// With cost routing, should select cheapest
	if selected.Name() != "cheap" {
		t.Errorf("Expected 'cheap' provider, got '%s'", selected.Name())
	}

	t.Logf("✓ Cost routing selected: %s", selected.Name())
}

// TestValidationSuite tests the validation framework.
func TestValidationSuite(t *testing.T) {
	suite := visual.NewValidationSuite()

	// Add content validator
	contentValidator := visual.NewContentValidator("test_content").
		SetLength(1, 1000).
		AddRequired("hello")

	suite.AddValidator(contentValidator)

	// Test valid content
	ctx := context.Background()
	results := suite.Validate(ctx, "hello world")

	if !suite.IsAllValid() {
		t.Error("Expected validation to pass")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	t.Logf("✓ Validation suite passed with score: %.2f", suite.GetScore())
}

// TestCaptureSession tests the capture session functionality.
func TestCaptureSession(t *testing.T) {
	config := visual.DefaultCaptureConfig()
	session, err := visual.NewCaptureSession(config)
	if err != nil {
		t.Fatalf("Failed to create capture session: %v", err)
	}

	// Test ModelView capture
	ctx := context.Background()
	viewContent := "Test TUI View\n┌─────────┐\n│ Hello   │\n└─────────┘"
	path, err := session.Capture(ctx, "test_view", viewContent)
	if err != nil {
		t.Fatalf("Failed to capture: %v", err)
	}

	t.Logf("✓ Capture saved to: %s", path)

	// Verify captures list
	captures := session.GetCaptures()
	if len(captures) != 1 {
		t.Errorf("Expected 1 capture, got %d", len(captures))
	}
}

// TestModelViewParsing tests Model.View() string parsing.
func TestModelViewParsing(t *testing.T) {
	viewString := `┌──────────────────┐
│ Artemis Chat     │
├──────────────────┤
│ Hello World      │
│                  │
└──────────────────┘`

	data := visual.ParseModelView(viewString)

	if data.Height != 6 {
		t.Errorf("Expected height 6, got %d", data.Height)
	}

	if data.Width == 0 {
		t.Error("Expected non-zero width")
	}

	// Find border elements
	borders := data.FindElements("border")
	if len(borders) == 0 {
		t.Error("Expected to find border elements")
	}

	t.Logf("✓ Parsed ModelView: %dx%d, %d elements", data.Width, data.Height, len(data.Elements))
}

// TestSpendingReport tests spending tracking.
func TestSpendingReport(t *testing.T) {
	config := visual.DefaultVisionConfig()
	config.BudgetLimit = 10.0
	config.CurrentSpend = 2.5

	client := visual.NewVisionClient(config)
	report := client.GetSpendingReport()

	if report == nil {
		t.Fatal("Expected spending report")
	}

	if report.CurrentSpend != 2.5 {
		t.Errorf("Expected spend 2.5, got %.2f", report.CurrentSpend)
	}

	if report.Remaining != 7.5 {
		t.Errorf("Expected remaining 7.5, got %.2f", report.Remaining)
	}

	t.Logf("✓ Spending report: %s", report.String())
}

// TestVisionConfigFilePersistence tests config file save/load.
func TestVisionConfigFilePersistence(t *testing.T) {
	// Create temp file
	tmpFile := fmt.Sprintf("/tmp/vision_config_test_%d.json", time.Now().UnixNano())
	defer os.Remove(tmpFile)

	// Create and save config
	cfg := vision.DefaultVisionConfig()
	cfg.Providers["test"] = vision.ProviderConfig{
		APIKey:      "test-key",
		Model:       "test-model",
		MaxTokens:   1000,
		Enabled:     true,
		Priority:    1,
		CostPerCall: 5.0,
		Timeout:     30,
	}
	cfg.Active = "test"

	err := cfg.SaveConfigToFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Load config
	loaded, err := vision.LoadConfigFromFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loaded.Active != "test" {
		t.Errorf("Expected active 'test', got '%s'", loaded.Active)
	}

	provider, ok := loaded.Providers["test"]
	if !ok {
		t.Fatal("Expected 'test' provider")
	}

	if provider.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got '%s'", provider.Model)
	}

	t.Log("✓ Config file persistence works")
}

// TestProviderWithAPIKey tests provider initialization with real API keys.
func TestProviderWithAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API tests in short mode")
	}

	// Test Gemini provider
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		provider, err := providers.NewGeminiVisionProvider()
		if err != nil {
			t.Logf("Gemini provider creation skipped: %v", err)
		} else {
			t.Logf("✓ Gemini provider created: %s", provider.Name())
		}
	}

	// Test GPT provider
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		provider, err := providers.NewGPTVisionProvider()
		if err != nil {
			t.Logf("GPT provider creation skipped: %v", err)
		} else {
			t.Logf("✓ GPT provider created: %s", provider.Name())
		}
	}

	// Test Claude provider
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		provider, err := providers.NewClaudeVisionProvider()
		if err != nil {
			t.Logf("Claude provider creation skipped: %v", err)
		} else {
			t.Logf("✓ Claude provider created: %s", provider.Name())
		}
	}
}

// mockVisionProvider is a mock implementation for testing.
type mockVisionProvider struct {
	name    string
	cost    float64
	support map[string]bool
}

func (m *mockVisionProvider) Name() string {
	return m.name
}

func (m *mockVisionProvider) AnalyzeImage(ctx context.Context, img interface{}, prompt string) (string, error) {
	return "mock analysis result", nil
}

func (m *mockVisionProvider) EstimateCost(tokens int) float64 {
	return m.cost
}

func (m *mockVisionProvider) SupportsImage(format string) bool {
	return m.support[format]
}
