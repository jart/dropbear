package clocky

import (
	"os"
	"time"
)

var TZ *time.Location
var NYC *time.Location
var MTV *time.Location

func init() {
	s := os.Getenv("TZ")
	if s == "" {
		s = "America/Los_Angeles"
	}
	tz, err := time.LoadLocation(s)
	if err != nil {
		panic("bad TZ variable: " + err.Error())
	}
	TZ = tz

	// new york financial time
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("failed to load America/New_York: " + err.Error())
	}
	NYC = nyc

	// google standard time
	mtv, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("failed to load America/Los_Angeles: " + err.Error())
	}
	MTV = mtv
}
