// Package judge provides judge agent coordination for axon-chief.
package judge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/axon-arena/axon-chief/internal/config"
)

var (
	ErrNoHealthyJudges     = errors.New("no healthy judges available")
	ErrConsensusNotReached = errors.New("consensus not reached")
	ErrPhilosopherDown     = errors.New("philosopher agent is down")
	ErrDirectorDown        = errors.New("director agent is down")
)

// JudgeStatus represents the health status of an agent.
type JudgeStatus struct {
	Endpoint  string
	Role      string
	Healthy   bool
	Provider  string
	LastCheck time.Time
	Error     error
}

// Coordinator manages typed agent clients and handles consensus.
type Coordinator struct {
	mu sync.RWMutex

	philosopher       PhilosopherClient
	director          DirectorClient
	judges            []JudgeClient

	philosopherStatus JudgeStatus
	directorStatus    JudgeStatus
	judgeStatuses     []JudgeStatus

	cfg *config.JudgeConfig
	rng *rand.Rand
}

// NewCoordinator creates a new coordinator with role-specific clients.
func NewCoordinator(cfg *config.JudgeConfig) *Coordinator {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond

	// Create philosopher client
	philClient := NewClient(cfg.PhilosopherEndpoint, timeout)

	// Create director client
	dirClient := NewClient(cfg.DirectorEndpoint, timeout)

	// Create judge clients
	judgeClients := make([]JudgeClient, len(cfg.JudgeEndpoints))
	judgeStatuses := make([]JudgeStatus, len(cfg.JudgeEndpoints))
	for i, endpoint := range cfg.JudgeEndpoints {
		judgeClients[i] = NewClient(endpoint, timeout)
		judgeStatuses[i] = JudgeStatus{
			Endpoint: endpoint,
			Role:     "judge",
			Healthy:  true,
		}
	}

	return &Coordinator{
		philosopher: philClient,
		director:    dirClient,
		judges:      judgeClients,
		philosopherStatus: JudgeStatus{
			Endpoint: cfg.PhilosopherEndpoint,
			Role:     "philosopher",
			Healthy:  true,
		},
		directorStatus: JudgeStatus{
			Endpoint: cfg.DirectorEndpoint,
			Role:     "director",
			Healthy:  true,
		},
		judgeStatuses: judgeStatuses,
		cfg:           cfg,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// CheckHealth performs health checks on all agents.
func (c *Coordinator) CheckHealth(ctx context.Context) []JudgeStatus {
	c.mu.Lock()
	defer c.mu.Unlock()

	var wg sync.WaitGroup

	// Check philosopher
	wg.Add(1)
	go func() {
		defer wg.Done()
		health, err := c.philosopher.Health(ctx)
		c.philosopherStatus.LastCheck = time.Now()
		if err != nil {
			c.philosopherStatus.Healthy = false
			c.philosopherStatus.Error = err
			log.Printf("Philosopher (%s) unhealthy: %v", c.philosopherStatus.Endpoint, err)
		} else {
			c.philosopherStatus.Healthy = true
			c.philosopherStatus.Provider = health.Provider
			c.philosopherStatus.Error = nil
		}
	}()

	// Check director
	wg.Add(1)
	go func() {
		defer wg.Done()
		health, err := c.director.Health(ctx)
		c.directorStatus.LastCheck = time.Now()
		if err != nil {
			c.directorStatus.Healthy = false
			c.directorStatus.Error = err
			log.Printf("Director (%s) unhealthy: %v", c.directorStatus.Endpoint, err)
		} else {
			c.directorStatus.Healthy = true
			c.directorStatus.Provider = health.Provider
			c.directorStatus.Error = nil
		}
	}()

	// Check judges
	for i := range c.judges {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			health, err := c.judges[idx].Health(ctx)
			c.judgeStatuses[idx].LastCheck = time.Now()
			if err != nil {
				c.judgeStatuses[idx].Healthy = false
				c.judgeStatuses[idx].Error = err
				log.Printf("Judge %d (%s) unhealthy: %v", idx, c.judgeStatuses[idx].Endpoint, err)
			} else {
				c.judgeStatuses[idx].Healthy = true
				c.judgeStatuses[idx].Provider = health.Provider
				c.judgeStatuses[idx].Error = nil
			}
		}(i)
	}

	wg.Wait()

	return c.collectStatuses()
}

// GetStatus returns the current status of all agents.
func (c *Coordinator) GetStatus() []JudgeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.collectStatuses()
}

