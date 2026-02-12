package strategy

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BudgetChecker reads and caches the agent's token balances from chain.
// Tracks both NEURON (for arena entry fees) and USDC (for inference costs).
type BudgetChecker struct {
	walletAddress string
	neuronAddress string
	usdcAddress   string
	rpcURL        string
	client        *http.Client

	mu              sync.Mutex
	neuronBalance   float64
	usdcBalance     float64
	lastFetched     time.Time
	cacheTTL        time.Duration
}

// NewBudgetChecker creates a new budget checker.
func NewBudgetChecker(walletAddress, neuronAddress, usdcAddress, rpcURL string) *BudgetChecker {
	return &BudgetChecker{
		walletAddress: walletAddress,
		neuronAddress: neuronAddress,
		usdcAddress:   usdcAddress,
		rpcURL:        rpcURL,
		client:        &http.Client{Timeout: 10 * time.Second},
		cacheTTL:      30 * time.Second,
	}
}

// GetNeuronBalance returns the cached NEURON balance, refreshing if stale.
func (b *BudgetChecker) GetNeuronBalance() (float64, error) {
	if err := b.refreshIfStale(); err != nil {
		return b.neuronBalance, err
	}
	return b.neuronBalance, nil
}

// GetUSDCBalance returns the cached USDC balance for inference spending.
func (b *BudgetChecker) GetUSDCBalance() (float64, error) {
	if err := b.refreshIfStale(); err != nil {
		return b.usdcBalance, err
	}
	return b.usdcBalance, nil
}

func (b *BudgetChecker) refreshIfStale() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if time.Since(b.lastFetched) < b.cacheTTL {
		return nil
	}

	neuron, err := b.fetchBalance(b.neuronAddress, 18)
	if err != nil && b.lastFetched.IsZero() {
		return err
	}
	if err == nil {
		b.neuronBalance = neuron
	}

	if b.usdcAddress != "" {
		usdc, err := b.fetchBalance(b.usdcAddress, 6)
		if err == nil {
			b.usdcBalance = usdc
		}
	}

	b.lastFetched = time.Now()
	return nil
}

// fetchBalance calls the ERC-20 balanceOf via eth_call.
func (b *BudgetChecker) fetchBalance(tokenAddress string, decimals int64) (float64, error) {
	addr := strings.TrimPrefix(b.walletAddress, "0x")
	data := "0x70a08231" + fmt.Sprintf("%064s", addr)

	payload := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"%s","data":"%s"},"latest"],"id":1}`,
		tokenAddress, data)

	resp, err := b.client.Post(b.rpcURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("unmarshal: %w", err)
	}
	if result.Error != nil {
		return 0, fmt.Errorf("rpc error: %s", result.Error.Message)
	}

	balanceHex := strings.TrimPrefix(result.Result, "0x")
	if balanceHex == "" || balanceHex == "0" {
		return 0, nil
	}

	balance, ok := new(big.Int).SetString(balanceHex, 16)
	if !ok {
		return 0, fmt.Errorf("parse balance hex: %s", balanceHex)
	}

	divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil))
	balFloat, _ := new(big.Float).Quo(new(big.Float).SetInt(balance), divisor).Float64()

	return balFloat, nil
}

// IsLowNeuronBalance checks if NEURON balance (for arena fees) is below threshold.
func (b *BudgetChecker) IsLowNeuronBalance(threshold float64) (bool, float64, error) {
	bal, err := b.GetNeuronBalance()
	if err != nil {
		return true, 0, err
	}
	return bal < threshold, bal, nil
}

// IsLowUSDCBalance checks if USDC balance (for inference) is below threshold.
func (b *BudgetChecker) IsLowUSDCBalance(threshold float64) (bool, float64, error) {
	bal, err := b.GetUSDCBalance()
	if err != nil {
		return true, 0, err
	}
	return bal < threshold, bal, nil
}
