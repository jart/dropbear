package main

import (
	"dropbear/decimal"
	"slices"
	"sort"
	"strings"
)

// kBaseFlags are included in every backtest run.
var kBaseFlags = "-bearish -hostile"

// kFlagDimensions defines the search space. Each inner slice is a dimension;
// the Cartesian product of all dimensions (plus a baseline with each dimension
// absent) generates the full set of flag combinations to test.
var kFlagDimensions = [][]string{
	{"-spread=1 -eval=3", "-eval=3"},
}

// generateFlagCombinations returns all Cartesian product combinations of
// kFlagDimensions. Each combination is a canonical sorted flag string.
// A baseline empty string is always included.
func generateFlagCombinations() ([]string, error) {
	combos := [][]string{{}} // start with one empty combo
	for _, dim := range kFlagDimensions {
		var next [][]string
		for _, combo := range combos {
			// include a version without this dimension
			next = append(next, combo)
			for _, opt := range dim {
				extended := slices.Clone(combo)
				extended = append(extended, strings.Fields(opt)...)
				next = append(next, extended)
			}
		}
		combos = next
	}
	base := strings.Fields(kBaseFlags)
	seen := map[string]bool{}
	var result []string
	for _, combo := range combos {
		full := append(slices.Clone(base), combo...)
		canon := canonicalizeFlags(full)
		if !seen[canon] {
			seen[canon] = true
			result = append(result, canon)
		}
	}
	sort.Strings(result)
	return result, nil
}

// canonicalizeFlags sorts flag tokens into a deterministic string.
// Flags and their values stay paired: "-spread 2 -eod -sigmas 3"
// becomes "-eod -sigmas 3 -spread 2".
func canonicalizeFlags(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	// parse into flag groups
	type flagGroup struct {
		flag  string
		value string
	}
	var groups []flagGroup
	for i := 0; i < len(tokens); i++ {
		f := tokens[i]
		if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
			groups = append(groups, flagGroup{f, tokens[i+1]})
			i++
		} else {
			groups = append(groups, flagGroup{f, ""})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].flag < groups[j].flag
	})
	var parts []string
	for _, g := range groups {
		if g.value == "" {
			parts = append(parts, g.flag)
		} else {
			parts = append(parts, g.flag+" "+g.value)
		}
	}
	return strings.Join(parts, " ")
}

// parseFlagString splits a canonical flag string back into argv tokens.
func parseFlagString(flags string) []string {
	if flags == "" {
		return nil
	}
	return strings.Fields(flags)
}

// parseWinning parses the "winning" amount from the last line of varu output.
func parseWinning(log string) (winning, balance, fees decimal.Decimal, ok bool) {
	lines := strings.Split(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		idx := strings.Index(line, "ending balance ")
		if idx < 0 {
			continue
		}
		fields := strings.Fields(line[idx:])
		// "ending balance X less Y fees winning Z at SYM price P"
		if len(fields) >= 8 && fields[3] == "less" && fields[5] == "fees" && fields[6] == "winning" {
			b, errB := decimal.ParseString(fields[2])
			f, errF := decimal.ParseString(fields[4])
			w, errW := decimal.ParseString(fields[7])
			if errB == nil && errF == nil && errW == nil {
				return w, b, f, true
			}
		}
	}
	return decimal.Zero, decimal.Zero, decimal.Zero, false
}
