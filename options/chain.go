package options

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"log"

	"github.com/emirpasic/gods/v2/maps/treemap"
)

// Chain represents the current state of an option chain.
type Chain struct {
	Price          decimal.Decimal
	AtTheMoney     *Strike
	LastPopulate   clocky.Time
	Strikes        *treemap.Map[decimal.Decimal, *Strike]
	pendingStrikes *treemap.Map[decimal.Decimal, *Strike]
}

// NewChain creates new options chain.
func NewChain() *Chain {
	return &Chain{
		Strikes:        treemap.New[decimal.Decimal, *Strike](),
		pendingStrikes: treemap.New[decimal.Decimal, *Strike](),
	}
}

// ExpectedMove returns the number of points in a one sigma underlying price movement.
// Returns positive number on success, or zero if data isn't ready. The returned number
// indicates the amount the price movement either up or down that's considered normal.
// There's a 68% probability the price movement will fall within this range. If you
// multiply the result by two, then you get the distance of a two sigma move, where
// there's a 95% probability the underlying will settle within that range. With three
// sigma, there's a 99.7% probability the underlying will settle within that range. In
// other words, a one sigma move happens about every one out of three trading days, a
// two sigma move happens about once a month, and three sigma happens about once a year.
func (c *Chain) ExpectedMove() decimal.Decimal {
	if c.AtTheMoney == nil || !c.AtTheMoney.IsReady() {
		return decimal.Zero
	}
	x := c.AtTheMoney.Call.MidPrice()
	y := c.AtTheMoney.Put.MidPrice()
	if !x.IsPositive() || !y.IsPositive() {
		return decimal.Zero
	}
	return x.Add(y)
}

// Add includes option in chain.
// This should be called repeatedly each time its quotes are updated.
// Returns true if this operation resulted in Price/AtTheMoney being updated.
func (c *Chain) Add(o *Option) bool {
	strikePrice := o.Strike.Price
	strike, _ := c.Strikes.Get(strikePrice)
	if strike == nil {
		strike = c.populateStrike(o, strikePrice)
	}
	if o.HasQuotes() && strike.IsReady() {
		if c.AtTheMoney == nil && strike.underlyingPrice().IsPositive() {
			c.AtTheMoney = strike
		}
		if strike == c.AtTheMoney {
			return c.updateFields()
		}
	}
	return false
}

//go:noinline
func (c *Chain) updateFields() bool {
	newPrice := c.AtTheMoney.underlyingPrice()
	if !newPrice.IsPositive() {
		return false
	}
	var atm *Strike
	_, strike1, _ := c.Strikes.Floor(newPrice)
	_, strike2, _ := c.Strikes.Ceiling(newPrice)
	if strike1 != nil && strike2 != nil {
		distance1 := strike1.Price.Sub(newPrice).Abs()
		distance2 := strike2.Price.Sub(newPrice).Abs()
		if distance2.Cmp(distance1) < 0 {
			atm = strike2
		} else {
			atm = strike1
		}
	} else if strike2 != nil {
		atm = strike2
	} else {
		atm = strike1
	}
	if atm != c.AtTheMoney {
		c.AtTheMoney = atm
		newPrice = atm.underlyingPrice()
	}
	if !newPrice.IsPositive() || newPrice == c.Price {
		return false
	}
	c.Price = newPrice
	return true
}

//go:noinline
func (c *Chain) populateStrike(o *Option, strikePrice decimal.Decimal) *Strike {
	strike, _ := c.pendingStrikes.Get(strikePrice)
	if strike == nil {
		strike = &Strike{Price: strikePrice, Chain: c}
		c.pendingStrikes.Put(strikePrice, strike)
	}
	o.Strike = strike
	o.Chain = c
	if o.Class == databento.InstrumentClassCall {
		strike.Call = o
	} else {
		strike.Put = o
	}
	if strike.IsReady() {
		c.pendingStrikes.Remove(strikePrice)
		_, strike.Prev, _ = c.Strikes.Floor(strikePrice)
		_, strike.Next, _ = c.Strikes.Ceiling(strikePrice)
		c.Strikes.Put(strikePrice, strike)
		if strike.Prev != nil {
			strike.Prev.Next = strike
		}
		if strike.Next != nil {
			strike.Next.Prev = strike
		}
		log.Printf("populated strike %s", strike)
		c.LastPopulate = clocky.Now()
	}
	return strike
}
