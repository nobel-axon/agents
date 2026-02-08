// Package watcher provides background watchers for match lifecycle events.
package watcher

import (
	"context"
	"errors"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// EventType represents the type of an event.
type EventType string

const (
	EventMatchCreated        EventType = "MatchCreated"
	EventAgentJoinedQueue    EventType = "AgentJoinedQueue"
	EventMatchStarted        EventType = "MatchStarted"
	EventQuestionRevealed    EventType = "QuestionRevealed"
	EventAnswerPeriodStarted EventType = "AnswerPeriodStarted"
	EventAnswerSubmitted     EventType = "AnswerSubmitted"
	EventMatchSettled        EventType = "MatchSettled"
	EventMatchRefunded       EventType = "MatchRefunded"
	EventMatchCancelled      EventType = "MatchCancelled"
)

// ChainEvent represents a parsed chain event.
type ChainEvent struct {
	Type        EventType
	MatchID     *big.Int
	BlockNumber uint64
	TxHash      common.Hash
	TxIndex     uint
	LogIndex    uint
	Data        map[string]interface{}
}

// EventHandler is called when an event is detected.
type EventHandler func(ctx context.Context, event *ChainEvent)

// maxBlockRange is the maximum number of blocks to query in a single eth_getLogs call.
// Monad RPC enforces a 100-block limit on eth_getLogs range queries.
const maxBlockRange = 100

// EventDetector monitors the chain for events using dual sources.
type EventDetector struct {
	mu sync.RWMutex

	client        *ethclient.Client
	arenaAddress  common.Address
	handler       EventHandler
	pollInterval  time.Duration
	lastBlock     uint64
	seenEvents    map[string]bool // txHash:logIndex -> seen
	ponderHealthy bool
	ponderLagStart time.Time
	lagThreshold  time.Duration
	lagBlocks     int

	ctx    context.Context
	cancel context.CancelFunc
}

// NewEventDetector creates a new event detector.
func NewEventDetector(
	client *ethclient.Client,
	arenaAddress common.Address,
	handler EventHandler,
	pollInterval time.Duration,
	lagThreshold time.Duration,
	lagBlocks int,
) *EventDetector {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventDetector{
		client:        client,
		arenaAddress:  arenaAddress,
		handler:       handler,
		pollInterval:  pollInterval,
		seenEvents:    make(map[string]bool),
		ponderHealthy: true,
		lagThreshold:  lagThreshold,
		lagBlocks:     lagBlocks,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start starts the event detector background loop.
func (ed *EventDetector) Start() error {
	// Get current block number
	block, err := ed.client.BlockNumber(ed.ctx)
	if err != nil {
		return err
	}
	ed.lastBlock = block

	go ed.backgroundLoop()
	return nil
}

// Stop stops the event detector.
func (ed *EventDetector) Stop() {
	ed.cancel()
}

// SetPonderHealth updates the Ponder health status.
// When Ponder is unhealthy for too long, the detector falls back to direct chain polling.
func (ed *EventDetector) SetPonderHealth(healthy bool, currentBlock uint64, ponderBlock uint64) {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	lag := int(currentBlock) - int(ponderBlock)
	if lag < 0 {
		lag = 0
	}

	if healthy && lag <= ed.lagBlocks {
		ed.ponderHealthy = true
		ed.ponderLagStart = time.Time{}
	} else {
		if ed.ponderHealthy {
			// Just became unhealthy
			ed.ponderLagStart = time.Now()
		} else if time.Since(ed.ponderLagStart) > ed.lagThreshold {
			// Been unhealthy for too long - switch to direct chain
			ed.ponderHealthy = false
			log.Printf("Ponder lag exceeded threshold (%d blocks for %v), using direct chain polling",
				lag, time.Since(ed.ponderLagStart))
		}
	}
}

// IsPonderHealthy returns whether Ponder is healthy.
func (ed *EventDetector) IsPonderHealthy() bool {
	ed.mu.RLock()
	defer ed.mu.RUnlock()
	return ed.ponderHealthy
}

// backgroundLoop periodically polls for events.
func (ed *EventDetector) backgroundLoop() {
	ticker := time.NewTicker(ed.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ed.ctx.Done():
			return
		case <-ticker.C:
			ed.pollEvents()
		}
	}
}

// pollEvents polls for new events from the chain.
// Block ranges are chunked to maxBlockRange (100) to stay within Monad RPC limits.
func (ed *EventDetector) pollEvents() {
	ed.mu.Lock()
	fromBlock := ed.lastBlock + 1
	ed.mu.Unlock()

	// Get current block
	currentBlock, err := ed.client.BlockNumber(ed.ctx)
	if err != nil {
		log.Printf("Failed to get block number: %v", err)
		return
	}

	if fromBlock > currentBlock {
		return
	}

	// Process in chunks of maxBlockRange to respect Monad RPC limits
	for chunkStart := fromBlock; chunkStart <= currentBlock; chunkStart += maxBlockRange + 1 {
		chunkEnd := chunkStart + maxBlockRange
		if chunkEnd > currentBlock {
			chunkEnd = currentBlock
		}

		// Query logs for this chunk
		query := ethereum.FilterQuery{
			FromBlock: big.NewInt(int64(chunkStart)),
			ToBlock:   big.NewInt(int64(chunkEnd)),
			Addresses: []common.Address{ed.arenaAddress},
		}

		logs, err := ed.client.FilterLogs(ed.ctx, query)
		if err != nil {
			log.Printf("Failed to filter logs (blocks %d-%d): %v", chunkStart, chunkEnd, err)
			return // Stop processing chunks on error; will retry from lastBlock+1 next tick
		}

		// Process logs
		for _, vLog := range logs {
			event := ed.parseLog(vLog)
			if event == nil {
				continue
			}

			// Check if already seen
			key := eventKey(vLog.TxHash, vLog.Index)
			ed.mu.Lock()
			if ed.seenEvents[key] {
				ed.mu.Unlock()
				continue
			}
			ed.seenEvents[key] = true
			ed.mu.Unlock()

			// Call handler
			ed.handler(ed.ctx, event)
		}

		// Update last block after each successful chunk
		ed.mu.Lock()
		ed.lastBlock = chunkEnd
		ed.mu.Unlock()
	}

	// Prune old seen events (keep last 1000)
	ed.mu.Lock()
	if len(ed.seenEvents) > 1000 {
		ed.seenEvents = make(map[string]bool)
	}
	ed.mu.Unlock()
}

// parseLog parses a log into a ChainEvent.
func (ed *EventDetector) parseLog(vLog types.Log) *ChainEvent {
	if len(vLog.Topics) == 0 {
		return nil
	}

	topic := vLog.Topics[0]
	event := &ChainEvent{
		BlockNumber: vLog.BlockNumber,
		TxHash:      vLog.TxHash,
		TxIndex:     vLog.TxIndex,
		LogIndex:    vLog.Index,
		Data:        make(map[string]interface{}),
	}

	// Event signatures
	matchCreatedSig := crypto.Keccak256Hash([]byte("MatchCreated(uint256,uint256,uint256,uint64,uint8,uint8)"))
	agentJoinedSig := crypto.Keccak256Hash([]byte("AgentJoinedQueue(uint256,address,uint256,uint256)"))
	matchStartedSig := crypto.Keccak256Hash([]byte("MatchStarted(uint256,uint256,uint256)"))
	questionRevealedSig := crypto.Keccak256Hash([]byte("QuestionRevealed(uint256,string,string,uint8,string,bytes32)"))
	answerPeriodSig := crypto.Keccak256Hash([]byte("AnswerPeriodStarted(uint256,uint256,uint256)"))
	answerSubmittedSig := crypto.Keccak256Hash([]byte("AnswerSubmitted(uint256,address,string,uint256,uint256)"))
	matchSettledSig := crypto.Keccak256Hash([]byte("MatchSettled(uint256,address,uint256,uint256,uint256)"))
	matchRefundedSig := crypto.Keccak256Hash([]byte("MatchRefunded(uint256,uint256,uint256)"))
	matchCancelledSig := crypto.Keccak256Hash([]byte("MatchCancelled(uint256)"))

	switch topic {
	case matchCreatedSig:
		event.Type = EventMatchCreated
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}
		// Parse non-indexed data: entryFee(uint256), baseAnswerFee(uint256), queueDeadline(uint64), minPlayers(uint8), maxPlayers(uint8)
		if len(vLog.Data) >= 160 {
			queueDeadline := new(big.Int).SetBytes(vLog.Data[64:96]) // 3rd field (after entryFee, baseAnswerFee)
			event.Data["queueDeadline"] = queueDeadline
		}

	case agentJoinedSig:
		event.Type = EventAgentJoinedQueue
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}
		if len(vLog.Topics) > 2 {
			event.Data["agent"] = common.BytesToAddress(vLog.Topics[2].Bytes())
		}

	case matchStartedSig:
		event.Type = EventMatchStarted
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}

	case questionRevealedSig:
		event.Type = EventQuestionRevealed
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}

	case answerPeriodSig:
		event.Type = EventAnswerPeriodStarted
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}

	case answerSubmittedSig:
		event.Type = EventAnswerSubmitted
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}
		if len(vLog.Topics) > 2 {
			event.Data["agent"] = common.BytesToAddress(vLog.Topics[2].Bytes())
		}
		// Decode non-indexed fields: answer (string), attemptNumber (uint256), neuronBurned (uint256)
		if len(vLog.Data) > 0 {
			stringT, _ := abi.NewType("string", "", nil)
			uint256T, _ := abi.NewType("uint256", "", nil)
			args := abi.Arguments{
				{Type: stringT},
				{Type: uint256T},
				{Type: uint256T},
			}
			values, err := args.Unpack(vLog.Data)
			if err == nil && len(values) >= 3 {
				event.Data["answer"] = values[0].(string)
				event.Data["attemptNumber"] = values[1].(*big.Int)
				event.Data["neuronBurned"] = values[2].(*big.Int)
			} else if err != nil {
				log.Printf("Warning: failed to decode AnswerSubmitted data: %v", err)
			}
		}

	case matchSettledSig:
		event.Type = EventMatchSettled
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}
		if len(vLog.Topics) > 2 {
			event.Data["winner"] = common.BytesToAddress(vLog.Topics[2].Bytes())
		}

	case matchRefundedSig:
		event.Type = EventMatchRefunded
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}

	case matchCancelledSig:
		event.Type = EventMatchCancelled
		if len(vLog.Topics) > 1 {
			event.MatchID = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
		}

	default:
		return nil
	}

	return event
}

func eventKey(txHash common.Hash, logIndex uint) string {
	return txHash.Hex() + ":" + string(rune(logIndex))
}

// DeduplicateEvent checks if an event has been seen before.
func (ed *EventDetector) DeduplicateEvent(txHash common.Hash, logIndex uint) bool {
	key := eventKey(txHash, logIndex)
	ed.mu.Lock()
	defer ed.mu.Unlock()

	if ed.seenEvents[key] {
		return true
	}
	ed.seenEvents[key] = true
	return false
}

// ReportPonderEvent reports an event from Ponder for deduplication.
func (ed *EventDetector) ReportPonderEvent(txHash common.Hash, logIndex uint) error {
	if ed.DeduplicateEvent(txHash, logIndex) {
		return errors.New("duplicate event")
	}
	return nil
}
