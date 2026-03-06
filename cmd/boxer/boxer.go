package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"sort"
)

var (
	boxes       []*Box
	lastBoxTime clocky.Time
)

func boxer() {

	// make boxes one at a time
	if len(boxes) > 0 {
		return
	}

	// cooldown between boxes
	if clocky.Now().Sub(lastBoxTime) < 15*clocky.Second {
		return
	}

	// group options by strike into call/put pairs
	strikes := make(map[decimal.Decimal]*Strike)
	for _, opt := range optionsByID {
		spread := opt.Ask.Sub(opt.Bid)
		if spread.Cmp(*minSpread) < 0 {
			continue
		}
		sp, ok := strikes[opt.Strike]
		if !ok {
			sp = &Strike{}
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
	var best *Box
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

			bs := &Box{
				low:      low,
				high:     high,
				width:    width,
				callLow:  spLow.call,
				callHigh: spHigh.call,
				putLow:   spLow.put,
				putHigh:  spHigh.put,
				mid:      boxMid,
			}

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
				best = bs
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

	log.Printf("POUNCING on %s box %s/%s for $%s profit", side,
		best.low.Format(0), best.high.Format(0), dollars.Format(2))

	if best.buying {
		best.legs = []*Leg{
			NewLeg(best.callLow, schwab.InstructionBuyToOpen),
			NewLeg(best.callHigh, schwab.InstructionSellToOpen),
			NewLeg(best.putHigh, schwab.InstructionBuyToOpen),
			NewLeg(best.putLow, schwab.InstructionSellToOpen),
		}
	} else {
		best.legs = []*Leg{
			NewLeg(best.callLow, schwab.InstructionSellToOpen),
			NewLeg(best.callHigh, schwab.InstructionBuyToOpen),
			NewLeg(best.putHigh, schwab.InstructionSellToOpen),
			NewLeg(best.putLow, schwab.InstructionBuyToOpen),
		}
	}

	// start manufacturing box
	lastBoxTime = clocky.Now()
	boxes = append(boxes, best)
	best.Order()
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
