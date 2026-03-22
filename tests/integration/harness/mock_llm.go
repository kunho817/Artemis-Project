package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/artemis-project/artemis/internal/llm"
)

// MockLLM is a deterministic mock LLM provider for testing.
// It provides pre-defined responses based on input patterns.
type MockLLM struct {
	mu       sync.Mutex
	responses map[string]string // pattern → response
	errors    map[string]error   // pattern → error (for simulating failures)
	delay     time.Duration      // artificial delay for testing cancellation
	callCount int
	messages  [][]llm.Message
}

// NewMockLLM creates a new mock LLM with default responses.
func NewMockLLM() *MockLLM {
	return &MockLLM{
		responses: make(map[string]string),
		errors:    make(map[string]error),
		messages:  make([][]llm.Message, 0),
	}
}

// Name returns the mock provider name.
func (m *MockLLM) Name() string {
	return "mock"
}

// Model returns the mock model name.
func (m *MockLLM) Model() string {
	return "mock-model"
}

// SetResponse sets a custom response for a given input pattern.
// The pattern is matched using substring search in the user message.
func (m *MockLLM) SetResponse(pattern, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[pattern] = response
}

// SetDefaultResponses sets up common default responses for testing.
func (m *MockLLM) SetDefaultResponses() {
	m.SetResponse("hello", "Hello! How can I help you today?")
	m.SetResponse("code", "Here's a code example:\n```go\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```")
	m.SetResponse("error", "I see an error in the code. Let me fix it.")
	m.SetResponse("file", "I've analyzed the file. Here are my findings:")
	m.SetResponse("test", "I'll write a test for this functionality.")
	m.SetResponse("plan", "Here's my plan:\n1. Analyze the codebase\n2. Identify issues\n3. Propose solutions")
	m.SetResponse("scout", "I've explored the codebase and found the following components:")
	m.SetResponse("consultant", "Based on my analysis, I recommend refactoring the code structure.")
	m.SetResponse("critic", "The code looks good overall. Here are some minor suggestions:")
	m.SetResponse("review", "LGTM! The code is well-structured and follows best practices.")
}

// Send returns a response based on the input messages.
// It matches patterns in the user message and returns the corresponding response.
func (m *MockLLM) Send(ctx context.Context, messages []llm.Message) (string, error) {
	m.mu.Lock()

	// Check for errors first
	var userMsg string
	for _, msg := range messages {
		if msg.Role == "user" {
			userMsg = msg.Content
		}
	}

	// Check if this pattern should return an error
	for pattern, err := range m.errors {
		if strings.Contains(strings.ToLower(userMsg), strings.ToLower(pattern)) {
			m.mu.Unlock()
			return "", err
		}
	}

	m.callCount++
	m.messages = append(m.messages, messages)

	// Apply delay if set
	delay := m.delay
	m.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
			// Delay completed, continue
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Find matching response
	for pattern, response := range m.responses {
		if strings.Contains(strings.ToLower(userMsg), strings.ToLower(pattern)) {
			return response, nil
		}
	}

	// Default response
	return fmt.Sprintf("Mock response to: %s", userMsg), nil
}

// Stream returns a streaming response based on the input messages.
// It splits the response into chunks and sends them through the channel.
func (m *MockLLM) Stream(ctx context.Context, messages []llm.Message) (<-chan llm.StreamChunk, error) {
	// Don't call Send() to avoid deadlock - replicate logic here
	m.mu.Lock()
	m.callCount++
	m.messages = append(m.messages, messages)

	// Extract the last user message
	var userMsg string
	for _, msg := range messages {
		if msg.Role == "user" {
			userMsg = msg.Content
		}
	}

	// Find matching response
	var response string
	for pattern, resp := range m.responses {
		if strings.Contains(strings.ToLower(userMsg), strings.ToLower(pattern)) {
			response = resp
			break
		}
	}

	// Default response
	if response == "" {
		response = fmt.Sprintf("Mock response to: %s", userMsg)
	}
	m.mu.Unlock()

	ch := make(chan llm.StreamChunk, 32)

	// Split response into chunks for streaming
	chunks := splitIntoChunks(response, 10)

	go func() {
		defer close(ch)
		for _, chunk := range chunks {
			select {
			case <-ctx.Done():
				return
			case ch <- llm.StreamChunk{Content: chunk}:
			}
		}
		ch <- llm.StreamChunk{
			Done: true,
			Usage: &llm.TokenUsage{
				PromptTokens:     100,
				CompletionTokens: len(chunks),
				TotalTokens:      100 + len(chunks),
			},
		}
	}()

	return ch, nil
}

// GetCallCount returns the number of times the mock was called.
func (m *MockLLM) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// GetMessages returns all messages received by the mock.
func (m *MockLLM) GetMessages() [][]llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

// Reset clears the mock state.
func (m *MockLLM) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount = 0
	m.messages = make([][]llm.Message, 0)
	m.delay = 0
	m.errors = make(map[string]error)
}

// SetDelay sets an artificial delay for all requests (useful for testing cancellation).
func (m *MockLLM) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

// SetError sets an error to be returned for a specific pattern.
func (m *MockLLM) SetError(pattern string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[pattern] = err
}

// splitIntoChunks splits a string into chunks of approximately the given size.
func splitIntoChunks(s string, chunkSize int) []string {
	if len(s) <= chunkSize {
		return []string{s}
	}

	var chunks []string
	for i := 0; i < len(s); i += chunkSize {
		end := i + chunkSize
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// MockLLMBuilder creates mock LLM providers for testing.
type MockLLMBuilder struct {
	mu    sync.Mutex
	mocks map[string]*MockLLM
}

// NewMockLLMBuilder creates a new mock builder.
func NewMockLLMBuilder() *MockLLMBuilder {
	return &MockLLMBuilder{
		mocks: make(map[string]*MockLLM),
	}
}

// Build creates or returns a mock LLM for the given provider name.
func (b *MockLLMBuilder) Build(provider string) llm.Provider {
	b.mu.Lock()
	defer b.mu.Unlock()

	if mock, exists := b.mocks[provider]; exists {
		return mock
	}

	mock := NewMockLLM()
	mock.SetDefaultResponses()
	b.mocks[provider] = mock
	return mock
}

// GetMock returns the mock LLM for a given provider name.
func (b *MockLLMBuilder) GetMock(provider string) *MockLLM {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mocks[provider]
}

// ResetAll resets all mocks.
func (b *MockLLMBuilder) ResetAll() {
	for _, mock := range b.mocks {
		mock.Reset()
	}
}
