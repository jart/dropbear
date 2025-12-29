package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"testing"
	"time"
)

func TestMaintenanceMargin_Long_DefaultRate(t *testing.T) {
	resetCubby()

	// 100 shares at $100, 30% margin rate
	qty := decimal.FromInt(100)
	price := decimal.FromInt(100)
	margin := MaintenanceMargin("AAPL", qty, price)

	// Expected: 100 * 100 * 0.30 = 3000
	expected := decimal.FromInt(3000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Long_LowPrice(t *testing.T) {
	resetCubby()

	// 100 shares at $2.00 (below $2.50 threshold) = 100% margin
	qty := decimal.FromInt(100)
	price := decimal.Parse("2.00")
	margin := MaintenanceMargin("PENNY", qty, price)

	// Expected: 100 * 2.00 = 200 (100% margin)
	expected := decimal.FromInt(200)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected 100%% margin %s for low-price stock, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Long_AtThreshold(t *testing.T) {
	resetCubby()

	// 100 shares at $2.50 (at threshold) = normal rate
	qty := decimal.FromInt(100)
	price := decimal.Parse("2.50")
	margin := MaintenanceMargin("AAPL", qty, price)

	// Expected: 100 * 2.50 * 0.30 = 75
	expected := decimal.Parse("75")
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s at threshold, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Short_DefaultRate(t *testing.T) {
	resetCubby()

	// Short 100 shares at $100 (negative quantity = short)
	qty := decimal.FromInt(-100)
	price := decimal.FromInt(100)
	margin := MaintenanceMargin("AAPL", qty, price)

	// Expected: max(100 * 100 * 0.30, 100 * 5.00) = max(3000, 500) = 3000
	expected := decimal.FromInt(3000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s for short, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Short_LowPrice(t *testing.T) {
	resetCubby()

	// Short 100 shares at $4.00 (below $5 threshold)
	qty := decimal.FromInt(-100)
	price := decimal.Parse("4.00")
	margin := MaintenanceMargin("PENNY", qty, price)

	// For stocks < $5: max(rate * value, $2.50/share, 100%)
	// value = 100 * 4 = 400
	// rate margin = 400 * 0.30 = 120
	// min per share = 100 * 2.50 = 250
	// 100% = 400
	// Expected: max(120, 250, 400) = 400
	expected := decimal.FromInt(400)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s for low-price short, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Short_MinPerShare(t *testing.T) {
	resetCubby()

	// Short 100 shares at $10 (>= $5)
	// This tests that the $5/share minimum kicks in when it's higher than rate
	qty := decimal.FromInt(-100)
	price := decimal.FromInt(10)
	margin := MaintenanceMargin("AAPL", qty, price)

	// For stocks >= $5: max(rate * value, $5/share)
	// value = 100 * 10 = 1000
	// rate margin = 1000 * 0.30 = 300
	// min per share = 100 * 5 = 500
	// Expected: max(300, 500) = 500
	expected := decimal.FromInt(500)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s with min per share, got %s", expected, margin)
	}
}

func TestMaintenanceMargin_Short_HighPrice(t *testing.T) {
	resetCubby()

	// Short 100 shares at $200
	// Rate margin should dominate over $5/share minimum
	qty := decimal.FromInt(-100)
	price := decimal.FromInt(200)
	margin := MaintenanceMargin("AAPL", qty, price)

	// value = 100 * 200 = 20000
	// rate margin = 20000 * 0.30 = 6000
	// min per share = 100 * 5 = 500
	// Expected: max(6000, 500) = 6000
	expected := decimal.FromInt(6000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s for high-price short, got %s", expected, margin)
	}
}

func TestInitialMargin_RegT50Percent(t *testing.T) {
	resetCubby()

	// 100 shares at $100, initial margin should be at least 50%
	qty := decimal.FromInt(100)
	price := decimal.FromInt(100)
	margin := InitialMargin("AAPL", qty, price)

	// Expected: 100 * 100 * 0.50 = 5000 (Reg-T minimum)
	expected := decimal.FromInt(5000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected initial margin %s (50%% Reg-T), got %s", expected, margin)
	}
}

func TestInitialMargin_MaxOfMaintenanceAnd50(t *testing.T) {
	resetCubby()

	// For a stock with maintenance > 50%, initial should equal maintenance
	// We can't easily mock the API, but we can verify the formula
	qty := decimal.FromInt(100)
	price := decimal.FromInt(100)

	maint := MaintenanceMargin("AAPL", qty, price)
	init := InitialMargin("AAPL", qty, price)

	// Initial should be >= maintenance
	if init.Cmp(maint) < 0 {
		t.Errorf("initial margin %s should be >= maintenance margin %s", init, maint)
	}

	// For 30% maintenance stock, initial should be 50% (Reg-T)
	value := qty.Mul(price)
	fiftyPercent := value.Mul(decimal.Half)
	if init.Cmp(fiftyPercent) < 0 {
		t.Errorf("initial margin %s should be >= 50%% of value %s", init, fiftyPercent)
	}
}

func TestInitialMargin_Short(t *testing.T) {
	resetCubby()

	// Short 100 shares at $100
	qty := decimal.FromInt(-100)
	price := decimal.FromInt(100)
	margin := InitialMargin("AAPL", qty, price)

	// Expected: max(100 * 100 * 0.50, 100 * 5.00) = max(5000, 500) = 5000
	expected := decimal.FromInt(5000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected initial margin %s for short, got %s", expected, margin)
	}
}

func TestInitialMargin_ZeroQuantity(t *testing.T) {
	resetCubby()

	qty := decimal.Zero
	price := decimal.FromInt(100)
	margin := InitialMargin("AAPL", qty, price)

	if !margin.IsZero() {
		t.Errorf("expected zero margin for zero quantity, got %s", margin)
	}
}

func TestMarginInterest_DailyCharge(t *testing.T) {
	resetCubby()

	// Create margin interest tracker with 1% spread over RFR
	mi := NewMarginInterest(100) // 100 bps = 1%

	// Simulate a negative cash balance of $10,000
	cashBalance := decimal.FromInt(-10000)

	// Wednesday charge
	loc, _ := time.LoadLocation("America/New_York")
	wed := time.Date(2025, 1, 8, 16, 0, 0, 0, loc) // Wednesday
	wedTime := clocky.Time(wed.UnixMicro())

	charge := mi.ChargeDaily(wedTime, cashBalance)

	// Daily rate = (RFR + 1%) / 360
	// For simplicity, assume RFR ~= 4.5%, total ~5.5%
	// Daily charge ~= 10000 * 0.055 / 360 ~= $1.53

	// Just verify charge is positive and reasonable
	if !charge.IsPositive() {
		t.Error("expected positive margin interest charge")
	}
	if charge.Cmp(decimal.FromInt(5)) > 0 {
		t.Errorf("charge %s seems too high for $10k debit", charge)
	}
}

func TestMarginInterest_FridayTriple(t *testing.T) {
	resetCubby()

	mi := NewMarginInterest(100) // 100 bps = 1%
	cashBalance := decimal.FromInt(-10000)

	loc, _ := time.LoadLocation("America/New_York")

	// Wednesday charge
	wed := time.Date(2025, 1, 8, 16, 0, 0, 0, loc) // Wednesday
	wedTime := clocky.Time(wed.UnixMicro())
	wedCharge := mi.ChargeDaily(wedTime, cashBalance)

	// Friday charge (different week to avoid duplicate day check)
	fri := time.Date(2025, 1, 17, 16, 0, 0, 0, loc) // Friday
	friTime := clocky.Time(fri.UnixMicro())
	friCharge := mi.ChargeDaily(friTime, cashBalance)

	// Friday should be 3x Wednesday
	expectedFriCharge := wedCharge.MulInt(3)
	if friCharge.Cmp(expectedFriCharge) != 0 {
		t.Errorf("Friday charge %s should be 3x Wednesday charge %s (expected %s)",
			friCharge, wedCharge, expectedFriCharge)
	}
}

func TestMarginInterest_NoDuplicateCharge(t *testing.T) {
	resetCubby()

	mi := NewMarginInterest(100)
	cashBalance := decimal.FromInt(-10000)

	loc, _ := time.LoadLocation("America/New_York")
	wed := time.Date(2025, 1, 8, 16, 0, 0, 0, loc)
	wedTime := clocky.Time(wed.UnixMicro())

	// First charge
	charge1 := mi.ChargeDaily(wedTime, cashBalance)

	// Second charge on same day should be zero
	charge2 := mi.ChargeDaily(wedTime, cashBalance)

	if !charge1.IsPositive() {
		t.Error("first charge should be positive")
	}
	if !charge2.IsZero() {
		t.Errorf("duplicate charge should be zero, got %s", charge2)
	}
}

func TestMarginInterest_PositiveBalance(t *testing.T) {
	resetCubby()

	mi := NewMarginInterest(100)
	cashBalance := decimal.FromInt(10000) // Positive balance

	loc, _ := time.LoadLocation("America/New_York")
	wed := time.Date(2025, 1, 8, 16, 0, 0, 0, loc)
	wedTime := clocky.Time(wed.UnixMicro())

	charge := mi.ChargeDaily(wedTime, cashBalance)

	// No charge for positive balance
	if !charge.IsZero() {
		t.Errorf("expected zero charge for positive balance, got %s", charge)
	}
}

func TestMarginInterest_Tracking(t *testing.T) {
	resetCubby()

	mi := NewMarginInterest(100)
	cashBalance := decimal.FromInt(-10000)

	loc, _ := time.LoadLocation("America/New_York")

	// Charge on multiple days
	day1 := time.Date(2025, 1, 6, 16, 0, 0, 0, loc) // Monday
	day2 := time.Date(2025, 1, 7, 16, 0, 0, 0, loc) // Tuesday

	mi.ChargeDaily(clocky.Time(day1.UnixMicro()), cashBalance)
	mi.ChargeDaily(clocky.Time(day2.UnixMicro()), cashBalance)

	total := mi.GetTotalCharged()
	yearly := mi.GetYearlyCharged(2025)

	if !total.IsPositive() {
		t.Error("total charged should be positive after two days")
	}
	if total.Cmp(yearly) != 0 {
		t.Errorf("total %s should equal yearly %s for single year", total, yearly)
	}
}

func TestIsMarginable(t *testing.T) {
	resetCubby()

	// AAPL should be marginable
	if !IsMarginable("AAPL") {
		t.Error("AAPL should be marginable")
	}
}

func TestIsShortable(t *testing.T) {
	resetCubby()

	// AAPL should be shortable
	if !IsShortable("AAPL") {
		t.Error("AAPL should be shortable")
	}
}
