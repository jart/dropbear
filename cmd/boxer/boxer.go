package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"fmt"
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
	if clocky.Now().Sub(lastBoxTime) < 10*clocky.Second {
		return
	}

	// ES midpoint for drift logging
	var esMid decimal.Decimal
	if es != nil {
		esMid = es.Price
	}

	// group options by strike into call/put pairs
	strikes := make(map[decimal.Decimal]*Strike)
	for _, opt := range optionsByID {
		if opt.Bid.IsZero() {
			continue
		}
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

			// compute per-leg prices based on quote freshness
			// fresh (<150ms, ES drift <= 0.25): demand midpoint
			// stale: cross the spread (buy at ask, sell at bid)
			buyCL := legPrice(spLow.call, true, esMid)
			buyCH := legPrice(spHigh.call, true, esMid)
			buyPL := legPrice(spLow.put, true, esMid)
			buyPH := legPrice(spHigh.put, true, esMid)
			sellCL := legPrice(spLow.call, false, esMid)
			sellCH := legPrice(spHigh.call, false, esMid)
			sellPL := legPrice(spLow.put, false, esMid)
			sellPH := legPrice(spHigh.put, false, esMid)

			// midpoints (for edge reference)
			midCL := spLow.call.Bid.Add(spLow.call.Ask).DivInt(2)
			midCH := spHigh.call.Bid.Add(spHigh.call.Ask).DivInt(2)
			midPL := spLow.put.Bid.Add(spLow.put.Ask).DivInt(2)
			midPH := spHigh.put.Bid.Add(spHigh.put.Ask).DivInt(2)

			// Buy box: Buy C(low) + Sell C(high) + Buy P(high) + Sell P(low)
			buyPrice := buyCL.Sub(sellCH).Add(buyPH).Sub(sellPL)
			buyProfit := width.Sub(buyPrice)

			// Sell box: Sell C(low) + Buy C(high) + Sell P(high) + Buy P(low)
			sellPrice := sellCL.Sub(buyCH).Add(sellPH).Sub(buyPL)
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

	if best.buying {
		log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/box)\n%s\n%s\n%s\n%s",
			side, best.low.Format(0), best.high.Format(0), best.width.Format(0),
			best.price.Format(2), best.profit.Format(2), best.edge.Format(2), dollars.Format(2),
			formatLeg(1, "BUY ", "C", best.callLow, esMid),
			formatLeg(2, "SELL", "C", best.callHigh, esMid),
			formatLeg(3, "BUY ", "P", best.putHigh, esMid),
			formatLeg(4, "SELL", "P", best.putLow, esMid))
	} else {
		log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/box)\n%s\n%s\n%s\n%s",
			side, best.low.Format(0), best.high.Format(0), best.width.Format(0),
			best.price.Format(2), best.profit.Format(2), best.edge.Format(2), dollars.Format(2),
			formatLeg(1, "SELL", "C", best.callLow, esMid),
			formatLeg(2, "BUY ", "C", best.callHigh, esMid),
			formatLeg(3, "SELL", "P", best.putHigh, esMid),
			formatLeg(4, "BUY ", "P", best.putLow, esMid))
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
			NewLeg("#1", best.callLow, schwab.InstructionBuyToOpen, legPrice(best.callLow, true, esMid)),
			NewLeg("#2", best.callHigh, schwab.InstructionSellToOpen, legPrice(best.callHigh, false, esMid)),
			NewLeg("#3", best.putHigh, schwab.InstructionBuyToOpen, legPrice(best.putHigh, true, esMid)),
			NewLeg("#4", best.putLow, schwab.InstructionSellToOpen, legPrice(best.putLow, false, esMid)),
		}
	} else {
		best.legs = []*Leg{
			NewLeg("#1", best.callLow, schwab.InstructionSellToOpen, legPrice(best.callLow, false, esMid)),
			NewLeg("#2", best.callHigh, schwab.InstructionBuyToOpen, legPrice(best.callHigh, true, esMid)),
			NewLeg("#3", best.putHigh, schwab.InstructionSellToOpen, legPrice(best.putHigh, false, esMid)),
			NewLeg("#4", best.putLow, schwab.InstructionBuyToOpen, legPrice(best.putLow, true, esMid)),
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

func formatLeg(num int, action, class string, opt *Option, esMid decimal.Decimal) string {
	mid := opt.Bid.Add(opt.Ask).DivInt(2)
	drift := esMid.Sub(opt.ES)
	fresh := "STALE"
	if isFresh(opt, esMid) {
		fresh = "fresh"
	}
	return fmt.Sprintf("  #%d %s %s %s mid=%s bid=%s ask=%s ES%+.2f age=%s [%s]",
		num, action, class, opt.Strike.Format(0), mid.Format(2), opt.Bid.Format(2),
		opt.Ask.Format(2), drift.Float64(), clocky.Since(opt.TS), fresh)
}
