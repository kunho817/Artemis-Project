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

// Gemini implements the Provider interface for Google's Gemini API.
type Gemini struct {
	cfg       config.ProviderConfig
	client    *http.Client
	lastUsage *TokenUsage
}

// NewGemini creates a new Gemini provider.
func NewGemini(cfg config.ProviderConfig) *Gemini {
	return &Gemini{
		cfg:    cfg,
		client: &http.Client{},
	}
}

func (g *Gemini) Name() string { return "gemini" }
func (g *Gemini) Model() string { return g.cfg.Model }

// Gemini API types
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (g *Gemini) Send(ctx context.Context, messages []Message) (string, error) {
	if g.cfg.APIKey == "" {
		return "", fmt.Errorf("gemini: API key not configured")
	}
	g.lastUsage = nil

	// Extract system prompt and build contents
	var systemInstruction *geminiContent
	var contents []geminiContent
	for _, m := range messages {
		if m.Role == "system" {
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	reqBody := geminiRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal error: %w", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s",
		g.cfg.Endpoint, g.cfg.Model, g.cfg.APIKey)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("gemini: request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gemini: read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result geminiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("gemini: unmarshal error: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini: API error: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}

	if result.UsageMetadata != nil {
		g.lastUsage = &TokenUsage{
			PromptTokens:     result.UsageMetadata.PromptTokenCount,
			CompletionTokens: result.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      result.UsageMetadata.TotalTokenCount,
		}
	} else {
		g.lastUsage = nil
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (g *Gemini) Stream(ctx context.Context, messages []Message) (<-chan StreamChunk, error) {
	if g.cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: API key not configured")
	}
	g.lastUsage = nil

	// Extract system prompt and build contents (same as Send)
	var systemInstruction *geminiContent
	var contents []geminiContent
	for _, m := range messages {
		if m.Role == "system" {
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	reqBody := geminiRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal error: %w", err)
	}

	// Use streamGenerateContent with alt=sse for SSE streaming
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s",
		g.cfg.Endpoint, g.cfg.Model, g.cfg.APIKey)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(errBody))
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

			var chunk geminiResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Error != nil {
				ch <- StreamChunk{Error: fmt.Errorf("gemini: %s", chunk.Error.Message), Done: true}
				return
			}

			if chunk.UsageMetadata != nil {
				usage = &TokenUsage{
					PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
					CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
				}
			}

			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				text := chunk.Candidates[0].Content.Parts[0].Text
				if text != "" {
					ch <- StreamChunk{Content: text}
				}
			}
		}
		// Connection closed — stream complete
		ch <- StreamChunk{Done: true, Usage: usage}
	}()

	return ch, nil
}
