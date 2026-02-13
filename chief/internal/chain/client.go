// Package chain provides blockchain interaction for axon-chief.
package chain

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/axon-arena/axon-chief/internal/chain/bindings"
	"github.com/axon-arena/axon-chief/internal/config"
)

// ChainClient handles all blockchain interactions for the chief service.
type ChainClient struct {
	client       *ethclient.Client
	arena        *bindings.AxonArena
	neuron       *bindings.NeuronToken
	privateKey   *ecdsa.PrivateKey
	address      common.Address
	chainID      *big.Int
	nonceManager *NonceManager
	cfg          *config.ChainConfig

	// txMu serializes all transaction sends to prevent nonce race conditions.
	// When two handlers try to send transactions concurrently, they would both
	// get the same nonce from NonceManager, causing one to revert on chain.
	txMu sync.Mutex

	// nad.fun contracts (initialized lazily)
	nadFunCfg      *config.NadFunConfig
	lens           *bindings.NadFunLens
	bondingRouter  *bindings.NadFunRouter
	dexRouter      *bindings.NadFunRouter
	treasuryAddr   common.Address

	// ERC-8004 reputation contracts (optional, V2)
	identityRegistry   *bindings.IdentityRegistry
	reputationRegistry *bindings.ReputationRegistry

	// BountyArena contract (optional, V2)
	bountyArena *bindings.BountyArena
}

// NewChainClient creates a new chain client.
func NewChainClient(cfg *config.ChainConfig) (*ChainClient, error) {
	// Connect to the RPC
	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	// Parse the operator private key
	keyStr := strings.TrimPrefix(cfg.OperatorKey, "0x")
	privateKey, err := crypto.HexToECDSA(keyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse operator key: %w", err)
	}

	// Derive the address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("failed to get public key")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	// Create the chain ID
	chainID := big.NewInt(cfg.ChainID)

	// Create contract bindings
	arenaAddr := common.HexToAddress(cfg.ArenaAddress)
	arena, err := bindings.NewAxonArena(arenaAddr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create arena binding: %w", err)
	}

	neuronAddr := common.HexToAddress(cfg.NeuronAddress)
	neuron, err := bindings.NewNeuronToken(neuronAddr, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create neuron binding: %w", err)
	}

	// Create nonce manager
	nonceManager := NewNonceManager(client, address)

	cc := &ChainClient{
		client:       client,
		arena:        arena,
		neuron:       neuron,
		privateKey:   privateKey,
		address:      address,
		chainID:      chainID,
		nonceManager: nonceManager,
		cfg:          cfg,
	}

	// Initialize ERC-8004 reputation contracts (optional — don't fail if not configured)
	if cfg.IdentityRegistryAddress != "" {
		idAddr := common.HexToAddress(cfg.IdentityRegistryAddress)
		idReg, err := bindings.NewIdentityRegistry(idAddr, client)
		if err != nil {
			log.Printf("Warning: failed to create IdentityRegistry binding: %v", err)
		} else {
			cc.identityRegistry = idReg
			log.Printf("IdentityRegistry initialized at %s", cfg.IdentityRegistryAddress)
		}
	}
	if cfg.ReputationRegistryAddress != "" {
		repAddr := common.HexToAddress(cfg.ReputationRegistryAddress)
		repReg, err := bindings.NewReputationRegistry(repAddr, client)
		if err != nil {
			log.Printf("Warning: failed to create ReputationRegistry binding: %v", err)
		} else {
			cc.reputationRegistry = repReg
			log.Printf("ReputationRegistry initialized at %s", cfg.ReputationRegistryAddress)
		}
	}

	// Initialize BountyArena contract (optional)
	if cfg.BountyArenaAddress != "" {
		bountyAddr := common.HexToAddress(cfg.BountyArenaAddress)
		bounty, err := bindings.NewBountyArena(bountyAddr, client)
		if err != nil {
			log.Printf("Warning: failed to create BountyArena binding: %v", err)
		} else {
			cc.bountyArena = bounty
			log.Printf("BountyArena initialized at %s", cfg.BountyArenaAddress)
		}
	}

	return cc, nil
}

