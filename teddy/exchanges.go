package teddy

import (
	"dropbear/ds"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type exchanges struct {
	OnReady       func()
	lock          sync.RWMutex
	exchangeMap   map[ds.Exchange]*Exchange
	exchangeArray []*Exchange
	unready       *treeset.Set[*Exchange]
}

var Exchanges = &exchanges{
	OnReady:       func() {},
	exchangeMap:   make(map[ds.Exchange]*Exchange),
	exchangeArray: make([]*Exchange, 0),
	unready:       treeset.NewWith(compareExchanges),
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
			es.unready.Add(ex)
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

func (es *exchanges) markReady(exchange *Exchange) {
	es.lock.Lock()
	es.unready.Remove(exchange)
	isReady := es.unready.Empty()
	es.lock.Unlock()
	if isReady {
		es.OnReady()
	}
}
