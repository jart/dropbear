package ds

import "dropbear/decimal"

// Book is an order book using arena-allocated red-black trees.
// Hash maps provide O(1) lookup by price; trees provide sorted iteration.
type Book struct {
	arena  *bookArena
	bids   *bidTree
	asks   *askTree
	bidIdx map[decimal.Decimal]int32 // price -> arena index for O(1) lookup
	askIdx map[decimal.Decimal]int32
}

// NewBook creates a new order book with arena allocation.
func NewBook() *Book {
	arena := newBookArena(1024)
	return &Book{
		arena:  arena,
		bids:   newBidTree(arena),
		asks:   newAskTree(arena),
		bidIdx: make(map[decimal.Decimal]int32, 1024),
		askIdx: make(map[decimal.Decimal]int32, 1024),
	}
}

func (b *Book) BestBidAsk() (bid, ask decimal.Decimal) {
	if idx := b.bids.first(); idx != -1 {
		bid = b.arena.get(idx).price
	}
	if idx := b.asks.first(); idx != -1 {
		ask = b.arena.get(idx).price
	}
	return
}

func (b *Book) MidPrice() decimal.Decimal {
	bid, ask := b.BestBidAsk()
	return bid.Add(ask).DivInt(2)
}

func (b *Book) UpdateBid(price, size decimal.Decimal) {
	idx, ok := b.bidIdx[price] // O(1) lookup
	if size.IsPositive() {
		if ok {
			b.arena.get(idx).size = size
		} else {
			idx = b.bids.insert(price, size)
			b.bidIdx[price] = idx
		}
	} else if ok {
		b.bids.remove(idx)
		delete(b.bidIdx, price)
	}
}

func (b *Book) UpdateAsk(price, size decimal.Decimal) {
	idx, ok := b.askIdx[price] // O(1) lookup
	if size.IsPositive() {
		if ok {
			b.arena.get(idx).size = size
		} else {
			idx = b.asks.insert(price, size)
			b.askIdx[price] = idx
		}
	} else if ok {
		b.asks.remove(idx)
		delete(b.askIdx, price)
	}
}

func (b *Book) Update(side Side, price, size decimal.Decimal) {
	if side == SideBuy {
		b.UpdateBid(price, size)
	} else {
		b.UpdateAsk(price, size)
	}
}

func (b *Book) Clear() {
	b.arena.free = -1
	b.arena.len = 0
	b.arena.nodes = b.arena.nodes[:0]
	b.bids.root = -1
	b.asks.root = -1
	clear(b.bidIdx)
	clear(b.askIdx)
}

// PickBid returns bid price where cumulative size >= depth.
func (b *Book) PickBid(depth decimal.Decimal) decimal.Decimal {
	var p, d decimal.Decimal
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		p = n.price
		d = d.Add(n.size)
		if d >= depth {
			break
		}
	}
	return p
}

// PickAsk returns ask price where cumulative size >= depth.
func (b *Book) PickAsk(depth decimal.Decimal) decimal.Decimal {
	var p, d decimal.Decimal
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		p = n.price
		d = d.Add(n.size)
		if d >= depth {
			break
		}
	}
	return p
}

// PickBidByValue returns bid price where cumulative value >= valueDepth.
func (b *Book) PickBidByValue(valueDepth decimal.Decimal) decimal.Decimal {
	var p, v decimal.Decimal
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		p = n.price
		v = v.Add(p.Mul(n.size))
		if v >= valueDepth {
			break
		}
	}
	return p
}

// PickAskByValue returns ask price where cumulative value >= valueDepth.
func (b *Book) PickAskByValue(valueDepth decimal.Decimal) decimal.Decimal {
	var p, v decimal.Decimal
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		p = n.price
		v = v.Add(p.Mul(n.size))
		if v >= valueDepth {
			break
		}
	}
	return p
}

// MidPriceAtDepth returns mid price at a given cumulative value depth.
func (b *Book) MidPriceAtDepth(valueDepth decimal.Decimal) decimal.Decimal {
	bid := b.PickBidByValue(valueDepth)
	ask := b.PickAskByValue(valueDepth)
	if bid.IsZero() || ask.IsZero() {
		return decimal.Zero
	}
	return bid.Add(ask).DivInt(2)
}

// MarketBuyCost estimates total cost to buy baseQty.
func (b *Book) MarketBuyCost(baseQty decimal.Decimal) (cost decimal.Decimal, ok bool) {
	remaining := baseQty
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		if n.size >= remaining {
			return cost.Add(remaining.Mul(n.price)), true
		}
		cost = cost.Add(n.size.Mul(n.price))
		remaining = remaining.Sub(n.size)
	}
	return cost, false
}

// MarketSellProceeds estimates total proceeds from selling baseQty.
func (b *Book) MarketSellProceeds(baseQty decimal.Decimal) (proceeds decimal.Decimal, ok bool) {
	remaining := baseQty
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		if n.size >= remaining {
			return proceeds.Add(remaining.Mul(n.price)), true
		}
		proceeds = proceeds.Add(n.size.Mul(n.price))
		remaining = remaining.Sub(n.size)
	}
	return proceeds, false
}

