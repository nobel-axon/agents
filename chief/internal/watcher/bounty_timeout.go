// Package watcher provides background watchers for match lifecycle events.
package watcher

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/axon-arena/axon-chief/internal/bounty"
)

// BountyTimeoutHandler is called when a bounty deadline expires.
type BountyTimeoutHandler func(ctx context.Context, bountyID *big.Int) error

// BountyDeadlineWatcher monitors active bounties for deadline expiration.
type BountyDeadlineWatcher struct {
	bountyManager *bounty.Manager
	handler       BountyTimeoutHandler
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewBountyDeadlineWatcher creates a new bounty deadline watcher.
func NewBountyDeadlineWatcher(bountyManager *bounty.Manager, handler BountyTimeoutHandler, checkInterval time.Duration) *BountyDeadlineWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &BountyDeadlineWatcher{
		bountyManager: bountyManager,
		handler:       handler,
		checkInterval: checkInterval,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start starts the bounty deadline watcher background loop.
func (bw *BountyDeadlineWatcher) Start() {
	go bw.backgroundLoop()
}

// Stop stops the bounty deadline watcher.
func (bw *BountyDeadlineWatcher) Stop() {
	bw.cancel()
}

// backgroundLoop periodically checks active bounties for expired deadlines.
func (bw *BountyDeadlineWatcher) backgroundLoop() {
	ticker := time.NewTicker(bw.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bw.ctx.Done():
			return
		case <-ticker.C:
			bw.checkDeadlines()
		}
	}
}

// checkDeadlines checks all active bounties for expired deadlines.
func (bw *BountyDeadlineWatcher) checkDeadlines() {
	active := bw.bountyManager.GetActiveBounties()
	now := time.Now()

	for _, b := range active {
		if b.Deadline.IsZero() {
			continue
		}
		if now.After(b.Deadline) {
			bountyID := new(big.Int).Set(b.ID)
			log.Printf("Bounty %s: deadline expired, triggering settlement", bountyID)

			ctx, cancel := context.WithTimeout(bw.ctx, 2*time.Minute)
			if err := bw.handler(ctx, bountyID); err != nil {
				log.Printf("Bounty %s: settlement failed: %v", bountyID, err)
			}
			cancel()
		}
	}
}
