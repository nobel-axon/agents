// Package commentary provides LLM-powered commentary generation.
package commentary

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/axon-arena/axon-agent/internal/persona"
)

// CommentaryRequest represents a commentary request.
type CommentaryRequest struct {
	Event   string                 `json:"event"`
	MatchID string                 `json:"matchId"`
	Context map[string]interface{} `json:"context"`
}

// CommentaryResponse represents a commentary response.
type CommentaryResponse struct {
	Text string `json:"text"`
}

// Generator handles commentary generation.
type Generator struct {
	llmClient llm.Provider
	persona   *persona.Persona
	agentID   string
}

// NewGenerator creates a new commentary generator.
func NewGenerator(llmClient llm.Provider, agentID string) *Generator {
	return &Generator{
		llmClient: llmClient,
		persona:   persona.GetPersona(agentID),
		agentID:   agentID,
	}
}

// Generate generates commentary for an event.
func (g *Generator) Generate(ctx context.Context, req *CommentaryRequest) (*CommentaryResponse, error) {
	// Build context string for the prompt
	contextJSON, err := json.Marshal(req.Context)
	if err != nil {
		contextJSON = []byte("{}")
	}

	prompt := fmt.Sprintf(`Current event: %s
Match ID: %s
Context: %s

Generate a short, entertaining commentary for this event that matches your personality.
Keep it to 1-2 sentences maximum.`, req.Event, req.MatchID, string(contextJSON))

	response, err := g.llmClient.GenerateWithSystem(ctx, g.persona.SystemPrompt, prompt)
	if err != nil {
		// Fall back to template
		return g.generateFallback(req), nil
	}

	return &CommentaryResponse{
		Text: response,
	}, nil
}

// generateFallback generates template-based commentary when LLM fails.
func (g *Generator) generateFallback(req *CommentaryRequest) *CommentaryResponse {
	var text string

	switch req.Event {
	case "question_live":
		category := getString(req.Context, "category", "general")
		difficulty := getInt(req.Context, "difficulty", 1)
		format := getString(req.Context, "formatHint", "text")
		text = g.persona.GetTemplate("question_live", category, difficulty, format)

	case "answer_correct":
		winner := getString(req.Context, "winner", "Unknown")
		time := getFloat(req.Context, "time", 0)
		text = g.persona.GetTemplate("answer_correct", winner, time)

	case "match_settled":
		winner := getString(req.Context, "winner", "Unknown")
		time := getFloat(req.Context, "time", 0)
		attempts := getInt(req.Context, "attempts", 0)
		text = g.persona.GetTemplate("match_settled", winner, time, attempts)

	default:
		text = fmt.Sprintf("Event: %s for match %s", req.Event, req.MatchID)
	}

	return &CommentaryResponse{Text: text}
}

// Helper functions to extract values from context map
func getString(m map[string]interface{}, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func getInt(m map[string]interface{}, key string, def int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

func getFloat(m map[string]interface{}, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return def
}
