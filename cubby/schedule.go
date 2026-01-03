package cubby

import (
	"dropbear/clocky"
	"time"
)

// Schedule holds callbacks for market events.
// Events are generated at startup and inserted into the priority heap.
type Schedule struct {
	beforeOpen       []func() // called before market opens (6:30 AM PT)
	afterOpen        []func() // called after market opens
	beforeCloseEarly []func() // called 10 min before close (12:50 PT) - ends day trading time
	beforeClose      []func() // called before market closes (1:00 PM PT)
	afterClose       []func() // called after market closes
}

// US equity market hours in Pacific Time
const (
	MarketOpenHour    = 6
	MarketOpenMinute  = 30
	MarketCloseHour   = 13
	MarketCloseMinute = 0
)

// Pacific is the cached America/Los_Angeles timezone (loaded once at startup).
var Pacific *time.Location

func init() {
	var err error
	Pacific, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("failed to load America/Los_Angeles timezone: " + err.Error())
	}
}

// ToPacific converts a clocky.Time to time.Time in Pacific timezone.
func ToPacific(ts clocky.Time) time.Time {
	return time.UnixMicro(int64(ts)).In(Pacific)
}

var gSchedule = &Schedule{}

// BeforeOpen registers a callback to run before market open.
func BeforeOpen(fn func()) {
	gSchedule.beforeOpen = append(gSchedule.beforeOpen, fn)
}

// AfterOpen registers a callback to run after market open.
func AfterOpen(fn func()) {
	gSchedule.afterOpen = append(gSchedule.afterOpen, fn)
}

// BeforeCloseEarly registers a callback to run 10 min before market close (15:50 ET).
// This is when day trading time ends and only 2x leverage is available.
func BeforeCloseEarly(fn func()) {
	gSchedule.beforeCloseEarly = append(gSchedule.beforeCloseEarly, fn)
}

// BeforeClose registers a callback to run before market close.
func BeforeClose(fn func()) {
	gSchedule.beforeClose = append(gSchedule.beforeClose, fn)
}

// AfterClose registers a callback to run after market close.
func AfterClose(fn func()) {
	gSchedule.afterClose = append(gSchedule.afterClose, fn)
}

// IsMarketOpen returns true if the market is currently open.
func IsMarketOpen() bool {
	t := ToPacific(clocky.Now())
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	timeOfDay := t.Hour()*60 + t.Minute()
	marketOpen := MarketOpenHour*60 + MarketOpenMinute
	marketClose := MarketCloseHour*60 + MarketCloseMinute
	return timeOfDay >= marketOpen && timeOfDay < marketClose
}

// IsMarketOpenCandle returns true if the given timestamp is the market open candle (6:30 AM PT).
func IsMarketOpenCandle(ts clocky.Time) bool {
	t := ToPacific(ts)
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	return t.Hour() == MarketOpenHour && t.Minute() == MarketOpenMinute
}

// IsMarketCloseCandle returns true if the given timestamp is the last candle before market close.
// For 1-minute candles, this is the 12:59 candle (which closes at 13:00 PT).
func IsMarketCloseCandle(ts clocky.Time) bool {
	t := ToPacific(ts)
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	return t.Hour() == MarketCloseHour-1 && t.Minute() == 59
}
