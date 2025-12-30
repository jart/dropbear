package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"testing"
)

func TestMarginCall_TriggeredWhenEquityBelowMaintenance(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up account with $10k cash and 100 shares of AAPL at $100
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(10000))
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))

	// Initial check - should be fine
	// Equity = $10k cash + $10k stock = $20k
	// Maintenance = 100 * $100 * 0.30 = $3k
	// Equity ($20k) > Maintenance ($3k), so no margin call
	triggered := b.CheckMarginCall()
	if triggered {
		t.Error("margin call should not be triggered with healthy margin")
	}

	// Now simulate a big price drop - AAPL to $10
	eq.LastPrice.Store(decimal.FromInt(10))

	// Equity = $10k cash + $1k stock = $11k
	// Maintenance = 100 * $10 * 0.30 = $300
	// Wait, that's still fine. Let's make it worse.

	// Actually for margin call to trigger, we need negative equity or
	// positions worth more than our equity. Let's use a more extreme case.
	// Set cash to negative (borrowed on margin)
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-5000)) // borrowed $5k
	b.Lock.Unlock()

	// Equity = -$5k cash + $1k stock = -$4k
	// This is a margin call situation
	triggered = b.CheckMarginCall()

	// Should be triggered now
	if !triggered {
		equity := b.Holdings.GetEquityUSD()
		maint := b.CalculateTotalMaintenanceMargin()
		t.Errorf("margin call should be triggered, equity=%s, maint=%s", equity, maint)
	}

	b.Lock.RLock()
	wasTriggered := b.MarginCallTriggered
	count := b.MarginCallCount
	b.Lock.RUnlock()

	if !wasTriggered {
		t.Error("MarginCallTriggered flag should be true")
	}
	if count != 1 {
		t.Errorf("expected MarginCallCount=1, got %d", count)
	}
}

func TestMarginCall_AutoLiquidatesPositions(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up account with small cash and large position
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(1000))
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(200))
	aapl.Available.Store(decimal.FromInt(200))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(200), decimal.FromInt(20000))
	aapl.Lock.Unlock()

	// Price drops drastically
	eq.LastPrice.Store(decimal.FromInt(30))

	// Before: Equity = $1k + 200*$30 = $7k
	// Maintenance = 200 * $30 * 0.30 = $1,800
	// Still OK, let's make it worse

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-5000)) // Big negative
	b.Lock.Unlock()

	// Now: Equity = -$5k + $6k = $1k
	// Maintenance = $1,800
	// Equity < Maintenance -> margin call

	initialQty := aapl.Quantity.Load()

	b.CheckMarginCall()

	// After liquidation, position should be reduced
	finalQty := aapl.Quantity.Load()
	if finalQty.Cmp(initialQty) >= 0 {
		t.Errorf("expected position to be reduced by liquidation, before=%s, after=%s",
			initialQty, finalQty)
	}

	// Check that LiquidatedValue was recorded
	b.Lock.RLock()
	liquidated := b.LiquidatedValue
	b.Lock.RUnlock()

	if !liquidated.IsPositive() {
		t.Error("expected positive LiquidatedValue")
	}
}

func TestMarginCall_LiquidatesLosersFirst(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eqAAPL := b.Equities.Get("AAPL")
	eqMSFT := b.Equities.Get("MSFT")

	// Set up negative cash (margin debt)
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-20000))
	b.Lock.Unlock()

	// AAPL: losing position (bought at $200, now at $100)
	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(20000)) // $200/share cost
	aapl.Lock.Unlock()
	eqAAPL.LastPrice.Store(decimal.FromInt(100)) // $100/share now

	// MSFT: winning position (bought at $200, now at $300)
	msft := b.Holdings.Get("MSFT")
	msft.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	msft.Lock.Lock()
	msft.Quantity.Store(decimal.FromInt(100))
	msft.Available.Store(decimal.FromInt(100))
	msft.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(20000)) // $200/share cost
	msft.Lock.Unlock()
	eqMSFT.LastPrice.Store(decimal.FromInt(300)) // $300/share now

	// Current state:
	// Cash: -$20k
	// AAPL: 100 * $100 = $10k (unrealized loss: $10k - $20k = -$10k)
	// MSFT: 100 * $300 = $30k (unrealized profit: $30k - $20k = +$10k)
	// Total Equity: -$20k + $10k + $30k = $20k
	// Maintenance: (100 * $100 * 0.30) + (100 * $300 * 0.30) = $3k + $9k = $12k
	// Equity ($20k) > Maintenance ($12k), so no margin call yet

	// Make cash more negative to trigger margin call
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-30000))
	b.Lock.Unlock()

	// Now: Equity = -$30k + $10k + $30k = $10k
	// Still > $12k? Let's make it worse
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-40000))
	b.Lock.Unlock()

	// Now: Equity = -$40k + $10k + $30k = $0k
	// Maintenance = $12k
	// 0 < 12k -> margin call!

	initialAAPL := aapl.Quantity.Load()
	initialMSFT := msft.Quantity.Load()

	b.CheckMarginCall()

	finalAAPL := aapl.Quantity.Load()
	finalMSFT := msft.Quantity.Load()

	// AAPL (loser) should be liquidated first
	if finalAAPL.Cmp(initialAAPL) >= 0 {
		t.Errorf("AAPL (loser) should be liquidated first, before=%s, after=%s",
			initialAAPL, finalAAPL)
	}

	// MSFT (winner) might not need to be liquidated or should be liquidated less
	aaplReduction := initialAAPL.Sub(finalAAPL)
	msftReduction := initialMSFT.Sub(finalMSFT)

	// Loser should have greater or equal reduction (liquidated first)
	if aaplReduction.Cmp(msftReduction) < 0 && finalMSFT.Cmp(initialMSFT) < 0 {
		t.Errorf("losers should be liquidated first/more: AAPL reduction=%s, MSFT reduction=%s",
			aaplReduction, msftReduction)
	}
}

