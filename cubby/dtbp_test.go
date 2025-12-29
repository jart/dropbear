package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"testing"
	"time"
)

func TestDTBP_PDTAccount_4xIntraday(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	// Set $50,000 initial balance (above $25k PDT threshold)
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(50000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.Lock.Unlock()

	// Initialize DTBP (simulates market open)
	ex.InitDTBP()

	// Should have 4x leverage
	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	isDayTradingTime := ex.IsDayTradingTime
	ex.Lock.RUnlock()

	// Expected: $50k * 4 = $200k
	expected := decimal.FromInt(200000)
	if bodDTBP.Cmp(expected) != 0 {
		t.Errorf("expected BodDTBP %s, got %s", expected, bodDTBP)
	}
	if !isDayTradingTime {
		t.Error("IsDayTradingTime should be true for PDT account")
	}

	// Verify leverage multiplier
	if ex.GetLeverageMultiplier() != 4 {
		t.Errorf("expected 4x leverage, got %dx", ex.GetLeverageMultiplier())
	}
}

func TestDTBP_PDTAccount_2xOvernight(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(50000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.IsDayTradingTime = true
	ex.Lock.Unlock()

	// End day trading time (simulates 15:50)
	ex.EndDayTradingTime()

	// Should now be 2x leverage only
	ex.Lock.RLock()
	isDayTradingTime := ex.IsDayTradingTime
	ex.Lock.RUnlock()

	if isDayTradingTime {
		t.Error("IsDayTradingTime should be false after EndDayTradingTime")
	}

	// Verify leverage multiplier is now 2x
	if ex.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after close, got %dx", ex.GetLeverageMultiplier())
	}
}

func TestDTBP_NonPDT_2xAlways(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	// Set $50,000 but NOT PDT
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(50000))
	ex.PatternDayTrader = false
	ex.LastEquity = decimal.FromInt(50000)
	ex.Lock.Unlock()

	// Initialize DTBP
	ex.InitDTBP()

	// Should have 2x leverage only (non-PDT)
	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	isDayTradingTime := ex.IsDayTradingTime
	ex.Lock.RUnlock()

	// Expected: $50k * 2 = $100k
	expected := decimal.FromInt(100000)
	if bodDTBP.Cmp(expected) != 0 {
		t.Errorf("expected BodDTBP %s for non-PDT, got %s", expected, bodDTBP)
	}
	if isDayTradingTime {
		t.Error("IsDayTradingTime should be false for non-PDT account")
	}
}

func TestDTBP_Under25k_No4x(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	// Set $20,000 (below $25k PDT threshold)
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(20000))
	ex.PatternDayTrader = true // Even if flagged as PDT
	ex.LastEquity = decimal.FromInt(20000)
	ex.Lock.Unlock()

	// Initialize DTBP
	ex.InitDTBP()

	// Should have 2x leverage (under $25k threshold)
	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	isDayTradingTime := ex.IsDayTradingTime
	ex.Lock.RUnlock()

	// Expected: $20k * 2 = $40k (no 4x because under $25k)
	expected := decimal.FromInt(40000)
	if bodDTBP.Cmp(expected) != 0 {
		t.Errorf("expected BodDTBP %s for under $25k, got %s", expected, bodDTBP)
	}
	if isDayTradingTime {
		t.Error("IsDayTradingTime should be false when under $25k")
	}
}

func TestDTBP_LockAtClose(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balance and position
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(30000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(15000))
	aapl.Lock.Unlock()

	// Set AAPL price to $200 = 100 * 200 = $20,000 value
	eq.LastPrice.Store(decimal.FromInt(200))

	// Lock DTBP at close
	ex.LockDTBP()

	// Total equity = $30k cash + $20k stock = $50k
	// Maintenance margin = 100 * 200 * 0.30 = $6,000
	// LastEquity = $50k - $6k = $44k
	ex.Lock.RLock()
	lastEquity := ex.LastEquity
	ex.Lock.RUnlock()

	expected := decimal.FromInt(44000)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s after lock, got %s", expected, lastEquity)
	}
}

func TestDTBP_10MinBeforeClose_SwitchTo2x(t *testing.T) {
	resetCubby()
	Paper = true

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(50000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.IsDayTradingTime = true
	ex.Lock.Unlock()

	// Verify 4x before
	if ex.GetLeverageMultiplier() != 4 {
		t.Errorf("expected 4x leverage before 15:50, got %dx", ex.GetLeverageMultiplier())
	}

	// Simulate 15:50 callback
	ex.EndDayTradingTime()

	// Should be 2x after
	if ex.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after 15:50, got %dx", ex.GetLeverageMultiplier())
	}
}

