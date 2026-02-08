// Package server provides HTTP handlers for the axon-chief API.
package server

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"

	"github.com/axon-arena/axon-chief/internal/burn"
	"github.com/axon-arena/axon-chief/internal/chain"
	"github.com/axon-arena/axon-chief/internal/config"
	"github.com/axon-arena/axon-chief/internal/judge"
	"github.com/axon-arena/axon-chief/internal/match"
	"github.com/axon-arena/axon-chief/internal/reporter"
	"github.com/axon-arena/axon-chief/internal/verification"
	"github.com/axon-arena/axon-chief/internal/watcher"
)

// Server holds all dependencies for the HTTP server.
type Server struct {
	mu sync.RWMutex

	cfg               *config.Config
	chainClient       *chain.ChainClient
	judgeCoordinator  *judge.Coordinator
	matchManager      *match.Manager
	verificationMgr   *verification.Manager
	timeoutWatcher    *watcher.TimeoutWatcher
	staleWatcher      *watcher.StaleQueueWatcher
	eventDetector     *watcher.EventDetector
	recoveryService   *RecoveryService
	burnManager       *burn.Manager
	reporter          *reporter.Reporter

	// Event handling state
	handledEvents map[string]time.Time   // eventKey (txHash:logIndex) -> timestamp
	gapTimers     map[string]*time.Timer // matchID -> gap timer

	// Idempotency guard: prevents concurrent start attempts for the same match.
	// Protected by startingMu (separate from mu to avoid lock contention).
	startingMu      sync.Mutex
	startingMatches map[string]bool // matchID string -> currently being started

	paused     bool
	lastCreate time.Time
	shutdownCh chan struct{} // Closed on Stop() to cancel background goroutines
}

// DefaultGapDuration is the default time between question posting and answer period start.
// This can be overridden by config.Match.GapDuration.
const DefaultGapDuration = 10 * time.Second

// GetGapDuration returns the gap duration from config, falling back to default.
func (s *Server) GetGapDuration() time.Duration {
	if s.cfg.Match.GapDuration > 0 {
		return s.cfg.Match.GapDuration
	}
	return DefaultGapDuration
}

// NewServer creates a new server instance.
func NewServer(
	cfg *config.Config,
	chainClient *chain.ChainClient,
	judgeCoordinator *judge.Coordinator,
	matchManager *match.Manager,
) *Server {
	s := &Server{
		cfg:              cfg,
		chainClient:      chainClient,
		judgeCoordinator: judgeCoordinator,
		matchManager:     matchManager,
		handledEvents:    make(map[string]time.Time),
		gapTimers:        make(map[string]*time.Timer),
		startingMatches:  make(map[string]bool),
		shutdownCh:       make(chan struct{}),
	}

	// Create verification manager with settlement callback
	s.verificationMgr = verification.NewManager(judgeCoordinator, s.onSettleMatch)

	// Create timeout watcher
	s.timeoutWatcher = watcher.NewTimeoutWatcher(
		matchManager,
		s.onMatchTimeout,
		cfg.Watcher.TimeoutCheckInterval,
	)

	// Create stale queue watcher
	s.staleWatcher = watcher.NewStaleQueueWatcher(
		matchManager,
		s.onStaleQueue,
		cfg.Watcher.StaleCheckInterval,
		cfg.Watcher.StaleThreshold,
		cfg.Match.MinPlayers,
	)

	// Create event detector
	s.eventDetector = watcher.NewEventDetector(
		chainClient.GetClient(),
		common.HexToAddress(cfg.Chain.ArenaAddress),
		s.onChainEvent,
		cfg.Watcher.EventPollInterval,
		cfg.Watcher.PonderHealthThreshold,
		cfg.Watcher.PonderLagBlocks,
	)

	// Create recovery service
	s.recoveryService = NewRecoveryService(
		chainClient,
		matchManager,
		s.timeoutWatcher,
		s.verificationMgr,
		judgeCoordinator,
		s.GetGapDuration(),
		s.generateAndPostQuestion,
		cfg.Server.URL,
	)

	// Create burn manager for nad.fun swap integration
	s.burnManager = burn.NewManager(&cfg.NadFun, chainClient)

	// Create reporter for pushing data to axon-server (optional)
	if cfg.Server.URL != "" {
		s.reporter = reporter.NewReporter(cfg.Server.URL, cfg.Server.InternalSecret)
	}

	return s
}

