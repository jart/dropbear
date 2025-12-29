package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"testing"
	"time"
)

// makeCandle creates a test candle at the given Eastern Time.
func makeCandle(year, month, day, hour, minute int, open, high, low, close float64) *indicators.Candle {
	loc, _ := time.LoadLocation("America/New_York")
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, loc)
	return &indicators.Candle{
		Start:  clocky.Time(t.UnixMicro()),
		Open:   decimal.Parse(floatToString(open)),
		High:   decimal.Parse(floatToString(high)),
		Low:    decimal.Parse(floatToString(low)),
		Close:  decimal.Parse(floatToString(close)),
		Volume: decimal.FromInt(1000),
	}
}

func floatToString(f float64) string {
	return decimal.FromFloat64(f).String()
}

// makeOpenCandle creates a candle at market open (9:30 AM ET).
func makeOpenCandle(open, high, low, close float64) *indicators.Candle {
	return makeCandle(2025, 1, 2, 9, 30, open, high, low, close) // Thursday Jan 2, 2025
}

// makeCloseCandle creates a candle at market close (15:59 ET).
func makeCloseCandle(open, high, low, close float64) *indicators.Candle {
	return makeCandle(2025, 1, 2, 15, 59, open, high, low, close)
}

// makeMidDayCandle creates a candle at mid-day (12:00 PM ET).
func makeMidDayCandle(open, high, low, close float64) *indicators.Candle {
	return makeCandle(2025, 1, 2, 12, 0, open, high, low, close)
}

func TestMOOOrder_FillsAtOpen(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place MOO buy order
	order := eq.MOOOrder(ds.SideBuy, 10)

	if order.Type != ds.OrderTypeMOO {
		t.Errorf("expected MOO order type, got %v", order.Type)
	}

	// Simulate market open candle (9:30 AM ET)
	candle := makeOpenCandle(101, 105, 99, 102)
	eq.simulateFills(candle)

	// Order should be filled at open price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
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

func TestMOCOrder_FillsAtClose(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place MOC buy order
	order := eq.MOCOrder(ds.SideBuy, 10)

	if order.Type != ds.OrderTypeMOC {
		t.Errorf("expected MOC order type, got %v", order.Type)
	}

	// Simulate market close candle (15:59 ET)
	candle := makeCloseCandle(100, 105, 98, 103)
	eq.simulateFills(candle)

	// Order should be filled at close price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(103)) != 0 {
		t.Errorf("expected fill price 103 (close), got %s", order.Price)
	}
}

func TestLOOOrder_LimitHonored(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOO buy order with limit at 102
	order := eq.LOOOrder(ds.SideBuy, 10, decimal.FromInt(102))

	if order.Type != ds.OrderTypeLOO {
		t.Errorf("expected LOO order type, got %v", order.Type)
	}

	// Simulate market open with open price at 101 (below limit)
	candle := makeOpenCandle(101, 105, 99, 102)
	eq.simulateFills(candle)

	// Order should be filled at open price (101), not limit price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(101)) != 0 {
		t.Errorf("expected fill price 101 (open), got %s", order.Price)
	}
}

func TestLOCOrder_LimitHonored(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOC buy order with limit at 104
	order := eq.LOCOrder(ds.SideBuy, 10, decimal.FromInt(104))

	if order.Type != ds.OrderTypeLOC {
		t.Errorf("expected LOC order type, got %v", order.Type)
	}

	// Simulate market close with close price at 103 (below limit)
	candle := makeCloseCandle(100, 105, 98, 103)
	eq.simulateFills(candle)

	// Order should be filled at close price (103), not limit price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(103)) != 0 {
		t.Errorf("expected fill price 103 (close), got %s", order.Price)
	}
}

func TestMOOOrder_DoesNotFillMidDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place MOO buy order
	order := eq.MOOOrder(ds.SideBuy, 10)

	// Simulate mid-day candle (12:00 PM ET)
	candle := makeMidDayCandle(101, 105, 99, 102)
	eq.simulateFills(candle)

	// Order should NOT be filled at mid-day
	if order.State == ds.OrderStateFilled {
		t.Error("MOO order should not fill at mid-day")
	}
}

func TestMOCOrder_DoesNotFillMidDay(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place MOC buy order
	order := eq.MOCOrder(ds.SideBuy, 10)

	// Simulate mid-day candle (12:00 PM ET)
	candle := makeMidDayCandle(101, 105, 99, 102)
	eq.simulateFills(candle)

	// Order should NOT be filled at mid-day
	if order.State == ds.OrderStateFilled {
		t.Error("MOC order should not fill at mid-day")
	}
}

func TestLOOOrder_NoFillIfGapAboveLimit(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOO buy order with limit at 100
	order := eq.LOOOrder(ds.SideBuy, 10, decimal.FromInt(100))

	// Simulate market open with gap up (open at 105, above limit)
	candle := makeOpenCandle(105, 110, 104, 108)
	eq.simulateFills(candle)

	// Order should NOT be filled because open > limit
	if order.State == ds.OrderStateFilled {
		t.Error("LOO buy order should not fill when open price exceeds limit")
	}
}

func TestLOCOrder_NoFillIfCloseAboveLimit(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(10000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(40000))
	ex.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOC buy order with limit at 100
	order := eq.LOCOrder(ds.SideBuy, 10, decimal.FromInt(100))

	// Simulate market close with close price above limit
	candle := makeCloseCandle(99, 110, 98, 105)
	eq.simulateFills(candle)

	// Order should NOT be filled because close > limit
	if order.State == ds.OrderStateFilled {
		t.Error("LOC buy order should not fill when close price exceeds limit")
	}
}

