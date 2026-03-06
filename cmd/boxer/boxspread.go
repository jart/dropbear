package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"log"
	"sort"
	"sync"
)

var (
	tick05  = decimal.Parse("0.05")
	tick10  = decimal.Parse("0.10")
	three   = decimal.FromInt(3)
	traded  bool
	tradeMu sync.Mutex
)

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(three) < 0 {
		return tick05
	}
	return tick10
}

type strikePair struct {
	call *Option
	put  *Option
}

type boxSpread struct {
	low      decimal.Decimal
	high     decimal.Decimal
	width    decimal.Decimal
	callLow  *Option
	callHigh *Option
	putLow   *Option
	putHigh  *Option
	mid      decimal.Decimal // unrounded box midpoint price
	price    decimal.Decimal // rounded limit price (what we'd pay or receive)
	profit   decimal.Decimal // guaranteed profit per point at expiration
	edge     decimal.Decimal // how much better than midpoint we're demanding
	buying   bool
}

func makeDecisions() {
	if traded {
		return
	}

	// group options by strike into call/put pairs
	minSpread := decimal.Parse("0.10")
	strikes := make(map[decimal.Decimal]*strikePair)
	for _, opt := range optionsByID {
		spread := opt.Ask.Sub(opt.Bid)
		if spread.Cmp(minSpread) < 0 {
			continue
		}
		sp, ok := strikes[opt.Strike]
		if !ok {
			sp = &strikePair{}
			strikes[opt.Strike] = sp
		}
		switch opt.Class {
		case databento.InstrumentClassCall:
			sp.call = opt
		case databento.InstrumentClassPut:
			sp.put = opt
		}
	}

	// collect strikes that have both a call and a put
	var valid []decimal.Decimal
	for strike, sp := range strikes {
		if sp.call != nil && sp.put != nil {
			valid = append(valid, strike)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Cmp(valid[j]) < 0
	})

	// evaluate all box spread combinations
	var best *boxSpread
	for i := 0; i < len(valid); i++ {
		for j := i + 1; j < len(valid); j++ {
			low := valid[i]
			high := valid[j]
			width := high.Sub(low)
			if width.Cmp(*widthFlag) > 0 {
				break // strikes are sorted, so all subsequent j values are wider
			}

			spLow := strikes[low]
			spHigh := strikes[high]

			// midpoints
			midCL := spLow.call.Bid.Add(spLow.call.Ask).DivInt(2)
			midCH := spHigh.call.Bid.Add(spHigh.call.Ask).DivInt(2)
			midPL := spLow.put.Bid.Add(spLow.put.Ask).DivInt(2)
			midPH := spHigh.put.Bid.Add(spHigh.put.Ask).DivInt(2)

			// round each leg's midpoint to its tick, then compute box price
			// buying: buy at truncated mid (pay less), sell at truncated mid (receive less)
			// selling: buy at away mid (pay more), sell at away mid (receive more)
			buyCL := midCL.QuantizeTruncate(optionTick(midCL))
			buyCH := midCH.QuantizeTruncate(optionTick(midCH))
			buyPL := midPL.QuantizeTruncate(optionTick(midPL))
			buyPH := midPH.QuantizeTruncate(optionTick(midPH))
			sellCL := midCL.QuantizeAway(optionTick(midCL))
			sellCH := midCH.QuantizeAway(optionTick(midCH))
			sellPL := midPL.QuantizeAway(optionTick(midPL))
			sellPH := midPH.QuantizeAway(optionTick(midPH))

			// Buy box: Buy C(low) + Sell C(high) + Buy P(high) + Sell P(low)
			buyPrice := buyCL.Sub(buyCH).Add(buyPH).Sub(buyPL)
			buyProfit := width.Sub(buyPrice)

			// Sell box: Sell C(low) + Buy C(high) + Sell P(high) + Buy P(low)
			sellPrice := sellCL.Sub(sellCH).Add(sellPH).Sub(sellPL)
			sellProfit := sellPrice.Sub(width)

			boxMid := midCL.Sub(midCH).Add(midPH).Sub(midPL)
			buyEdge := boxMid.Sub(buyPrice)
			sellEdge := sellPrice.Sub(boxMid)

			var bs boxSpread
			bs.low = low
			bs.high = high
			bs.width = width
			bs.callLow = spLow.call
			bs.callHigh = spHigh.call
			bs.putLow = spLow.put
			bs.putHigh = spHigh.put
			bs.mid = boxMid

			if buyProfit.Cmp(sellProfit) >= 0 {
				bs.buying = true
				bs.price = buyPrice
				bs.profit = buyProfit
				bs.edge = buyEdge
			} else {
				bs.buying = false
				bs.price = sellPrice
				bs.profit = sellProfit
				bs.edge = sellEdge
			}

			// skip boxes that would clobber existing positions
			if bs.buying {
				if !canOpen(spLow.call, true) || !canOpen(spHigh.call, false) ||
					!canOpen(spHigh.put, true) || !canOpen(spLow.put, false) {
					continue
				}
			} else {
				if !canOpen(spLow.call, false) || !canOpen(spHigh.call, true) ||
					!canOpen(spHigh.put, false) || !canOpen(spLow.put, true) {
					continue
				}
			}

			if bs.profit.IsPositive() && (best == nil || bs.profit.Cmp(best.profit) > 0) {
				clone := bs
				best = &clone
			}
		}
	}

	if best == nil {
		return
	}

	side := "BUY"
	if !best.buying {
		side = "SELL"
	}
	dollars := best.profit.MulInt(100)
	if dollars.Cmp(*demandFlag) < 0 {
		return
	}

	log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/box)",
		side, best.low.Format(0), best.high.Format(0), best.width.Format(0),
		best.price.Format(2), best.profit.Format(2), best.edge.Format(2),
		dollars.Format(2))

	if best.buying {
		logLeg("BUY ", "C", best.callLow)
		logLeg("SELL", "C", best.callHigh)
		logLeg("BUY ", "P", best.putHigh)
		logLeg("SELL", "P", best.putLow)
	} else {
		logLeg("SELL", "C", best.callLow)
		logLeg("BUY ", "C", best.callHigh)
		logLeg("SELL", "P", best.putHigh)
		logLeg("BUY ", "P", best.putLow)
	}

	if *dry {
		log.Printf("DRY RUN: would pounce on %s box %s/%s for $%s profit",
			side, best.low.Format(0), best.high.Format(0), dollars.Format(2))
		return
	}
	tradeMu.Lock()
	if traded {
		tradeMu.Unlock()
		return
	}
	traded = true
	tradeMu.Unlock()

	log.Printf("POUNCING on %s box %s/%s for $%s profit", side,
		best.low.Format(0), best.high.Format(0), dollars.Format(2))

	type leg struct {
		opt         *Option
		instruction schwab.Instruction
	}
	var legs []leg
	if best.buying {
		legs = []leg{
			{best.callLow, schwab.InstructionBuyToOpen},
			{best.callHigh, schwab.InstructionSellToOpen},
			{best.putHigh, schwab.InstructionBuyToOpen},
			{best.putLow, schwab.InstructionSellToOpen},
		}
	} else {
		legs = []leg{
			{best.callLow, schwab.InstructionSellToOpen},
			{best.callHigh, schwab.InstructionBuyToOpen},
			{best.putHigh, schwab.InstructionSellToOpen},
			{best.putLow, schwab.InstructionBuyToOpen},
		}
	}

	var wg sync.WaitGroup
	for _, l := range legs {
		wg.Add(1)
		go func(l leg) {
			defer wg.Done()
			mid := l.opt.Bid.Add(l.opt.Ask).DivInt(2)
			t := optionTick(mid)
			var price decimal.Decimal
			if l.instruction == schwab.InstructionBuyToOpen {
				price = mid.QuantizeTruncate(t)
			} else {
				price = mid.QuantizeAway(t)
			}
			orderID, err := schwabClient.CreateOrder(&schwab.Order{
				OrderType:         schwab.OrderTypeLimit,
				Price:             price,
				Duration:          schwab.DurationDay,
				Session:           schwab.SessionNormal,
				OrderStrategyType: schwab.OrderStrategyTypeSingle,
				OrderLegCollection: []schwab.OrderLeg{
					{
						Instruction: l.instruction,
						Quantity:    decimal.FromInt(1),
						Instrument: schwab.Instrument{
							Symbol: l.opt.OSI(),
							Type:   schwab.AssetTypeOption,
						},
					},
				},
			})
			if err != nil {
				log.Printf("order FAILED %s %s %s @ %s: %v",
					l.instruction, l.opt.Class, l.opt.Strike.Format(0), price.Format(2), err)
				return
			}
			log.Printf("order SENT %s %s %s @ %s (id=%d)",
				l.instruction, l.opt.Class, l.opt.Strike.Format(0), price.Format(2), orderID)
		}(l)
	}
	wg.Wait()
}

// canOpen returns true if opening this leg won't reduce an existing position.
// Buying requires qty >= 0 (no existing short to clobber).
// Selling requires qty <= 0 (no existing long to clobber).
func canOpen(opt *Option, buying bool) bool {
	qty := holdings[opt.OSI()]
	if buying {
		return qty >= 0
	}
	return qty <= 0
}

func logLeg(action, class string, opt *Option) {
	mid := opt.Bid.Add(opt.Ask).DivInt(2)
	log.Printf("  %s %s %s mid=%s bid=%s ask=%s",
		action, class, opt.Strike.Format(0), mid.Format(2), opt.Bid.Format(2), opt.Ask.Format(2))
}