// Address returns the operator address.
func (c *ChainClient) Address() common.Address {
	return c.address
}

// GetBalance returns the MON balance of the operator.
func (c *ChainClient) GetBalance(ctx context.Context) (*big.Int, error) {
	return c.client.BalanceAt(ctx, c.address, nil)
}

// GetNeuronBalance returns the NEURON balance of the operator.
func (c *ChainClient) GetNeuronBalance(ctx context.Context) (*big.Int, error) {
	return c.neuron.BalanceOf(&bind.CallOpts{Context: ctx}, c.address)
}

// IsOperator checks if the operator is authorized.
func (c *ChainClient) IsOperator(ctx context.Context) (bool, error) {
	return c.arena.IsOperator(&bind.CallOpts{Context: ctx}, c.address)
}

// GetNextMatchID returns the next match ID.
func (c *ChainClient) GetNextMatchID(ctx context.Context) (*big.Int, error) {
	return c.arena.NextMatchId(&bind.CallOpts{Context: ctx})
}

// GetMatchState returns the state of a match.
func (c *ChainClient) GetMatchState(ctx context.Context, matchID *big.Int) (bindings.MatchState, error) {
	return c.arena.GetMatchState(&bind.CallOpts{Context: ctx}, matchID)
}

// GetMatchConfig returns the configuration of a match.
func (c *ChainClient) GetMatchConfig(ctx context.Context, matchID *big.Int) (bindings.MatchConfig, error) {
	return c.arena.GetMatchConfig(&bind.CallOpts{Context: ctx}, matchID)
}

// GetMatchPlayers returns the players in a match.
func (c *ChainClient) GetMatchPlayers(ctx context.Context, matchID *big.Int) ([]common.Address, error) {
	return c.arena.GetMatchPlayers(&bind.CallOpts{Context: ctx}, matchID)
}

// GetMatchQuestion returns the question for a match.
func (c *ChainClient) GetMatchQuestion(ctx context.Context, matchID *big.Int) (bindings.MatchQuestion, error) {
	return c.arena.GetMatchQuestion(&bind.CallOpts{Context: ctx}, matchID)
}

// GetTreasury returns the treasury address from the arena contract.
func (c *ChainClient) GetTreasury(ctx context.Context) (common.Address, error) {
	return c.arena.Treasury(&bind.CallOpts{Context: ctx})
}

// GetPlayerCount returns the number of players in a match.
func (c *ChainClient) GetPlayerCount(ctx context.Context, matchID *big.Int) (*big.Int, error) {
	return c.arena.GetPlayerCount(&bind.CallOpts{Context: ctx}, matchID)
}

// CreateMatch creates a new match on-chain.
func (c *ChainClient) CreateMatch(ctx context.Context, entryFee, baseAnswerFee *big.Int, queueDuration, answerDuration uint64, minPlayers, maxPlayers uint8) (*big.Int, error) {
	tx, err := c.sendTxWithRetry(ctx, "createMatch", func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.CreateMatch(opts, entryFee, baseAnswerFee, queueDuration, answerDuration, minPlayers, maxPlayers)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create match: %w", err)
	}

	// Get the match ID from the transaction receipt
	receipt, err := c.waitForReceipt(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to get receipt: %w", err)
	}

	// Parse the MatchCreated event from logs
	for _, vLog := range receipt.Logs {
		// MatchCreated event topic
		if vLog.Topics[0].Hex() == crypto.Keccak256Hash([]byte("MatchCreated(uint256,uint256,uint256,uint64,uint8,uint8)")).Hex() {
			matchID := new(big.Int).SetBytes(vLog.Topics[1].Bytes())
			return matchID, nil
		}
	}

	return nil, errors.New("MatchCreated event not found in logs")
}

