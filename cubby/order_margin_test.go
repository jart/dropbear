package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"testing"
)

func TestOpenLimitOrder_ReservesMargin(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.IsDayTradingTime = true // Enable 4x day trading mode
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Initially no open orders
	reserved := b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero reserved margin initially, got %s", reserved)
	}

	// Place a limit buy order for 100 shares at $100
	order := eq.LimitOrder(ds.SideBuy, 100, decimal.FromInt(100), ds.OrderStrategyMarketable)

	// Now there should be margin reserved
	// Initial margin = max(50%, 30%) of $10,000 = $5,000
	reserved = b.GetMarginUsedByOpenOrders()
	expected := decimal.FromInt(5000) // 50% of $10k
	if reserved.Cmp(expected) != 0 {
		t.Errorf("expected reserved margin %s, got %s", expected, reserved)
	}

	// Net buying power should be reduced
	// Note: LimitOrder already deducts full cost ($10k) from DayTradingBuyingPower
	// GetMarginUsedByOpenOrders reports margin ($5k) separately for tracking
	netBP := b.GetNetAvailableBuyingPower()
	// Available BP ($200k - $10k hold) - margin ($5k) = $185k
	expectedNet := decimal.FromInt(185000)
	if netBP.Cmp(expectedNet) != 0 {
		t.Errorf("expected net buying power %s, got %s", expectedNet, netBP)
	}

	// Cancel the order
	// In paper mode, we need to manually set the OrderID for cancel to work
	order.Lock.Lock()
	order.OrderID = order.ClientOrderID // Use ClientOrderID as OrderID for paper mode
	order.Lock.Unlock()
	err := order.Cancel()
	if err != nil {
		t.Fatalf("failed to cancel order: %v", err)
	}

	// Reserved margin should be zero again
	reserved = b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero reserved margin after cancel, got %s", reserved)
	}
}

func TestOpenMOCOrder_ReservesMargin(t *testing.T) {
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

	// Place a MOC buy order for 100 shares
	_ = eq.MOCOrder(ds.SideBuy, 100)

	// MOC uses last price for margin estimation
	reserved := b.GetMarginUsedByOpenOrders()
	expected := decimal.FromInt(5000) // 50% of $10k
	if reserved.Cmp(expected) != 0 {
		t.Errorf("expected reserved margin %s for MOC, got %s", expected, reserved)
	}
}

func TestFilledOrder_ReleasesMargin(t *testing.T) {
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

	// Place market order
	_ = eq.MarketOrder(ds.SideBuy, 100)

	// Simulate fill
	candle := newTestCandle(clocky.Now(), 100, 102, 99, 101, 1000)
	eq.simulateFills(candle)

	// After fill, no margin should be reserved by open orders
	reserved := b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero reserved margin after fill, got %s", reserved)
	}
}

func TestCancelledOrder_ReleasesMargin(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.IsDayTradingTime = true
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place limit order
	order := eq.LimitOrder(ds.SideBuy, 100, decimal.FromInt(100), ds.OrderStrategyMarketable)

	// Verify margin is reserved
	reserved := b.GetMarginUsedByOpenOrders()
	if reserved.IsZero() {
		t.Error("expected margin to be reserved for open order")
	}

	// Cancel the order
	// In paper mode, we need to manually set the OrderID for cancel to work
	order.Lock.Lock()
	order.OrderID = order.ClientOrderID
	order.Lock.Unlock()
	err := order.Cancel()
	if err != nil {
		t.Fatalf("failed to cancel order: %v", err)
	}

	// Margin should be released
	reserved = b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero reserved margin after cancel, got %s", reserved)
	}
}

func TestMultipleOrders_CumulativeMargin(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eqAAPL := b.Equities.Get("AAPL")
	eqMSFT := b.Equities.Get("MSFT")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(100000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(400000))
	b.Lock.Unlock()

	eqAAPL.LastPrice.Store(decimal.FromInt(100))
	eqAAPL.isReady = true
	eqMSFT.LastPrice.Store(decimal.FromInt(200))
	eqMSFT.isReady = true

	// Place two limit orders
	_ = eqAAPL.LimitOrder(ds.SideBuy, 100, decimal.FromInt(100), ds.OrderStrategyMarketable) // $10k notional, $5k margin
	_ = eqMSFT.LimitOrder(ds.SideBuy, 50, decimal.FromInt(200), ds.OrderStrategyMarketable)  // $10k notional, $5k margin

	// Total margin should be cumulative
	reserved := b.GetMarginUsedByOpenOrders()
	expected := decimal.FromInt(10000) // $5k + $5k
	if reserved.Cmp(expected) != 0 {
		t.Errorf("expected cumulative reserved margin %s, got %s", expected, reserved)
	}
}

