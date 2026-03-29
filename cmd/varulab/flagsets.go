package main

import (
	"database/sql"
	"dropbear/decimal"
	"slices"
	"sort"
	"strings"
)

type FlagSet struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Flag    string `json:"flag"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

func listFlagSets(db *sql.DB) ([]FlagSet, error) {
	rows, err := db.Query(`SELECT id, name, flag, value, enabled FROM varulab_flag_sets ORDER BY name, flag, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sets []FlagSet
	for rows.Next() {
		var fs FlagSet
		if err := rows.Scan(&fs.ID, &fs.Name, &fs.Flag, &fs.Value, &fs.Enabled); err != nil {
			return nil, err
		}
		sets = append(sets, fs)
	}
	return sets, rows.Err()
}

func addFlagSet(db *sql.DB, name, flag, value string) (int64, error) {
	res, err := db.Exec(`INSERT OR IGNORE INTO varulab_flag_sets (name, flag, value, enabled) VALUES (?, ?, ?, 1)`,
		name, flag, value)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func deleteFlagSet(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM varulab_flag_sets WHERE id = ?`, id)
	return err
}

func toggleFlagSet(db *sql.DB, id int64, enabled bool) error {
	_, err := db.Exec(`UPDATE varulab_flag_sets SET enabled = ? WHERE id = ?`, enabled, id)
	return err
}

// generateFlagCombinations returns all Cartesian product combinations of
// enabled flag sets. Each combination is a canonical sorted flag string.
// A baseline empty string is always included.
func generateFlagCombinations(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name, flag, value FROM varulab_flag_sets WHERE enabled = 1 ORDER BY name, flag, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// group by dimension name
	dimensions := map[string][][2]string{} // name -> []{"flag", "value"}
	var dimOrder []string
	for rows.Next() {
		var name, flag, value string
		if err := rows.Scan(&name, &flag, &value); err != nil {
			return nil, err
		}
		if _, ok := dimensions[name]; !ok {
			dimOrder = append(dimOrder, name)
		}
		dimensions[name] = append(dimensions[name], [2]string{flag, value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Cartesian product
	combos := [][]string{{}} // start with one empty combo
	for _, name := range dimOrder {
		opts := dimensions[name]
		var next [][]string
		for _, combo := range combos {
			// include a version without this dimension (baseline)
			next = append(next, combo)
			for _, opt := range opts {
				extended := slices.Clone(combo)
				if opt[1] == "" {
					extended = append(extended, opt[0]) // boolean flag like -eod
				} else {
					extended = append(extended, opt[0], opt[1]) // -spread 2
				}
				next = append(next, extended)
			}
		}
		combos = next
	}
	// canonicalize each combo
	seen := map[string]bool{}
	var result []string
	for _, combo := range combos {
		canon := canonicalizeFlags(combo)
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
