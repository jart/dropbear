package main

import "dropbear/decimal"

type Box struct {
	low      decimal.Decimal
	high     decimal.Decimal
	width    decimal.Decimal
	callLow  *Option
	callHigh *Option
	putLow   *Option
	putHigh  *Option
	mid      decimal.Decimal // unrounded box midpoint price
	price    decimal.Decimal // rounded limit price (what we'd pay or receive)
	profit   decimal.Decimal // guaranteed profit per point at expiration
	edge     decimal.Decimal // how much better than midpoint we're demanding
	buying   bool
	legs     []*Leg
}

func (b *Box) Order() {
	for _, l := range b.legs {
		go l.Order()
	}
}