// PostQuestion posts a question to a match.
// Uses fixed gas limit (Monad's eth_estimateGas can return tight estimates
// that fail during eth_sendRawTransaction pre-flight for string-heavy txs).
func (c *ChainClient) PostQuestion(ctx context.Context, matchID *big.Int, question, category string, difficulty uint8, formatHint string, answerHash [32]byte) error {
	log.Printf("[DEBUG PostQuestion] matchID=%s operator=%s question_len=%d category=%q difficulty=%d formatHint=%q answerHash=%x",
		matchID, c.address.Hex(), len(question), category, difficulty, formatHint, answerHash)

	// Debug: read chain state right before sending
	state, err := c.GetMatchState(ctx, matchID)
	if err != nil {
		log.Printf("[DEBUG PostQuestion] failed to read chain state: %v", err)
	} else {
		log.Printf("[DEBUG PostQuestion] chain state: phase=%d pool=%s answerHash=%x answerDeadline=%d winner=%s",
			state.Phase, state.Pool, state.AnswerHash, state.AnswerDeadline, state.Winner)
	}

	// Debug: check operator status
	isOp, opErr := c.arena.IsOperator(nil, c.address)
	if opErr != nil {
		log.Printf("[DEBUG PostQuestion] failed to check operator: %v", opErr)
	} else {
		log.Printf("[DEBUG PostQuestion] operators[%s] = %v", c.address.Hex(), isOp)
	}

	label := fmt.Sprintf("Match %s: postQuestion", matchID)
	_, err = c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		log.Printf("[DEBUG PostQuestion] sending TX: nonce=%s gasPrice=%s", opts.Nonce, opts.GasPrice)
		return c.arena.PostQuestion(opts, matchID, question, category, difficulty, formatHint, answerHash)
	})
	if err != nil {
		return fmt.Errorf("failed to post question: %w", err)
	}
	return nil
}

// StartMatch starts a match (transitions from Queue to Active).
func (c *ChainClient) StartMatch(ctx context.Context, matchID *big.Int) error {
	label := fmt.Sprintf("Match %s: startMatch", matchID)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.StartMatch(opts, matchID)
	})
	if err != nil {
		return fmt.Errorf("failed to start match: %w", err)
	}
	return nil
}

// StartAnswerPeriod starts the answer period for a match.
func (c *ChainClient) StartAnswerPeriod(ctx context.Context, matchID *big.Int) error {
	label := fmt.Sprintf("Match %s: startAnswerPeriod", matchID)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.StartAnswerPeriod(opts, matchID)
	})
	if err != nil {
		return fmt.Errorf("failed to start answer period: %w", err)
	}
	return nil
}

// SettleWinner settles a match with the given winner. Returns the TX hash.
func (c *ChainClient) SettleWinner(ctx context.Context, matchID *big.Int, winner common.Address) (string, error) {
	label := fmt.Sprintf("Match %s: settleWinner(%s)", matchID, winner.Hex()[:10])
	tx, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.SettleWinner(opts, matchID, winner)
	})
	if err != nil {
		return "", fmt.Errorf("failed to settle winner: %w", err)
	}
	return tx.Hash().Hex(), nil
}

// RevealAnswer reveals the answer for a match.
func (c *ChainClient) RevealAnswer(ctx context.Context, matchID *big.Int, answer string, salt [32]byte) error {
	label := fmt.Sprintf("Match %s: revealAnswer", matchID)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.RevealAnswer(opts, matchID, answer, salt)
	})
	if err != nil {
		return fmt.Errorf("failed to reveal answer: %w", err)
	}
	return nil
}

// RefundMatch refunds all players in a match.
func (c *ChainClient) RefundMatch(ctx context.Context, matchID *big.Int) error {
	label := fmt.Sprintf("Match %s: refundMatch", matchID)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.RefundMatch(opts, matchID)
	})
	if err != nil {
		return fmt.Errorf("failed to refund match: %w", err)
	}
	return nil
}

// CancelMatch cancels a match.
func (c *ChainClient) CancelMatch(ctx context.Context, matchID *big.Int) error {
	label := fmt.Sprintf("Match %s: cancelMatch", matchID)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.CancelMatch(opts, matchID)
	})
	if err != nil {
		return fmt.Errorf("failed to cancel match: %w", err)
	}
	return nil
}

