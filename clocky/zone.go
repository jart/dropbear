package clocky

import (
	"time"
)

var TZ *time.Location
var NYC *time.Location
var MTV *time.Location
var UTC *time.Location

func init() {
	UTC = time.UTC

	// new york financial time
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("failed to load America/New_York: " + err.Error())
	}
	TZ = nyc
	NYC = nyc

	// google standard time
	mtv, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("failed to load America/Los_Angeles: " + err.Error())
	}
	MTV = mtv
}

func (t Time) In(loc *time.Location) time.Time {
	return t.goTime().In(loc)
}
