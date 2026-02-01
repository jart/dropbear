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
	flagVerbose  = flag.Bool("verbose", false, "enable verbose logging")
	flagDry      = flag.Bool("dry", false, "dry run mode - don't place real orders")
	flagCash     = decimal.Flag("cash", "100000", "USDC balance for backtesting")
)

// SymbolState tracks state for a single trading symbol
type SymbolState struct {
	Symbol        string
	Pair          *teddy.Pair
	StinkOrder    *teddy.Order    // current stink bid (nil if none or filled)
	SellOrder     *teddy.Order    // current recovery sell (nil if none or filled)
	StinkPrice    decimal.Decimal // current bid price
	PreCrashPrice decimal.Decimal // sell target after fill (captured when bid placed)
	Allocation    decimal.Decimal // quote currency allocated to this symbol
	LastUpdate    clocky.Time     // last time we updated the bid
	LastPrice     decimal.Decimal // last seen trade price
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
			Symbol: symbol,
			Pair:   pair,
		}
		gStates = append(gStates, state)
	}

	if len(gStates) == 0 {
		loggy.Fatalf("no valid symbols")
	}

	log.Printf("[startup] trading %d symbols: %s", len(gStates), *flagSymbols)
	log.Printf("[startup] drop=%s buffer=%s", *flagDrop, *flagBuffer)

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

	// update price from trades
	for i := range tick.TradeCount() {
		state.LastPrice = tick.Trade(i).Price
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

	// if no order, place one
	if state.StinkOrder == nil {
		placeStinkBid(state, stinkPrice, dropPercent)
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
	replaceStinkBid(state, stinkPrice)
}

func placeStinkBid(state *SymbolState, stinkPrice, dropPercent decimal.Decimal) {
	// compute quantity based on allocation, accounting for maker fee reserve
	// use 99.9% of allocation to give headroom for rounding
	makerFee := gCoinbase.MakerFee.Load()
	maxCostPerCoin := stinkPrice.Mul(decimal.One.Add(makerFee))
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

	if *flagVerbose {
		log.Printf("[stink] %s placing bid %s @ $%s (%.1f%% below market, pre-crash target $%s)",
			state.Symbol, qty, stinkPrice,
			decimal.One.Sub(stinkPrice.Div(state.LastPrice)).MulInt(100).Float64(),
			state.PreCrashPrice)
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
	state.PreCrashPrice = state.LastPrice // capture current price as sell target
	state.LastUpdate = clocky.Now()

	dropBps := dropPercent.MulInt(10000)
	log.Printf("[placed] %s %s @ $%s (%.0f bps below, target $%s)",
		state.Symbol, qty, stinkPrice.FormatThousand(2), dropBps.Float64(),
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

func replaceStinkBid(state *SymbolState, newPrice decimal.Decimal) {
	if state.StinkOrder == nil {
		return
	}

	// compute new quantity based on allocation, accounting for maker fee reserve
	// use 99.9% of allocation to give headroom for rounding
	makerFee := gCoinbase.MakerFee.Load()
	maxCostPerCoin := newPrice.Mul(decimal.One.Add(makerFee))
	effectiveAlloc := state.Allocation.MulInt(999).DivInt(1000)
	newQty := effectiveAlloc.Div(maxCostPerCoin)
	newQty = newQty.QuantizeTruncate(state.Pair.BaseIncrement.Load())

	// delta := newPrice.Sub(state.StinkPrice)
	// deltaBps := delta.Div(state.StinkPrice).MulInt(10000)
	// log.Printf("[replace] %s $%s -> $%s (%+.1f bps)", state.Symbol, state.StinkPrice.FormatThousand(2), newPrice.FormatThousand(2), deltaBps.Float64())

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
	state.PreCrashPrice = state.LastPrice // update sell target to current price
	state.LastUpdate = clocky.Now()
}

func cancelStinkBid(state *SymbolState, reason string) {
	if state.StinkOrder == nil {
		return
	}

	log.Printf("[cancel] %s cancelling stink bid: %s", state.Symbol, reason)

	if err := state.StinkOrder.Cancel(); err != nil {
		if err == ds.ErrTooManyRequests {
			// rate limited, will retry next tick
			return
		}
		if err != ds.ErrNotFound {
			log.Printf("[error] %s failed to cancel: %v", state.Symbol, err)
		}
	}

	state.StinkOrder = nil
	state.StinkPrice = decimal.Zero
}
