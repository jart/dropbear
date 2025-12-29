package clocky

import (
	"fmt"
	"sync"
	"time"
)

var (
	mok    Time
	lok    sync.RWMutex
	isLive = true // set to false for backtesting
)

// Now returns current timestamp.
var Now = RealNow

// Now returns current timestamp (for live-trading only).
func RealNow() Time {
	return Time(time.Now().UnixMicro())
}

// FakeNow returns the mocked out Now time for backtesting.
func FakeNow() Time {
	lok.RLock()
	t := mok
	lok.RUnlock()
	return t
}

// SetNow mocks out Now for backtesting.
func SetNow(t Time) {
	lok.Lock()
	if !t.IsZero() && t < mok {
		panic(fmt.Sprintf("cannot go back in time from %s to %s", mok, t))
	}
	mok = t
	lok.Unlock()
}

// SetLive sets whether we're in live mode (true) or backtest mode (false).
// In backtest mode, Sleep becomes a no-op.
func SetLive(live bool) {
	isLive = live
}

// IsLive returns true if we're in live trading mode.
func IsLive() bool {
	return isLive
}

// Since returns duration elapsed since t.
func Since(t Time) Duration {
	return Now().Sub(t)
}

// Sleep pauses the current goroutine for the given duration.
// Panics if called in backtest mode (sleeping during backtest is a bug).
func Sleep(d Duration) {
	if !isLive {
		panic("clocky.Sleep called in backtest mode")
	}
	time.Sleep(time.Duration(d) * time.Microsecond)
}

// Ticker wraps time.Ticker for use with clocky.Duration.
type Ticker struct {
	C      <-chan time.Time
	ticker *time.Ticker
}

// NewTicker creates a new Ticker that sends on C at the given interval.
// In backtest mode, returns a Ticker with a nil channel (never fires).
func NewTicker(d Duration) *Ticker {
	if !isLive {
		return &Ticker{}
	}
	t := time.NewTicker(time.Duration(d) * time.Microsecond)
	return &Ticker{C: t.C, ticker: t}
}

// Stop turns off the ticker.
func (t *Ticker) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
}
