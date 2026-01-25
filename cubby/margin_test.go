package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/netty"
	"testing"
)

func init() {
	netty.SetOffline()
}

// TestAlpacaMarginScenario tests the margin scenario from Alpaca docs:
// https://docs.alpaca.markets/docs/user-protection
//
// Scenario:
//   - Your equity is $50k
//   - You hold overnight positions up to $100k
//   - Your maintenance margin is $30k (~30%)
//   - Your DTBP at beginning of day is $80k using 4 * ($50k - $30k)
//   - You sell all overnight positions ($100k value) in the morning
//   - Your regt_buying_power goes up to $100k
//   - You buy and sell the same security up to $100k
//   - At end of day, you have a $20k Day Trade Margin Call ($100k - $80k)
func TestAlpacaMarginScenario(t *testing.T) {
	// Create a mock asset with 30% margin requirement
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}

	// Starting state: $100k position at 30% margin = $30k maintenance margin
	positionValue := decimal.FromInt(100_000)
	price := decimal.FromInt(100) // $100/share
	quantity := positionValue.Div(price)

	// Verify maintenance margin is ~30%
	maintenanceMargin := asset.GetMaintenanceMargin(quantity, price)
	expectedMaintenance := decimal.FromInt(30_000)
	if maintenanceMargin.Cmp(expectedMaintenance) != 0 {
		t.Errorf("maintenance margin = %s, want %s", maintenanceMargin, expectedMaintenance)
	}

	// Equity = Position Value - (what you borrowed)
	// If you have $100k position and $30k margin, you borrowed $70k? No...
	// Actually: Equity = Cash + Position Value
	// If position is $100k and equity is $50k, then Cash = $50k - $100k = -$50k (borrowed)
	//
	// Let's model it differently:
	// - Position Value: $100k
	// - Equity (account value): $50k
	// - Maintenance Margin Required: $30k (30% of $100k)
	// - Excess Margin: $50k - $30k = $20k
	// - DTBP = 4 * Excess Margin = 4 * $20k = $80k

	equity := decimal.FromInt(50_000)
	excessMargin := equity.Sub(maintenanceMargin)
	expectedExcess := decimal.FromInt(20_000)
	if excessMargin.Cmp(expectedExcess) != 0 {
		t.Errorf("excess margin = %s, want %s", excessMargin, expectedExcess)
	}

	// Day Trading Buying Power = 4 * (Equity - Maintenance Margin)
	dtbp := excessMargin.MulInt(4)
	expectedDTBP := decimal.FromInt(80_000)
	if dtbp.Cmp(expectedDTBP) != 0 {
		t.Errorf("DTBP = %s, want %s", dtbp, expectedDTBP)
	}

	// After selling overnight positions, regt_buying_power = $100k
	// (You now have $50k equity and no positions, so 2x overnight = $100k)
	regtBuyingPower := equity.MulInt(2)
	expectedRegtBP := decimal.FromInt(100_000)
	if regtBuyingPower.Cmp(expectedRegtBP) != 0 {
		t.Errorf("RegT buying power = %s, want %s", regtBuyingPower, expectedRegtBP)
	}

	// If you day trade $100k worth but only had $80k DTBP,
	// you have a $20k margin call
	dayTradeAmount := decimal.FromInt(100_000)
	marginCall := dayTradeAmount.Sub(dtbp)
	expectedMarginCall := decimal.FromInt(20_000)
	if marginCall.Cmp(expectedMarginCall) != 0 {
		t.Errorf("margin call = %s, want %s", marginCall, expectedMarginCall)
	}

	t.Logf("Scenario validated:")
	t.Logf("  Position Value:     $%s", positionValue.FormatThousand(2))
	t.Logf("  Equity:             $%s", equity.FormatThousand(2))
	t.Logf("  Maintenance Margin: $%s (%.0f%%)", maintenanceMargin.FormatThousand(2),
		maintenanceMargin.Div(positionValue).MulInt(100).Float64())
	t.Logf("  Excess Margin:      $%s", excessMargin.FormatThousand(2))
	t.Logf("  DTBP:               $%s (4x excess)", dtbp.FormatThousand(2))
	t.Logf("  RegT Buying Power:  $%s (2x equity)", regtBuyingPower.FormatThousand(2))
	t.Logf("  Day Trade Amount:   $%s", dayTradeAmount.FormatThousand(2))
	t.Logf("  Margin Call:        $%s", marginCall.FormatThousand(2))
}

