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

// VLLM implements the Provider interface for a local vLLM server (OpenAI-compatible API).
type VLLM struct {
	cfg       config.ProviderConfig
	client    *http.Client
	lastUsage *TokenUsage
}

// NewVLLM creates a new vLLM provider.
func NewVLLM(cfg config.ProviderConfig) *VLLM {
	return &VLLM{
		cfg:    cfg,
		client: newHTTPClient(),
	}
}

func (v *VLLM) Name() string  { return "vllm" }
func (v *VLLM) Model() string { return v.cfg.Model }

// vLLM API types (OpenAI-compatible)
type vllmRequest struct {
	Model         string        `json:"model"`
	Messages      []vllmMessage `json:"messages"`
	Stream        bool          `json:"stream,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type vllmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type vllmResponse struct {
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

func (v *VLLM) Send(ctx context.Context, messages []Message) (string, error) {
	if v.cfg.Endpoint == "" {
		return "", fmt.Errorf("vllm: endpoint not configured")
	}
	v.lastUsage = nil

	var msgs []vllmMessage
	for _, m := range messages {
		msgs = append(msgs, vllmMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqBody := vllmRequest{
		Model:    v.cfg.Model,
		Messages: msgs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("vllm: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("vllm: request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if v.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.cfg.APIKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vllm: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vllm: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vllm: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result vllmResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("vllm: unmarshal error: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("vllm: API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("vllm: empty response")
	}

	if result.Usage != nil {
		v.lastUsage = &TokenUsage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	} else {
		v.lastUsage = nil
	}

	return result.Choices[0].Message.Content, nil
}

func (v *VLLM) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	if v.cfg.Endpoint == "" {
		return nil, fmt.Errorf("vllm: endpoint not configured")
	}
	v.lastUsage = nil

	var msgs []vllmMessage
	for _, m := range messages {
		msgs = append(msgs, vllmMessage{Role: m.Role, Content: m.Content})
	}

	reqBody := vllmRequest{
		Model:    v.cfg.Model,
		Messages: msgs,
		Stream:   true,
		StreamOptions: &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vllm: marshal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vllm: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if v.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.cfg.APIKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vllm: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("vllm: API error (status %d): %s", resp.StatusCode, string(errBody))
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
			var event vllmResponse
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if event.Usage != nil {
				usage = &TokenUsage{
					PromptTokens:     event.Usage.PromptTokens,
					CompletionTokens: event.Usage.CompletionTokens,
					TotalTokens:      event.Usage.TotalTokens,
				}
				v.lastUsage = usage
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
