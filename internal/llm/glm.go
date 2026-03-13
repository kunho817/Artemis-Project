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

// GLM implements the Provider interface for ZhipuAI's GLM Coding Plan API.
// This provider uses the Coding Plan endpoint exclusively.
type GLM struct {
	cfg       config.GLMConfig
	client    *http.Client
	lastUsage *TokenUsage
}

// NewGLM creates a new GLM Coding Plan provider.
func NewGLM(cfg config.GLMConfig) *GLM {
	return &GLM{
		cfg:    cfg,
		client: &http.Client{},
	}
}

func (g *GLM) Name() string { return "glm" }
func (g *GLM) Model() string { return g.cfg.Model }

// GLM API types (OpenAI-compatible format)
type glmRequest struct {
	Model    string       `json:"model"`
	Messages []glmMessage `json:"messages"`
	Stream   bool         `json:"stream,omitempty"`
}

type glmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type glmResponse struct {
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Send sends a message via the Coding Plan endpoint.
func (g *GLM) Send(ctx context.Context, messages []Message) (string, error) {
	if g.cfg.APIKey == "" {
		return "", fmt.Errorf("glm: API key not configured")
	}
	g.lastUsage = nil

	var msgs []glmMessage
	for _, m := range messages {
		msgs = append(msgs, glmMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := glmRequest{
		Model:    g.cfg.Model,
		Messages: msgs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("glm: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("glm: request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("glm: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("glm: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("glm: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result glmResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("glm: unmarshal error: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("glm: API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("glm: empty response")
	}

	if result.Usage != nil {
		g.lastUsage = &TokenUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	} else {
		g.lastUsage = nil
	}

	choice := result.Choices[0].Message
	if choice.ReasoningContent != "" {
		return choice.ReasoningContent + "\n\n" + choice.Content, nil
	}
	return choice.Content, nil
}

func (g *GLM) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	if g.cfg.APIKey == "" {
		return nil, fmt.Errorf("glm: API key not configured")
	}
	g.lastUsage = nil

	var msgs []glmMessage
	for _, m := range messages {
		msgs = append(msgs, glmMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := glmRequest{
		Model:    g.cfg.Model,
		Messages: msgs,
		Stream:   true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("glm: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("glm: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("glm: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("glm: API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan StreamChunk, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		var usage *TokenUsage
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true, Usage: usage}
				return
			}
			var event glmResponse
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if event.Usage != nil {
				usage = &TokenUsage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
				g.lastUsage = usage
			}

			var deltaEvent struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &deltaEvent); err != nil {
				continue
			}
			if len(deltaEvent.Choices) > 0 {
				delta := deltaEvent.Choices[0].Delta
				if delta.Content != "" || delta.ReasoningContent != "" {
					ch <- StreamChunk{
						Content:   delta.Content,
						Reasoning: delta.ReasoningContent,
					}
				}
			}
		}
		ch <- StreamChunk{Done: true, Usage: usage}
	}()

	return ch, nil
}
