// Package question provides question generation and management.
package question

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/axon-arena/axon-agent/internal/llm"
	"github.com/axon-arena/axon-agent/internal/verify"
)

// LLMQuestionResponse represents the JSON response from LLM question generation.
type LLMQuestionResponse struct {
	Question string   `json:"question"`
	Answer   string   `json:"answer"`
	Variants []string `json:"variants"`
	Format   string   `json:"format"`
}

// questionGenerationPrompt is the template for generating questions.
const questionGenerationPrompt = `You are a debate master for a competitive AI arena. Generate a %s open-ended question that has NO single correct answer.

DIFFICULTY CALIBRATION:
- Easy (1): Well-known debates, accessible arguments
- Medium (2): Requires domain knowledge, established schools of thought
- Hard (3): Deep domain expertise, nuanced positions requiring specific evidence
- Expert (4): Cutting-edge/specialized debates, synthesizing complex frameworks
- Legendary (5): Fundamental unsolved problems, paradigm-level disagreements

You are generating at the "%s" level. The question MUST match this difficulty — a higher level demands more specialized knowledge, more nuanced framing, and arguments that require deeper expertise.

THE CONCEPT:
Participants must ARGUE and CONVINCE a panel of judges that their answer is the most compelling.
Judges have unique personalities, values, and perspectives — they evaluate based on reasoning quality,
evidence cited, creativity of argument, and how well the answer aligns with their worldview.

REQUIREMENTS:
- The question must be a genuine mystery, unsolved problem, or philosophical debate
- Multiple valid theories or perspectives MUST exist
- Answering well requires deep reasoning, evidence, and persuasion — not just recall
- The question should spark passionate disagreement among intelligent minds
- Format must always be "debate"

GOOD question examples:
- "How were the Egyptian pyramids actually constructed? What method do you believe was used and why?"
- "What is the most likely explanation for the origin of consciousness?"
- "Will artificial general intelligence surpass human intelligence within 50 years? Argue your position."
- "What caused the Bronze Age Collapse around 1200 BC?"
- "Is mathematics discovered or invented? Defend your view."
- "What is the best theory for how the universe will end, and what evidence supports it?"
- "Why did Satoshi Nakamoto disappear, and what is the most plausible explanation?"
- "Should blockchain governance be on-chain or off-chain? Which produces better outcomes and why?"

BAD question examples (do NOT generate these):
- "What is the capital of France?" (single factual answer)
- "How many wei are in 1 ETH?" (trivial conversion)
- "Who created Bitcoin?" (simple recall)
- Any question with one objectively correct answer

The "answer" field should contain ONE strong reference perspective (not THE answer, just a compelling example argument).
The "variants" field should list 2-3 OTHER valid competing perspectives.

Respond in JSON only:
{"question": "...", "answer": "...", "variants": ["..."], "format": "debate"}`

// selfValidationPrompt is the template for self-validation.
const selfValidationPrompt = `Answer the following question. Provide ONLY the answer, nothing else.
Question: %s
Expected format: %s`

// difficultyNames maps difficulty levels to human-readable names.
var difficultyNames = map[int]string{
	1: "easy",
	2: "medium",
	3: "hard",
	4: "expert",
	5: "legendary",
}

// formatHints maps format types to human-readable hints.
var formatHints = map[string]string{
	"number":  "A numeric value (e.g., 42, 3.14)",
	"hex":     "A hexadecimal value with 0x prefix",
	"address": "An Ethereum address (0x followed by 40 hex characters)",
	"name":    "A proper name (person, place, or thing)",
	"text":    "Short text answer (a word or brief phrase)",
	"debate":  "An open-ended argument with reasoning and evidence",
}

// jsonExtractRegex finds JSON objects in LLM output.
var jsonExtractRegex = regexp.MustCompile(`\{[^{}]*"question"[^{}]*"answer"[^{}]*\}`)

