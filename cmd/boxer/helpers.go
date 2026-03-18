package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
)

var (
	tick05  = decimal.Parse("0.05")
	tick10  = decimal.Parse("0.10")
	three   = decimal.FromInt(3)
	fifteen = decimal.FromInt(15)
)

// quantizeTruncateSPX rounds to the SPX tick size for buying.
func quantizeTruncateSPX(price decimal.Decimal) decimal.Decimal {
	tick := optionTick(price)
	price = price.QuantizeTruncate(tick)
	if price.IsZero() {
		// handle cases like 0.00/0.05 bid/ask on far otm strikes
		price = tick
	}
	return price
}

// applyGreed adds ticks to limit price to demand better deal.
func applyGreed(price decimal.Decimal, count int) (newPrice, greed decimal.Decimal) {
	for range count {
		// price is negative for buy legs and positive for sell legs.
		// positive price + tick = sell higher
		// negative price + tick = buy lower
		tick := optionTick(price)
		price2 := price.Add(tick)
		if price2.IsZero() {
			break
		}
		greed = greed.Add(tick)
		price = price2
	}
	return price, greed
}

// applyGenerosity adds ticks to limit price to offer better deal.
func applyGenerosity(price decimal.Decimal, count int) (newPrice, greed decimal.Decimal) {
	for range count {
		// price is negative for buy legs and positive for sell legs.
		// positive price - tick = sell lower
		// negative price - tick = buy higher
		tick := optionTick(price)
		price2 := price.Sub(tick)
		if price2.IsZero() {
			break
		}
		greed = greed.Sub(tick)
		price = price2
	}
	return price, greed
}

// quantizeAwaySPX rounds to the SPX tick size for selling.
func quantizeAwaySPX(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeAway(optionTick(price))
}

// optionTick returns the minimum tick size for a Penny Pilot option.
// Options priced under $3 tick in $0.05; $3 and over tick in $0.10.
func optionTick(price decimal.Decimal) decimal.Decimal {
	if price.Abs().Cmp(three) < 0 {
		return tick05
	}
	return tick10
}

func dbnPrice(p int64) decimal.Decimal {
	if p == databento.UndefPrice {
		return decimal.Zero
	}
	return decimal.Decimal(p / 1000)
}