// sendTxWithRetry sends a transaction with retries and exponential backoff.
// It holds txMu for the entire lifecycle (nonce acquisition -> send -> receipt confirmation)
// to prevent concurrent handlers from racing on the same nonce.
// The label parameter provides context for log messages (e.g., "Match 142: settleWinner").
func (c *ChainClient) sendTxWithRetry(ctx context.Context, label string, txFunc func(*bind.TransactOpts) (*types.Transaction, error)) (*types.Transaction, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()

	var lastErr error
	retryInterval := time.Duration(c.cfg.TxRetryIntervalMs) * time.Millisecond

	for i := 0; i < c.cfg.TxRetryCount; i++ {
		if i > 0 {
			log.Printf("[%s] Retrying transaction (attempt %d/%d) after %v...", label, i+1, c.cfg.TxRetryCount, retryInterval)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryInterval):
			}
			// Exponential backoff
			retryInterval *= 2
			if retryInterval > 30*time.Second {
				retryInterval = 30 * time.Second
			}
		}

		// Get nonce
		nonce, err := c.nonceManager.GetNonce(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get nonce: %w", err)
			continue
		}

		// Get gas price
		gasPrice, err := c.client.SuggestGasPrice(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get gas price: %w", err)
			continue
		}

		// Cap gas price
		maxGasPrice := c.cfg.GetMaxGasPrice()
		if gasPrice.Cmp(maxGasPrice) > 0 {
			gasPrice = maxGasPrice
		}

		// Create transaction options
		opts, err := bind.NewKeyedTransactorWithChainID(c.privateKey, c.chainID)
		if err != nil {
			lastErr = fmt.Errorf("failed to create transactor: %w", err)
			continue
		}
		opts.Context = ctx
		opts.Nonce = big.NewInt(int64(nonce))
		opts.GasPrice = gasPrice
		opts.GasLimit = 0 // Let go-ethereum estimate gas automatically

		// Send transaction
		tx, err := txFunc(opts)
		if err != nil {
			lastErr = fmt.Errorf("failed to send transaction: %w", err)
			log.Printf("[%s] TX failed (attempt %d/%d, nonce=%d, gasPrice=%s gwei): %v",
				label, i+1, c.cfg.TxRetryCount, nonce, new(big.Int).Div(gasPrice, big.NewInt(1e9)), err)
			// Extract revert data if available (Monad returns custom error data)
			var dataErr rpc.DataError
			if errors.As(err, &dataErr) {
				log.Printf("[%s] Revert data: %v", label, dataErr.ErrorData())
			}
			// Reset nonce on failure
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		// Wait for receipt
		receipt, err := c.waitForReceipt(ctx, tx)
		if err != nil {
			lastErr = fmt.Errorf("failed to wait for receipt: %w", err)
			// Reset nonce on failure
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		// Check status
		if receipt.Status == types.ReceiptStatusFailed {
			lastErr = errors.New("transaction reverted")
			log.Printf("[%s] TX reverted: %s", label, tx.Hash().Hex())
			// Reset nonce on failure
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		// Success - increment nonce
		c.nonceManager.IncrementNonce()
		log.Printf("[%s] TX confirmed: %s (gas: %d)", label, tx.Hash().Hex(), receipt.GasUsed)
		return tx, nil
	}

	return nil, fmt.Errorf("[%s] transaction failed after %d retries: %w", label, c.cfg.TxRetryCount, lastErr)
}

// waitForReceipt waits for a transaction receipt.
func (c *ChainClient) waitForReceipt(ctx context.Context, tx *types.Transaction) (*types.Receipt, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, errors.New("timeout waiting for receipt")
		case <-ticker.C:
			receipt, err := c.client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				continue // Not mined yet
			}
			return receipt, nil
		}
	}
}

// GetClient returns the underlying ethclient.
func (c *ChainClient) GetClient() *ethclient.Client {
	return c.client
}