// Start starts all background watchers.
func (s *Server) Start(ctx context.Context) error {
	// Initialize nad.fun contracts for burn allocation swaps
	treasuryAddr, err := s.chainClient.GetTreasury(ctx)
	if err != nil {
		log.Printf("Warning: failed to get treasury address, burn fallback disabled: %v", err)
	} else {
		if err := s.chainClient.InitNadFun(&s.cfg.NadFun, treasuryAddr); err != nil {
			log.Printf("Warning: nad.fun integration disabled: %v", err)
		}
	}

	// Recover active matches from chain
	if err := s.recoverActiveMatches(ctx); err != nil {
		log.Printf("Warning: failed to recover active matches: %v", err)
	}

	// Start watchers
	s.timeoutWatcher.Start()
	s.staleWatcher.Start()

	// Only start direct chain polling if explicitly enabled.
	// By default, events arrive via Ponder (axon-server POST /event).
	// The EventDetector struct is still used for deduplication (DeduplicateEvent/ReportPonderEvent).
	if s.cfg.Watcher.DirectChainPolling {
		log.Println("Direct chain polling ENABLED — EventDetector background loop starting")
		if err := s.eventDetector.Start(); err != nil {
			return err
		}
	} else {
		log.Println("Direct chain polling DISABLED — relying on Ponder via axon-server for chain events")
	}

	// Start health check loop
	go s.healthCheckLoop(ctx)

	// Auto-create first match if enabled
	if s.cfg.Match.AutoCreate {
		go func() {
			time.Sleep(2 * time.Second) // Wait for watchers to start
			if err := s.CreateNextMatch(context.Background()); err != nil {
				log.Printf("Failed to create initial match: %v", err)
			}
		}()
	}

	return nil
}

// Stop stops all background watchers and cancels grace timers.
func (s *Server) Stop() {
	close(s.shutdownCh)
	s.timeoutWatcher.Stop()
	s.staleWatcher.Stop()
	s.eventDetector.Stop()
}

// SetupRoutes configures the gin router.
func (s *Server) SetupRoutes(r *gin.Engine) {
	// Public endpoints
	r.GET("/health", s.handleHealth)
	r.GET("/status", s.handleStatus)
	r.POST("/event", s.handleEvent)

	// Match detail endpoint (question + personalities)
	r.GET("/matches/:matchId", s.handleGetMatch)

	// Admin endpoints
	admin := r.Group("/admin")
	admin.Use(s.adminAuth())
	{
		admin.POST("/pause", s.handleAdminPause)
		admin.POST("/resume", s.handleAdminResume)
		admin.POST("/settle/:matchId", s.handleAdminSettle)
		admin.POST("/refund/:matchId", s.handleAdminRefund)
		admin.POST("/create-match", s.handleAdminCreateMatch)
	}
}

// IsPaused returns whether the server is paused.
func (s *Server) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

// SetPaused sets the paused state.
func (s *Server) SetPaused(paused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = paused
}

// recoverActiveMatches recovers active match states from the chain using the recovery service.
func (s *Server) recoverActiveMatches(ctx context.Context) error {
	result, err := s.recoveryService.RecoverAllState(ctx)
	if err != nil {
		return err
	}

	log.Printf("Recovered %d active matches", result.TotalMatches)
	return nil
}

// healthCheckLoop periodically checks judge health.
func (s *Server) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.judgeCoordinator.CheckHealth(ctx)
		}
	}
}

