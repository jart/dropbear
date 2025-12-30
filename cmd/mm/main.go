//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|         market making algorithm x3.160-2025

package main

import (
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/teddy"
	"flag"
	"log"
	"sync"
)

var (
	flagVerbose   = flag.Bool("verbose", false, "enable verbosity")
	flagSize      = decimal.Flag("size", "500", "order size in usd")
	flagUSD       = decimal.Flag("usd", "20000", "coinbase usd balance")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase product to trade")
	flagTarget    = decimal.Flag("target", "7000", "target inventory in usd")
	flagSpread    = decimal.FlagBPS("spread", "2", "spread threshold in basis points")
	flagSpreadMin = decimal.FlagBPS("spread-min", "0.5", "minimum spread threshold in basis points")
	flagSpreadMax = decimal.FlagBPS("spread-max", "10", "maximum spread threshold in basis points")
	flagProfit    = decimal.FlagBPS("profit", "5", "profit threshold in basis points")
	flagPanic     = decimal.FlagBPS("panic", "15", "panic threshold to sell at a loss")
	flagBuyGap    = decimal.FlagBPS("buygap", "7", "only buy if price is this many basis points below last buy")
	flagBuyDecay  = clocky.DurationFlag("decay", "1m", "base decay period for buygap after sells")
	flagSkew      = decimal.Flag("skew", "1", "spread adjustment per 100% inventory imbalance")
	flagWindow    = clocky.DurationFlag("window", "42m", "time window for min/max range protection (0 disables)")
	flagComfort   = decimal.FlagPercent("comfort", "20", "percent of min/max window we're comfortable buying or selling")
	flagDanger    = decimal.FlagPercent("danger", "0", "percent of min/max window at which it's probably volatility expansion")
	flagSamples   = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagCooldown  = clocky.DurationFlag("cooldown", "5s", "duration to wait between activities")
	flagFreshness = clocky.DurationFlag("freshness", "1500ms", "suspend trading after this long an outage")
	flagIntensity = clocky.DurationFlag("intensity", "2h", "trading intensity window (e.g. 5m, 0 for disabled)")
	flagPassive   = decimal.FlagBPS("passive", "5", "passive order spread from mid in basis points")
	flagRelist    = decimal.FlagBPS("relist", "1", "relist passive order if price moves this many bps")
)

var (
	gCoinbase      *teddy.Broker
	gBinance       *teddy.Broker
	gHolding       *teddy.Holding
	gCoinbasePair  *teddy.Pair
	gBinancePair   *teddy.Pair
	gLastCoinbase  clocky.Time
	gLastBinance   clocky.Time
	gLastTrade     clocky.Time
	gLastActivity  clocky.Time
	gLastSellTime  clocky.Time
	gLastStatus    clocky.Time
	gBinancePrice  decimal.Decimal
	gCoinbaseMin   decimal.Decimal
	gCoinbaseMax   decimal.Decimal
	gCoinbasePrice decimal.Decimal
	gTradeLock     sync.Mutex
	gStatusLock    sync.Mutex
	gIntensityLock sync.Mutex
	gSpreadLock    sync.Mutex
	gPriceLock     sync.Mutex
	gPassiveLock   sync.Mutex
	gPassiveBid    *teddy.Order
	gPassiveAsk    *teddy.Order
	gIntensity     *indicators.Intensity
	gSpreadEMA     *indicators.WWMA
	gPriceMin      *indicators.Min
	gPriceMax      *indicators.Max
	gFirstEvent    clocky.Time
	gWarmedUp      bool

	// status indicators (raw values for logging)
	gDeviation decimal.Decimal // current spread deviation from baseline
	gGreed     decimal.Decimal // inventory-based greed factor
	gBalance   decimal.Decimal // buy/sell volume balance ratio
	gBidDelta  decimal.Decimal // passive bid spread from mid
	gAskDelta  decimal.Decimal // passive ask spread from mid
	gInvRatio  decimal.Decimal // inventory / target ratio
	gRangePos  decimal.Decimal // price position in min/max range (0=min, 1=max)
)