// WaitForPhase polls the chain until the match reaches the expected phase.
// This is critical for Monad testnet where the RPC load balancer may route
// requests to nodes that haven't yet synced the latest block.
func (c *ChainClient) WaitForPhase(ctx context.Context, matchID *big.Int, expectedPhase uint8, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for match %s to reach phase %d", matchID, expectedPhase)
		case <-ticker.C:
			state, err := c.GetMatchState(ctx, matchID)
			if err != nil {
				log.Printf("WaitForPhase: failed to get state for match %s: %v", matchID, err)
				continue
			}
			if state.Phase == expectedPhase {
				log.Printf("WaitForPhase: match %s confirmed in phase %d", matchID, expectedPhase)
				return nil
			}
			log.Printf("WaitForPhase: match %s phase=%d, waiting for %d", matchID, state.Phase, expectedPhase)
		}
	}
}

// Close closes the chain client.
func (c *ChainClient) Close() {
	c.client.Close()
}

// InitNadFun initializes nad.fun contract bindings.
func (c *ChainClient) InitNadFun(nadFunCfg *config.NadFunConfig, treasuryAddr common.Address) error {
	c.nadFunCfg = nadFunCfg
	c.treasuryAddr = treasuryAddr

	// Initialize Lens contract
	lensAddr := common.HexToAddress(nadFunCfg.LensAddress)
	lens, err := bindings.NewNadFunLens(lensAddr, c.client)
	if err != nil {
		return fmt.Errorf("failed to create lens binding: %w", err)
	}
	c.lens = lens

	// Initialize BondingCurveRouter
	bondingAddr := common.HexToAddress(nadFunCfg.BondingCurveRouterAddress)
	bondingRouter, err := bindings.NewNadFunBondingCurveRouter(bondingAddr, c.client)
	if err != nil {
		return fmt.Errorf("failed to create bonding router binding: %w", err)
	}
	c.bondingRouter = bondingRouter

	// Initialize DexRouter
	dexAddr := common.HexToAddress(nadFunCfg.DexRouterAddress)
	dexRouter, err := bindings.NewNadFunDexRouter(dexAddr, c.client)
	if err != nil {
		return fmt.Errorf("failed to create dex router binding: %w", err)
	}
	c.dexRouter = dexRouter

	log.Printf("nad.fun contracts initialized: lens=%s, bonding=%s, dex=%s",
		nadFunCfg.LensAddress, nadFunCfg.BondingCurveRouterAddress, nadFunCfg.DexRouterAddress)

	return nil
}

// ClaimBurnAllocationFor claims the burn allocation for a winner from AxonArena.
// Returns the MON amount that was claimed and transferred to operator.
func (c *ChainClient) ClaimBurnAllocationFor(ctx context.Context, winner common.Address) (*big.Int, error) {
	// First, check how much allocation the winner has
	allocation, err := c.arena.BurnAllocation(&bind.CallOpts{Context: ctx}, winner)
	if err != nil {
		return nil, fmt.Errorf("failed to check burn allocation: %w", err)
	}

	if allocation.Sign() == 0 {
		return nil, errors.New("no burn allocation for winner")
	}

	// Claim the allocation (sends MON to operator)
	_, err = c.sendTxWithRetry(ctx, fmt.Sprintf("claimBurnAllocation(%s)", winner.Hex()[:10]), func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.arena.ClaimBurnAllocationFor(opts, winner)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to claim burn allocation: %w", err)
	}

	return allocation, nil
}

// GetSwapQuote gets the expected NEURON output for a MON input via nad.fun.
// Returns: expected output amount, router address, error
func (c *ChainClient) GetSwapQuote(ctx context.Context, monAmount *big.Int) (*big.Int, common.Address, error) {
	if c.lens == nil {
		return nil, common.Address{}, errors.New("nad.fun not initialized")
	}

	neuronAddr := common.HexToAddress(c.cfg.NeuronAddress)

	// Query Lens for quote - isBuy=true because we're buying NEURON with MON
	routerAddr, expectedOut, err := c.lens.GetAmountOut(
		&bind.CallOpts{Context: ctx},
		neuronAddr,
		monAmount,
		true, // isBuy
	)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to get swap quote: %w", err)
	}

	return expectedOut, routerAddr, nil
}

