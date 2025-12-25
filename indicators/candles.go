package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
)

// Candle represents a single OHLC candlestick.
type Candle struct {
	Start  clocky.Time     // microsecond unix timestamp
	Open   decimal.Decimal // first price in period
	High   decimal.Decimal // highest price in period
	Low    decimal.Decimal // lowest price in period
	Close  decimal.Decimal // last price in period
	Volume decimal.Decimal // volume in period
}

// Candles is a ring buffer of OHLC candlesticks.
type Candles struct {
	Period  clocky.Duration // candle period in microseconds
	End     clocky.Time     // timestamp of last time Add() was called
	Candles []Candle        // ring buffer of candles
	Index   int             // current index in ring buffer
	Count   int             // number of populated candles
	OnClose func(c *Candle) // called when candle closes
}

// NewCandles creates a new Candles ring buffer with the given period and capacity.
// The onClose callback is invoked each time a candle closes (may be called multiple
// times per Add() if there are gaps in the data).
func NewCandles(period clocky.Duration, capacity int, onClose func(c *Candle)) *Candles {
	return &Candles{
		Period:  period,
		Candles: make([]Candle, capacity),
		OnClose: onClose,
	}
}

func (cs *Candles) Seed(candle Candle) {
	if cs.Count > 0 {
		cs.Index++
		if cs.Index == len(cs.Candles) {
			cs.Index = 0
		}
	}
	cs.End = candle.Start.Add(cs.Period)
	cs.Candles[cs.Index] = candle
	if cs.OnClose != nil {
		cs.OnClose(&cs.Candles[cs.Index])
	}
	cs.Count++
	if cs.Count > len(cs.Candles) {
		cs.Count = len(cs.Candles)
	}
}

// Add adds a price tick at the given timestamp.
// Your timestamp must be monotonically increasing.
func (cs *Candles) Add(timestamp clocky.Time, price decimal.Decimal, size decimal.Decimal) {

	// check arguments
	if timestamp < cs.End {
		loggy.Fatalf("indicators: timestamp went backwards: %d -> %d", cs.End, timestamp)
	}

	// initialize first candle
	if cs.Count == 0 {
		cs.End = timestamp
		cs.Candles[0] = Candle{
			Start:  timestamp,
			Open:   price,
			High:   price,
			Low:    price,
			Close:  price,
			Volume: size,
		}
		if cs.OnClose != nil {
			cs.OnClose(&cs.Candles[0])
		}
		cs.Count = 1
		return
	}

	// backfill candles
	var c *Candle
	for {
		c = &cs.Candles[cs.Index]
		if timestamp < c.Start.Add(cs.Period) {
			break
		}
		// current candle just closed
		if cs.OnClose != nil {
			cs.OnClose(c)
		}
		// start new candle
		cs.Index++
		if cs.Index == len(cs.Candles) {
			cs.Index = 0
		} else if cs.Index == cs.Count {
			cs.Count++
			cs.Candles[cs.Index] = Candle{}
		}
		d := &cs.Candles[cs.Index]
		d.Start = c.Start.Add(cs.Period)
		d.Open = c.Close
		d.High = c.Close
		d.Low = c.Close
		d.Close = c.Close
		d.Volume = decimal.Zero
	}

	// update candle
	c.Volume = c.Volume.Add(size)
	c.High = price.Max(c.High)
	c.Low = price.Min(c.Low)
	c.Close = price
}

// Get returns the candle at position i.
// For example, 0 is current candle, 1 is previous candle, etc.
func (cs *Candles) Get(i int) *Candle {
	if i < 0 || i >= cs.Count {
		loggy.Fatalf("indicators: candle index out of range: %d", i)
	}
	idx := cs.Index - i
	if idx < 0 {
		idx += len(cs.Candles)
	}
	return &cs.Candles[idx]
}