func main() {
	flag.Parse()
	loggy.Init()
	teddy.Init()

	log.Printf("spread=%sbps, panic=%sbps, samples=%d, cooldown=%s, size=$%s, window=%s comfort=%s danger=%s intensity=%s",
		(*flagSpread).BPS(), (*flagPanic).BPS(), *flagSamples, *flagCooldown, *flagSize, *flagWindow, *flagComfort, *flagDanger, *flagIntensity)

	gSpreadEMA = indicators.NewWWMA(*flagSamples)
	if *flagWindow != 0 {
		gPriceMin = indicators.NewMin(*flagWindow)
		gPriceMax = indicators.NewMax(*flagWindow)
	}
	if *flagIntensity != 0 {
		gIntensity = indicators.NewIntensity(*flagIntensity)
	}

	// preseed min/max indicators from historical candles BEFORE starting
	// the live data stream, otherwise live ticks could set first > last
	if teddy.Live && gPriceMin != nil {
		client := teddy.CoinbaseClient
		symbol := *flagSymbol + "-USD"
		granularity := coinbase.CandleGranularityMinute
		candles, err := client.GetCandles(symbol, granularity, 0, 0, 0)
		if err != nil {
			log.Printf("failed to fetch candles: %v", err)
		} else if len(candles) == 0 {
			log.Printf("no candles returned for %s", symbol)
		} else {
			log.Printf("candles go from %s to %s", candles[0].Start, candles[len(candles)-1].Start)
			for _, c := range candles {
				gPriceMin.Add(c.Start, c.Low)
				gPriceMax.Add(c.Start, c.High)
			}
			if gPriceMin.IsReady() && gPriceMax.IsReady() {
				gCoinbaseMin = gPriceMin.Value
				gCoinbaseMax = gPriceMax.Value
				log.Printf("preseeded min/max from %d candles: $%s - $%s",
					len(candles), gCoinbaseMin.Format(2), gCoinbaseMax.Format(2))
			} else {
				log.Printf("loaded %d candles but min/max not ready", len(candles))
			}
		}
	}

	gCoinbase = teddy.Brokers.Get(ds.BrokerCoinbase)
	gHolding = gCoinbase.Holdings.Get(*flagSymbol)
	gCoinbasePair = gCoinbase.Pairs.Get(*flagSymbol + "-USD")
	gCoinbasePair.OnTick = onBinanceTick

	gBinance = teddy.Brokers.Get(ds.BrokerBinance)
	if *flagSymbol == "ZEC" {
		gBinancePair = gBinance.Pairs.Get(*flagSymbol + "USDT")
	} else {
		gBinancePair = gBinance.Pairs.Get(*flagSymbol + "FDUSD")
	}
	gBinancePair.OnTick = onBinanceTick

	teddy.SetBalance(ds.BrokerCoinbase, "USD", *flagUSD)
	teddy.SetBenchmark(gCoinbasePair)
	teddy.Run()
}

func onBinanceTick(tick *ds.Tick) {
	teddy.Spawn(func() {
		onBinanceTickImpl(tick)
	})
}

func onCoinbaseTick(tick *ds.Tick) {
	teddy.Spawn(func() {
		onCoinbaseTickImpl(tick)
	})
}

func onBinanceTickImpl(tick *ds.Tick) {
	if gFirstEvent.IsZero() {
		gFirstEvent = tick.Time
	}

	// log status every minute
	if shouldLogStatus(tick.Time) {
		logStatus()
	}

	// track intensity of binance trading
	if gIntensity != nil && len(tick.Trades) > 0 {
		midPrice := gBinancePair.OrderBook.MidPrice()
		gIntensityLock.Lock()
		gIntensity.SetMidPrice(midPrice)
		for _, trade := range tick.Trades {
			gIntensity.AddTrade(trade.Time, trade.Price, trade.Quantity)
		}
		gIntensityLock.Unlock()
	}

	// react to binance trade data
	for _, trade := range tick.Trades {
		if trade.Price.IsPositive() {
			gBinancePrice = trade.Price
			if trade.Time > gLastBinance {
				gLastBinance = trade.Time
			}
		}
	}
	if gBinancePrice.IsPositive() {
		checkSpread(tick.Time)
	}
}

func onCoinbaseTickImpl(tick *ds.Tick) {
	if gFirstEvent.IsZero() {
		gFirstEvent = tick.Time
	}

	// update min/max range indicators only on coinbase ticks
	// to maintain consistent sampling rate for range calculation
	if len(tick.Trades) > 0 {
		gPriceLock.Lock()
		for _, trade := range tick.Trades {
			gPriceMin.Add(trade.Time, trade.Price)
			gPriceMax.Add(trade.Time, trade.Price)
			if gPriceMin.IsReady() && gPriceMax.IsReady() {
				gCoinbaseMin = gPriceMin.Value
				gCoinbaseMax = gPriceMax.Value
			}
		}
		gPriceLock.Unlock()
	}

	// react to coinbase order book changes
	if len(tick.Bids) > 0 || len(tick.Asks) > 0 {
		if gBinancePrice.IsPositive() {
			if tick.Time > gLastCoinbase {
				gLastCoinbase = tick.Time
			}
			checkSpread(tick.Time)
		}
	}
}