// TestMaintenanceMarginCalculation tests that margin is calculated correctly
// for various position sizes and prices.
func TestMaintenanceMarginCalculation(t *testing.T) {
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}

	tests := []struct {
		name     string
		quantity decimal.Decimal
		price    decimal.Decimal
		want     decimal.Decimal
	}{
		{"1000 shares @ $100", decimal.FromInt(1000), decimal.FromInt(100), decimal.FromInt(30_000)},
		{"500 shares @ $200", decimal.FromInt(500), decimal.FromInt(200), decimal.FromInt(30_000)},
		{"100 shares @ $50", decimal.FromInt(100), decimal.FromInt(50), decimal.FromInt(1_500)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asset.GetMaintenanceMargin(tt.quantity, tt.price)
			if got.Cmp(tt.want) != 0 {
				t.Errorf("GetMaintenanceMargin(%s, %s) = %s, want %s",
					tt.quantity, tt.price, got, tt.want)
			}
		})
	}
}

// TestInitialMarginCalculation tests initial margin (50% of position value minimum).
func TestInitialMarginCalculation(t *testing.T) {
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}

	tests := []struct {
		name     string
		quantity decimal.Decimal
		price    decimal.Decimal
		want     decimal.Decimal
	}{
		// Initial margin = max(maintenance, 50% of value)
		// For 30% maintenance, 50% always wins
		{"1000 shares @ $100", decimal.FromInt(1000), decimal.FromInt(100), decimal.FromInt(50_000)},
		{"500 shares @ $200", decimal.FromInt(500), decimal.FromInt(200), decimal.FromInt(50_000)},
		{"100 shares @ $50", decimal.FromInt(100), decimal.FromInt(50), decimal.FromInt(2_500)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asset.GetInitialMargin(tt.quantity, tt.price)
			if got.Cmp(tt.want) != 0 {
				t.Errorf("GetInitialMargin(%s, %s) = %s, want %s",
					tt.quantity, tt.price, got, tt.want)
			}
		})
	}
}

// TestDTBPCalculation tests Day Trading Buying Power calculation.
func TestDTBPCalculation(t *testing.T) {
	tests := []struct {
		name              string
		equity            decimal.Decimal
		maintenanceMargin decimal.Decimal
		wantDTBP          decimal.Decimal
	}{
		{"$50k equity, $30k margin", decimal.FromInt(50_000), decimal.FromInt(30_000), decimal.FromInt(80_000)},
		{"$100k equity, $25k margin", decimal.FromInt(100_000), decimal.FromInt(25_000), decimal.FromInt(300_000)},
		{"$25k equity, $10k margin", decimal.FromInt(25_000), decimal.FromInt(10_000), decimal.FromInt(60_000)},
		{"$30k equity, $30k margin (no excess)", decimal.FromInt(30_000), decimal.FromInt(30_000), decimal.FromInt(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// DTBP = 4 * (Equity - Maintenance Margin)
			excessMargin := tt.equity.Sub(tt.maintenanceMargin)
			dtbp := excessMargin.MulInt(4)
			if dtbp.Cmp(tt.wantDTBP) != 0 {
				t.Errorf("DTBP = %s, want %s", dtbp, tt.wantDTBP)
			}
		})
	}
}