// CreateNextMatch creates a new match on-chain.
func (s *Server) CreateNextMatch(ctx context.Context) error {
	if s.IsPaused() {
		log.Println("Server is paused, not creating new match")
		return nil
	}

	if !s.matchManager.CanCreateNewMatch() {
		log.Println("Cannot create new match (cooldown not elapsed)")
		return nil
	}

	cfg := s.matchManager.GetConfig()

	// Create match on chain
	matchID, err := s.chainClient.CreateMatch(
		ctx,
		cfg.EntryFee,
		cfg.BaseAnswerFee,
		uint64(cfg.QueueDuration.Seconds()),
		uint64(cfg.AnswerDuration.Seconds()),
		cfg.MinPlayers,
		cfg.MaxPlayers,
	)
	if err != nil {
		return err
	}

	// Calculate queue deadline
	queueDeadline := time.Now().Add(cfg.QueueDuration)

	// Track in memory
	_, err = s.matchManager.CreateMatch(matchID, queueDeadline)
	if err != nil {
		log.Printf("Warning: failed to track match %s in memory: %v", matchID, err)
	}

	log.Printf("Created match %s (queue deadline: %v)", matchID, queueDeadline)

	// Report new match to server
	if s.reporter != nil {
		go s.reporter.ReportMatchUpdate(context.Background(), reporter.MatchUpdateRequest{
			MatchID:   matchID.Int64(),
			Phase:     "registration",
			EntryFee:  cfg.EntryFee.String(),
			AnswerFee: cfg.BaseAnswerFee.String(),
		})
	}

	return nil
}

// onSettleMatch is called when verification determines a winner.
func (s *Server) onSettleMatch(ctx context.Context, matchID *big.Int, winner common.Address) error {
	// Cancel timeout
	s.timeoutWatcher.CancelMatch(matchID)

	// Settle on chain
	if err := s.chainClient.SettleWinner(ctx, matchID, winner); err != nil {
		return err
	}

	// Update match state
	if state, err := s.matchManager.GetMatch(matchID); err == nil {
		state.SetPhase(match.PhaseSettled)
		state.SetWinner(winner)
	}

	// Report settlement to server (winner gets ~90% of pool)
	if s.reporter != nil {
		prizeMON := "0"
		if state, err := s.matchManager.GetMatch(matchID); err == nil && state.Pool != nil && state.Pool.Sign() > 0 {
			// 90% of pool goes to winner (contract enforced)
			prize := new(big.Int).Mul(state.Pool, big.NewInt(90))
			prize.Div(prize, big.NewInt(100))
			prizeMON = prize.String()
		}
		go s.reporter.ReportSettlement(context.Background(), reporter.SettleMatchRequest{
			MatchID:    matchID.Int64(),
			WinnerAddr: winner.Hex(),
			PrizeMON:   prizeMON,
		})
	}

	// Reveal answer
	if state, err := s.matchManager.GetMatch(matchID); err == nil {
		q := state.GetQuestion()
		if q != nil && q.Answer != "" {
			if err := s.chainClient.RevealAnswer(ctx, matchID, q.Answer, q.Salt); err != nil {
				log.Printf("Warning: failed to reveal answer for match %s: %v", matchID, err)
			}
			// Report answer reveal to server
			if s.reporter != nil {
				go s.reporter.ReportAnswerRevealed(context.Background(), matchID.String(), q.Answer, fmt.Sprintf("%x", q.Salt))
			}
		} else {
			// Get answer from philosopher
			reveal, err := s.judgeCoordinator.RequestReveal(ctx, matchID.String())
			if err != nil {
				log.Printf("Warning: failed to get reveal from philosopher: %v", err)
			} else {
				var salt [32]byte
				copy(salt[:], common.FromHex(reveal.Salt))
				if err := s.chainClient.RevealAnswer(ctx, matchID, reveal.Answer, salt); err != nil {
					log.Printf("Warning: failed to reveal answer for match %s: %v", matchID, err)
				}
				// Report answer reveal to server
				if s.reporter != nil {
					go s.reporter.ReportAnswerRevealed(context.Background(), matchID.String(), reveal.Answer, reveal.Salt)
				}
			}
		}
	}

	// Clean up
	s.verificationMgr.RemoveQueue(matchID)

	// Execute burn allocation swap (async, non-blocking)
	// This claims the 5% burn allocation, swaps MON→NEURON via nad.fun, and burns the NEURON
	go func() {
		burnCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.burnManager.ExecuteBurnAllocation(burnCtx, matchID, winner); err != nil {
			log.Printf("Match %s: burn allocation failed (will be reconciled): %v", matchID, err)
		}
	}()

	// Create next match
	if s.cfg.Match.AutoCreate {
		go func() {
			time.Sleep(s.cfg.Match.Cooldown)
			if err := s.CreateNextMatch(context.Background()); err != nil {
				log.Printf("Failed to create next match: %v", err)
			}
		}()
	}

	return nil
}

