package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"fmt"
	"log"
)

type Leg struct {
	Box        *Box            // the box this leg belongs to
	Name       string          // arbitrary human friendly name for logs, e.g. "#1", "#2", "#3", "#4"
	Option     *Option         // the option instrument for this leg
	LimitPrice decimal.Decimal // our limit order price (negative if buying, e.g. -0.15 means we get a $15 debit or in otherwords are paying $15 for the leg)
	Greed      decimal.Decimal // the amount of greed applied to the limit price (positive means more greedy, negative means more generous)
	OrderID    int64           // schwab order ID, or 0 if not yet placed, or canceled
	FillPrice  decimal.Decimal // the fill price of the leg (always positive, zero if not yet filled)
}

type LegUpdate struct {
	Leg     *Leg
	OrderID int64
}

func (l *Leg) String() string {
	kind := "bear"
	if l.IsBull() {
		kind = "bull"
	}
	return fmt.Sprintf("%s %s %s %s @ %s (greed=%s bid=%s ask=%s)",
		l.Name, l.Instruction(), kind, l.Option, l.LimitPrice, l.Greed, l.Option.Bid, l.Option.Ask)
}

func (l *Leg) IsBull() bool {
	return (l.Option.Class == databento.InstrumentClassCall && l.IsBuying()) ||
		(l.Option.Class == databento.InstrumentClassPut && !l.IsBuying())
}

func (l *Leg) IsSafe() bool {
	return (l.Option.Class == databento.InstrumentClassCall && es.Price.Add(*safetyFlag).Cmp(l.Option.Strike) <= 0) ||
		(l.Option.Class == databento.InstrumentClassPut && es.Price.Sub(*safetyFlag).Cmp(l.Option.Strike) >= 0)
}

func (l *Leg) IsBuying() bool {
	return l.LimitPrice.IsNegative()
}

func (l *Leg) Filled() bool {
	return !l.FillPrice.IsZero()
}

func (l *Leg) ApplyGreed() {
	bullCount := unfilledBulls.Size()
	bearCount := unfilledBears.Size()
	bullImbalance := bullCount - bearCount
	bearImbalance := bearCount - bullCount
	if l.IsBull() {
		if bearImbalance > 0 {
			// we have too many bear options in flight and this leg reduces our risk
			// therefore be generous with our limit price in accordance with our pain
			l.LimitPrice, l.Greed = applyGenerosity(l.LimitPrice, max(bearImbalance, 1))
		} else if bullImbalance > 0 && !l.IsSafe() {
			// we have too many bull options in flight and this fill would worsen it
			// therefore be greedy unless it's a safe leg (no greed on safe legs)
			l.LimitPrice, l.Greed = applyGreed(l.LimitPrice, max(bullImbalance, 2))
		}
	} else {
		if bullImbalance > 0 {
			// we have too many bull options in flight and this leg reduces our risk
			l.LimitPrice, l.Greed = applyGenerosity(l.LimitPrice, max(bullImbalance, 1))
		} else if bearImbalance > 0 && !l.IsSafe() {
			// we have too many bear options in flight and this fill would worsen it
			l.LimitPrice, l.Greed = applyGreed(l.LimitPrice, max(bearImbalance, 2))
		}
	}
}

func (l *Leg) Instruction() schwab.Instruction {
	if l.IsBuying() {
		return schwab.InstructionBuyToOpen
	}
	return schwab.InstructionSellToOpen
}

func (l *Leg) Order(legUpdates chan<- LegUpdate) {
	l.Check()
	if l.IsBuying() {
		restrictedToBuying.Add(l.Option.ID)
	} else {
		restrictedToSelling.Add(l.Option.ID)
	}
	if l.IsBull() {
		unfilledBulls.Add(l)
	} else {
		unfilledBears.Add(l)
	}
	go l.doOrder(legUpdates)
}

func (l *Leg) doOrder(legUpdates chan<- LegUpdate) {
	orderID, err := schwabClient.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             l.LimitPrice.Abs(),
		Duration:          schwab.DurationDay,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Instruction: l.Instruction(),
			Quantity:    decimal.One,
			Instrument: schwab.Instrument{
				Symbol: l.Option.OSI(),
				Type:   schwab.AssetTypeOption,
			},
		}},
	})
	if err != nil {
		log.Fatalf("order FAILED %s %s %s %s @ %s: %v",
			l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), err)
	}
	log.Printf("order SENT %s %s %s %s @ %s (id=%d)",
		l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), orderID)
	legUpdates <- LegUpdate{Leg: l, OrderID: orderID}
}

// Check checks invariants.
func (l *Leg) Check() {
	if l.Option == nil {
		panic("Leg must have an option")
	}
	if l.LimitPrice.IsZero() {
		panic("Leg must have a nonzero limit price")
	}
	if l.LimitPrice.Cmp(quantizeTruncateSPX(l.LimitPrice)) != 0 {
		panic("Leg limit price must be quantized to SPX tick size")
	}
	if l.IsBuying() {
		if restrictedToSelling.Contains(l.Option.ID) {
			panic("Leg is buying but option is restricted to selling only")
		}
	} else {
		if restrictedToBuying.Contains(l.Option.ID) {
			panic("Leg is selling but option is restricted to buying only")
		}
	}
}

func (l *Leg) LogTickIfRelevant(option *Option) {
	if l.Option == option && !l.Filled() {
		log.Printf("tick %s %s %s %s bid=%s ask=%s spread=%s",
			l.Name, l.Instruction(), option.Class, option.Strike,
			option.Bid, option.Ask, option.Ask.Sub(option.Bid))
	}
}

func (l *Leg) UpdateOrderID(newID int64) {
	oldID := l.OrderID
	l.OrderID = newID
	log.Printf("leg %s order ID updated in thinkorswim %d -> %d", l.Name, oldID, newID)
}
