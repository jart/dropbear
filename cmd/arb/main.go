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
	flagVerbose   = flag.Bool("verbose", false, "enable verbose logging")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase product to trade")
	flagPredictor = flag.String("predictor", "BTCUSDT@binanceusd", "predictor symbol@exchange (e.g. BTCUSDT@binanceusd, BTCFDUSD@binance)")
	flagUSD       = decimal.Flag("usd", "10000", "coinbase usd balance")
	flagCoin      = decimal.Flag("coin", "0.4", "symbol balance in base currency")
	flagBuffer    = decimal.FlagPercent("buffer", "0.5", "percent of balance buffer to keep free")
	flagThreshold = decimal.FlagBPS("threshold", "3", "minimum spread deviation to trade (basis points)")
	flagCooldown  = clocky.DurationFlag("cooldown", "150ms", "minimum time between trades")
	flagFreshness = clocky.DurationFlag("freshness", "1s", "max age of market data before suspending")
	flagSamples   = flag.Int("samples", 5000, "number of sample trades used to determine baseline spread")
)

var (
	gCash          *teddy.Holding
	gHolding       *teddy.Holding
	gCoinbase      *teddy.Exchange
	gCoinbasePair  *teddy.Pair
	gPredictorPair *teddy.Pair
	gLastCoinbase  clocky.Time
	gLastOrder     clocky.Time
	gOrderLock     sync.Mutex
	gSpreadEMA     *indicators.WWMA
	gSpreadLock    sync.Mutex
)

func main() {
	flag.Parse()
	loggy.Init()
	teddy.Init()

	// parse predictor flag
	predictorExchange, predictorSymbol := parsePredictor(*flagPredictor)

	log.Printf("arb: symbol=%s predictor=%s threshold=%sbps cooldown=%s samples=%d",
		*flagSymbol, *flagPredictor, (*flagThreshold).BPS(), *flagCooldown, *flagSamples)

	// initialize spread baseline tracker
	gSpreadEMA = indicators.NewWWMA(*flagSamples)

	// setup coinbase (where we trade)
	gCoinbase = teddy.Exchanges.Get(ds.ExchangeCoinbase)
	gCoinbasePair = gCoinbase.Pairs.Get(*flagSymbol + "-USD")
	gHolding = gCoinbase.Holdings.Get(*flagSymbol)
	gCash = gCoinbase.Holdings.Get("USD")
	gCoinbasePair.OnTick = onCoinbaseTick

	// setup predictor (where we watch)
	predictor := teddy.Exchanges.Get(predictorExchange)
	gPredictorPair = predictor.Pairs.Get(predictorSymbol)
	gPredictorPair.OnTick = onPredictorTick

	teddy.SetBalance(ds.ExchangeCoinbase, *flagSymbol, *flagCoin)
	teddy.SetBalance(ds.ExchangeCoinbase, "USD", *flagUSD)
	teddy.SetBenchmark(gCoinbasePair)
	teddy.Run()
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
		for _, trade := range tick.Trades {
			arbitrage(tick.Time, trade.Price)
		}
	})
}