// now is the time when the tick was read by our websocket
func checkSpread(now clocky.Time) {

	// force warmup after an hour, come hell or high water
	forceStart := !gWarmedUp && now.Sub(gFirstEvent) > clocky.Hour

	// don't trade if firehose is compromised
	if now.Sub(gLastTrade) < *flagCooldown ||
		now.Sub(gLastActivity) < *flagCooldown ||
		now.Sub(gLastBinance) > *flagFreshness ||
		now.Sub(gLastCoinbase) > *flagFreshness {
		return
	}

	// compute spread
	depth := (*flagSize).Div(gBinancePrice)
	bid := gCoinbasePair.OrderBook.PickBid(depth)
	ask := gCoinbasePair.OrderBook.PickAsk(depth)
	gCoinbasePrice = bid.Add(ask).DivInt(2)
	spread := gCoinbasePrice.Sub(gBinancePrice).Div(gBinancePrice)
	gSpreadLock.Lock()
	gSpreadEMA.Add(spread)
	baseline := gSpreadEMA.Value
	deviation := spread.Sub(baseline)
	gDeviation = deviation
	isReady := gSpreadEMA.IsReady() || gPriceMin != nil
	gSpreadLock.Unlock()
	if !isReady && !forceStart {
		return
	}

	// get other indicators
	coinbaseMin := gCoinbaseMin
	coinbaseMax := gCoinbaseMax
	if gPriceMin != nil && coinbaseMin.IsZero() && !forceStart {
		return
	}
	// wait for intensity indicator to warm up before trading
	if gIntensity != nil && !forceStart {
		gIntensityLock.Lock()
		intensityReady := gIntensity.IsReady()
		gIntensityLock.Unlock()
		if !intensityReady {
			return
		}
	}
	if !gWarmedUp {
		gWarmedUp = true
		if forceStart {
			log.Printf("warmup forced after %s", now.Sub(gFirstEvent))
		} else {
			log.Printf("warmup complete in %s", now.Sub(gFirstEvent))
		}
	}

	// calculate coin health based on velocity balance
	// balance = min(buy, sell) / max(buy, sell)
	// health calculation combines:
	// 1. balance = min(buys, sells) / max(buys, sells) - trading flow balance
	// 2. winRate = wins / (wins + losses) - are we making money?
	// healthScore = balance * winRate (both must be healthy)
	gHolding.Lock.RLock()
	inventoryQty := gHolding.Quantity
	buyVolume := gHolding.BuyVolume
	sellVolume := gHolding.SellVolume
	winCount := gHolding.WinCount
	lossCount := gHolding.LossCount
	gHolding.Lock.RUnlock()

	// balance: 1.0 = perfect, 0 = completely one-sided
	balance := decimal.One
	maxVolume := max(buyVolume, sellVolume)
	if maxVolume > 0 {
		minVolume := min(buyVolume, sellVolume)
		balance = decimal.FromFloat64(minVolume / maxVolume)
	}
	gBalance = balance

	// winRate: 1.0 = all wins, 0 = all losses, default 1.0 if no trades
	winRate := decimal.One
	totalTrades := winCount + lossCount
	if totalTrades > 0 {
		winRate = decimal.FromInt(winCount).Div(decimal.FromInt(totalTrades))
	}

	// combined health score
	healthScore := balance.Mul(winRate)

	// effective target scales with health
	// if healthScore is 0.5, target becomes 50%
	// this prevents accumulating garbage coins
	effectiveTarget := (*flagTarget).Mul(healthScore)

	// calculate inventory-based spread adjustment using exponential greed
	// imbalance > 0 means overweight (have too much), want to sell more / buy less
	// imbalance < 0 means underweight (have too little), want to buy more / sell less
	// greed = exp(imbalance * skew) gives exponential penalty for being off-target
	greed := decimal.One
	buySpread := *flagSpread
	sellSpread := *flagSpread
	inventoryValue := inventoryQty.Mul(gCoinbasePrice)
	if (*flagTarget).IsPositive() {
		gInvRatio = inventoryValue.Div(*flagTarget)
	}
	if effectiveTarget.IsPositive() {
		imbalance := inventoryValue.Sub(effectiveTarget).Div(effectiveTarget)
		// exponential greed: exp(imbalance * skew)
		// when overweight: greed > 1, harder to buy, easier to sell
		// when underweight: greed < 1, easier to buy, harder to sell
		greed = imbalance.Mul(*flagSkew).Exp()
		// apply health penalty to greed - when health is poor, be more protective
		// this prevents accumulating garbage even when underweight
		// healthPenalty ranges from 1 (healthy) to 2 (unhealthy)
		healthPenalty := decimal.Two.Sub(healthScore)
		greed = greed.Mul(healthPenalty)
		buySpread = (*flagSpread).Mul(greed)
		sellSpread = (*flagSpread).Div(greed)
	}
	gGreed = greed

	// trading intensity adjustment (Avellaneda-Stoikov κ parameter)
	// high κ = tight liquidity (trades near mid) = can use tighter spreads
	// low κ = dispersed liquidity (trades spread out) = need wider spreads
	// scale = referenceKappa / actualKappa, clamped to [0.5, 2.0]
	if gIntensity != nil {
		kappa := gIntensity.Kappa
		if kappa.IsPositive() {
			// reference κ - BTC typically has κ around 20,000 (very tight)
			// scale = ref/κ, so higher ref = tighter spreads when tight
			referenceKoopa := decimal.FromInt(20000)
			intensityScale := referenceKoopa.Div(kappa)
			// clamp to reasonable range
			intensityScale = intensityScale.Max(decimal.Half).Min(decimal.Two)
			buySpread = buySpread.Mul(intensityScale)
			sellSpread = sellSpread.Mul(intensityScale)
		}
	}

	// clamp spread to reasonable range
	buySpread = buySpread.Max(*flagSpreadMin).Min(*flagSpreadMax)
	sellSpread = sellSpread.Max(*flagSpreadMin).Min(*flagSpreadMax)

	// figure out where price is positioned in recent price range
	disposition := Normal
	rangeSize := coinbaseMax.Sub(coinbaseMin)
	if rangeSize.IsPositive() {
		rangePosition := gCoinbasePrice.Sub(coinbaseMin).Div(rangeSize)
		gRangePos = rangePosition
		if rangePosition.Cmp(*flagComfort) < 0 {
			if rangePosition.Cmp(*flagDanger) < 0 {
				disposition = TooCheap
			} else {
				disposition = Cheap
			}
		}
		if rangePosition.Cmp(decimal.One.Sub(*flagComfort)) > 0 {
			if rangePosition.Cmp(decimal.One.Sub(*flagDanger)) > 0 {
				disposition = TooExpensive
			} else {
				disposition = Expensive
			}
		}
	}

	// deviation > threshold: coinbase got MORE expensive relative to normal, sell
	// when balance is poor (accumulating), be more eager to sell - reduce threshold
	effectiveSellSpread := sellSpread
	if balance.Cmp(decimal.Half) < 0 && balance.IsPositive() {
		// at balance=0.5, sellSpread is halved; at balance=0.25, quartered
		effectiveSellSpread = sellSpread.Mul(balance.MulInt(2))
	}

	if deviation.Cmp(effectiveSellSpread) > 0 && disposition == Expensive {
		if *flagVerbose {
			log.Printf("[logic] spread signal SELL spread=%sbps baseline=%sbps dev=%sbps coinbase=$%s binance=$%s",
				spread.BPS().Format(2),
				baseline.BPS().Format(2),
				deviation.BPS().Format(2),
				gCoinbasePrice,
				gBinancePrice)
		}
		executeTrade(now, ds.SideSell, deviation)
		return
	}

	// deviation < -threshold: coinbase got CHEAPER relative to normal, buy
	// but only if balance allows (don't accumulate when we can't sell)
	oneThird := decimal.One.DivInt(3)
	balanceOK := balance.Cmp(oneThird) >= 0 || balance.IsZero()
	hardCap := (*flagTarget).MulInt(2)
	underHardCap := inventoryValue.Cmp(hardCap) < 0

	// when balance is poor, require larger deviation to buy
	effectiveBuySpread := buySpread
	if balance.Cmp(decimal.Half) < 0 && balance.IsPositive() {
		// at balance=0.5, buySpread is doubled; at balance=0.25, quadrupled
		effectiveBuySpread = buySpread.Div(balance.MulInt(2))
	}

	if deviation.Neg().Cmp(effectiveBuySpread) > 0 && disposition == Cheap && balanceOK && underHardCap {
		if *flagVerbose {
			log.Printf("[logic] spread signal BUY spread=%sbps baseline=%sbps dev=%sbps coinbase=$%s binance=$%s",
				spread.BPS().Format(2),
				baseline.BPS().Format(2),
				deviation.BPS().Format(2),
				gCoinbasePrice,
				gBinancePrice)
		}
		executeTrade(now, ds.SideBuy, deviation)
		return
	}

	// passive market making: place resting orders when not taking
	updatePassiveOrders(now, disposition, greed, balance)
}

