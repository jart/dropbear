//    _       _    ,
//   / |_|_  o)   / ) o    )
//   _\  |  | |  /__/_|  _)
//                       /_)
//
//   stink bid trading - catch whale panic sells

package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/teddy"
	"flag"
	"log"
	"strings"
	"sync"
)

var (
	flagSymbols      = flag.String("symbol", "BTC", "comma-separated symbols to trade (e.g. BTC,ETH,SOL)")
	flagDrop         = decimal.FlagPercent("drop", "0.5", "percent below market to place stink bid")
	flagGreed        = decimal.Flag("greed", "1", "fraction of recovery to target (0.9 = take 90% of profit)")
	flagBuffer       = decimal.Flag("buffer", "0", "quote currency to keep in reserve")
	flagBuyDeadline  = clocky.DurationFlag("buy-deadline", "10s", "cancel partial fills that don't complete in time")
	flagSellDeadline = clocky.DurationFlag("sell-deadline", "0", "dump at market if recovery sell doesn't fill in time (0=disabled)")
	flagWall         = flag.Bool("wall", false, "optimize bid placement by front-running the fattest bid in range")
	flagVerbose      = flag.Bool("verbose", false, "enable verbose logging")
	flagDry          = flag.Bool("dry", false, "dry run mode - don't place real orders")
	flagCash         = decimal.Flag("cash", "100000", "USDC balance for backtesting")
)

// SymbolState tracks state for a single trading symbol
type SymbolState struct {
	Symbol           string
	Pair             *teddy.Pair
	PriceMA          *indicators.WWMA // smoothed price for sell target
	StinkOrder       *teddy.Order     // current stink bid (nil if none or filled)
	SellOrder        *teddy.Order     // current recovery sell (nil if none or filled)
	PreCrashPrice    decimal.Decimal  // sell target after fill (WWMA when bid placed)
	SellTargetPrice  decimal.Decimal  // current sell limit price (for deadline logic)
	Allocation       decimal.Decimal  // quote currency allocated to this symbol
	PartialFillStart clocky.Time      // when partial fill was first detected (zero if none)
	SellOrderStart   clocky.Time      // when recovery sell was placed (zero if none)
	Lock             sync.Mutex
}

var (
	gCoinbase *teddy.Broker
	gQuote    *teddy.Holding
	gStates   []*SymbolState
	gReady    bool
)

func main() {
	flag.Parse()
	loggy.Init()
	teddy.Init()

	if *flagDry {
		log.Printf("[startup] DRY RUN MODE - no real orders will be placed")
		teddy.Paper = true
	}

	// parse symbols
	symbols := strings.Split(*flagSymbols, ",")
	if len(symbols) == 0 {
		loggy.Fatalf("no symbols specified")
	}

	// setup coinbase
	// USDC for live (3.5% yield), USD for backtest (data availability)
	gCoinbase = teddy.Brokers.Get(ds.BrokerCoinbase)
	quoteCurrency := "USDC"
	if !teddy.Live {
		quoteCurrency = "USD"
		if flagCash.IsPositive() {
			teddy.SetBalance(ds.BrokerCoinbase, "USD", *flagCash)
		}
	}
	gQuote = gCoinbase.Holdings.Get(quoteCurrency)

	// create state for each symbol
	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			continue
		}
		pair := gCoinbase.Pairs.Get(symbol + "-" + quoteCurrency)
		state := &SymbolState{
			Symbol:   symbol,
			Pair:     pair,
			PriceMA: indicators.NewWWMA(20), // smooth price for sell target
		}
		gStates = append(gStates, state)
	}

	if len(gStates) == 0 {
		loggy.Fatalf("no valid symbols")
	}

	log.Printf("[startup] trading %d symbols: %s", len(gStates), *flagSymbols)
	log.Printf("[startup] drop=%s greed=%s buffer=%s buy-deadline=%s sell-deadline=%s",
		*flagDrop, *flagGreed, *flagBuffer, *flagBuyDeadline, *flagSellDeadline)

	// setup callbacks and run
	teddy.Brokers.OnReady = onReady
	teddy.SetBenchmark(gStates[0].Pair)
	teddy.Run()
}

func onReady() {
	log.Printf("[startup] market data ready")

	// calculate allocation per symbol
	available := gQuote.Available.Load()
	reservedBuffer := *flagBuffer
	allocatable := available.Sub(reservedBuffer)
	if !allocatable.IsPositive() {
		log.Printf("[warning] only %s %s available, need at least %s buffer", available, gQuote, reservedBuffer)
		allocatable = decimal.Zero
	}

	perSymbol := allocatable.DivInt(len(gStates))
	log.Printf("[startup] %s %s available, %s buffer, %s per symbol",
		available, gQuote, reservedBuffer, perSymbol)

	for _, state := range gStates {
		state.Allocation = perSymbol
		s := state // capture loop variable
		state.Pair.OnTick = func(tick ds.Tick) {
			handleTick(s, tick)
		}
	}

	gReady = true
}

