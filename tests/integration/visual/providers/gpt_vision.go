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
	"time"

	"github.com/artemis-project/artemis/tests/integration/visual"
)

// GPTVisionProvider implements VisionProvider for GPT-4o.
type GPTVisionProvider struct {
	apiKey string
	model  string // "gpt-4o" or "gpt-4-vision-preview"
	client *http.Client

	// Cost tracking
	costPerImage float64 // $5 per 1K images
}

// NewGPTVisionProvider creates a new GPT vision provider.
func NewGPTVisionProvider() (*GPTVisionProvider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	return &GPTVisionProvider{
		apiKey:       apiKey,
		model:        "gpt-4o",
		client:       &http.Client{Timeout: 30 * time.Second},
		costPerImage: 5.0, // $5 per 1K images
	}, nil
}

// Name returns the provider name.
func (gvp *GPTVisionProvider) Name() string {
	return "gpt"
}

// AnalyzeImage analyzes an image using GPT-4o Vision API.
func (gvp *GPTVisionProvider) AnalyzeImage(ctx context.Context, img interface{}, prompt string) (string, error) {
	// Convert image to base64
	base64Data, format, err := gvp.encodeImage(img)
	if err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	// Build request
	reqBody := map[string]interface{}{
		"model": gvp.model,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": fmt.Sprintf("data:image/%s;base64,%s", format, base64Data),
						},
					},
				},
			},
		},
		"max_tokens": 4096,
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", gvp.apiKey))

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
	latency := int(time.Since(start).Milliseconds())
	_ = latency // Use to avoid unused variable warning

	return content, nil
}

// EstimateCost estimates the cost of analysis.
func (gvp *GPTVisionProvider) EstimateCost(tokens int) float64 {
	// GPT-4o: $5 per 1K images
	images := (tokens / 1000) + 1
	return gvp.costPerImage * float64(images)
}

// SupportsImage checks if the format is supported.
func (gvp *GPTVisionProvider) SupportsImage(format string) bool {
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
func (gvp *GPTVisionProvider) encodeImage(img interface{}) (string, string, error) {
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

// extractContent extracts text content from GPT response.
func (gvp *GPTVisionProvider) extractContent(respData map[string]interface{}) (string, bool) {
	choices, ok := respData["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", false
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", false
	}

	message, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return "", false
	}

	content, ok := message["content"].(string)
	if !ok {
		return "", false
	}

	return content, true
}

// SetModel sets the GPT model to use.
func (gvp *GPTVisionProvider) SetModel(model string) {
	gvp.model = model
}

// GetModel returns the current model.
func (gvp *GPTVisionProvider) GetModel() string {
	return gvp.model
}

// GPTVisionConfig holds configuration for GPT Vision provider.
type GPTVisionConfig struct {
	APIKey string
	Model  string
}

// DefaultGPTConfig returns default configuration.
func DefaultGPTConfig() *GPTVisionConfig {
	return &GPTVisionConfig{
		Model: "gpt-4o",
	}
}

// LoadConfigFromEnv loads configuration from environment.
func (g *GPTVisionConfig) LoadConfigFromEnv() error {
	g.APIKey = os.Getenv("OPENAI_API_KEY")
	if g.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY not set")
	}
	return nil
}