func updatePassiveOrders(_ clocky.Time, disposition Disposition, greed, balance decimal.Decimal) {
	gPassiveLock.Lock()
	defer gPassiveLock.Unlock()

	// get current inventory state
	gHolding.Lock.RLock()
	inventoryQty := gHolding.Quantity
	gHolding.Lock.RUnlock()
	inventoryValue := inventoryQty.Mul(gCoinbasePrice)

	// calculate desired passive order prices
	gCoinbasePair.Lock.RLock()
	baseIncrement := gCoinbasePair.BaseIncrement
	quoteIncrement := gCoinbasePair.QuoteIncrement
	gCoinbasePair.Lock.RUnlock()

	midPrice := gCoinbasePair.OrderBook.MidPrice()
	if !midPrice.IsPositive() {
		return
	}

	// base spread from Avellaneda-Stoikov optimal spread (1/κ)
	// high κ = tight market = tighter spreads = more fills
	// low κ = dispersed = wider spreads = fewer but safer fills
	baseSpread := *flagPassive
	gIntensityLock.Lock()
	if gIntensity != nil && gIntensity.IsReady() {
		optimalSpread := gIntensity.OptimalSpread()
		// blend flagPassive with optimal spread (use optimal as floor)
		// this ensures we're at least as wide as market conditions suggest
		baseSpread = baseSpread.Max(optimalSpread)
	}
	gIntensityLock.Unlock()

	// calculate passive spread, adjusted by greed
	// greed > 1: overweight, widen bid spread, tighten ask spread
	// greed < 1: underweight, tighten bid spread, widen ask spread
	bidSpread := baseSpread.Mul(greed)
	askSpread := baseSpread.Div(greed)
	gAskDelta = askSpread

	// inventory imbalance defense: when we're accumulating (balance < 0.5),
	// exponentially widen bid spread to slow buying
	// balance = min(buys,sells)/max(buys,sells), so 0.5 means 2:1 ratio
	minBalance := decimal.Parse("0.1") // floor to prevent division by tiny numbers
	if balance.Cmp(decimal.Half) < 0 && balance.IsPositive() {
		// defenseMultiplier = 1/balance², so at balance=0.5 → 4x, balance=0.1 → 100x
		safeBalance := balance.Max(minBalance)
		defenseMultiplier := decimal.One.Div(safeBalance.Sqr())
		bidSpread = bidSpread.Mul(defenseMultiplier)
	}
	gBidDelta = bidSpread

	// desired passive order prices
	desiredBidPrice := midPrice.Mul(decimal.One.Sub(bidSpread)).QuantizeFloor(quoteIncrement)
	desiredAskPrice := midPrice.Mul(decimal.One.Add(askSpread)).QuantizeCeil(quoteIncrement)

	// quantity for passive orders
	// scale down bid size when balance is poor (accumulating)
	bidSize := *flagSize
	if balance.Cmp(decimal.Half) < 0 && balance.IsPositive() {
		// at balance=0.5, bid size is halved; at balance=0.1, 1/5th
		safeBalance := balance.Max(minBalance)
		bidSize = bidSize.Mul(safeBalance.MulInt(2))
	}
	passiveBidQty := bidSize.Div(midPrice).QuantizeNearest(baseIncrement)
	passiveAskQty := (*flagSize).Div(midPrice).QuantizeNearest(baseIncrement)

	// determine what passive orders we should have
	// DEFENSE 1: hard cap at 2x target - never bid above this
	hardCap := (*flagTarget).MulInt(2)
	// DEFENSE 2: if balance < 0.33 (3:1 buy/sell ratio), stop bidding entirely
	oneThird := decimal.One.DivInt(3)
	balanceOK := balance.Cmp(oneThird) >= 0 || balance.IsZero() // zero balance = no trades yet = OK

	// minimum viable bid quantity (don't place tiny orders)
	minQty := (*flagSize).DivInt(4).Div(midPrice).QuantizeNearest(baseIncrement)

	// place bids when: underweight AND balance is OK AND not at extremes AND under hard cap AND sufficient size
	wantBid := disposition >= Cheap && disposition <= Normal &&
		inventoryValue.Cmp(*flagTarget) < 0 &&
		inventoryValue.Cmp(hardCap) < 0 &&
		balanceOK &&
		passiveBidQty.Cmp(minQty) >= 0

	// place asks when: overweight OR balance is poor (need to sell)
	// be more aggressive about asking when we're accumulating
	wantAsk := (disposition >= Normal && disposition <= Expensive && inventoryValue.Cmp(*flagTarget) > 0) ||
		(balance.Cmp(decimal.Half) < 0 && balance.IsPositive() && inventoryValue.IsPositive())

	// handle passive bid
	if gPassiveBid != nil {
		// check if existing bid is still valid
		gPassiveBid.Lock.RLock()
		bidState := gPassiveBid.State
		bidPrice := gPassiveBid.LimitPrice
		gPassiveBid.Lock.RUnlock()

		if bidState.IsFinal() {
			// order completed (filled/canceled), clear it
			gPassiveBid = nil
		} else if !wantBid {
			// no longer want a bid, cancel it
			if *flagVerbose {
				log.Printf("[passive] cancel bid: disposition=%s inv=$%s", disposition, inventoryValue.Format(0))
			}
			err := gPassiveBid.Cancel()
			if err != nil {
				log.Printf("[passive] cancel bid error: %v", err)
			} else {
				gPassiveBid = nil
			}
		} else {
			// check if price moved enough to warrant relisting
			priceDiff := desiredBidPrice.Sub(bidPrice).Abs().Div(bidPrice)
			if priceDiff.Cmp(*flagRelist) > 0 {
				if *flagVerbose {
					log.Printf("[passive] relist bid: old=$%s new=$%s diff=%sbps",
						bidPrice.Format(2), desiredBidPrice.Format(2), priceDiff.BPS().Format(2))
				}
				err := gPassiveBid.Cancel()
				if err != nil {
					log.Printf("[passive] relist bid cancel error: %v", err)
				} else {
					gPassiveBid = nil
				}
			}
		}
	}

	// place new bid if needed
	if wantBid && gPassiveBid == nil {
		order, err := gCoinbasePair.LimitOrder(ds.SideBuy, passiveBidQty, desiredBidPrice, ds.OrderStrategyPostOnly)
		if err != nil {
			if *flagVerbose && err != ds.ErrPostOnly && err != ds.ErrSelfTrade {
				log.Printf("[passive] bid failed: %v", err)
			}
		} else {
			gPassiveBid = order
			if *flagVerbose {
				log.Printf("[passive] placed bid %s @ $%s", passiveBidQty, desiredBidPrice)
			}
		}
	}

	// handle passive ask
	if gPassiveAsk != nil {
		// check if existing ask is still valid
		gPassiveAsk.Lock.RLock()
		askState := gPassiveAsk.State
		askPrice := gPassiveAsk.LimitPrice
		gPassiveAsk.Lock.RUnlock()

		if askState.IsFinal() {
			// order completed (filled/canceled), clear it
			gPassiveAsk = nil
		} else if !wantAsk {
			// no longer want an ask, cancel it
			if *flagVerbose {
				log.Printf("[passive] cancel ask: disposition=%s inv=$%s", disposition, inventoryValue.Format(0))
			}
			gPassiveAsk.Cancel()
			gPassiveAsk = nil
		} else {
			// check if price moved enough to warrant relisting
			priceDiff := desiredAskPrice.Sub(askPrice).Abs().Div(askPrice)
			if priceDiff.Cmp(*flagRelist) > 0 {
				if *flagVerbose {
					log.Printf("[passive] relist ask: old=$%s new=$%s diff=%sbps",
						askPrice.Format(2), desiredAskPrice.Format(2), priceDiff.BPS().Format(2))
				}
				gPassiveAsk.Cancel()
				gPassiveAsk = nil
			}
		}
	}

	// place new ask if needed
	if wantAsk && gPassiveAsk == nil {
		order, err := gCoinbasePair.LimitOrder(ds.SideSell, passiveAskQty, desiredAskPrice, ds.OrderStrategyPostOnly)
		if err != nil {
			if *flagVerbose && err != ds.ErrPostOnly && err != ds.ErrSelfTrade {
				log.Printf("[passive] ask failed: %v", err)
			}
		} else {
			gPassiveAsk = order
			if *flagVerbose {
				log.Printf("[passive] placed ask %s @ $%s", passiveAskQty, desiredAskPrice)
			}
		}
	}
}