func TestMOOSellOrder_FillsAtOpen(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(5000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(20000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(20))
	aapl.Available.Store(decimal.FromInt(20))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(20), decimal.FromInt(2000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place MOO sell order
	order := eq.MOOOrder(ds.SideSell, 10)

	// Simulate market open candle
	candle := makeOpenCandle(102, 105, 100, 103)
	eq.simulateFills(candle)

	// Order should be filled at open price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(102)) != 0 {
		t.Errorf("expected fill price 102 (open), got %s", order.Price)
	}

	// Check holdings - should have 10 shares left
	aapl.Lock.RLock()
	qty := aapl.Quantity.Load()
	aapl.Lock.RUnlock()
	if qty.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected 10 AAPL shares remaining, got %s", qty)
	}
}

func TestLOOSellOrder_FillsWhenLimitMet(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(5000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(20000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(20))
	aapl.Available.Store(decimal.FromInt(20))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(20), decimal.FromInt(2000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOO sell order with limit at 100
	order := eq.LOOOrder(ds.SideSell, 10, decimal.FromInt(100))

	// Simulate market open with open price at 102 (above sell limit)
	candle := makeOpenCandle(102, 105, 100, 103)
	eq.simulateFills(candle)

	// Order should be filled at open price
	if order.State != ds.OrderStateFilled {
		t.Errorf("expected filled state, got %v", order.State)
	}
	if order.Price.Cmp(decimal.FromInt(102)) != 0 {
		t.Errorf("expected fill price 102 (open), got %s", order.Price)
	}
}

func TestLOOSellOrder_NoFillWhenOpenBelowLimit(t *testing.T) {
	resetCubby()
	Paper = true
	gRateLimiter = newRateLimiter()

	ex := Exchanges.Get(ds.ExchangeAlpaca)
	eq := ex.Equities.Get("AAPL")

	ex.Lock.Lock()
	ex.Cash.Store(decimal.FromInt(5000))
	ex.DayTradingBuyingPower.Store(decimal.FromInt(20000))
	ex.Lock.Unlock()

	aapl := ex.Holdings.Get("AAPL")
	aapl.Lots = ds.NewLots(ds.CostBasisMethodLIFO)
	aapl.Lock.Lock()
	aapl.Quantity.Store(decimal.FromInt(20))
	aapl.Available.Store(decimal.FromInt(20))
	aapl.Lots.Add(clocky.Now(), decimal.FromInt(20), decimal.FromInt(2000))
	aapl.Lock.Unlock()

	eq.LastPrice.Store(decimal.FromInt(100))
	eq.isReady = true

	// Place LOO sell order with limit at 105
	order := eq.LOOOrder(ds.SideSell, 10, decimal.FromInt(105))

	// Simulate market open with open price at 102 (below sell limit)
	candle := makeOpenCandle(102, 104, 100, 103)
	eq.simulateFills(candle)

	// Order should NOT be filled because open < sell limit
	if order.State == ds.OrderStateFilled {
		t.Error("LOO sell order should not fill when open price is below limit")
	}
}

func TestIsMarketOpenCandle(t *testing.T) {
	// Test 9:30 AM ET - should be open
	openCandle := makeOpenCandle(100, 101, 99, 100)
	if !IsMarketOpenCandle(openCandle.Start) {
		t.Error("9:30 AM ET should be market open candle")
	}

	// Test 12:00 PM ET - should not be open
	midDayCandle := makeMidDayCandle(100, 101, 99, 100)
	if IsMarketOpenCandle(midDayCandle.Start) {
		t.Error("12:00 PM ET should not be market open candle")
	}

	// Test 15:59 ET - should not be open
	closeCandle := makeCloseCandle(100, 101, 99, 100)
	if IsMarketOpenCandle(closeCandle.Start) {
		t.Error("15:59 ET should not be market open candle")
	}
}

func TestIsMarketCloseCandle(t *testing.T) {
	// Test 15:59 ET - should be close
	closeCandle := makeCloseCandle(100, 101, 99, 100)
	if !IsMarketCloseCandle(closeCandle.Start) {
		t.Error("15:59 ET should be market close candle")
	}

	// Test 9:30 AM ET - should not be close
	openCandle := makeOpenCandle(100, 101, 99, 100)
	if IsMarketCloseCandle(openCandle.Start) {
		t.Error("9:30 AM ET should not be market close candle")
	}

	// Test 12:00 PM ET - should not be close
	midDayCandle := makeMidDayCandle(100, 101, 99, 100)
	if IsMarketCloseCandle(midDayCandle.Start) {
		t.Error("12:00 PM ET should not be market close candle")
	}
}

func TestWeekendCandles(t *testing.T) {
	// Saturday Jan 4, 2025 at 9:30 AM - should NOT be open
	loc, _ := time.LoadLocation("America/New_York")
	sat := time.Date(2025, 1, 4, 9, 30, 0, 0, loc)
	satTime := clocky.Time(sat.UnixMicro())

	if IsMarketOpenCandle(satTime) {
		t.Error("Saturday should not be market open candle")
	}
	if IsMarketCloseCandle(satTime) {
		t.Error("Saturday should not be market close candle")
	}

	// Sunday Jan 5, 2025 at 15:59
	sun := time.Date(2025, 1, 5, 15, 59, 0, 0, loc)
	sunTime := clocky.Time(sun.UnixMicro())

	if IsMarketOpenCandle(sunTime) {
		t.Error("Sunday should not be market open candle")
	}
	if IsMarketCloseCandle(sunTime) {
		t.Error("Sunday should not be market close candle")
	}
}
