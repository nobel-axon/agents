package burn

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/axon-arena/axon-chief/internal/config"
)

// mockChainOperator implements ChainOperator for testing.
type mockChainOperator struct {
	claimErr       error
	claimAmount    *big.Int
	swapErr        error
	swapAmount     *big.Int
	burnErr        error
	treasuryErr    error
	quoteAmount    *big.Int
	quoteRouter    common.Address

	claimCalls   int
	swapCalls    int
	burnCalls    int
	treasuryCalls int
}

func (m *mockChainOperator) ClaimBurnAllocationFor(ctx context.Context, winner common.Address) (*big.Int, error) {
	m.claimCalls++
	if m.claimErr != nil {
		return nil, m.claimErr
	}
	return m.claimAmount, nil
}

func (m *mockChainOperator) SwapMONToNEURON(ctx context.Context, monAmount, minNeuronOut *big.Int) (*big.Int, error) {
	m.swapCalls++
	if m.swapErr != nil {
		return nil, m.swapErr
	}
	return m.swapAmount, nil
}

func (m *mockChainOperator) BurnNEURON(ctx context.Context, amount *big.Int) error {
	m.burnCalls++
	return m.burnErr
}

func (m *mockChainOperator) GetSwapQuote(ctx context.Context, monAmount *big.Int) (*big.Int, common.Address, error) {
	return m.quoteAmount, m.quoteRouter, nil
}

func (m *mockChainOperator) SendToTreasury(ctx context.Context, amount *big.Int) error {
	m.treasuryCalls++
	return m.treasuryErr
}

func defaultTestConfig() *config.NadFunConfig {
	return &config.NadFunConfig{
		BondingCurveRouterAddress: "0x6F6B8F1a20703309951a5127c45B49b1CD981A22",
		DexRouterAddress:          "0x0B79d71AE99528D1dB24A4148b5f4F865cc2b137",
		LensAddress:               "0x7e78A8DE94f21804F7a17F4E8BF9EC2c872187ea",
		WMONAddress:               "0x3bd359C1119dA7Da1D913D1C4D2B7c461115433A",
		SwapSlippageBps:           100,
		SwapDeadlineSeconds:       300,
		BurnSwapRetries:           3,
		BurnSwapRetryDelayMs:      10, // Fast for tests
		TreasuryFallback:          true,
	}
}

