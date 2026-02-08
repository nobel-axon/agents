// Package server provides HTTP handlers for the axon-chief API.
package server

import (
	"context"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/axon-arena/axon-chief/internal/chain"
	"github.com/axon-arena/axon-chief/internal/judge"
	"github.com/axon-arena/axon-chief/internal/match"
	"github.com/axon-arena/axon-chief/internal/verification"
	"github.com/axon-arena/axon-chief/internal/watcher"
)

// RecoveryService handles state recovery on startup.
type RecoveryService struct {
	chainClient       *chain.ChainClient
	matchManager      *match.Manager
	timeoutWatcher    *watcher.TimeoutWatcher
	verificationMgr   *verification.Manager
	judgeCoordinator  *judge.Coordinator
	gapDuration       time.Duration
	onQuestionGenerate func(ctx context.Context, state *match.State) error
}

// RecoveryResult holds statistics about the recovery process.
type RecoveryResult struct {
	TotalMatches     int
	QueueMatches     int
	ActiveMatches    int
	AnswerPeriod     int
	TimersRegistered int
	StuckRecovered   int
}

// NewRecoveryService creates a new recovery service.
func NewRecoveryService(
	chainClient *chain.ChainClient,
	matchManager *match.Manager,
	timeoutWatcher *watcher.TimeoutWatcher,
	verificationMgr *verification.Manager,
	judgeCoordinator *judge.Coordinator,
	gapDuration time.Duration,
	onQuestionGenerate func(ctx context.Context, state *match.State) error,
) *RecoveryService {
	return &RecoveryService{
		chainClient:        chainClient,
		matchManager:       matchManager,
		timeoutWatcher:     timeoutWatcher,
		verificationMgr:    verificationMgr,
		judgeCoordinator:   judgeCoordinator,
		gapDuration:        gapDuration,
		onQuestionGenerate: onQuestionGenerate,
	}
}

// chainPhaseToInternal maps on-chain phase values to internal phases.
// Contract: Queue=0, QuestionRevealed=1, AnswerPeriod=2, Settled=3, Refunded=4
// Internal: None=0, Queue=1, Active=2, AnswerPeriod=3, Settled=4, Refunded=5, Cancelled=6
func chainPhaseToInternal(chainPhase uint8) match.Phase {
	switch chainPhase {
	case 0:
		return match.PhaseQueue
	case 1:
		return match.PhaseActive // QuestionRevealed -> Active
	case 2:
		return match.PhaseAnswerPeriod
	case 3:
		return match.PhaseSettled
	case 4:
		return match.PhaseRefunded
	default:
		return match.PhaseNone
	}
}

