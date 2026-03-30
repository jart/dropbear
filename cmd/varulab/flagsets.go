package main

import (
	"dropbear/decimal"
	"slices"
	"sort"
	"strings"
)

// generateFlagCombinations returns all Cartesian product combinations of
// kFlagDimensions. Each combination is a canonical sorted flag string.
// A baseline empty string is always included.
// validateConfig panics if base flags overlap with any dimension flags.
func validateConfig() {
	baseFlags := map[string]bool{}
	for _, tok := range strings.Fields(kBaseFlags) {
		name := tok
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		baseFlags[name] = true
	}
	for _, dim := range kFlagDimensions {
		for _, opt := range dim {
			for _, tok := range strings.Fields(opt) {
				name := tok
				if i := strings.IndexByte(name, '='); i >= 0 {
					name = name[:i]
				}
				if baseFlags[name] {
					panic("flag " + name + " is in both kBaseFlags and kFlagDimensions")
				}
			}
		}
	}
}

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
	// deduplicate: last value wins for same flag name
	deduped := groups[:0]
	for i, g := range groups {
		name := g.flag
		if j := strings.IndexByte(name, '='); j >= 0 {
			name = name[:j]
		}
		if i > 0 {
			prev := groups[i-1].flag
			if j := strings.IndexByte(prev, '='); j >= 0 {
				prev = prev[:j]
			}
			if name == prev {
				deduped[len(deduped)-1] = g // overwrite with last value
				continue
			}
		}
		deduped = append(deduped, g)
	}
	groups = deduped
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
