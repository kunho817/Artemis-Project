package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

const (
	defaultVoyageEndpoint = "https://api.voyageai.com/v1/embeddings"
	defaultVoyageModel    = "voyage-code-3"
)

// voyageRequest is the Voyage AI embedding API request format.
type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"` // "document" or "query"
}

// voyageResponse is the Voyage AI embedding API response format.
type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// NewVoyageEmbeddingFunc creates a chromem-go compatible EmbeddingFunc
// using the Voyage AI embedding API.
// inputType should be "document" for indexing or "query" for searching.
func NewVoyageEmbeddingFunc(apiKey, model, inputType string) chromem.EmbeddingFunc {
	if model == "" {
		model = defaultVoyageModel
	}
	if inputType == "" {
		inputType = "document"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	return func(ctx context.Context, text string) ([]float32, error) {
		reqBody := voyageRequest{
			Input:     []string{text},
			Model:     model,
			InputType: inputType,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("voyage: marshal request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", defaultVoyageEndpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("voyage: create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("voyage: send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("voyage: API error %d: %s", resp.StatusCode, string(body))
		}

		var voyageResp voyageResponse
		if err := json.NewDecoder(resp.Body).Decode(&voyageResp); err != nil {
			return nil, fmt.Errorf("voyage: decode response: %w", err)
		}

		if len(voyageResp.Data) == 0 {
			return nil, fmt.Errorf("voyage: empty embedding response")
		}

		return voyageResp.Data[0].Embedding, nil
	}
}
