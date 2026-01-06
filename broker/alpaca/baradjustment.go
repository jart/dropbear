package alpaca

import (
	"fmt"
	"strings"
)

// BarAdjustment specifies how historical bar prices should be adjusted.
//
// Adjustments normalize ALL returned data to current levels, not just data
// spanning a corporate action. For example, GOOG had a 20:1 split in July
// 2022. Here's what the bars look like around that split:
//
//	                              Raw     Split-Adjusted
//	2022-07-15 (pre-split)      $2255.34     $112.77
//	2022-07-18 (post-split)     $109.91      $109.91
//
// With Raw, you see the actual historical prices (discontinuity at split).
// With Split adjustment, pre-split prices are divided by 20 to match today's
// share count, giving continuous price series suitable for indicators.
//
// For short-term intraday trading (e.g. 1000-minute warmup ~2.5 days), splits
// are rare enough that Raw is typically fine. For longer historical analysis,
// Split or All adjustment prevents massive discontinuities in your data.
type BarAdjustment uint8

const (
	// BarAdjustmentRaw returns actual historical prices with no adjustment.
	// Prices reflect what the stock traded at on that day.
	BarAdjustmentRaw = BarAdjustment(0)

	// BarAdjustmentSplit adjusts prices for stock splits. Pre-split prices
	// are scaled to match the current share count. Use this when you need
	// continuous price series for technical indicators.
	BarAdjustmentSplit = BarAdjustment(1)

	// BarAdjustmentDividend adjusts prices for dividends. Prices before
	// ex-dividend dates are reduced by the dividend amount. Useful for
	// total-return analysis but can distort technical patterns.
	BarAdjustmentDividend = BarAdjustment(2)

	// BarAdjustmentSpinoff adjusts prices for corporate spinoffs.
	BarAdjustmentSpinoff = BarAdjustment(4)

	// BarAdjustmentAll applies all adjustments (split + dividend + spinoff).
	BarAdjustmentAll = BarAdjustmentSplit | BarAdjustmentDividend | BarAdjustmentSpinoff
)

func ParseBarAdjustment(s string) (BarAdjustment, error) {
	// now split commas and turn into bitmask
	var adj BarAdjustment
	parts := map[string]BarAdjustment{
		"raw":      BarAdjustmentRaw,
		"split":    BarAdjustmentSplit,
		"dividend": BarAdjustmentDividend,
		"spin-off": BarAdjustmentSpinoff,
		"all":      BarAdjustmentAll,
	}
	for _, part := range splitAndTrim(s, ",") {
		v, ok := parts[part]
		if !ok {
			return 0, fmt.Errorf("unknown bar adjustment: %s", part)
		}
		adj |= v
	}
	return adj, nil
}

func (ba BarAdjustment) String() string {
	if ba == BarAdjustmentRaw {
		return "raw"
	}
	if ba == BarAdjustmentAll {
		return "all"
	}
	var parts []string
	if ba&BarAdjustmentSplit != 0 {
		parts = append(parts, "split")
	}
	if ba&BarAdjustmentDividend != 0 {
		parts = append(parts, "dividend")
	}
	if ba&BarAdjustmentSpinoff != 0 {
		parts = append(parts, "spin-off")
	}
	return strings.Join(parts, ",")
}

func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for part := range strings.SplitSeq(s, sep) {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
