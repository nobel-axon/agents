// Package watcher provides background watchers for match lifecycle events.
package watcher

import (
	"context"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/axon-arena/axon-chief/internal/match"
)

// StaleAction represents the action to take for a stale match.
type StaleAction int

const (
	ActionNone StaleAction = iota
	ActionCancel
	ActionStartMatch
)

// StaleHandler is called when a stale match is detected.
type StaleHandler func(ctx context.Context, matchID *big.Int, action StaleAction) error

// StaleQueueWatcher monitors the queue phase for stale matches.
type StaleQueueWatcher struct {
	mu            sync.RWMutex
	matchManager  *match.Manager
	handler       StaleHandler
	checkInterval time.Duration
	staleThreshold time.Duration
	minPlayers    uint8
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewStaleQueueWatcher creates a new stale queue watcher.
func NewStaleQueueWatcher(
	matchManager *match.Manager,
	handler StaleHandler,
	checkInterval time.Duration,
	staleThreshold time.Duration,
	minPlayers uint8,
) *StaleQueueWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &StaleQueueWatcher{
		matchManager:   matchManager,
		handler:        handler,
		checkInterval:  checkInterval,
		staleThreshold: staleThreshold,
		minPlayers:     minPlayers,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start starts the stale queue watcher background loop.
func (sw *StaleQueueWatcher) Start() {
	go sw.backgroundLoop()
}

// Stop stops the stale queue watcher.
func (sw *StaleQueueWatcher) Stop() {
	sw.cancel()
}

// backgroundLoop periodically checks for stale queue matches.
func (sw *StaleQueueWatcher) backgroundLoop() {
	ticker := time.NewTicker(sw.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sw.ctx.Done():
			return
		case <-ticker.C:
			sw.checkStaleQueues()
		}
	}
}

// checkStaleQueues checks for matches stuck in queue phase past the deadline.
func (sw *StaleQueueWatcher) checkStaleQueues() {
	// Get all matches in queue phase
	matches := sw.matchManager.GetMatchesInPhase(match.PhaseQueue)
	now := time.Now()

	for _, state := range matches {
		deadline := state.QueueDeadline
		if deadline.IsZero() {
			continue
		}

		// Check if past deadline + stale threshold
		staleTime := deadline.Add(sw.staleThreshold)
		if now.Before(staleTime) {
			continue
		}

		matchID := state.ID
		playerCount := state.PlayerCount

		// Determine action based on player count
		var action StaleAction
		if playerCount < int(sw.minPlayers) {
			// Not enough players - cancel and refund
			action = ActionCancel
			log.Printf("Stale queue detected for match %s: %d players (min: %d), cancelling",
				matchID, playerCount, sw.minPlayers)
		} else {
			// Enough players but stuck - try to start
			action = ActionStartMatch
			log.Printf("Stale queue detected for match %s: %d players, attempting to start",
				matchID, playerCount)
		}

		// Try to lock the match
		if err := state.Lock(); err != nil {
			log.Printf("Stale handler: match %s is locked, skipping", matchID)
			continue
		}

		// Call the stale handler
		ctx, cancel := context.WithTimeout(sw.ctx, 2*time.Minute)
		if err := sw.handler(ctx, matchID, action); err != nil {
			log.Printf("Stale handler error for match %s: %v", matchID, err)
		}
		cancel()

		state.Unlock()
	}
}

// SetStaleThreshold updates the stale threshold.
func (sw *StaleQueueWatcher) SetStaleThreshold(threshold time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.staleThreshold = threshold
}

// SetMinPlayers updates the minimum players threshold.
func (sw *StaleQueueWatcher) SetMinPlayers(minPlayers uint8) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.minPlayers = minPlayers
}