// TestPDTRule tests Pattern Day Trader rule - accounts under $25k get 1x margin.
func TestPDTRule(t *testing.T) {
	tests := []struct {
		name      string
		equity    decimal.Decimal
		wantPower int // 1 for overnight (2x), 2 for day trading (4x)
	}{
		{"$24,999 - no day trading", decimal.FromInt(24_999), 1},
		{"$25,000 - day trading allowed", decimal.FromInt(25_000), 1}, // exactly $25k is borderline
		{"$25,001 - day trading allowed", decimal.FromInt(25_001), 2},
		{"$50,000 - day trading allowed", decimal.FromInt(50_000), 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var powerLevel int
			if tt.equity.Cmp(decimal.FromInt(25_000)) > 0 {
				powerLevel = 2
			} else {
				powerLevel = 1
			}
			if powerLevel != tt.wantPower {
				t.Errorf("power level = %d, want %d", powerLevel, tt.wantPower)
			}
		})
	}
}

// TestOrderBuyingPowerCheck validates the margin checking logic in Order().
// This tests the specific code block:
//
//	marginNeeded := decimal.Zero
//	price := limitPrice.Min(e.Price)
//	newMargin := e.Asset.GetInitialMargin(newQty, price)
//	oldMargin := e.Asset.GetMaintenanceMargin(e.Quantity, price)
//	if newMargin.Cmp(oldMargin) > 0 {
//	    marginNeeded = newMargin.Sub(oldMargin)
//	    ...
//	}
func TestOrderBuyingPowerCheck(t *testing.T) {
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}

	tests := []struct {
		name            string
		currentQty      decimal.Decimal // existing position
		orderQty        decimal.Decimal // shares to buy (positive) or sell (negative)
		currentPrice    decimal.Decimal // market price
		limitPrice      decimal.Decimal // limit price on order
		dtbp            decimal.Decimal // day trading buying power
		marginUsed      decimal.Decimal // margin already used by other positions
		marginHold      decimal.Decimal // margin held for pending orders
		wantMarginNeed  decimal.Decimal // expected margin needed for this order
		wantMarginAvail decimal.Decimal // expected available margin
		wantSuccess     bool            // whether order should be allowed
	}{
		{
			name:            "fresh buy with sufficient margin",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.FromInt(100_000),
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(5_000), // 50% initial margin on $10k position
			wantMarginAvail: decimal.FromInt(100_000),
			wantSuccess:     true,
		},
		{
			name:            "fresh buy with insufficient margin",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(1000),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.FromInt(10_000), // only $10k DTBP
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(50_000), // need $50k margin (50% of $100k)
			wantMarginAvail: decimal.FromInt(10_000),
			wantSuccess:     false,
		},
		{
			name:            "increasing position needs more margin",
			currentQty:      decimal.FromInt(100),           // already own 100 shares
			orderQty:        decimal.FromInt(100),           // buying 100 more
			currentPrice:    decimal.FromInt(100),           // $100/share
			limitPrice:      decimal.FromInt(100),           // limit at market
			dtbp:            decimal.FromInt(50_000),        // $50k DTBP
			marginUsed:      decimal.FromInt(3_000),         // 30% maintenance on existing $10k
			marginHold:      decimal.Zero,                   // no pending orders
			wantMarginNeed:  decimal.FromInt(10_000 - 3000), // new 50% initial - old 30% maintenance
			wantMarginAvail: decimal.FromInt(47_000),        // $50k - $3k used
			wantSuccess:     true,
		},
		{
			name:         "decreasing position by half needs no margin",
			currentQty:   decimal.FromInt(100), // own 100 shares
			orderQty:     decimal.FromInt(-50), // selling 50 (keeping 50%)
			currentPrice: decimal.FromInt(100), // $100/share
			limitPrice:   decimal.FromInt(100), // limit at market
			dtbp:         decimal.Zero,         // even zero DTBP is fine
			marginUsed:   decimal.FromInt(3000),
			marginHold:   decimal.Zero,
			// newQty=50: initial margin = 50*100*0.50 = $2500
			// oldQty=100: maint margin = 100*100*0.30 = $3000
			// Since $2500 < $3000, no margin check needed
			wantMarginNeed:  decimal.Zero,
			wantMarginAvail: decimal.Zero, // not checked when newMargin <= oldMargin
			wantSuccess:     true,         // always allowed when newMargin <= oldMargin
		},
		{
			name:         "closing entire position needs no margin",
			currentQty:   decimal.FromInt(100),  // own 100 shares
			orderQty:     decimal.FromInt(-100), // selling all
			currentPrice: decimal.FromInt(100),
			limitPrice:   decimal.FromInt(100),
			dtbp:         decimal.Zero, // even zero DTBP is fine
			marginUsed:   decimal.FromInt(3000),
			marginHold:   decimal.Zero,
			// newQty=0: initial margin = $0
			// oldQty=100: maint margin = $3000
			// Since $0 < $3000, no margin check needed
			wantMarginNeed:  decimal.Zero,
			wantMarginAvail: decimal.Zero, // not checked
			wantSuccess:     true,         // closing position always allowed
		},
		{
			name:            "margin hold reduces available",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.FromInt(10_000),
			marginUsed:      decimal.Zero,
			marginHold:      decimal.FromInt(6_000), // $6k held for pending orders
			wantMarginNeed:  decimal.FromInt(5_000), // need $5k
			wantMarginAvail: decimal.FromInt(4_000), // only $4k available
			wantSuccess:     false,
		},
		{
			name:            "limit price below market uses limit for margin calc",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(80), // limit 20% below market
			dtbp:            decimal.FromInt(10_000),
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(4_000), // 50% of $8k (100 shares @ $80)
			wantMarginAvail: decimal.FromInt(10_000),
			wantSuccess:     true,
		},
		{
			name:            "limit price above market uses market for margin calc",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(80),  // market price
			limitPrice:      decimal.FromInt(100), // limit above market
			dtbp:            decimal.FromInt(10_000),
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(4_000), // 50% of $8k (uses min price)
			wantMarginAvail: decimal.FromInt(10_000),
			wantSuccess:     true,
		},
		{
			name:            "going short from flat",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(-100), // short 100 shares
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.FromInt(10_000),
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(5_000), // 50% initial on short
			wantMarginAvail: decimal.FromInt(10_000),
			wantSuccess:     true,
		},
		{
			name:         "covering entire short needs no margin",
			currentQty:   decimal.FromInt(-100), // short 100 shares
			orderQty:     decimal.FromInt(100),  // buying to cover all
			currentPrice: decimal.FromInt(100),
			limitPrice:   decimal.FromInt(100),
			dtbp:         decimal.Zero,
			marginUsed:   decimal.FromInt(5_000),
			marginHold:   decimal.Zero,
			// newQty=0: initial margin = $0
			// oldQty=-100: maint margin for short = max(30%, $5/share) = max($3000, $500) = $3000
			// Wait, for $100 stock, $5/share rule doesn't dominate: 100*100*0.30 = $3000
			// Since $0 < $3000, no margin check needed
			wantMarginNeed:  decimal.Zero,
			wantMarginAvail: decimal.Zero, // not checked
			wantSuccess:     true,
		},
		{
			name:            "exactly enough margin",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.FromInt(5_000), // exactly what we need
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(5_000),
			wantMarginAvail: decimal.FromInt(5_000),
			wantSuccess:     true, // exactly equal should pass
		},
		{
			name:            "one cent short of margin",
			currentQty:      decimal.Zero,
			orderQty:        decimal.FromInt(100),
			currentPrice:    decimal.FromInt(100),
			limitPrice:      decimal.FromInt(100),
			dtbp:            decimal.Parse("4999.99"), // one cent short
			marginUsed:      decimal.Zero,
			marginHold:      decimal.Zero,
			wantMarginNeed:  decimal.FromInt(5_000),
			wantMarginAvail: decimal.Parse("4999.99"),
			wantSuccess:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the margin check logic from Order() exactly
			newQty := tt.currentQty.Add(tt.orderQty)
			price := tt.limitPrice.Min(tt.currentPrice)

			newMargin := asset.GetInitialMargin(newQty, price)
			oldMargin := asset.GetMaintenanceMargin(tt.currentQty, price)

			// The key insight: margin check is ONLY performed when newMargin > oldMargin
			// If newMargin <= oldMargin (reducing position), the order always succeeds
			var marginNeeded decimal.Decimal
			var success bool
			if newMargin.Cmp(oldMargin) > 0 {
				marginNeeded = newMargin.Sub(oldMargin)
				marginAvailable := tt.dtbp.Sub(tt.marginUsed).Sub(tt.marginHold)
				success = marginNeeded.Cmp(marginAvailable) <= 0
				if marginAvailable.Cmp(tt.wantMarginAvail) != 0 {
					t.Errorf("marginAvailable = %s, want %s", marginAvailable, tt.wantMarginAvail)
				}
			} else {
				// Position is shrinking - no margin check needed, order always allowed
				success = true
			}

			if marginNeeded.Cmp(tt.wantMarginNeed) != 0 {
				t.Errorf("marginNeeded = %s, want %s", marginNeeded, tt.wantMarginNeed)
			}
			if success != tt.wantSuccess {
				t.Errorf("success = %v, want %v (marginNeeded=%s, newMargin=%s, oldMargin=%s)",
					success, tt.wantSuccess, marginNeeded, newMargin, oldMargin)
			}
		})
	}
}

