package clocky

import (
	"os"
	"time"
)

var TZ *time.Location

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
}
