package teddy

import (
	"dropbear/ds"
	"log"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type brokers struct {
	OnReady     func()
	lock        sync.RWMutex
	brokerMap   map[ds.Broker]*Broker
	brokerArray []*Broker
	unready     *treeset.Set[*Broker]
}

var Brokers = &brokers{
	OnReady:     func() {},
	brokerMap:   make(map[ds.Broker]*Broker),
	brokerArray: make([]*Broker, 0),
	unready:     treeset.NewWith(compareBrokers),
}

// Get returns the Broker for the given broker.
func (es *brokers) Get(broker ds.Broker) *Broker {
	es.lock.RLock()
	ex, ok := es.brokerMap[broker]
	es.lock.RUnlock()
	if !ok {
		es.lock.Lock()
		ex, ok = es.brokerMap[broker]
		if !ok {
			ex = newBroker(broker)
			es.unready.Add(ex)
			es.brokerMap[broker] = ex
			es.brokerArray = append(es.brokerArray, ex)
		}
		es.lock.Unlock()
	}
	return ex
}

// All returns all Broker objects in insertion order.
func (es *brokers) All() []*Broker {
	es.lock.RLock()
	defer es.lock.RUnlock()
	result := make([]*Broker, 0, len(es.brokerArray))
	for _, broker := range es.brokerArray {
		result = append(result, broker)
	}
	return result
}

func (es *brokers) markReady(broker *Broker) {
	es.lock.Lock()
	es.unready.Remove(broker)
	isReady := es.unready.Empty()
	es.lock.Unlock()
	if isReady {
		if *flagVerbose {
			log.Printf("[teddy] all brokers ready")
		}
		es.OnReady()
	}
}
