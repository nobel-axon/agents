package personality

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/axon-arena/axon-agent/internal/llm"
)

// Evaluator evaluates answers using judge personalities.
type Evaluator struct {
	llmClient llm.Provider
}

// NewEvaluator creates a new personality-based evaluator.
func NewEvaluator(llmClient llm.Provider) *Evaluator {
	return &Evaluator{
		llmClient: llmClient,
	}
}

// evaluationSystemPrompt returns the system prompt for evaluation.
func evaluationSystemPrompt(p *Personality) string {
	values := strings.Join(p.Values, ", ")
	return fmt.Sprintf(`You are %s, a judge in a competitive debate arena. You are NOT a neutral referee — you have STRONG personal opinions shaped by your worldview.

Your perspective: %s

Your core values: %s

Your judging style: %s

CRITICAL: You have your OWN stance on every topic. Before evaluating the participant's answer, you MUST first form your own opinion on the question based on your values and perspective. Then judge the answer based on:
1. How well it aligns with OR intelligently challenges YOUR worldview (you respect well-argued opposing views)
2. Whether the reasoning would convince YOU specifically, given your values
3. The quality of evidence and logic FROM YOUR PERSPECTIVE

Different judges WILL and SHOULD score the same answer differently. A security-focused judge might love an answer about trustless systems, while a pragmatist judge might dismiss it as idealistic. This disagreement is the WHOLE POINT.

You are NOT evaluating generic "answer quality" — you are judging whether this answer resonates with YOU as %s.`, p.Name, p.Perspective, values, p.Style, p.Name)
}

// evaluationPrompt returns the prompt for evaluating an answer.
func evaluationPrompt(question, answer, reasoning string) string {
	reasoningSection := ""
	if reasoning != "" {
		reasoningSection = fmt.Sprintf("\n\nPARTICIPANT'S REASONING:\n%s", reasoning)
	}

	return fmt.Sprintf(`DEBATE QUESTION:
%s

PARTICIPANT'S ANSWER:
%s%s

STEP 1 — Form your own stance on this question FIRST (do this internally, don't output it).
STEP 2 — Now evaluate the participant's answer.

Scoring guide (use the FULL range, don't cluster around 5-7):
- 9-10: This answer deeply resonates with your worldview, or brilliantly challenges it with evidence you can't dismiss
- 7-8: Strong argument that mostly aligns with your values, or a compelling opposing view with solid reasoning
- 5-6: Decent argument but doesn't particularly move you — generic or surface-level
- 3-4: Fundamentally disagrees with your values AND fails to provide convincing evidence to change your mind
- 1-2: Lazy, unsupported, or directly contradicts what you believe with no justification

IMPORTANT: Your score reflects YOUR personal reaction, not objective quality. Two judges SHOULD give different scores to the same answer. If the answer argues against something you deeply value, score it LOW unless the reasoning is extraordinary.

Respond with ONLY this JSON:
{
  "score": <1-10>,
  "convinced": "<what specifically resonated with your values, or 'Nothing — this contradicts my core belief that...' >",
  "concerns": "<what made you uncomfortable or what's missing from YOUR perspective>",
  "verdict": "<your personal judgment as this character in 1-2 sentences>",
  "agreeLevel": "<strongly_agree|agree|neutral|disagree|strongly_disagree>"
}`, question, answer, reasoningSection)
}

// Evaluate evaluates an answer using a specific personality.
func (e *Evaluator) Evaluate(ctx context.Context, req *EvaluateRequest) (*EvaluateResponse, error) {
	systemPrompt := evaluationSystemPrompt(&req.Personality)
	prompt := evaluationPrompt(req.Question, req.Answer, req.Reasoning)

	response, err := e.llmClient.GenerateWithSystem(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM evaluation failed: %w", err)
	}

	// Clean up response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Parse JSON response
	var rawEval struct {
		Score      int    `json:"score"`
		Convinced  string `json:"convinced"`
		Concerns   string `json:"concerns"`
		Verdict    string `json:"verdict"`
		AgreeLevel string `json:"agreeLevel"`
	}

	if err := json.Unmarshal([]byte(response), &rawEval); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation JSON: %w (response: %s)", err, response)
	}

	// Validate and normalize score
	if rawEval.Score < 1 {
		rawEval.Score = 1
	} else if rawEval.Score > 10 {
		rawEval.Score = 10
	}

	// Normalize agree level
	agreeLevel := normalizeAgreeLevel(rawEval.AgreeLevel)

	return &EvaluateResponse{
		PersonalityID:   req.Personality.ID,
		PersonalityName: req.Personality.Name,
		Score:           rawEval.Score,
		Convinced:       rawEval.Convinced,
		Concerns:        rawEval.Concerns,
		Verdict:         rawEval.Verdict,
		AgreeLevel:      string(agreeLevel),
	}, nil
}

// EvaluateWithAll evaluates an answer with all provided personalities.
func (e *Evaluator) EvaluateWithAll(ctx context.Context, matchID, question, answer, reasoning string, personalities []Personality) (*ParticipantEvaluation, error) {
	pe := &ParticipantEvaluation{
		Answer:      answer,
		Reasoning:   reasoning,
		Evaluations: make([]EvaluateResponse, 0, len(personalities)),
	}

	for _, p := range personalities {
		req := &EvaluateRequest{
			MatchID:     matchID,
			Question:    question,
			Personality: p,
			Answer:      answer,
			Reasoning:   reasoning,
		}

		eval, err := e.Evaluate(ctx, req)
		if err != nil {
			// On error, create a neutral evaluation
			eval = &EvaluateResponse{
				PersonalityID:   p.ID,
				PersonalityName: p.Name,
				Score:           5,
				Convinced:       "Unable to evaluate",
				Concerns:        fmt.Sprintf("Evaluation error: %v", err),
				Verdict:         "Evaluation could not be completed",
				AgreeLevel:      string(AgreeLevelNeutral),
			}
		}

		pe.Evaluations = append(pe.Evaluations, *eval)
	}

	pe.CalculateTotalScore()
	pe.DetermineAgreement()

	return pe, nil
}

// normalizeAgreeLevel converts various string formats to AgreeLevel.
func normalizeAgreeLevel(s string) AgreeLevel {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")

	switch s {
	case "strongly_agree", "stronglyagree", "strong_agree":
		return AgreeLevelStronglyAgree
	case "agree":
		return AgreeLevelAgree
	case "neutral":
		return AgreeLevelNeutral
	case "disagree":
		return AgreeLevelDisagree
	case "strongly_disagree", "stronglydisagree", "strong_disagree":
		return AgreeLevelStronglyDisagree
	default:
		return AgreeLevelNeutral
	}
}
