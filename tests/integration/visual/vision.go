// Package visual provides Vision API integration for visual testing.
package visual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VisionProvider defines the interface for vision analysis providers.
type VisionProvider interface {
	// Name returns the provider name.
	Name() string

	// AnalyzeImage analyzes an image and returns a text description.
	AnalyzeImage(ctx context.Context, img interface{}, prompt string) (string, error)

	// EstimateCost estimates the cost of an analysis (in USD).
	EstimateCost(tokens int) float64

	// SupportsImage returns true if the provider supports the given image format.
	SupportsImage(format string) bool
}

// VisionClient manages multiple vision providers.
type VisionClient struct {
	providers map[string]VisionProvider
	router    *VisionRouter
	config    *VisionConfig
}

// NewVisionClient creates a new vision client.
func NewVisionClient(config *VisionConfig) *VisionClient {
	if config == nil {
		config = DefaultVisionConfig()
	}

	client := &VisionClient{
		providers: make(map[string]VisionProvider),
		config:    config,
	}

	// Initialize router
	client.router = NewVisionRouter(config.Strategy)

	return client
}

// RegisterProvider registers a vision provider.
func (vc *VisionClient) RegisterProvider(name string, provider VisionProvider) {
	vc.providers[name] = provider
}

// Analyze analyzes an image using the best available provider.
func (vc *VisionClient) Analyze(ctx context.Context, img interface{}, prompt string) (string, error) {
	provider := vc.router.SelectProvider(vc.providers)
	if provider == nil {
		return "", fmt.Errorf("no vision provider available")
	}

	return provider.AnalyzeImage(ctx, img, prompt)
}

// AnalyzeWithProvider analyzes an image using a specific provider.
func (vc *VisionClient) AnalyzeWithProvider(ctx context.Context, providerName string, img interface{}, prompt string) (string, error) {
	provider, ok := vc.providers[providerName]
	if !ok {
		return "", fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.AnalyzeImage(ctx, img, prompt)
}

// EstimateTotalCost estimates the total cost for all providers.
func (vc *VisionClient) EstimateTotalCost(tokens int) map[string]float64 {
	costs := make(map[string]float64)
	for name, provider := range vc.providers {
		costs[name] = provider.EstimateCost(tokens)
	}
	return costs
}

// ImageData represents image data for analysis.
type ImageData struct {
	Format    string // "png", "jpeg", etc.
	Base64    string // Base64-encoded image data
	Width     int    // Image width
	Height    int    // Image height
	SizeBytes int    // Size in bytes
}

// NewImageDataFromBytes creates ImageData from raw bytes.
func NewImageDataFromBytes(data []byte, format string) (*ImageData, error) {
	return &ImageData{
		Format:    format,
		Base64:    base64.StdEncoding.EncodeToString(data),
		SizeBytes: len(data),
	}, nil
}

// NewImageDataFromBase64 creates ImageData from a base64 string.
func NewImageDataFromBase64(base64Str, format string) (*ImageData, error) {
	return &ImageData{
		Format: format,
		Base64: base64Str,
	}, nil
}

// ToDataURL creates a data URL (for web display).
func (img *ImageData) ToDataURL() string {
	return fmt.Sprintf("data:image/%s;base64,%s", img.Format, img.Base64)
}

// VisionConfig configures vision client behavior.
type VisionConfig struct {
	// Strategy determines how to select providers.
	Strategy RouterStrategy

	// Fallback is the order of providers to try.
	Fallback []string

	// Timeout is the default timeout for analysis.
	Timeout time.Duration

	// MaxRetries is the number of retries on failure.
	MaxRetries int

	// BudgetLimit is the maximum cost per session (in USD).
	BudgetLimit float64

	// CurrentSpend tracks spending in this session.
	CurrentSpend float64
}

// DefaultVisionConfig returns default vision configuration.
func DefaultVisionConfig() *VisionConfig {
	return &VisionConfig{
		Strategy:     RouteByQuality,
		Fallback:     []string{"claude", "gpt", "gemini"},
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		BudgetLimit:  10.0, // $10 per session
		CurrentSpend: 0.0,
	}
}

// AnalysisResult represents the result of a vision analysis.
type AnalysisResult struct {
	// Provider is the name of the provider used.
	Provider string

	// Content is the analysis text.
	Content string

	// Cost is the estimated cost in USD.
	Cost float64

	// Duration is how long the analysis took.
	Duration time.Duration

	// Timestamp is when the analysis was performed.
	Timestamp time.Time

	// Success indicates if the analysis succeeded.
	Success bool

	// Error contains any error message.
	Error string
}

// AnalyzeWithContext performs analysis with full result tracking.
func (vc *VisionClient) AnalyzeWithContext(ctx context.Context, img interface{}, prompt string) *AnalysisResult {
	start := time.Now()
	result := &AnalysisResult{
		Timestamp: start,
	}

	// Check budget
	if vc.config.CurrentSpend >= vc.config.BudgetLimit {
		result.Error = fmt.Sprintf("budget limit reached: %.2f/%.2f", vc.config.CurrentSpend, vc.config.BudgetLimit)
		return result
	}

	// Select provider
	provider := vc.router.SelectProvider(vc.providers)
	if provider == nil {
		result.Error = "no vision provider available"
		return result
	}

	result.Provider = provider.Name()

	// Perform analysis with retries
	var lastErr error
	for attempt := 0; attempt <= vc.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				result.Error = ctx.Err().Error()
				return result
			}
		}

		content, err := provider.AnalyzeImage(ctx, img, prompt)
		if err == nil {
			result.Content = content
			result.Success = true
			result.Duration = time.Since(start)

			// Update cost estimate (rough estimate based on content length)
			tokens := len(content) / 4 // Rough token estimate
			result.Cost = provider.EstimateCost(tokens)
			vc.config.CurrentSpend += result.Cost

			return result
		}

		lastErr = err
	}

	result.Error = lastErr.Error()
	result.Duration = time.Since(start)
	return result
}

