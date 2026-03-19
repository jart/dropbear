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
	if es == nil || sr1 == nil {
		return
	}
	if now.Sub(es.TS) > *freshFlag {
		// log.Printf("ES quote is stale (last update %s ago), skipping box creation", now.Sub(es.TS))
		return
	}

	// prevent creating new boxes when imbalance is too high
	bulls := unfilledBulls.Size()
	bears := unfilledBears.Size()
	imbalance := bulls - bears
	if imbalance < 0 {
		imbalance = -imbalance
	}
	if imbalance >= *maxImbalanceFlag {
		// log.Printf("imbalance too high (bulls=%d bears=%d), skipping box creation", bulls, bears)
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
	var fail1, fail2, fail3, fail4 int
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
			if strikeI.Sub(es.Price).Abs().Cmp(*moneynessFlag) > 0 ||
				strikeJ.Sub(es.Price).Abs().Cmp(*moneynessFlag) > 0 {
				fail1 += 1
				continue
			}

			// skip boxes where both strikes are on the same side of ES
			if (strikeI.Cmp(es.Price) > 0 && strikeJ.Cmp(es.Price) > 0) ||
				(strikeI.Cmp(es.Price) < 0 && strikeJ.Cmp(es.Price) < 0) {
				continue
			}

			// check box isn't too large
			width := strikeJ.Sub(strikeI)
			if width.Abs().Cmp(*widthFlag) > 0 {
				fail2 += 1
				continue
			}

			// check if opening these legs won't clobber existing positions
			if !spI.Call.CanBuy() || !spI.Put.CanSell() || !spJ.Call.CanSell() || !spJ.Put.CanBuy() {
				fail4 += 1
				continue
			}

			// create box
			box := &Box{}
			box.CallLeg1 = &Leg{
				Box:        box,
				Name:       "#1",
				Option:     spI.Call,
				LimitPrice: quantizeAwaySPX(spI.Call.FairPrice()).Neg(),
			}
			box.CallLeg2 = &Leg{
				Box:        box,
				Name:       "#2",
				Option:     spJ.Call,
				LimitPrice: quantizeTruncateSPX(spJ.Call.FairPrice()),
			}
			box.PutLeg1 = &Leg{
				Box:        box,
				Name:       "#3",
				Option:     spI.Put,
				LimitPrice: quantizeTruncateSPX(spI.Put.FairPrice()),
			}
			box.PutLeg2 = &Leg{
				Box:        box,
				Name:       "#4",
				Option:     spJ.Put,
				LimitPrice: quantizeAwaySPX(spJ.Put.FairPrice()).Neg(),
			}
			box.ApplyGreed()
			box.Check()

			// check if box is profitable enough
			profit := box.LimitProfit()
			if best == nil || profit.Cmp(bestProfit) > 0 {
				best = box
				bestProfit = profit
			}
		}
	}

	// check if we found an acceptable box
	if best == nil {
		log.Printf("no box found (fail1=%d fail2=%d fail3=%d fail4=%d)", fail1, fail2, fail3, fail4)
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
	lastBoxTime = now
	best.Order(legUpdates)
}