func executeTrade(now clocky.Time, side ds.Side, spread decimal.Decimal) {

	// get pair info
	gCoinbasePair.Lock.RLock()
	lastPrice := gCoinbasePair.LastPrice
	baseIncrement := gCoinbasePair.BaseIncrement
	gCoinbasePair.Lock.RUnlock()

	// calculate quantity
	quantity := (*flagSize).Div(lastPrice).QuantizeNearest(baseIncrement)
	quote, ok := getQuote(side, quantity)
	if !ok {
		return
	}

	// buy gap protection - only buy if price dropped enough from last buy
	// this prevents accumulating lots at similar prices (ratchet effect)
	// protection decays over time after sells, faster when under target
	// gap INCREASES with inventory to slow down accumulation rate
	if side == ds.SideBuy && !(*flagBuyGap).IsZero() {
		gHolding.Lock.RLock()
		topCost := gHolding.Lots.PeekTopCostPerUnit()
		inventoryQty := gHolding.Quantity
		gHolding.Lock.RUnlock()
		if !topCost.IsZero() {
			// use actual price we'd pay from order book, not stale lastPrice
			buyPrice := quote.Div(quantity)
			invested := inventoryQty.Mul(buyPrice)

			// scale buygap up with inventory: at 100% target, buygap doubles
			// this slows accumulation rate as position grows
			inventoryScale := decimal.One
			if (*flagTarget).IsPositive() {
				invRatio := invested.Div(*flagTarget)
				inventoryScale = decimal.One.Add(invRatio) // 1 + inv/target
			}

			// calculate decay factor based on time since last sell
			// decay period = basePeriod * e^(inventoryRatio * 2)
			decayFactor := decimal.One
			if (*flagTarget).IsPositive() && gLastSellTime > 0 {
				invRatio := invested.Div(*flagTarget)
				timeSinceSell := now - gLastSellTime
				if timeSinceSell > 0 {
					timeRatio := decimal.FromInt(int(timeSinceSell)).Div(decimal.FromInt(int(*flagBuyDecay)))
					periodScale := invRatio.MulInt(2).Neg().Exp()                // e^(-invRatio*2)
					decayFactor = timeRatio.Mul(periodScale).Neg().Exp()         // e^(-timeRatio*periodScale)
					decayFactor = decayFactor.Max(decimal.Zero).Min(decimal.One) // clamp [0,1]
				}
			}

			// effective buygap = base * inventoryScale * decayFactor
			effectiveBuygap := (*flagBuyGap).Mul(inventoryScale).Mul(decayFactor)
			if effectiveBuygap.BPS().Cmp(decimal.Tenth) > 0 {
				maxBuyPrice := topCost.Mul(decimal.One.Sub(effectiveBuygap))
				if buyPrice.Cmp(maxBuyPrice) > 0 {
					if *flagVerbose {
						gap := topCost.Sub(buyPrice).Div(topCost)
						log.Printf("[logic] skip buy: price $%s not %sbps below last buy $%s (gap=%sbps scale=%s decay=%s%% inv=$%s)",
							buyPrice,
							effectiveBuygap.BPS().Format(1),
							topCost,
							gap.BPS().Format(2),
							inventoryScale.Format(2),
							decayFactor.MulInt(100).Format(0),
							invested)
						gLastActivity = now
					}
					return
				}
			}
		}
	}

	// cost basis protection for sells
	rawCostBasis := decimal.Zero // saved for win/loss tracking after fill
	if side == ds.SideSell {
		gHolding.Lock.RLock()
		costBasis := gHolding.Lots.GetCostBasis(quantity, gBinancePrice)
		gHolding.Lock.RUnlock()
		rawCostBasis = costBasis // save before adding profit margin
		costBasis = costBasis.Mul(decimal.One.Add(*flagProfit))
		// would this sell be profitable?
		if quote.Cmp(costBasis) < 0 {
			// selling at a loss - only allow if panic threshold exceeded
			if spread.Abs().Cmp(*flagPanic) < 0 {
				if *flagVerbose {
					loss := costBasis.Sub(quote)
					log.Printf("[logic] skip sell: would lose $%s (cost=$%s quote=$%s spread=%sbps < panic=%sbps)",
						loss, costBasis, quote, spread.Abs().BPS().Format(2), (*flagPanic).BPS())
					gLastActivity = now
				}
				return
			}
			log.Printf("[logic] panic sell: spread=%sbps >= panic=%sbps",
				spread.Abs().BPS().Format(2), (*flagPanic).BPS())
		}
	}

	// don't place trades too quickly
	gTradeLock.Lock()
	if now.Sub(gLastTrade) < *flagCooldown {
		gTradeLock.Unlock()
		return
	}
	gLastTrade = now
	gTradeLock.Unlock()

	// place IOC limit order to take liquidity with price protection
	t0 := now
	t1 := clocky.Now()
	gCoinbasePair.Lock.RLock()
	quoteIncrement := gCoinbasePair.QuoteIncrement
	gCoinbasePair.Lock.RUnlock()
	// set limit price at expected fill price (protects against adverse moves)
	limitPrice := quote.Div(quantity).QuantizeNearest(quoteIncrement)
	order, err := gCoinbasePair.LimitOrder(side, quantity, limitPrice, ds.OrderStrategyIOC)
	t2 := clocky.Now()
	if err != nil {
		if err == ds.ErrSelfTrade {
			if *flagVerbose {
				log.Printf("[order] skipped %s: would self-trade", side)
			}
		} else {
			log.Printf("[error] failed to place %s order: %v", side, err)
		}
		return
	}

	// wait for order completion
	order.Wait()
	t3 := clocky.Now()
	if order.State != ds.OrderStateFilled && order.State != ds.OrderStatePartiallyFilled {
		if *flagVerbose {
			log.Printf("[order] %s not filled: state=%s", side, order.State)
		}
		return
	}
	// use actual filled quantity for reporting
	quantity = order.Filled
	if teddy.Live {
		log.Printf("[perf] decided in %s ordered in %s acknowledged in %s (total %s)",
			t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), t3.Sub(t0))
	}

	// report trade
	value := order.Notional
	slippage := order.Price.Sub(lastPrice).Div(lastPrice)
	var theft decimal.Decimal
	if side == ds.SideBuy {
		theft = value.Div(quote).Sub(decimal.One) // positive = paid more = robbed
	} else {
		theft = decimal.One.Sub(value.Div(quote)) // positive = received less = robbed
	}
	log.Printf("[trade] %s $%s @ last=$%s quote=$%s fill=$%s slip=%sbps theft=%sbps",
		side, value, lastPrice, quote, order.Price, slippage.BPS().Format(2), theft.BPS().Format(2))
	if side == ds.SideSell {
		gLastSellTime = clocky.Now()
		// track win/loss for this trade
		gHolding.Lock.Lock()
		if value.Cmp(rawCostBasis) >= 0 {
			gHolding.WinCount++
		} else {
			gHolding.LossCount++
		}
		gHolding.Lock.Unlock()
	}
}

