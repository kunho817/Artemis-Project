package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/artemis-project/artemis/internal/config"
)

// Claude implements the Provider interface for Anthropic's Claude API.
type Claude struct {
	cfg    config.ProviderConfig
	client *http.Client
}

// NewClaude creates a new Claude provider.
func NewClaude(cfg config.ProviderConfig) *Claude {
	return &Claude{
		cfg:    cfg,
		client: &http.Client{},
	}
}

func (c *Claude) Name() string { return "claude" }

// claudeRequest is the Anthropic API request format.
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []claudeMessage `json:"messages"`
	Stream    bool            `json:"stream,omitempty"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse is the Anthropic API response format.
type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Claude) Send(ctx context.Context, messages []Message) (string, error) {
	if c.cfg.APIKey == "" {
		return "", fmt.Errorf("claude: API key not configured")
	}

	// Extract system prompt and convert messages
	var systemPrompt string
	var msgs []claudeMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		msgs = append(msgs, claudeMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := claudeRequest{
		Model:     c.cfg.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  msgs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("claude: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("claude: request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("claude: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result claudeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("claude: unmarshal error: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("claude: API error: %s", result.Error.Message)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("claude: empty response")
	}

	return result.Content[0].Text, nil
}

func (c *Claude) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("claude: API key not configured")
	}

	// Extract system prompt and convert messages
	var systemPrompt string
	var msgs []claudeMessage
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		msgs = append(msgs, claudeMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := claudeRequest{
		Model:     c.cfg.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  msgs,
		Stream:    true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("claude: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("claude: API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamChunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if event.Type == "content_block_delta" && event.Delta.Text != "" {
				ch <- StreamChunk{Content: event.Delta.Text}
			} else if event.Type == "message_stop" {
				ch <- StreamChunk{Done: true}
				return
			}
		}
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}
