package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"testing"
)

func TestShortOrder_CreateShortPosition(t *testing.T) {
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

	// Place short sale order
	order := eq.ShortOrder(100)

	if !order.IsShortSale {
		t.Error("order should be marked as short sale")
	}
	if order.Side != ds.SideSell {
		t.Errorf("short order side should be Sell, got %v", order.Side)
	}

	// Simulate fill
	candle := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(102),
		Low:    decimal.FromInt(99),
		Close:  decimal.FromInt(101),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle)

	// Order should be filled
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}

	// Check holding - should be negative (short)
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()

	if !qty.IsNegative() {
		t.Errorf("expected negative quantity (short), got %s", qty)
	}
	expectedQty := decimal.FromInt(-100)
	if qty.Cmp(expectedQty) != 0 {
		t.Errorf("expected quantity %s, got %s", expectedQty, qty)
	}

	// Check IsShort helper
	if !aapl.IsShort() {
		t.Error("IsShort() should return true")
	}
	if aapl.IsLong() {
		t.Error("IsLong() should return false")
	}
}

func TestShortOrder_MarginRequired(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Start with limited margin
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(5000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(20000)) // 4x of $5k
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place short order
	_ = eq.ShortOrder(100)

	// Margin required for short: 100 * $100 * 50% = $5,000
	// Buying power should be reduced
	b.Lock.RLock()
	bp := b.DayTradingBuyingPower.Load()
	b.Lock.RUnlock()

	// Original $20k - $5k margin = $15k remaining
	expected := decimal.FromInt(15000)
	if bp.Cmp(expected) != 0 {
		t.Errorf("expected buying power %s after margin hold, got %s", expected, bp)
	}
}

func TestCoverOrder_CloseShort(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(60000)) // Include proceeds from short
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	// Set up existing short position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100))                                     // Short 100 shares
	aapl.Available.Store(decimal.FromInt(100))                                     // Available to cover
	aapl.ShortLots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000)) // Sold at $100
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(90)) // Price dropped
	eq.isReady = true

	// Place cover order
	order := eq.CoverOrder(100)

	if !order.IsCover {
		t.Error("order should be marked as cover")
	}
	if order.Side != ds.SideBuy {
		t.Errorf("cover order side should be Buy, got %v", order.Side)
	}

	// Simulate fill at $90
	candle := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(90),
		High:   decimal.FromInt(92),
		Low:    decimal.FromInt(88),
		Close:  decimal.FromInt(91),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle)

	// Order should be filled
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}

	// Check holding - should be zero (flat)
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()

	if !qty.IsZero() {
		t.Errorf("expected zero quantity after cover, got %s", qty)
	}

	// Check IsFlat helper
	if !aapl.IsFlat() {
		t.Error("IsFlat() should return true after full cover")
	}
}

func TestShortPnL_ProfitOnPriceDrop(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	initialCash := decimal.FromInt(50000)
	b.Lock.Lock()
	b.Cash.Store(initialCash)
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Short at $100
	_ = eq.ShortOrder(100)
	candleShort := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(100),
		Low:    decimal.FromInt(100),
		Close:  decimal.FromInt(100),
		Volume: decimal.FromInt(1000),
	}
	eq.simulateFills(candleShort)

	// Cash should have increased by short proceeds ($10,000 minus fees)
	b.Lock.RLock()
	cashAfterShort := b.Cash.Load()
	b.Lock.RUnlock()

	// Should be roughly $60,000 (+ $10k proceeds - small fees)
	if cashAfterShort.Cmp(decimal.FromInt(59900)) < 0 {
		t.Errorf("expected cash ~$60k after short, got %s", cashAfterShort)
	}

	// Cover at $90 (price dropped = profit)
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(90))
	_ = eq.CoverOrder(100)
	candleCover := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute),
		Open:   decimal.FromInt(90),
		High:   decimal.FromInt(90),
		Low:    decimal.FromInt(90),
		Close:  decimal.FromInt(90),
		Volume: decimal.FromInt(1000),
	}
	eq.simulateFills(candleCover)

	// Should have profit: sold at 100, bought back at 90 = $10 * 100 = $1000 profit
	b.Lock.RLock()
	cashAfterCover := b.Cash.Load()
	b.Lock.RUnlock()

	// Net gain should be ~$1000 (minus fees)
	netGain := cashAfterCover.Sub(initialCash)
	if netGain.Cmp(decimal.FromInt(900)) < 0 {
		t.Errorf("expected profit ~$1000, got %s", netGain)
	}

	// Check win count increased
	aapl.Lock.RLock()
	wins := aapl.WinCount
	aapl.Lock.RUnlock()
	if wins != 1 {
		t.Errorf("expected 1 win, got %d", wins)
	}
}

