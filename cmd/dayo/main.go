//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|             trading algorithm x3.101-2026

// dayo is a momentum-based day trading strategy.
//
// Usage:
//
//	go run ./cmd/dayo -backtest -start 2025-12-01
//	go run ./cmd/dayo -backtest -start 2025-12-01 -auction  # opening auction mode
//
// In default mode, it waits for positive momentum before entering, uses trailing
// stops, and liquidates before market close with adaptive pricing.
//
// In auction mode (-auction), it participates in the opening auction:
//   - If pre-market GOOG > previous close: buy GOOG at open
//   - If pre-market GOOG < previous close: buy GLD instead (safe haven)
//   - Uses trailing stops and EOD liquidation same as default mode
package main

import (
	"dropbear/clocky"
	"dropbear/cubby"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"dropbear/indicators"
	"dropbear/loggy"
	"flag"
	"log"
)

var (
	flagSymbols    = flag.String("symbol", "GOOG", "primary symbol to trade")
	flagFallback   = flag.String("fallback", "GLD", "fallback symbol when primary looks weak")
	flagBenchmark  = flag.String("benchmark", "QQQ", "benchmark symbol")
	flagSlipOpen   = decimal.FlagBPS("slip-open", "25", "slippage on open orders")
	flagSlipClose  = decimal.FlagBPS("slip-close", "50", "slippage on close orders")
	flagUrgency    = decimal.FlagBPS("urgency", "100", "extra slippage per minute when closing late")
	flagStopATR    = decimal.Flag("stop-atr", "1.5", "stop loss in ATR multiples")
	flagMomentum   = flag.Int("momentum", 7, "momentum lookback period in bars")
	flagWarmup     = flag.Int("warmup", 30, "warmup bars before trading")
	flagEntryStart = flag.Int("entry-start", 6_45_00, "earliest time to enter (HHMMSS)")
	flagEntryEnd   = flag.Int("entry-end", 11_00_00, "latest time to enter (HHMMSS)")
	flagBOP        = flag.Bool("bop", false, "enable BOP filter")
	flagNoMomentum = flag.Bool("no-momentum", false, "disable momentum filter")
	flagAuction    = flag.Bool("auction", false, "enable opening auction mode")
	flagStopDelay  = flag.Int("stop-delay", 0, "minutes after entry before stop becomes active (0=immediate)")
)

const (
	openTime         = 6_30_00
	preAuctionCutoff = 6_28_00 // must submit LOO before this
	normalClose      = 13_00_00
	normalStartClose = 12_50_00
	earlyClose       = 10_00_00
	earlyStartClose  = 9_50_00
)

var (
	gPrimaryTrader  *Trader
	gFallbackTrader *Trader
	gAllTraders     []*Trader
	gDate           int
	gCloseTime      int
	gStartCloseTime int
	gOpened         bool
	gClosing        bool
	gAuctionDone    bool // have we submitted our LOO order today?
	gPrevClose      decimal.Decimal
)

var earlyCloseDates = map[int]bool{
	20240703: true, 20241129: true, 20241224: true,
	20250703: true, 20251128: true, 20251224: true,
	20260703: true, 20261127: true, 20261224: true,
}

func isEarlyCloseDay(date int) bool {
	return earlyCloseDates[date]
}

func main() {
	flag.Parse()
	loggy.Init()
	cubby.Init()

	if *flagSymbols == "" {
		log.Fatal("usage: dayo -symbol GOOG [-backtest] [-start DATE]")
	}

	// Parse primary symbol
	primarySym, err := symbol.Parse(*flagSymbols)
	if err != nil {
		log.Fatalf("invalid symbol: %v", err)
	}

	primaryEquity, err := cubby.AddEquity(primarySym)
	if err != nil {
		log.Fatalf("error adding symbol %s: %v", primarySym, err)
	}
	gPrimaryTrader = NewTrader(primaryEquity)
	gAllTraders = append(gAllTraders, gPrimaryTrader)
	primaryEquity.OnBar = gPrimaryTrader.onBar

	// Parse fallback symbol (for auction mode)
	if *flagAuction && *flagFallback != "" {
		fallbackSym, err := symbol.Parse(*flagFallback)
		if err != nil {
			log.Fatalf("invalid fallback symbol: %v", err)
		}
		fallbackEquity, err := cubby.AddEquity(fallbackSym)
		if err != nil {
			log.Fatalf("error adding fallback %s: %v", fallbackSym, err)
		}
		gFallbackTrader = NewTrader(fallbackEquity)
		gAllTraders = append(gAllTraders, gFallbackTrader)
		fallbackEquity.OnBar = gFallbackTrader.onBar
	}

	benchSym := symbol.MustParse(*flagBenchmark)
	benchEquity, _ := cubby.AddEquity(benchSym)
	cubby.Benchmark = benchEquity

	if *flagAuction {
		log.Printf("Opening Auction Day Trading Strategy")
		log.Printf("  Primary:  %s", gPrimaryTrader.equity.Symbol)
		if gFallbackTrader != nil {
			log.Printf("  Fallback: %s", gFallbackTrader.equity.Symbol)
		}
	} else {
		log.Printf("Momentum Day Trading Strategy")
		log.Printf("  Symbol: %s", gPrimaryTrader.equity.Symbol)
	}

	cubby.Run()
}

