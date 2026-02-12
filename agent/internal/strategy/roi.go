package strategy

// EstimateWinProbability estimates the probability of winning a match.
// Uses ERC-8004 ratings when available, otherwise falls back to 1/playerCount.
func EstimateWinProbability(agentRating, avgOpponentRating float64, playerCount int) float64 {
	if playerCount <= 0 {
		return 0
	}

	// If no ratings available, use uniform probability
	if agentRating <= 0 || avgOpponentRating <= 0 {
		return 1.0 / float64(playerCount)
	}

	// Elo-like formula: expected score = 1 / (1 + 10^((opponent-agent)/400))
	// Adjusted for multi-player: multiply by scaling factor
	diff := avgOpponentRating - agentRating
	expected := 1.0 / (1.0 + pow10(diff/400.0))

	// Scale for multi-player: in a 4-player match, base probability is 0.25
	// If our Elo edge gives us 0.6 in heads-up, scale proportionally
	baseProbability := 1.0 / float64(playerCount)
	scaledProbability := baseProbability * (expected / 0.5)

	// Clamp to [0.01, 0.95]
	if scaledProbability < 0.01 {
		return 0.01
	}
	if scaledProbability > 0.95 {
		return 0.95
	}
	return scaledProbability
}

// CalculateROI computes expected value of entering a match.
// ROI = (prizePool * winShare * winProbability) - (inferenceCost + entryFee)
func CalculateROI(prizePool, winShare, winProbability, inferenceCost, entryFee float64) float64 {
	expectedGain := prizePool * winShare * winProbability
	totalCost := inferenceCost + entryFee
	return expectedGain - totalCost
}

// pow10 computes 10^x using repeated multiplication for small values.
func pow10(x float64) float64 {
	// Use natural exponent: 10^x = e^(x * ln(10))
	// ln(10) ≈ 2.302585
	return exp(x * 2.302585)
}

// exp computes e^x using Taylor series (sufficient precision for our use case).
func exp(x float64) float64 {
	if x > 20 {
		return 1e9 // cap to avoid overflow
	}
	if x < -20 {
		return 0
	}
	result := 1.0
	term := 1.0
	for i := 1; i <= 20; i++ {
		term *= x / float64(i)
		result += term
	}
	return result
}
