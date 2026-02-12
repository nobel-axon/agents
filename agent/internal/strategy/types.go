// Package strategy implements economic decision-making for match participation.
package strategy

// MatchEvaluationRequest contains all information needed to decide whether to enter a match.
type MatchEvaluationRequest struct {
	MatchID      string  `json:"matchId"`
	EntryFee     float64 `json:"entryFee"`     // NEURON cost to enter (burned on answer submit)
	PrizePool    float64 `json:"prizePool"`     // total MON prize
	PlayerCount  int     `json:"playerCount"`   // current players in match
	Difficulty   int     `json:"difficulty"`     // 1-5
	Category     string  `json:"category"`
	IsBounty     bool    `json:"isBounty"`      // bounty question (higher stakes)
	AgentRating  float64 `json:"agentRating"`   // agent's ERC-8004 rating (0 if unknown)
	AvgOpponent  float64 `json:"avgOpponent"`   // average opponent rating (0 if unknown)
}

// MatchDecision is the output of the strategy engine.
type MatchDecision struct {
	Enter       bool    `json:"enter"`
	ModelChoice string  `json:"modelChoice"` // model ID to use
	ModelTier   string  `json:"modelTier"`   // cheap, mid, expensive
	Reasoning   string  `json:"reasoning"`
	ExpectedROI float64 `json:"expectedROI"` // estimated return on investment
}

// AgentEconomicStatus represents the agent's current economic state.
type AgentEconomicStatus struct {
	WalletAddress      string  `json:"walletAddress"`
	NeuronBalance      float64 `json:"neuronBalance"`       // for arena entry fees (burned)
	USDCBalance        float64 `json:"usdcBalance"`         // for LLM inference costs (via x402)
	RechargeMode       bool    `json:"rechargeMode"`        // true when either balance is low
	RechargeThreshold  float64 `json:"rechargeThreshold"`
	MatchesPlayed      int     `json:"matchesPlayed"`
	TotalSpent         float64 `json:"totalSpent"`
	TotalEarned        float64 `json:"totalEarned"`
}

// ModelTier defines inference cost tiers.
type ModelTier struct {
	Name     string  // cheap, mid, expensive
	Model    string  // model ID for OpenRouter
	CostUSDC float64 // estimated USDC cost per call (via x402 proxy)
}