type Trader struct {
	equity        *cubby.Equity
	order         *cubby.Order
	monotonicTime clocky.Time

	// indicators
	momentum *indicators.Momentum
	atr      *indicators.ATR
	bop      *indicators.BOP

	// position tracking
	entryPrice decimal.Decimal
	highSince  decimal.Decimal
	stopPrice  decimal.Decimal
	entryTime  clocky.Time // when position was entered (skip stop check on entry bar)
	barCount   int
}

func NewTrader(equity *cubby.Equity) *Trader {
	return &Trader{
		equity:   equity,
		momentum: indicators.NewMomentum(*flagMomentum),
		atr:      indicators.NewATR(14),
		bop:      indicators.NewBOP(5),
	}
}

func (t *Trader) onBar(bar *ds.Bar) {
	if !bar.Timestamp.After(t.monotonicTime) {
		return
	}
	t.monotonicTime = bar.Timestamp

	// new day reset
	date := bar.Timestamp.DateInt()
	if date != gDate {
		// save previous close before resetting
		if gPrimaryTrader.equity.Price.IsPositive() {
			gPrevClose = gPrimaryTrader.equity.Price
		}
		gDate = date
		gOpened = false
		gClosing = false
		gAuctionDone = false
		t.barCount = 0
		t.entryPrice = decimal.Zero
		t.highSince = decimal.Zero
		t.stopPrice = decimal.Zero
		t.entryTime = 0
		if isEarlyCloseDay(date) {
			gCloseTime = earlyClose
			gStartCloseTime = earlyStartClose
		} else {
			gCloseTime = normalClose
			gStartCloseTime = normalStartClose
		}
	}

	if cubby.IsWarmingUp {
		return
	}

	// update indicators
	candle := &indicators.Candle{
		Start:  bar.Timestamp,
		Open:   bar.Open,
		High:   bar.High,
		Low:    bar.Low,
		Close:  bar.Close,
		Volume: bar.Volume,
	}
	t.momentum.Add(bar.Close)
	t.atr.Add(candle)
	t.bop.Add(candle)
	t.barCount++

	// check order status
	if t.order != nil && t.order.Status.IsFinal() {
		t.order = nil
	}

	// track high since entry for trailing stop
	if !t.equity.Quantity.IsZero() {
		// detect entry (transition from no position to having position)
		if t.entryTime == 0 {
			t.entryTime = bar.Timestamp
			t.entryPrice = t.equity.Price
			t.highSince = bar.High
			if t.atr.IsReady() {
				t.stopPrice = bar.High.Sub(t.atr.Value.Mul(*flagStopATR))
			}
		} else if bar.High.Cmp(t.highSince) > 0 {
			// update trailing stop on new highs
			t.highSince = bar.High
			if t.atr.IsReady() {
				t.stopPrice = t.highSince.Sub(t.atr.Value.Mul(*flagStopATR))
			}
		}
	} else {
		// position closed, reset tracking
		if t.entryTime != 0 {
			t.entryTime = 0
			t.entryPrice = decimal.Zero
			t.highSince = decimal.Zero
			t.stopPrice = decimal.Zero
		}
	}

	now := clocky.Now()
	time := now.ClockInt()

	// auction mode: submit LOO order before market open
	if *flagAuction && !gAuctionDone && time < preAuctionCutoff {
		t.tryAuctionOrder(time)
		return
	}

	if time < openTime || time >= gCloseTime {
		return
	}

	// check stop loss (skip on entry bar and during stop delay period)
	stopDelayOver := t.entryTime == 0 || bar.Timestamp.Sub(t.entryTime) >= clocky.Duration(*flagStopDelay)*clocky.Minute
	if !t.equity.Quantity.IsZero() && t.stopPrice.IsPositive() && bar.Timestamp != t.entryTime && stopDelayOver {
		if bar.Low.Cmp(t.stopPrice) <= 0 {
			if cubby.Verbose {
				log.Printf("stop loss triggered for %s at %s (stop: %s)",
					t.equity.Symbol, bar.Close, t.stopPrice)
			}
			t.closePosition(time, gStartCloseTime, gCloseTime)
			return
		}
	}

	// time to close
	if time >= gStartCloseTime {
		if !gClosing {
			gClosing = true
			if cubby.Verbose {
				log.Printf("starting end-of-day liquidation")
			}
		}
		t.closePosition(time, gStartCloseTime, gCloseTime)
		return
	}

	// try to open (only in non-auction mode, or if auction order didn't fill)
	if !gOpened {
		gOpened = true
	}
	if !*flagAuction {
		t.tryOpen(time)
	}
}

