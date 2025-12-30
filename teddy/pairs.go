package teddy

import (
	"dropbear/decimal"
	"dropbear/loggy"
	"log"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Pairs struct {
	lock       sync.RWMutex
	broker     *Broker
	pairsMap   map[string]*Pair
	unready    *treeset.Set[*Pair]
	pairsArray []*Pair
}

func newPairs(broker *Broker) *Pairs {
	return &Pairs{
		broker:     broker,
		pairsMap:   make(map[string]*Pair),
		pairsArray: make([]*Pair, 0),
		unready:    treeset.NewWith(comparePairs),
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
		ps.lock.Lock()
		pair = ps.pairsMap[productID]
		if pair == nil {
			pair = newPair(ps.broker, productID)
			ps.unready.Add(pair)
			ps.pairsMap[productID] = pair
			ps.pairsArray = append(ps.pairsArray, pair)
		}
		ps.lock.Unlock()
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

// GetPriceUSD returns the price of the given symbol in USD on this broker.
func (ps *Pairs) GetPriceUSD(symbol string) decimal.Decimal {
	switch symbol {
	case "USD", "USDT", "USDC", "FDUSD":
		return decimal.One
	}
	ps.lock.RLock()
	pair := ps.pairsMap[symbol]
	if pair == nil {
		pair = ps.pairsMap[symbol+"-USD"]
		if pair == nil {
			pair = ps.pairsMap[symbol+"-USDT"]
			if pair == nil {
				pair = ps.pairsMap[symbol+"-USDC"]
				if pair == nil {
					pair = ps.pairsMap[symbol+"USD"]
					if pair == nil {
						pair = ps.pairsMap[symbol+"USDT"]
						if pair == nil {
							pair = ps.pairsMap[symbol+"USDC"]
							if pair == nil {
								pair = ps.pairsMap[symbol+"FDUSD"]
								if pair == nil {
									loggy.Fatalf("don't know to determine USD value of %s on %s", symbol, ps.broker)
								}
							}
						}
					}
				}
			}
		}
	}
	ps.lock.RUnlock()
	return pair.LastPrice.Load()
}

func (ps *Pairs) markReady(pair *Pair) {
	ps.lock.Lock()
	ps.unready.Remove(pair)
	isReady := ps.unready.Empty()
	ps.lock.Unlock()
	if isReady {
		if *flagVerbose {
			log.Printf("[teddy] %s broker ready", ps.broker)
		}
		ps.broker.OnReady()
		Brokers.markReady(ps.broker)
	}
}
