package question

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/axon-arena/axon-agent/internal/llm"
)

// BufferedQuestion holds a pre-generated question without match-specific data.
type BufferedQuestion struct {
	Question   string
	Answer     string
	Variants   []string
	Format     string
	Category   string
	Difficulty int
}

// QuestionBuffer pre-generates questions in the background so they're available instantly.
type QuestionBuffer struct {
	mu          sync.Mutex
	buffer      map[string][]BufferedQuestion // category -> FIFO queue
	bufferSize  int                           // target per category
	categories  []string
	llmClient   llm.Provider
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	replenishCh chan string // category to replenish
}

// NewQuestionBuffer creates a new question buffer.
func NewQuestionBuffer(ctx context.Context, llmClient llm.Provider, categories []string, bufferSize int) *QuestionBuffer {
	ctx, cancel := context.WithCancel(ctx)
	return &QuestionBuffer{
		buffer:      make(map[string][]BufferedQuestion),
		bufferSize:  bufferSize,
		categories:  categories,
		llmClient:   llmClient,
		ctx:         ctx,
		cancel:      cancel,
		replenishCh: make(chan string, 16),
	}
}

// Start launches the background replenish goroutine and does the initial fill.
func (qb *QuestionBuffer) Start() {
	qb.wg.Add(1)
	go qb.replenishLoop()
}

// Stop cancels the context and waits for the background goroutine to finish.
func (qb *QuestionBuffer) Stop() {
	qb.cancel()
	qb.wg.Wait()
}

// Pop removes and returns the first buffered question for the given category.
// Returns false if the buffer is empty for that category.
func (qb *QuestionBuffer) Pop(category string) (*BufferedQuestion, bool) {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	items := qb.buffer[category]
	if len(items) == 0 {
		return nil, false
	}

	q := items[0]
	qb.buffer[category] = items[1:]

	// Async replenish signal (non-blocking)
	select {
	case qb.replenishCh <- category:
	default:
	}

	return &q, true
}

// Sizes returns the current buffer size per category.
func (qb *QuestionBuffer) Sizes() map[string]int {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	sizes := make(map[string]int, len(qb.categories))
	for _, cat := range qb.categories {
		sizes[cat] = len(qb.buffer[cat])
	}
	return sizes
}

func (qb *QuestionBuffer) replenishLoop() {
	defer qb.wg.Done()

	// Initial fill for all categories
	for _, cat := range qb.categories {
		if qb.ctx.Err() != nil {
			return
		}
		qb.fillCategory(cat)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-qb.ctx.Done():
			return
		case cat := <-qb.replenishCh:
			qb.fillCategory(cat)
		case <-ticker.C:
			for _, cat := range qb.categories {
				if qb.ctx.Err() != nil {
					return
				}
				qb.fillCategory(cat)
			}
		}
	}
}

func (qb *QuestionBuffer) fillCategory(category string) {
	for {
		if qb.ctx.Err() != nil {
			return
		}

		qb.mu.Lock()
		current := len(qb.buffer[category])
		qb.mu.Unlock()

		if current >= qb.bufferSize {
			return
		}

		q, err := qb.generateOne(category)
		if err != nil {
			log.Printf("[buffer] failed to generate question for %s: %v", category, err)
			return
		}

		qb.mu.Lock()
		qb.buffer[category] = append(qb.buffer[category], *q)
		newSize := len(qb.buffer[category])
		qb.mu.Unlock()

		log.Printf("[buffer] %s: %d/%d questions", category, newSize, qb.bufferSize)

		// Delay between LLM calls to avoid rate limiting
		select {
		case <-qb.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (qb *QuestionBuffer) generateOne(category string) (*BufferedQuestion, error) {
	// Random difficulty weighted toward 2-3
	difficulty := WeightedDifficulty()

	llmResp, err := GenerateQuestionRaw(qb.ctx, qb.llmClient, category, difficulty)
	if err != nil {
		return nil, err
	}

	return &BufferedQuestion{
		Question:   llmResp.Question,
		Answer:     llmResp.Answer,
		Variants:   llmResp.Variants,
		Format:     llmResp.Format,
		Category:   category,
		Difficulty: difficulty,
	}, nil
}

// Categories returns the configured category list.
func (qb *QuestionBuffer) Categories() []string {
	return qb.categories
}

// WeightedDifficulty returns a random difficulty weighted toward 2-3.
func WeightedDifficulty() int {
	// Weights: 1=10%, 2=35%, 3=35%, 4=15%, 5=5%
	r := rand.Intn(100)
	switch {
	case r < 10:
		return 1
	case r < 45:
		return 2
	case r < 80:
		return 3
	case r < 95:
		return 4
	default:
		return 5
	}
}
