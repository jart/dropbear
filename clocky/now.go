package clocky

import (
	"fmt"
	"sync"
	"time"
)

var (
	mok Time
	lok sync.RWMutex
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

// Since returns duration elapsed since t.
func Since(t Time) Duration {
	return Now().Sub(t)
}
