// Package burn provides NEURON buyback and burn functionality via nad.fun.
package burn

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/axon-arena/axon-chief/internal/config"
)

// BurnPhase represents the phase of a burn operation.
type BurnPhase string

const (
	PhasePending   BurnPhase = "pending"
	PhaseClaimed   BurnPhase = "claimed"
	PhaseSwapped   BurnPhase = "swapped"
	PhaseBurned    BurnPhase = "burned"
	PhaseFailed    BurnPhase = "failed"
	PhaseTreasury  BurnPhase = "treasury" // Fallback: sent to treasury
)

// BurnRecord tracks the state of a burn allocation operation.
type BurnRecord struct {
	MatchID       *big.Int
	Winner        common.Address
	MONAmount     *big.Int       // Original MON claimed
	NEURONAmount  *big.Int       // NEURON received from swap (if any)
	Phase         BurnPhase
	Attempts      int
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ChainOperator defines the interface for chain operations needed by BurnManager.
type ChainOperator interface {
	// ClaimBurnAllocationFor claims the burn allocation for a winner.
	// Returns the MON amount claimed.
	ClaimBurnAllocationFor(ctx context.Context, winner common.Address) (*big.Int, error)

	// SwapMONToNEURON swaps MON to NEURON via nad.fun.
	// Returns the NEURON amount received.
	SwapMONToNEURON(ctx context.Context, monAmount, minNeuronOut *big.Int) (*big.Int, error)

	// BurnNEURON burns NEURON tokens from operator wallet.
	BurnNEURON(ctx context.Context, amount *big.Int) error

	// GetSwapQuote gets the expected NEURON output for a MON input.
	GetSwapQuote(ctx context.Context, monAmount *big.Int) (*big.Int, common.Address, error)

	// SendToTreasury sends MON to treasury as fallback.
	SendToTreasury(ctx context.Context, amount *big.Int) error
}

// Manager orchestrates the burn allocation claim → swap → burn flow.
type Manager struct {
	mu sync.RWMutex

	cfg      *config.NadFunConfig
	chain    ChainOperator

	// Track pending and historical burn operations
	records map[string]*BurnRecord // key: matchID.String()

	// Failed records for manual reconciliation
	failedRecords []*BurnRecord
}

// NewManager creates a new burn manager.
func NewManager(cfg *config.NadFunConfig, chain ChainOperator) *Manager {
	return &Manager{
		cfg:           cfg,
		chain:         chain,
		records:       make(map[string]*BurnRecord),
		failedRecords: make([]*BurnRecord, 0),
	}
}

// ExecuteBurnAllocation performs the full burn allocation flow for a match winner.
// Flow: claim MON → swap MON→NEURON → burn NEURON
// If any step fails after retries, falls back to sending MON to treasury.
func (m *Manager) ExecuteBurnAllocation(ctx context.Context, matchID *big.Int, winner common.Address) error {
	record := &BurnRecord{
		MatchID:   matchID,
		Winner:    winner,
		Phase:     PhasePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.mu.Lock()
	m.records[matchID.String()] = record
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		record.UpdatedAt = time.Now()
		m.mu.Unlock()
	}()

	// Step 1: Claim burn allocation from contract
	monAmount, err := m.claimWithRetry(ctx, record, winner)
	if err != nil {
		log.Printf("Match %s: failed to claim burn allocation: %v", matchID, err)
		return m.handleFailure(ctx, record, err)
	}

	record.MONAmount = monAmount
	record.Phase = PhaseClaimed
	log.Printf("Match %s: claimed %s MON for burn", matchID, monAmount)

	// Step 2: Get swap quote and apply slippage
	minNeuronOut, err := m.getMinOutputWithSlippage(ctx, monAmount)
	if err != nil {
		log.Printf("Match %s: failed to get swap quote: %v", matchID, err)
		return m.handleSwapFailure(ctx, record, err)
	}

	// Step 3: Swap MON → NEURON
	neuronAmount, err := m.swapWithRetry(ctx, record, monAmount, minNeuronOut)
	if err != nil {
		log.Printf("Match %s: failed to swap MON→NEURON: %v", matchID, err)
		return m.handleSwapFailure(ctx, record, err)
	}

	record.NEURONAmount = neuronAmount
	record.Phase = PhaseSwapped
	log.Printf("Match %s: swapped %s MON → %s NEURON", matchID, monAmount, neuronAmount)

	// Step 4: Burn NEURON
	if err := m.burnWithRetry(ctx, record, neuronAmount); err != nil {
		log.Printf("Match %s: failed to burn NEURON (will retry manually): %v", matchID, err)
		// NEURON is in operator wallet - log for manual burn
		record.LastError = fmt.Sprintf("burn failed, NEURON in wallet: %v", err)
		m.addFailedRecord(record)
		return err
	}

	record.Phase = PhaseBurned
	log.Printf("Match %s: burned %s NEURON successfully", matchID, neuronAmount)

	return nil
}

// claimWithRetry attempts to claim burn allocation with retries.
func (m *Manager) claimWithRetry(ctx context.Context, record *BurnRecord, winner common.Address) (*big.Int, error) {
	var lastErr error
	retryDelay := time.Duration(m.cfg.BurnSwapRetryDelayMs) * time.Millisecond

	for attempt := 0; attempt < m.cfg.BurnSwapRetries; attempt++ {
		record.Attempts = attempt + 1

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
			retryDelay *= 2 // Exponential backoff
		}

		monAmount, err := m.chain.ClaimBurnAllocationFor(ctx, winner)
		if err != nil {
			lastErr = err
			log.Printf("Claim attempt %d/%d failed: %v", attempt+1, m.cfg.BurnSwapRetries, err)
			continue
		}

		return monAmount, nil
	}

	return nil, fmt.Errorf("claim failed after %d attempts: %w", m.cfg.BurnSwapRetries, lastErr)
}

