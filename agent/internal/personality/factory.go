package personality

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/google/uuid"
)

// Factory generates unique judge personalities via LLM.
type Factory struct {
	llmClient llm.Provider
}

// extractJSON extracts and validates a JSON object from a response string.
func extractJSON(response string) (string, error) {
	response = strings.TrimSpace(response)

	// Remove markdown code blocks
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Find JSON object boundaries
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start == -1 || end == -1 || end <= start {
		return "", fmt.Errorf("no valid JSON object found in response")
	}

	jsonStr := response[start : end+1]

	// Sanitize common LLM encoding issues (smart quotes, etc.)
	jsonStr = sanitizeJSON(jsonStr)

	// Validate it's actually valid JSON
	var test map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &test); err != nil {
		return "", fmt.Errorf("extracted text is not valid JSON: %w", err)
	}

	return jsonStr, nil
}

// sanitizeJSON fixes common encoding issues from LLM responses.
func sanitizeJSON(s string) string {
	// Replace smart/curly quotes with standard quotes
	// Using Unicode escape sequences to avoid encoding issues in source
	replacements := map[string]string{
		"\u201C": "\"", // Left double quote "
		"\u201D": "\"", // Right double quote "
		"\u2018": "'",  // Left single quote '
		"\u2019": "'",  // Right single quote '
		"\u00AB": "\"", // Left guillemet «
		"\u00BB": "\"", // Right guillemet »
		"\u201E": "\"", // Low double quote „
		"\u201F": "\"", // Double high-reversed-9 quote ‟
		"\u2032": "'",  // Prime ′
		"\u2033": "\"", // Double prime ″
		"\u201A": "'",  // Single low-9 quote ‚
		"\u201B": "'",  // Single high-reversed-9 quote ‛
		"\u00A0": " ",  // Non-breaking space
		"\u2028": "",   // Line separator
		"\u2029": "",   // Paragraph separator
	}

	for old, new := range replacements {
		s = strings.ReplaceAll(s, old, new)
	}

	return s
}

// NewFactory creates a new personality factory.
func NewFactory(llmClient llm.Provider) *Factory {
	return &Factory{
		llmClient: llmClient,
	}
}

// generateSystemPrompt returns the system prompt for personality generation.
func generateSystemPrompt() string {
	return `You are a creative personality designer for a quiz judging panel.
Your task is to create unique, memorable judge personalities that will evaluate quiz answers.

Each personality should have:
- A distinctive name (like "The Skeptic", "The Optimist", "The Pragmatist")
- A clear perspective on how they approach evaluation
- 3-5 values they appreciate in answers
- A communication style that reflects their character

Make personalities diverse and interesting. They should feel like real characters with depth.
Avoid generic descriptions - each personality should be memorable and distinct.`
}

// generatePrompt returns the prompt for generating a personality.
func generatePrompt(category string, existingNames []string) string {
	exclusion := ""
	if len(existingNames) > 0 {
		exclusion = fmt.Sprintf("\n\nAvoid these personality names that already exist: %s", strings.Join(existingNames, ", "))
	}

	categoryHint := ""
	if category != "" && category != "general" {
		categoryHint = fmt.Sprintf(" The quiz is focused on %s topics.", category)
	}

	return fmt.Sprintf(`Generate a unique judge personality for a quiz competition.%s%s

RESPONSE FORMAT:
You MUST respond with ONLY valid JSON. No markdown, no explanation, no text before or after.

Use this exact schema:
{
  "name": "string (required, e.g., 'The Skeptic')",
  "perspective": "string (required, 1-2 sentences)",
  "values": ["string array, 3-5 items"],
  "style": "string (required, 1 sentence)"
}

Example valid response:
{"name":"The Pragmatist","perspective":"Values practical wisdom over theoretical perfection, seeking answers that work in the real world.","values":["practicality","clarity","real-world applicability"],"style":"Direct and solution-focused, cutting through abstraction to find what matters."}`, categoryHint, exclusion)
}

