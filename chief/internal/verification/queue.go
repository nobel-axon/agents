// Package verification provides answer verification queue management.
package verification

import (
	"context"
	"errors"
	"log"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/axon-arena/axon-chief/internal/judge"
	"github.com/axon-arena/axon-chief/internal/match"
)

var (
	ErrMatchNotInAnswerPeriod = errors.New("match not in answer period")
	ErrAlreadySettled         = errors.New("match already settled")
	ErrNoCorrectAnswer        = errors.New("no correct answer found")
)

// SubmittedAnswer represents an answer submitted by an agent.
type SubmittedAnswer struct {
	MatchID       *big.Int
	Agent         common.Address
	Answer        string
	Reasoning     string // Optional reasoning for the answer
	BlockNumber   uint64
	TxIndex       uint
	AttemptNumber uint64
	NeuronBurned  *big.Int
	Timestamp     time.Time
}

// VerificationResult represents the result of verifying an answer.
type VerificationResult struct {
	Answer     *SubmittedAnswer
	Correct    bool
	Confidence float64
	Votes      int
	Total      int
}

// PersonalityVerificationResult represents the result from personality-based evaluation.
type PersonalityVerificationResult struct {
	Answer      *SubmittedAnswer
	TotalScore  int                          // Sum of scores from all 3 judges
	Agreement   string                       // unanimous/majority/split
	Evaluations []judge.PersonalityEvaluation // Individual judge evaluations
}

// ConsensusSettlementResult holds the final settlement outcome.
type ConsensusSettlementResult struct {
	Winner       common.Address
	WinnerScore  int
	Agreement    string                                    // How judges agreed
	AllResults   map[string]*PersonalityVerificationResult // All participant results
}

// MatchQueue holds the verification queue for a single match.
type MatchQueue struct {
	mu       sync.Mutex
	matchID  *big.Int
	answers  []*SubmittedAnswer
	settled  bool
	winner   common.Address
}

// Manager manages verification queues for all matches.
type Manager struct {
	mu          sync.RWMutex
	queues      map[string]*MatchQueue
	coordinator *judge.Coordinator
	onSettle    func(ctx context.Context, matchID *big.Int, winner common.Address) error
}

// NewManager creates a new verification manager.
func NewManager(coordinator *judge.Coordinator, onSettle func(ctx context.Context, matchID *big.Int, winner common.Address) error) *Manager {
	return &Manager{
		queues:      make(map[string]*MatchQueue),
		coordinator: coordinator,
		onSettle:    onSettle,
	}
}

// GetOrCreateQueue gets or creates a queue for a match.
func (m *Manager) GetOrCreateQueue(matchID *big.Int) *MatchQueue {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := matchID.String()
	if queue, ok := m.queues[key]; ok {
		return queue
	}

	queue := &MatchQueue{
		matchID: matchID,
		answers: make([]*SubmittedAnswer, 0),
	}
	m.queues[key] = queue
	return queue
}

// RemoveQueue removes a queue for a match.
func (m *Manager) RemoveQueue(matchID *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queues, matchID.String())
}

// EnqueueAnswer adds an answer to the verification queue.
// Answers are automatically sorted by (BlockNumber, TxIndex) for deterministic ordering.
func (m *Manager) EnqueueAnswer(answer *SubmittedAnswer) {
	queue := m.GetOrCreateQueue(answer.MatchID)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if queue.settled {
		log.Printf("Match %s already settled, ignoring answer from %s", answer.MatchID, answer.Agent.Hex())
		return
	}

	// Add to queue
	queue.answers = append(queue.answers, answer)

	// Sort by (BlockNumber, TxIndex) for deterministic ordering
	sort.Slice(queue.answers, func(i, j int) bool {
		if queue.answers[i].BlockNumber != queue.answers[j].BlockNumber {
			return queue.answers[i].BlockNumber < queue.answers[j].BlockNumber
		}
		return queue.answers[i].TxIndex < queue.answers[j].TxIndex
	})

	log.Printf("Enqueued answer from %s for match %s (block %d, tx %d)",
		answer.Agent.Hex(), answer.MatchID, answer.BlockNumber, answer.TxIndex)
}

