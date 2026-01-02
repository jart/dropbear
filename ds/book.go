package ds

import (
	"dropbear/decimal"

	"github.com/emirpasic/gods/v2/maps/treemap"
)

// Book represents an order book for a single product.
type Book struct {
	Bids *treemap.Map[decimal.Decimal, decimal.Decimal] // price -> size, descending (best bid = first)
	Asks *treemap.Map[decimal.Decimal, decimal.Decimal] // price -> size, ascending (best ask = first)
}

// NewBook creates a new order book.
func NewBook() *Book {
	return &Book{
		Bids: treemap.NewWith[decimal.Decimal, decimal.Decimal](decimal.CompareReverse),
		Asks: treemap.NewWith[decimal.Decimal, decimal.Decimal](decimal.Compare),
	}
}

// MidPrice returns the market price.
func (b *Book) MidPrice() decimal.Decimal {
	bid, ask := b.BestBidAsk()
	return bid.Add(ask).DivInt(2)
}

// BestBidAsk returns the national best bid and ask prices.
// Normally ask is just a little bit higher than bid.
// However it's possible for the books to overlap.
func (b *Book) BestBidAsk() (bid, ask decimal.Decimal) {
	bid, _, _ = b.Bids.Min()
	ask, _, _ = b.Asks.Min()
	return
}

// Update updates or removes a level.
func (b *Book) Update(side Side, price, size decimal.Decimal) {
	switch side {
	case SideBuy:
		b.UpdateBid(price, size)
	case SideSell:
		b.UpdateAsk(price, size)
	}
}

// UpdateBid updates or removes a bid level.
func (b *Book) UpdateBid(price, size decimal.Decimal) {
	if size.IsPositive() {
		b.Bids.Put(price, size)
	} else {
		b.Bids.Remove(price)
	}
}

// UpdateAsk updates or removes an ask level.
func (b *Book) UpdateAsk(price, size decimal.Decimal) {
	if size.IsPositive() {
		b.Asks.Put(price, size)
	} else {
		b.Asks.Remove(price)
	}
}

// Clear removes all levels from the book.
// The write lock must be held by the caller.
func (b *Book) Clear() {
	b.Bids.Clear()
	b.Asks.Clear()
}

// Add includes limit order in order book.
func (b *Book) Add(side Side, price, size decimal.Decimal) {
	book := b.Asks
	if side == SideBuy {
		book = b.Bids
	}
	existing, found := book.Get(price)
	if found {
		book.Put(price, existing.Add(size))
	} else {
		book.Put(price, size)
	}
}

// BidDepthAbove returns the cumulative bid volume at prices strictly above the given price.
// This represents the queue ahead of a bid order at the given price.
func (b *Book) BidDepthAbove(price decimal.Decimal) decimal.Decimal {
	if price.IsZero() {
		return decimal.Zero
	}
	var depth decimal.Decimal
	it := b.Bids.Iterator()
	for it.Next() {
		p := it.Key()
		if p.Cmp(price) <= 0 {
			break // Bids are descending, so we're done
		}
		size := it.Value()
		newDepth := depth.Add(size)
		if newDepth.Cmp(depth) < 0 {
			newDepth = decimal.Max
		}
		depth = newDepth
	}
	return depth
}

// AskDepthBelow returns the cumulative ask volume at prices strictly below the given price.
// This represents the queue ahead of an ask order at the given price.
func (b *Book) AskDepthBelow(price decimal.Decimal) decimal.Decimal {
	if price.IsZero() {
		return decimal.Zero
	}
	var depth decimal.Decimal
	it := b.Asks.Iterator()
	for it.Next() {
		p := it.Key()
		if p.Cmp(price) >= 0 {
			break // Asks are ascending, so we're done
		}
		size := it.Value()
		newDepth := depth.Add(size)
		if newDepth.Cmp(depth) < 0 {
			newDepth = decimal.Max
		}
		depth = newDepth
	}
	return depth
}

