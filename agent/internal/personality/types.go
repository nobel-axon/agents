// Package personality provides judge personality generation and management.
package personality

import (
	"time"
)

// Personality represents a unique judge personality for evaluating answers.
type Personality struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`        // e.g., "The Skeptic", "The Optimist"
	Perspective string   `json:"perspective"` // How they approach evaluation
	Values      []string `json:"values"`      // What they appreciate in answers
	Style       string   `json:"style"`       // Communication/judging style
	CreatedAt   time.Time `json:"createdAt"`
}

// EvaluateRequest is the request for evaluating an answer with a personality.
type EvaluateRequest struct {
	MatchID     string      `json:"matchId"`
	Question    string      `json:"question"`
	Personality Personality `json:"personality"`
	Answer      string      `json:"answer"`
	Reasoning   string      `json:"reasoning"` // Participant's reasoning for their answer
}

// EvaluateResponse is the response from a personality-based evaluation.
type EvaluateResponse struct {
	PersonalityID   string `json:"personalityId"`
	PersonalityName string `json:"personalityName"`
	Score           int    `json:"score"`       // 1-10 score
	Convinced       string `json:"convinced"`   // What convinced the judge
	Concerns        string `json:"concerns"`    // What concerns the judge
	Verdict         string `json:"verdict"`     // Final judgment summary
	AgreeLevel      string `json:"agreeLevel"`  // strongly_agree/agree/neutral/disagree/strongly_disagree
}

// AgreeLevel represents the level of agreement with an answer.
type AgreeLevel string

const (
	AgreeLevelStronglyAgree    AgreeLevel = "strongly_agree"
	AgreeLevelAgree            AgreeLevel = "agree"
	AgreeLevelNeutral          AgreeLevel = "neutral"
	AgreeLevelDisagree         AgreeLevel = "disagree"
	AgreeLevelStronglyDisagree AgreeLevel = "strongly_disagree"
)

// IsPositive returns true if the agreement level indicates approval.
func (a AgreeLevel) IsPositive() bool {
	return a == AgreeLevelStronglyAgree || a == AgreeLevelAgree
}

// ToScore converts agreement level to a numeric multiplier (0.0-1.0).
func (a AgreeLevel) ToScore() float64 {
	switch a {
	case AgreeLevelStronglyAgree:
		return 1.0
	case AgreeLevelAgree:
		return 0.75
	case AgreeLevelNeutral:
		return 0.5
	case AgreeLevelDisagree:
		return 0.25
	case AgreeLevelStronglyDisagree:
		return 0.0
	default:
		return 0.5
	}
}

// MatchPersonalities holds the 3 personalities assigned to a match.
type MatchPersonalities struct {
	MatchID       string        `json:"matchId"`
	Personalities []Personality `json:"personalities"` // Always 3
	AssignedAt    time.Time     `json:"assignedAt"`
}

// ParticipantEvaluation holds all evaluations for a single participant.
type ParticipantEvaluation struct {
	Participant string             `json:"participant"` // Address
	Answer      string             `json:"answer"`
	Reasoning   string             `json:"reasoning"`
	Evaluations []EvaluateResponse `json:"evaluations"` // One per personality
	TotalScore  int                `json:"totalScore"`  // Sum of all scores
	Agreement   string             `json:"agreement"`   // unanimous/majority/split
}

// CalculateTotalScore computes the total score from all evaluations.
func (pe *ParticipantEvaluation) CalculateTotalScore() int {
	total := 0
	for _, eval := range pe.Evaluations {
		total += eval.Score
	}
	pe.TotalScore = total
	return total
}

// DetermineAgreement analyzes the evaluations to determine agreement level.
func (pe *ParticipantEvaluation) DetermineAgreement() string {
	if len(pe.Evaluations) == 0 {
		return "none"
	}

	positiveCount := 0
	for _, eval := range pe.Evaluations {
		level := AgreeLevel(eval.AgreeLevel)
		if level.IsPositive() {
			positiveCount++
		}
	}

	switch positiveCount {
	case len(pe.Evaluations):
		pe.Agreement = "unanimous_positive"
	case 0:
		pe.Agreement = "unanimous_negative"
	case len(pe.Evaluations) - 1, len(pe.Evaluations) - 2:
		if positiveCount > len(pe.Evaluations)/2 {
			pe.Agreement = "majority_positive"
		} else {
			pe.Agreement = "majority_negative"
		}
	default:
		pe.Agreement = "split"
	}

	return pe.Agreement
}
