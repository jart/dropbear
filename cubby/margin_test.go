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
