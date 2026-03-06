package main

import (
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"log"
	"sync"
)

type Leg struct {
	opt         *Option
	instruction schwab.Instruction
	price       decimal.Decimal // actual order price
	lock        sync.RWMutex    // protects orderID
	orderID     int64           // schwab order ID, or 0 if not yet placed, or canceled
	finished    bool            // true if order submission completed (success or failure)
	filled      bool            // true if order was filled (confirmed by schwab websocket)
}

func NewLeg(opt *Option, instruction schwab.Instruction) *Leg {
	l := &Leg{
		opt:         opt,
		instruction: instruction,
	}
	mid := opt.Bid.Add(opt.Ask).DivInt(2)
	t := optionTick(mid)
	if l.instruction == schwab.InstructionBuyToOpen {
		l.price = mid.QuantizeTruncate(t)
	} else {
		l.price = mid.QuantizeAway(t)
	}
	return l
}

func (l *Leg) Order() {
	orderID, err := schwabClient.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             l.price,
		Duration:          schwab.DurationDay,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{
			{
				Instruction: l.instruction,
				Quantity:    decimal.FromInt(1),
				Instrument: schwab.Instrument{
					Symbol: l.opt.OSI(),
					Type:   schwab.AssetTypeOption,
				},
			},
		},
	})
	if err != nil {
		log.Printf("order FAILED %s %s %s @ %s: %v",
			l.instruction, l.opt.Class, l.opt.Strike.Format(0), l.price.Format(2), err)
		l.lock.Lock()
		l.finished = true
		l.lock.Unlock()
		return
	}
	log.Printf("order SENT %s %s %s @ %s (id=%d)",
		l.instruction, l.opt.Class, l.opt.Strike.Format(0), l.price.Format(2), orderID)
	l.lock.Lock()
	l.orderID = orderID
	l.finished = true
	l.lock.Unlock()
}
