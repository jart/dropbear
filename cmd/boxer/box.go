package main

import "dropbear/decimal"

type Box struct {
	Low      decimal.Decimal
	High     decimal.Decimal
	Width    decimal.Decimal
	CallLow  *Option
	CallHigh *Option
	PutLow   *Option
	PutHigh  *Option
	Mid      decimal.Decimal // unrounded box midpoint price
	Price    decimal.Decimal // rounded limit price (what we'd pay or receive)
	Profit   decimal.Decimal // guaranteed profit per point at expiration
	Edge     decimal.Decimal // how much better than midpoint we're demanding
	Buying   bool
	Legs     []*Leg
}

func (b *Box) AllFilled() bool {
	for _, l := range b.Legs {
		if !l.Filled {
			return false
		}
	}
	return len(b.Legs) == 4
}

func (b *Box) Order() {
	for _, l := range b.Legs {
		go l.Order()
	}
}
