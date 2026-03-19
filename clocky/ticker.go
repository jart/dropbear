package clocky

import "time"

// NewTicker is the function used to create new Tickers.
var NewTicker = RealNewTicker

// RealNewTicker creates a new Ticker that sends on C at the given interval.
func RealNewTicker(d Duration) *Ticker {
	t := time.NewTicker(time.Duration(d))
	return &Ticker{C: t.C, ticker: t}
}

// FakeNewTicker creates a new Ticker that never fires.
func FakeNewTicker(d Duration) *Ticker {
	return &Ticker{}
}

// Ticker wraps time.Ticker for use with clocky.Duration.
type Ticker struct {
	C      <-chan time.Time
	ticker *time.Ticker
}

// Stop turns off the ticker.
func (t *Ticker) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
}
