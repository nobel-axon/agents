package personality

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"
)

// PersonalityTriplet holds 3 pre-generated personalities.
type PersonalityTriplet struct {
	Personalities []*Personality
	Category      string
	GeneratedAt   time.Time
}

// PreStocker pre-generates personality triplets in the background.
type PreStocker struct {
	mu          sync.Mutex
	triplets    []PersonalityTriplet // FIFO queue
	targetSize  int
	categories  []string
	factory     *Factory
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	replenishCh chan struct{}
}

// NewPreStocker creates a new personality pre-stocker.
func NewPreStocker(ctx context.Context, factory *Factory, categories []string, targetSize int) *PreStocker {
	ctx, cancel := context.WithCancel(ctx)
	return &PreStocker{
		triplets:    make([]PersonalityTriplet, 0, targetSize),
		targetSize:  targetSize,
		categories:  categories,
		factory:     factory,
		ctx:         ctx,
		cancel:      cancel,
		replenishCh: make(chan struct{}, 16),
	}
}

// Start launches the background replenish goroutine.
func (ps *PreStocker) Start() {
	ps.wg.Add(1)
	go ps.replenishLoop()
}

// Stop cancels the context and waits for the background goroutine to finish.
func (ps *PreStocker) Stop() {
	ps.cancel()
	ps.wg.Wait()
}

// PopTriplet removes and returns a triplet. Prefers matching category, falls back to any.
func (ps *PreStocker) PopTriplet(category string) (*PersonalityTriplet, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.triplets) == 0 {
		return nil, false
	}

	// Try to find matching category first
	for i, t := range ps.triplets {
		if t.Category == category {
			ps.triplets = append(ps.triplets[:i], ps.triplets[i+1:]...)
			// Async replenish (non-blocking)
			select {
			case ps.replenishCh <- struct{}{}:
			default:
			}
			return &t, true
		}
	}

	// Fall back to any available triplet
	t := ps.triplets[0]
	ps.triplets = ps.triplets[1:]

	select {
	case ps.replenishCh <- struct{}{}:
	default:
	}

	return &t, true
}

// Size returns the current pool size.
func (ps *PreStocker) Size() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.triplets)
}

func (ps *PreStocker) replenishLoop() {
	defer ps.wg.Done()

	// Initial fill
	ps.fill()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ps.ctx.Done():
			return
		case <-ps.replenishCh:
			ps.fill()
		case <-ticker.C:
			ps.fill()
		}
	}
}

func (ps *PreStocker) fill() {
	for {
		if ps.ctx.Err() != nil {
			return
		}

		ps.mu.Lock()
		current := len(ps.triplets)
		ps.mu.Unlock()

		if current >= ps.targetSize {
			return
		}

		// Pick a random category
		category := ps.categories[rand.Intn(len(ps.categories))]

		triplet, err := ps.generateTriplet(category)
		if err != nil {
			log.Printf("[prestocker] failed to generate triplet for %s: %v", category, err)
			return
		}

		ps.mu.Lock()
		ps.triplets = append(ps.triplets, *triplet)
		newSize := len(ps.triplets)
		ps.mu.Unlock()

		log.Printf("[prestocker] pool: %d/%d triplets (added %s)", newSize, ps.targetSize, category)

		// Delay between LLM calls
		select {
		case <-ps.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (ps *PreStocker) generateTriplet(category string) (*PersonalityTriplet, error) {
	personalities, err := ps.factory.GenerateBatchDiverse(ps.ctx, 3, category)
	if err != nil {
		return nil, err
	}

	return &PersonalityTriplet{
		Personalities: personalities,
		Category:      category,
		GeneratedAt:   time.Now(),
	}, nil
}
