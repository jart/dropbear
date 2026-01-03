package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"testing"
	"time"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

func TestDTBP_PDTAccount_4xIntraday(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)

	// Set $50,000 initial balance (above $25k PDT threshold)
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	// Initialize DTBP (simulates market open)
	b.InitDTBP()

	// Should have 4x leverage
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	isDayTradingTime := b.IsDayTradingTime
	b.Lock.RUnlock()

	// Expected: $50k * 4 = $200k
	expected := decimal.FromInt(200000)
	if bodDTBP.Cmp(expected) != 0 {
		t.Errorf("expected BodDTBP %s, got %s", expected, bodDTBP)
	}
	if !isDayTradingTime {
		t.Error("IsDayTradingTime should be true for PDT account")
	}

	// Verify leverage multiplier
	if b.GetLeverageMultiplier() != 4 {
		t.Errorf("expected 4x leverage, got %dx", b.GetLeverageMultiplier())
	}
}

func TestDTBP_PDTAccount_2xOvernight(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.IsDayTradingTime = true
	b.Lock.Unlock()

	// End day trading time (simulates 15:50)
	b.EndDayTradingTime()

	// Should now be 2x leverage only
	b.Lock.RLock()
	isDayTradingTime := b.IsDayTradingTime
	b.Lock.RUnlock()

	if isDayTradingTime {
		t.Error("IsDayTradingTime should be false after EndDayTradingTime")
	}

	// Verify leverage multiplier is now 2x
	if b.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after close, got %dx", b.GetLeverageMultiplier())
	}
}

func TestDTBP_NonPDT_2xAlways(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)

	// Set $50,000 but NOT PDT
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = false
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	// Initialize DTBP
	b.InitDTBP()

	// Should have 2x leverage only (non-PDT)
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	isDayTradingTime := b.IsDayTradingTime
	b.Lock.RUnlock()

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

	b := Brokers.Get(ds.BrokerAlpaca)

	// Set $20,000 (below $25k PDT threshold)
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(20000))
	b.PatternDayTrader = true // Even if flagged as PDT
	b.LastEquity = decimal.FromInt(20000)
	b.Lock.Unlock()

	// Initialize DTBP
	b.InitDTBP()

	// Should have 2x leverage (under $25k threshold)
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	isDayTradingTime := b.IsDayTradingTime
	b.Lock.RUnlock()

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

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set initial balance and position
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(30000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(15000))
	aapl.Lock.Unlock()

	// Set AAPL price to $200 = 100 * 200 = $20,000 value
	eq.LastPrice.Store(decimal.FromInt(200))

	// Lock DTBP at close
	b.LockDTBP()

	// Total equity = $30k cash + $20k stock = $50k
	// Maintenance margin = 100 * 200 * 0.30 = $6,000
	// LastEquity = $50k - $6k = $44k
	b.Lock.RLock()
	lastEquity := b.LastEquity
	b.Lock.RUnlock()

	expected := decimal.FromInt(44000)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s after lock, got %s", expected, lastEquity)
	}
}

func TestDTBP_10MinBeforeClose_SwitchTo2x(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.IsDayTradingTime = true
	b.Lock.Unlock()

	// Verify 4x before
	if b.GetLeverageMultiplier() != 4 {
		t.Errorf("expected 4x leverage before 15:50, got %dx", b.GetLeverageMultiplier())
	}

	// Simulate 15:50 callback
	b.EndDayTradingTime()

	// Should be 2x after
	if b.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after 15:50, got %dx", b.GetLeverageMultiplier())
	}
}

func TestDTBP_HoldingOvernight_Reduces4x(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Start with $50k cash, hold $25k in stock
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(25000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000) // Initial equity
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(250))
	aapl.Available.Store(decimal.FromInt(250))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(250), decimal.FromInt(25000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100)) // 250 * $100 = $25k

	// Lock DTBP at EOD (holding position overnight)
	b.LockDTBP()

	// Total equity = $25k cash + $25k stock = $50k
	// Maintenance margin = 250 * 100 * 0.30 = $7,500
	// LastEquity = $50k - $7.5k = $42,500
	b.Lock.RLock()
	lastEquity := b.LastEquity
	b.Lock.RUnlock()

	expected := decimal.FromInt(42500)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s, got %s", expected, lastEquity)
	}

	// Initialize next day
	b.InitDTBP()

	// DTBP for next day = $42,500 * 4 = $170,000 (reduced from $200k)
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	b.Lock.RUnlock()

	expectedDTBP := decimal.FromInt(170000)
	if bodDTBP.Cmp(expectedDTBP) != 0 {
		t.Errorf("expected next day BodDTBP %s, got %s", expectedDTBP, bodDTBP)
	}
}