// getMinOutputWithSlippage calculates minimum output with slippage protection.
func (m *Manager) getMinOutputWithSlippage(ctx context.Context, monAmount *big.Int) (*big.Int, error) {
	expectedOut, _, err := m.chain.GetSwapQuote(ctx, monAmount)
	if err != nil {
		return nil, err
	}

	// Apply slippage: minOut = expectedOut * (10000 - slippageBps) / 10000
	slippageMultiplier := big.NewInt(int64(10000 - m.cfg.SwapSlippageBps))
	minOut := new(big.Int).Mul(expectedOut, slippageMultiplier)
	minOut.Div(minOut, big.NewInt(10000))

	return minOut, nil
}

// swapWithRetry attempts to swap MON to NEURON with retries.
func (m *Manager) swapWithRetry(ctx context.Context, record *BurnRecord, monAmount, minNeuronOut *big.Int) (*big.Int, error) {
	var lastErr error
	retryDelay := time.Duration(m.cfg.BurnSwapRetryDelayMs) * time.Millisecond

	for attempt := 0; attempt < m.cfg.BurnSwapRetries; attempt++ {
		record.Attempts = attempt + 1

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
			retryDelay *= 2

			// Re-fetch quote in case price moved
			newMinOut, err := m.getMinOutputWithSlippage(ctx, monAmount)
			if err == nil {
				minNeuronOut = newMinOut
			}
		}

		neuronAmount, err := m.chain.SwapMONToNEURON(ctx, monAmount, minNeuronOut)
		if err != nil {
			lastErr = err
			log.Printf("Swap attempt %d/%d failed: %v", attempt+1, m.cfg.BurnSwapRetries, err)
			continue
		}

		return neuronAmount, nil
	}

	return nil, fmt.Errorf("swap failed after %d attempts: %w", m.cfg.BurnSwapRetries, lastErr)
}

// burnWithRetry attempts to burn NEURON with retries.
func (m *Manager) burnWithRetry(ctx context.Context, record *BurnRecord, amount *big.Int) error {
	var lastErr error
	retryDelay := time.Duration(m.cfg.BurnSwapRetryDelayMs) * time.Millisecond

	for attempt := 0; attempt < m.cfg.BurnSwapRetries; attempt++ {
		record.Attempts = attempt + 1

		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
			retryDelay *= 2
		}

		err := m.chain.BurnNEURON(ctx, amount)
		if err != nil {
			lastErr = err
			log.Printf("Burn attempt %d/%d failed: %v", attempt+1, m.cfg.BurnSwapRetries, err)
			continue
		}

		return nil
	}

	return fmt.Errorf("burn failed after %d attempts: %w", m.cfg.BurnSwapRetries, lastErr)
}

// handleFailure handles a complete failure (claim failed).
func (m *Manager) handleFailure(ctx context.Context, record *BurnRecord, err error) error {
	record.Phase = PhaseFailed
	record.LastError = err.Error()
	m.addFailedRecord(record)
	return err
}

// handleSwapFailure handles swap/quote failure by falling back to treasury.
func (m *Manager) handleSwapFailure(ctx context.Context, record *BurnRecord, err error) error {
	record.LastError = err.Error()

	if !m.cfg.TreasuryFallback || record.MONAmount == nil {
		record.Phase = PhaseFailed
		m.addFailedRecord(record)
		return err
	}

	// Fallback: send MON to treasury
	log.Printf("Match %s: swap failed, sending %s MON to treasury as fallback", record.MatchID, record.MONAmount)

	if sendErr := m.chain.SendToTreasury(ctx, record.MONAmount); sendErr != nil {
		record.Phase = PhaseFailed
		record.LastError = fmt.Sprintf("swap failed (%v), treasury fallback also failed (%v)", err, sendErr)
		m.addFailedRecord(record)
		return fmt.Errorf("swap and treasury fallback both failed: %w", errors.Join(err, sendErr))
	}

	record.Phase = PhaseTreasury
	log.Printf("Match %s: sent %s MON to treasury (burn fallback)", record.MatchID, record.MONAmount)
	return nil // Not an error - fallback succeeded
}

// addFailedRecord adds a record to the failed list for reconciliation.
func (m *Manager) addFailedRecord(record *BurnRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failedRecords = append(m.failedRecords, record)
}

// GetRecord returns the burn record for a match.
func (m *Manager) GetRecord(matchID *big.Int) (*BurnRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.records[matchID.String()]
	return record, ok
}

// GetFailedRecords returns all failed burn records for reconciliation.
func (m *Manager) GetFailedRecords() []*BurnRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*BurnRecord, len(m.failedRecords))
	copy(result, m.failedRecords)
	return result
}

// GetStats returns burn operation statistics.
func (m *Manager) GetStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int{
		"total":    len(m.records),
		"pending":  0,
		"claimed":  0,
		"swapped":  0,
		"burned":   0,
		"failed":   0,
		"treasury": 0,
	}

	for _, r := range m.records {
		switch r.Phase {
		case PhasePending:
			stats["pending"]++
		case PhaseClaimed:
			stats["claimed"]++
		case PhaseSwapped:
			stats["swapped"]++
		case PhaseBurned:
			stats["burned"]++
		case PhaseFailed:
			stats["failed"]++
		case PhaseTreasury:
			stats["treasury"]++
		}
	}

	return stats
}
