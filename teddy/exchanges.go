package teddy

import (
	"dropbear/ds"
	"sync"
)

type exchanges struct {
	lock          sync.RWMutex
	exchangeMap   map[ds.Exchange]*Exchange
	exchangeArray []*Exchange
}

var Exchanges = &exchanges{
	exchangeMap:   make(map[ds.Exchange]*Exchange),
	exchangeArray: make([]*Exchange, 0),
}

// Get returns the Exchange for the given exchange.
func (es *exchanges) Get(exchange ds.Exchange) *Exchange {
	es.lock.RLock()
	ex, ok := es.exchangeMap[exchange]
	es.lock.RUnlock()
	if !ok {
		es.lock.Lock()
		ex, ok = es.exchangeMap[exchange]
		if !ok {
			ex = newExchange(exchange)
			es.exchangeMap[exchange] = ex
			es.exchangeArray = append(es.exchangeArray, ex)
		}
		es.lock.Unlock()
	}
	return ex
}

// All returns all Exchange objects in insertion order.
func (es *exchanges) All() []*Exchange {
	es.lock.RLock()
	defer es.lock.RUnlock()
	result := make([]*Exchange, 0, len(es.exchangeArray))
	for _, exchange := range es.exchangeArray {
		result = append(result, exchange)
	}
	return result
}
