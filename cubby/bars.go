package cubby

import "dropbear/broker/alpaca"

// Bars is an alias for alpaca.Bars for backwards compatibility.
type Bars = alpaca.Bars

// OpenBars opens an equitydata file for reading.
func OpenBars(path string) (*Bars, error) {
	return alpaca.OpenBars(path)
}