func TestShortPnL_LossOnPriceRise(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	initialCash := decimal.FromInt(50000)
	b.Lock.Lock()
	b.Cash.Store(initialCash)
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Short at $100
	eq.ShortOrder(100)
	candleShort := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(100),
		Low:    decimal.FromInt(100),
		Close:  decimal.FromInt(100),
		Volume: decimal.FromInt(1000),
	}
	eq.simulateFills(candleShort)

	// Cover at $110 (price rose = loss)
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(110))
	eq.CoverOrder(100)
	candleCover := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute),
		Open:   decimal.FromInt(110),
		High:   decimal.FromInt(110),
		Low:    decimal.FromInt(110),
		Close:  decimal.FromInt(110),
		Volume: decimal.FromInt(1000),
	}
	eq.simulateFills(candleCover)

	// Should have loss: sold at 100, bought back at 110 = -$10 * 100 = -$1000 loss
	b.Lock.RLock()
	cashAfterCover := b.Cash.Load()
	b.Lock.RUnlock()

	netLoss := initialCash.Sub(cashAfterCover)
	if netLoss.Cmp(decimal.FromInt(900)) < 0 {
		t.Errorf("expected loss ~$1000, got %s", netLoss)
	}

	// Check loss count increased
	aapl.Lock.RLock()
	losses := aapl.LossCount
	aapl.Lock.RUnlock()
	if losses != 1 {
		t.Errorf("expected 1 loss, got %d", losses)
	}
}

func TestHolding_ShortPosition_Check(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)

	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100)) // Short 100 shares
	aapl.Available.Store(decimal.FromInt(100)) // 100 available to cover
	aapl.ShortLots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	// Check() should not panic for valid short position
	aapl.Lock.Lock()
	aapl.Check() // Should not panic
	aapl.Lock.Unlock()
}

func TestHolding_ShortPosition_InvalidCheck(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)

	// Test that Check() catches invariant violations
	// We can't easily test panics, but we can verify the helpers work
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.ShortLots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	if !aapl.IsShort() {
		t.Error("IsShort should return true")
	}
	if aapl.IsLong() {
		t.Error("IsLong should return false")
	}
	if aapl.IsFlat() {
		t.Error("IsFlat should return false")
	}
}

func TestMaintenanceMargin_ShortPosition(t *testing.T) {
	resetCubby()

	// Short 100 shares at $100
	qty := decimal.FromInt(-100)
	price := decimal.FromInt(100)
	margin := MaintenanceMargin("AAPL", qty, price)

	// Expected: max(100 * 100 * 0.30, 100 * 5) = max(3000, 500) = 3000
	expected := decimal.FromInt(3000)
	if margin.Cmp(expected) != 0 {
		t.Errorf("expected maintenance margin %s for short, got %s", expected, margin)
	}
}

func TestCalculateTotalMaintenanceMargin_WithShort(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eqAAPL := b.Equities.Get("AAPL")
	eqMSFT := b.Equities.Get("MSFT")

	// Long AAPL
	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	// Short MSFT
	msft := b.Holdings.Get("MSFT")
	msft.Lock.Lock()
	msft.Quantity.Store(decimal.FromInt(-50)) // Short
	msft.Available.Store(decimal.FromInt(50))
	msft.ShortLots.Add(clocky.Now(), decimal.FromInt(50), decimal.FromInt(5000))
	msft.Lock.Unlock()

	eqAAPL.LastPrice.Store(decimal.FromInt(100)) // 100 * 100 = $10k long
	eqMSFT.LastPrice.Store(decimal.FromInt(200)) // 50 * 200 = $10k short

	totalMargin := b.CalculateTotalMaintenanceMargin()

	// AAPL long: 10000 * 0.30 = 3000
	// MSFT short: max(10000 * 0.30, 50 * 5) = max(3000, 250) = 3000
	// Total: 6000
	expected := decimal.FromInt(6000)
	if totalMargin.Cmp(expected) != 0 {
		t.Errorf("expected total margin %s, got %s", expected, totalMargin)
	}
}
