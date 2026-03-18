package main

import (
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"log"
	"sync"
)

type Leg struct {
	Name        string // e.g. "#1", "#2", "#3", "#4"
	Option      *Option
	Instruction schwab.Instruction
	Price       decimal.Decimal // actual order price
	Lock        sync.RWMutex    // protects orderID, finished, filled
	OrderID     int64           // schwab order ID, or 0 if not yet placed, or canceled
	Finished    bool            // true if order submission completed (success or failure)
	Filled      bool            // true if order was filled (confirmed by schwab websocket)
}

func NewLeg(name string, option *Option, instruction schwab.Instruction, price decimal.Decimal) *Leg {
	return &Leg{
		Name:        name,
		Option:      option,
		Instruction: instruction,
		Price:       price,
	}
}

func (l *Leg) Order() {
	orderID, err := schwabClient.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             l.Price,
		Duration:          schwab.DurationDay,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Instruction: l.Instruction,
			Quantity:    decimal.One,
			Instrument: schwab.Instrument{
				Symbol: l.Option.OSI(),
				Type:   schwab.AssetTypeOption,
			},
		}},
	})
	if err != nil {
		log.Printf("order FAILED %s %s %s %s @ %s: %v",
			l.Name, l.Instruction, l.Option.Class, l.Option.Strike, l.Price, err)
		l.Lock.Lock()
		defer l.Lock.Unlock()
		l.Finished = true
		return
	}
	log.Printf("order SENT %s %s %s %s @ %s (id=%d)",
		l.Name, l.Instruction, l.Option.Class, l.Option.Strike, l.Price, orderID)
	l.Lock.Lock()
	defer l.Lock.Unlock()
	l.OrderID = orderID
	l.Finished = true
}