func arbitrage(now clocky.Time, predictorPrice decimal.Decimal) {

	// don't trade too frequently
	if now.Sub(gLastOrder) < *flagCooldown {
		return
	}

	// calculate spread between coinbase and predictor
	// spread = (midpoint - prediction) / prediction
	bid, ask := gCoinbasePair.OrderBook.BestBidAsk()
	mid := bid.Add(ask).DivInt(2)
	spread := mid.Sub(predictorPrice).Div(predictorPrice)

	// determine how much spread differs from what it normally is
	gSpreadLock.Lock()
	gSpreadEMA.Add(spread)
	baseline := gSpreadEMA.Value
	isReady := gSpreadEMA.IsReady()
	gSpreadLock.Unlock()
	if !isReady {
		return
	}
	deviation := spread.Sub(baseline)

	// let's say btc on coinbase usually costs 99, and on binance it's usually 100.
	// in that case, m=99, P=100, and b=-0.01. suddenly the price on binance shoots
	// up to 102 but market makers on coinbase are still offering to buy / sell btc
	// for 99. this is an abnormal price difference between the two exchanges, plus
	// we know from statistical analysis that where binance goes, coinbase follows.
	//
	// so right now
	//
	//     s = (m-P)/P = -0.03
	//
	// and
	//
	//     d = s-b = -0.02
	//
	// that deviation is our opportunity. to become normal again, coinbase's price
	// must rise to 101 so it's only 1% below binance. that's the price we predict
	// coinbase will reach soon
	//
	//     p = P*(1+b) = 102*(1-0.01) = 100.98
	//
	// since p > m, we want to buy on coinbase, because we're predicting the price
	// will rise to p. all the market makers on coinbase with ask orders, offering
	// to sell btc to use between m (99) and p (100.98) are offering an attractive
	// deal. but how much of that liquidity is safe for us to take?
	//
	// if we're vip4 on coinbase, we pay a 0.04875% taker fee, which is 0.0004875,
	// also known as f which is about five basis points. so what is the highest we
	// can pay (q) for btc, before we're underwater? consider how order fees work:
	//
	//     p = q*(1+f)
	//
	// you have your order value and they add their fee. therefore to round trip:
	//
	//     q = p/(1+f)
	//
	// we should use q as our limit price. which is
	//
	//     q = 100.98 / (1 + 0.0004875) = 100.93079623683455
	//
	// because
	//
	//     100.93079623683455 * (1 + 0.0004875) = 100.98
	//
	// however the exchange will reject that limit price because it's not rounded
	// to the nearest penny. so which direction do we round? for buys, we want to
	// round down, because buying low is good. whereas sells we want to round up.

	var side ds.Side
	var size decimal.Decimal
	var limi decimal.Decimal
	predictedPrice := predictorPrice.Mul(decimal.One.Add(baseline))
	if predictedPrice.Cmp(mid) > 0 {
		side = ds.SideBuy
		size = gCash.Available.Mul(decimal.One.Sub(*flagBuffer)).Div(predictedPrice)
		limi = predictedPrice.Div(decimal.One.Add(*flagThreshold))
		limi = limi.QuantizeDown(gCoinbasePair.QuoteIncrement)
		if limi.Cmp(ask) < 0 {
			return
		}
	} else {
		side = ds.SideSell
		size = gHolding.Available.Mul(decimal.One.Sub(*flagBuffer))
		limi = predictedPrice.Mul(decimal.One.Add(*flagThreshold))
		limi = limi.QuantizeUp(gCoinbasePair.QuoteIncrement)
		if limi.Cmp(bid) > 0 {
			return
		}
	}

	if *flagVerbose {
		log.Printf("[signal] spread=%sbps baseline=%sbps dev=%sbps predicted=$%s limit=$%s action=%s",
			spread.BPS().Format(2), baseline.BPS().Format(2),
			deviation.BPS().Format(2), predictedPrice.Format(2), limi.Format(2), side)
	}

	// acquire trade lock
	gOrderLock.Lock()
	if now.Sub(gLastOrder) < *flagCooldown {
		gOrderLock.Unlock()
		return
	}
	gLastOrder = now
	gOrderLock.Unlock()

	// place immediate-or-cancel limit order
	t0 := now
	t1 := clocky.Now()
	size = size.QuantizeDown(gCoinbasePair.BaseIncrement)
	order, err := gCoinbasePair.LimitOrder(side, size, limi, ds.OrderStrategyIOC)
	t2 := clocky.Now()
	if err != nil {
		log.Printf("[error] failed to place order: %v", err)
		return
	}

	// wait for order completion
	order.Wait()
	t3 := clocky.Now()

	// report result
	if order.State == ds.OrderStateFilled {
		value := order.Filled.Mul(order.FillPrice)
		log.Printf("[trade] %s $%s @ $%s latency=%s",
			side, value.Quantize(gCoinbasePair.QuoteIncrement),
			order.FillPrice.Quantize(gCoinbasePair.QuoteIncrement),
			t3.Sub(t0))
	} else if order.Filled.IsPositive() {
		value := order.Filled.Mul(order.FillPrice)
		log.Printf("[partial] %s $%s @ $%s (wanted $%s)",
			side, value.Quantize(gCoinbasePair.QuoteIncrement),
			order.FillPrice.Quantize(gCoinbasePair.QuoteIncrement),
			size.Format(0))
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
