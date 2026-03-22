// Package providers provides Vision API implementations.
package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/artemis-project/artemis/tests/integration/visual"
)

// GeminiVisionProvider implements VisionProvider for Gemini Pro Vision.
type GeminiVisionProvider struct {
	apiKey string
	model  string // "gemini-2.0-flash-exp" or "gemini-pro-vision"
	client *http.Client

	// Cost tracking
	costPerImage float64 // $3 per 1K images
}

// NewGeminiVisionProvider creates a new Gemini vision provider.
func NewGeminiVisionProvider() (*GeminiVisionProvider, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	return &GeminiVisionProvider{
		apiKey:       apiKey,
		model:        "gemini-2.0-flash-exp",
		client:       &http.Client{Timeout: 30 * time.Second},
		costPerImage: 3.0, // $3 per 1K images
	}, nil
}

// Name returns the provider name.
func (gvp *GeminiVisionProvider) Name() string {
	return "gemini"
}

// AnalyzeImage analyzes an image using Gemini Pro Vision API.
func (gvp *GeminiVisionProvider) AnalyzeImage(ctx context.Context, img interface{}, prompt string) (string, error) {
	// Convert image to base64
	base64Data, format, err := gvp.encodeImage(img)
	if err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	// Build request for Gemini API
	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": prompt,
					},
					{
						"inline_data": map[string]string{
							"mime_type": fmt.Sprintf("image/%s", format),
							"data":      base64Data,
						},
					},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 4096,
		},
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", gvp.model, gvp.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send request
	start := time.Now()
	resp, err := gvp.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract content
	content, ok := gvp.extractContent(respData)
	if !ok {
		return "", fmt.Errorf("failed to extract content from response")
	}

	// Log latency
	_ = time.Since(start).Milliseconds()

	return content, nil
}

// EstimateCost estimates the cost of analysis.
func (gvp *GeminiVisionProvider) EstimateCost(tokens int) float64 {
	// Gemini: $3 per 1K images
	images := (tokens / 1000) + 1
	return gvp.costPerImage * float64(images)
}

// SupportsImage checks if the format is supported.
func (gvp *GeminiVisionProvider) SupportsImage(format string) bool {
	supported := map[string]bool{
		"png":  true,
		"jpeg": true,
		"jpg":  true,
		"gif":  true,
		"webp": true,
	}
	return supported[format]
}

// encodeImage converts image to base64.
func (gvp *GeminiVisionProvider) encodeImage(img interface{}) (string, string, error) {
	switch v := img.(type) {
	case string:
		return v, "png", nil
	case []byte:
		return base64.StdEncoding.EncodeToString(v), "png", nil
	case *visual.ImageData:
		return v.Base64, v.Format, nil
	default:
		return "", "", fmt.Errorf("unsupported image type: %T", img)
	}
}

// extractContent extracts text content from Gemini response.
func (gvp *GeminiVisionProvider) extractContent(respData map[string]interface{}) (string, bool) {
	candidates, ok := respData["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", false
	}

	firstCandidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", false
	}

	content, ok := firstCandidate["content"].(map[string]interface{})
	if !ok {
		return "", false
	}

	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", false
	}

	var result strings.Builder
	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}

		text, ok := partMap["text"].(string)
		if ok {
			result.WriteString(text)
		}
	}

	responseText := result.String()
	return responseText, responseText != ""
}

// SetModel sets the Gemini model to use.
func (gvp *GeminiVisionProvider) SetModel(model string) {
	gvp.model = model
}

// GetModel returns the current model.
func (gvp *GeminiVisionProvider) GetModel() string {
	return gvp.model
}

// GeminiVisionConfig holds configuration for Gemini Vision provider.
type GeminiVisionConfig struct {
	APIKey string
	Model  string
}

// DefaultGeminiConfig returns default configuration.
func DefaultGeminiConfig() *GeminiVisionConfig {
	return &GeminiVisionConfig{
		Model: "gemini-2.0-flash-exp",
	}
}

// LoadConfigFromEnv loads configuration from environment.
func (g *GeminiVisionConfig) LoadConfigFromEnv() error {
	g.APIKey = os.Getenv("GEMINI_API_KEY")
	if g.APIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY not set")
	}
	return nil
}