// collectStatuses assembles all statuses into a single slice (must hold lock).
func (c *Coordinator) collectStatuses() []JudgeStatus {
	result := make([]JudgeStatus, 0, 2+len(c.judgeStatuses))
	result = append(result, c.philosopherStatus)
	result = append(result, c.directorStatus)
	result = append(result, c.judgeStatuses...)
	return result
}

// PickGenerator returns the philosopher client directly.
func (c *Coordinator) PickGenerator(ctx context.Context) (PhilosopherClient, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.philosopherStatus.Healthy {
		return nil, ErrPhilosopherDown
	}
	return c.philosopher, nil
}

// healthyJudges returns indices of healthy judge clients (must hold RLock).
func (c *Coordinator) healthyJudges() []int {
	var healthy []int
	for i, s := range c.judgeStatuses {
		if s.Healthy {
			healthy = append(healthy, i)
		}
	}
	return healthy
}

// VerificationResult holds the result of a verification from a single judge.
type VerificationResult struct {
	JudgeIndex int
	Response   *VerifyAnswerResponse
	Error      error
}

// ConsensusOutcome represents the outcome of consensus verification.
type ConsensusOutcome int

const (
	OutcomeCorrect ConsensusOutcome = iota
	OutcomeIncorrect
	OutcomeNoConsensus
)

func (o ConsensusOutcome) String() string {
	switch o {
	case OutcomeCorrect:
		return "correct"
	case OutcomeIncorrect:
		return "incorrect"
	case OutcomeNoConsensus:
		return "no_consensus"
	default:
		return "unknown"
	}
}

// ConsensusResult holds the result of consensus verification.
type ConsensusResult struct {
	Outcome        ConsensusOutcome
	Correct        bool
	CorrectVotes   int
	IncorrectVotes int
	UnclearVotes   int
	Votes          int
	Total          int
	Confidence     float64
	Results        []VerificationResult
}

const lowConfidenceThreshold = 0.5

// VerifyWithConsensus verifies an answer with judge agents and requires consensus.
func (c *Coordinator) VerifyWithConsensus(ctx context.Context, matchID, submitted, question, formatHint string) (*ConsensusResult, error) {
	c.mu.RLock()
	healthy := c.healthyJudges()
	judges := make([]JudgeClient, len(healthy))
	for i, idx := range healthy {
		judges[i] = c.judges[idx]
	}
	c.mu.RUnlock()

	if len(judges) == 0 {
		return nil, ErrNoHealthyJudges
	}

	var wg sync.WaitGroup
	results := make([]VerificationResult, len(judges))

	for i, judge := range judges {
		wg.Add(1)
		go func(resultIdx int, j JudgeClient, jIdx int) {
			defer wg.Done()
			resp, err := j.VerifyAnswer(ctx, matchID, submitted, question, formatHint)
			results[resultIdx] = VerificationResult{
				JudgeIndex: jIdx,
				Response:   resp,
				Error:      err,
			}
		}(i, judge, healthy[i])
	}
	wg.Wait()

	var correctVotes, incorrectVotes, unclearVotes int
	var totalConfidence float64
	var successfulResults []VerificationResult

	for _, r := range results {
		if r.Error != nil {
			log.Printf("Judge %d verification error: %v", r.JudgeIndex, r.Error)
			unclearVotes++
			continue
		}
		successfulResults = append(successfulResults, r)
		totalConfidence += r.Response.Confidence

		if r.Response.Confidence < lowConfidenceThreshold {
			unclearVotes++
			continue
		}

		if r.Response.Correct {
			correctVotes++
		} else {
			incorrectVotes++
		}
	}

	if len(successfulResults) == 0 {
		return nil, errors.New("no successful verifications")
	}

	avgConfidence := totalConfidence / float64(len(successfulResults))
	threshold := c.cfg.ConsensusThreshold
	if threshold == 0 {
		threshold = 2
	}

	var outcome ConsensusOutcome
	if correctVotes >= threshold {
		outcome = OutcomeCorrect
	} else if incorrectVotes >= threshold {
		outcome = OutcomeIncorrect
	} else {
		outcome = OutcomeNoConsensus
		log.Printf("No consensus reached: %d correct, %d incorrect, %d unclear out of %d total",
			correctVotes, incorrectVotes, unclearVotes, len(results))
	}

	return &ConsensusResult{
		Outcome:        outcome,
		Correct:        outcome == OutcomeCorrect,
		CorrectVotes:   correctVotes,
		IncorrectVotes: incorrectVotes,
		UnclearVotes:   unclearVotes,
		Votes:          correctVotes,
		Total:          len(successfulResults),
		Confidence:     avgConfidence,
		Results:        successfulResults,
	}, nil
}

