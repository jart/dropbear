package osi

import "fmt"

// Humanize converts an OSI symbol string into a human-friendly format, e.g. "SPXW 2026-04-09 C 645".
func Humanize(osi string) string {
	sym, strike, class, year, month, day, err := Parse(osi)
	if err != nil {
		return osi
	}
	return fmt.Sprintf("%s %04d-%02d-%02d %c %s", sym, year, month, day, class, strike.String())
}
