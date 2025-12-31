package teddy

import (
	"dropbear/decimal"
	"sync"
)

type Holdings struct {
	lock          sync.RWMutex
	broker        *Broker
	holdingsMap   map[string]*Holding
	holdingsArray []*Holding
}

func newHoldings(broker *Broker) *Holdings {
	hs := &Holdings{
		broker:        broker,
		holdingsMap:   make(map[string]*Holding),
		holdingsArray: make([]*Holding, 0),
	}
	return hs
}

func (hs *Holdings) Lookup(symbol string) *Holding {
	hs.lock.RLock()
	ho := hs.holdingsMap[symbol]
	hs.lock.RUnlock()
	return ho
}

func (hs *Holdings) Get(symbol string) *Holding {
	ho := hs.Lookup(symbol)
	if ho == nil {
		hs.lock.Lock()
		ho = hs.holdingsMap[symbol]
		if ho == nil {
			ho = newHolding(hs.broker, symbol)
			hs.holdingsMap[symbol] = ho
			hs.holdingsArray = append(hs.holdingsArray, ho)
		}
		hs.lock.Unlock()
	}
	return ho
}

// All returns all Holding objects in insertion order.
func (hs *Holdings) All() []*Holding {
	hs.lock.RLock()
	defer hs.lock.RUnlock()
	result := make([]*Holding, 0, len(hs.holdingsArray))
	for _, holding := range hs.holdingsArray {
		result = append(result, holding)
	}
	return result
}

// GetEquityUSD returns the total equity of all holdings in USD on this broker.
func (hs *Holdings) GetEquityUSD() decimal.Decimal {
	total := decimal.Zero
	for _, holding := range hs.All() {
		price := holding.Broker.Pairs.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

// GetInvestedUSD returns the total invested (non-cash) equity of all holdings in USD on this broker.
func (hs *Holdings) GetInvestedUSD() decimal.Decimal {
	total := decimal.Zero
	for _, holding := range hs.All() {
		if holding.IsCash {
			continue
		}
		price := holding.Broker.Pairs.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}