// MarketBuyCostLimited estimates cost to buy baseQty up to limitPrice.
func (b *Book) MarketBuyCostLimited(baseQty, limitPrice decimal.Decimal) (cost decimal.Decimal, ok bool) {
	remaining := baseQty
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		if n.price > limitPrice {
			break
		}
		if n.size >= remaining {
			return cost.Add(remaining.Mul(n.price)), true
		}
		cost = cost.Add(n.size.Mul(n.price))
		remaining = remaining.Sub(n.size)
	}
	return cost, false
}

// MarketSellProceedsLimited estimates proceeds down to limitPrice.
func (b *Book) MarketSellProceedsLimited(baseQty, limitPrice decimal.Decimal) (proceeds decimal.Decimal, ok bool) {
	remaining := baseQty
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		if n.price < limitPrice {
			break
		}
		if n.size >= remaining {
			return proceeds.Add(remaining.Mul(n.price)), true
		}
		proceeds = proceeds.Add(n.size.Mul(n.price))
		remaining = remaining.Sub(n.size)
	}
	return proceeds, false
}

// BidDepthAbove returns cumulative bid volume at prices strictly above price.
func (b *Book) BidDepthAbove(price decimal.Decimal) decimal.Decimal {
	if price.IsZero() {
		return decimal.Zero
	}
	var depth decimal.Decimal
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		if n.price <= price {
			break
		}
		depth = depth.Add(n.size)
	}
	return depth
}

// AskDepthBelow returns cumulative ask volume at prices strictly below price.
func (b *Book) AskDepthBelow(price decimal.Decimal) decimal.Decimal {
	if price.IsZero() {
		return decimal.Zero
	}
	var depth decimal.Decimal
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		if n.price >= price {
			break
		}
		depth = depth.Add(n.size)
	}
	return depth
}

// Add includes limit order in order book (adds to existing size).
func (b *Book) Add(side Side, price, size decimal.Decimal) {
	if side == SideBuy {
		idx, ok := b.bidIdx[price]
		if ok {
			b.arena.get(idx).size = b.arena.get(idx).size.Add(size)
		} else {
			idx = b.bids.insert(price, size)
			b.bidIdx[price] = idx
		}
	} else {
		idx, ok := b.askIdx[price]
		if ok {
			b.arena.get(idx).size = b.arena.get(idx).size.Add(size)
		} else {
			idx = b.asks.insert(price, size)
			b.askIdx[price] = idx
		}
	}
}

// Pop removes up to maxQty from the best price on the opposite side.
// For buys, pops from Asks at or below limitPrice.
// For sells, pops from Bids at or above limitPrice.
func (b *Book) Pop(side Side, maxQty, limitPrice decimal.Decimal) (Level, bool) {
	if side == SideBuy {
		idx := b.asks.first()
		if idx == -1 {
			return Level{}, false
		}
		n := b.arena.get(idx)
		if n.price > limitPrice {
			return Level{}, false
		}
		fillSize := min(n.size, maxQty)
		newSize := n.size.Sub(fillSize)
		price := n.price
		if newSize.IsZero() {
			b.asks.remove(idx)
			delete(b.askIdx, price)
		} else {
			n.size = newSize
		}
		return Level{Price: price, Size: fillSize}, true
	} else {
		idx := b.bids.first()
		if idx == -1 {
			return Level{}, false
		}
		n := b.arena.get(idx)
		if n.price < limitPrice {
			return Level{}, false
		}
		fillSize := min(n.size, maxQty)
		newSize := n.size.Sub(fillSize)
		price := n.price
		if newSize.IsZero() {
			b.bids.remove(idx)
			delete(b.bidIdx, price)
		} else {
			n.size = newSize
		}
		return Level{Price: price, Size: fillSize}, true
	}
}

// Push adds liquidity back to the book on the given side.
func (b *Book) Push(side Side, price, size decimal.Decimal) {
	b.Add(side, price, size)
}

// EachBid iterates bids from best (highest) to worst.
// Returns false from fn to stop iteration.
func (b *Book) EachBid(fn func(price, size decimal.Decimal) bool) {
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		if !fn(n.price, n.size) {
			break
		}
	}
}

// EachAsk iterates asks from best (lowest) to worst.
// Returns false from fn to stop iteration.
func (b *Book) EachAsk(fn func(price, size decimal.Decimal) bool) {
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		if !fn(n.price, n.size) {
			break
		}
	}
}

// FindFattestBidBelow walks down from startPrice through the bid book,
// accumulates depth until minDepth is reached, then returns the price
// and size of the largest single bid in that interval. Used for stink
// bid optimization - place your bid just above the fattest bid in range.
func (b *Book) FindFattestBidBelow(startPrice, minDepth decimal.Decimal) (fatPrice, fatSize decimal.Decimal) {
	var depth decimal.Decimal
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		// skip bids at or above our start price (we can only outbid walls BELOW)
		if n.price >= startPrice {
			continue
		}
		// track the fattest bid we've seen
		if n.size.Cmp(fatSize) > 0 {
			fatPrice = n.price
			fatSize = n.size
		}
		// accumulate depth
		depth = depth.Add(n.size)
		if depth.Cmp(minDepth) >= 0 {
			break
		}
	}
	return
}