func handleTick(state *SymbolState, tick ds.Tick) {
	if !gReady {
		return
	}

	// feed trade prices to SMA for smoothed sell target
	for i := range tick.TradeCount() {
		state.PriceMA.Add(tick.Trade(i).Price)
	}

	state.Lock.Lock()
	defer state.Lock.Unlock()

	// check if recovery sell completed -> place new stink bid
	if state.SellOrder != nil && state.SellOrder.State.Load().IsFinal() {
		sold := state.SellOrder.Filled.Load()
		if sold.IsPositive() {
			sellPrice := state.SellOrder.Price.Load()
			notional := sold.Mul(sellPrice)
			log.Printf("[sold] %s sold %s @ $%s ($%s)",
				state.Symbol, sold, sellPrice.FormatThousand(2), notional.FormatThousand(2))
		}
		state.SellOrder = nil
		state.SellTargetPrice = decimal.Zero
		state.SellOrderStart = 0
		// will place new stink bid below
	}

	// check for stale recovery sell - dump at market if deadline exceeded
	if *flagSellDeadline > 0 && state.SellOrder != nil && !state.SellOrder.State.Load().IsFinal() {
		now := clocky.Now()
		if state.SellOrderStart == 0 {
			state.SellOrderStart = now
		}
		elapsed := now.Sub(state.SellOrderStart)
		if elapsed >= *flagSellDeadline {
			// get current market price
			state.Pair.Lock.RLock()
			bid, _ := state.Pair.Book.BestBidAsk()
			state.Pair.Lock.RUnlock()
			targetPrice := state.SellTargetPrice
			qty := state.SellOrder.Quantity.Load().Sub(state.SellOrder.Filled.Load())
			if bid.IsPositive() && targetPrice.IsPositive() && qty.IsPositive() {
				lossBps := targetPrice.Sub(bid).Div(targetPrice).MulInt(10000)
				log.Printf("[dump] %s recovery sell exceeded %s, dumping %s @ $%s (%+.0f bps vs target $%s)",
					state.Symbol, *flagSellDeadline, qty, bid.FormatThousand(2), lossBps.Neg().Float64(), targetPrice.FormatThousand(2))
				// cancel old sell and place market sell at current bid
				if err := state.SellOrder.Cancel(); err != nil {
					log.Printf("[error] %s failed to cancel stale sell: %v", state.Symbol, err)
				} else {
					dumpPrice := bid.QuantizeTruncate(state.Pair.QuoteIncrement.Load())
					dumpOrder, err := state.Pair.LimitOrder(ds.SideSell, qty, dumpPrice, ds.OrderStrategyMarketable)
					if err != nil {
						log.Printf("[error] %s failed to place dump sell: %v", state.Symbol, err)
					} else {
						state.SellOrder = dumpOrder
						state.SellTargetPrice = dumpPrice
						state.SellOrderStart = now // reset deadline for the dump order
					}
				}
			}
		}
	}

	// check for stale partial fills - cancel if deadline exceeded
	if state.StinkOrder != nil && !state.StinkOrder.State.Load().IsFinal() {
		filled := state.StinkOrder.Filled.Load()
		if filled.IsPositive() {
			now := clocky.Now()
			if state.PartialFillStart == 0 {
				state.PartialFillStart = now
				log.Printf("[partial] %s detected partial fill %s, starting deadline timer",
					state.Symbol, filled)
			} else if now.Sub(state.PartialFillStart) >= *flagBuyDeadline {
				log.Printf("[deadline] %s partial fill %s exceeded %s, canceling",
					state.Symbol, filled, *flagBuyDeadline)
				if err := state.StinkOrder.Cancel(); err != nil {
					log.Printf("[error] %s failed to cancel: %v", state.Symbol, err)
				}
				// order will become final next tick and trigger recovery sell
			}
		}
	}

	// check if stink bid filled -> place recovery sell
	if state.StinkOrder != nil && state.StinkOrder.State.Load().IsFinal() {
		filled := state.StinkOrder.Filled.Load()
		if filled.IsPositive() && state.SellOrder == nil {
			placeRecoverySell(state, filled)
		}
		state.StinkOrder = nil
		state.PartialFillStart = 0
	}

	// if we have a pending sell, don't place new stink bids yet
	if state.SellOrder != nil {
		return
	}

	// get current price (midpoint)
	state.Pair.Lock.RLock()
	bid, ask := state.Pair.Book.BestBidAsk()
	state.Pair.Lock.RUnlock()
	if !bid.IsPositive() || !ask.IsPositive() {
		return
	}
	midPrice := bid.Add(ask).DivInt(2)

	// compute stink price
	dropPercent := *flagDrop
	stinkPrice := midPrice.Mul(decimal.One.Sub(dropPercent))
	stinkPrice = stinkPrice.QuantizeTruncate(state.Pair.QuoteIncrement.Load())

	// use SMA of trade prices for sell target (smoother than raw midpoint)
	// fall back to midpoint if SMA not ready yet
	sellTarget := state.PriceMA.Value
	if !state.PriceMA.IsReady() {
		sellTarget = midPrice
	}

	// if no order, place one and leave it alone until filled
	if state.StinkOrder == nil {
		placeStinkBid(state, stinkPrice, sellTarget)
	}
}