// onMatchTimeout is called when a match times out.
func (s *Server) onMatchTimeout(ctx context.Context, matchID *big.Int) error {
	log.Printf("Match %s answer period ended", matchID)

	state, err := s.matchManager.GetMatch(matchID)
	if err != nil {
		log.Printf("Match %s not found, issuing refund", matchID)
		return s.chainClient.RefundMatch(ctx, matchID)
	}

	// Check if this match uses personality-based judging
	if state.HasPersonalities() {
		// Determine winner by highest score from personality evaluations
		winner, score, err := s.verificationMgr.DetermineWinnerByScore(state)
		if err != nil {
			log.Printf("Match %s: no valid evaluations, issuing refund", matchID)
			return s.refundMatch(ctx, matchID, state)
		}

		if winner != (common.Address{}) && score > 0 {
			log.Printf("Match %s: settling with winner %s (score: %d)", matchID, winner.Hex(), score)

			// Settle on chain
			if err := s.chainClient.SettleWinner(ctx, matchID, winner); err != nil {
				return err
			}

			state.SetPhase(match.PhaseSettled)
			state.SetWinner(winner)

			// Report settlement to server (winner gets ~90% of pool)
			if s.reporter != nil {
				prizeMON := "0"
				if state.Pool != nil && state.Pool.Sign() > 0 {
					prize := new(big.Int).Mul(state.Pool, big.NewInt(90))
					prize.Div(prize, big.NewInt(100))
					prizeMON = prize.String()
				}
				go s.reporter.ReportSettlement(context.Background(), reporter.SettleMatchRequest{
					MatchID:    matchID.Int64(),
					WinnerAddr: winner.Hex(),
					PrizeMON:   prizeMON,
				})
			}

			// Reveal answer
			s.revealAnswer(ctx, matchID, state)

			// Clean up and create next match
			s.verificationMgr.RemoveQueue(matchID)
			s.createNextMatchAfterCooldown()

			return nil
		}

		// No winner determined (no submissions or all zero scores)
		log.Printf("Match %s: no winner determined, issuing refund", matchID)
		return s.refundMatch(ctx, matchID, state)
	}

	// Legacy (no personalities): settle with first submitter if any answers exist
	if first, ok := s.verificationMgr.GetFirstSubmitter(matchID); ok {
		log.Printf("Match %s: no personalities, settling with first submitter %s", matchID, first.Agent.Hex())

		if err := s.chainClient.SettleWinner(ctx, matchID, first.Agent); err != nil {
			return err
		}

		state.SetPhase(match.PhaseSettled)
		state.SetWinner(first.Agent)

		// Report settlement to server (winner gets ~90% of pool)
		if s.reporter != nil {
			prizeMON := "0"
			if state.Pool != nil && state.Pool.Sign() > 0 {
				prize := new(big.Int).Mul(state.Pool, big.NewInt(90))
				prize.Div(prize, big.NewInt(100))
				prizeMON = prize.String()
			}
			go s.reporter.ReportSettlement(context.Background(), reporter.SettleMatchRequest{
				MatchID:    matchID.Int64(),
				WinnerAddr: first.Agent.Hex(),
				PrizeMON:   prizeMON,
			})
		}

		s.revealAnswer(ctx, matchID, state)
		s.verificationMgr.RemoveQueue(matchID)
		s.createNextMatchAfterCooldown()
		return nil
	}

	// No answers at all — refund
	log.Printf("Match %s timed out with no answers, issuing refund", matchID)
	return s.refundMatch(ctx, matchID, state)
}

