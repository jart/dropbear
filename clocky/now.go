package clocky

import (
	"fmt"
	"time"
)

var (
	Now      = RealNow
	Sleep    = RealSleep
	mockTime Time
)

// RealNow returns the current time.
func RealNow() Time {
	return Time(time.Now().UnixNano())
}

// FakeNow returns the mocked current time for backtesting.
func FakeNow() Time {
	return mockTime.Load()
}

// SetNow mocks out Now for backtesting.
func SetNow(t Time) {
	for {
		old := mockTime.Load()
		if t != 0 && t < old {
			panic(fmt.Sprintf("cannot go back in time from %s to %s", old, t))
		}
		if mockTime.CompareAndSwap(old, t) {
			break
		}
	}
}

// Since returns duration elapsed since t.
func Since(t Time) Duration {
	return Now().Sub(t)
}

// RealSleep sleeps for the given duration.
func RealSleep(d Duration) {
	time.Sleep(time.Duration(d))
}

// NoSleep panics because sleeping is not allowed in backtest mode.
func FakeSleep(_ Duration) {
	panic("clocky.Sleep called in backtest mode")
}
