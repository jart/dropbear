package decimal

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2.7", "2"},
		{"2.3", "2"},
		{"-2.7", "-2"},
		{"-2.3", "-2"},
		{"0.9", "0"},
		{"-0.9", "0"},
		{"100.999", "100"},
		{"-100.999", "-100"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		result := d.Truncate()
		if result.String() != tt.expected {
			t.Errorf("Truncate(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}

func TestInventoryPenalty(t *testing.T) {
	preferredOrderSize := FromInt(50)
	targetInventory := FromInt(50)
	orderNotional := FromInt(50)
	maxPenalty := FromInt(25)

	// BID scenarios (buying increases inventory)
	// Penalty is applied AFTER computing base price, so it's additive
	bidTests := []struct {
		currentInv int
		wantBps    int
	}{
		{0, 0},    // $0 -> $50 = at target, no penalty
		{25, 0},   // $25 -> $75 = 0.5 orders over, truncates to 0
		{50, 1},   // $50 -> $100 = 1 order over, 1^2 = 1
		{75, 1},   // $75 -> $125 = 1.5 orders over, truncates to 1
		{100, 4},  // $100 -> $150 = 2 orders over, 2^2 = 4
		{150, 9},  // $150 -> $200 = 3 orders over, 3^2 = 9
		{200, 16}, // $200 -> $250 = 4 orders over, 4^2 = 16
		{300, 25}, // $300 -> $350 = 6 orders over, capped at 25
	}
	for _, tt := range bidTests {
		currentInventoryUSD := FromInt(tt.currentInv)
		afterFillUSD := currentInventoryUSD.Add(orderNotional)
		penalty := Zero
		if afterFillUSD.Cmp(targetInventory) > 0 {
			deviationOrders := afterFillUSD.Sub(targetInventory).Div(preferredOrderSize).Truncate()
			penalty = deviationOrders.Mul(deviationOrders).Min(maxPenalty)
		}
		if penalty.Int() != tt.wantBps {
			t.Errorf("BID at $%d: penalty = %d bps, want %d bps", tt.currentInv, penalty.Int(), tt.wantBps)
		}

		// Verify the penalty is applied multiplicatively to price
		// newBid = baseBid / (1 + penalty)
		if penalty.IsPositive() {
			baseBid := Parse("100.00")
			penaltyMultiplier := One.Add(FromBPS(penalty.Int()))
			adjustedBid := baseBid.Div(penaltyMultiplier)
			if adjustedBid.Cmp(baseBid) >= 0 {
				t.Errorf("BID penalty should lower price: base=%s adjusted=%s", baseBid, adjustedBid)
			}
		}
	}

	// ASK scenarios (selling decreases inventory)
	askTests := []struct {
		currentInv int
		wantBps    int
	}{
		{300, 0}, // $300 -> $250 = above target, no penalty
		{150, 0}, // $150 -> $100 = above target, no penalty
		{100, 0}, // $100 -> $50 = at target, no penalty
		{75, 0},  // $75 -> $25 = 0.5 orders under, truncates to 0
		{50, 1},  // $50 -> $0 = 1 order under, 1^2 = 1
		{25, 1},  // $25 -> -$25 = 1.5 orders under, truncates to 1
		{0, 4},   // $0 -> -$50 = 2 orders under, 2^2 = 4
	}
	for _, tt := range askTests {
		currentInventoryUSD := FromInt(tt.currentInv)
		afterFillUSD := currentInventoryUSD.Sub(orderNotional)
		penalty := Zero
		if afterFillUSD.Cmp(targetInventory) < 0 {
			deviationOrders := targetInventory.Sub(afterFillUSD).Div(preferredOrderSize).Truncate()
			penalty = deviationOrders.Mul(deviationOrders).Min(maxPenalty)
		}
		if penalty.Int() != tt.wantBps {
			t.Errorf("ASK at $%d: penalty = %d bps, want %d bps", tt.currentInv, penalty.Int(), tt.wantBps)
		}

		// Verify the penalty is applied multiplicatively to price
		// newAsk = baseAsk * (1 + penalty)
		if penalty.IsPositive() {
			baseAsk := Parse("100.00")
			penaltyMultiplier := One.Add(FromBPS(penalty.Int()))
			adjustedAsk := baseAsk.Mul(penaltyMultiplier)
			if adjustedAsk.Cmp(baseAsk) <= 0 {
				t.Errorf("ASK penalty should raise price: base=%s adjusted=%s", baseAsk, adjustedAsk)
			}
		}
	}
}