// ProcessQueue processes the verification queue for a match.
// Verifies answers in order and settles on first correct answer.
func (m *Manager) ProcessQueue(ctx context.Context, matchState *match.State) (*VerificationResult, error) {
	matchID := matchState.ID
	queue := m.GetOrCreateQueue(matchID)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if queue.settled {
		return nil, ErrAlreadySettled
	}

	// Get match question data
	question := matchState.GetQuestion()
	if question == nil {
		return nil, errors.New("no question data for match")
	}

	// Process answers in order
	for _, answer := range queue.answers {
		log.Printf("Verifying answer from %s: %s", answer.Agent.Hex(), answer.Answer)

		// Verify with judge consensus
		result, err := m.coordinator.VerifyWithConsensus(
			ctx,
			matchID.String(),
			answer.Answer,
			question.Text,
			question.FormatHint,
		)
		if err != nil {
			log.Printf("Verification error for %s: %v", answer.Agent.Hex(), err)
			continue
		}

		switch result.Outcome {
		case judge.OutcomeCorrect:
			// Found correct answer - settle the match
			log.Printf("Correct answer from %s! Settling match %s", answer.Agent.Hex(), matchID)

			queue.settled = true
			queue.winner = answer.Agent

			// Call settlement callback
			if m.onSettle != nil {
				if err := m.onSettle(ctx, matchID, answer.Agent); err != nil {
					log.Printf("Failed to settle match %s: %v", matchID, err)
					queue.settled = false
					queue.winner = common.Address{}
					return nil, err
				}
			}

			return &VerificationResult{
				Answer:     answer,
				Correct:    true,
				Confidence: result.Confidence,
				Votes:      result.CorrectVotes,
				Total:      result.Total,
			}, nil

		case judge.OutcomeIncorrect:
			// Answer is incorrect - continue to next
			log.Printf("Incorrect answer from %s (correct: %d, incorrect: %d, unclear: %d, confidence: %.2f)",
				answer.Agent.Hex(), result.CorrectVotes, result.IncorrectVotes, result.UnclearVotes, result.Confidence)

		case judge.OutcomeNoConsensus:
			// No clear consensus - skip this answer and continue
			// This could be a split decision (1-1-1) or too many unclear votes
			log.Printf("No consensus for answer from %s (correct: %d, incorrect: %d, unclear: %d), skipping",
				answer.Agent.Hex(), result.CorrectVotes, result.IncorrectVotes, result.UnclearVotes)
			// Don't return error - just continue to the next answer
		}
	}

	// No correct answer found yet
	return nil, ErrNoCorrectAnswer
}

// IsSettled checks if a match has been settled.
func (m *Manager) IsSettled(matchID *big.Int) bool {
	m.mu.RLock()
	queue, ok := m.queues[matchID.String()]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.settled
}

// GetWinner returns the winner of a settled match.
func (m *Manager) GetWinner(matchID *big.Int) (common.Address, bool) {
	m.mu.RLock()
	queue, ok := m.queues[matchID.String()]
	m.mu.RUnlock()

	if !ok {
		return common.Address{}, false
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if !queue.settled {
		return common.Address{}, false
	}
	return queue.winner, true
}

// GetQueueSize returns the number of answers in the queue for a match.
func (m *Manager) GetQueueSize(matchID *big.Int) int {
	m.mu.RLock()
	queue, ok := m.queues[matchID.String()]
	m.mu.RUnlock()

	if !ok {
		return 0
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.answers)
}

// ClearQueue clears the queue for a match.
func (m *Manager) ClearQueue(matchID *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.queues, matchID.String())
}

