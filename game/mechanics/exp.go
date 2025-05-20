package mechanics

import "math"

// KillExpReward calculates exp reward when a player kills another player
// The reward is based on the level difference between killer and victim
func KillExpReward(killerLevel, victimLevel int) int {
	baseReward := 200.0

	// Level difference modifier
	levelDiff := float64(victimLevel - killerLevel)

	// Multiplier based on level difference
	multiplier := 1.0
	if levelDiff > 0 {
		// Bonus exp for killing higher level players
		// Each level difference increases reward by 20%
		multiplier = 1.0 + (levelDiff * 0.2)
	} else if levelDiff < 0 {
		// Reduced exp for killing lower level players
		// Each level difference reduces reward by 30%
		// Minimum multiplier is 0.1 (10% of base reward)
		multiplier = math.Max(0.1, 1.0+(levelDiff*0.3))
	}

	// Calculate final reward
	reward := baseReward * multiplier

	// Add victim level bonus (3% of base reward per victim level)
	// Reduced from 5% to 3% for better balance
	levelBonus := baseReward * (float64(victimLevel) * 0.03)

	return int(reward + levelBonus)
}