// BatchAnalysis performs multiple analyses in parallel.
func (vc *VisionClient) BatchAnalysis(ctx context.Context, requests []AnalysisRequest) []*AnalysisResult {
	results := make([]*AnalysisResult, len(requests))

	// Create channels
	type resultPair struct {
		index  int
		result *AnalysisResult
	}
	resultChan := make(chan resultPair, len(requests))

	// Launch analyses in parallel
	for i, req := range requests {
		go func(idx int, r AnalysisRequest) {
			result := vc.AnalyzeWithContext(ctx, r.Image, r.Prompt)
			resultChan <- resultPair{idx, result}
		}(i, req)
	}

	// Collect results
	for i := 0; i < len(requests); i++ {
		pair := <-resultChan
		results[pair.index] = pair.result
	}

	return results
}

// AnalysisRequest represents a single analysis request.
type AnalysisRequest struct {
	Image  interface{}
	Prompt string
}

// GetSpendingReport returns the current spending report.
func (vc *VisionClient) GetSpendingReport() *SpendingReport {
	providerCosts := vc.EstimateTotalCost(1000) // Per 1K tokens

	return &SpendingReport{
		CurrentSpend:  vc.config.CurrentSpend,
		BudgetLimit:   vc.config.BudgetLimit,
		Remaining:     vc.config.BudgetLimit - vc.config.CurrentSpend,
		ProviderCosts: providerCosts,
		AnalyzedCount: len(vc.providers),
	}
}

// SpendingReport reports on vision API spending.
type SpendingReport struct {
	CurrentSpend  float64
	BudgetLimit   float64
	Remaining     float64
	ProviderCosts map[string]float64
	AnalyzedCount int
}

// String returns a formatted spending report.
func (r *SpendingReport) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Vision API Spending:\n"))
	sb.WriteString(fmt.Sprintf("  Current: $%.4f / $%.2f\n", r.CurrentSpend, r.BudgetLimit))
	sb.WriteString(fmt.Sprintf("  Remaining: $%.4f\n", r.Remaining))
	sb.WriteString(fmt.Sprintf("  Providers: %d\n", r.AnalyzedCount))
	sb.WriteString("\nProvider Costs (per 1K tokens):\n")
	for provider, cost := range r.ProviderCosts {
		sb.WriteString(fmt.Sprintf("  %s: $%.2f\n", provider, cost))
	}
	return sb.String()
}

// HTTPClient is a reusable HTTP client for vision API calls.
type HTTPClient struct {
	client    *http.Client
	apiKey    string
	baseURL   string
	userAgent string
}

// NewHTTPClient creates a new HTTP client for vision APIs.
func NewHTTPClient(apiKey, baseURL string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey:    apiKey,
		baseURL:   baseURL,
		userAgent: "Artemis-VisualTesting/1.0",
	}
}

// PostJSON sends a JSON POST request.
func (h *HTTPClient) PostJSON(ctx context.Context, endpoint string, body interface{}) ([]byte, error) {
	// Marshal body to JSON
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build full URL
	fullURL := h.baseURL + endpoint

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", h.userAgent)
	if h.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", h.apiKey))
	}

	// Send request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// DownloadImage downloads an image from a URL.
func (h *HTTPClient) DownloadImage(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", h.userAgent)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// ImageDecoder handles image decoding for vision APIs.
type ImageDecoder struct {
	// MaxSize is the maximum image size in bytes.
	MaxSize int64

	// SupportedFormats lists supported image formats.
	SupportedFormats map[string]bool
}

// NewImageDecoder creates a new image decoder.
func NewImageDecoder() *ImageDecoder {
	return &ImageDecoder{
		MaxSize: 10 * 1024 * 1024, // 10MB
		SupportedFormats: map[string]bool{
			"png":  true,
			"jpeg": true,
			"jpg":  true,
			"gif":  true,
			"webp": true,
		},
	}
}

// Decode decodes image data and returns base64-encoded string.
func (d *ImageDecoder) Decode(data []byte) (string, string, error) {
	// Check size
	if int64(len(data)) > d.MaxSize {
		return "", "", fmt.Errorf("image too large: %d bytes (max %d)", len(data), d.MaxSize)
	}

	// Detect format from magic bytes
	format := d.detectFormat(data)
	if !d.SupportedFormats[format] {
		return "", "", fmt.Errorf("unsupported format: %s", format)
	}

	// Return base64-encoded data
	base64Str := base64.StdEncoding.EncodeToString(data)
	return base64Str, format, nil
}

// detectFormat detects image format from magic bytes.
func (d *ImageDecoder) detectFormat(data []byte) string {
	if len(data) < 4 {
		return "unknown"
	}

	// PNG magic bytes
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}

	// JPEG magic bytes
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}

	// GIF magic bytes
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "gif"
	}

	// WebP magic bytes
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp"
	}

	return "unknown"
}

// Validate validates image data before sending to vision API.
func (d *ImageDecoder) Validate(data []byte) error {
	if int64(len(data)) > d.MaxSize {
		return fmt.Errorf("image exceeds maximum size")
	}

	format := d.detectFormat(data)
	if !d.SupportedFormats[format] {
		return fmt.Errorf("unsupported format: %s", format)
	}

	return nil
}
