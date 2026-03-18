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
			sp.Call = opt
		case databento.InstrumentClassPut:
			sp.Put = opt
		}
	}

	// collect strikes that have both a call and a put
	var valid []decimal.Decimal
	for strike, sp := range strikes {
		if sp.Call != nil && sp.Put != nil {
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
			buyCL := legPrice(spLow.Call, true, esMid)
			buyCH := legPrice(spHigh.Call, true, esMid)
			buyPL := legPrice(spLow.Put, true, esMid)
			buyPH := legPrice(spHigh.Put, true, esMid)
			sellCL := legPrice(spLow.Call, false, esMid)
			sellCH := legPrice(spHigh.Call, false, esMid)
			sellPL := legPrice(spLow.Put, false, esMid)
			sellPH := legPrice(spHigh.Put, false, esMid)

			// apply directional bias: positive = greed on bull legs,
			// negative = greed on bear legs
			// bull legs: buy call (long call), sell put (short put)
			// bear legs: sell call (short call), buy put (long put)
			bullBias := (*biasFlag).Max(decimal.Zero)
			bearBias := (*biasFlag).Neg().Max(decimal.Zero)
			buyCL = buyCL.Sub(bullBias)   // long call = bull
			buyCH = buyCH.Sub(bullBias)   // long call = bull
			sellPL = sellPL.Add(bullBias) // short put = bull
			sellPH = sellPH.Add(bullBias) // short put = bull
			sellCL = sellCL.Add(bearBias) // short call = bear
			sellCH = sellCH.Add(bearBias) // short call = bear
			buyPL = buyPL.Sub(bearBias)   // long put = bear
			buyPH = buyPH.Sub(bearBias)   // long put = bear

			// midpoints (for edge reference)
			midCL := spLow.Call.Bid.Add(spLow.Call.Ask).DivInt(2)
			midCH := spHigh.Call.Bid.Add(spHigh.Call.Ask).DivInt(2)
			midPL := spLow.Put.Bid.Add(spLow.Put.Ask).DivInt(2)
			midPH := spHigh.Put.Bid.Add(spHigh.Put.Ask).DivInt(2)

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
				Low:      low,
				High:     high,
				Width:    width,
				CallLow:  spLow.Call,
				CallHigh: spHigh.Call,
				PutLow:   spLow.Put,
				PutHigh:  spHigh.Put,
				Mid:      boxMid,
			}

			if buyProfit.Cmp(sellProfit) >= 0 {
				bs.Buying = true
				bs.Price = buyPrice
				bs.Profit = buyProfit
				bs.Edge = buyEdge
			} else {
				bs.Buying = false
				bs.Price = sellPrice
				bs.Profit = sellProfit
				bs.Edge = sellEdge
			}

			// skip boxes that would clobber existing positions
			if bs.Buying {
				if !canOpen(spLow.Call, true) || !canOpen(spHigh.Call, false) ||
					!canOpen(spHigh.Put, true) || !canOpen(spLow.Put, false) {
					continue
				}
			} else {
				if !canOpen(spLow.Call, false) || !canOpen(spHigh.Call, true) ||
					!canOpen(spHigh.Put, false) || !canOpen(spLow.Put, true) {
					continue
				}
			}

			if bs.Profit.IsPositive() && (best == nil || bs.Profit.Cmp(best.Profit) > 0) {
				best = bs
			}
		}
	}

	if best == nil {
		return
	}

	side := "BUY"
	if !best.Buying {
		side = "SELL"
	}
	dollars := best.Profit.MulInt(100)
	if dollars.Cmp(*demandFlag) < 0 {
		return
	}

	if best.Buying {
		log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/box)\n%s\n%s\n%s\n%s",
			side, best.Low.Format(0), best.High.Format(0), best.Width.Format(0),
			best.Price.Format(2), best.Profit.Format(2), best.Edge.Format(2), dollars.Format(2),
			formatLeg(1, "BUY ", "C", best.CallLow, esMid),
			formatLeg(2, "SELL", "C", best.CallHigh, esMid),
			formatLeg(3, "BUY ", "P", best.PutHigh, esMid),
			formatLeg(4, "SELL", "P", best.PutLow, esMid))
	} else {
		log.Printf("best box: %s %s/%s w=%s price=%s profit=%s edge=%s ($%s/box)\n%s\n%s\n%s\n%s",
			side, best.Low.Format(0), best.High.Format(0), best.Width.Format(0),
			best.Price.Format(2), best.Profit.Format(2), best.Edge.Format(2), dollars.Format(2),
			formatLeg(1, "SELL", "C", best.CallLow, esMid),
			formatLeg(2, "BUY ", "C", best.CallHigh, esMid),
			formatLeg(3, "SELL", "P", best.PutHigh, esMid),
			formatLeg(4, "BUY ", "P", best.PutLow, esMid))
	}

	if *dry {
		log.Printf("DRY RUN: would pounce on %s box %s/%s for $%s profit",
			side, best.Low.Format(0), best.High.Format(0), dollars.Format(2))
		return
	}

	log.Printf("POUNCING on %s box %s/%s for $%s profit", side,
		best.Low.Format(0), best.High.Format(0), dollars.Format(2))

	if best.Buying {
		best.Legs = []*Leg{
			NewLeg("#1", best.CallLow, schwab.InstructionBuyToOpen, legPrice(best.CallLow, true, esMid)),
			NewLeg("#2", best.CallHigh, schwab.InstructionSellToOpen, legPrice(best.CallHigh, false, esMid)),
			NewLeg("#3", best.PutHigh, schwab.InstructionBuyToOpen, legPrice(best.PutHigh, true, esMid)),
			NewLeg("#4", best.PutLow, schwab.InstructionSellToOpen, legPrice(best.PutLow, false, esMid)),
		}
	} else {
		best.Legs = []*Leg{
			NewLeg("#1", best.CallLow, schwab.InstructionSellToOpen, legPrice(best.CallLow, false, esMid)),
			NewLeg("#2", best.CallHigh, schwab.InstructionBuyToOpen, legPrice(best.CallHigh, true, esMid)),
			NewLeg("#3", best.PutHigh, schwab.InstructionSellToOpen, legPrice(best.PutHigh, false, esMid)),
			NewLeg("#4", best.PutLow, schwab.InstructionBuyToOpen, legPrice(best.PutLow, true, esMid)),
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