// SwapMONToNEURON swaps MON to NEURON via nad.fun.
// Uses the appropriate router (bonding curve or DEX) based on token graduation status.
func (c *ChainClient) SwapMONToNEURON(ctx context.Context, monAmount, minNeuronOut *big.Int) (*big.Int, error) {
	if c.lens == nil || c.bondingRouter == nil || c.dexRouter == nil {
		return nil, errors.New("nad.fun not initialized")
	}

	neuronAddr := common.HexToAddress(c.cfg.NeuronAddress)

	// Get the appropriate router from Lens
	routerAddr, _, err := c.lens.GetAmountOut(
		&bind.CallOpts{Context: ctx},
		neuronAddr,
		monAmount,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to determine router: %w", err)
	}

	// Prepare buy params
	deadline := big.NewInt(time.Now().Unix() + int64(c.nadFunCfg.SwapDeadlineSeconds))
	params := bindings.BuyParams{
		AmountOutMin: minNeuronOut,
		Token:        neuronAddr,
		To:           c.address, // Receive NEURON to operator wallet
		Deadline:     deadline,
	}

	// Select the correct router
	var router *bindings.NadFunRouter
	if routerAddr == c.bondingRouter.Address() {
		router = c.bondingRouter
		log.Printf("Using BondingCurveRouter for swap")
	} else if routerAddr == c.dexRouter.Address() {
		router = c.dexRouter
		log.Printf("Using DexRouter for swap")
	} else {
		return nil, fmt.Errorf("unknown router address: %s", routerAddr.Hex())
	}

	// Execute swap
	var neuronReceived *big.Int
	_, err = c.sendTxWithRetryValue(ctx, fmt.Sprintf("swapMON→NEURON(%s)", monAmount), monAmount, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return router.Buy(opts, params)
	})
	if err != nil {
		return nil, fmt.Errorf("swap failed: %w", err)
	}

	// Check NEURON balance after swap
	neuronReceived, err = c.neuron.BalanceOf(&bind.CallOpts{Context: ctx}, c.address)
	if err != nil {
		// Swap succeeded but couldn't verify balance - return minNeuronOut as estimate
		log.Printf("Warning: couldn't verify NEURON balance after swap: %v", err)
		return minNeuronOut, nil
	}

	return neuronReceived, nil
}

// BurnNEURON burns NEURON tokens from the operator's wallet.
func (c *ChainClient) BurnNEURON(ctx context.Context, amount *big.Int) error {
	// Check balance first
	balance, err := c.neuron.BalanceOf(&bind.CallOpts{Context: ctx}, c.address)
	if err != nil {
		return fmt.Errorf("failed to check NEURON balance: %w", err)
	}

	if balance.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient NEURON balance: have %s, need %s", balance, amount)
	}

	// Burn the tokens
	_, err = c.sendTxWithRetry(ctx, fmt.Sprintf("burnNEURON(%s)", amount), func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.neuron.Burn(opts, amount)
	})
	if err != nil {
		return fmt.Errorf("failed to burn NEURON: %w", err)
	}

	log.Printf("Burned %s NEURON tokens", amount)
	return nil
}

// SendToTreasury sends MON to the treasury address as a fallback.
func (c *ChainClient) SendToTreasury(ctx context.Context, amount *big.Int) error {
	c.txMu.Lock()
	defer c.txMu.Unlock()

	if c.treasuryAddr == (common.Address{}) {
		return errors.New("treasury address not set")
	}

	// Send MON to treasury
	nonce, err := c.nonceManager.GetNonce(ctx)
	if err != nil {
		return fmt.Errorf("failed to get nonce: %w", err)
	}

	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get gas price: %w", err)
	}

	// Cap gas price
	maxGasPrice := c.cfg.GetMaxGasPrice()
	if gasPrice.Cmp(maxGasPrice) > 0 {
		gasPrice = maxGasPrice
	}

	tx := types.NewTransaction(
		nonce,
		c.treasuryAddr,
		amount,
		21000, // Standard ETH transfer gas
		gasPrice,
		nil,
	)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(c.chainID), c.privateKey)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	if err := c.client.SendTransaction(ctx, signedTx); err != nil {
		if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
			log.Printf("Warning: failed to reset nonce: %v", resetErr)
		}
		return fmt.Errorf("failed to send transaction: %w", err)
	}

	// Wait for receipt
	receipt, err := c.waitForReceipt(ctx, signedTx)
	if err != nil {
		if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
			log.Printf("Warning: failed to reset nonce: %v", resetErr)
		}
		return fmt.Errorf("failed to wait for receipt: %w", err)
	}

	if receipt.Status == types.ReceiptStatusFailed {
		if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
			log.Printf("Warning: failed to reset nonce: %v", resetErr)
		}
		return errors.New("treasury transfer reverted")
	}

	c.nonceManager.IncrementNonce()
	log.Printf("Sent %s MON to treasury %s", amount, c.treasuryAddr.Hex())
	return nil
}

