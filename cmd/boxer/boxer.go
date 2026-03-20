package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"log"
)

var (
	gLastBoxTime clocky.Time
)

func boxer() {

	// ensure dependencies are ready
	now := clocky.Now()
	if gES == nil || gSR1 == nil || gSPXPrice.IsZero() {
		return
	}
	if now.Sub(gES.TS) > *freshFlag {
		return
	}
	if now.Sub(gSPXPriceTime) > *freshFlag {
		return
	}

	// // check if we need to close partially filled boxes
	// for it := gPendingBoxes.Iterator(); it.Next(); {
	// 	box := it.Value()
	// 	if box.Closing {
	// 		continue
	// 	}
	// 	if !box.PartiallyFilled() {
	// 		continue
	// 	}
	// 	closingProfit := box.ClosingProfit()
	// 	if !closingProfit.IsPositive() {
	// 		continue
	// 	}
	// 	elapsed := now.Sub(box.Created)
	// 	if elapsed < *patienceFlag {
	// 		continue
	// 	}
	// 	log.Printf("aborting partially filled box for %s profit (planned %s) after waiting %s: %s",
	// 		closingProfit, box.FillProfit(), elapsed, box)
	// 	box.Close()
	// }

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

	// evaluate all box spread combinations
	var best *Box
	var bestProfit decimal.Decimal
	for itI := gStrikes.Iterator(); itI.Next(); {
		spI := itI.Value()
		for itJ := gStrikes.Iterator(); itJ.Next(); {
			spJ := itJ.Value()
			if spI == spJ {
				continue
			}
			strikeI := spI.Strike()
			strikeJ := spJ.Strike()

			// only trade strikes near the money
			if strikeI.Sub(gSPXPrice).Abs().Cmp(*moneynessFlag) > 0 ||
				strikeJ.Sub(gSPXPrice).Abs().Cmp(*moneynessFlag) > 0 {
				continue
			}

			// skip boxes where both strikes are on the same side of money
			if (strikeI.Cmp(gSPXPrice) > 0 && strikeJ.Cmp(gSPXPrice) > 0) ||
				(strikeI.Cmp(gSPXPrice) < 0 && strikeJ.Cmp(gSPXPrice) < 0) {
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
