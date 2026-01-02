//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|         market taking algorithm x3.161-2025

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
	"net/http"
	_ "net/http/pprof"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var (
	flagDebug     = flag.Bool("debug", false, "enable debug thing")
	flagLevel2    = flag.Bool("level2", false, "order book prediction")
	flagVerbose   = flag.Bool("verbose", false, "enable verbose logging")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase currency to trade")
	flagPredictor = flag.String("predictor", "BTCFDUSD@binance", "predictor symbol@broker")
	flagPricer    = flag.String("pricer", "", "currency to usd pair, e.g. FDUSDUSDT@binance")
	flagDepth     = decimal.Flag("depth", "1", "order book depth for determining bid/ask")
	flagUSD       = decimal.Flag("usd", "50000", "coinbase usd balance")
	flagCoin      = decimal.Flag("coin", "0.4", "symbol balance in base currency")
	flagBuffer    = decimal.FlagPercent("buffer", "1", "percent of balance buffer to keep free")
	flagThreshold = decimal.FlagBPS("threshold", "5", "minimum spread deviation to trade (basis points)")
	flagCooldown  = clocky.DurationFlag("cooldown", "400ms", "minimum time between trades")
	flagFreshness = clocky.DurationFlag("freshness", "400ms", "max age of market data before suspending")
	flagSamples   = flag.Int("samples", 1000, "number of sample trades used to determine baseline spread")
	flagClean     = decimal.FlagPercent("clean", "0", "we must clean at least this percent of stale content")
)

var (
	gCash          *teddy.Holding
	gHolding       *teddy.Holding
	gCoinbase      *teddy.Broker
	gCoinbasePair  *teddy.Pair
	gPredictorPair *teddy.Pair
	gPricerPair    *teddy.Pair
	gLastCoinbase  clocky.Time
	gPricerPrice   decimal.Decimal
	gLastOrder     clocky.Time
	gOrderLock     sync.Mutex
	gSpreadEMA     *indicators.WWMA
	gSpreadLock    sync.Mutex
	gStarted       clocky.Time
	gReady         bool
)

func main() {
	flag.Parse()
	loggy.Init()
	teddy.Init()

	if *flagDebug {
		go http.ListenAndServe("localhost:6060", nil)
		launchBrowser("http://localhost:6060/debug/pprof/")
	}

	// initialize spread baseline tracker
	gSpreadEMA = indicators.NewWWMA(*flagSamples)

	// setup coinbase (where we trade)
	gCoinbase = teddy.Brokers.Get(ds.BrokerCoinbase)
	gCoinbasePair = gCoinbase.Pairs.Get(*flagSymbol + "-USD")
	gHolding = gCoinbase.Holdings.Get(*flagSymbol)
	gCash = gCoinbase.Holdings.Get("USD")

	// setup predictor (where we watch)
	predictorBroker, predictorSymbol := parsePredictor(*flagPredictor)
	predictor := teddy.Brokers.Get(predictorBroker)
	gPredictorPair = predictor.Pairs.Get(predictorSymbol)

	// setup pricer (optional currency to usd pair)
	if *flagPricer != "" {
		pricerBroker, pricerSymbol := parsePredictor(*flagPricer)
		pricer := teddy.Brokers.Get(pricerBroker)
		gPricerPair = pricer.Pairs.Get(pricerSymbol)
		gPricerPair.OnTick = onPricerTick
	}

	// prepare for arbitrage
	gPricerPrice = decimal.One
	teddy.Brokers.OnReady = onReady
	teddy.SetBalance(ds.BrokerCoinbase, *flagSymbol, *flagCoin)
	teddy.SetBalance(ds.BrokerCoinbase, "USD", *flagUSD)
	teddy.SetBenchmark(gCoinbasePair)
	teddy.Run()
}

func onReady() {
	log.Printf("[startup] ready, steady, go")
	gCoinbasePair.OnTick = onCoinbaseTick
	gPredictorPair.OnTick = onPredictorTick
	gSpreadLock.Lock()
	gStarted = clocky.Now()
	gSpreadLock.Unlock()
}

func onCoinbaseTick(tick *ds.Tick) {
	if len(tick.Bids) > 0 || len(tick.Asks) > 0 {
		gLastCoinbase.Store(tick.Time)
	}
}

func onPricerTick(tick *ds.Tick) {
	if len(tick.Bids) > 0 || len(tick.Asks) > 0 {
		gPricerPair.Lock.RLock()
		gPricerPrice.Store(gPricerPair.Book.MidPrice())
		gPricerPair.Lock.RUnlock()
	}
}