// RecoverAllState recovers all active matches from the chain on startup.
func (r *RecoveryService) RecoverAllState(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{}

	// Get the next match ID to determine range
	nextID, err := r.chainClient.GetNextMatchID(ctx)
	if err != nil {
		return nil, err
	}

	startMatch := int64(1)
	if v := os.Getenv("RECOVERY_START_MATCH"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			startMatch = n
		}
	}

	log.Printf("Recovery: scanning matches %d to %s", startMatch, nextID)

	// Iterate through all matches
	for i := startMatch; i < nextID.Int64(); i++ {
		matchID := big.NewInt(i)

		// Get chain state
		chainState, err := r.chainClient.GetMatchState(ctx, matchID)
		if err != nil {
			log.Printf("Recovery: failed to get state for match %d: %v", i, err)
			continue
		}

		phase := chainPhaseToInternal(chainState.Phase)

		// Skip terminal states
		if match.IsTerminalPhase(phase) {
			continue
		}

		result.TotalMatches++

		// Get players
		players, err := r.chainClient.GetMatchPlayers(ctx, matchID)
		if err != nil {
			log.Printf("Recovery: failed to get players for match %d: %v", i, err)
			players = nil
		}

		// Create or update match state
		state, err := r.matchManager.GetMatch(matchID)
		if err != nil {
			// Create new state
			state, err = r.matchManager.CreateMatch(matchID, time.Time{})
			if err != nil {
				log.Printf("Recovery: failed to create state for match %d: %v", i, err)
				continue
			}
		}

		// Sync from chain data
		state.SyncFromChain(
			chainState.Pool,
			phase,
			chainState.AnswerDeadline,
			chainState.Winner,
			players,
		)

		// For queue matches, also get the queue deadline from chain config
		if phase == match.PhaseQueue {
			if cfg, err := r.chainClient.GetMatchConfig(ctx, matchID); err == nil && cfg.QueueDeadline > 0 {
				state.SetQueueDeadline(time.Unix(int64(cfg.QueueDeadline), 0))
			}
		}

		// Recover based on current phase
		switch phase {
		case match.PhaseQueue:
			result.QueueMatches++
			log.Printf("Recovery: match %d is in queue phase (deadline: %s)", i, state.QueueDeadline.Format(time.RFC3339))

		case match.PhaseActive:
			result.ActiveMatches++
			if err := r.recoverActivePhase(ctx, matchID, state); err != nil {
				log.Printf("Recovery: failed to recover active match %d: %v", i, err)
			} else {
				result.StuckRecovered++
			}

		case match.PhaseAnswerPeriod:
			result.AnswerPeriod++
			if err := r.recoverAnswerPeriod(ctx, matchID, state, chainState.AnswerDeadline); err != nil {
				log.Printf("Recovery: failed to recover answer period for match %d: %v", i, err)
			} else {
				result.TimersRegistered++
			}
		}
	}

	log.Printf("Recovery complete: %d active matches (%d queue, %d active, %d answer period), %d timers registered, %d stuck recovered",
		result.TotalMatches, result.QueueMatches, result.ActiveMatches, result.AnswerPeriod,
		result.TimersRegistered, result.StuckRecovered)

	return result, nil
}

// recoverAnswerPeriod recovers a match in the answer period phase.
func (r *RecoveryService) recoverAnswerPeriod(ctx context.Context, matchID *big.Int, state *match.State, answerDeadline uint64) error {
	if answerDeadline == 0 {
		log.Printf("Recovery: match %s has no answer deadline set", matchID)
		return nil
	}

	// Recover question data if missing
	if state.GetQuestion() == nil {
		question, err := r.chainClient.GetMatchQuestion(ctx, matchID)
		if err != nil {
			log.Printf("Recovery: failed to get question for match %s: %v", matchID, err)
		} else if question.QuestionText != "" {
			chainState, err := r.chainClient.GetMatchState(ctx, matchID)
			if err == nil {
				qData := &match.QuestionData{
					Text:       question.QuestionText,
					Category:   question.Category,
					Difficulty: chainState.Difficulty,
					FormatHint: question.FormatHint,
					AnswerHash: chainState.AnswerHash,
				}
				state.SetQuestion(qData)
				log.Printf("Recovery: match %s question data restored: %q", matchID, question.QuestionText)
			}
		}
	}

	// Generate personalities if missing
	if len(state.Personalities) == 0 && r.judgeCoordinator != nil {
		question, _ := r.chainClient.GetMatchQuestion(ctx, matchID)
		category := "general"
		if question.Category != "" {
			category = question.Category
		}
		resp, err := r.judgeCoordinator.GeneratePersonalities(ctx, matchID.String(), category)
		if err != nil {
			log.Printf("Recovery: failed to generate personalities for match %s: %v", matchID, err)
		} else {
			personalities := make([]match.Personality, len(resp.Personalities))
			for i, p := range resp.Personalities {
				personalities[i] = match.Personality{
					ID:          p.ID,
					Name:        p.Name,
					Perspective: p.Perspective,
					Values:      p.Values,
					Style:       p.Style,
				}
			}
			state.SetPersonalities(personalities)
			log.Printf("Recovery: match %s generated %d judge personalities", matchID, len(personalities))
		}
	}

	deadline := time.Unix(int64(answerDeadline), 0)
	state.SetAnswerDeadline(deadline)

	remaining := time.Until(deadline)
	if remaining <= 0 {
		// Deadline already passed - timeout should be triggered
		log.Printf("Recovery: match %s deadline already passed (%v ago), will trigger timeout", matchID, -remaining)
	} else {
		log.Printf("Recovery: match %s has %v remaining until deadline", matchID, remaining)
	}

	// Register with timeout watcher (it handles already-expired deadlines)
	r.timeoutWatcher.RegisterMatch(matchID, deadline)

	return nil
}