// Generate creates a single new personality.
func (f *Factory) Generate(ctx context.Context, category string, existingNames []string) (*Personality, error) {
	prompt := generatePrompt(category, existingNames)

	response, err := f.llmClient.GenerateWithSystem(ctx, generateSystemPrompt(), prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Extract and validate JSON from response
	jsonStr, err := extractJSON(response)
	if err != nil {
		return nil, fmt.Errorf("failed to extract JSON: %w (response: %s)", err, response)
	}

	// Parse JSON response
	var rawPersonality struct {
		Name        string   `json:"name"`
		Perspective string   `json:"perspective"`
		Values      []string `json:"values"`
		Style       string   `json:"style"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawPersonality); err != nil {
		return nil, fmt.Errorf("failed to parse personality JSON: %w (response: %s)", err, response)
	}

	// Validate required fields
	if rawPersonality.Name == "" {
		return nil, fmt.Errorf("missing required field: name")
	}
	if rawPersonality.Perspective == "" {
		return nil, fmt.Errorf("missing required field: perspective")
	}
	if rawPersonality.Style == "" {
		return nil, fmt.Errorf("missing required field: style")
	}
	if len(rawPersonality.Values) == 0 {
		return nil, fmt.Errorf("missing required field: values (must have at least one)")
	}

	return &Personality{
		ID:          uuid.New().String(),
		Name:        rawPersonality.Name,
		Perspective: rawPersonality.Perspective,
		Values:      rawPersonality.Values,
		Style:       rawPersonality.Style,
		CreatedAt:   time.Now(),
	}, nil
}

// batchDiversePrompt is the prompt for generating diverse personalities in a single LLM call.
const batchDiversePrompt = `Generate exactly %d judge personalities for a %s debate competition.

MANDATORY DIVERSITY RULES:
1. Personality A: SKEPTIC/TRADITIONALIST — values evidence, established knowledge, proven approaches
2. Personality B: VISIONARY/INNOVATOR — values creativity, new paradigms, bold thinking
3. Personality C: WILDCARD — unexpected angle (pragmatist, aesthete, contrarian, historian, etc.)

Their perspectives MUST create genuine disagreement. Given any topic, at least two should naturally take opposing sides.

Each personality needs:
- "name": distinctive title (e.g., "The Skeptic", "The Visionary")
- "perspective": 1-2 sentences on how they evaluate
- "values": 3-5 things they appreciate in answers
- "style": 1 sentence communication style

RESPONSE FORMAT: JSON array ONLY. No markdown, no explanation.
[{"name":"...","perspective":"...","values":[...],"style":"..."}, ...]`

// GenerateBatchDiverse creates multiple diverse personalities in a single LLM call.
func (f *Factory) GenerateBatchDiverse(ctx context.Context, count int, category string) ([]*Personality, error) {
	prompt := fmt.Sprintf(batchDiversePrompt, count, category)

	genCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	response, err := f.llmClient.GenerateWithSystem(genCtx, generateSystemPrompt(), prompt)
	if err != nil {
		// Fall back to sequential generation
		return f.GenerateBatch(ctx, count, category)
	}

	jsonStr, err := extractJSONArray(response)
	if err != nil {
		// Fall back to sequential generation
		return f.GenerateBatch(ctx, count, category)
	}

	var rawPersonalities []struct {
		Name        string   `json:"name"`
		Perspective string   `json:"perspective"`
		Values      []string `json:"values"`
		Style       string   `json:"style"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawPersonalities); err != nil {
		return f.GenerateBatch(ctx, count, category)
	}

	if len(rawPersonalities) < count {
		return f.GenerateBatch(ctx, count, category)
	}

	personalities := make([]*Personality, 0, count)
	for _, raw := range rawPersonalities[:count] {
		if raw.Name == "" || raw.Perspective == "" || raw.Style == "" || len(raw.Values) == 0 {
			return f.GenerateBatch(ctx, count, category)
		}
		personalities = append(personalities, &Personality{
			ID:          uuid.New().String(),
			Name:        raw.Name,
			Perspective: raw.Perspective,
			Values:      raw.Values,
			Style:       raw.Style,
			CreatedAt:   time.Now(),
		})
	}

	return personalities, nil
}

// extractJSONArray extracts a JSON array from a response string.
func extractJSONArray(response string) (string, error) {
	response = strings.TrimSpace(response)

	// Remove markdown code blocks
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Sanitize common LLM encoding issues
	response = sanitizeJSON(response)

	// Find JSON array boundaries
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")

	if start == -1 || end == -1 || end <= start {
		return "", fmt.Errorf("no valid JSON array found in response")
	}

	jsonStr := response[start : end+1]

	// Validate it's actually valid JSON
	var test []interface{}
	if err := json.Unmarshal([]byte(jsonStr), &test); err != nil {
		return "", fmt.Errorf("extracted text is not valid JSON array: %w", err)
	}

	return jsonStr, nil
}

// GenerateBatch creates multiple unique personalities.
func (f *Factory) GenerateBatch(ctx context.Context, count int, category string) ([]*Personality, error) {
	personalities := make([]*Personality, 0, count)
	existingNames := make([]string, 0, count)

	for i := 0; i < count; i++ {
		p, err := f.Generate(ctx, category, existingNames)
		if err != nil {
			// If we have at least one personality, return what we have
			if len(personalities) > 0 {
				return personalities, nil
			}
			return nil, err
		}

		personalities = append(personalities, p)
		existingNames = append(existingNames, p.Name)
	}

	return personalities, nil
}

// GenerateForMatch creates exactly 3 unique personalities for a match.
func (f *Factory) GenerateForMatch(ctx context.Context, matchID, category string) (*MatchPersonalities, error) {
	personalities, err := f.GenerateBatch(ctx, 3, category)
	if err != nil {
		return nil, err
	}

	if len(personalities) < 3 {
		return nil, fmt.Errorf("failed to generate 3 personalities, only got %d", len(personalities))
	}

	// Convert to non-pointer slice
	ps := make([]Personality, 3)
	for i, p := range personalities[:3] {
		ps[i] = *p
	}

	return &MatchPersonalities{
		MatchID:       matchID,
		Personalities: ps,
		AssignedAt:    time.Now(),
	}, nil
}

// DefaultPersonalities returns a set of fallback personalities if LLM generation fails.
func DefaultPersonalities() []Personality {
	return []Personality{
		{
			ID:          "default-skeptic",
			Name:        "The Skeptic",
			Perspective: "Approaches every answer with healthy doubt, looking for evidence and logical consistency.",
			Values:      []string{"precision", "evidence", "logical reasoning", "thoroughness"},
			Style:       "Questions assumptions and demands proof before accepting claims.",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "default-optimist",
			Name:        "The Optimist",
			Perspective: "Looks for the merit in answers and appreciates creative thinking even if imperfect.",
			Values:      []string{"creativity", "effort", "partial correctness", "innovative thinking"},
			Style:       "Encourages good attempts while noting areas for improvement.",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "default-purist",
			Name:        "The Purist",
			Perspective: "Values technical accuracy and proper form above all else.",
			Values:      []string{"accuracy", "proper formatting", "completeness", "correct terminology"},
			Style:       "Strict but fair, with high standards for correctness.",
			CreatedAt:   time.Now(),
		},
	}
}
