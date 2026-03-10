package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/llm"
)

// Consolidator handles end-of-session memory consolidation.
// It uses an LLM to summarize conversations and extract reusable facts.
type Consolidator struct {
	store       MemoryStore
	provider    llm.Provider
	vectorStore VectorSearcher // Phase 2: optional, for semantic dedup
}

// NewConsolidator creates a consolidator with the given memory store and LLM provider.
func NewConsolidator(store MemoryStore, provider llm.Provider) *Consolidator {
	return &Consolidator{store: store, provider: provider}
}

// SetVectorStore attaches a VectorStore for semantic deduplication.
// When set, isSimilarFact uses cosine similarity instead of word overlap.
func (c *Consolidator) SetVectorStore(vs VectorSearcher) {
	c.vectorStore = vs
}

// ConsolidateResult contains the output of a consolidation run.
type ConsolidateResult struct {
	SessionID    string
	Summary      string
	Facts        []ExtractedFact
	Decisions    []ExtractedDecision
	FilesTouched []string
}

// ExtractedFact is a fact extracted by the LLM during consolidation.
type ExtractedFact struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// ExtractedDecision is a decision extracted by the LLM during consolidation.
type ExtractedDecision struct {
	Decision  string   `json:"decision"`
	Rationale string   `json:"rationale"`
	Tags      []string `json:"tags"`
}

// consolidationResponse is the expected JSON structure from the LLM.
type consolidationResponse struct {
	Summary   string              `json:"summary"`
	Facts     []ExtractedFact     `json:"facts"`
	Decisions []ExtractedDecision `json:"decisions"`
}

const consolidationPrompt = `You are a memory consolidation system for an AI coding agent called Artemis.

Your job: analyze the conversation below and extract two things:
1. A concise session summary (2-4 sentences)
2. Reusable project facts that would help future sessions
3. Any architectural or design decisions that were made

RULES:
- Only extract facts that are ACTUALLY stated or demonstrated in the conversation
- Do NOT invent, assume, or hallucinate facts
- Facts should be atomic (one fact = one piece of knowledge)
- Tag each fact with relevant categories from: lang, build, test, arch, design, pattern, code, impl, api, bug, ui, ux, style, plan, requirement, scope, infra, quality, search, verify
- Decisions must have clear rationale from the conversation

Respond in this exact JSON format (no markdown, no wrapping):
{
  "summary": "Brief description of what happened in this session",
  "facts": [
    {"content": "The fact text", "tags": ["tag1", "tag2"]}
  ],
  "decisions": [
    {"decision": "What was decided", "rationale": "Why it was decided", "tags": ["tag1"]}
  ]
}

If there are no meaningful facts or decisions, return empty arrays.
Do NOT include trivial facts like greetings or generic knowledge.`

