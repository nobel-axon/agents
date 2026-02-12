package strategy

// Default model tiers for OpenRouter (prices in USDC via x402 proxy).
var (
	CheapTier = ModelTier{
		Name:     "cheap",
		Model:    "moonshotai/kimi-k2",
		CostUSDC: 0.002,
	}
	MidTier = ModelTier{
		Name:     "mid",
		Model:    "moonshotai/kimi-k2.5",
		CostUSDC: 0.005,
	}
	ExpensiveTier = ModelTier{
		Name:     "expensive",
		Model:    "moonshotai/kimi-k2-thinking",
		CostUSDC: 0.008,
	}
)

// SelectModel picks a model tier based on difficulty and budget constraints.
func SelectModel(difficulty int, balance float64, rechargeMode bool) ModelTier {
	// In recharge mode, always use cheapest model
	if rechargeMode {
		return CheapTier
	}

	// Can't afford expensive models
	if balance < ExpensiveTier.CostUSDC*3 {
		if balance < MidTier.CostUSDC*3 {
			return CheapTier
		}
		if difficulty >= 4 {
			return MidTier
		}
		return CheapTier
	}

	// Full budget available: select by difficulty
	switch {
	case difficulty >= 5:
		return ExpensiveTier
	case difficulty >= 3:
		return MidTier
	default:
		return CheapTier
	}
}
