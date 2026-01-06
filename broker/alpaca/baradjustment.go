package alpaca

import (
	"fmt"
	"strings"
)

type BarAdjustment uint8

const (
	BarAdjustmentRaw      = BarAdjustment(0)
	BarAdjustmentSplit    = BarAdjustment(1)
	BarAdjustmentDividend = BarAdjustment(2)
	BarAdjustmentSpinoff  = BarAdjustment(4)
	BarAdjustmentAll      = BarAdjustmentSplit | BarAdjustmentDividend | BarAdjustmentSpinoff
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
