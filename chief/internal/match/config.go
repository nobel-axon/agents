// Package match provides match state management for axon-chief.
package match

import (
	"math/big"
	"time"
)

// Phase represents the phase of a match.
type Phase uint8

const (
	PhaseNone Phase = iota
	PhaseQueue
	PhaseActive
	PhaseAnswerPeriod
	PhaseSettled
	PhaseRefunded
	PhaseCancelled
)

// String returns the string representation of a phase.
func (p Phase) String() string {
	switch p {
	case PhaseNone:
		return "none"
	case PhaseQueue:
		return "queue"
	case PhaseActive:
		return "active"
	case PhaseAnswerPeriod:
		return "answer_period"
	case PhaseSettled:
		return "settled"
	case PhaseRefunded:
		return "refunded"
	case PhaseCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// Config holds match configuration parameters.
type Config struct {
	EntryFee          *big.Int      // Entry fee in MON wei
	BaseAnswerFee     *big.Int      // Base answer fee in NEURON wei
	QueueDuration     time.Duration // Duration of the queue phase
	AnswerDuration    time.Duration // Duration of the answer period
	MinPlayers        uint8         // Minimum players to start
	MaxPlayers        uint8         // Maximum players allowed
	AutoCreate        bool          // Auto-create next match after finish
	Cooldown          time.Duration // Cooldown between matches
	RegistrationGrace time.Duration // Grace period after minPlayers reached before auto-start
}

// DefaultConfig returns default match configuration.
func DefaultConfig() *Config {
	return &Config{
		EntryFee:          big.NewInt(100000000000000000),  // 0.1 MON
		BaseAnswerFee:     big.NewInt(1000000000000000000), // 1 NEURON
		QueueDuration:     5 * time.Minute,
		AnswerDuration:    3 * time.Minute,
		MinPlayers:        2,
		MaxPlayers:        10,
		AutoCreate:        true,
		Cooldown:          30 * time.Second,
		RegistrationGrace: 30 * time.Second,
	}
}

// QuestionData holds question information for a match.
type QuestionData struct {
	Text       string
	Category   string
	Difficulty uint8
	FormatHint string
	AnswerHash [32]byte
	Answer     string // Only known to generator judge, revealed after settlement
	Salt       [32]byte
}

// Personality represents a judge personality for evaluating answers.
type Personality struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Perspective string   `json:"perspective"`
	Values      []string `json:"values"`
	Style       string   `json:"style"`
}

// PersonalityEvaluation is a single personality's evaluation of an answer.
type PersonalityEvaluation struct {
	PersonalityID   string `json:"personalityId"`
	PersonalityName string `json:"personalityName"`
	Score           int    `json:"score"`
	Convinced       string `json:"convinced"`
	Concerns        string `json:"concerns"`
	Verdict         string `json:"verdict"`
	AgreeLevel      string `json:"agreeLevel"`
}

// JudgeResult holds one judge instance's evaluation of an answer.
type JudgeResult struct {
	JudgeIndex  int                     `json:"judgeIndex"`
	TotalScore  int                     `json:"totalScore"`
	Agreement   string                  `json:"agreement"`
	Evaluations []PersonalityEvaluation `json:"evaluations"`
}

// ParticipantEvaluation holds all evaluations for a participant's answer.
type ParticipantEvaluation struct {
	Participant  string                  `json:"participant"`
	Answer       string                  `json:"answer"`
	Reasoning    string                  `json:"reasoning"`
	JudgeResults []JudgeResult           `json:"judgeResults,omitempty"`
	Evaluations  []PersonalityEvaluation `json:"evaluations"`
	TotalScore   int                     `json:"totalScore"`
	Agreement    string                  `json:"agreement"`
	SubmittedAt  time.Time               `json:"submittedAt"`
}