func getQuote(side ds.Side, size decimal.Decimal) (decimal.Decimal, bool) {
	switch side {
	case ds.SideBuy:
		return gCoinbasePair.OrderBook.MarketBuyCost(size)
	case ds.SideSell:
		return gCoinbasePair.OrderBook.MarketSellProceeds(size)
	default:
		return decimal.Zero, false
	}
}

func shouldLogStatus(now clocky.Time) bool {
	gStatusLock.Lock()
	defer gStatusLock.Unlock()
	if now.Sub(gLastStatus) < clocky.Minute {
		return false
	}
	if !teddy.Live && !*flagVerbose {
		return false
	}
	gLastStatus = now
	return true
}

// logStatus prints indicator status every minute
// Shows raw values with '?' suffix if indicator not ready
func logStatus() {

	// intensity ready?
	var alpha, kappa decimal.Decimal
	var intensityReady bool
	gIntensityLock.Lock()
	if gIntensity != nil {
		intensityReady = gIntensity.IsReady()
		alpha = gIntensity.Alpha
		kappa = gIntensity.Kappa
	}
	gIntensityLock.Unlock()

	// spread ready?
	gSpreadLock.Lock()
	spreadReady := gSpreadEMA.IsReady()
	beta := gSpreadEMA.Value
	gSpreadLock.Unlock()

	// range ready?
	gPriceLock.Lock()
	rangeReady := gCoinbaseMin.IsPositive() && gCoinbaseMax.IsPositive()
	gPriceLock.Unlock()

	// format with '?' suffix if not ready
	q := func(ready bool) string {
		if ready {
			return ""
		}
		return "?"
	}

	log.Printf("[status] α=%s%s κ=%s%s β=%s%sbps δ=%sbps μ=%s ρ=%s%% q=%s%% r=%s%s%% bδ=%sbps sδ=%sbps $%s",
		alpha.Format(0), q(intensityReady),
		kappa.Format(0), q(intensityReady),
		beta.BPS().Format(2), q(spreadReady),
		gDeviation.BPS().Format(2),
		gGreed.Format(2),
		gBalance.MulInt(100).Format(0),
		gInvRatio.MulInt(100).Format(0),
		gRangePos.MulInt(100).Format(0), q(rangeReady),
		gBidDelta.BPS().Format(2),
		gAskDelta.BPS().Format(2),
		gCoinbasePrice.Format(2))
}