// --- ERC-8004 Reputation Methods ---

// HasReputationContracts returns true if ERC-8004 contracts are configured.
func (c *ChainClient) HasReputationContracts() bool {
	return c.identityRegistry != nil && c.reputationRegistry != nil
}

// RegisterAgent registers the operator as an agent in the IdentityRegistry.
func (c *ChainClient) RegisterAgent(ctx context.Context, agentURI string) error {
	if c.identityRegistry == nil {
		return errors.New("IdentityRegistry not configured")
	}
	label := fmt.Sprintf("register(%s)", agentURI)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.identityRegistry.Register(opts, agentURI)
	})
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}
	return nil
}

// OwnerOf returns the owner address for a given agent ID.
func (c *ChainClient) OwnerOf(ctx context.Context, agentId *big.Int) (common.Address, error) {
	if c.identityRegistry == nil {
		return common.Address{}, errors.New("IdentityRegistry not configured")
	}
	return c.identityRegistry.OwnerOf(&bind.CallOpts{Context: ctx}, agentId)
}

// GetAgentWallet returns the wallet address for a given agent ID.
func (c *ChainClient) GetAgentWallet(ctx context.Context, agentId *big.Int) (common.Address, error) {
	if c.identityRegistry == nil {
		return common.Address{}, errors.New("IdentityRegistry not configured")
	}
	return c.identityRegistry.GetAgentWallet(&bind.CallOpts{Context: ctx}, agentId)
}

// GiveFeedback submits reputation feedback for an agent.
func (c *ChainClient) GiveFeedback(ctx context.Context, agentId *big.Int, value *big.Int, valueDecimals uint8, tag1, tag2, endpoint, feedbackURI string, feedbackHash [32]byte) error {
	if c.reputationRegistry == nil {
		return errors.New("ReputationRegistry not configured")
	}
	label := fmt.Sprintf("giveFeedback(agent=%s, value=%s)", agentId, value)
	_, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.reputationRegistry.GiveFeedback(opts, agentId, value, valueDecimals, tag1, tag2, endpoint, feedbackURI, feedbackHash)
	})
	if err != nil {
		return fmt.Errorf("failed to give feedback: %w", err)
	}
	return nil
}

// GetSummary returns the reputation summary for an agent.
func (c *ChainClient) GetSummary(ctx context.Context, agentId *big.Int, clientAddresses []common.Address, tag1, tag2 string) (bindings.ReputationData, error) {
	if c.reputationRegistry == nil {
		return bindings.ReputationData{}, errors.New("ReputationRegistry not configured")
	}
	count, summaryValue, summaryValueDecimals, err := c.reputationRegistry.GetSummary(&bind.CallOpts{Context: ctx}, agentId, clientAddresses, tag1, tag2)
	if err != nil {
		return bindings.ReputationData{}, err
	}
	return bindings.ReputationData{
		Count:                count,
		SummaryValue:         summaryValue,
		SummaryValueDecimals: summaryValueDecimals,
	}, nil
}

// --- BountyArena Methods ---

// HasBountyArena returns true if the BountyArena contract is configured.
func (c *ChainClient) HasBountyArena() bool {
	return c.bountyArena != nil
}

// GetBountyState returns the on-chain state of a bounty.
func (c *ChainClient) GetBountyState(ctx context.Context, bountyID *big.Int) (bindings.BountyState, error) {
	if c.bountyArena == nil {
		return bindings.BountyState{}, errors.New("BountyArena not configured")
	}
	return c.bountyArena.GetBountyState(&bind.CallOpts{Context: ctx}, bountyID)
}