// RequestReveal requests the answer reveal from the philosopher.
func (c *Coordinator) RequestReveal(ctx context.Context, matchID string) (*RevealResponse, error) {
	c.mu.RLock()
	if !c.philosopherStatus.Healthy {
		c.mu.RUnlock()
		return nil, ErrPhilosopherDown
	}
	phil := c.philosopher
	c.mu.RUnlock()

	return phil.Reveal(ctx, matchID)
}

// RequestCommentary requests commentary from a healthy judge.
func (c *Coordinator) RequestCommentary(ctx context.Context, req *CommentaryRequest) (*CommentaryResponse, error) {
	c.mu.RLock()
	healthy := c.healthyJudges()
	if len(healthy) == 0 {
		c.mu.RUnlock()
		return nil, ErrNoHealthyJudges
	}
	judge := c.judges[healthy[0]]
	c.mu.RUnlock()

	return judge.Commentary(ctx, req)
}

// QuestionValidationResult holds the result of validating a question with a single judge.
type QuestionValidationResult struct {
	JudgeIndex int
	Response   *ValidateQuestionResponse
	Error      error
}

// QuestionConsensusResult holds the result of question validation consensus.
type QuestionConsensusResult struct {
	Valid       bool
	ValidVotes  int
	Total       int
	Issues      []string
	Validations []QuestionValidationResult
}

// ValidateQuestionWithConsensus validates a generated question with judge agents.
func (c *Coordinator) ValidateQuestionWithConsensus(
	ctx context.Context,
	matchID, question, formatHint, category string,
	difficulty int,
) (*QuestionConsensusResult, error) {
	c.mu.RLock()
	healthy := c.healthyJudges()
	judges := make([]JudgeClient, len(healthy))
	for i, idx := range healthy {
		judges[i] = c.judges[idx]
	}
	c.mu.RUnlock()

	if len(judges) == 0 {
		return nil, errors.New("no healthy judge agents available")
	}

	var wg sync.WaitGroup
	results := make([]QuestionValidationResult, len(judges))

	req := &ValidateQuestionRequest{
		MatchID:    matchID,
		Question:   question,
		FormatHint: formatHint,
		Category:   category,
		Difficulty: difficulty,
	}

	for i, judge := range judges {
		wg.Add(1)
		go func(resultIdx int, j JudgeClient, jIdx int) {
			defer wg.Done()
			resp, err := j.ValidateQuestion(ctx, req)
			results[resultIdx] = QuestionValidationResult{
				JudgeIndex: jIdx,
				Response:   resp,
				Error:      err,
			}
		}(i, judge, healthy[i])
	}
	wg.Wait()

	var validVotes, invalidVotes int
	var allIssues []string
	var successfulResults []QuestionValidationResult

	for _, r := range results {
		if r.Error != nil {
			log.Printf("Judge %d validation error: %v", r.JudgeIndex, r.Error)
			continue
		}
		successfulResults = append(successfulResults, r)

		if r.Response.Valid {
			validVotes++
		} else {
			invalidVotes++
			allIssues = append(allIssues, r.Response.Issues...)
		}
	}

	if len(successfulResults) == 0 {
		return nil, errors.New("no successful validations")
	}

	threshold := 2
	if len(successfulResults) < 2 {
		threshold = len(successfulResults)
	}

	result := &QuestionConsensusResult{
		Valid:       validVotes >= threshold,
		ValidVotes:  validVotes,
		Total:       len(successfulResults),
		Issues:      allIssues,
		Validations: successfulResults,
	}

	if !result.Valid {
		log.Printf("Question validation failed: %d valid, %d invalid out of %d. Issues: %v",
			validVotes, invalidVotes, len(successfulResults), allIssues)
	}

	return result, nil
}

