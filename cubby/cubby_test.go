package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"testing"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

func init() {
	ds.SetOffline()
	clocky.Now = clocky.FakeNow
}

func resetCubby() {
	gRunning = false
	gBenchmark = nil
	gRateLimiter = nil
	Paper = true
	Live = false
	Exchanges = &exchanges{
		OnReady:       func() {},
		exchangeMap:   make(map[ds.Exchange]*Exchange),
		exchangeArray: make([]*Exchange, 0),
		unready:       treeset.NewWith(compareExchanges),
	}
}

func TestHolding(t *testing.T) {
	resetCubby()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	usd := ex.Holdings.Get("USD")
	aapl := ex.Holdings.Get("AAPL")

	// Check initial state
	if !usd.IsCash {
		t.Error("USD should be cash")
	}
	if aapl.IsCash {
		t.Error("AAPL should not be cash")
	}

	// Set balance
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(10000))
	usd.Available.Store(decimal.FromInt(10000))
	usd.Lock.Unlock()

	if usd.Quantity.Load().Cmp(decimal.FromInt(10000)) != 0 {
		t.Errorf("expected 10000 USD, got %s", usd.Quantity.Load())
	}
}

func TestOrder(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balances
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(10000))
	usd.Available.Store(decimal.FromInt(10000))
	usd.Lock.Unlock()

	// Set a price so we can place orders
	eq.LastPrice.Store(decimal.FromInt(100))

	// Place a buy order
	order := eq.LimitOrder(ds.SideBuy, 10, decimal.FromInt(100), ds.OrderStrategyPostOnly)

	if order.Side != ds.SideBuy {
		t.Error("order should be buy")
	}
	if order.Quantity.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected quantity 10, got %s", order.Quantity)
	}
	if order.LimitPrice.Cmp(decimal.FromInt(100)) != 0 {
		t.Errorf("expected limit price 100, got %s", order.LimitPrice)
	}
	if order.State != ds.OrderStateNew {
		t.Errorf("expected state New, got %v", order.State)
	}

	// Check that USD was held
	usd.Lock.RLock()
	available := usd.Available.Load()
	usd.Lock.RUnlock()

	expected := decimal.FromInt(10000).Sub(decimal.FromInt(1000)) // 10 shares * $100
	if available.Cmp(expected) != 0 {
		t.Errorf("expected available %s, got %s", expected, available)
	}
}

func TestMarketOrderFill(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balances
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(10000))
	usd.Available.Store(decimal.FromInt(10000))
	usd.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	// Set a price
	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a market buy order
	order := eq.MarketOrder(ds.SideBuy, 10)

	if order.Type != ds.OrderTypeMarket {
		t.Errorf("expected market order, got %v", order.Type)
	}

	// Simulate candle that fills the order
	candle := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(101),
		High:   decimal.FromInt(105),
		Low:    decimal.FromInt(99),
		Close:  decimal.FromInt(102),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle)

	// Order should be filled at open price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Filled.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected 10 filled, got %s", order.Filled)
	}
	// Fill price should be 101 (the open)
	if order.Price.Cmp(decimal.FromInt(101)) != 0 {
		t.Errorf("expected fill price 101, got %s", order.Price)
	}

	// Check holdings
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()
	if qty.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected 10 AAPL shares, got %s", qty)
	}
}

func TestLimitOrderFill(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balances
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(10000))
	usd.Available.Store(decimal.FromInt(10000))
	usd.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	// Set a price
	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a limit buy order at $99
	order := eq.LimitOrder(ds.SideBuy, 10, decimal.FromInt(99), ds.OrderStrategyPostOnly)

	// Candle that doesn't hit our limit (low is 100)
	candle1 := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(101),
		High:   decimal.FromInt(105),
		Low:    decimal.FromInt(100),
		Close:  decimal.FromInt(102),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle1)

	// Order should NOT be filled
	if order.State == ds.OrderStateFilled {
		t.Error("order should not be filled yet")
	}

	// Candle that hits our limit (low is 98)
	candle2 := &indicators.Candle{
		Start:  clocky.Now().Add(clocky.Minute),
		Open:   decimal.FromInt(100),
		High:   decimal.FromInt(101),
		Low:    decimal.FromInt(98),
		Close:  decimal.FromInt(99),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle2)

	// Order should be filled at limit price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(99)) != 0 {
		t.Errorf("expected fill price 99, got %s", order.Price)
	}

	// Check holdings
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()
	if qty.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected 10 AAPL shares, got %s", qty)
	}
}

