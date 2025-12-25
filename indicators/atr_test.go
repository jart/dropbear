package indicators

import (
	"dropbear/decimal"
	"testing"
)

// Test data from QuantConnect's spy_atr_wilder.txt
// https://github.com/QuantConnect/Lean/blob/master/Tests/TestData/spy_atr_wilder.txt
var spyData = []struct {
	open, high, low, close string
	atr                    string // empty until period is reached
}{
	{"146.77", "147.28", "146.61", "147.05", ""},
	{"147.7", "148.42", "147.43", "148", ""},
	{"147.97", "148.49", "147.43", "148.33", ""},
	{"148.33", "149.13", "147.98", "149.13", ""},
	{"149.13", "149.5", "148.86", "149.37", ""},
	{"149.15", "150.14", "149.01", "149.41", ""},
	{"149.88", "150.25", "149.37", "150.25", ""},
	{"150.29", "150.33", "149.51", "150.07", ""},
	{"149.77", "150.85", "149.67", "150.66", ""},
	{"150.64", "150.94", "149.93", "150.07", ""},
	{"149.89", "150.38", "149.6", "149.7", ""},
	{"150.65", "151.42", "150.39", "151.24", ""},
	{"150.32", "150.58", "149.43", "149.54", ""},
	{"150.35", "151.48", "150.29", "151.05", "1.15429"},
	{"150.52", "151.26", "150.41", "151.16", "1.13255"},
	{"151.21", "151.35", "149.86", "150.96", "1.15808"},
	{"151.23", "151.89", "151.22", "151.8", "1.14179"},
	{"151.75", "151.9", "151.39", "151.77", "1.09666"},
	{"151.78", "152.3", "151.61", "152.02", "1.06762"},
	{"152.33", "152.61", "151.72", "152.15", "1.05493"},
	{"151.69", "152.47", "151.52", "152.29", "1.04743"},
	{"152.43", "152.59", "151.55", "152.11", "1.04690"},
	{"152.37", "153.28", "152.36", "153.25", "1.05570"},
	{"153.14", "153.19", "151.3", "151.34", "1.11957"},
	{"150.92", "151.42", "149.94", "150.42", "1.14532"},
	{"151.16", "151.89", "150.49", "151.89", "1.16851"},
	{"152.63", "152.86", "149", "149", "1.36076"},
	{"149.72", "150.2", "148.73", "150.02", "1.36856"},
	{"149.89", "152.33", "149.76", "151.91", "1.45438"},
}

func TestATR(t *testing.T) {
	atr := NewATR(14)

	for i, d := range spyData {
		candle := &Candle{
			Open:  decimal.Parse(d.open),
			High:  decimal.Parse(d.high),
			Low:   decimal.Parse(d.low),
			Close: decimal.Parse(d.close),
		}
		atr.Add(candle)

		if d.atr == "" {
			if atr.IsReady() {
				t.Errorf("row %d: expected not ready, got ready with value %s", i, atr.Value)
			}
			continue
		}

		if !atr.IsReady() {
			t.Errorf("row %d: expected ready, got not ready", i)
			continue
		}

		expected := decimal.Parse(d.atr)
		// allow small tolerance for floating point differences
		diff := atr.Value.Sub(expected).Abs()
		tolerance := decimal.Parse("0.00001")
		if diff.Cmp(tolerance) > 0 {
			t.Errorf("row %d: expected ATR %s, got %s (diff %s)", i, expected, atr.Value, diff)
		}
	}
}