// refundMatch issues a refund for a match.
func (s *Server) refundMatch(ctx context.Context, matchID *big.Int, state *match.State) error {
	if err := s.chainClient.RefundMatch(ctx, matchID); err != nil {
		return err
	}

	state.SetPhase(match.PhaseRefunded)
	s.verificationMgr.RemoveQueue(matchID)
	s.createNextMatchAfterCooldown()

	return nil
}

// revealAnswer reveals the answer after settlement.
func (s *Server) revealAnswer(ctx context.Context, matchID *big.Int, state *match.State) {
	q := state.GetQuestion()
	if q != nil && q.Answer != "" {
		if err := s.chainClient.RevealAnswer(ctx, matchID, q.Answer, q.Salt); err != nil {
			log.Printf("Warning: failed to reveal answer for match %s: %v", matchID, err)
		}
		// Report answer reveal to server
		if s.reporter != nil {
			go s.reporter.ReportAnswerRevealed(context.Background(), matchID.String(), q.Answer, fmt.Sprintf("%x", q.Salt))
		}
	} else {
		// Get answer from philosopher
		reveal, err := s.judgeCoordinator.RequestReveal(ctx, matchID.String())
		if err != nil {
			log.Printf("Warning: failed to get reveal from philosopher: %v", err)
		} else {
			var salt [32]byte
			copy(salt[:], common.FromHex(reveal.Salt))
			if err := s.chainClient.RevealAnswer(ctx, matchID, reveal.Answer, salt); err != nil {
				log.Printf("Warning: failed to reveal answer for match %s: %v", matchID, err)
			}
			// Report answer reveal to server
			if s.reporter != nil {
				go s.reporter.ReportAnswerRevealed(context.Background(), matchID.String(), reveal.Answer, reveal.Salt)
			}
		}
	}
}

// createNextMatchAfterCooldown creates a new match after cooldown if auto-create is enabled.
func (s *Server) createNextMatchAfterCooldown() {
	if s.cfg.Match.AutoCreate {
		go func() {
			time.Sleep(s.cfg.Match.Cooldown)
			if err := s.CreateNextMatch(context.Background()); err != nil {
				log.Printf("Failed to create next match: %v", err)
			}
		}()
	}
}