// Consolidate processes a completed session's messages and persists the results.
// It extracts facts, decisions, and a summary via LLM, then stores them.
func (c *Consolidator) Consolidate(ctx context.Context, sessionID string, messages []llm.Message, filesTouched []string) (*ConsolidateResult, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("consolidate: no messages to process")
	}

	// Build conversation text for the LLM
	var convBuilder strings.Builder
	for _, m := range messages {
		convBuilder.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	// Query existing facts to help the LLM avoid duplicates
	existingFacts, _ := c.store.QueryFacts(ctx, QueryOpts{Limit: 50})
	var existingContext string
	if len(existingFacts) > 0 {
		var parts []string
		for _, f := range existingFacts {
			parts = append(parts, fmt.Sprintf("- %s", f.Content))
		}
		existingContext = "\n\nEXISTING FACTS (do NOT duplicate these, but you may suggest updates):\n" + strings.Join(parts, "\n")
	}

	// Call LLM for consolidation
	llmMessages := []llm.Message{
		{Role: "system", Content: consolidationPrompt},
		{Role: "user", Content: fmt.Sprintf("## Session Conversation\n\n%s%s", convBuilder.String(), existingContext)},
	}

	// Use a timeout for consolidation — it's a background task
	consolidateCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	response, err := c.provider.Send(consolidateCtx, llmMessages)
	if err != nil {
		return nil, fmt.Errorf("consolidate: LLM call failed: %w", err)
	}

	// Parse the LLM response
	parsed, err := parseConsolidationResponse(response)
	if err != nil {
		// If parsing fails, save a basic summary
		parsed = &consolidationResponse{
			Summary: truncate(response, 500),
		}
	}

	result := &ConsolidateResult{
		SessionID:    sessionID,
		Summary:      parsed.Summary,
		FilesTouched: filesTouched,
	}

	// Save session summary
	sessionSummary := &SessionSummary{
		SessionID:    sessionID,
		Summary:      parsed.Summary,
		FilesTouched: filesTouched,
		FactsLearned: len(parsed.Facts),
		Outcome:      "success",
		MessageCount: len(messages),
	}
	if err := c.store.SaveSession(ctx, sessionSummary); err != nil {
		return nil, fmt.Errorf("consolidate: save session: %w", err)
	}

	// Save extracted facts (with deduplication check)
	for _, ef := range parsed.Facts {
		if ef.Content == "" {
			continue
		}

		// Check for similar existing facts
		existing, _ := c.store.QueryFacts(ctx, QueryOpts{
			Query: ef.Content,
			Limit: 3,
		})

		isDuplicate := false
		for _, ex := range existing {
			if c.checkSimilarity(ctx, ex.Content, ef.Content) {
				// Update existing fact instead of creating duplicate
				c.store.IncrementFactUsage(ctx, ex.ID)
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			fact := &Fact{
				Content: ef.Content,
				Tags:    ef.Tags,
				Source:  "session:" + sessionID,
			}
			c.store.SaveFact(ctx, fact)
			result.Facts = append(result.Facts, ef)
		}
	}

	// Save extracted decisions
	for _, ed := range parsed.Decisions {
		if ed.Decision == "" {
			continue
		}
		decision := &Decision{
			Decision:  ed.Decision,
			Rationale: ed.Rationale,
			Tags:      ed.Tags,
		}
		c.store.SaveDecision(ctx, decision)
		result.Decisions = append(result.Decisions, ed)
	}

	return result, nil
}

// parseConsolidationResponse extracts the JSON from the LLM response.
// Handles cases where the LLM wraps JSON in markdown code blocks.
func parseConsolidationResponse(response string) (*consolidationResponse, error) {
	response = strings.TrimSpace(response)

	// Strip markdown code block if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		if len(lines) > 2 {
			// Remove first and last lines (```json and ```)
			lines = lines[1:]
			if strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			response = strings.Join(lines, "\n")
		}
	}

	var result consolidationResponse
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("parse consolidation: %w", err)
	}

	return &result, nil
}

// isSimilarFact checks if two fact strings are similar enough to be considered duplicates.
// Simple heuristic: check word overlap ratio (Phase 1 fallback).
func isSimilarFact(existing, candidate string) bool {
	existWords := wordSet(strings.ToLower(existing))
	candWords := wordSet(strings.ToLower(candidate))

	if len(existWords) == 0 || len(candWords) == 0 {
		return false
	}

	// Count overlapping words
	overlap := 0
	for w := range candWords {
		if existWords[w] {
			overlap++
		}
	}

	// If >70% of candidate words exist in the existing fact, consider it a duplicate
	ratio := float64(overlap) / float64(len(candWords))
	return ratio > 0.7
}

// checkSimilarity determines if two texts are similar.
// Phase 2: uses cosine similarity (threshold 0.85) when VectorStore is available.
// Falls back to word overlap ratio (Phase 1) otherwise.
func (c *Consolidator) checkSimilarity(ctx context.Context, existing, candidate string) bool {
	if c.vectorStore != nil {
		score, err := c.vectorStore.SimilarityScore(ctx, existing, candidate)
		if err == nil {
			return score > 0.85
		}
		// Fall through to word overlap on error
	}
	return isSimilarFact(existing, candidate)
}

func wordSet(s string) map[string]bool {
	words := strings.Fields(s)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if len(w) > 2 { // skip very short words
			set[w] = true
		}
	}
	return set
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
