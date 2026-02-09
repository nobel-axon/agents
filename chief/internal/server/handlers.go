// Package server provides HTTP handlers for the axon-chief API.
package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"

	"github.com/axon-arena/axon-chief/internal/judge"
	"github.com/axon-arena/axon-chief/internal/match"
	"github.com/axon-arena/axon-chief/internal/reporter"
	"github.com/axon-arena/axon-chief/internal/verification"
	"github.com/axon-arena/axon-chief/internal/watcher"
)

// eventCleanupInterval determines how often we clean up old handled events.
const eventCleanupInterval = 10 * time.Minute

// eventRetentionPeriod determines how long we keep track of handled events.
const eventRetentionPeriod = 1 * time.Hour

// HealthResponse is the response for the health endpoint.
type HealthResponse struct {
	Status  string `json:"status"`
	Paused  bool   `json:"paused"`
	Version string `json:"version"`
}

func (s *Server) handleHealth(c *gin.Context) {
	status := "ok"
	if s.IsPaused() {
		status = "paused"
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:  status,
		Paused:  s.IsPaused(),
		Version: "1.0.0",
	})
}

// StatusResponse is the response for the status endpoint.
type StatusResponse struct {
	Operator       string        `json:"operator"`
	OperatorBalance string        `json:"operatorBalance"`
	IsOperator     bool          `json:"isOperator"`
	Paused         bool          `json:"paused"`
	ActiveMatches  int           `json:"activeMatches"`
	Matches        []MatchStatus `json:"matches"`
	Judges         []JudgeStatus `json:"judges"`
	Chain          ChainStatus   `json:"chain"`
}

// MatchStatus represents the status of a match.
type MatchStatus struct {
	ID             string `json:"id"`
	Phase          string `json:"phase"`
	PlayerCount    int    `json:"playerCount"`
	Pool           string `json:"pool"`
	QueueDeadline  string `json:"queueDeadline,omitempty"`
	AnswerDeadline string `json:"answerDeadline,omitempty"`
	Winner         string `json:"winner,omitempty"`
}

// JudgeStatus represents the status of a judge.
type JudgeStatus struct {
	Endpoint  string `json:"endpoint"`
	Role      string `json:"role"`
	Healthy   bool   `json:"healthy"`
	Provider  string `json:"provider"`
	LastCheck string `json:"lastCheck"`
	Error     string `json:"error,omitempty"`
}

// ChainStatus represents the chain connection status.
type ChainStatus struct {
	RPCURL   string `json:"rpcUrl"`
	ChainID  int64  `json:"chainId"`
	Arena    string `json:"arena"`
	Neuron   string `json:"neuron"`
	Connected bool   `json:"connected"`
}

func (s *Server) handleStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Get operator balance
	balance, err := s.chainClient.GetBalance(ctx)
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		balance = big.NewInt(0)
	}

	// Check if operator
	isOperator, err := s.chainClient.IsOperator(ctx)
	if err != nil {
		log.Printf("Failed to check operator: %v", err)
	}

	// Get active matches
	activeMatches := s.matchManager.GetActiveMatches()
	matchStatuses := make([]MatchStatus, 0, len(activeMatches))
	for _, m := range activeMatches {
		ms := MatchStatus{
			ID:          m.ID.String(),
			Phase:       m.GetPhase().String(),
			PlayerCount: m.PlayerCount,
			Pool:        m.Pool.String(),
		}
		if !m.QueueDeadline.IsZero() {
			ms.QueueDeadline = m.QueueDeadline.Format(time.RFC3339)
		}
		if !m.GetAnswerDeadline().IsZero() {
			ms.AnswerDeadline = m.GetAnswerDeadline().Format(time.RFC3339)
		}
		if m.GetWinner() != (common.Address{}) {
			ms.Winner = m.GetWinner().Hex()
		}
		matchStatuses = append(matchStatuses, ms)
	}

	// Get judge statuses
	judgeStatuses := s.judgeCoordinator.GetStatus()
	jsStatuses := make([]JudgeStatus, 0, len(judgeStatuses))
	for _, js := range judgeStatuses {
		status := JudgeStatus{
			Endpoint:  js.Endpoint,
			Role:      js.Role,
			Healthy:   js.Healthy,
			Provider:  js.Provider,
			LastCheck: js.LastCheck.Format(time.RFC3339),
		}
		if js.Error != nil {
			status.Error = js.Error.Error()
		}
		jsStatuses = append(jsStatuses, status)
	}

	c.JSON(http.StatusOK, StatusResponse{
		Operator:        s.chainClient.Address().Hex(),
		OperatorBalance: balance.String(),
		IsOperator:      isOperator,
		Paused:          s.IsPaused(),
		ActiveMatches:   len(activeMatches),
		Matches:         matchStatuses,
		Judges:          jsStatuses,
		Chain: ChainStatus{
			RPCURL:    s.cfg.Chain.RPCURL,
			ChainID:   s.cfg.Chain.ChainID,
			Arena:     s.cfg.Chain.ArenaAddress,
			Neuron:    s.cfg.Chain.NeuronAddress,
			Connected: true,
		},
	})
}