// startMatchAndGenerateQuestion starts a match on-chain, waits for confirmation, and generates a question.
// It includes an idempotency guard to prevent concurrent start attempts for the same match.
func (s *Server) startMatchAndGenerateQuestion(ctx context.Context, matchID *big.Int, state *match.State) error {
	// Idempotency guard: prevent concurrent start attempts for the same match.
	matchKey := matchID.String()
	s.startingMu.Lock()
	if s.startingMatches[matchKey] {
		s.startingMu.Unlock()
		log.Printf("Match %s: skipping duplicate startMatchAndGenerateQuestion (already in progress)", matchID)
		return nil
	}
	s.startingMatches[matchKey] = true
	s.startingMu.Unlock()
	defer func() {
		s.startingMu.Lock()
		delete(s.startingMatches, matchKey)
		s.startingMu.Unlock()
	}()

	// Double-check phase under the guard — another goroutine may have already started this match.
	if state.GetPhase() != match.PhaseQueue {
		log.Printf("Match %s: skipping start (already in phase %s)", matchID, state.GetPhase())
		return nil
	}

	// Start match on chain
	if err := s.chainClient.StartMatch(ctx, matchID); err != nil {
		return fmt.Errorf("start match on chain: %w", err)
	}
	state.SetPhase(match.PhaseActive)

	// Wait for chain state to propagate
	log.Printf("Match %s: waiting for QuestionRevealed phase confirmation on chain", matchID)
	if err := s.chainClient.WaitForPhase(ctx, matchID, 1, 30*time.Second); err != nil {
		log.Printf("Match %s: chain phase confirmation warning: %v (proceeding anyway)", matchID, err)
	}

	// Generate and post question
	if err := s.generateAndPostQuestion(ctx, state); err != nil {
		return fmt.Errorf("generate question: %w", err)
	}

	return nil
}

// onStaleQueue is called when a stale queue is detected.
func (s *Server) onStaleQueue(ctx context.Context, matchID *big.Int, action watcher.StaleAction) error {
	switch action {
	case watcher.ActionCancel:
		// Check if already cancelled/settled
		if state, err := s.matchManager.GetMatch(matchID); err == nil && state.Phase != match.PhaseQueue {
			log.Printf("Match %s: skipping stale cancel (already in phase %d)", matchID, state.Phase)
			return nil
		}
		// Capture player count before cancel
		playerCount := 0
		if state, err := s.matchManager.GetMatch(matchID); err == nil {
			playerCount = state.PlayerCount
		}
		log.Printf("Cancelling stale match %s (%d players)", matchID, playerCount)
		if err := s.chainClient.CancelMatch(ctx, matchID); err != nil {
			return err
		}
		if state, err := s.matchManager.GetMatch(matchID); err == nil {
			state.SetPhase(match.PhaseCancelled)
		}

		// Only auto-create if there were players (they deserve a fresh match).
		// If 0 players, don't waste gas — next match created on server restart
		// or after a real match completes.
		if s.cfg.Match.AutoCreate && playerCount > 0 {
			go func() {
				time.Sleep(s.cfg.Match.Cooldown)
				if err := s.CreateNextMatch(context.Background()); err != nil {
					log.Printf("Failed to create next match: %v", err)
				}
			}()
		} else if s.cfg.Match.AutoCreate {
			log.Println("Stale match had 0 players — skipping auto-create (waiting for activity)")
		}

	case watcher.ActionStartMatch:
		// Check if already started (handler or another stale check may have done it)
		if state, err := s.matchManager.GetMatch(matchID); err == nil && state.Phase != match.PhaseQueue {
			log.Printf("Match %s: skipping stale start (already in phase %d)", matchID, state.Phase)
			return nil
		}
		log.Printf("Starting stale match %s", matchID)
		if state, err := s.matchManager.GetMatch(matchID); err == nil {
			go func() {
				bgCtx, bgCancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer bgCancel()
				if err := s.startMatchAndGenerateQuestion(bgCtx, matchID, state); err != nil {
					log.Printf("Failed to start/generate question for match %s: %v", matchID, err)
				}
			}()
		}
	}

	return nil
}