// ProcessQueueWithPersonalities processes the queue using personality-based evaluation.
// Each answer is evaluated by 3 judge personalities, and the winner is determined
// by the highest total score across all personalities.
func (m *Manager) ProcessQueueWithPersonalities(ctx context.Context, matchState *match.State) (*ConsensusSettlementResult, error) {
	matchID := matchState.ID
	queue := m.GetOrCreateQueue(matchID)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if queue.settled {
		return nil, ErrAlreadySettled
	}

	// Get match question data
	question := matchState.GetQuestion()
	if question == nil {
		return nil, errors.New("no question data for match")
	}

	// Check if personalities are assigned
	if !matchState.HasPersonalities() {
		return nil, errors.New("no personalities assigned to match")
	}

	// Extract personalities from match state to pass to judge agents
	matchPersonalities := matchState.GetPersonalities()
	judgePersonalities := make([]judge.Personality, len(matchPersonalities))
	for i, p := range matchPersonalities {
		judgePersonalities[i] = judge.Personality{
			ID:          p.ID,
			Name:        p.Name,
			Perspective: p.Perspective,
			Values:      p.Values,
			Style:       p.Style,
		}
	}

	// Get judge panel from match state
	panel := matchState.GetJudgePanel()
	if len(panel) == 0 {
		return nil, errors.New("no judge panel assigned to match")
	}

	// Track all evaluations
	allResults := make(map[string]*PersonalityVerificationResult)
	var highestScore int
	var winner common.Address
	var winnerResult *PersonalityVerificationResult

	// Process all answers
	for _, answer := range queue.answers {
		participant := answer.Agent.Hex()
		log.Printf("Evaluating answer from %s with personalities: %s", participant, answer.Answer)

		// Evaluate with all panel judges in parallel
		panelResult, err := m.coordinator.EvaluateWithPanel(
			ctx,
			panel,
			matchID.String(),
			question.Text,
			answer.Answer,
			answer.Reasoning,
			judgePersonalities,
		)
		if err != nil {
			log.Printf("Evaluation error for %s: %v", participant, err)
			continue
		}

		// Build result
		result := &PersonalityVerificationResult{
			Answer:     answer,
			TotalScore: panelResult.AverageScore,
			Agreement:  panelResult.Agreement,
		}
		// Use first successful judge's evaluations for the flat list
		for _, jr := range panelResult.JudgeResults {
			if jr.Error == nil && jr.Response != nil {
				result.Evaluations = jr.Response.Evaluations
				break
			}
		}

		allResults[participant] = result

		// Build per-judge results for match state
		judgeResults := make([]match.JudgeResult, 0, len(panelResult.JudgeResults))
		for _, jr := range panelResult.JudgeResults {
			if jr.Error != nil || jr.Response == nil {
				continue
			}
			evals := make([]match.PersonalityEvaluation, len(jr.Response.Evaluations))
			for i, e := range jr.Response.Evaluations {
				evals[i] = match.PersonalityEvaluation{
					PersonalityID:   e.PersonalityID,
					PersonalityName: e.PersonalityName,
					Score:           e.Score,
					Convinced:       e.Convinced,
					Concerns:        e.Concerns,
					Verdict:         e.Verdict,
					AgreeLevel:      e.AgreeLevel,
				}
			}
			judgeResults = append(judgeResults, match.JudgeResult{
				JudgeIndex:  jr.JudgeIndex,
				TotalScore:  jr.Response.TotalScore,
				Agreement:   jr.Response.Agreement,
				Evaluations: evals,
			})
		}

		// Store evaluation in match state
		matchEval := &match.ParticipantEvaluation{
			Participant:  participant,
			Answer:       answer.Answer,
			Reasoning:    answer.Reasoning,
			JudgeResults: judgeResults,
			TotalScore:   panelResult.AverageScore,
			Agreement:    panelResult.Agreement,
			SubmittedAt:  answer.Timestamp,
		}
		if len(judgeResults) > 0 {
			matchEval.Evaluations = judgeResults[0].Evaluations
		}
		matchState.AddEvaluation(participant, matchEval)

		log.Printf("Evaluated %s: avgScore=%d (%d judges), agreement=%s",
			participant, panelResult.AverageScore, panelResult.Responded, panelResult.Agreement)

		// Track highest score (tiebreak: first submitter wins)
		if panelResult.AverageScore > highestScore ||
			(panelResult.AverageScore == highestScore && winnerResult != nil && answer.Timestamp.Before(winnerResult.Answer.Timestamp)) {
			highestScore = panelResult.AverageScore
			winner = answer.Agent
			winnerResult = result
		}
	}

	if len(allResults) == 0 {
		return nil, ErrNoCorrectAnswer
	}

	// Settle the match with the highest scoring participant
	if winner != (common.Address{}) {
		log.Printf("Winner determined by personality consensus: %s with score %d", winner.Hex(), highestScore)

		queue.settled = true
		queue.winner = winner

		// Call settlement callback
		if m.onSettle != nil {
			if err := m.onSettle(ctx, matchID, winner); err != nil {
				log.Printf("Failed to settle match %s: %v", matchID, err)
				queue.settled = false
				queue.winner = common.Address{}
				return nil, err
			}
		}
	}

	return &ConsensusSettlementResult{
		Winner:      winner,
		WinnerScore: highestScore,
		Agreement:   winnerResult.Agreement,
		AllResults:  allResults,
	}, nil
}