// MarketBuyCost estimates the total cost (in quote currency) to buy baseQty
// by walking the ask side of the book. Returns cost and whether there was
// sufficient liquidity. This simulates a market buy order.
func (b *Book) MarketBuyCost(baseQty decimal.Decimal) (cost decimal.Decimal, ok bool) {
	remaining := baseQty
	it := b.Asks.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		if size.Cmp(remaining) >= 0 {
			cost = cost.Add(remaining.Mul(price))
			return cost, true
		}
		cost = cost.Add(size.Mul(price))
		remaining = remaining.Sub(size)
	}
	return cost, false // insufficient liquidity
}

// MarketSellProceeds estimates the total proceeds (in quote currency) from
// selling baseQty by walking the bid side of the book. Returns proceeds and
// whether there was sufficient liquidity. This simulates a market sell order.
func (b *Book) MarketSellProceeds(baseQty decimal.Decimal) (proceeds decimal.Decimal, ok bool) {
	remaining := baseQty
	it := b.Bids.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		if size.Cmp(remaining) >= 0 {
			proceeds = proceeds.Add(remaining.Mul(price))
			return proceeds, true
		}
		proceeds = proceeds.Add(size.Mul(price))
		remaining = remaining.Sub(size)
	}
	return proceeds, false // insufficient liquidity
}

// MarketBuyCostLimited estimates the cost to buy baseQty, but only walks
// the ask side up to limitPrice. Returns cost and whether there was sufficient
// liquidity below the limit. This simulates an IOC buy order with limit.
func (b *Book) MarketBuyCostLimited(baseQty, limitPrice decimal.Decimal) (cost decimal.Decimal, ok bool) {
	remaining := baseQty
	it := b.Asks.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		// Stop if we've exceeded the limit price
		if price.Cmp(limitPrice) > 0 {
			break
		}
		if size.Cmp(remaining) >= 0 {
			cost = cost.Add(remaining.Mul(price))
			return cost, true
		}
		cost = cost.Add(size.Mul(price))
		remaining = remaining.Sub(size)
	}
	return cost, false // insufficient liquidity at or below limit
}

// MarketSellProceedsLimited estimates the proceeds from selling baseQty, but only
// walks the bid side down to limitPrice. Returns proceeds and whether there was
// sufficient liquidity above the limit. This simulates an IOC sell order with limit.
func (b *Book) MarketSellProceedsLimited(baseQty, limitPrice decimal.Decimal) (proceeds decimal.Decimal, ok bool) {
	remaining := baseQty
	it := b.Bids.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		// Stop if we've fallen below the limit price
		if price.Cmp(limitPrice) < 0 {
			break
		}
		if size.Cmp(remaining) >= 0 {
			proceeds = proceeds.Add(remaining.Mul(price))
			return proceeds, true
		}
		proceeds = proceeds.Add(size.Mul(price))
		remaining = remaining.Sub(size)
	}
	return proceeds, false // insufficient liquidity at or above limit
}

// TopBids returns the top n bid levels (best first).
func (b *Book) TopBids(n int) []Level {
	var levels []Level
	it := b.Bids.Iterator()
	for it.Next() && len(levels) < n {
		levels = append(levels, Level{
			Price: it.Key(),
			Size:  it.Value(),
		})
	}
	return levels
}

// TopAsks returns the top n ask levels (best first).
func (b *Book) TopAsks(n int) []Level {
	var levels []Level
	it := b.Asks.Iterator()
	for it.Next() && len(levels) < n {
		levels = append(levels, Level{
			Price: it.Key(),
			Size:  it.Value(),
		})
	}
	return levels
}

