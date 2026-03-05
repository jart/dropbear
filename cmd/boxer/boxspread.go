package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
	"log"
	"sort"
)

var tick = decimal.Parse("0.05")

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

			// box midpoint = net cost to buy the box at midpoint
			// Buy C(low) + Sell C(high) + Buy P(high) + Sell P(low)
			boxMid := midCL.Sub(midCH).Add(midPH).Sub(midPL)

			// evaluate buying: round price down, profit = width - price
			buyPrice := boxMid.QuantizeTruncate(tick)
			buyProfit := width.Sub(buyPrice)
			buyEdge := boxMid.Sub(buyPrice)

			// evaluate selling: round price up, profit = price - width
			sellPrice := boxMid.QuantizeAway(tick)
			sellProfit := sellPrice.Sub(width)
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
	dollars := best.profit.MulInt(int(*multFlag))
	log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/contract)",
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
}

func logLeg(action, class string, opt *Option) {
	mid := opt.Bid.Add(opt.Ask).DivInt(2)
	log.Printf("  %s %s %s mid=%s bid=%s ask=%s",
		action, class, opt.Strike.Format(0), mid.Format(2), opt.Bid.Format(2), opt.Ask.Format(2))
}