// GeneratePersonalities generates 3 unique judge personalities for a match via the Director.
func (c *Coordinator) GeneratePersonalities(ctx context.Context, matchID, category string) (*GeneratePersonalitiesResponse, error) {
	c.mu.RLock()
	if !c.directorStatus.Healthy {
		c.mu.RUnlock()
		return nil, ErrDirectorDown
	}
	dir := c.director
	c.mu.RUnlock()

	return dir.GeneratePersonalities(ctx, matchID, category)
}

// EvaluateAnswerWithPersonalities requests personality-based answer evaluation from a healthy judge.
// Deprecated: Use EvaluateWithPanel for all-judge panel evaluation.
func (c *Coordinator) EvaluateAnswerWithPersonalities(ctx context.Context, matchID, question, answer, reasoning string, personalities []Personality) (*EvaluateAnswerResponse, error) {
	c.mu.RLock()
	healthy := c.healthyJudges()
	if len(healthy) == 0 {
		c.mu.RUnlock()
		return nil, ErrNoHealthyJudges
	}
	// Pick a random healthy judge for load distribution
	idx := healthy[c.rng.Intn(len(healthy))]
	judge := c.judges[idx]
	c.mu.RUnlock()

	return judge.EvaluateAnswer(ctx, matchID, question, answer, reasoning, personalities)
}

// PanelJudgeResult holds one judge's evaluation result.
type PanelJudgeResult struct {
	JudgeIndex int
	Response   *EvaluateAnswerResponse
	Error      error
}

// PanelEvaluationResult holds aggregated results from all panel judges.
type PanelEvaluationResult struct {
	AverageScore int
	Agreement    string
	JudgeResults []PanelJudgeResult
	Responded    int
}

// AssignJudgePanel returns indices of all currently healthy judges.
// Called once per match at personality generation time.
func (c *Coordinator) AssignJudgePanel() []int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	healthy := c.healthyJudges()
	result := make([]int, len(healthy))
	copy(result, healthy)
	return result
}

// EvaluateWithPanel sends answer to all panel judges in parallel, averages scores.
// Requires ≥2 successful responses (same threshold as VerifyWithConsensus).
func (c *Coordinator) EvaluateWithPanel(
	ctx context.Context,
	panel []int,
	matchID, question, answer, reasoning string,
	personalities []Personality,
) (*PanelEvaluationResult, error) {
	c.mu.RLock()
	judges := make([]JudgeClient, len(panel))
	for i, idx := range panel {
		if idx >= len(c.judges) {
			c.mu.RUnlock()
			return nil, fmt.Errorf("invalid judge index %d", idx)
		}
		judges[i] = c.judges[idx]
	}
	c.mu.RUnlock()

	if len(judges) == 0 {
		return nil, ErrNoHealthyJudges
	}

	var wg sync.WaitGroup
	results := make([]PanelJudgeResult, len(judges))

	for i, j := range judges {
		wg.Add(1)
		go func(resultIdx int, judge JudgeClient, jIdx int) {
			defer wg.Done()
			resp, err := judge.EvaluateAnswer(ctx, matchID, question, answer, reasoning, personalities)
			results[resultIdx] = PanelJudgeResult{
				JudgeIndex: jIdx,
				Response:   resp,
				Error:      err,
			}
		}(i, j, panel[i])
	}
	wg.Wait()

	// Collect successful responses
	var successful []PanelJudgeResult
	for _, r := range results {
		if r.Error != nil {
			log.Printf("Judge %d evaluation error: %v", r.JudgeIndex, r.Error)
			continue
		}
		successful = append(successful, r)
	}

	threshold := c.cfg.ConsensusThreshold
	if threshold == 0 {
		threshold = 2
	}
	if len(successful) < threshold {
		return nil, fmt.Errorf("insufficient judge responses: got %d, need %d", len(successful), threshold)
	}

	// Average scores
	totalScore := 0
	for _, r := range successful {
		totalScore += r.Response.TotalScore
	}
	avgScore := totalScore / len(successful)

	// Derive agreement from majority
	agreementCounts := make(map[string]int)
	for _, r := range successful {
		agreementCounts[r.Response.Agreement]++
	}
	bestAgreement := ""
	bestCount := 0
	for a, count := range agreementCounts {
		if count > bestCount {
			bestAgreement = a
			bestCount = count
		}
	}

	return &PanelEvaluationResult{
		AverageScore: avgScore,
		Agreement:    bestAgreement,
		JudgeResults: results,
		Responded:    len(successful),
	}, nil
}