// PickAsk picks a good ask price.
// Assume we want to sell $1000 of coins and match the national best asking price of $10.00.
// Since crypto lets you make $1 trades, there's no point in us doing that if the existing Asks
// at price $10.00, $10.01, and $10.02 sum up to fifty dollars.
func (b *Book) PickAsk(depth decimal.Decimal) decimal.Decimal {
	var p, d decimal.Decimal
	it := b.Asks.Iterator()
	for it.Next() {
		p = it.Key()
		d = d.Add(it.Value())
		if d.Cmp(depth) >= 0 {
			break
		}
	}
	return p
}

// PickBid picks a good bid price.
// This is the mirror of PickAsk for the buy side.
// Assume we want to buy $1000 of coins and match the national best bid price of $10.00.
// Since crypto lets you make $1 trades, there's no point in us doing that if the existing Bids
// at price $10.00, $9.99, and $9.98 sum up to fifty dollars.
func (b *Book) PickBid(depth decimal.Decimal) decimal.Decimal {
	var p, d decimal.Decimal
	it := b.Bids.Iterator()
	for it.Next() {
		p = it.Key()
		d = d.Add(it.Value())
		if d.Cmp(depth) >= 0 {
			break
		}
	}
	return p
}

// PickAskByValue returns the ask price where cumulative value (price*size) >= valueDepth.
// This represents the worst price you'd pay to buy valueDepth worth of asset.
func (b *Book) PickAskByValue(valueDepth decimal.Decimal) decimal.Decimal {
	var p, v decimal.Decimal
	it := b.Asks.Iterator()
	for it.Next() {
		p = it.Key()
		v = v.Add(p.Mul(it.Value()))
		if v.Cmp(valueDepth) >= 0 {
			break
		}
	}
	return p
}

// PickBidByValue returns the bid price where cumulative value (price*size) >= valueDepth.
// This represents the worst price you'd get selling valueDepth worth of asset.
func (b *Book) PickBidByValue(valueDepth decimal.Decimal) decimal.Decimal {
	var p, v decimal.Decimal
	it := b.Bids.Iterator()
	for it.Next() {
		p = it.Key()
		v = v.Add(p.Mul(it.Value()))
		if v.Cmp(valueDepth) >= 0 {
			break
		}
	}
	return p
}

// MidPriceAtDepth returns the mid price at a given cumulative value depth.
func (b *Book) MidPriceAtDepth(valueDepth decimal.Decimal) decimal.Decimal {
	bid := b.PickBidByValue(valueDepth)
	ask := b.PickAskByValue(valueDepth)
	if bid.IsZero() || ask.IsZero() {
		return decimal.Zero
	}
	return bid.Add(ask).DivInt(2)
}

// Buy helps fill an IOC (immediate-or-cancel) market buy order.
// Fills as much quantity as possible without exceeding maxValue.
// Returns quantity filled and array of fills at each price level hit.
func (b *Book) Buy(quantity, buyingPower decimal.Decimal) (decimal.Decimal, []Level) {
	var fills []Level
	remaining := quantity
	var spent decimal.Decimal
	it := b.Asks.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		// How much value budget do we have left?
		budget := buyingPower.Sub(spent)
		if budget.IsZero() || !budget.IsPositive() {
			break
		}
		// How much can we afford at this price?
		affordable := budget.Div(price)
		// Take the minimum of: what we want, what's available, what we can afford
		fillSize := remaining
		if size.Cmp(fillSize) < 0 {
			fillSize = size
		}
		if affordable.Cmp(fillSize) < 0 {
			fillSize = affordable
		}
		if fillSize.IsZero() || !fillSize.IsPositive() {
			break
		}
		fills = append(fills, Level{Price: price, Size: fillSize})
		spent = spent.Add(fillSize.Mul(price))
		remaining = remaining.Sub(fillSize)
		newSize := size.Sub(fillSize)
		if newSize.IsZero() {
			b.Asks.Remove(price)
		} else {
			b.Asks.Put(price, newSize)
		}
		if remaining.IsZero() {
			break
		}
	}
	return quantity.Sub(remaining), fills
}