func placeStinkBid(state *SymbolState, naiveStinkPrice, sellTarget decimal.Decimal) {
	// compute quantity based on allocation, accounting for maker fee reserve
	// use 99.9% of allocation to give headroom for rounding
	makerFee := gCoinbase.MakerFee.Load()
	maxCostPerCoin := naiveStinkPrice.Mul(decimal.One.Add(makerFee))
	effectiveAlloc := state.Allocation.MulInt(999).DivInt(1000)
	qty := effectiveAlloc.Div(maxCostPerCoin)
	qty = qty.QuantizeTruncate(state.Pair.BaseIncrement.Load())

	// check minimum order size
	if qty.Cmp(state.Pair.BaseMinSize.Load()) < 0 {
		if *flagVerbose {
			log.Printf("[skip] %s qty %s below min %s",
				state.Symbol, qty, state.Pair.BaseMinSize.Load())
		}
		return
	}

	// optimize stink price by front-running the fattest bid in range
	stinkPrice := naiveStinkPrice
	if *flagWall {
		state.Pair.Lock.RLock()
		fatPrice, fatSize := state.Pair.Book.FindFattestBidBelow(naiveStinkPrice, qty)
		state.Pair.Lock.RUnlock()
		if fatPrice.IsPositive() {
			// outbid the fattest bid by one increment
			optimizedPrice := fatPrice.Add(state.Pair.QuoteIncrement.Load())
			if optimizedPrice.Cmp(naiveStinkPrice) < 0 {
				if *flagVerbose {
					log.Printf("[wall] %s found fat bid %s @ $%s, optimizing $%s -> $%s",
						state.Symbol, fatSize, fatPrice.FormatThousand(2),
						naiveStinkPrice.FormatThousand(2), optimizedPrice.FormatThousand(2))
				}
				stinkPrice = optimizedPrice
				// recalculate qty for the new (lower) price - we can buy more
				maxCostPerCoin = stinkPrice.Mul(decimal.One.Add(makerFee))
				qty = effectiveAlloc.Div(maxCostPerCoin)
				qty = qty.QuantizeTruncate(state.Pair.BaseIncrement.Load())
			}
		}
	}

	if *flagVerbose {
		log.Printf("[stink] %s placing bid %s @ $%s (%.1f%% below, target $%s)",
			state.Symbol, qty, stinkPrice,
			decimal.One.Sub(stinkPrice.Div(sellTarget)).MulInt(100).Float64(),
			sellTarget.FormatThousand(2))
	}

	order, err := state.Pair.LimitOrder(ds.SideBuy, qty, stinkPrice, ds.OrderStrategyPostOnly)
	if err == ds.ErrTooManyRequests {
		// rate limited, will retry next tick
		return
	}
	if err != nil {
		log.Printf("[error] %s failed to place stink bid: %v", state.Symbol, err)
		return
	}

	state.StinkOrder = order
	state.PreCrashPrice = sellTarget // smoothed trade prices (smoother than raw midpoint)

	log.Printf("[placed] %s %s @ $%s (%.1f%% below, target $%s)",
		state.Symbol, qty, stinkPrice.FormatThousand(2), flagDrop.MulInt(100).Float64(),
		state.PreCrashPrice.FormatThousand(2))
}

func placeRecoverySell(state *SymbolState, filled decimal.Decimal) {
	fillPrice := state.StinkOrder.Price.Load()
	notional := filled.Mul(fillPrice)
	log.Printf("[filled] %s bought %s @ $%s ($%s)",
		state.Symbol, filled, fillPrice.FormatThousand(2), notional.FormatThousand(2))

	// sell target = fillPrice + greed * (PreCrashPrice - fillPrice)
	// greed=1.0 means full recovery, greed=0.9 means take 90% of profit
	recoveryAmount := state.PreCrashPrice.Sub(fillPrice)
	if !recoveryAmount.IsPositive() {
		// pre-crash tracking failed, shouldn't happen but fallback to 1% profit
		recoveryAmount = fillPrice.DivInt(100)
	}
	sellPrice := fillPrice.Add(recoveryAmount.Mul(*flagGreed))
	sellPrice = sellPrice.QuantizeAway(state.Pair.QuoteIncrement.Load())

	profitBps := sellPrice.Sub(fillPrice).Div(fillPrice).MulInt(10000)
	log.Printf("[sell] %s targeting $%s (%+.1f bps profit)", state.Symbol, sellPrice.FormatThousand(2), profitBps.Float64())

	sellOrder, err := state.Pair.LimitOrder(ds.SideSell, filled, sellPrice, ds.OrderStrategyMarketable)
	if err == ds.ErrTooManyRequests {
		// rate limited - this is bad, we have coins to sell
		// but we'll retry next tick when we see StinkOrder is filled but SellOrder is nil
		log.Printf("[warning] %s rate limited placing recovery sell, will retry", state.Symbol)
		return
	}
	if err != nil {
		log.Printf("[error] %s failed to place recovery sell: %v", state.Symbol, err)
		return
	}

	state.SellOrder = sellOrder
	state.SellTargetPrice = sellPrice
	state.SellOrderStart = clocky.Now()
}
