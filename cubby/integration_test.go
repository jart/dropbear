package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"testing"
	"time"
)

// TestIntegration_DayTradingWithMargin tests a complete day trading scenario
// with 4x leverage, multiple trades, and proper DTBP tracking.
func TestIntegration_DayTradingWithMargin(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up PDT account with $50k
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	// Initialize DTBP (simulates market open)
	b.InitDTBP()

	// Verify 4x leverage available
	if b.GetLeverageMultiplier() != 4 {
		t.Fatalf("expected 4x leverage, got %dx", b.GetLeverageMultiplier())
	}

	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	b.Lock.RUnlock()
	if bodDTBP.Cmp(decimal.FromInt(200000)) != 0 {
		t.Fatalf("expected $200k DTBP, got %s", bodDTBP)
	}

	eq.LastPrice.Store(decimal.FromInt(150))
	eq.isReady = true

	// Day trade 1: Buy 100 shares at $150
	order1 := eq.MarketOrder(ds.SideBuy, 100)
	candle1 := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(150),
		High:   decimal.FromInt(152),
		Low:    decimal.FromInt(149),
		Close:  decimal.FromInt(151),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(candle1)

	if order1.State != ds.OrderStateFilled {
		t.Fatalf("order1 should be filled, got %v", order1.State)
	}

	// Verify we have the position
	aapl := b.Holdings.Get("AAPL")
	if aapl.Quantity.Load().Cmp(decimal.FromInt(100)) != 0 {
		t.Fatalf("expected 100 shares, got %s", aapl.Quantity.Load())
	}

	// Day trade 1: Sell at profit
	aapl.Lock.Lock()
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()
	eq.LastPrice.Store(decimal.FromInt(155))
	order2 := eq.MarketOrder(ds.SideSell, 100)
	candle2 := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute * 30),
		Open:   decimal.FromInt(155),
		High:   decimal.FromInt(156),
		Low:    decimal.FromInt(154),
		Close:  decimal.FromInt(155),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(candle2)

	if order2.State != ds.OrderStateFilled {
		t.Fatalf("order2 should be filled, got %v", order2.State)
	}

	// Position should be flat
	if !aapl.Quantity.Load().IsZero() {
		t.Fatalf("expected flat position, got %s", aapl.Quantity.Load())
	}

	// Should have made ~$500 profit (100 * $5)
	// (minus fees)
	aapl.Lock.RLock()
	wins := aapl.WinCount
	aapl.Lock.RUnlock()
	if wins != 1 {
		t.Errorf("expected 1 win, got %d", wins)
	}
}

// TestIntegration_OvernightPosition tests holding a position through close
// and verifies DTBP is properly locked for the next day.
func TestIntegration_OvernightPosition(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Start with $50k
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(50000)
	b.Lock.Unlock()

	b.InitDTBP()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Buy 200 shares at $100 = $20k position
	order := eq.MarketOrder(ds.SideBuy, 200)
	candle := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(102),
		Low:    decimal.FromInt(99),
		Close:  decimal.FromInt(101),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(candle)

	if order.State != ds.OrderStateFilled {
		t.Fatalf("order should be filled")
	}

	// End day trading time (15:50)
	b.EndDayTradingTime()

	// Verify 2x leverage now
	if b.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after 15:50, got %dx", b.GetLeverageMultiplier())
	}

	// Lock DTBP at close
	b.LockDTBP()

	// LastEquity should be reduced by maintenance margin
	// Position: 200 * $100 = $20k (using fill price estimate)
	// Maintenance: $20k * 30% = $6k
	// Expected LastEquity: ~$50k - $6k = $44k (approximately, fees affect this)
	b.Lock.RLock()
	lastEquity := b.LastEquity
	b.Lock.RUnlock()

	// Just verify it's less than starting equity due to maintenance margin
	if lastEquity.Cmp(decimal.FromInt(50000)) >= 0 {
		t.Errorf("LastEquity should be reduced by maintenance margin")
	}

	// Initialize next day
	b.InitDTBP()

	// Verify reduced DTBP due to overnight position
	b.Lock.RLock()
	newBodDTBP := b.BodDTBP
	b.Lock.RUnlock()

	// Should be less than $200k
	if newBodDTBP.Cmp(decimal.FromInt(200000)) >= 0 {
		t.Errorf("DTBP should be reduced due to overnight position, got %s", newBodDTBP)
	}
}

