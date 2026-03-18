package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"sort"

	"github.com/emirpasic/gods/v2/sets/hashset"
)

var (
	boxes       = hashset.New[*Box]() // set of boxes currently being worked on (not yet fully filled)
	lastBoxTime clocky.Time
)

func boxer() {

	// ensure dependencies are ready
	now := clocky.Now()
	if es == nil {
		return
	}
	if now.Sub(es.TS) > *freshFlag {
		return
	}

	// make boxes one at a time
	if boxes.Size() > 0 {
		return
	}

	// cooldown between boxes
	if now.Sub(lastBoxTime) < *cooldownFlag {
		return
	}

	// group options by strike into call/put pairs
	strikes := make(map[decimal.Decimal]*Strike)
	for _, opt := range optionsByID {
		sp := strikes[opt.Strike]
		if sp == nil {
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
	var bestProfit decimal.Decimal
	for i := 0; i < len(valid); i++ {
		for j := 0; j < len(valid); j++ {
			if i == j {
				continue
			}
			strikeI := valid[i]
			strikeJ := valid[j]
			spI := strikes[strikeI]
			spJ := strikes[strikeJ]

			// check box isn't too large
			width := strikeJ.Sub(strikeI)
			if width.Abs().Cmp(*widthFlag) > 0 {
				continue
			}

			// only trade if quotes are fresh and underlying hasn't moved significantly
			// schwab leaks our order flow so we can not actually pick off stale quotes
			if !spI.Call.IsFresh(now) ||
				!spI.Put.IsFresh(now) ||
				!spJ.Call.IsFresh(now) ||
				!spJ.Put.IsFresh(now) {
				continue
			}

			// check if opening these legs won't clobber existing positions
			if !spI.Call.CanBuy() ||
				!spI.Put.CanSell() ||
				!spJ.Call.CanSell() ||
				!spJ.Put.CanBuy() {
				continue
			}

			// create box
			box := &Box{}
			box.CallLeg1 = &Leg{
				Box:        box,
				Name:       "#1",
				Option:     spI.Call,
				LimitPrice: quantizeTruncateSPX(spI.Call.MarketPrice()),
			}
			box.CallLeg2 = &Leg{
				Box:        box,
				Name:       "#2",
				Option:     spJ.Call,
				LimitPrice: quantizeAwaySPX(spJ.Call.MarketPrice()).Neg(),
			}
			box.PutLeg1 = &Leg{
				Box:        box,
				Name:       "#3",
				Option:     spI.Put,
				LimitPrice: quantizeAwaySPX(spI.Put.MarketPrice()).Neg(),
			}
			box.PutLeg2 = &Leg{
				Box:        box,
				Name:       "#4",
				Option:     spJ.Put,
				LimitPrice: quantizeTruncateSPX(spJ.Put.MarketPrice()),
			}
			box.ApplyGreed()

			// check if box is profitable enough
			profit := box.LimitProfit()
			if profit.IsPositive() && (best == nil || profit.Cmp(bestProfit) > 0) {
				best = box
				bestProfit = profit
				box.Check()
			}
		}
	}

	// check if we found an acceptable box
	if best == nil {
		return
	}
	dollars := bestProfit.MulInt(100)
	if dollars.Cmp(*demandFlag) < 0 {
		return
	}
	log.Printf("best box: %s", best)
	if *dryFlag {
		return
	}

	// start manufacturing box
	lastBoxTime = now
	best.Order(legUpdates)
}