// EvaluateSingleAnswer evaluates a single answer with personalities and stores the result.
// This can be called as answers come in, before final settlement.
func (m *Manager) EvaluateSingleAnswer(ctx context.Context, matchState *match.State, answer *SubmittedAnswer) (*PersonalityVerificationResult, error) {
	matchID := matchState.ID
	question := matchState.GetQuestion()
	if question == nil {
		return nil, errors.New("no question data for match")
	}

	if !matchState.HasPersonalities() {
		return nil, errors.New("no personalities assigned to match")
	}

	// Extract personalities from match state to pass to judge agents
	matchPersonalities := matchState.GetPersonalities()
	judgePersonalities := make([]judge.Personality, len(matchPersonalities))
	for i, p := range matchPersonalities {
		judgePersonalities[i] = judge.Personality{
			ID:          p.ID,
			Name:        p.Name,
			Perspective: p.Perspective,
			Values:      p.Values,
			Style:       p.Style,
		}
	}

	participant := answer.Agent.Hex()
	log.Printf("Evaluating answer from %s: %s", participant, answer.Answer)

	// Get judge panel from match state
	panel := matchState.GetJudgePanel()
	if len(panel) == 0 {
		return nil, errors.New("no judge panel assigned to match")
	}

	// Evaluate with all panel judges in parallel
	panelResult, err := m.coordinator.EvaluateWithPanel(
		ctx,
		panel,
		matchID.String(),
		question.Text,
		answer.Answer,
		answer.Reasoning,
		judgePersonalities,
	)
	if err != nil {
		return nil, err
	}

	// Build result
	result := &PersonalityVerificationResult{
		Answer:     answer,
		TotalScore: panelResult.AverageScore,
		Agreement:  panelResult.Agreement,
	}
	// Use first successful judge's personality evaluations for the flat list
	for _, jr := range panelResult.JudgeResults {
		if jr.Error == nil && jr.Response != nil {
			result.Evaluations = jr.Response.Evaluations
			break
		}
	}

	// Build per-judge results for match state
	judgeResults := make([]match.JudgeResult, 0, len(panelResult.JudgeResults))
	for _, jr := range panelResult.JudgeResults {
		if jr.Error != nil || jr.Response == nil {
			continue
		}
		evals := make([]match.PersonalityEvaluation, len(jr.Response.Evaluations))
		for i, e := range jr.Response.Evaluations {
			evals[i] = match.PersonalityEvaluation{
				PersonalityID:   e.PersonalityID,
				PersonalityName: e.PersonalityName,
				Score:           e.Score,
				Convinced:       e.Convinced,
				Concerns:        e.Concerns,
				Verdict:         e.Verdict,
				AgreeLevel:      e.AgreeLevel,
			}
		}
		judgeResults = append(judgeResults, match.JudgeResult{
			JudgeIndex:  jr.JudgeIndex,
			TotalScore:  jr.Response.TotalScore,
			Agreement:   jr.Response.Agreement,
			Evaluations: evals,
		})
	}

	// Store in match state
	matchEval := &match.ParticipantEvaluation{
		Participant:  participant,
		Answer:       answer.Answer,
		Reasoning:    answer.Reasoning,
		JudgeResults: judgeResults,
		TotalScore:   panelResult.AverageScore,
		Agreement:    panelResult.Agreement,
		SubmittedAt:  answer.Timestamp,
	}
	// Flatten first judge's evaluations for backward compat
	if len(judgeResults) > 0 {
		matchEval.Evaluations = judgeResults[0].Evaluations
	}
	matchState.AddEvaluation(participant, matchEval)

	log.Printf("Evaluated %s: avgScore=%d (%d judges), agreement=%s",
		participant, panelResult.AverageScore, panelResult.Responded, panelResult.Agreement)

	return result, nil
}

// GetFirstSubmitter returns the first answer in the queue (by block/tx order).
// Used as graceful degradation when no personality evaluations are available.
func (m *Manager) GetFirstSubmitter(matchID *big.Int) (*SubmittedAnswer, bool) {
	m.mu.RLock()
	queue, ok := m.queues[matchID.String()]
	m.mu.RUnlock()

	if !ok {
		return nil, false
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.answers) == 0 {
		return nil, false
	}
	return queue.answers[0], true
}

// DetermineWinnerByScore looks at all evaluations in the match state and returns the winner.
func (m *Manager) DetermineWinnerByScore(matchState *match.State) (common.Address, int, error) {
	evals := matchState.GetAllEvaluations()
	if len(evals) == 0 {
		return common.Address{}, 0, ErrNoCorrectAnswer
	}

	var highestScore int
	var winner string
	var winnerEval *match.ParticipantEvaluation

	for participant, eval := range evals {
		if eval.TotalScore > highestScore ||
			(eval.TotalScore == highestScore && winnerEval != nil && eval.SubmittedAt.Before(winnerEval.SubmittedAt)) {
			highestScore = eval.TotalScore
			winner = participant
			winnerEval = eval
		}
	}

	return common.HexToAddress(winner), highestScore, nil
}
