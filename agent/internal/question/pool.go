// Package question provides question generation and management.
package question

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/axon-arena/axon-agent/internal/llm"
	"golang.org/x/crypto/sha3"
)

// Question represents a question with its answer metadata.
type Question struct {
	ID         string   `json:"id"`
	Category   string   `json:"category"`
	Difficulty int      `json:"difficulty"`
	Question   string   `json:"question"`
	Answer     string   `json:"answer"`
	Variants   []string `json:"variants"`
	Format     string   `json:"format"`
}

// MatchAnswer holds the answer data for a match.
type MatchAnswer struct {
	Question string
	Answer   string
	Salt     []byte
	Variants []string
	Format   string
}

// Generator handles question generation and answer storage.
type Generator struct {
	matchAnswers   map[string]*MatchAnswer
	mu             sync.RWMutex
	llmClient      llm.Provider
	llmEnabled     bool
	questionBuffer *QuestionBuffer // nil if pre-stocking disabled
}

// GeneratorOption is a functional option for configuring a Generator.
type GeneratorOption func(*Generator)

// WithLLMClient configures the generator to use LLM-based question generation.
func WithLLMClient(client llm.Provider) GeneratorOption {
	return func(g *Generator) {
		g.llmClient = client
		g.llmEnabled = true
	}
}

// WithPreStocking configures the generator to use a pre-stocked question buffer.
func WithPreStocking(buf *QuestionBuffer) GeneratorOption {
	return func(g *Generator) {
		g.questionBuffer = buf
	}
}

// BufferSizes returns the current buffer sizes per category, or nil if no buffer.
func (g *Generator) BufferSizes() map[string]int {
	if g.questionBuffer == nil {
		return nil
	}
	return g.questionBuffer.Sizes()
}

// NewGenerator creates a new question generator.
func NewGenerator(opts ...GeneratorOption) *Generator {
	g := &Generator{
		matchAnswers: make(map[string]*MatchAnswer),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// GenerateQuestion generates a question and the answer hash.
// Priority: buffer → LLM on-demand. Retries with backoff until success or context cancellation.
func (g *Generator) GenerateQuestion(matchID, category string, difficulty int, excludeRecent []string) (*GenerateResponse, error) {
	return g.GenerateQuestionWithContext(context.Background(), matchID, category, difficulty, excludeRecent)
}

// GenerateQuestionWithContext is like GenerateQuestion but accepts a context.
// It retries buffer and LLM sources until one succeeds, the context is cancelled,
// or the 30s deadline expires.
func (g *Generator) GenerateQuestionWithContext(ctx context.Context, matchID, category string, difficulty int, excludeRecent []string) (*GenerateResponse, error) {
	// No sources at all — fail immediately (nothing to wait for)
	if !g.llmEnabled && g.questionBuffer == nil {
		return nil, fmt.Errorf("no question source available (no LLM or buffer configured)")
	}

	retryDelay := 2 * time.Second
	maxWait := 30 * time.Second
	deadline := time.Now().Add(maxWait)

	for {
		// [1] Try buffer first (instant)
		if g.questionBuffer != nil {
			if resp, err := g.popFromBuffer(matchID, category); err == nil {
				return resp, nil
			}
		}

		// [2] Try LLM on-demand
		if g.llmEnabled && g.llmClient != nil {
			resp, err := g.GenerateQuestionWithLLM(ctx, matchID, category, difficulty)
			if err == nil {
				return resp, nil
			}
			log.Printf("LLM generation attempt failed for %s/%s: %v", matchID, category, err)
		}

		// [3] Check deadline
		if time.Now().Add(retryDelay).After(deadline) {
			return nil, fmt.Errorf("question generation timed out after %s (buffer empty, LLM failed)", maxWait)
		}

		// [4] Wait before retry (respect context cancellation)
		log.Printf("Waiting %s before retry for match %s (category=%s)...", retryDelay, matchID, category)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("question generation cancelled: %w", ctx.Err())
		case <-time.After(retryDelay):
		}
	}
}

// popFromBuffer pops a question from the buffer and prepares it for a specific match.
func (g *Generator) popFromBuffer(matchID, category string) (*GenerateResponse, error) {
	bq, ok := g.questionBuffer.Pop(category)
	if !ok {
		return nil, fmt.Errorf("buffer empty for category %s", category)
	}

	// Generate fresh salt for this match
	salt, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	// Compute answer hash
	answerHash := computeAnswerHash(bq.Answer, salt)

	// Store answer for later reveal
	g.mu.Lock()
	g.matchAnswers[matchID] = &MatchAnswer{
		Question: bq.Question,
		Answer:   bq.Answer,
		Salt:     salt,
		Variants: bq.Variants,
		Format:   bq.Format,
	}
	g.mu.Unlock()

	log.Printf("[buffer] served question for match %s from buffer (category=%s, difficulty=%d)", matchID, category, bq.Difficulty)

	return &GenerateResponse{
		Question:   bq.Question,
		AnswerHash: answerHash,
		FormatHint: bq.Format,
		Category:   bq.Category,
		Difficulty: bq.Difficulty,
	}, nil
}

// GetAnswer retrieves the stored answer for a match.
func (g *Generator) GetAnswer(matchID string) (*MatchAnswer, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	answer, ok := g.matchAnswers[matchID]
	return answer, ok
}

// RevealAnswer returns and deletes the stored answer.
func (g *Generator) RevealAnswer(matchID string) (*RevealResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	answer, ok := g.matchAnswers[matchID]
	if !ok {
		return nil, fmt.Errorf("no answer stored for match %s", matchID)
	}

	delete(g.matchAnswers, matchID)

	return &RevealResponse{
		Answer: answer.Answer,
		Salt:   "0x" + hex.EncodeToString(answer.Salt),
	}, nil
}

// ActiveMatchCount returns the number of active matches.
func (g *Generator) ActiveMatchCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.matchAnswers)
}

// GenerateResponse is the response for question generation.
type GenerateResponse struct {
	Question   string `json:"question"`
	AnswerHash string `json:"answerHash"`
	FormatHint string `json:"formatHint"`
	Category   string `json:"category"`
	Difficulty int    `json:"difficulty"`
}

// RevealResponse is the response for revealing an answer.
type RevealResponse struct {
	Answer string `json:"answer"`
	Salt   string `json:"salt"`
}

// computeAnswerHash computes keccak256(answer + salt).
func computeAnswerHash(answer string, salt []byte) string {
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write([]byte(answer))
	hasher.Write(salt)
	hash := hasher.Sum(nil)
	return "0x" + hex.EncodeToString(hash)
}

// randomInt returns a random int in [0, max).
func randomInt(max int) (int, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	var n uint64
	for i := 0; i < 8; i++ {
		n = n<<8 | uint64(b[i])
	}
	return int(n % uint64(max)), nil
}
