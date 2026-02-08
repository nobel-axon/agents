// Package judge provides judge agent coordination for axon-chief.
package judge

import "context"

// PhilosopherClient defines the interface for the Philosopher agent.
// The Philosopher generates Nobel Inquiries, proposes questions, and reveals answers.
type PhilosopherClient interface {
	Health(ctx context.Context) (*HealthResponse, error)
	GenerateQuestion(ctx context.Context, matchID, category string, difficulty int, excludeRecent []string) (*GenerateQuestionResponse, error)
	ValidateQuestion(ctx context.Context, req *ValidateQuestionRequest) (*ValidateQuestionResponse, error)
	Reveal(ctx context.Context, matchID string) (*RevealResponse, error)
}

// DirectorClient defines the interface for the Director agent.
// The Director creates unique judge personalities for each match.
type DirectorClient interface {
	Health(ctx context.Context) (*HealthResponse, error)
	GeneratePersonalities(ctx context.Context, matchID, category string) (*GeneratePersonalitiesResponse, error)
	GetPersonalities(ctx context.Context, matchID string) (*GeneratePersonalitiesResponse, error)
}

// JudgeClient defines the interface for a Judge agent.
// Judges evaluate answers using personality-based scoring, verify answers, and generate commentary.
type JudgeClient interface {
	Health(ctx context.Context) (*HealthResponse, error)
	VerifyAnswer(ctx context.Context, matchID, submitted, question, formatHint string) (*VerifyAnswerResponse, error)
	ValidateQuestion(ctx context.Context, req *ValidateQuestionRequest) (*ValidateQuestionResponse, error)
	EvaluateAnswer(ctx context.Context, matchID, question, answer, reasoning string, personalities []Personality) (*EvaluateAnswerResponse, error)
	Commentary(ctx context.Context, req *CommentaryRequest) (*CommentaryResponse, error)
}

// Compile-time interface satisfaction checks.
var (
	_ PhilosopherClient = (*Client)(nil)
	_ DirectorClient    = (*Client)(nil)
	_ JudgeClient       = (*Client)(nil)
)