func TestDTBP_HoldingOvernight_Reduces4x(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Start with $50k cash, hold $25k in stock
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(25000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000) // Initial equity
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(250))
	aapl.Available.Store(decimal.FromInt(250))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(250), decimal.FromInt(25000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100)) // 250 * $100 = $25k

	// Lock DTBP at EOD (holding position overnight)
	ex.LockDTBP()

	// Total equity = $25k cash + $25k stock = $50k
	// Maintenance margin = 250 * 100 * 0.30 = $7,500
	// LastEquity = $50k - $7.5k = $42,500
	ex.Lock.RLock()
	lastEquity := ex.LastEquity
	ex.Lock.RUnlock()

	expected := decimal.FromInt(42500)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s, got %s", expected, lastEquity)
	}

	// Initialize next day
	ex.InitDTBP()

	// DTBP for next day = $42,500 * 4 = $170,000 (reduced from $200k)
	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	ex.Lock.RUnlock()

	expectedDTBP := decimal.FromInt(170000)
	if bodDTBP.Cmp(expectedDTBP) != 0 {
		t.Errorf("expected next day BodDTBP %s, got %s", expectedDTBP, bodDTBP)
	}
}

func TestDTBP_ZeroAtClose_Full4xNextDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)

	// Flat at close (no positions)
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(50000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.Lock.Unlock()

	// No holdings
	ex.LockDTBP()

	// Total equity = $50k cash, maintenance = 0
	// LastEquity = $50k - 0 = $50k
	ex.Lock.RLock()
	lastEquity := ex.LastEquity
	ex.Lock.RUnlock()

	expected := decimal.FromInt(50000)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s, got %s", expected, lastEquity)
	}

	// Initialize next day
	ex.InitDTBP()

	// Full 4x = $50k * 4 = $200k
	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	ex.Lock.RUnlock()

	expectedDTBP := decimal.FromInt(200000)
	if bodDTBP.Cmp(expectedDTBP) != 0 {
		t.Errorf("expected BodDTBP %s, got %s", expectedDTBP, bodDTBP)
	}
}

func TestDTBP_50PercentAtClose_Half4xNextDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// $25k cash, $25k in stock = 50% invested
	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(25000))
	ex.PatternDayTrader = true
	ex.LastEquity = decimal.FromInt(50000)
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(250))
	aapl.Available.Store(decimal.FromInt(250))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(250), decimal.FromInt(25000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))

	ex.LockDTBP()
	ex.InitDTBP()

	ex.Lock.RLock()
	bodDTBP := ex.BodDTBP
	ex.Lock.RUnlock()

	// Equity = $50k, Maintenance = $7.5k
	// Available for next day = $50k - $7.5k = $42.5k
	// DTBP = $42.5k * 4 = $170k
	expected := decimal.FromInt(170000)
	if bodDTBP.Cmp(expected) != 0 {
		t.Errorf("expected BodDTBP %s for 50%% position, got %s", expected, bodDTBP)
	}
}

func TestCalculateTotalMaintenanceMargin(t *testing.T) {
	resetCubby()
	Paper = true

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eqAAPL := ex.Equities.Get("AAPL")
	eqMSFT := ex.Equities.Get("MSFT")

	// Set up holdings
	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	msft := ex.Holdings.Get("MSFT")
	msft.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	msft.Lock.Lock()
	msft.Quantity.Store(decimal.FromInt(50))
	msft.Available.Store(decimal.FromInt(50))
	msft.Lots.Add(clocky.Now(), decimal.FromInt(50), decimal.FromInt(5000))
	msft.Lock.Unlock()

	eqAAPL.LastPrice.Store(decimal.FromInt(150)) // 100 * 150 = $15k
	eqMSFT.LastPrice.Store(decimal.FromInt(400)) // 50 * 400 = $20k

	totalMargin := ex.CalculateTotalMaintenanceMargin()

	// AAPL: 15000 * 0.30 = 4500
	// MSFT: 20000 * 0.30 = 6000
	// Total: 10500
	expected := decimal.FromInt(10500)
	if totalMargin.Cmp(expected) != 0 {
		t.Errorf("expected total maintenance margin %s, got %s", expected, totalMargin)
	}
}

func TestSchedule_CloseEarly_Fires(t *testing.T) {
	resetCubby()

	var closeEarlyFired bool
	BeforeCloseEarly(func() {
		closeEarlyFired = true
	})

	loc, _ := time.LoadLocation("America/New_York")
	// 15:50 ET
	closeEarlyTime := time.Date(2025, 1, 2, 15, 50, 0, 0, loc)
	checkSchedule(clocky.Time(closeEarlyTime.UnixMicro()))

	if !closeEarlyFired {
		t.Error("BeforeCloseEarly callback should have fired at 15:50")
	}
}

func TestSchedule_CloseEarly_DoesNotFireEarly(t *testing.T) {
	resetCubby()

	var closeEarlyFired bool
	BeforeCloseEarly(func() {
		closeEarlyFired = true
	})

	loc, _ := time.LoadLocation("America/New_York")
	// 15:49 ET - one minute before
	beforeTime := time.Date(2025, 1, 2, 15, 49, 0, 0, loc)
	checkSchedule(clocky.Time(beforeTime.UnixMicro()))

	if closeEarlyFired {
		t.Error("BeforeCloseEarly should not fire at 15:49")
	}
}
