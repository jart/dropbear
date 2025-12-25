package ds

import (
	"cmp"
	"dropbear/decimal"
	"sync"
	"time"

	"github.com/emirpasic/gods/v2/maps/treemap"
)

// Book represents an order book for a single product.
// Uses tree maps for O(log n) updates and O(1) best bid/ask.
type Book struct {
	Lock       sync.RWMutex
	updated    chan struct{} // closed on each update, then recreated
	ready      chan struct{} // closed once book has data
	readyOnce  sync.Once
	bids       *treemap.Map[decimal.Decimal, decimal.Decimal] // price -> size, descending (best bid = first)
	asks       *treemap.Map[decimal.Decimal, decimal.Decimal] // price -> size, ascending (best ask = first)
	lastUpdate time.Time                                      // system time of last websocket message
}

// NewBook creates a new order book.
func NewBook() *Book {
	return &Book{
		updated: make(chan struct{}),
		ready:   make(chan struct{}),
		bids:    treemap.NewWith[decimal.Decimal, decimal.Decimal](reverseDecimalComparator),
		asks:    treemap.NewWith[decimal.Decimal, decimal.Decimal](cmp.Compare[decimal.Decimal]),
	}
}

// Ready returns a channel that is closed once the book has received data.
func (b *Book) Ready() <-chan struct{} {
	return b.ready
}

// Wait blocks until the book is updated.
func (b *Book) Wait() {
	<-b.Updated()
}

// Updated returns a channel that will be closed when the book is updated.
// Use this with select for cancellable waits.
func (b *Book) Updated() <-chan struct{} {
	b.Lock.RLock()
	ch := b.updated
	b.Lock.RUnlock()
	return ch
}

// Staleness returns how long since the last websocket update.
func (b *Book) Staleness() time.Duration {
	b.Lock.RLock()
	t := b.lastUpdate
	b.Lock.RUnlock()
	if t.IsZero() {
		return 0
	}
	return time.Since(t)
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	bid, _, _ = b.bids.Min()
	ask, _, _ = b.asks.Min()
	return
}

// UpdateBid updates or removes a bid level.
// The write lock must be held by the caller.
func (b *Book) UpdateBid(price, size decimal.Decimal) {
	b.lastUpdate = time.Now()
	if size.IsPositive() {
		b.bids.Put(price, size)
	} else {
		b.bids.Remove(price)
	}
}

// UpdateAsk updates or removes an ask level.
// The write lock must be held by the caller.
func (b *Book) UpdateAsk(price, size decimal.Decimal) {
	if size.IsPositive() {
		b.asks.Put(price, size)
	} else {
		b.asks.Remove(price)
	}
}

// Broadcast signals all waiters that the book has changed.
// The write lock must be held by the caller.
func (b *Book) Broadcast() {
	if !b.bids.Empty() && !b.asks.Empty() {
		b.readyOnce.Do(func() { close(b.ready) })
		close(b.updated) // ready steady go
		b.updated = make(chan struct{})
	}
}

// Clear removes all levels from the book.
// The write lock must be held by the caller.
func (b *Book) Clear() {
	b.bids.Clear()
	b.asks.Clear()
}

// Add includes limit order in order book.
func (b *Book) Add(side Side, price, size decimal.Decimal) {
	b.Lock.Lock()
	defer b.Lock.Unlock()
	book := b.asks
	if side == SideBuy {
		book = b.bids
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var depth decimal.Decimal
	it := b.bids.Iterator()
	for it.Next() {
		p := it.Key()
		if p.Cmp(price) <= 0 {
			break // bids are descending, so we're done
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var depth decimal.Decimal
	it := b.asks.Iterator()
	for it.Next() {
		p := it.Key()
		if p.Cmp(price) >= 0 {
			break // asks are ascending, so we're done
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	remaining := baseQty
	it := b.asks.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	remaining := baseQty
	it := b.bids.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	remaining := baseQty
	it := b.asks.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	remaining := baseQty
	it := b.bids.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var levels []Level
	it := b.bids.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var levels []Level
	it := b.asks.Iterator()
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
// Since crypto lets you make $1 trades, there's no point in us doing that if the existing asks
// at price $10.00, $10.01, and $10.02 sum up to fifty dollars.
func (b *Book) PickAsk(depth decimal.Decimal) decimal.Decimal {
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var p, d decimal.Decimal
	it := b.asks.Iterator()
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
// Since crypto lets you make $1 trades, there's no point in us doing that if the existing bids
// at price $10.00, $9.99, and $9.98 sum up to fifty dollars.
func (b *Book) PickBid(depth decimal.Decimal) decimal.Decimal {
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var p, d decimal.Decimal
	it := b.bids.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var p, v decimal.Decimal
	it := b.asks.Iterator()
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
	b.Lock.RLock()
	defer b.Lock.RUnlock()
	var p, v decimal.Decimal
	it := b.bids.Iterator()
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
	b.Lock.Lock()
	defer b.Lock.Unlock()
	var fills []Level
	remaining := quantity
	var spent decimal.Decimal
	it := b.asks.Iterator()
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
			b.asks.Remove(price)
		} else {
			b.asks.Put(price, newSize)
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
	b.Lock.Lock()
	defer b.Lock.Unlock()
	var fills []Level
	remaining := quantity
	it := b.bids.Iterator()
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
			b.bids.Remove(price)
		} else {
			b.bids.Put(price, newSize)
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
	b.Lock.Lock()
	defer b.Lock.Unlock()
	var fills []Level
	remaining := quantity
	it := b.asks.Iterator()
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
			b.asks.Remove(price)
		} else {
			b.asks.Put(price, newSize)
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
	b.Lock.Lock()
	defer b.Lock.Unlock()
	var fills []Level
	remaining := quantity
	it := b.bids.Iterator()
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
			b.bids.Remove(price)
		} else {
			b.bids.Put(price, newSize)
		}
		if remaining.IsZero() {
			break
		}
	}
	return quantity.Sub(remaining), fills
}

// reverseDecimalComparator for descending order (bids)
func reverseDecimalComparator(a, b decimal.Decimal) int {
	return -cmp.Compare(a, b)
}
