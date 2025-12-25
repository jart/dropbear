package indicators

import "dropbear/decimal"

// Last holds the last added value.
type Last struct {
	Value     decimal.Decimal
	hasSample bool
}

// NewLast creates an indicator that holds the last added value.
func NewLast() *Last {
	return &Last{}
}

// IsReady returns true if Add() has been called at least once.
func (n *Last) IsReady() bool {
	return n.hasSample
}

// Add adds a simple value to the indicator.
func (n *Last) Add(value decimal.Decimal) {
	n.Value = value
	n.hasSample = true
}
