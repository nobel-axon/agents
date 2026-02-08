// Package server provides HTTP handlers for the axon-chief API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
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
	chainClient        *chain.ChainClient
	matchManager       *match.Manager
	timeoutWatcher     *watcher.TimeoutWatcher
	verificationMgr    *verification.Manager
	judgeCoordinator   *judge.Coordinator
	gapDuration        time.Duration
	onQuestionGenerate func(ctx context.Context, state *match.State) error
	serverURL          string
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
	serverURL string,
) *RecoveryService {
	return &RecoveryService{
		chainClient:        chainClient,
		matchManager:       matchManager,
		timeoutWatcher:     timeoutWatcher,
		verificationMgr:    verificationMgr,
		judgeCoordinator:   judgeCoordinator,
		gapDuration:        gapDuration,
		onQuestionGenerate: onQuestionGenerate,
		serverURL:          serverURL,
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

// backendMatchResponse mirrors the JSON from GET /api/matches/open and /api/matches/live.
type backendMatchResponse struct {
	MatchID int64  `json:"matchId"`
	Phase   string `json:"phase"`
}

type backendMatchesEnvelope struct {
	Matches []backendMatchResponse `json:"matches"`
}

// fetchActiveMatchIDsFromBackend calls the backend's /api/matches/open and /api/matches/live
// endpoints and returns the deduplicated set of match IDs that need recovery.
func (r *RecoveryService) fetchActiveMatchIDsFromBackend(ctx context.Context) ([]int64, error) {
	if r.serverURL == "" {
		return nil, fmt.Errorf("no server URL configured")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	seen := make(map[int64]bool)
	var ids []int64

	for _, endpoint := range []string{"/api/matches/open", "/api/matches/live"} {
		matches, err := func() ([]backendMatchResponse, error) {
			url := r.serverURL + endpoint
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, fmt.Errorf("create request for %s: %w", endpoint, err)
			}

			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("fetch %s: %w", endpoint, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("%s returned status %d", endpoint, resp.StatusCode)
			}

			var envelope backendMatchesEnvelope
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				return nil, fmt.Errorf("decode %s response: %w", endpoint, err)
			}
			return envelope.Matches, nil
		}()
		if err != nil {
			return nil, err
		}

		for _, m := range matches {
			if !seen[m.MatchID] {
				seen[m.MatchID] = true
				ids = append(ids, m.MatchID)
			}
		}
	}

	return ids, nil
}

// RecoverAllState recovers all active matches on startup.
// It first tries to ask the backend for active matches (fast path: typically 0-2 matches).
// If the backend is unreachable, it falls back to scanning the last RECOVERY_LOOKBACK
// matches on-chain with rate limiting.
func (r *RecoveryService) RecoverAllState(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{}

	// Fast path: ask the backend for active match IDs
	activeIDs, err := r.fetchActiveMatchIDsFromBackend(ctx)
	if err == nil {
		log.Printf("Recovery: fetched %d active matches from backend", len(activeIDs))
		for _, id := range activeIDs {
			matchID := big.NewInt(id)
			if err := r.recoverMatch(ctx, matchID, result); err != nil {
				log.Printf("Recovery: failed to recover match %d: %v", id, err)
			}
		}
		r.logResult(result)
		return result, nil
	}

	log.Printf("Recovery: backend unreachable (%v), falling back to chain scan", err)

	// Fallback: rate-limited chain scan of recent matches
	return r.recoverFromChainScan(ctx)
}

// recoverFromChainScan scans the last RECOVERY_LOOKBACK matches on-chain with rate limiting.
func (r *RecoveryService) recoverFromChainScan(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{}

	nextID, err := r.chainClient.GetNextMatchID(ctx)
	if err != nil {
		return nil, err
	}

	lookback := int64(20)
	if v := os.Getenv("RECOVERY_LOOKBACK"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			lookback = n
		}
	}

	// Also support the legacy RECOVERY_START_MATCH for explicit override
	startMatch := nextID.Int64() - lookback
	if startMatch < 1 {
		startMatch = 1
	}
	if v := os.Getenv("RECOVERY_START_MATCH"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			startMatch = n
		}
	}

	log.Printf("Recovery: chain scan fallback — scanning matches %d to %d (lookback=%d, rate=10/sec)",
		startMatch, nextID.Int64()-1, lookback)

	// Rate limiter: 10 requests/sec (100ms between ticks)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for i := startMatch; i < nextID.Int64(); i++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
		}

		matchID := big.NewInt(i)

		chainState, err := r.chainClient.GetMatchState(ctx, matchID)
		if err != nil {
			log.Printf("Recovery: failed to get state for match %d: %v", i, err)
			continue
		}

		phase := chainPhaseToInternal(chainState.Phase)
		if match.IsTerminalPhase(phase) {
			continue
		}

		// Non-terminal match found — recover it (additional RPC calls also rate-limited
		// by the ticker on next iteration)
		if err := r.recoverMatch(ctx, matchID, result); err != nil {
			log.Printf("Recovery: failed to recover match %d: %v", i, err)
		}
	}

	r.logResult(result)
	return result, nil
}

