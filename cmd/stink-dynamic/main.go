//    _       _    ,
//   / |_|_  o)   / ) o    )
//   _\  |  | |  /__/_|  _)
//                       /_)
//
//   stink bid trading (dynamic) - nearly risk-free market tracking
//
//   This version continuously updates the stink bid to track the market.
//   Lower risk but catches fewer big crashes. Use stink-static for
//   catching whale panic sells at the cost of tail risk.

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
	flagSymbols  = flag.String("symbol", "BTC", "comma-separated symbols to trade (e.g. BTC,ETH,SOL)")
	flagDrop     = decimal.FlagPercent("drop", "0.5", "percent below market to place stink bid")
	flagBuffer   = decimal.Flag("buffer", "0", "quote currency to keep in reserve")
	flagCooldown = clocky.DurationFlag("cooldown", "30s", "minimum time between bid updates")
	flagWall     = flag.Bool("wall", false, "optimize bid placement by front-running the fattest bid in range")
	flagVerbose  = flag.Bool("verbose", false, "enable verbose logging")
	flagDry      = flag.Bool("dry", false, "dry run mode - don't place real orders")
	flagCash     = decimal.Flag("cash", "100000", "USDC balance for backtesting")
)

// SymbolState tracks state for a single trading symbol
type SymbolState struct {
	Symbol        string
	Pair          *teddy.Pair
	PriceWWMA     *indicators.WWMA // smoothed price for sell target
	StinkOrder    *teddy.Order     // current stink bid (nil if none or filled)
	SellOrder     *teddy.Order     // current recovery sell (nil if none or filled)
	StinkPrice    decimal.Decimal  // current bid price
	PreCrashPrice decimal.Decimal  // sell target after fill (WWMA when bid placed)
	Allocation    decimal.Decimal  // quote currency allocated to this symbol
	LastUpdate    clocky.Time      // last time we updated the bid
	Lock          sync.Mutex
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
			Symbol:    symbol,
			Pair:      pair,
			PriceWWMA: indicators.NewWWMA(20), // smooth price for sell target
		}
		gStates = append(gStates, state)
	}

	if len(gStates) == 0 {
		loggy.Fatalf("no valid symbols")
	}

	log.Printf("[startup] trading %d symbols: %s", len(gStates), *flagSymbols)
	log.Printf("[startup] drop=%s buffer=%s cooldown=%s", *flagDrop, *flagBuffer, *flagCooldown)

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

	// feed trade prices to WWMA for smoothed sell target
	for i := range tick.TradeCount() {
		state.PriceWWMA.Add(tick.Trade(i).Price)
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
		// will place new stink bid below
	}

	// check if stink bid filled -> place recovery sell
	if state.StinkOrder != nil && state.StinkOrder.State.Load().IsFinal() {
		filled := state.StinkOrder.Filled.Load()
		if filled.IsPositive() && state.SellOrder == nil {
			placeRecoverySell(state, filled)
		}
		state.StinkOrder = nil
		state.StinkPrice = decimal.Zero
	}

	// if we have a pending sell, don't place new stink bids yet
	if state.SellOrder != nil {
		return
	}

	// get current price
	state.Pair.Lock.RLock()
	bid, ask := state.Pair.Book.BestBidAsk()
	state.Pair.Lock.RUnlock()
	if !bid.IsPositive() || !ask.IsPositive() {
		return
	}
	currentPrice := bid.Add(ask).DivInt(2)

	// compute stink price
	dropPercent := *flagDrop
	stinkPrice := currentPrice.Mul(decimal.One.Sub(dropPercent))
	stinkPrice = stinkPrice.QuantizeTruncate(state.Pair.QuoteIncrement.Load())

	// check cooldown
	now := clocky.Now()
	if now.Sub(state.LastUpdate) < *flagCooldown {
		return
	}

	// use WWMA of trade prices for sell target (smoother than raw midpoint)
	// fall back to midpoint if WWMA not ready yet
	sellTarget := state.PriceWWMA.Value
	if !state.PriceWWMA.IsReady() {
		sellTarget = currentPrice
	}

	// if no order, place one
	if state.StinkOrder == nil {
		placeStinkBid(state, stinkPrice, sellTarget)
		return
	}

	// replace threshold scales with stink distance to avoid API spam
	// if stink is 100bps below market, only replace when price moves 10bps
	replaceThreshold := dropPercent.DivInt(10)
	oldPrice := state.StinkPrice
	if oldPrice.IsPositive() {
		priceDelta := stinkPrice.Sub(oldPrice).Abs().Div(oldPrice)
		if priceDelta.Cmp(replaceThreshold) < 0 {
			return
		}
	}

	// replace the order
	replaceStinkBid(state, stinkPrice, sellTarget)
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
	state.StinkPrice = stinkPrice
	state.PreCrashPrice = sellTarget // WWMA of trade prices (smoother than raw midpoint)
	state.LastUpdate = clocky.Now()

	log.Printf("[placed] %s %s @ $%s (%.1f%% below, target $%s)",
		state.Symbol, qty, stinkPrice.FormatThousand(2), flagDrop.MulInt(100).Float64(),
		state.PreCrashPrice.FormatThousand(2))
}