// Sell helps fill an IOC (immediate-or-cancel) market sell order.
// Fills as much quantity as possible.
// Returns quantity filled and array of fills at each price level hit.
func (b *Book) Sell(quantity decimal.Decimal) (decimal.Decimal, []Level) {
	var fills []Level
	remaining := quantity
	it := b.Bids.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		fillSize := remaining
		if size.Cmp(fillSize) < 0 {
			fillSize = size
		}
		fills = append(fills, Level{Price: price, Size: fillSize})
		remaining = remaining.Sub(fillSize)
		newSize := size.Sub(fillSize)
		if newSize.IsZero() {
			b.Bids.Remove(price)
		} else {
			b.Bids.Put(price, newSize)
		}
		if remaining.IsZero() {
			break
		}
	}
	return quantity.Sub(remaining), fills
}

// BuyLimit is a helper for filling an IOC buy limit order.
// Returns quantity filled and array of fills at each price level hit.
func (b *Book) BuyLimit(quantity, limitPrice decimal.Decimal) (decimal.Decimal, []Level) {
	var fills []Level
	remaining := quantity
	it := b.Asks.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		if price.Cmp(limitPrice) > 0 {
			break
		}
		fillSize := remaining
		if size.Cmp(fillSize) < 0 {
			fillSize = size
		}
		fills = append(fills, Level{Price: price, Size: fillSize})
		remaining = remaining.Sub(fillSize)
		newSize := size.Sub(fillSize)
		if newSize.IsZero() {
			b.Asks.Remove(price)
		} else {
			b.Asks.Put(price, newSize)
		}
		if remaining.IsZero() {
			break
		}
	}
	return quantity.Sub(remaining), fills
}

// SellLimit is a helper for filling an IOC sell limit order.
// Returns quantity filled and array of fills at each price level hit.
func (b *Book) SellLimit(quantity, limitPrice decimal.Decimal) (decimal.Decimal, []Level) {
	var fills []Level
	remaining := quantity
	it := b.Bids.Iterator()
	for it.Next() {
		price := it.Key()
		size := it.Value()
		if price.Cmp(limitPrice) < 0 {
			break
		}
		fillSize := remaining
		if size.Cmp(fillSize) < 0 {
			fillSize = size
		}
		fills = append(fills, Level{Price: price, Size: fillSize})
		remaining = remaining.Sub(fillSize)
		newSize := size.Sub(fillSize)
		if newSize.IsZero() {
			b.Bids.Remove(price)
		} else {
			b.Bids.Put(price, newSize)
		}
		if remaining.IsZero() {
			break
		}
	}
	return quantity.Sub(remaining), fills
}

// Pop removes up to maxQty from the best price on the opposite side.
// For buys, pops from Asks at or below limitPrice.
// For sells, pops from Bids at or above limitPrice.
// Returns the fill and true if successful, or zero and false if no matching liquidity.
func (b *Book) Pop(side Side, maxQty, limitPrice decimal.Decimal) (Level, bool) {
	var book *treemap.Map[decimal.Decimal, decimal.Decimal]
	switch side {
	case SideBuy:
		book = b.Asks
	case SideSell:
		book = b.Bids
	}
	it := book.Iterator()
	if !it.Next() {
		return Level{}, false
	}
	price := it.Key()
	size := it.Value()
	// buy: price must be <= limit; sell: price must be >= limit
	// using: price * side <= limitPrice * side
	if price.Mul(decimal.Decimal(side)).Cmp(limitPrice.Mul(decimal.Decimal(side))) > 0 {
		return Level{}, false
	}
	fillSize := maxQty.Min(size)
	newSize := size.Sub(fillSize)
	if newSize.IsZero() {
		book.Remove(price)
	} else {
		book.Put(price, newSize)
	}
	return Level{Price: price, Size: fillSize}, true
}

// Push adds liquidity back to the book on the given side.
func (b *Book) Push(side Side, price, size decimal.Decimal) {
	b.Add(side, price, size)
}
