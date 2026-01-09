package clocky

import (
	"os"
	"time"
)

var TZ *time.Location
var NYC *time.Location

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

	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic("failed to load America/New_York: " + err.Error())
	}
	NYC = nyc
}