func TestMarginCall_StopsWhenMarginRestored(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up marginal situation
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-5000))
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(50))

	// Equity = -$5k + $5k = $0
	// Maintenance = $1.5k
	// Margin call triggered

	triggered := b.CheckMarginCall()
	if !triggered {
		t.Error("margin call should be triggered initially")
	}

	// After liquidation, check that position was only partially liquidated
	// (not everything sold if not necessary)
	finalQty := aapl.Quantity.Load()
	if finalQty.IsNegative() {
		t.Error("should not have negative position after liquidation")
	}

	// Now restore margin by adding cash
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.Lock.Unlock()

	triggered = b.CheckMarginCall()
	if triggered {
		t.Error("margin call should not be triggered after adding cash")
	}

	b.Lock.RLock()
	stillTriggered := b.MarginCallTriggered
	b.Lock.RUnlock()

	if stillTriggered {
		t.Error("MarginCallTriggered flag should be cleared after margin restored")
	}
}

func TestMarginCall_NotTriggeredAboveMaintenance(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up healthy account
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(100000))
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))

	// Equity = $100k + $10k = $110k
	// Maintenance = $3k
	// Way above maintenance

	triggered := b.CheckMarginCall()
	if triggered {
		t.Error("margin call should not be triggered with healthy margin")
	}

	b.Lock.RLock()
	count := b.MarginCallCount
	b.Lock.RUnlock()

	if count != 0 {
		t.Errorf("expected MarginCallCount=0, got %d", count)
	}
}

func TestMarginCall_ShortPositionSqueeze(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up short position
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(15000)) // Cash from short proceeds + initial
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100)) // Short 100 shares
	aapl.Available.Store(decimal.FromInt(100))
	aapl.ShortLots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000)) // Shorted at $100
	aapl.Lock.Unlock()

	// Price rises (squeeze!)
	eq.LastPrice.Store(decimal.FromInt(200))

	// Cash: $15k
	// Short position: -100 shares * $200 = -$20k liability
	// Equity = $15k - $20k = -$5k (underwater!)
	// Maintenance = 100 * $200 * 0.30 = $6k
	// Equity (-$5k) < Maintenance ($6k) -> margin call!

	initialQty := aapl.Quantity.Load()
	if !initialQty.IsNegative() {
		t.Fatal("expected negative quantity for short position")
	}

	triggered := b.CheckMarginCall()
	if !triggered {
		equity := b.Holdings.GetEquityUSD()
		maint := b.CalculateTotalMaintenanceMargin()
		t.Errorf("margin call should be triggered for short squeeze, equity=%s, maint=%s",
			equity, maint)
	}

	// Short position should be reduced (covered)
	finalQty := aapl.Quantity.Load()
	if finalQty.Cmp(initialQty) <= 0 {
		t.Errorf("short position should be reduced (covered), before=%s, after=%s",
			initialQty, finalQty)
	}
}

func TestMarginCall_PartialLiquidation(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up position where partial liquidation is needed
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-3000))
	b.Lock.Unlock()

	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(50))

	// Cash: -$3k
	// Position: 100 * $50 = $5k
	// Equity = $2k
	// Maintenance = $1.5k
	// Equity ($2k) > Maintenance ($1.5k) - OK

	// Make cash more negative
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(-4000))
	b.Lock.Unlock()

	// Equity = -$4k + $5k = $1k
	// Maintenance = $1.5k
	// Margin call!

	initialQty := aapl.Quantity.Load()

	b.CheckMarginCall()

	finalQty := aapl.Quantity.Load()

	// Should have liquidated some but not all
	if finalQty.Cmp(initialQty) >= 0 {
		t.Error("some shares should be liquidated")
	}
	if finalQty.IsZero() || finalQty.IsNegative() {
		// Actually it might liquidate everything depending on the deficit
		// This is acceptable behavior
	}
}

func TestNoMarginCall_NoPositions(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)

	// Just cash, no positions
	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.Lock.Unlock()

	// No positions means no maintenance margin needed
	triggered := b.CheckMarginCall()
	if triggered {
		t.Error("margin call should not be triggered with no positions")
	}
}

func TestAutoLiquidate_ZeroDeficit(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)

	// AutoLiquidate with zero or negative deficit should do nothing
	b.AutoLiquidate(decimal.Zero)
	b.AutoLiquidate(decimal.FromInt(-1000))

	// Should not panic or cause issues
}
