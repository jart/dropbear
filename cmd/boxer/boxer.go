package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
	"sort"
)

var (
	gLastBoxTime clocky.Time
)

func boxer() {

	// ensure dependencies are ready
	now := clocky.Now()
	if gES == nil || gSR1 == nil {
		return
	}
	if now.Sub(gES.TS) > *freshFlag {
		return
	}

	// check if we need to close partially filled boxes
	for it := gPendingBoxes.Iterator(); it.Next(); {
		box := it.Value()
		if box.Closing {
			continue
		}
		if !box.PartiallyFilled() {
			continue
		}
		if box.ClosingProfit().IsNegative() {
			continue
		}
		if now.Sub(box.Created) < *patienceFlag {
			continue
		}
		box.Close()
	}

	// prevent creating new boxes when imbalance is too high
	bulls := gUnfilledBulls.Size()
	bears := gUnfilledBears.Size()
	if bulls+bears >= *maxUnfilledFlag {
		return
	}
	imbalance := bulls - bears
	if imbalance < 0 {
		imbalance = -imbalance
	}
	if imbalance >= *maxImbalanceFlag {
		return
	}

	// cooldown between boxes
	if now.Sub(gLastBoxTime) < *cooldownFlag {
		return
	}

	// group options by strike into call/put pairs
	strikes := make(map[decimal.Decimal]*Strike)
	for _, opt := range gOptionsByID {
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

	// collect strikes that have both a call and a put that are ready to trade
	var valid []decimal.Decimal
	for strike, sp := range strikes {
		if sp.Call != nil && sp.Put != nil &&
			sp.Call.IsReady() && sp.Put.IsReady() {
			valid = append(valid, strike)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].Cmp(valid[j]) < 0
	})
	if len(valid) == 0 {
		return
	}

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

			// only trade strikes near the money
			if strikeI.Sub(gES.Price).Abs().Cmp(*moneynessFlag) > 0 ||
				strikeJ.Sub(gES.Price).Abs().Cmp(*moneynessFlag) > 0 {
				continue
			}

			// skip boxes where both strikes are on the same side of ES
			if (strikeI.Cmp(gES.Price) > 0 && strikeJ.Cmp(gES.Price) > 0) ||
				(strikeI.Cmp(gES.Price) < 0 && strikeJ.Cmp(gES.Price) < 0) {
				continue
			}

			// check box isn't too large
			width := strikeJ.Sub(strikeI)
			if width.Abs().Cmp(*widthFlag) > 0 {
				continue
			}

			// check if opening these legs won't clobber existing positions
			if !spI.Call.CanBuy() || !spI.Put.CanSell() || !spJ.Call.CanSell() || !spJ.Put.CanBuy() {
				continue
			}

			// we shall create a box spread
			box := &Box{Created: now}
			box.BuyCall = &Leg{
				Box:    box,
				Name:   "#1",
				Option: spI.Call,
			}
			box.SellCall = &Leg{
				Box:    box,
				Name:   "#2",
				Option: spJ.Call,
			}
			box.SellPut = &Leg{
				Box:    box,
				Name:   "#3",
				Option: spI.Put,
			}
			box.BuyPut = &Leg{
				Box:    box,
				Name:   "#4",
				Option: spJ.Put,
			}

			// choose limit prices
			box.ChooseLimitPrices()
			box.ApplyGreed()
			box.Check()

			// check if box is profitable enough
			// todo(jart): break ties based on what improves risk profile most
			profit := box.LimitProfit()
			if best == nil || profit.Cmp(bestProfit) > 0 {
				best = box
				bestProfit = profit
			}
		}
	}

	// check if we found an acceptable box
	if best == nil {
		return
	}
	if bestProfit.MulInt(100).Cmp(*demandFlag) < 0 {
		return
	}
	log.Printf("best box: %s", best)
	if *dryFlag {
		return
	}

	// start manufacturing box
	gLastBoxTime = now
	best.Order()
}
