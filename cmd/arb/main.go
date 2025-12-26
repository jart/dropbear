package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/binanceusd"
	"dropbear/loggy"
	"dropbear/teddy"
	"flag"
	"log"
	"strings"
	"sync"
)

var (
	flagVerbose   = flag.Bool("verbose", false, "enable verbose logging")
	flagSymbol    = flag.String("symbol", "BTC-USD", "coinbase product to trade")
	flagPredictor = flag.String("predictor", "BTCUSDT@binanceusd", "predictor symbol@exchange (e.g. BTCUSDT@binanceusd, BTCFDUSD@binance)")
	flagSize      = decimal.Flag("size", "200", "order size in usd")
	flagUSD       = decimal.Flag("usd", "20000", "coinbase usd balance")
	flagThreshold = decimal.FlagBPS("threshold", "3", "minimum spread deviation to trade (basis points)")
	flagCooldown  = clocky.DurationFlag("cooldown", "1s", "minimum time between trades")
	flagFreshness = clocky.DurationFlag("freshness", "500ms", "max age of market data before suspending")
)

var (
	gCoinbase       *teddy.Exchange
	gCoinbasePair   *teddy.Pair
	gPredictorPair  *teddy.Pair
	gPredictorPrice decimal.Decimal
	gLastPredictor  clocky.Time
	gLastCoinbase   clocky.Time
	gLastTrade      clocky.Time
	gTradeLock      sync.Mutex
)

func main() {
	flag.Parse()
	loggy.Init()
	teddy.Init()

	// parse predictor flag
	predictorExchange, predictorSymbol := parsePredictor(*flagPredictor)

	log.Printf("arb: symbol=%s predictor=%s threshold=%sbps cooldown=%s",
		*flagSymbol, *flagPredictor, (*flagThreshold).BPS(), *flagCooldown)

	// setup coinbase (where we trade)
	gCoinbase = teddy.Exchanges.Get(ds.ExchangeCoinbase)
	gCoinbasePair = gCoinbase.Pairs.Get(*flagSymbol)
	gCoinbasePair.OnTick = onCoinbaseTick

	// setup predictor (where we watch)
	predictor := teddy.Exchanges.Get(predictorExchange)
	gPredictorPair = predictor.Pairs.Get(predictorSymbol)
	gPredictorPair.OnTick = onPredictorTick

	// for live mode, we need to start the binanceusd stream ourselves
	// since teddy doesn't have built-in support for it yet
	if teddy.Live && predictorExchange == ds.ExchangeBinanceUSD {
		go startBinanceUSDStream(predictorSymbol)
	}

	teddy.SetBalance(ds.ExchangeCoinbase, "USD", *flagUSD)
	teddy.SetBenchmark(gCoinbasePair)
	teddy.Run()
}

func startBinanceUSDStream(symbol string) {
	// convert BTC-USDT back to BTCUSDT for the api
	apiSymbol := strings.ReplaceAll(symbol, "-", "")
	client := binanceusd.NewClient()
	channel := binanceusd.MarketData(apiSymbol, client)
	for tick := range channel {
		gPredictorPair.OrderBook.Lock.Lock()
		if tick.Snap {
			gPredictorPair.OrderBook.Clear()
		}
		for _, bid := range tick.Bids {
			gPredictorPair.OrderBook.UpdateBid(bid.Price, bid.Size)
		}
		for _, ask := range tick.Asks {
			gPredictorPair.OrderBook.UpdateAsk(ask.Price, ask.Size)
		}
		gPredictorPair.OrderBook.Lock.Unlock()
		for _, trade := range tick.Trades {
			gPredictorPair.LastPrice = trade.Price
		}
		teddy.Spawn(func() { onPredictorTickImpl(tick) })
	}
}

func onCoinbaseTick(tick *ds.Tick) {
	teddy.Spawn(func() {
		if len(tick.Bids) > 0 || len(tick.Asks) > 0 {
			if tick.Time > gLastCoinbase {
				gLastCoinbase = tick.Time
			}
		}
	})
}

func onPredictorTick(tick *ds.Tick) {
	teddy.Spawn(func() {
		onPredictorTickImpl(tick)
	})
}

func onPredictorTickImpl(tick *ds.Tick) {
	// react to predictor trades
	for _, trade := range tick.Trades {
		if trade.Price.IsPositive() {
			prevPrice := gPredictorPrice
			gPredictorPrice = trade.Price
			if trade.Time > gLastPredictor {
				gLastPredictor = trade.Time
			}
			// only check for arb opportunity if we have a previous price
			if prevPrice.IsPositive() {
				checkArb(tick.Time, trade.Side, prevPrice, trade.Price)
			}
		}
	}
}