// onChainEvent is called when a chain event is detected.
func (s *Server) onChainEvent(ctx context.Context, event *watcher.ChainEvent) {
	log.Printf("Chain event: %s for match %s (block %d)", event.Type, event.MatchID, event.BlockNumber)

	switch event.Type {
	case watcher.EventMatchCreated:
		// Register new match in manager if not already known (e.g. created externally)
		if _, err := s.matchManager.GetMatch(event.MatchID); err != nil {
			queueDeadline := time.Time{}
			if dl, ok := event.Data["queueDeadline"].(*big.Int); ok && dl.Sign() > 0 {
				queueDeadline = time.Unix(dl.Int64(), 0)
			}
			if state, err := s.matchManager.CreateMatch(event.MatchID, queueDeadline); err == nil {
				log.Printf("Match %s: registered from chain event (deadline: %s)", event.MatchID, queueDeadline)
				_ = state
			} else {
				log.Printf("Match %s: failed to register from chain event: %v", event.MatchID, err)
			}
		}
	case watcher.EventAgentJoinedQueue:
		// Register match if not known yet, then update player count
		if _, err := s.matchManager.GetMatch(event.MatchID); err != nil {
			// Match not known — try to register it with zero deadline (stale watcher will handle)
			s.matchManager.CreateMatch(event.MatchID, time.Time{})
			log.Printf("Match %s: late-registered from AgentJoinedQueue event", event.MatchID)
		}
		if state, err := s.matchManager.GetMatch(event.MatchID); err == nil {
			state.PlayerCount++
			log.Printf("Match %s: player joined, count now %d", event.MatchID, state.PlayerCount)

			// Auto-start logic with registration grace period
			cfg := s.matchManager.GetConfig()
			if state.GetPhase() == match.PhaseQueue && state.PlayerCount >= int(cfg.MinPlayers) {
				if state.PlayerCount >= int(cfg.MaxPlayers) {
					// MaxPlayers reached — start immediately (cancel grace timer if running)
					log.Printf("Match %s: maxPlayers reached (%d), starting immediately", event.MatchID, state.PlayerCount)
					if state.GraceTimerStarted {
						// Signal the grace timer goroutine to trigger early start
						select {
						case <-state.GraceDone:
							// Already closed — start is already in progress
							return
						default:
							close(state.GraceDone)
							// The grace timer goroutine will handle the start
							return
						}
					}
					// No grace timer was running — start directly
					matchID := new(big.Int).Set(event.MatchID)
					go func() {
						bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
						defer cancel()
						if err := s.startMatchAndGenerateQuestion(bgCtx, matchID, state); err != nil {
							log.Printf("Match %s: failed to auto-start: %v", matchID, err)
						}
					}()
				} else if !state.GraceTimerStarted {
					// MinPlayers reached but not maxPlayers — start grace period
					state.GraceTimerStarted = true
					grace := cfg.RegistrationGrace
					if grace <= 0 {
						grace = 30 * time.Second
					}
					log.Printf("Match %s: minPlayers reached (%d/%d), registration closing in %s",
						event.MatchID, state.PlayerCount, cfg.MaxPlayers, grace)

					// Announce via commentary that registration is closing soon
					s.reporter.ReportCommentary(context.Background(), reporter.CommentaryRequest{
						MatchID:   event.MatchID.Int64(),
						AgentID:   "system",
						EventType: "registration_closing",
						Text: fmt.Sprintf("Minimum players reached (%d/%d). Registration closes in %s — more players can still join (max %d).",
							state.PlayerCount, cfg.MinPlayers, grace, cfg.MaxPlayers),
					})

					// Start grace timer goroutine
					matchID := new(big.Int).Set(event.MatchID)
					go func() {
						timer := time.NewTimer(grace)
						defer timer.Stop()
						select {
						case <-timer.C:
							// Grace period expired — start the match
							log.Printf("Match %s: registration grace period expired, starting match with %d players",
								matchID, state.PlayerCount)
						case <-state.GraceDone:
							// MaxPlayers reached during grace — start immediately
							log.Printf("Match %s: maxPlayers reached during grace period, starting match immediately",
								matchID)
						case <-s.shutdownCh:
							log.Printf("Match %s: grace timer cancelled (server shutting down)", matchID)
							return
						}
						bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
						defer cancel()
						if err := s.startMatchAndGenerateQuestion(bgCtx, matchID, state); err != nil {
							log.Printf("Match %s: failed to auto-start after grace: %v", matchID, err)
						}
					}()
				} else {
					// Grace timer already running, new player joined during grace period
					log.Printf("Match %s: player joined during grace period, count now %d/%d",
						event.MatchID, state.PlayerCount, cfg.MaxPlayers)
				}
			}
		}
	case watcher.EventAnswerSubmitted:
		s.handleAnswerSubmittedEvent(ctx, event)
	case watcher.EventAnswerPeriodStarted:
		s.handleAnswerPeriodStartedEvent(ctx, event)
	case watcher.EventMatchCancelled:
		if state, err := s.matchManager.GetMatch(event.MatchID); err == nil {
			state.SetPhase(match.PhaseCancelled)
			log.Printf("Match %s: cancelled (chain event)", event.MatchID)
		}
	case watcher.EventMatchSettled:
		if state, err := s.matchManager.GetMatch(event.MatchID); err == nil {
			prevPhase := state.GetPhase()
			state.SetPhase(match.PhaseSettled)
			log.Printf("Match %s: phase %s -> settled (chain event)", event.MatchID, prevPhase)
		} else {
			log.Printf("Match %s: settled (chain event) but match not tracked locally", event.MatchID)
		}
	case watcher.EventMatchRefunded:
		if state, err := s.matchManager.GetMatch(event.MatchID); err == nil {
			prevPhase := state.GetPhase()
			state.SetPhase(match.PhaseRefunded)
			log.Printf("Match %s: phase %s -> refunded (chain event)", event.MatchID, prevPhase)
		} else {
			log.Printf("Match %s: refunded (chain event) but match not tracked locally", event.MatchID)
		}
	}
}