func (t *Trader) tryAuctionOrder(time int) {
	// only the primary trader makes the auction decision
	if t != gPrimaryTrader {
		return
	}

	// need a previous close to compare against
	if !gPrevClose.IsPositive() {
		return
	}

	// need current price (pre-market)
	if !t.equity.Price.IsPositive() {
		return
	}

	gAuctionDone = true

	// mean reversion: buy primary when gapped DOWN, buy safe haven when gapped UP
	var targetTrader *Trader
	gapPct := t.equity.Price.Sub(gPrevClose).Div(gPrevClose).MulInt(100)
	if t.equity.Price.Cmp(gPrevClose) < 0 {
		// pre-market < previous close: gap down, buy the dip
		targetTrader = gPrimaryTrader
		if cubby.Verbose {
			log.Printf("auction: %s gap %.2f%% (premarket $%s < prev $%s), buying the dip",
				t.equity.Symbol, gapPct.Float64(), t.equity.Price, gPrevClose)
		}
	} else {
		// pre-market >= previous close: gap up or flat, buy safe haven instead
		if gFallbackTrader != nil {
			targetTrader = gFallbackTrader
			if cubby.Verbose {
				log.Printf("auction: %s gap %.2f%% (premarket $%s >= prev $%s), buying %s instead",
					t.equity.Symbol, gapPct.Float64(), t.equity.Price, gPrevClose, targetTrader.equity.Symbol)
			}
		} else {
			// no fallback configured, skip today
			if cubby.Verbose {
				log.Printf("auction: %s gap %.2f%% (premarket $%s >= prev $%s), no fallback, skipping",
					t.equity.Symbol, gapPct.Float64(), t.equity.Price, gPrevClose)
			}
			return
		}
	}

	// calculate limit price (generous to ensure fill at auction)
	limitPrice := targetTrader.equity.Price.Mul(decimal.One.Add(*flagSlipOpen))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	// use full buying power for the auction order
	qty := targetTrader.equity.GetMaxOrderQuantity(limitPrice).Truncate()
	if !qty.IsPositive() {
		return
	}

	order, err := targetTrader.equity.OrderAtOpen(qty, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("auction: error placing LOO order for %s: %v", targetTrader.equity.Symbol, err)
		}
		return
	}
	targetTrader.order = order
	// Note: don't set stopPrice here - wait until fill is confirmed and set based on actual price
}

func (t *Trader) tryOpen(time int) {
	// already have position
	if !t.equity.Quantity.IsZero() {
		return
	}

	// already have pending order
	if t.order != nil && t.order.Status.IsOpen() {
		return
	}

	if !t.equity.Price.IsPositive() {
		return
	}

	// check entry window
	if time < *flagEntryStart || time > *flagEntryEnd {
		return
	}

	// wait for warmup
	if t.barCount < *flagWarmup {
		return
	}

	// check indicators are ready
	if !t.atr.IsReady() {
		return
	}

	if !*flagNoMomentum {
		if !t.momentum.IsReady() {
			return
		}
		// only enter on positive momentum
		if !t.momentum.Value.IsPositive() {
			return
		}
		// optional: check BOP for confirmation
		if *flagBOP && t.bop.IsReady() && t.bop.Value.IsNegative() {
			return // sellers in control, skip
		}
	}

	// calculate limit price
	limitPrice := t.equity.Price.Mul(decimal.One.Add(*flagSlipOpen))
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	qty := t.equity.GetMaxOrderQuantity(limitPrice).Truncate()
	if !qty.IsPositive() {
		return
	}

	order, err := t.equity.Order(qty, limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("error opening %s: %v", t.equity.Symbol, err)
		}
		return
	}
	t.order = order
	// Note: entryTime, entryPrice, highSince, and stopPrice are set when fill is detected in onBar
}

func (t *Trader) closePosition(time, startClose, closeTime int) {
	shares := t.equity.Quantity
	if shares.IsZero() {
		return
	}

	// calculate urgency for adaptive slippage
	urgency := decimal.Zero
	if time >= startClose {
		totalWindow := closeTime - startClose
		elapsed := time - startClose
		if totalWindow > 0 {
			urgency = decimal.FromInt(elapsed).Div(decimal.FromInt(totalWindow))
		}
	}
	adaptiveSlip := flagSlipClose.Add(urgency.Mul(*flagUrgency))

	// check if we should cancel and replace stale order
	if t.order != nil && t.order.Status.IsOpen() {
		if t.order.Side == ds.SideSell && shares.IsPositive() {
			priceDiff := t.equity.Price.Sub(t.order.LimitPrice).Div(t.equity.Price).Abs()
			if priceDiff.Cmp(decimal.Parse("0.005")) > 0 {
				t.order.Cancel()
				t.order = nil
			} else {
				return
			}
		} else if t.order.Side == ds.SideBuy && shares.IsNegative() {
			return
		}
	}

	// calculate limit price
	limitPrice := t.equity.Price
	if shares.IsPositive() {
		limitPrice = limitPrice.Mul(decimal.One.Sub(adaptiveSlip))
	} else {
		limitPrice = limitPrice.Mul(decimal.One.Add(adaptiveSlip))
	}
	limitPrice = limitPrice.QuantizeNearest(decimal.Cent)

	order, err := t.equity.Order(shares.Neg(), limitPrice)
	if err != nil {
		if cubby.Verbose {
			log.Printf("error closing %s: %v", t.equity.Symbol, err)
		}
		return
	}
	t.order = order
}