func TestClosingOrder_NoMarginReserved(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(40000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(160000))
	b.Lock.Unlock()

	// Set up existing long position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a sell order to close position
	_ = eq.MarketOrder(ds.SideSell, 100)

	// Sell order should NOT reserve additional margin (it's closing, not opening)
	reserved := b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero margin for closing order, got %s", reserved)
	}
}

func TestCoverOrder_NoMarginReserved(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.Lock.Unlock()

	// Set up existing short position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100))
	aapl.Available.Store(decimal.FromInt(100))
	aapl.ShortLots.Add(clocky.Now(), decimal.FromInt(100), decimal.FromInt(10000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a cover order
	_ = eq.CoverOrder(100)

	// Cover order should NOT reserve additional margin (it's closing the short)
	reserved := b.GetMarginUsedByOpenOrders()
	if !reserved.IsZero() {
		t.Errorf("expected zero margin for cover order, got %s", reserved)
	}
}

func TestShortOrder_ReservesMargin(t *testing.T) {
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

	// Place a short order
	_ = eq.ShortOrder(100)

	// Short order should reserve margin
	reserved := b.GetMarginUsedByOpenOrders()
	expected := decimal.FromInt(5000) // 50% of $10k
	if reserved.Cmp(expected) != 0 {
		t.Errorf("expected reserved margin %s for short order, got %s", expected, reserved)
	}
}

func TestGetNetAvailableBuyingPower(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	b.Lock.Lock()
	b.Cash.Store(decimal.FromInt(50000))
	b.DayTradingBuyingPower.Store(decimal.FromInt(200000))
	b.IsDayTradingTime = true
	b.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Initial net buying power equals available
	netBP := b.GetNetAvailableBuyingPower()
	if netBP.Cmp(decimal.FromInt(200000)) != 0 {
		t.Errorf("expected initial net BP of $200k, got %s", netBP)
	}

	// Place order that reserves $5k margin
	_ = eq.LimitOrder(ds.SideBuy, 100, decimal.FromInt(100), ds.OrderStrategyMarketable)

	// Net BP should be reduced
	// LimitOrder deducted $10k from DTBP, GetNetAvailableBuyingPower subtracts $5k margin
	netBP = b.GetNetAvailableBuyingPower()
	expected := decimal.FromInt(185000) // $200k - $10k hold - $5k margin reservation
	if netBP.Cmp(expected) != 0 {
		t.Errorf("expected net BP %s after order, got %s", expected, netBP)
	}
}

func TestIsOpeningOrder_BuyNoPosition(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")
	eq.isReady = true

	// Buy with no existing position is opening
	if !isOpeningOrder(ds.SideBuy, false, false, eq) {
		t.Error("buy with no position should be opening")
	}
}

func TestIsOpeningOrder_BuyLongPosition(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up long position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()

	// Buy with existing long is opening (adding to position)
	if !isOpeningOrder(ds.SideBuy, false, false, eq) {
		t.Error("buy with long position should be opening (adding)")
	}
}

func TestIsOpeningOrder_SellLongPosition(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up long position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(100))
	aapl.Lock.Unlock()

	// Sell with existing long is closing
	if isOpeningOrder(ds.SideSell, false, false, eq) {
		t.Error("sell with long position should be closing, not opening")
	}
}

func TestIsOpeningOrder_ShortSale(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Short sale is always opening
	if !isOpeningOrder(ds.SideSell, true, false, eq) {
		t.Error("short sale should always be opening")
	}
}

func TestIsOpeningOrder_Cover(t *testing.T) {
	resetCubby()
	Paper = true

	b := Brokers.Get(ds.BrokerAlpaca)
	eq := b.Equities.Get("AAPL")

	// Set up short position
	aapl := b.Holdings.Get("AAPL")
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(-100))
	aapl.Lock.Unlock()

	// Cover is always closing
	if isOpeningOrder(ds.SideBuy, false, true, eq) {
		t.Error("cover should always be closing")
	}
}

func newTestCandle(start clocky.Time, open, high, low, close, volume int) *indicators.Candle {
	return &indicators.Candle{
		Start:  start,
		Open:   decimal.FromInt(open),
		High:   decimal.FromInt(high),
		Low:    decimal.FromInt(low),
		Close:  decimal.FromInt(close),
		Volume: decimal.FromInt(volume),
	}
}