// handleAnswerSubmittedEvent handles an AnswerSubmitted event.
func (s *Server) handleAnswerSubmittedEvent(ctx context.Context, event *watcher.ChainEvent) {
	agent, ok := event.Data["agent"].(common.Address)
	if !ok {
		log.Printf("Warning: agent not found in AnswerSubmitted event")
		return
	}

	answerText, _ := event.Data["answer"].(string)
	attemptNum, _ := event.Data["attemptNumber"].(*big.Int)
	neuronBurned, _ := event.Data["neuronBurned"].(*big.Int)

	var attempt uint64
	if attemptNum != nil {
		attempt = attemptNum.Uint64()
	}

	answer := &verification.SubmittedAnswer{
		MatchID:       event.MatchID,
		Agent:         agent,
		Answer:        answerText,
		BlockNumber:   event.BlockNumber,
		TxIndex:       event.TxIndex,
		AttemptNumber: attempt,
		NeuronBurned:  neuronBurned,
		Timestamp:     time.Now(),
	}

	s.verificationMgr.EnqueueAnswer(answer)

	// Answer queued — settlement handled by timeout or personality evaluation
	log.Printf("Match %s: answer from %s queued via chain event", event.MatchID, agent.Hex())
}

// handleAnswerPeriodStartedEvent handles an AnswerPeriodStarted event.
func (s *Server) handleAnswerPeriodStartedEvent(ctx context.Context, event *watcher.ChainEvent) {
	state, err := s.matchManager.GetMatch(event.MatchID)
	if err != nil {
		log.Printf("Warning: match %s not found for AnswerPeriodStarted", event.MatchID)
		return
	}

	prevPhase := state.GetPhase()

	// Get deadline from chain
	chainState, err := s.chainClient.GetMatchState(ctx, event.MatchID)
	if err != nil {
		log.Printf("Warning: failed to get match state: %v", err)
		return
	}

	deadline := time.Unix(int64(chainState.AnswerDeadline), 0)
	state.SetAnswerDeadline(deadline)
	state.SetPhase(match.PhaseAnswerPeriod)

	log.Printf("Match %s: phase %s -> answer_period (deadline: %s)", event.MatchID, prevPhase, deadline.Format(time.RFC3339))

	// Register timeout
	s.timeoutWatcher.RegisterMatch(event.MatchID, deadline)
}