func onPredictorTick(tick *ds.Tick) {
	teddy.Spawn(func() {
		if *flagLevel2 {
			if len(tick.Bids) > 0 || len(tick.Asks) > 0 {
				gPredictorPair.Lock.RLock()
				bid := gPredictorPair.Book.PickBidByValue(*flagDepth)
				ask := gPredictorPair.Book.PickAskByValue(*flagDepth)
				mid := bid.Add(ask).DivInt(2).Mul(gPricerPrice.Load())
				gPredictorPair.Lock.RUnlock()
				if !mid.IsPositive() {
					log.Printf("[error] somehow have non-positive predictor midpoint price %s", mid)
					return
				}
				arbitrage(tick.Time, tick.Time, mid)
			}
		} else {
			for _, trade := range tick.Trades {
				arbitrage(trade.Time, tick.Time, trade.Price)
			}
		}
	})
}

func arbitrage(tradeTime, receivedTime clocky.Time, predictorPrice decimal.Decimal) {

	if teddy.Live {
		now := clocky.Now()
		goDelay := now.Sub(receivedTime)
		if goDelay > 2*clocky.Millisecond {
			log.Printf("[info] tick took %s to deliver", goDelay)
		}
	}

	// calculate spread between coinbase and predictor
	// spread = (midpoint - prediction) / prediction
	gCoinbasePair.Lock.RLock()
	bid, ask := gCoinbasePair.Book.BestBidAsk()
	gCoinbasePair.Lock.RUnlock()
	mid := bid.Add(ask).DivInt(2)
	if !mid.IsPositive() {
		log.Printf("[error] somehow have non-positive midpoint price %s", mid)
		return
	}
	spread := mid.Sub(predictorPrice).Div(predictorPrice)

	// determine how much spread differs from what it normally is
	gSpreadLock.Lock()
	gSpreadEMA.Add(spread)
	baseline := gSpreadEMA.Value
	isReady := gSpreadEMA.IsReady()
	if !isReady {
		if teddy.Live {
			log.Printf("[warmup] WWMA is %.2f%% ready", gSpreadEMA.Progress()*100)
		}
		gSpreadLock.Unlock()
		return
	}
	if !gReady {
		warmupDuration := receivedTime.Sub(gStarted)
		log.Printf("[warmup] warmup took %s", warmupDuration)
		gReady = true
	}
	gSpreadLock.Unlock()
	deviation := spread.Sub(baseline)

	// the important thing to understand about arbitrage, is this game is not about
	// how we can make money off a big spread. the question is how much trash we're
	// able to clear from the order book while recovering our costs.
	//
	// let's say btc on coinbase usually costs 99, and on binance it's usually 100.
	// in that case, m=99, P=100, and b=-0.01. suddenly the price on binance shoots
	// up to 102 but market makers on coinbase are still offering to buy / sell btc
	// for 99. this is an abnormal price difference between the two brokers, plus
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
	// however the broker will reject that limit price because it's not rounded
	// to the nearest penny. so which direction do we round? for buys, we want to
	// round down, because buying low is good. whereas sells we want to round up.

	var side ds.Side
	var size, limi decimal.Decimal
	predictedPrice := predictorPrice.Mul(decimal.One.Add(baseline))
	move := predictedPrice.Sub(mid).Div(mid)

	if predictedPrice.Cmp(mid) > 0 {
		side = ds.SideBuy
		size = gCash.Available.Load().Mul(decimal.One.Sub(*flagBuffer)).Div(predictedPrice)
		size = size.QuantizeTruncate(gCoinbasePair.BaseIncrement.Load())
		limi = predictedPrice.Div(decimal.One.Add(*flagThreshold))
		limi = limi.QuantizeTruncate(gCoinbasePair.QuoteIncrement.Load())
		if limi.Cmp(ask) < 0 {
			return
		}
	} else {
		side = ds.SideSell
		size = gHolding.Available.Load().Mul(decimal.One.Sub(*flagBuffer))
		size = size.QuantizeTruncate(gCoinbasePair.BaseIncrement.Load())
		limi = predictedPrice.Mul(decimal.One.Add(*flagThreshold))
		limi = limi.QuantizeAway(gCoinbasePair.QuoteIncrement.Load())
		if limi.Cmp(bid) > 0 {
			return
		}
	}

	logDecision := func() {
		if *flagVerbose {
			var edge decimal.Decimal
			switch side {
			case ds.SideBuy:
				edge = ask.Sub(limi).Div(limi)
			case ds.SideSell:
				edge = limi.Sub(bid).Div(bid)
			}
			log.Printf("[signal] %s spread=%s baseline=%s deviation=%s move=%s edge=%s mid=%s predictor=%s predicted=%s limit=%s",
				side, spread.BPS().Format(3), baseline.BPS().Format(3), deviation.BPS().Format(3), move.BPS().Format(3),
				edge.BPS().Format(3), mid, predictorPrice, predictedPrice, limi)
		}
	}

	// don't feed mount everest
	var wallLevels int
	var wallSize decimal.Decimal
	if !gCoinbasePair.Lock.TryRLock() {
		log.Printf("[skip] coinbase pair lock is busy")
		return
	}
	if side == ds.SideBuy {
		it := gCoinbasePair.Book.Asks.Iterator()
		for it.Next() {
			price := it.Key()
			size := it.Value()
			if price.Cmp(limi) > 0 {
				break
			}
			wallSize = wallSize.Add(size)
			wallLevels++
		}
	} else {
		it := gCoinbasePair.Book.Bids.Iterator()
		for it.Next() {
			price := it.Key()
			size := it.Value()
			if price.Cmp(limi) < 0 {
				break
			}
			wallSize = wallSize.Add(size)
			wallLevels++
		}
	}
	gCoinbasePair.Lock.RUnlock()
	cleaned := size.Div(wallSize)
	if cleaned.Cmp(*flagClean) < 0 {
		logDecision()
		log.Printf("[skip] would only clean %s%% of $%s across %d levels", cleaned.MulInt(100).Format(0), wallSize.Mul(mid).FormatThousand(2), wallLevels)
		return
	}

	// make sure it's a good time to trade
	gOrderLock.Lock()
	openOrders := gCoinbase.Orders.Open()
	for _, order := range openOrders {
		limitPrice := order.LimitPrice.Load()
		switch order.Side {
		case ds.SideBuy:
			if side == ds.SideBuy && limi.Cmp(order.LimitPrice) <= 0 {
				gOrderLock.Unlock()
				logDecision()
				log.Printf("[skip] our buy edge is deteriorating! limit price went from %s to %s", limitPrice, limi)
				return
			}
		case ds.SideSell:
			if side == ds.SideSell && limi.Cmp(order.LimitPrice) >= 0 {
				gOrderLock.Unlock()
				logDecision()
				log.Printf("[skip] our sell edge is deteriorating! limit price went from %s to %s", limitPrice, limi)
				return
			}
		}
	}
	orderTime := clocky.Now()
	lastCoinbase := gLastCoinbase.Load()
	if orderTime.Sub(lastCoinbase) > *flagFreshness {
		gOrderLock.Unlock()
		logDecision()
		log.Printf("[skip] coinbase order book isn't fresh because last update was received %s ago",
			orderTime.Sub(lastCoinbase))
		return
	}
	if orderTime.Sub(tradeTime) > *flagFreshness {
		gOrderLock.Unlock()
		logDecision()
		log.Printf("[skip] predictor trade data isn't fresh because it happened %s ago, which we learned about %s ago",
			orderTime.Sub(tradeTime), orderTime.Sub(receivedTime))
		return
	}
	gOrderLock.Unlock()

	// place immediate-or-cancel limit order
	order, err := gCoinbasePair.LimitOrder(side, size, limi, ds.OrderStrategyIOC)
	if err != nil {
		logDecision()
		log.Printf("[error] failed to place %s order: %v", side, err)
		return
	}

	// wait for order completion
	sentTime := clocky.Now()
	logDecision()
	order.Wait()
	ackTime := clocky.Now()

	// report result
	if order.Filled.IsPositive() {
		log.Printf("[trade] %s %s %s of %s at $%s", side, order.Notional, order.Pair.QuoteCurrency, order.Pair.BaseCurrency, order.Price)
	} else if *flagVerbose {
		log.Printf("[miss] %s order not filled (state=%s)", side, order.State)
	}

	if teddy.Live {
		log.Printf("[perf] decided in %s ordered in %s acknowledged in %s round-tripped in %s",
			orderTime.Sub(receivedTime), sentTime.Sub(orderTime), ackTime.Sub(sentTime), ackTime.Sub(receivedTime))
	}
}

func parsePredictor(s string) (ds.Broker, string) {
	parts := strings.Split(s, "@")
	if len(parts) != 2 {
		loggy.Fatalf("invalid predictor format: %s (expected SYMBOL@broker)", s)
	}
	return ds.MustParseBroker(parts[1]), parts[0]
}

func launchBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	if cmd != nil {
		cmd.Run()
	}
}