// GetBountyAgents returns the agents in a bounty.
func (c *ChainClient) GetBountyAgents(ctx context.Context, bountyID *big.Int) ([]common.Address, error) {
	if c.bountyArena == nil {
		return nil, errors.New("BountyArena not configured")
	}
	return c.bountyArena.GetBountyAgents(&bind.CallOpts{Context: ctx}, bountyID)
}

// PickWinner settles a bounty on-chain by picking the winner. Returns the TX hash.
func (c *ChainClient) PickWinner(ctx context.Context, bountyID *big.Int, winner common.Address) (string, error) {
	if c.bountyArena == nil {
		return "", errors.New("BountyArena not configured")
	}
	label := fmt.Sprintf("Bounty %s: pickWinner(%s)", bountyID, winner.Hex()[:10])
	tx, err := c.sendTxWithRetry(ctx, label, func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return c.bountyArena.PickWinner(opts, bountyID, winner)
	})
	if err != nil {
		return "", fmt.Errorf("failed to pick winner: %w", err)
	}
	return tx.Hash().Hex(), nil
}

// sendTxWithRetryValue sends a transaction with value and retries.
// It holds txMu for the entire lifecycle to prevent nonce race conditions.
func (c *ChainClient) sendTxWithRetryValue(ctx context.Context, label string, value *big.Int, txFunc func(*bind.TransactOpts) (*types.Transaction, error)) (*types.Transaction, error) {
	c.txMu.Lock()
	defer c.txMu.Unlock()

	var lastErr error
	retryInterval := time.Duration(c.cfg.TxRetryIntervalMs) * time.Millisecond

	for i := 0; i < c.cfg.TxRetryCount; i++ {
		if i > 0 {
			log.Printf("[%s] Retrying transaction (attempt %d/%d) after %v...", label, i+1, c.cfg.TxRetryCount, retryInterval)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryInterval):
			}
			retryInterval *= 2
			if retryInterval > 30*time.Second {
				retryInterval = 30 * time.Second
			}
		}

		nonce, err := c.nonceManager.GetNonce(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get nonce: %w", err)
			continue
		}

		gasPrice, err := c.client.SuggestGasPrice(ctx)
		if err != nil {
			lastErr = fmt.Errorf("failed to get gas price: %w", err)
			continue
		}

		maxGasPrice := c.cfg.GetMaxGasPrice()
		if gasPrice.Cmp(maxGasPrice) > 0 {
			gasPrice = maxGasPrice
		}

		opts, err := bind.NewKeyedTransactorWithChainID(c.privateKey, c.chainID)
		if err != nil {
			lastErr = fmt.Errorf("failed to create transactor: %w", err)
			continue
		}
		opts.Context = ctx
		opts.Nonce = big.NewInt(int64(nonce))
		opts.GasPrice = gasPrice
		opts.GasLimit = 0 // Let go-ethereum estimate gas automatically
		opts.Value = value // Set the value for payable functions

		tx, err := txFunc(opts)
		if err != nil {
			lastErr = fmt.Errorf("failed to send transaction: %w", err)
			log.Printf("[%s] TX failed (attempt %d/%d): %v", label, i+1, c.cfg.TxRetryCount, err)
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		receipt, err := c.waitForReceipt(ctx, tx)
		if err != nil {
			lastErr = fmt.Errorf("failed to wait for receipt: %w", err)
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		if receipt.Status == types.ReceiptStatusFailed {
			lastErr = errors.New("transaction reverted")
			log.Printf("[%s] TX reverted: %s", label, tx.Hash().Hex())
			if resetErr := c.nonceManager.ResetNonce(ctx); resetErr != nil {
				log.Printf("Warning: failed to reset nonce: %v", resetErr)
			}
			continue
		}

		c.nonceManager.IncrementNonce()
		log.Printf("[%s] TX confirmed: %s (gas: %d)", label, tx.Hash().Hex(), receipt.GasUsed)
		return tx, nil
	}

	return nil, fmt.Errorf("[%s] transaction failed after %d retries: %w", label, c.cfg.TxRetryCount, lastErr)
}