// recoverMatch recovers a single match by its ID from chain state.
func (r *RecoveryService) recoverMatch(ctx context.Context, matchID *big.Int, result *RecoveryResult) error {
	chainState, err := r.chainClient.GetMatchState(ctx, matchID)
	if err != nil {
		return fmt.Errorf("get match state: %w", err)
	}

	phase := chainPhaseToInternal(chainState.Phase)
	if match.IsTerminalPhase(phase) {
		return nil
	}

	result.TotalMatches++

	players, err := r.chainClient.GetMatchPlayers(ctx, matchID)
	if err != nil {
		log.Printf("Recovery: failed to get players for match %s: %v", matchID, err)
		players = nil
	}

	state, err := r.matchManager.GetMatch(matchID)
	if err != nil {
		state, err = r.matchManager.CreateMatch(matchID, time.Time{})
		if err != nil {
			return fmt.Errorf("create match state: %w", err)
		}
	}

	state.SyncFromChain(
		chainState.Pool,
		phase,
		chainState.AnswerDeadline,
		chainState.Winner,
		players,
	)

	if phase == match.PhaseQueue {
		if cfg, err := r.chainClient.GetMatchConfig(ctx, matchID); err == nil && cfg.QueueDeadline > 0 {
			state.SetQueueDeadline(time.Unix(int64(cfg.QueueDeadline), 0))
		}
	}

	switch phase {
	case match.PhaseQueue:
		result.QueueMatches++
		log.Printf("Recovery: match %s is in queue phase (deadline: %s)", matchID, state.QueueDeadline.Format(time.RFC3339))

	case match.PhaseActive:
		result.ActiveMatches++
		if err := r.recoverActivePhase(ctx, matchID, state); err != nil {
			log.Printf("Recovery: failed to recover active match %s: %v", matchID, err)
		} else {
			result.StuckRecovered++
		}

	case match.PhaseAnswerPeriod:
		result.AnswerPeriod++
		if err := r.recoverAnswerPeriod(ctx, matchID, state, chainState.AnswerDeadline); err != nil {
			log.Printf("Recovery: failed to recover answer period for match %s: %v", matchID, err)
		} else {
			result.TimersRegistered++
		}
	}

	return nil
}

// logResult logs the final recovery statistics.
func (r *RecoveryService) logResult(result *RecoveryResult) {
	log.Printf("Recovery complete: %d active matches (%d queue, %d active, %d answer period), %d timers registered, %d stuck recovered",
		result.TotalMatches, result.QueueMatches, result.ActiveMatches, result.AnswerPeriod,
		result.TimersRegistered, result.StuckRecovered)
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
