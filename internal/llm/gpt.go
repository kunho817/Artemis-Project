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

// GPT implements the Provider interface for OpenAI's GPT API.
type GPT struct {
	cfg       config.ProviderConfig
	client    *http.Client
	lastUsage *TokenUsage
}

// NewGPT creates a new GPT provider.
func NewGPT(cfg config.ProviderConfig) *GPT {
	return &GPT{
		cfg:    cfg,
		client: newHTTPClient(),
	}
}

func (g *GPT) Name() string  { return "gpt" }
func (g *GPT) Model() string { return g.cfg.Model }

// OpenAI API types
type gptRequest struct {
	Model         string       `json:"model"`
	Messages      []gptMessage `json:"messages"`
	Stream        bool         `json:"stream,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type gptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type gptResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
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

func (g *GPT) Send(ctx context.Context, messages []Message) (string, error) {
	if g.cfg.APIKey == "" {
		return "", fmt.Errorf("gpt: API key not configured")
	}
	g.lastUsage = nil

	var msgs []gptMessage
	for _, m := range messages {
		msgs = append(msgs, gptMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := gptRequest{
		Model:    g.cfg.Model,
		Messages: msgs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gpt: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gpt: request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gpt: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gpt: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gpt: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result gptResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("gpt: unmarshal error: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("gpt: API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("gpt: empty response")
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

	return result.Choices[0].Message.Content, nil
}

func (g *GPT) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	if g.cfg.APIKey == "" {
		return nil, fmt.Errorf("gpt: API key not configured")
	}
	g.lastUsage = nil

	var msgs []gptMessage
	for _, m := range messages {
		msgs = append(msgs, gptMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := gptRequest{
		Model:    g.cfg.Model,
		Messages: msgs,
		Stream:   true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gpt: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", g.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gpt: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gpt: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gpt: API error (status %d): %s", resp.StatusCode, string(errBody))
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
			var event gptResponse
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
			var delta struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &delta); err != nil {
				continue
			}
			if len(delta.Choices) > 0 && delta.Choices[0].Delta.Content != "" {
				ch <- StreamChunk{Content: delta.Choices[0].Delta.Content}
			}
		}
		ch <- StreamChunk{Done: true, Usage: usage}
	}()

	return ch, nil
}
