package cubby

import (
	"dropbear/clocky"
	"time"
)

// Schedule holds callbacks for market events.
type Schedule struct {
	beforeOpen  []func() // called before market opens (9:30 AM ET)
	afterOpen   []func() // called after market opens
	beforeClose []func() // called before market closes (4:00 PM ET)
	afterClose  []func() // called after market closes
	lastDate    string
	firedOpen   bool
	firedClose  bool
	nyLoc       *time.Location
}

// US equity market hours in Eastern Time
const (
	MarketOpenHour    = 9
	MarketOpenMinute  = 30
	MarketCloseHour   = 16
	MarketCloseMinute = 0
)

var gSchedule = &Schedule{}

func init() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("failed to load America/New_York timezone")
	}
	gSchedule.nyLoc = loc
}

// BeforeOpen registers a callback to run before market open.
func BeforeOpen(fn func()) {
	gSchedule.beforeOpen = append(gSchedule.beforeOpen, fn)
}

// AfterOpen registers a callback to run after market open.
func AfterOpen(fn func()) {
	gSchedule.afterOpen = append(gSchedule.afterOpen, fn)
}

// BeforeClose registers a callback to run before market close.
func BeforeClose(fn func()) {
	gSchedule.beforeClose = append(gSchedule.beforeClose, fn)
}

// AfterClose registers a callback to run after market close.
func AfterClose(fn func()) {
	gSchedule.afterClose = append(gSchedule.afterClose, fn)
}

// checkSchedule fires scheduled callbacks based on current time.
// Called by the manager for each candle.
func checkSchedule(now clocky.Time) {
	s := gSchedule
	t := time.UnixMicro(int64(now)).In(s.nyLoc)
	date := t.Format("2006-01-02")
	hour, min := t.Hour(), t.Minute()
	timeOfDay := hour*60 + min

	marketOpen := MarketOpenHour*60 + MarketOpenMinute    // 9:30 = 570
	marketClose := MarketCloseHour*60 + MarketCloseMinute // 16:00 = 960

	// New trading day
	if date != s.lastDate {
		s.lastDate = date
		s.firedOpen = false
		s.firedClose = false
	}

	// Before/After Open (fire once per day when we cross 9:30)
	if !s.firedOpen && timeOfDay >= marketOpen {
		for _, fn := range s.beforeOpen {
			fn()
		}
		s.firedOpen = true
		for _, fn := range s.afterOpen {
			fn()
		}
	}

	// Before/After Close (fire once per day when we cross 16:00)
	if !s.firedClose && timeOfDay >= marketClose {
		for _, fn := range s.beforeClose {
			fn()
		}
		s.firedClose = true
		for _, fn := range s.afterClose {
			fn()
		}
	}
}

// IsMarketOpen returns true if the market is currently open.
func IsMarketOpen() bool {
	now := clocky.Now()
	t := time.UnixMicro(int64(now)).In(gSchedule.nyLoc)
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	hour, min := t.Hour(), t.Minute()
	timeOfDay := hour*60 + min
	marketOpen := MarketOpenHour*60 + MarketOpenMinute
	marketClose := MarketCloseHour*60 + MarketCloseMinute
	return timeOfDay >= marketOpen && timeOfDay < marketClose
}
