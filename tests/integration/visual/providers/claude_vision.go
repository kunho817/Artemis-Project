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

// ClaudeVisionProvider implements VisionProvider for Claude 3.5 Sonnet.
type ClaudeVisionProvider struct {
	apiKey string
	model  string // "claude-3-5-sonnet-20241022"
	client *http.Client

	// Cost tracking
	costPerImage float64 // $15 per 1K images
}

// NewClaudeVisionProvider creates a new Claude vision provider.
func NewClaudeVisionProvider() (*ClaudeVisionProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	return &ClaudeVisionProvider{
		apiKey:       apiKey,
		model:        "claude-3-5-sonnet-20241022",
		client:       &http.Client{Timeout: 30 * time.Second},
		costPerImage: 15.0, // $15 per 1K images
	}, nil
}

// Name returns the provider name.
func (cvp *ClaudeVisionProvider) Name() string {
	return "claude"
}

// AnalyzeImage analyzes an image using Claude Vision API.
func (cvp *ClaudeVisionProvider) AnalyzeImage(ctx context.Context, img interface{}, prompt string) (string, error) {
	// Convert image to base64
	base64Data, format, err := cvp.encodeImage(img)
	if err != nil {
		return "", fmt.Errorf("failed to encode image: %w", err)
	}

	// Build request
	reqBody := map[string]interface{}{
		"model": cvp.model,
		"max_tokens": 4096,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": prompt,
					},
					{
						"type": "image",
						"source": map[string]string{
							"type":      "base64",
							"media_type": fmt.Sprintf("image/%s", format),
							"data":      base64Data,
						},
					},
				},
			},
		},
	}

	// Marshal request
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cvp.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	start := time.Now()
	resp, err := cvp.client.Do(req)
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
	content, ok := cvp.extractContent(respData)
	if !ok {
		return "", fmt.Errorf("failed to extract content from response")
	}

	// Log latency (for router performance tracking)
	_ = time.Since(start).Milliseconds()

	return content, nil
}

// EstimateCost estimates the cost of analysis.
func (cvp *ClaudeVisionProvider) EstimateCost(tokens int) float64 {
	// Claude: $15 per 1K images
	// Tokens are roughly 1/4 of characters for text
	images := (tokens / 1000) + 1
	return cvp.costPerImage * float64(images)
}

// SupportsImage checks if the format is supported.
func (cvp *ClaudeVisionProvider) SupportsImage(format string) bool {
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
func (cvp *ClaudeVisionProvider) encodeImage(img interface{}) (string, string, error) {
	// Handle different input types
	switch v := img.(type) {
	case string:
		// Assume it's already base64 encoded
		return v, "png", nil
	case []byte:
		// Raw bytes - assume PNG
		return base64.StdEncoding.EncodeToString(v), "png", nil
	case *visual.ImageData:
		// ImageData struct
		return v.Base64, v.Format, nil
	default:
		return "", "", fmt.Errorf("unsupported image type: %T", img)
	}
}

// extractContent extracts text content from Claude response.
func (cvp *ClaudeVisionProvider) extractContent(respData map[string]interface{}) (string, bool) {
	contentBlock, ok := respData["content"]
	if !ok {
		return "", false
	}

	blocks, ok := contentBlock.([]interface{})
	if !ok {
		return "", false
	}

	var result strings.Builder
	for _, block := range blocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, ok := blockMap["type"].(string)
		if !ok || blockType != "text" {
			continue
		}

		text, ok := blockMap["text"].(string)
		if ok {
			result.WriteString(text)
		}
	}

	content := result.String()
	return content, content != ""
}

// SetModel sets the Claude model to use.
func (cvp *ClaudeVisionProvider) SetModel(model string) {
	cvp.model = model
}

// GetModel returns the current model.
func (cvp *ClaudeVisionProvider) GetModel() string {
	return cvp.model
}

// ClaudeVisionConfig holds configuration for Claude Vision provider.
type ClaudeVisionConfig struct {
	APIKey string
	Model  string
}

// DefaultClaudeConfig returns default configuration.
func DefaultClaudeConfig() *ClaudeVisionConfig {
	return &ClaudeVisionConfig{
		Model: "claude-3-5-sonnet-20241022",
	}
}

// LoadConfigFromEnv loads configuration from environment.
func (c *ClaudeVisionConfig) LoadConfigFromEnv() error {
	c.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	if c.APIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	return nil
}