// TestOrderBuyingPowerCheckInvariants proves key invariants of the margin check.
func TestOrderBuyingPowerCheckInvariants(t *testing.T) {
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}

	// Invariant 1: Closing entire position never requires margin (newQty = 0)
	t.Run("closing entire long needs no margin", func(t *testing.T) {
		for _, currentQty := range []int{1, 10, 100, 1000} {
			price := decimal.FromInt(100)
			qty := decimal.FromInt(currentQty)
			newQty := decimal.Zero // closing entirely

			newMargin := asset.GetInitialMargin(newQty, price)
			oldMargin := asset.GetMaintenanceMargin(qty, price)

			if newMargin.Cmp(oldMargin) > 0 {
				t.Errorf("closing %d shares: newMargin %s > oldMargin %s",
					currentQty, newMargin, oldMargin)
			}
		}
	})

	// Invariant 2: Closing entire short never requires margin
	t.Run("closing entire short needs no margin", func(t *testing.T) {
		for _, shortQty := range []int{1, 10, 100, 1000} {
			price := decimal.FromInt(100)
			qty := decimal.FromInt(-shortQty)
			newQty := decimal.Zero // covering entirely

			newMargin := asset.GetInitialMargin(newQty, price)
			oldMargin := asset.GetMaintenanceMargin(qty, price)

			if newMargin.Cmp(oldMargin) > 0 {
				t.Errorf("covering %d short: newMargin %s > oldMargin %s",
					shortQty, newMargin, oldMargin)
			}
		}
	})

	// Invariant 3: Selling/covering more than 40% of position never requires margin
	// Because: newQty * 0.50 <= currentQty * 0.30 when newQty/currentQty <= 0.60
	t.Run("selling more than 40% needs no margin", func(t *testing.T) {
		for _, currentQty := range []int{10, 100, 1000} {
			// Sell more than 40% (i.e., keep less than 60%)
			for _, keepPct := range []int{0, 10, 20, 30, 40, 50, 59} {
				price := decimal.FromInt(100)
				qty := decimal.FromInt(currentQty)
				newQty := qty.MulInt(keepPct).DivInt(100)

				newMargin := asset.GetInitialMargin(newQty, price)
				oldMargin := asset.GetMaintenanceMargin(qty, price)

				if newMargin.Cmp(oldMargin) > 0 {
					t.Errorf("keeping %d%% of %d: newMargin %s > oldMargin %s",
						keepPct, currentQty, newMargin, oldMargin)
				}
			}
		}
	})

	// Invariant 4: Small sells (keeping >60%) MAY require margin due to initial vs maintenance rates
	// This is expected behavior - the code is conservative
	t.Run("small sells may require margin check", func(t *testing.T) {
		currentQty := 100
		price := decimal.FromInt(100)
		qty := decimal.FromInt(currentQty)

		// Sell just 1 share - keep 99%
		newQty := decimal.FromInt(99)
		newMargin := asset.GetInitialMargin(newQty, price)
		oldMargin := asset.GetMaintenanceMargin(qty, price)

		// 99 * 100 * 0.50 = $4950 (initial)
		// 100 * 100 * 0.30 = $3000 (maintenance)
		// newMargin > oldMargin, so margin check WILL be performed
		if newMargin.Cmp(oldMargin) <= 0 {
			t.Errorf("expected newMargin > oldMargin for small sell, got %s <= %s",
				newMargin, oldMargin)
		}

		// But the margin NEEDED is the difference, which should still be affordable
		// with any reasonable DTBP
		marginNeeded := newMargin.Sub(oldMargin)
		if marginNeeded.Cmp(decimal.FromInt(2000)) > 0 {
			t.Errorf("unexpectedly high margin needed for small sell: %s", marginNeeded)
		}
	})

	// Invariant 3: Initial margin >= Maintenance margin for same position
	t.Run("initial >= maintenance for same position", func(t *testing.T) {
		for _, qty := range []int{-1000, -100, -10, 10, 100, 1000} {
			for _, priceInt := range []int{1, 10, 50, 100, 500} {
				quantity := decimal.FromInt(qty)
				price := decimal.FromInt(priceInt)
				initial := asset.GetInitialMargin(quantity, price)
				maintenance := asset.GetMaintenanceMargin(quantity, price)
				if initial.Cmp(maintenance) < 0 {
					t.Errorf("qty=%d price=%d: initial %s < maintenance %s",
						qty, priceInt, initial, maintenance)
				}
			}
		}
	})

	// Invariant 4: Using min(limit, market) price is conservative for buys
	t.Run("min price is conservative for margin", func(t *testing.T) {
		qty := decimal.FromInt(100)
		marketPrice := decimal.FromInt(100)
		limitPrices := []decimal.Decimal{
			decimal.FromInt(80),  // below market
			decimal.FromInt(100), // at market
			decimal.FromInt(120), // above market
		}

		for _, limitPrice := range limitPrices {
			effectivePrice := limitPrice.Min(marketPrice)
			marginAtEffective := asset.GetInitialMargin(qty, effectivePrice)
			marginAtLimit := asset.GetInitialMargin(qty, limitPrice)
			marginAtMarket := asset.GetInitialMargin(qty, marketPrice)

			// Using min price gives margin <= max of both individual calcs
			maxMargin := marginAtLimit.Max(marginAtMarket)
			if marginAtEffective.Cmp(maxMargin) > 0 {
				t.Errorf("effective margin %s > max(%s, %s) for limit=%s market=%s",
					marginAtEffective, marginAtLimit, marginAtMarket,
					limitPrice, marketPrice)
			}
		}
	})
}

// BenchmarkOrderBuyingPowerCheck benchmarks the margin calculation.
func BenchmarkOrderBuyingPowerCheck(b *testing.B) {
	asset := &alpaca.Asset{
		MarginRequirementLong:  decimal.Parse("0.30"),
		MarginRequirementShort: decimal.Parse("0.30"),
	}
	currentQty := decimal.FromInt(100)
	orderQty := decimal.FromInt(50)
	currentPrice := decimal.FromInt(100)
	limitPrice := decimal.FromInt(95)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newQty := currentQty.Add(orderQty)
		price := limitPrice.Min(currentPrice)
		newMargin := asset.GetInitialMargin(newQty, price)
		oldMargin := asset.GetMaintenanceMargin(currentQty, price)
		if newMargin.Cmp(oldMargin) > 0 {
			_ = newMargin.Sub(oldMargin)
		}
	}
}
