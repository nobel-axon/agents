// Package chain provides blockchain interaction for axon-chief.
package chain

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// NonceManager handles nonce management for transaction submission.
// It tracks the local nonce and syncs with the chain when needed.
type NonceManager struct {
	mu       sync.Mutex
	client   *ethclient.Client
	address  common.Address
	nonce    uint64
	synced   bool
}

// NewNonceManager creates a new nonce manager.
func NewNonceManager(client *ethclient.Client, address common.Address) *NonceManager {
	return &NonceManager{
		client:  client,
		address: address,
		synced:  false,
	}
}

// GetNonce returns the next nonce to use.
// It syncs from the chain if not yet synced.
func (nm *NonceManager) GetNonce(ctx context.Context) (uint64, error) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.synced {
		if err := nm.syncFromChain(ctx); err != nil {
			return 0, err
		}
	}

	return nm.nonce, nil
}

// IncrementNonce increments the local nonce after a successful transaction.
func (nm *NonceManager) IncrementNonce() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.nonce++
}

// ResetNonce resets the nonce by syncing from the chain.
// Call this after a transaction failure to get the correct nonce.
func (nm *NonceManager) ResetNonce(ctx context.Context) error {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.synced = false
	return nm.syncFromChain(ctx)
}

// syncFromChain fetches the pending nonce from the chain.
func (nm *NonceManager) syncFromChain(ctx context.Context) error {
	nonce, err := nm.client.PendingNonceAt(ctx, nm.address)
	if err != nil {
		return err
	}
	nm.nonce = nonce
	nm.synced = true
	return nil
}

// CurrentNonce returns the current local nonce without incrementing.
func (nm *NonceManager) CurrentNonce() uint64 {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	return nm.nonce
}

// SetNonce sets the nonce to a specific value.
// Use with caution - primarily for testing.
func (nm *NonceManager) SetNonce(nonce uint64) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.nonce = nonce
	nm.synced = true
}

// GetBalance returns the MON balance of the operator address.
func (nm *NonceManager) GetBalance(ctx context.Context) (*big.Int, error) {
	return nm.client.BalanceAt(ctx, nm.address, nil)
}
