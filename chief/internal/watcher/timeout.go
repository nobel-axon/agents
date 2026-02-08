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

// TimeoutHandler is called when a match times out.
type TimeoutHandler func(ctx context.Context, matchID *big.Int) error

// TimeoutWatcher monitors matches for answer period timeouts.
type TimeoutWatcher struct {
	mu            sync.RWMutex
	timers        map[string]*time.Timer
	matchManager  *match.Manager
	handler       TimeoutHandler
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewTimeoutWatcher creates a new timeout watcher.
func NewTimeoutWatcher(matchManager *match.Manager, handler TimeoutHandler, checkInterval time.Duration) *TimeoutWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &TimeoutWatcher{
		timers:        make(map[string]*time.Timer),
		matchManager:  matchManager,
		handler:       handler,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start starts the timeout watcher background loop.
func (tw *TimeoutWatcher) Start() {
	go tw.backgroundLoop()
}

// Stop stops the timeout watcher.
func (tw *TimeoutWatcher) Stop() {
	tw.cancel()

	tw.mu.Lock()
	defer tw.mu.Unlock()

	for _, timer := range tw.timers {
		timer.Stop()
	}
	tw.timers = make(map[string]*time.Timer)
}

// RegisterMatch registers a match with a timeout deadline.
func (tw *TimeoutWatcher) RegisterMatch(matchID *big.Int, deadline time.Time) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	key := matchID.String()

	// Cancel existing timer if any
	if timer, ok := tw.timers[key]; ok {
		timer.Stop()
	}

	// Calculate duration until deadline
	duration := time.Until(deadline)
	if duration <= 0 {
		// Already expired - handle immediately
		go tw.handleTimeout(matchID)
		return
	}

	// Create new timer
	tw.timers[key] = time.AfterFunc(duration, func() {
		tw.handleTimeout(matchID)
	})

	log.Printf("Registered timeout for match %s at %v (in %v)", key, deadline, duration)
}

// CancelMatch cancels the timeout for a match (called on settlement).
func (tw *TimeoutWatcher) CancelMatch(matchID *big.Int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	key := matchID.String()
	if timer, ok := tw.timers[key]; ok {
		timer.Stop()
		delete(tw.timers, key)
		log.Printf("Cancelled timeout for match %s", key)
	}
}

// handleTimeout handles a match timeout.
func (tw *TimeoutWatcher) handleTimeout(matchID *big.Int) {
	tw.mu.Lock()
	delete(tw.timers, matchID.String())
	tw.mu.Unlock()

	// Get match state
	state, err := tw.matchManager.GetMatch(matchID)
	if err != nil {
		log.Printf("Timeout handler: match %s not found: %v", matchID, err)
		return
	}

	// Verify match is still in answer period
	phase := state.GetPhase()
	if phase != match.PhaseAnswerPeriod {
		log.Printf("Timeout handler: match %s not in answer period (phase: %s)", matchID, phase)
		return
	}

	// Try to lock the match to prevent race with late answers
	if err := state.Lock(); err != nil {
		log.Printf("Timeout handler: match %s is locked, skipping", matchID)
		return
	}
	defer state.Unlock()

	// Double-check phase after acquiring lock
	if state.GetPhase() != match.PhaseAnswerPeriod {
		log.Printf("Timeout handler: match %s phase changed after lock", matchID)
		return
	}

	log.Printf("Match %s timed out, triggering refund", matchID)

	// Call the timeout handler
	ctx, cancel := context.WithTimeout(tw.ctx, 2*time.Minute)
	defer cancel()

	if err := tw.handler(ctx, matchID); err != nil {
		log.Printf("Timeout handler error for match %s: %v", matchID, err)
	}
}

// backgroundLoop periodically checks for missed timeouts.
func (tw *TimeoutWatcher) backgroundLoop() {
	ticker := time.NewTicker(tw.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tw.ctx.Done():
			return
		case <-ticker.C:
			tw.checkMissedTimeouts()
		}
	}
}

// checkMissedTimeouts checks for matches that should have timed out but didn't.
func (tw *TimeoutWatcher) checkMissedTimeouts() {
	// Get all matches in answer period
	matches := tw.matchManager.GetMatchesInPhase(match.PhaseAnswerPeriod)
	now := time.Now()

	for _, state := range matches {
		deadline := state.GetAnswerDeadline()
		if deadline.IsZero() {
			continue
		}

		if now.After(deadline) {
			matchID := state.ID
			tw.mu.RLock()
			_, hasTimer := tw.timers[matchID.String()]
			tw.mu.RUnlock()

			if !hasTimer {
				log.Printf("Found missed timeout for match %s, handling now", matchID)
				go tw.handleTimeout(matchID)
			}
		}
	}
}

// GetRegisteredMatches returns the IDs of matches with registered timeouts.
func (tw *TimeoutWatcher) GetRegisteredMatches() []string {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	ids := make([]string, 0, len(tw.timers))
	for id := range tw.timers {
		ids = append(ids, id)
	}
	return ids
}