func placeRecoverySell(state *SymbolState, filled decimal.Decimal) {
	fillPrice := state.StinkOrder.Price.Load()
	notional := filled.Mul(fillPrice)
	log.Printf("[filled] %s bought %s @ $%s ($%s)",
		state.Symbol, filled, fillPrice.FormatThousand(2), notional.FormatThousand(2))

	// sell target = where price was before the crash
	// if we bid 3% below and got filled, target selling at the pre-crash level
	sellPrice := state.PreCrashPrice
	if sellPrice.Cmp(fillPrice) <= 0 {
		// pre-crash tracking failed, shouldn't happen but fallback to 1% profit
		sellPrice = fillPrice.MulInt(101).DivInt(100)
	}
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
}

func replaceStinkBid(state *SymbolState, naiveNewPrice, sellTarget decimal.Decimal) {
	if state.StinkOrder == nil {
		return
	}

	// don't replace orders that have partial fills - let them complete
	if state.StinkOrder.Filled.Load().IsPositive() {
		return
	}

	// compute new quantity based on allocation, accounting for maker fee reserve
	// use 99.9% of allocation to give headroom for rounding
	makerFee := gCoinbase.MakerFee.Load()
	maxCostPerCoin := naiveNewPrice.Mul(decimal.One.Add(makerFee))
	effectiveAlloc := state.Allocation.MulInt(999).DivInt(1000)
	newQty := effectiveAlloc.Div(maxCostPerCoin)
	newQty = newQty.QuantizeTruncate(state.Pair.BaseIncrement.Load())

	// optimize price by front-running the fattest bid in range
	newPrice := naiveNewPrice
	if *flagWall {
		state.Pair.Lock.RLock()
		fatPrice, fatSize := state.Pair.Book.FindFattestBidBelow(naiveNewPrice, newQty)
		state.Pair.Lock.RUnlock()
		if fatPrice.IsPositive() {
			// outbid the fattest bid by one increment
			optimizedPrice := fatPrice.Add(state.Pair.QuoteIncrement.Load())
			if optimizedPrice.Cmp(naiveNewPrice) < 0 {
				if *flagVerbose {
					log.Printf("[wall] %s replace: found fat bid %s @ $%s, optimizing $%s -> $%s",
						state.Symbol, fatSize, fatPrice.FormatThousand(2),
						naiveNewPrice.FormatThousand(2), optimizedPrice.FormatThousand(2))
				}
				newPrice = optimizedPrice
				// recalculate qty for the new (lower) price - we can buy more
				maxCostPerCoin = newPrice.Mul(decimal.One.Add(makerFee))
				newQty = effectiveAlloc.Div(maxCostPerCoin)
				newQty = newQty.QuantizeTruncate(state.Pair.BaseIncrement.Load())
			}
		}
	}

	order := state.StinkOrder
	if err := order.Replace(newQty, newPrice); err != nil {
		if err == ds.ErrTooManyRequests {
			// rate limited, will retry next tick
			return
		}
		if err == ds.ErrNotFound || err == ds.ErrOrderNotOpen {
			// order was filled or cancelled, clear it
			log.Printf("[info] %s order no longer exists, will place new one", state.Symbol)
			state.StinkOrder = nil
			state.StinkPrice = decimal.Zero
		} else {
			log.Printf("[error] %s failed to replace: %v", state.Symbol, err)
		}
		return
	}

	state.StinkPrice = newPrice
	state.PreCrashPrice = sellTarget // WWMA of trade prices (smoother than raw midpoint)
	state.LastUpdate = clocky.Now()
}