// TestIntegration_ShortSellingProfitable tests a profitable short trade.
func TestIntegration_ShortSellingProfitable(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Start with $50k
	initialCash := decimal.FromInt(50000)
	b.Lock.Lock()
	b.Cash.Store(initialCash)
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Short 100 shares at $100
	shortOrder := eq.ShortOrder(100)
	candle1 := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(101),
		Low:    decimal.FromInt(99),
		Close:  decimal.FromInt(100),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(candle1)

	if shortOrder.State != ds.OrderStateFilled {
		t.Fatalf("short order should be filled")
	}

	// Verify short position
	aapl := b.Holdings.Get("AAPL")
	if !aapl.IsShort() {
		t.Fatalf("should have short position")
	}

	// Price drops to $90 (profitable for short)
	eq.LastPrice.Store(decimal.FromInt(90))
	aapl.Lock.Lock()
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()

	// Cover at $90
	coverOrder := eq.CoverOrder(100)
	candle2 := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute * 30),
		Open:   decimal.FromInt(90),
		High:   decimal.FromInt(91),
		Low:    decimal.FromInt(89),
		Close:  decimal.FromInt(90),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(candle2)

	if coverOrder.State != ds.OrderStateFilled {
		t.Fatalf("cover order should be filled")
	}

	// Position should be flat
	if !aapl.IsFlat() {
		t.Fatalf("should be flat after cover")
	}

	// Should have profit
	b.Lock.RLock()
	finalCash := b.Cash.Load()
	b.Lock.RUnlock()

	profit := finalCash.Sub(initialCash)
	if !profit.IsPositive() {
		t.Errorf("expected profit from short trade, got %s", profit)
	}

	// Should have recorded a win
	aapl.Lock.RLock()
	wins := aapl.WinCount
	aapl.Lock.RUnlock()
	if wins != 1 {
		t.Errorf("expected 1 win, got %d", wins)
	}
}

// TestIntegration_MOCOrderExecution tests MOC order filling at close.
func TestIntegration_MOCOrderExecution(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Load Eastern Time location
	nyLoc, _ := time.LoadLocation("America/New_York")

	// Place MOC order
	mocOrder := eq.MOCOrder(ds.SideBuy, 100)

	// Simulate mid-day candle - should NOT fill
	midDayCandle := &indicators.Candle{
		Start:  clocky.Date(2025, time.January, 2, 14, 0, 0, 0, nyLoc), // 2pm ET
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(102),
		Low:    decimal.FromInt(99),
		Close:  decimal.FromInt(101),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(midDayCandle)

	if mocOrder.State == ds.OrderStateFilled {
		t.Error("MOC order should not fill mid-day")
	}

	// Simulate close candle - should fill
	closeCandle := &indicators.Candle{
		Start:  clocky.Date(2025, time.January, 2, 15, 59, 0, 0, nyLoc), // 3:59pm ET
		Open:   decimal.FromInt(101),
		High:   decimal.FromInt(103),
		Low:    decimal.FromInt(100),
		Close:  decimal.FromInt(102),
		Volume: decimal.FromInt(10000),
	}
	eq.simulateFills(closeCandle)

	if mocOrder.State != ds.OrderStateFilled {
		t.Errorf("MOC order should fill at close, got %v", mocOrder.State)
	}

	// Should have filled at close price
	if mocOrder.Price.Load().Cmp(decimal.FromInt(102)) != 0 {
		t.Errorf("expected fill at close price $102, got %s", mocOrder.Price.Load())
	}
}

// TestIntegration_MarginCallRecovery tests margin call triggering and recovery.
func TestIntegration_MarginCallRecovery(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Start with cash and a large leveraged position
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(20000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(80000))
	b.Lock.Unlock()

	// Set up leveraged position: 400 shares at $100 = $40k
	// This uses $20k cash + $20k margin
	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(400))
	aapl.Available.Store(decimal.FromInt(400))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(400), decimal.FromInt(40000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Now cash should be negative due to margin usage
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-20000)) // Borrowed $20k
	b.Lock.Unlock()

	// Initial state: Equity = -$20k + $40k = $20k
	// Maintenance = 400 * $100 * 0.30 = $12k
	// Equity > Maintenance, so no margin call yet

	triggered := b.CheckMarginCall()
	if triggered {
		t.Error("should not trigger margin call initially")
	}

	// Price drops to $35 - severe loss
	eq.LastPrice.Store(decimal.FromInt(35))

	// New state: Equity = -$20k + $14k = -$6k
	// Maintenance = 400 * $35 * 0.30 = $4.2k
	// Equity (-$6k) < Maintenance ($4.2k) - MARGIN CALL!

	triggered = b.CheckMarginCall()
	if !triggered {
		equity := b.Holdings.GetEquityUSD()
		maint := b.CalculateTotalMaintenanceMargin()
		t.Errorf("should trigger margin call, equity=%s, maint=%s", equity, maint)
	}

	// Verify margin call was recorded
	b.Lock.RLock()
	calls := b.MarginCallCount
	liquidated := b.LiquidatedValue
	b.Lock.RUnlock()

	if calls == 0 {
		t.Error("MarginCallCount should be > 0")
	}
	if !liquidated.IsPositive() {
		t.Error("LiquidatedValue should be > 0")
	}

	// Position should be reduced
	finalQty := aapl.Quantity.Load()
	if finalQty.Cmp(decimal.FromInt(400)) >= 0 {
		t.Errorf("position should be reduced, still have %s shares", finalQty)
	}
}

