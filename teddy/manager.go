package teddy

import (
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/emirpasic/gods/v2/trees/binaryheap"
)

type managerEntry struct {
	data *recordedMarketData
	tick *ds.Tick
	pair *Pair
}

type manager struct {
	heap     *binaryheap.Heap[*managerEntry]
	sample   func(clocky.Time)
	lock     sync.Mutex
	report   *report
	ready    bool
	finished bool
}

func newManager(report *report) *manager {
	return &manager{
		heap:   binaryheap.NewWith(compareManagerEntries),
		sample: func(clocky.Time) {},
		report: report,
	}
}

func (m *manager) Close() {
	for !m.heap.Empty() {
		entry, _ := m.heap.Pop()
		entry.data.Close()
	}
	m.finished = false
}

func (m *manager) Register(pair *Pair) {
	data := openRecordedMarketData(*flagBacktest, pair.Broker.Broker, pair.Symbol)
	entry := &managerEntry{pair: pair, data: data, tick: &ds.Tick{}}
	err := data.Read(entry.tick)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
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
}

func (m *manager) Run() {
	m.lock.Lock()
	m.finished = true
	m.lock.Unlock()
	oldOnReady := Brokers.OnReady
	Brokers.OnReady = func() {
		if oldOnReady != nil {
			oldOnReady()
		}
		m.report.Init()
		m.sample = m.report.Sample
		m.ready = true
	}
	for !m.heap.Empty() {

		// globally order ticks by time
		entry, _ := m.heap.Pop()
		now := entry.tick.Time
		clocky.SetNow(now)

		// update data structures
		m.sample(now)
		entry.pair.handleTick(entry.tick)

		// dispatch to user
		if m.ready {
			entry.pair.OnTick(entry.tick)
		}

		// read next tick
		err := entry.data.Read(entry.tick)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				entry.data.Close()
				break // yes we really want the intersection of chosen datasets
			}
			panic("failed to read tick: " + err.Error())
		}
		m.heap.Push(entry)
	}
	if !m.ready {
		panic("the intersection of market data across all pairs is empty; cannot run backtest")
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