func TestSellOrder(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balances with some shares
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(5000))
	usd.Available.Store(decimal.FromInt(5000))
	usd.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(20))
	aapl.Available.Store(decimal.FromInt(20))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(20), decimal.FromInt(2000))
	aapl.Lock.Unlock()

	// Set a price
	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a market sell order
	order := eq.MarketOrder(ds.SideSell, 10)

	// Simulate fill
	candle := &indicators.Candle{
		Start:  clocky.Now(),
		Open:   decimal.FromInt(102),
		High:   decimal.FromInt(105),
		Low:    decimal.FromInt(100),
		Close:  decimal.FromInt(103),
		Volume: decimal.FromInt(1000),
	}

	eq.simulateFills(candle)

	// Order should be filled
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}

	// Check holdings - should have 10 shares left
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()
	if qty.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected 10 AAPL shares remaining, got %s", qty)
	}

	// Check USD increased (sold at 102, minus fees)
	usd.Lock.RLock()
	usdQty := usd.Quantity.Load()
	usd.Lock.RUnlock()
	// Base amount: 5000 + 10*102 = 6020
	// Minus fees: broker 0.025 + exchange 0.02 + CAT 0.0003 + TAF 0.002 ≈ 0.0473
	expectedBase := decimal.FromInt(5000).Add(decimal.FromInt(1020))
	if usdQty.Cmp(expectedBase) > 0 {
		t.Errorf("expected USD <= %s (before fees), got %s", expectedBase, usdQty)
	}
	minExpected := expectedBase.Sub(decimal.Parse("0.05")) // allow up to 5 cents of fees
	if usdQty.Cmp(minExpected) < 0 {
		t.Errorf("expected USD >= %s (fees too high), got %s", minExpected, usdQty)
	}
}

func TestCancelOrder(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set initial balances
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(10000))
	usd.Available.Store(decimal.FromInt(10000))
	usd.Lock.Unlock()

	// Set a price
	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place a limit order
	order := eq.LimitOrder(ds.SideBuy, 10, decimal.FromInt(95), ds.OrderStrategyPostOnly)
	order.OrderID = "test-order-id" // Set order ID so cancel works

	// Available should be reduced by hold
	usd.Lock.RLock()
	availBefore := usd.Available.Load()
	usd.Lock.RUnlock()

	// Cancel the order
	err := order.Cancel()
	if err != nil {
		t.Errorf("cancel failed: %v", err)
	}

	if order.State != ds.OrderStateCanceled {
		t.Errorf("expected canceled state, got %v", order.State)
	}

	// Available should be restored
	usd.Lock.RLock()
	availAfter := usd.Available.Load()
	usd.Lock.RUnlock()

	if availAfter.Cmp(decimal.FromInt(10000)) != 0 {
		t.Errorf("expected available restored to 10000, got %s (was %s before cancel)",
			availAfter, availBefore)
	}
}

func TestEquityCalculation(t *testing.T) {
	resetCubby()
	Paper = true

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	// Set balances
	usd := ex.Holdings.Get("USD")
	usd.Lock.Lock()
	usd.Quantity.Store(decimal.FromInt(5000))
	usd.Available.Store(decimal.FromInt(5000))
	usd.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(50))
	aapl.Available.Store(decimal.FromInt(50))
	aapl.Lock.Unlock()

	// Set AAPL price
	eq.LastPrice.Store(decimal.FromInt(200))

	// Total equity should be 5000 + 50*200 = 15000
	equity := GetEquityUSD()
	expected := decimal.FromInt(15000)
	if equity.Cmp(expected) != 0 {
		t.Errorf("expected equity %s, got %s", expected, equity)
	}

	// Invested should be 50*200 = 10000
	invested := GetInvestedUSD()
	expectedInvested := decimal.FromInt(10000)
	if invested.Cmp(expectedInvested) != 0 {
		t.Errorf("expected invested %s, got %s", expectedInvested, invested)
	}
}
