//    o               o
//              , _|_     _  _    _     , _|_  ,_    _   _ _|_
//    | |   |  / \_|  |  / |/ |  |/    / \_|  /  |  |/  |/  |
//    |/ \_/|_/ \/ |_/|_/  |  |_/|__/   \/ |_/   |_/|__/|__/|_/
//   /|
//   \|         market taking algorithm x3.159-2025

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
	flagSize      = decimal.Flag("size", "200", "order size in usd")
	flagUSD       = decimal.Flag("usd", "20000", "coinbase usd balance")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase product to trade")
	flagTarget    = decimal.Flag("target", "5000", "target inventory in usd")
	flagSpread    = decimal.FlagBPS("spread", "3", "spread threshold in basis points")
	flagSpreadMin = decimal.FlagBPS("spread-min", "0.5", "minimum spread threshold in basis points")
	flagSpreadMax = decimal.FlagBPS("spread-max", "10", "maximum spread threshold in basis points")
	flagProfit    = decimal.FlagBPS("profit", "5", "profit threshold in basis points")
	flagPanic     = decimal.FlagBPS("panic", "15", "panic threshold to sell at a loss")
	flagBuyGap    = decimal.FlagBPS("buygap", "5", "only buy if price is this many basis points below last buy")
	flagBuyDecay  = clocky.DurationFlag("decay", "30s", "base decay period for buygap after sells")
	flagSkew      = decimal.Flag("skew", "1", "spread adjustment per 100% inventory imbalance")
	flagWindow    = clocky.DurationFlag("window", "42m", "time window for min/max range protection (0 disables)")
	flagComfort   = decimal.FlagPercent("comfort", "20", "percent of min/max window we're comfortable buying or selling")
	flagDanger    = decimal.FlagPercent("danger", "0", "percent of min/max window at which it's probably volatility expansion")
	flagSamples   = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagCooldown  = clocky.DurationFlag("cooldown", "5s", "duration to wait between activities")
	flagFreshness = clocky.DurationFlag("freshness", "1500ms", "suspend trading after this long an outage")
	flagIntensity = clocky.DurationFlag("intensity", "2h", "trading intensity window (e.g. 5m, 0 for disabled)")
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
	gBinancePrice  decimal.Decimal
	gCoinbaseMin   decimal.Decimal
	gCoinbaseMax   decimal.Decimal
	gCoinbasePrice decimal.Decimal
	gTradeLock     sync.Mutex
	gIntensityLock sync.Mutex
	gSpreadLock    sync.Mutex
	gPriceLock     sync.Mutex
	gIntensity     *indicators.Intensity
	gSpreadEMA     *indicators.WWMA
	gPriceMin      *indicators.Min
	gPriceMax      *indicators.Max
	gFirstEvent    clocky.Time
	gWarmedUp      bool
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

	gCoinbase = teddy.Brokers.Get(ds.BrokerCoinbase)
	gHolding = gCoinbase.Holdings.Get(*flagSymbol)
	gCoinbasePair = gCoinbase.Pairs.Get(*flagSymbol + "-USD")
	gCoinbasePair.OnTick = onCoinbaseTick

	gBinance = teddy.Brokers.Get(ds.BrokerBinance)
	if *flagSymbol == "ZEC" {
		gBinancePair = gBinance.Pairs.Get(*flagSymbol + "USDT")
	} else {
		gBinancePair = gBinance.Pairs.Get(*flagSymbol + "FDUSD")
	}
	gBinancePair.OnTick = onBinanceTick

	if teddy.Live {
		client := teddy.CoinbaseClient
		symbol := gCoinbasePair.Symbol
		granularity := coinbase.CandleGranularityMinute
		if candles, err := client.GetCandles(symbol, granularity, 0, 0, 0); err == nil {
			for _, c := range candles {
				gPriceMin.Add(c.Start, c.Low)
				gPriceMax.Add(c.Start, c.High)
			}
		}
	}

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
	isReady := gSpreadEMA.IsReady()
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
	if effectiveTarget.IsPositive() {
		inventoryValue := inventoryQty.Mul(gCoinbasePrice)
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
	if deviation.Cmp(sellSpread) > 0 && disposition == Expensive {
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
	if deviation.Neg().Cmp(buySpread) > 0 && disposition == Cheap {
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
				timeSinceSell := now.Sub(gLastSellTime)
				if timeSinceSell > 0 {
					timeRatio := decimal.FromFloat64(float64(timeSinceSell) / float64(*flagBuyDecay))
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

	// place order
	t0 := now
	t1 := clocky.Now()
	order, err := gCoinbasePair.MarketOrder(side, quantity)
	t2 := clocky.Now()
	if err != nil {
		log.Printf("[error] failed to place order: %v", err)
		return
	}

	// wait for order completion
	order.Wait()
	t3 := clocky.Now()
	if order.State != ds.OrderStateFilled {
		log.Printf("[error] order not filled: state=%s", order.State)
		return
	}
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