func checkArb(now clocky.Time, predictorSide ds.Side, prevPrice, newPrice decimal.Decimal) {
	// don't trade too frequently
	if now.Sub(gLastTrade) < *flagCooldown {
		return
	}

	// don't trade if data is stale
	if now.Sub(gLastPredictor) > *flagFreshness || now.Sub(gLastCoinbase) > *flagFreshness {
		return
	}

	// get coinbase book
	depth := (*flagSize).Div(newPrice)
	coinbaseBid := gCoinbasePair.OrderBook.PickBid(depth)
	coinbaseAsk := gCoinbasePair.OrderBook.PickAsk(depth)
	if coinbaseBid.IsZero() || coinbaseAsk.IsZero() {
		return
	}
	coinbaseMid := coinbaseBid.Add(coinbaseAsk).DivInt(2)

	// calculate how stale coinbase is relative to predictor
	// staleness = (coinbase_mid - predictor_price) / predictor_price
	staleness := coinbaseMid.Sub(newPrice).Div(newPrice)

	// trading logic:
	// if predictor sold (price dropped) and Coinbase is still high -> SELL on Coinbase
	// if predictor bought (price rose) and Coinbase is still low -> BUY on Coinbase
	var side ds.Side
	var edgeBPS decimal.Decimal

	if predictorSide == ds.SideSell && staleness.Cmp(*flagThreshold) > 0 {
		// predictor dumped, coinbase bid is stale high - sell into it
		side = ds.SideSell
		edgeBPS = staleness.BPS()
	} else if predictorSide == ds.SideBuy && staleness.Neg().Cmp(*flagThreshold) > 0 {
		// predictor pumped, coinbase ask is stale low - buy it
		side = ds.SideBuy
		edgeBPS = staleness.Neg().BPS()
	} else {
		return
	}

	if *flagVerbose {
		log.Printf("[signal] predictor %s, coinbase stale by %sbps, action=%s",
			predictorSide, staleness.BPS().Format(2), side)
	}

	executeTrade(now, side, edgeBPS)
}

func executeTrade(now clocky.Time, side ds.Side, edgeBPS decimal.Decimal) {
	// get pair info
	gCoinbasePair.Lock.RLock()
	lastPrice := gCoinbasePair.LastPrice
	baseIncrement := gCoinbasePair.BaseIncrement
	quoteIncrement := gCoinbasePair.QuoteIncrement
	gCoinbasePair.Lock.RUnlock()

	if !lastPrice.IsPositive() {
		return
	}

	// calculate quantity
	quantity := (*flagSize).Div(lastPrice).QuantizeNearest(baseIncrement)

	// calculate limit price with some slippage allowance
	// for buys: willing to pay up to 5bps above mid
	// for sells: willing to sell down to 5bps below mid
	slippageBPS := decimal.Parse("0.0005") // 5 bps
	var limitPrice decimal.Decimal
	switch side {
	case ds.SideBuy:
		limitPrice = lastPrice.Mul(decimal.One.Add(slippageBPS))
	case ds.SideSell:
		limitPrice = lastPrice.Mul(decimal.One.Sub(slippageBPS))
	}
	limitPrice = limitPrice.Quantize(quoteIncrement)

	// acquire trade lock
	gTradeLock.Lock()
	if now.Sub(gLastTrade) < *flagCooldown {
		gTradeLock.Unlock()
		return
	}
	gLastTrade = now
	gTradeLock.Unlock()

	// place ioc limit order
	t0 := now
	t1 := clocky.Now()
	order, err := gCoinbasePair.LimitOrder(side, quantity, limitPrice, ds.OrderStrategyIOC)
	t2 := clocky.Now()
	if err != nil {
		if *flagVerbose {
			log.Printf("[error] failed to place order: %v", err)
		}
		return
	}

	// wait for order completion
	order.Wait()
	t3 := clocky.Now()

	// report result
	if order.State == ds.OrderStateFilled {
		value := order.Filled.Mul(order.FillPrice)
		log.Printf("[trade] %s $%s @ $%s edge=%sbps latency=%s",
			side,
			value.Quantize(quoteIncrement),
			order.FillPrice.Quantize(quoteIncrement),
			edgeBPS.Format(1),
			t3.Sub(t0))
	} else if order.Filled.IsPositive() {
		value := order.Filled.Mul(order.FillPrice)
		log.Printf("[partial] %s $%s @ $%s (wanted $%s)",
			side,
			value.Quantize(quoteIncrement),
			order.FillPrice.Quantize(quoteIncrement),
			(*flagSize).Format(0))
	} else if *flagVerbose {
		log.Printf("[miss] %s order not filled (state=%s)", side, order.State)
	}

	if teddy.Live {
		log.Printf("[perf] decided in %s ordered in %s acknowledged in %s",
			t1.Sub(t0), t2.Sub(t1), t3.Sub(t2))
	}
}

func parsePredictor(s string) (ds.Exchange, string) {
	parts := strings.Split(s, "@")
	if len(parts) != 2 {
		loggy.Fatalf("invalid predictor format: %s (expected SYMBOL@exchange)", s)
	}
	symbol := parts[0]
	exchangeName := parts[1]
	var exchange ds.Exchange
	switch exchangeName {
	case "binance":
		exchange = ds.ExchangeBinance
		// convert to teddy format: BTCFDUSD -> BTC-FDUSD
		symbol = convertBinanceSymbol(symbol)
	case "binanceusd":
		exchange = ds.ExchangeBinanceUSD
		symbol = convertBinanceSymbol(symbol)
	default:
		loggy.Fatalf("unknown exchange: %s", exchangeName)
	}
	return exchange, symbol
}

func convertBinanceSymbol(s string) string {
	// Convert BTCUSDT -> BTC-USDT, BTCFDUSD -> BTC-FDUSD, etc.
	for _, quote := range []string{"USDT", "USDC", "FDUSD", "BUSD"} {
		if strings.HasSuffix(s, quote) {
			base := strings.TrimSuffix(s, quote)
			return base + "-" + quote
		}
	}
	return s
}
