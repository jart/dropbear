package teddy

import (
	"sync"
)

type Pairs struct {
	lock       sync.RWMutex
	exchange   *Exchange
	pairsMap   map[string]*Pair
	pairsArray []*Pair
}

func newPairs(exchange *Exchange) *Pairs {
	return &Pairs{
		exchange:   exchange,
		pairsMap:   make(map[string]*Pair),
		pairsArray: make([]*Pair, 0),
	}
}

// Lookup returns the Pair for a given symbol, e.g. BTC-USD, or nil if not found.
func (ps *Pairs) Lookup(productID string) *Pair {
	ps.lock.RLock()
	pair := ps.pairsMap[productID]
	ps.lock.RUnlock()
	return pair
}

// Get returns the Pair for the given symbol, e.g. BTC-USD.
// Unlike Lookup, this creates Pair if it doesn't already exist.
func (ps *Pairs) Get(productID string) *Pair {
	pair := ps.Lookup(productID)
	if pair == nil {
		new := false
		ps.lock.Lock()
		pair = ps.pairsMap[productID]
		if pair == nil {
			new = true
			pair = newPair(ps.exchange, productID)
			ps.pairsMap[productID] = pair
			ps.pairsArray = append(ps.pairsArray, pair)
		}
		ps.lock.Unlock()
		if new {
			pair.init()
		}
	}
	return pair
}

// All returns all Pair objects in insertion order.
func (ps *Pairs) All() []*Pair {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	result := make([]*Pair, 0, len(ps.pairsArray))
	for _, pair := range ps.pairsArray {
		result = append(result, pair)
	}
	return result
}
