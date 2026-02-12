package strategy

import (
	"fmt"
	"log"
)

// Engine is the main strategy decision engine.
type Engine struct {
	budget             *BudgetChecker
	rechargeThreshold  float64
	walletAddress      string
}

// NewEngine creates a new strategy engine.
func NewEngine(walletAddress, neuronAddress, usdcAddress, rpcURL string, rechargeThreshold float64) *Engine {
	return &Engine{
		budget:            NewBudgetChecker(walletAddress, neuronAddress, usdcAddress, rpcURL),
		rechargeThreshold: rechargeThreshold,
		walletAddress:     walletAddress,
	}
}

// Evaluate decides whether the agent should enter a match.
func (e *Engine) Evaluate(req MatchEvaluationRequest) MatchDecision {
	// Step 1: Check USDC balance (for inference costs)
	isLowUSDC, usdcBalance, err := e.budget.IsLowUSDCBalance(0.01) // $0.01 minimum
	if err != nil {
		log.Printf("[strategy] USDC balance check error: %v (assuming low)", err)
		isLowUSDC = true
		usdcBalance = 0
	}

	// Also check NEURON balance (for arena entry fees)
	isLowNeuron, _, err := e.budget.IsLowNeuronBalance(e.rechargeThreshold)
	if err != nil {
		log.Printf("[strategy] NEURON balance check error: %v", err)
		isLowNeuron = true
	}

	rechargeMode := isLowUSDC || isLowNeuron

	// Step 2: Select model based on difficulty + USDC budget
	model := SelectModel(req.Difficulty, usdcBalance, rechargeMode)

	// Step 3: In recharge mode, only enter easy matches
	if rechargeMode && req.Difficulty > 2 {
		return MatchDecision{
			Enter:       false,
			ModelChoice: model.Model,
			ModelTier:   model.Name,
			Reasoning:   fmt.Sprintf("recharge mode (USDC=$%.4f), skipping difficulty %d match", usdcBalance, req.Difficulty),
			ExpectedROI: 0,
		}
	}

	// Step 4: Estimate win probability
	winProb := EstimateWinProbability(req.AgentRating, req.AvgOpponent, req.PlayerCount)

	// Winner takes ~60% of prize pool in most configurations
	winShare := 0.6
	if req.PlayerCount <= 2 {
		winShare = 0.8
	}

	// Step 5: Calculate ROI (prize is in MON, inference cost in USDC — different tokens,
	// but we compare in relative terms for the decision)
	roi := CalculateROI(req.PrizePool, winShare, winProb, model.CostUSDC, req.EntryFee)

	// Step 6: Decision
	if roi < 0 && !req.IsBounty {
		return MatchDecision{
			Enter:       false,
			ModelChoice: model.Model,
			ModelTier:   model.Name,
			Reasoning:   fmt.Sprintf("negative ROI (%.4f): prize=%.2f MON, winProb=%.2f, inference=$%.4f, entry=%.2f NEURON", roi, req.PrizePool, winProb, model.CostUSDC, req.EntryFee),
			ExpectedROI: roi,
		}
	}

	// Bounty questions are always worth entering (reputation gain)
	reasoning := fmt.Sprintf("positive ROI (%.4f): prize=%.2f, winProb=%.2f, model=%s", roi, req.PrizePool, winProb, model.Name)
	if req.IsBounty {
		reasoning = fmt.Sprintf("bounty match, %s", reasoning)
	}

	return MatchDecision{
		Enter:       true,
		ModelChoice: model.Model,
		ModelTier:   model.Name,
		Reasoning:   reasoning,
		ExpectedROI: roi,
	}
}

// Status returns the current economic status of the agent.
func (e *Engine) Status() AgentEconomicStatus {
	neuronBal, _ := e.budget.GetNeuronBalance()
	usdcBal, _ := e.budget.GetUSDCBalance()
	isLowNeuron, _, _ := e.budget.IsLowNeuronBalance(e.rechargeThreshold)
	isLowUSDC, _, _ := e.budget.IsLowUSDCBalance(0.01)

	return AgentEconomicStatus{
		WalletAddress:     e.walletAddress,
		NeuronBalance:     neuronBal,
		USDCBalance:       usdcBal,
		RechargeMode:      isLowNeuron || isLowUSDC,
		RechargeThreshold: e.rechargeThreshold,
	}
}