// recoverActivePhase recovers a match stuck in the active phase.
// This typically means question generation was interrupted.
func (r *RecoveryService) recoverActivePhase(ctx context.Context, matchID *big.Int, state *match.State) error {
	// Check if question exists on chain
	question, err := r.chainClient.GetMatchQuestion(ctx, matchID)
	if err != nil {
		log.Printf("Recovery: failed to get question for match %s: %v", matchID, err)
	}

	// If question exists (non-empty text), the question was posted but answer period not started
	if question.QuestionText != "" {
		log.Printf("Recovery: match %s has question on chain, starting answer period", matchID)

		// Recover question data into match state
		chainState2, err := r.chainClient.GetMatchState(ctx, matchID)
		if err != nil {
			log.Printf("Recovery: failed to get state for question data: %v", err)
		} else {
			qData := &match.QuestionData{
				Text:       question.QuestionText,
				Category:   question.Category,
				Difficulty: chainState2.Difficulty,
				FormatHint: question.FormatHint,
				AnswerHash: chainState2.AnswerHash,
			}
			state.SetQuestion(qData)
			log.Printf("Recovery: match %s question data restored: %q", matchID, question.QuestionText)
		}

		// Generate personalities if missing
		if len(state.Personalities) == 0 && r.judgeCoordinator != nil {
			log.Printf("Recovery: match %s generating judge personalities", matchID)
			resp, err := r.judgeCoordinator.GeneratePersonalities(ctx, matchID.String(), question.Category)
			if err != nil {
				log.Printf("Recovery: failed to generate personalities for match %s: %v", matchID, err)
			} else {
				personalities := make([]match.Personality, len(resp.Personalities))
				for i, p := range resp.Personalities {
					personalities[i] = match.Personality{
						ID:          p.ID,
						Name:        p.Name,
						Perspective: p.Perspective,
						Values:      p.Values,
						Style:       p.Style,
					}
				}
				state.SetPersonalities(personalities)
				log.Printf("Recovery: match %s generated %d judge personalities", matchID, len(personalities))
			}
		}

		// Start answer period if not already started
		if chainState2.Phase == 1 { // QuestionRevealed, not yet AnswerPeriod
			if err := r.chainClient.StartAnswerPeriod(ctx, matchID); err != nil {
				return err
			}
		}

		// Get updated state
		chainState, err := r.chainClient.GetMatchState(ctx, matchID)
		if err != nil {
			return err
		}

		deadline := time.Unix(int64(chainState.AnswerDeadline), 0)
		state.SetAnswerDeadline(deadline)
		state.SetPhase(match.PhaseAnswerPeriod)
		r.timeoutWatcher.RegisterMatch(matchID, deadline)

		return nil
	}

	// No question on chain - question generation was interrupted
	// Restart question generation
	log.Printf("[DEBUG Recovery] match %s stuck in active phase without question — calling onQuestionGenerate (same path as normal flow)", matchID)

	if r.onQuestionGenerate != nil {
		go func() {
			if err := r.onQuestionGenerate(context.Background(), state); err != nil {
				log.Printf("Recovery: failed to regenerate question for match %s: %v", matchID, err)
			}
		}()
	}

	return nil
}
