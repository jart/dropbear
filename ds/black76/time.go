package black76

import "dropbear/clocky"

var (
	DaysPerYear = 365.25
	HoursPerDay = 24.
)

// Annualize converts a duration to the T parameter (fraction of a year).
func Annualize(d clocky.Duration) float64 {
	return float64(d) / (DaysPerYear * HoursPerDay * float64(clocky.Hour))
}
