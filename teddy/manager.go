package teddy

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/emirpasic/gods/v2/sets/treeset"
	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

type managerEntry struct {
	data         *recordedMarketData
	tick         *ds.Tick
	pair         *Pair
	hasTradeData bool
	hasL2Data    bool
}

type manager struct {
	heap     *binaryheap.Heap[*managerEntry]
	unready  *treeset.Set[*managerEntry]
	lock     sync.Mutex
	finished bool
}

func newManager() *manager {
	return &manager{
		heap:    binaryheap.NewWith(compareManagerEntries),
		unready: treeset.NewWith(compareManagerEntriesByPtr),
	}
}

func (m *manager) Close() {
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		entry.data.Close()
	}
	m.unready.Clear()
	m.finished = false
}

func (m *manager) Register(pair *Pair) {
	data := openRecordedMarketData(*flagBacktest, pair.Exchange.Exchange, pair.Symbol)
	entry := &managerEntry{pair: pair, data: data, tick: &ds.Tick{}}
	err := data.Read(entry.tick)
	if err != nil {
		if err == io.EOF {
			data.Close()
			return
		}
		loggy.Fatalf("failed to read initial tick: %v", err)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.finished {
		panic(fmt.Sprintf("cannot register %v pair after market data manager has started running", pair))
	}
	m.heap.Push(entry)
	m.unready.Add(entry)
}

func (m *manager) Run() {
	m.lock.Lock()
	m.finished = true
	m.lock.Unlock()
	isReady := false
	for !m.heap.Empty() {

		// globally order ticks by time
		entry, _ := m.heap.Pop()
		now := entry.tick.Time
		clocky.SetNow(now)
		gRateLimiter.Pulse(now)

		// wait all pairs have received some data
		entry.pair.handleTick(entry.tick)
		if !isReady {
			if len(entry.tick.Trades) > 0 {
				entry.hasTradeData = true
			}
			if entry.tick.Bids != nil || entry.tick.Asks != nil {
				entry.hasL2Data = true
			}
			if entry.hasTradeData && entry.hasL2Data && m.unready.Contains(entry) {
				m.unready.Remove(entry)
				if m.unready.Empty() {
					for _, exchange := range Exchanges.All() {
						for _, holding := range exchange.Holdings.All() {
							holding.Lock.RLock()
							quantity := holding.Quantity
							holding.Lock.RUnlock()
							if holding.IsCash {
								// convert cash to benchmark asset
								gBenchmarkQty = gBenchmarkQty.Add(quantity.Div(gBenchmark.LastPrice))
							} else if holding.Symbol == gBenchmark.BaseCurrency {
								// include existing holdings of benchmark asset
								gBenchmarkQty = gBenchmarkQty.Add(quantity)
							}
						}
					}
					gReport.startTime = now
					gReport.startEquity = GetEquityUSD()
					isReady = true
				}
			}
		}

		// track strategy performance versus hodling
		if isReady && gStrategyEquity.ShouldSample(now) {
			gStrategyEquity.Sample(now, GetEquityUSD())
			gBenchmarkEquity.Sample(now, gBenchmarkQty.Mul(gBenchmark.LastPrice))
			gStrategyInvested.Sample(GetInvestedUSD().Float64())
		}

		// dispatch to user
		if isReady {
			gReport.endTime = now
			entry.pair.OnTick(entry.tick)
		}

		// read next tick
		err := entry.data.Read(entry.tick)
		if err != nil {
			if err == io.EOF {
				entry.data.Close()
				break
			}
			loggy.Fatalf("failed to read tick: %v", err)
		}
		m.heap.Push(entry)
	}
	if !isReady {
		loggy.Fatalf("the intersection of market data across all pairs is empty; cannot run backtest")
	}
}

func compareManagerEntries(a, b *managerEntry) int {
	if a.tick.Time.Before(b.tick.Time) {
		return -1
	}
	if a.tick.Time.After(b.tick.Time) {
		return +1
	}
	return 0
}

func compareManagerEntriesByPtr(a, b *managerEntry) int {
	pa, pb := uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b))
	if pa < pb {
		return -1
	}
	if pa > pb {
		return +1
	}
	return 0
}
