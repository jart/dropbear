package main

import (
	"fmt"
	"strconv"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
)

func dbnPrice(p int64) decimal.Decimal {
	return decimal.FromInt64(p).DivInt64(databento.FixedPriceScale)
}

func formatET(ts clocky.Time) string {
	return ts.In(clocky.NYC).Format("15:04:05.000")
}

func formatPnl(d decimal.Decimal) string {
	if d.IsNegative() {
		return "-$" + d.Neg().Format(2)
	}
	return "+$" + d.Format(2)
}

func snapBid(px decimal.Decimal) decimal.Decimal {
	return px.QuantizeTruncate(*tickFlag)
}

func snapAsk(px decimal.Decimal) decimal.Decimal {
	return px.QuantizeAway(*tickFlag)
}

func greedyMid(q *quote, side byte, greed decimal.Decimal) decimal.Decimal {
	mid := q.bidPx.Add(q.askPx).DivInt(2)
	if greed.IsZero() {
		if side == 'B' {
			return snapBid(mid)
		}
		return snapAsk(mid)
	}
	if side == 'B' {
		target := mid.Sub(mid.Sub(q.bidPx).Mul(greed))
		return snapBid(target)
	} else {
		target := mid.Add(q.askPx.Sub(mid).Mul(greed))
		return snapAsk(target)
	}
}

func computeGreed(greedFlag decimal.Decimal, elapsed, patience clocky.Duration, filled int) decimal.Decimal {
	if greedFlag.Cmp(decimal.FromInt(1)) >= 0 || patience == 0 {
		return decimal.One
	}
	f := decimal.FromFloat64(float64(elapsed) / float64(patience)).Min(decimal.One)
	return greedFlag.Add(f.Mul(decimal.One.Sub(greedFlag)))
}

func budgetLimit(ab *activeBox, legIdx int, width decimal.Decimal, quotes map[uint32]*quote) decimal.Decimal {
	targetValue := width.Sub(*edgeFlag)
	if !ab.long {
		targetValue = width.Add(*edgeFlag).Neg()
	}
	var currentNet decimal.Decimal
	for i := range 4 {
		if i == legIdx {
			continue
		}
		if ab.legs[i] != nil {
			if ab.legs[i].side == 'B' {
				currentNet = currentNet.Add(ab.legs[i].fillPx)
			} else {
				currentNet = currentNet.Sub(ab.legs[i].fillPx)
			}
		} else {
			q := quotes[ab.pairLeg(i)]
			if q == nil || q.bidPx.IsZero() || q.askPx.IsZero() {
				currentNet = currentNet.Add(decimal.FromInt(500))
				continue
			}
			mid := q.bidPx.Add(q.askPx).DivInt(2)
			if ab.pairSide(i) == 'B' {
				currentNet = currentNet.Add(mid)
			} else {
				currentNet = currentNet.Sub(mid)
			}
		}
	}
	budget := targetValue.Sub(currentNet)
	if ab.pairSide(legIdx) == 'S' {
		budget = currentNet.Sub(targetValue)
	}
	return budget
}

func clampLimit(side byte, px, budget decimal.Decimal) decimal.Decimal {
	if side == 'B' {
		return px.Min(budget).Max(decimal.Zero)
	}
	return px.Max(budget).Max(*tickFlag)
}

func legQualified(q *quote) bool {
	if q.bidPx.Cmp(*minBidFlag) < 0 {
		return false
	}
	spread := q.askPx.Sub(q.bidPx)
	return spread.Cmp(*maxSpreadFlag) <= 0
}

func parseOSI(osi string) (strike decimal.Decimal, class byte, expDateInt int, err error) {
	if len(osi) < 21 {
		return decimal.Zero, 0, 0, fmt.Errorf("osi too short")
	}
	strikeVal, err := strconv.ParseInt(osi[13:21], 10, 64)
	if err != nil {
		return decimal.Zero, 0, 0, err
	}
	strike = decimal.FromInt64(strikeVal).DivInt(1000)
	class = osi[12]
	expVal, err := strconv.Atoi("20" + osi[6:12])
	if err != nil {
		return decimal.Zero, 0, 0, err
	}
	return strike, class, expVal, nil
}

func hasActivePair(active []*activeBox, bp *boxPair) bool {
	for _, ab := range active {
		if ab.pair == bp {
			return true
		}
	}
	return false
}

func legSpecs(bp *boxPair, long bool) [4]legSpec {
	if long {
		return [4]legSpec{{bp.callLoID, 'B'}, {bp.callHiID, 'S'}, {bp.putLoID, 'S'}, {bp.putHiID, 'B'}}
	}
	return [4]legSpec{{bp.callLoID, 'S'}, {bp.callHiID, 'B'}, {bp.putLoID, 'B'}, {bp.putHiID, 'S'}}
}

func startBox(b *broker, bp *boxPair, long bool, quotes map[uint32]*quote, ts clocky.Time, width decimal.Decimal) *activeBox {
	ab := &activeBox{
		pair:    bp,
		long:    long,
		startTS: ts,
	}
	specs := legSpecs(bp, long)
	for i, s := range specs {
		q := quotes[s.instID]
		px := greedyMid(q, s.side, *greedFlag)
		ab.orders[i] = b.limitOrder(s.instID, s.side, px, ts)
	}
	return ab
}

func printLegQuotes(bp *boxPair, quotes map[uint32]*quote) {
	qCL, qCH, qPL, qPH := quotes[bp.callLoID], quotes[bp.callHiID], quotes[bp.putLoID], quotes[bp.putHiID]
	fmt.Printf("    C_K1(%s): %s / %s\n", bp.loStrike.Format(2), qCL.bidPx.Format(2), qCL.askPx.Format(2))
	fmt.Printf("    C_K2(%s): %s / %s\n", bp.hiStrike.Format(2), qCH.bidPx.Format(2), qCH.askPx.Format(2))
	fmt.Printf("    P_K1(%s): %s / %s\n", bp.loStrike.Format(2), qPL.bidPx.Format(2), qPL.askPx.Format(2))
	fmt.Printf("    P_K2(%s): %s / %s\n", bp.hiStrike.Format(2), qPH.bidPx.Format(2), qPH.askPx.Format(2))
}