func TestDTBP_ZeroAtClose_Full4xNextDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)

	// Flat at close (no positions)
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	// No holdings
	b.LockDTBP()

	// Total equity = $50k cash, maintenance = 0
	// LastEquity = $50k - 0 = $50k
	b.Lock.RLock()
	lastEquity := b.LastEquity
	b.Lock.RUnlock()

	expected := decimal.FromInt(50000)
	if lastEquity.Cmp(expected) != 0 {
		t.Errorf("expected LastEquity %s, got %s", expected, lastEquity)
	}

	// Initialize next day
	b.InitDTBP()

	// Full 4x = $50k * 4 = $200k
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	b.Lock.RUnlock()

	expectedDTBP := decimal.FromInt(200000)
	if bodDTBP.Cmp(expectedDTBP) != 0 {
		t.Errorf("expected BodDTBP %s, got %s", expectedDTBP, bodDTBP)
	}
}

func TestDTBP_50PercentAtClose_Half4xNextDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// $25k cash, $25k in stock = 50% invested
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(25000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(250))
	aapl.Available.Store(decimal.FromInt(250))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(250), decimal.FromInt(25000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))

	b.LockDTBP()
	b.InitDTBP()

	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	b.Lock.RUnlock()

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

	b := Brokers.Get(ds.BrokerAlpaca)
	eqAAPL := b.Equities.Get("AAPL")
	eqMSFT := b.Equities.Get("MSFT")

	// Set up holdings
	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	msft := b.Holdings.Get("MSFT")
	msft.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	msft.Lock.Lock()
	msft.Quantity.Store(decimal.FromInt(50))
	msft.Available.Store(decimal.FromInt(50))
	msft.Lots.Add(clocky.Now(), decimal.FromInt(50), decimal.FromInt(5000))
	msft.Lock.Unlock()

	eqAAPL.LastPrice.Store(decimal.FromInt(150)) // 100 * 150 = $15k
	eqMSFT.LastPrice.Store(decimal.FromInt(400)) // 50 * 400 = $20k

	totalMargin := b.CalculateTotalMaintenanceMargin()

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

	// Set range to include 12:50 PT on Jan 2 (close early time)
	start := time.Date(2025, 1, 2, 6, 30, 0, 0, Pacific)
	end := time.Date(2025, 1, 2, 13, 0, 0, 0, Pacific)

	m := &manager{
		heap:  binaryheap.NewWith(compareHeapEntries),
		start: clocky.Time(start.UnixMicro()),
		end:   clocky.Time(end.UnixMicro()),
	}
	m.generateScheduledEvents()

	// Find and execute the beforeCloseEarly event
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		if entry.callback != nil {
			entry.callback()
		}
	}

	if !closeEarlyFired {
		t.Error("BeforeCloseEarly callback should have fired")
	}
}

func TestSchedule_CloseEarly_DoesNotFireEarly(t *testing.T) {
	resetCubby()

	var closeEarlyFired bool
	BeforeCloseEarly(func() {
		closeEarlyFired = true
	})

	// Set range to end at 12:49 PT - before the 12:50 close early event
	start := time.Date(2025, 1, 2, 6, 30, 0, 0, Pacific)
	end := time.Date(2025, 1, 2, 12, 49, 0, 0, Pacific)

	m := &manager{
		heap:  binaryheap.NewWith(compareHeapEntries),
		start: clocky.Time(start.UnixMicro()),
		end:   clocky.Time(end.UnixMicro()),
	}
	m.generateScheduledEvents()

	// Execute all events in range
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		if entry.callback != nil {
			entry.callback()
		}
	}

	if closeEarlyFired {
		t.Error("BeforeCloseEarly should not fire when range ends at 12:49")
	}
}