// EventRequest is the request for the event endpoint.
type EventRequest struct {
	Type    string                 `json:"type" binding:"required"`
	MatchID uint64                 `json:"matchId" binding:"required"`
	Data    map[string]interface{} `json:"data"`
}

// EventResponse is the response for the event endpoint.
type EventResponse struct {
	Acknowledged bool   `json:"acknowledged"`
	Message      string `json:"message,omitempty"`
}

func (s *Server) handleEvent(c *gin.Context) {
	if s.IsPaused() {
		log.Printf("POST /event: rejected (server paused)")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server is paused"})
		return
	}

	var req EventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("POST /event: bad request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("POST /event: type=%s matchId=%d", req.Type, req.MatchID)

	ctx := c.Request.Context()
	matchID := big.NewInt(int64(req.MatchID))

	switch req.Type {
	case "registration_closed":
		// Queue deadline passed - start match if enough players
		if err := s.handleRegistrationClosed(ctx, matchID, req.Data); err != nil {
			log.Printf("POST /event: registration_closed for match %d failed: %v", req.MatchID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("POST /event: registration_closed for match %d processed", req.MatchID)

	case "question_generated":
		// Question generated by judge - post to chain
		if err := s.handleQuestionGenerated(ctx, matchID, req.Data); err != nil {
			log.Printf("POST /event: question_generated for match %d failed: %v", req.MatchID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("POST /event: question_generated for match %d processed", req.MatchID)

	case "answer_submitted":
		// Answer submitted - queue for verification
		if err := s.handleAnswerSubmitted(ctx, matchID, req.Data); err != nil {
			log.Printf("POST /event: answer_submitted for match %d failed: %v", req.MatchID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		log.Printf("POST /event: answer_submitted for match %d processed", req.MatchID)

	case "agent_joined_queue":
		// Agent joined queue - triggers grace period + auto-start logic
		log.Printf("POST /event: agent_joined_queue for match %d", req.MatchID)
		s.handleAgentJoinedQueueEvent(ctx, matchID, req.Data)

	default:
		log.Printf("POST /event: unknown event type %q for match %d", req.Type, req.MatchID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown event type"})
		return
	}

	c.JSON(http.StatusOK, EventResponse{
		Acknowledged: true,
	})
}

func (s *Server) handleRegistrationClosed(ctx context.Context, matchID *big.Int, data map[string]interface{}) error {
	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		return err
	}

	// Skip if already past queue phase (stale watcher may have handled it)
	if state.Phase != match.PhaseQueue {
		log.Printf("Match %s: skipping registration_closed (already in phase %d)", matchID, state.Phase)
		return nil
	}

	// Idempotency guard: prevent concurrent start attempts for the same match.
	// This protects against duplicate registration_closed events from the server poller.
	matchKey := matchID.String()
	s.startingMu.Lock()
	if s.startingMatches[matchKey] {
		s.startingMu.Unlock()
		log.Printf("Match %s: skipping duplicate registration_closed (start already in progress)", matchID)
		return nil
	}
	s.startingMatches[matchKey] = true
	s.startingMu.Unlock()
	defer func() {
		s.startingMu.Lock()
		delete(s.startingMatches, matchKey)
		s.startingMu.Unlock()
	}()

	// Use a detached context for chain operations — HTTP request context may timeout
	chainCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

	// Use in-memory player count (updated by chain events) instead of server's DB count.
	// The server's poller may report stale data if the indexer hasn't caught up yet.
	serverCount, _ := data["playerCount"].(float64)
	inMemCount := state.PlayerCount
	playerCount := inMemCount
	if int(serverCount) > inMemCount {
		playerCount = int(serverCount) // Use whichever is higher
	}
	log.Printf("Match %s: registration closed — server reports %d players, chain events show %d, using %d",
		matchID, int(serverCount), inMemCount, playerCount)
	if playerCount < int(s.cfg.Match.MinPlayers) {
		// Not enough players - cancel
		log.Printf("Match %s: not enough players (%d < %d), cancelling",
			matchID, playerCount, s.cfg.Match.MinPlayers)
		if err := s.chainClient.CancelMatch(chainCtx, matchID); err != nil {
			cancel()
			return err
		}
		cancel()
		state.SetPhase(match.PhaseCancelled)
		s.createNextMatchAfterCooldown()
		return nil
	}

	// Start match
	log.Printf("Match %s: starting with %d players", matchID, playerCount)
	if err := s.chainClient.StartMatch(chainCtx, matchID); err != nil {
		cancel()
		return err
	}
	state.SetPhase(match.PhaseActive)

	// Generate question in background — don't block the HTTP handler
	go func() {
		defer cancel()
		// Wait for chain state to propagate across RPC load-balanced nodes.
		// Monad testnet RPC uses multiple nodes; after startMatch tx is confirmed,
		// other nodes may still report the old phase (Queue instead of QuestionRevealed).
		log.Printf("Match %s: waiting for QuestionRevealed phase confirmation on chain", matchID)
		if err := s.chainClient.WaitForPhase(chainCtx, matchID, 1, 30*time.Second); err != nil {
			log.Printf("Match %s: chain phase confirmation warning: %v (proceeding anyway)", matchID, err)
		}
		if err := s.generateAndPostQuestion(chainCtx, state); err != nil {
			log.Printf("Match %s: failed to generate/post question: %v", matchID, err)
		}
	}()
	return nil
}

// maxQuestionRetries is the maximum number of attempts to generate a valid question.
const maxQuestionRetries = 3

func (s *Server) generateAndPostQuestion(ctx context.Context, state *match.State) error {
	matchID := state.ID

	var lastQResp *judge.GenerateQuestionResponse
	var lastErr error

	// Get the philosopher (single instance, no rotation)
	philosopher, err := s.judgeCoordinator.PickGenerator(ctx)
	if err != nil {
		return fmt.Errorf("philosopher unavailable: %w", err)
	}

	for attempt := 0; attempt < maxQuestionRetries; attempt++ {
		// Generate question via philosopher
		qResp, err := philosopher.GenerateQuestion(ctx, matchID.String(), "", 0, nil)
		if err != nil {
			lastErr = err
			log.Printf("Match %s: failed to generate question (attempt %d): %v", matchID, attempt+1, err)
			continue
		}
		lastQResp = qResp

		// Validate with judge agents
		validation, err := s.judgeCoordinator.ValidateQuestionWithConsensus(
			ctx,
			matchID.String(),
			qResp.Question,
			qResp.FormatHint,
			qResp.Category,
			qResp.Difficulty,
		)
		if err != nil {
			log.Printf("Match %s: question validation failed (attempt %d): %v", matchID, attempt+1, err)
			// Continue with the question anyway if validation fails due to errors
			return s.postValidatedQuestion(ctx, state, qResp)
		}

		if validation.Valid {
			log.Printf("Match %s: question validated by judges (attempt %d)", matchID, attempt+1)
			return s.postValidatedQuestion(ctx, state, qResp)
		}

		// Question rejected - log issues and retry
		log.Printf("Match %s: question rejected by judges (attempt %d): %v", matchID, attempt+1, validation.Issues)
	}

	// All retries failed - post the last generated question anyway (better than stuck match)
	if lastQResp != nil {
		log.Printf("Match %s: all validation attempts failed, posting last question anyway", matchID)
		return s.postValidatedQuestion(ctx, state, lastQResp)
	}

	return fmt.Errorf("failed to generate question after %d attempts: %w", maxQuestionRetries, lastErr)
}

// postValidatedQuestion posts a validated question to chain and starts the gap timer.
func (s *Server) postValidatedQuestion(ctx context.Context, state *match.State, qResp *judge.GenerateQuestionResponse) error {
	matchID := state.ID

	// Store question data
	var answerHash [32]byte
	hashBytes, _ := hex.DecodeString(qResp.AnswerHash[2:]) // Remove 0x prefix
	copy(answerHash[:], hashBytes)

	qData := &match.QuestionData{
		Text:       qResp.Question,
		Category:   qResp.Category,
		Difficulty: uint8(qResp.Difficulty),
		FormatHint: qResp.FormatHint,
		AnswerHash: answerHash,
	}
	state.SetQuestion(qData)

	// Generate 3 judge personalities for this match
	if err := s.generatePersonalities(ctx, state, qResp.Category); err != nil {
		log.Printf("Match %s: failed to generate personalities, using defaults: %v", matchID, err)
		// Continue even if personality generation fails - can use defaults
	}

	// Pre-flight check: verify match is in QuestionRevealed phase on chain
	chainState, pfErr := s.chainClient.GetMatchState(ctx, matchID)
	if pfErr != nil {
		log.Printf("Match %s: pre-flight phase check failed: %v", matchID, pfErr)
	} else {
		log.Printf("Match %s: pre-flight chain phase=%d, answerHash=%x before PostQuestion",
			matchID, chainState.Phase, chainState.AnswerHash)
		if chainState.Phase != 1 { // 1 = QuestionRevealed
			return fmt.Errorf("match %s not in QuestionRevealed phase on chain (got phase %d)", matchID, chainState.Phase)
		}
	}

	// Post question to chain
	if err := s.chainClient.PostQuestion(
		ctx,
		matchID,
		qData.Text,
		qData.Category,
		qData.Difficulty,
		qData.FormatHint,
		qData.AnswerHash,
	); err != nil {
		return err
	}

	log.Printf("Match %s: question posted to chain", matchID)

	// Report question to server
	if s.reporter != nil {
		go s.reporter.ReportMatchUpdate(context.Background(), reporter.MatchUpdateRequest{
			MatchID:      matchID.Int64(),
			Phase:        "question_live",
			QuestionText: qData.Text,
			Category:     qData.Category,
			Difficulty:   int(qData.Difficulty),
			FormatHint:   qData.FormatHint,
			AnswerHash:   "0x" + hex.EncodeToString(qData.AnswerHash[:]),
		})
	}

	// Start gap timer instead of immediately starting answer period
	// The gap timer will call StartAnswerPeriod when it fires
	return s.handleQuestionPosted(ctx, matchID, state)
}

// generatePersonalities generates 3 unique judge personalities for the match.
func (s *Server) generatePersonalities(ctx context.Context, state *match.State, category string) error {
	matchID := state.ID

	// Request personality generation from judge agent
	resp, err := s.judgeCoordinator.GeneratePersonalities(ctx, matchID.String(), category)
	if err != nil {
		return err
	}

	if len(resp.Personalities) < 3 {
		return fmt.Errorf("expected 3 personalities, got %d", len(resp.Personalities))
	}

	// Convert to match.Personality type and store
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

	// Assign judge panel for this match (snapshot of healthy judges)
	panel := s.judgeCoordinator.AssignJudgePanel()
	state.SetJudgePanel(panel)

	log.Printf("Match %s: generated %d judge personalities: %s, %s, %s",
		matchID,
		len(personalities),
		personalities[0].Name,
		personalities[1].Name,
		personalities[2].Name,
	)

	log.Printf("Match %s: assigned judge panel %v (%d judges)",
		matchID, panel, len(panel))

	// Report personalities to server
	if s.reporter != nil {
		go s.reporter.ReportPersonalities(context.Background(), matchID.String(), personalities, panel)
	}

	return nil
}

func (s *Server) handleQuestionGenerated(ctx context.Context, matchID *big.Int, data map[string]interface{}) error {
	// This event comes from external sources (Ponder) when question is already on chain
	// Used to sync state if we missed events

	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		// Match not tracked - try to recover from chain
		chainState, err := s.chainClient.GetMatchState(ctx, matchID)
		if err != nil {
			return err
		}

		// Create match state
		state, _ = s.matchManager.CreateMatch(matchID, time.Time{})
		state.SyncFromChain(
			chainState.Pool,
			match.Phase(chainState.Phase),
			chainState.AnswerDeadline,
			chainState.Winner,
			nil,
		)
	}

	// Update phase
	state.SetPhase(match.PhaseActive)
	return nil
}

// handleAgentJoinedQueueEvent processes an agent_joined_queue event from axon-server.
// It constructs a ChainEvent and routes it through onChainEvent for grace period + auto-start logic.
func (s *Server) handleAgentJoinedQueueEvent(ctx context.Context, matchID *big.Int, data map[string]interface{}) {
	agentStr, _ := data["agent"].(string)
	playerCount, _ := data["playerCount"].(float64)
	txHash, _ := data["txHash"].(string)

	log.Printf("Match %s: agent_joined_queue event from server — agent=%s playerCount=%d",
		matchID, agentStr, int(playerCount))

	event := &watcher.ChainEvent{
		Type:    watcher.EventAgentJoinedQueue,
		MatchID: matchID,
		Data:    make(map[string]interface{}),
	}

	if agentStr != "" {
		event.Data["agent"] = common.HexToAddress(agentStr)
	}
	if playerCount > 0 {
		event.Data["playerCount"] = big.NewInt(int64(playerCount))
	}
	if txHash != "" {
		event.TxHash = common.HexToHash(txHash)
	}

	s.onChainEvent(ctx, event)
}

func (s *Server) handleAnswerSubmitted(ctx context.Context, matchID *big.Int, data map[string]interface{}) error {
	// Extract answer data
	agentStr, _ := data["agent"].(string)
	answer, _ := data["answer"].(string)
	reasoning, _ := data["reasoning"].(string) // Optional reasoning for the answer
	blockNumber, _ := data["blockNumber"].(float64)
	txIndex, _ := data["txIndex"].(float64)
	attemptNumber, _ := data["attemptNumber"].(float64)
	neuronBurned, _ := data["neuronBurned"].(string)
	txHash, _ := data["txHash"].(string)

	// Idempotency check — use txHash:logIndex when available,
	// otherwise fall back to matchId:agent:attempt
	var eventKey string
	if txHash != "" {
		eventKey = makeEventKey(txHash, uint(txIndex))
	} else {
		eventKey = fmt.Sprintf("%s:%s:%d", matchID, agentStr, int(attemptNumber))
	}
	if s.isEventHandled(eventKey) {
		log.Printf("Match %s: answer event already handled (key: %s)", matchID, eventKey)
		return nil
	}

	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		return err
	}

	agent := common.HexToAddress(agentStr)
	burned, _ := new(big.Int).SetString(neuronBurned, 10)

	// Enqueue for verification
	submitted := &verification.SubmittedAnswer{
		MatchID:       matchID,
		Agent:         agent,
		Answer:        answer,
		Reasoning:     reasoning,
		BlockNumber:   uint64(blockNumber),
		TxIndex:       uint(txIndex),
		AttemptNumber: uint64(attemptNumber),
		NeuronBurned:  burned,
		Timestamp:     time.Now(),
	}

	s.verificationMgr.EnqueueAnswer(submitted)

	// Report answer submission to server
	if s.reporter != nil {
		go s.reporter.ReportAnswerSubmitted(context.Background(), reporter.AnswerSubmittedRequest{
			MatchID:       matchID.Int64(),
			Agent:         agent.Hex(),
			Answer:        answer,
			Reasoning:     reasoning,
			AttemptNumber: int(attemptNumber),
			NeuronBurned:  neuronBurned,
		})
	}

	// Mark event as handled
	s.markEventHandled(eventKey, "answer_submitted", matchID.String())

	// Check if we have personality-based judging enabled for this match
	if state.HasPersonalities() {
		// Use personality-based evaluation in background goroutine
		// (HTTP request context would be canceled before judges finish LLM evaluation)
		go func() {
			evalCtx, evalCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer evalCancel()
			evalResult, err := s.verificationMgr.EvaluateSingleAnswer(evalCtx, state, submitted)
			if err != nil {
				log.Printf("Match %s: personality evaluation failed for %s: %v (will retry on timeout)",
					matchID, agent.Hex(), err)
				return
			}
			log.Printf("Match %s: evaluated answer from %s - score=%d, agreement=%s",
				matchID, agent.Hex(), evalResult.TotalScore, evalResult.Agreement)

			// Report evaluation result to server
			if s.reporter != nil {
				evaluationsJSON, _ := json.Marshal(evalResult)
				go s.reporter.ReportAnswerResult(context.Background(), reporter.AnswerResultRequest{
					MatchID:       matchID.Int64(),
					AgentAddr:     agent.Hex(),
					AttemptNumber: int(attemptNumber),
					IsCorrect:     true,
				Consensus:     evalResult.Agreement,
				Confidence:    1.0,
				TotalScore:    evalResult.TotalScore,
				Agreement:     evalResult.Agreement,
				Reasoning:     reasoning,
				Evaluations:   evaluationsJSON,
			})
		}
		}()

		// Don't settle immediately - wait for answer period to end or all participants to submit
		return nil
	}

	// No personalities — match was likely created externally (bypassing Chief).
	// Answer is queued; timeout handler will settle with best available submitter.
	log.Printf("Match %s: no personalities assigned, answer from %s queued (will settle on timeout)",
		matchID, agent.Hex())
	return nil
}

// Idempotency helpers

// makeEventKey creates a unique key for an event based on txHash and logIndex.
func makeEventKey(txHash string, logIndex uint) string {
	return fmt.Sprintf("%s:%d", txHash, logIndex)
}

// isEventHandled checks if an event has already been handled.
func (s *Server) isEventHandled(eventKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.handledEvents[eventKey]
	return exists
}

// markEventHandled marks an event as handled.
func (s *Server) markEventHandled(eventKey, eventType, matchID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handledEvents[eventKey] = time.Now()
	log.Printf("Marked event as handled: %s (type: %s, match: %s)", eventKey, eventType, matchID)
}

// cleanupHandledEvents removes old entries from the handled events map.
func (s *Server) cleanupHandledEvents() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-eventRetentionPeriod)
	for key, timestamp := range s.handledEvents {
		if timestamp.Before(cutoff) {
			delete(s.handledEvents, key)
		}
	}
}

// Gap timer helpers

// startGapTimer starts a gap timer for a match.
// When the timer fires, it will start the answer period.
func (s *Server) startGapTimer(matchID *big.Int, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := matchID.String()

	// Cancel existing timer if any
	if timer, ok := s.gapTimers[key]; ok {
		timer.Stop()
	}

	log.Printf("Match %s: starting gap timer for %v", matchID, duration)

	s.gapTimers[key] = time.AfterFunc(duration, func() {
		s.handleGapElapsed(matchID)
	})
}

// cancelGapTimer cancels a gap timer for a match.
func (s *Server) cancelGapTimer(matchID *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := matchID.String()
	if timer, ok := s.gapTimers[key]; ok {
		timer.Stop()
		delete(s.gapTimers, key)
		log.Printf("Match %s: cancelled gap timer", matchID)
	}
}

// handleGapElapsed is called when the gap timer fires.
func (s *Server) handleGapElapsed(matchID *big.Int) {
	s.mu.Lock()
	delete(s.gapTimers, matchID.String())
	s.mu.Unlock()

	log.Printf("Match %s: gap timer elapsed, starting answer period", matchID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get match state
	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		log.Printf("Match %s: gap elapsed but match not found: %v", matchID, err)
		return
	}

	// Verify we're still in Active phase (question posted but answer period not started)
	if state.GetPhase() != match.PhaseActive {
		log.Printf("Match %s: gap elapsed but phase is %s, not active", matchID, state.GetPhase())
		return
	}

	// Start answer period on chain
	if err := s.chainClient.StartAnswerPeriod(ctx, matchID); err != nil {
		log.Printf("Match %s: failed to start answer period: %v", matchID, err)
		return
	}

	// Compute deadline locally from config — reading chain state immediately after TX
	// confirmation can return stale data due to RPC node sync lag (answerDeadline=0).
	cfg := s.matchManager.GetConfig()
	deadline := time.Now().Add(cfg.AnswerDuration)
	state.SetAnswerDeadline(deadline)
	state.SetPhase(match.PhaseAnswerPeriod)

	// Register timeout
	s.timeoutWatcher.RegisterMatch(matchID, deadline)

	log.Printf("Match %s: answer period started, deadline %v", matchID, deadline)

	// Announce answer period via commentary
	if s.reporter != nil {
		go s.reporter.ReportCommentary(context.Background(), reporter.CommentaryRequest{
			MatchID:   matchID.Int64(),
			AgentID:   "system",
			EventType: "answer_period_started",
			Text:      "The arena is open for answers. Challengers, state your case.",
		})
	}
}

// handleQuestionPosted is called when a question is posted to chain.
// It starts the gap timer instead of immediately starting the answer period.
func (s *Server) handleQuestionPosted(ctx context.Context, matchID *big.Int, state *match.State) error {
	gapDuration := s.GetGapDuration()
	log.Printf("Match %s: question posted, starting gap timer (%v)", matchID, gapDuration)

	// Start gap timer (configurable, default 10 seconds)
	s.startGapTimer(matchID, gapDuration)

	return nil
}

// Mutex for concurrent access protection (used by gap timer and idempotency)
var _ sync.RWMutex

// Match detail response types

// QuestionResponse is the question data in the match detail response.
type QuestionResponse struct {
	Text       string `json:"text"`
	Category   string `json:"category"`
	Difficulty uint8  `json:"difficulty"`
	FormatHint string `json:"formatHint"`
}

// PersonalityResponse is a judge personality in the match detail response.
type PersonalityResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Perspective string   `json:"perspective"`
	Values      []string `json:"values"`
	Style       string   `json:"style"`
}

// EvaluationResponse is a participant's evaluation in the match detail response.
type EvaluationResponse struct {
	Participant  string                           `json:"participant"`
	Answer       string                           `json:"answer"`
	Reasoning    string                           `json:"reasoning,omitempty"`
	JudgeResults []JudgeResultResponse            `json:"judgeResults,omitempty"`
	Evaluations  []PersonalityEvaluationResponse  `json:"evaluations"`
	TotalScore   int                              `json:"totalScore"`
	Agreement    string                           `json:"agreement"`
}

// JudgeResultResponse is a single judge instance's evaluation in the API response.
type JudgeResultResponse struct {
	JudgeIndex  int                              `json:"judgeIndex"`
	TotalScore  int                              `json:"totalScore"`
	Agreement   string                           `json:"agreement"`
	Evaluations []PersonalityEvaluationResponse  `json:"evaluations"`
}

// PersonalityEvaluationResponse is a single personality's evaluation.
type PersonalityEvaluationResponse struct {
	PersonalityID   string `json:"personalityId"`
	PersonalityName string `json:"personalityName"`
	Score           int    `json:"score"`
	Convinced       string `json:"convinced"`
	Concerns        string `json:"concerns"`
	Verdict         string `json:"verdict"`
	AgreeLevel      string `json:"agreeLevel"`
}

// MatchDetailResponse is the response for GET /matches/:matchId.
type MatchDetailResponse struct {
	MatchID        string                `json:"matchId"`
	Phase          string                `json:"phase"`
	Pool           string                `json:"pool"`
	PlayerCount    int                   `json:"playerCount"`
	Question       *QuestionResponse     `json:"question,omitempty"`
	Personalities  []PersonalityResponse `json:"personalities,omitempty"`
	Evaluations    []EvaluationResponse  `json:"evaluations,omitempty"`
	QueueDeadline  int64                 `json:"queueDeadline,omitempty"`
	AnswerDeadline int64                 `json:"answerDeadline,omitempty"`
	Winner         string                `json:"winner,omitempty"`
}

// handleGetMatch handles GET /matches/:matchId.
// Returns match state including question and personalities if revealed.
func (s *Server) handleGetMatch(c *gin.Context) {
	matchIDStr := c.Param("matchId")
	matchID, ok := new(big.Int).SetString(matchIDStr, 10)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid match ID"})
		return
	}

	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "match not found"})
		return
	}

	resp := MatchDetailResponse{
		MatchID:     matchID.String(),
		Phase:       state.GetPhase().String(),
		Pool:        state.Pool.String(),
		PlayerCount: state.PlayerCount,
	}

	// Include queue deadline if set
	if !state.QueueDeadline.IsZero() {
		resp.QueueDeadline = state.QueueDeadline.Unix()
	}

	// Include answer deadline if set
	answerDeadline := state.GetAnswerDeadline()
	if !answerDeadline.IsZero() {
		resp.AnswerDeadline = answerDeadline.Unix()
	}

	// Include winner if settled
	winner := state.GetWinner()
	if winner != (common.Address{}) {
		resp.Winner = winner.Hex()
	}

	// Include question if revealed (phase >= Active)
	phase := state.GetPhase()
	if phase >= match.PhaseActive {
		q := state.GetQuestion()
		if q != nil {
			resp.Question = &QuestionResponse{
				Text:       q.Text,
				Category:   q.Category,
				Difficulty: q.Difficulty,
				FormatHint: q.FormatHint,
			}
		}
	}

	// Include personalities if assigned (phase >= Active)
	if phase >= match.PhaseActive {
		personalities := state.GetPersonalities()
		if len(personalities) > 0 {
			resp.Personalities = make([]PersonalityResponse, len(personalities))
			for i, p := range personalities {
				resp.Personalities[i] = PersonalityResponse{
					ID:          p.ID,
					Name:        p.Name,
					Perspective: p.Perspective,
					Values:      p.Values,
					Style:       p.Style,
				}
			}
		}
	}

	// Include evaluations if any (phase >= AnswerPeriod)
	if phase >= match.PhaseAnswerPeriod {
		evals := state.GetAllEvaluations()
		if len(evals) > 0 {
			resp.Evaluations = make([]EvaluationResponse, 0, len(evals))
			for _, eval := range evals {
				evalResp := EvaluationResponse{
					Participant: eval.Participant,
					Answer:      eval.Answer,
					Reasoning:   eval.Reasoning,
					TotalScore:  eval.TotalScore,
					Agreement:   eval.Agreement,
					Evaluations: make([]PersonalityEvaluationResponse, len(eval.Evaluations)),
				}
				for j, pe := range eval.Evaluations {
					evalResp.Evaluations[j] = PersonalityEvaluationResponse{
						PersonalityID:   pe.PersonalityID,
						PersonalityName: pe.PersonalityName,
						Score:           pe.Score,
						Convinced:       pe.Convinced,
						Concerns:        pe.Concerns,
						Verdict:         pe.Verdict,
						AgreeLevel:      pe.AgreeLevel,
					}
				}
				// Include per-judge results if available
				if len(eval.JudgeResults) > 0 {
					evalResp.JudgeResults = make([]JudgeResultResponse, len(eval.JudgeResults))
					for j, jr := range eval.JudgeResults {
						jrResp := JudgeResultResponse{
							JudgeIndex: jr.JudgeIndex,
							TotalScore: jr.TotalScore,
							Agreement:  jr.Agreement,
							Evaluations: make([]PersonalityEvaluationResponse, len(jr.Evaluations)),
						}
						for k, pe := range jr.Evaluations {
							jrResp.Evaluations[k] = PersonalityEvaluationResponse{
								PersonalityID:   pe.PersonalityID,
								PersonalityName: pe.PersonalityName,
								Score:           pe.Score,
								Convinced:       pe.Convinced,
								Concerns:        pe.Concerns,
								Verdict:         pe.Verdict,
								AgreeLevel:      pe.AgreeLevel,
							}
						}
						evalResp.JudgeResults[j] = jrResp
					}
				}
				resp.Evaluations = append(resp.Evaluations, evalResp)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}