func TestExecuteBurnAllocation_HappyPath(t *testing.T) {
	mock := &mockChainOperator{
		claimAmount: big.NewInt(1000000000000000000), // 1 MON
		swapAmount:  big.NewInt(5000000000000000000), // 5 NEURON
		quoteAmount: big.NewInt(5000000000000000000),
	}

	manager := NewManager(defaultTestConfig(), mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	matchID := big.NewInt(1)
	winner := common.HexToAddress("0x1234567890123456789012345678901234567890")

	err := manager.ExecuteBurnAllocation(ctx, matchID, winner)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify all steps were called
	if mock.claimCalls != 1 {
		t.Errorf("expected 1 claim call, got %d", mock.claimCalls)
	}
	if mock.swapCalls != 1 {
		t.Errorf("expected 1 swap call, got %d", mock.swapCalls)
	}
	if mock.burnCalls != 1 {
		t.Errorf("expected 1 burn call, got %d", mock.burnCalls)
	}

	// Verify record state
	record, ok := manager.GetRecord(matchID)
	if !ok {
		t.Fatal("expected record to exist")
	}
	if record.Phase != PhaseBurned {
		t.Errorf("expected phase %s, got %s", PhaseBurned, record.Phase)
	}
}

func TestExecuteBurnAllocation_ClaimFails(t *testing.T) {
	mock := &mockChainOperator{
		claimErr: errors.New("claim failed"),
	}

	cfg := defaultTestConfig()
	cfg.BurnSwapRetries = 2
	manager := NewManager(cfg, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	matchID := big.NewInt(1)
	winner := common.HexToAddress("0x1234567890123456789012345678901234567890")

	err := manager.ExecuteBurnAllocation(ctx, matchID, winner)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should have retried
	if mock.claimCalls != 2 {
		t.Errorf("expected 2 claim calls (retries), got %d", mock.claimCalls)
	}

	// Should have failed record
	failed := manager.GetFailedRecords()
	if len(failed) != 1 {
		t.Errorf("expected 1 failed record, got %d", len(failed))
	}
}

func TestExecuteBurnAllocation_SwapFailsFallbackToTreasury(t *testing.T) {
	mock := &mockChainOperator{
		claimAmount: big.NewInt(1000000000000000000),
		swapErr:     errors.New("swap failed"),
		quoteAmount: big.NewInt(5000000000000000000),
	}

	cfg := defaultTestConfig()
	cfg.BurnSwapRetries = 2
	manager := NewManager(cfg, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	matchID := big.NewInt(1)
	winner := common.HexToAddress("0x1234567890123456789012345678901234567890")

	err := manager.ExecuteBurnAllocation(ctx, matchID, winner)
	if err != nil {
		t.Fatalf("expected no error (treasury fallback), got: %v", err)
	}

	// Should have tried swap multiple times
	if mock.swapCalls < 2 {
		t.Errorf("expected at least 2 swap calls, got %d", mock.swapCalls)
	}

	// Should have called treasury fallback
	if mock.treasuryCalls != 1 {
		t.Errorf("expected 1 treasury call, got %d", mock.treasuryCalls)
	}

	// Record should be in treasury state
	record, _ := manager.GetRecord(matchID)
	if record.Phase != PhaseTreasury {
		t.Errorf("expected phase %s, got %s", PhaseTreasury, record.Phase)
	}
}

func TestExecuteBurnAllocation_SwapFailsTreasuryDisabled(t *testing.T) {
	mock := &mockChainOperator{
		claimAmount: big.NewInt(1000000000000000000),
		swapErr:     errors.New("swap failed"),
		quoteAmount: big.NewInt(5000000000000000000),
	}

	cfg := defaultTestConfig()
	cfg.TreasuryFallback = false
	cfg.BurnSwapRetries = 1
	manager := NewManager(cfg, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	matchID := big.NewInt(1)
	winner := common.HexToAddress("0x1234567890123456789012345678901234567890")

	err := manager.ExecuteBurnAllocation(ctx, matchID, winner)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should NOT have called treasury
	if mock.treasuryCalls != 0 {
		t.Errorf("expected 0 treasury calls, got %d", mock.treasuryCalls)
	}

	// Should have failed record
	failed := manager.GetFailedRecords()
	if len(failed) != 1 {
		t.Errorf("expected 1 failed record, got %d", len(failed))
	}
}

func TestExecuteBurnAllocation_BurnFails(t *testing.T) {
	mock := &mockChainOperator{
		claimAmount: big.NewInt(1000000000000000000),
		swapAmount:  big.NewInt(5000000000000000000),
		quoteAmount: big.NewInt(5000000000000000000),
		burnErr:     errors.New("burn failed"),
	}

	cfg := defaultTestConfig()
	cfg.BurnSwapRetries = 1
	manager := NewManager(cfg, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	matchID := big.NewInt(1)
	winner := common.HexToAddress("0x1234567890123456789012345678901234567890")

	err := manager.ExecuteBurnAllocation(ctx, matchID, winner)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// NEURON should still be in wallet (swap succeeded)
	record, _ := manager.GetRecord(matchID)
	if record.Phase != PhaseSwapped {
		t.Errorf("expected phase %s, got %s", PhaseSwapped, record.Phase)
	}

	// Should have failed record for manual reconciliation
	failed := manager.GetFailedRecords()
	if len(failed) != 1 {
		t.Errorf("expected 1 failed record, got %d", len(failed))
	}
}

func TestGetStats(t *testing.T) {
	manager := NewManager(defaultTestConfig(), nil)

	// Add some mock records
	manager.records["1"] = &BurnRecord{Phase: PhaseBurned}
	manager.records["2"] = &BurnRecord{Phase: PhaseBurned}
	manager.records["3"] = &BurnRecord{Phase: PhaseFailed}
	manager.records["4"] = &BurnRecord{Phase: PhaseTreasury}
	manager.records["5"] = &BurnRecord{Phase: PhasePending}

	stats := manager.GetStats()

	if stats["total"] != 5 {
		t.Errorf("expected total 5, got %d", stats["total"])
	}
	if stats["burned"] != 2 {
		t.Errorf("expected burned 2, got %d", stats["burned"])
	}
	if stats["failed"] != 1 {
		t.Errorf("expected failed 1, got %d", stats["failed"])
	}
	if stats["treasury"] != 1 {
		t.Errorf("expected treasury 1, got %d", stats["treasury"])
	}
	if stats["pending"] != 1 {
		t.Errorf("expected pending 1, got %d", stats["pending"])
	}
}

func TestSlippageCalculation(t *testing.T) {
	mock := &mockChainOperator{
		quoteAmount: big.NewInt(10000), // Expected 10000
	}

	cfg := defaultTestConfig()
	cfg.SwapSlippageBps = 100 // 1%
	manager := NewManager(cfg, mock)

	ctx := context.Background()
	minOut, err := manager.getMinOutputWithSlippage(ctx, big.NewInt(1000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Min output should be 99% of expected: 10000 * 9900 / 10000 = 9900
	expected := big.NewInt(9900)
	if minOut.Cmp(expected) != 0 {
		t.Errorf("expected minOut %s, got %s", expected, minOut)
	}
}
