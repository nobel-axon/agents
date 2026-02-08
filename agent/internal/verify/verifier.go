package verify

import (
	"context"
	"fmt"
	"strings"

	"github.com/axon-arena/axon-agent/internal/llm"
)

// VerificationResult holds the result of answer verification.
type VerificationResult struct {
	Correct    bool    `json:"correct"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning,omitempty"`
}

// StoredAnswer holds the stored answer for a match.
type StoredAnswer struct {
	Answer   string   `json:"answer"`
	Salt     string   `json:"salt"`
	Variants []string `json:"variants"`
	Format   string   `json:"format"`
}

// Verifier handles answer verification.
type Verifier struct {
	llmClient   llm.Provider
	storedAnswers map[string]*StoredAnswer // matchId -> stored answer
}

// NewVerifier creates a new verifier.
func NewVerifier(llmClient llm.Provider) *Verifier {
	return &Verifier{
		llmClient:     llmClient,
		storedAnswers: make(map[string]*StoredAnswer),
	}
}

// StoreAnswer stores the answer for a match (generator mode).
func (v *Verifier) StoreAnswer(matchID string, answer *StoredAnswer) {
	v.storedAnswers[matchID] = answer
}

// GetStoredAnswer retrieves the stored answer for a match.
func (v *Verifier) GetStoredAnswer(matchID string) (*StoredAnswer, bool) {
	answer, ok := v.storedAnswers[matchID]
	return answer, ok
}

// DeleteStoredAnswer removes the stored answer for a match.
func (v *Verifier) DeleteStoredAnswer(matchID string) {
	delete(v.storedAnswers, matchID)
}

// Verify verifies a submitted answer.
func (v *Verifier) Verify(ctx context.Context, matchID, submitted, question string, format FormatHint) (*VerificationResult, error) {
	// Check if we have the stored answer (generator mode)
	stored, hasStored := v.storedAnswers[matchID]
	if hasStored {
		return v.verifyExact(submitted, stored, format)
	}

	// Use LLM verification (verifier mode)
	return v.verifyWithLLM(ctx, submitted, question, format)
}

// verifyExact performs exact match verification (generator mode).
func (v *Verifier) verifyExact(submitted string, stored *StoredAnswer, format FormatHint) (*VerificationResult, error) {
	normalizedSubmitted := Normalize(submitted, format)
	normalizedStored := Normalize(stored.Answer, format)

	if normalizedSubmitted == normalizedStored {
		return &VerificationResult{
			Correct:    true,
			Confidence: 1.0,
		}, nil
	}

	// Check variants
	for _, variant := range stored.Variants {
		normalizedVariant := Normalize(variant, format)
		if normalizedSubmitted == normalizedVariant {
			return &VerificationResult{
				Correct:    true,
				Confidence: 1.0,
			}, nil
		}
	}

	return &VerificationResult{
		Correct:    false,
		Confidence: 0.0,
	}, nil
}

// verifyWithLLM uses LLM reasoning for verification (verifier mode).
func (v *Verifier) verifyWithLLM(ctx context.Context, submitted, question string, format FormatHint) (*VerificationResult, error) {
	prompt := fmt.Sprintf(`You are verifying answers in a quiz competition.

Question: %s
Expected format: %s
Submitted answer: %s

Is this answer correct? Consider common variations and the expected format.
Reply with ONLY "YES" or "NO" followed by a brief explanation.`, question, format, submitted)

	response, err := v.llmClient.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM verification failed: %w", err)
	}

	response = strings.TrimSpace(response)
	upperResponse := strings.ToUpper(response)

	result := &VerificationResult{
		Reasoning: response,
	}

	if strings.HasPrefix(upperResponse, "YES") {
		result.Correct = true
		result.Confidence = 0.9
		// Lower confidence for hedged responses
		if strings.Contains(strings.ToLower(response), "probably") ||
			strings.Contains(strings.ToLower(response), "likely") ||
			strings.Contains(strings.ToLower(response), "might") {
			result.Confidence = 0.7
		}
	} else if strings.HasPrefix(upperResponse, "NO") {
		result.Correct = false
		result.Confidence = 0.9
	} else {
		// Unclear response
		result.Correct = false
		result.Confidence = 0.5
	}

	return result, nil
}