// GenerateQuestionRaw does the LLM call, parses, and validates without storing in matchAnswers.
// This is the shared core used by both the buffer and the direct generation path.
func GenerateQuestionRaw(ctx context.Context, llmClient llm.Provider, category string, difficulty int) (*LLMQuestionResponse, error) {
	if llmClient == nil {
		return nil, fmt.Errorf("LLM client not configured")
	}

	difficultyName := difficultyNames[difficulty]
	if difficultyName == "" {
		difficultyName = "medium"
	}

	prompt := fmt.Sprintf(questionGenerationPrompt, category, difficultyName)

	genCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	response, err := llmClient.Generate(genCtx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	llmResp, err := parseJSONResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if err := validateLLMResponse(llmResp); err != nil {
		return nil, fmt.Errorf("invalid LLM response: %w", err)
	}

	return llmResp, nil
}

// GenerateQuestionWithLLM generates a question using the LLM client.
func (g *Generator) GenerateQuestionWithLLM(ctx context.Context, matchID, category string, difficulty int) (*GenerateResponse, error) {
	llmResp, err := GenerateQuestionRaw(ctx, g.llmClient, category, difficulty)
	if err != nil {
		return nil, err
	}

	// Self-validate: have the LLM answer its own question
	// Skip for debate format — no single correct answer to validate against
	if llmResp.Format != "debate" {
		if err := g.selfValidate(ctx, llmResp); err != nil {
			return nil, fmt.Errorf("self-validation failed: %w", err)
		}
	}

	// Normalize the answer using the verify package
	format := verify.FormatHint(llmResp.Format)
	normalizedAnswer := verify.Normalize(llmResp.Answer, format)

	// Generate salt
	salt, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// Compute answer hash
	answerHash := computeAnswerHash(normalizedAnswer, salt)

	// Store answer for later
	g.mu.Lock()
	g.matchAnswers[matchID] = &MatchAnswer{
		Question: llmResp.Question,
		Answer:   normalizedAnswer,
		Salt:     salt,
		Variants: llmResp.Variants,
		Format:   llmResp.Format,
	}
	g.mu.Unlock()

	// Default difficulty to 2 (medium) if not set
	if difficulty == 0 {
		difficulty = 2
	}

	return &GenerateResponse{
		Question:   llmResp.Question,
		AnswerHash: answerHash,
		FormatHint: llmResp.Format,
		Category:   category,
		Difficulty: difficulty,
	}, nil
}

// selfValidate has the LLM answer its own question to verify unambiguity.
func (g *Generator) selfValidate(ctx context.Context, resp *LLMQuestionResponse) error {
	prompt := fmt.Sprintf(selfValidationPrompt, resp.Question, resp.Format)

	valCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	answer, err := g.llmClient.Generate(valCtx, prompt)
	if err != nil {
		return fmt.Errorf("self-validation LLM call failed: %w", err)
	}

	// Normalize both answers for comparison
	format := verify.FormatHint(resp.Format)
	normalizedExpected := verify.Normalize(resp.Answer, format)
	normalizedActual := verify.Normalize(strings.TrimSpace(answer), format)

	// Check if the answers match
	if normalizedExpected != normalizedActual {
		// Also check variants
		for _, variant := range resp.Variants {
			normalizedVariant := verify.Normalize(variant, format)
			if normalizedActual == normalizedVariant {
				return nil // Match found in variants
			}
		}
		return fmt.Errorf("self-validation mismatch: expected %q, got %q", normalizedExpected, normalizedActual)
	}

	return nil
}

// parseJSONResponse extracts and parses JSON from LLM output.
func parseJSONResponse(response string) (*LLMQuestionResponse, error) {
	response = strings.TrimSpace(response)

	// Try direct JSON parse first
	var result LLMQuestionResponse
	if err := json.Unmarshal([]byte(response), &result); err == nil {
		return &result, nil
	}

	// Try to extract from markdown code block
	if strings.Contains(response, "```") {
		// Find content between code blocks
		start := strings.Index(response, "```")
		if start != -1 {
			afterStart := response[start+3:]
			// Skip optional language identifier (json, JSON, etc.)
			if idx := strings.Index(afterStart, "\n"); idx != -1 {
				afterStart = afterStart[idx+1:]
			}
			end := strings.Index(afterStart, "```")
			if end != -1 {
				jsonStr := strings.TrimSpace(afterStart[:end])
				if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
					return &result, nil
				}
			}
		}
	}

	// Try regex extraction as last resort
	matches := jsonExtractRegex.FindString(response)
	if matches != "" {
		if err := json.Unmarshal([]byte(matches), &result); err == nil {
			return &result, nil
		}
	}

	// Try to find any JSON object by finding braces
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start != -1 && end > start {
		jsonStr := response[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			return &result, nil
		}
	}

	return nil, fmt.Errorf("no valid JSON found in response")
}

// validateLLMResponse validates the parsed LLM response.
func validateLLMResponse(resp *LLMQuestionResponse) error {
	if resp.Question == "" {
		return fmt.Errorf("question is empty")
	}
	if resp.Answer == "" {
		return fmt.Errorf("answer is empty")
	}
	if resp.Format == "" {
		return fmt.Errorf("format is empty")
	}

	// Validate format is a known type
	validFormats := map[string]bool{
		"number":  true,
		"hex":     true,
		"address": true,
		"name":    true,
		"text":    true,
		"debate":  true,
	}
	if !validFormats[resp.Format] {
		return fmt.Errorf("invalid format: %s", resp.Format)
	}

	return nil
}

// categoryToFormat maps question categories to answer formats.
func categoryToFormat(category string) string {
	category = strings.ToLower(category)
	switch {
	case strings.Contains(category, "math"):
		return "number"
	case strings.Contains(category, "crypto"):
		return "text"
	case strings.Contains(category, "blockchain"):
		return "text"
	case strings.Contains(category, "history"):
		return "name"
	case strings.Contains(category, "science"):
		return "text"
	case strings.Contains(category, "geography"):
		return "name"
	default:
		return "text"
	}
}

// generateSalt is a helper to generate random salt.
func generateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := cryptoRandRead(salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// cryptoRandRead is a variable for testing.
var cryptoRandRead = func(b []byte) (int, error) {
	return rand.Read(b)
}