// TestIntegration_CompleteScenario tests a full trading day with all features.
func TestIntegration_CompleteScenario(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eqAAPL := b.Equities.Get("AAPL")
	eqMSFT := b.Equities.Get("MSFT")

	// Start of day: $100k cash, PDT account
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(100000))
	b.PatternDayTrader = true
	b.LastEquity = decimal.FromInt(100000)
	b.Lock.Unlock()

	b.InitDTBP()

	// Should have $400k DTBP
	b.Lock.RLock()
	bodDTBP := b.BodDTBP
	b.Lock.RUnlock()
	if bodDTBP.Cmp(decimal.FromInt(400000)) != 0 {
		t.Fatalf("expected $400k DTBP, got %s", bodDTBP)
	}

	eqAAPL.LastPrice.Store(decimal.FromInt(150))
	eqAAPL.isReady = true
	eqMSFT.LastPrice.Store(decimal.FromInt(400))
	eqMSFT.isReady = true

	// Trade 1: Long AAPL
	order1 := eqAAPL.MarketOrder(ds.SideBuy, 200)
	candle1 := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(150),
		High:   decimal.FromInt(152),
		Low:    decimal.FromInt(149),
		Close:  decimal.FromInt(151),
		Volume: decimal.FromInt(10000),
	}
	eqAAPL.simulateFills(candle1)

	if order1.State != ds.OrderStateFilled {
		t.Fatalf("AAPL buy should fill")
	}

	// Trade 2: Short MSFT
	order2 := eqMSFT.ShortOrder(50)
	candle2 := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute * 10),
		Open:   decimal.FromInt(400),
		High:   decimal.FromInt(402),
		Low:    decimal.FromInt(398),
		Close:  decimal.FromInt(401),
		Volume: decimal.FromInt(5000),
	}
	eqMSFT.simulateFills(candle2)

	if order2.State != ds.OrderStateFilled {
		t.Fatalf("MSFT short should fill")
	}

	// Verify positions
	aapl := b.Holdings.Get("AAPL")
	msft := b.Holdings.Get("MSFT")

	if !aapl.IsLong() {
		t.Error("should have long AAPL position")
	}
	if !msft.IsShort() {
		t.Error("should have short MSFT position")
	}

	// Check maintenance margin is being calculated
	margin := b.CalculateTotalMaintenanceMargin()
	if margin.IsZero() {
		t.Error("should have maintenance margin for positions")
	}

	// End day trading time
	b.EndDayTradingTime()

	// Verify 2x leverage
	if b.GetLeverageMultiplier() != 2 {
		t.Errorf("expected 2x leverage after close, got %dx", b.GetLeverageMultiplier())
	}

	// Lock DTBP for next day
	b.LockDTBP()

	b.Lock.RLock()
	lastEquity := b.LastEquity
	b.Lock.RUnlock()

	// LastEquity should be reduced by maintenance margin
	if lastEquity.Cmp(decimal.FromInt(100000)) >= 0 {
		t.Error("LastEquity should be less than initial due to maintenance margin")
	}
}
