// topvol shows equities with the highest trading volume today.
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func init() {
	netty.SetOffline()
}

var (
	nFlag = flag.Int("n", 50, "number of results to show")
)

type result struct {
	symbol   string
	volume   decimal.Decimal
	notional decimal.Decimal // in millions
	close    decimal.Decimal
}

func main() {
	flag.Parse()

	today := clocky.Now()
	todayStart := clocky.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, clocky.UTC)

	dir := ds.EquityMinutesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", dir, err)
		os.Exit(1)
	}

	var results []result
	for _, entry := range entries {
		if entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		name := entry.Name()
		path := filepath.Join(dir, name)
		bars, err := ds.OpenBars(path)
		if err != nil {
			continue
		}

		// check if last bar is from today by only touching the last page
		last := bars.Get(bars.Count() - 1)
		if last.Timestamp < todayStart {
			bars.Close()
			continue
		}

		// seek to today and sum volume
		bars.Seek(todayStart)
		var vol, notional decimal.Decimal
		var close decimal.Decimal
		for !bars.EOF() {
			bar := bars.Read()
			vol = vol.Add(bar.Volume.DivInt(1_000))
			price := bar.VWAP
			if price.IsZero() {
				price = bar.Close
			}
			notional = notional.Add(bar.Volume.Mul(price).DivInt64(1_000_000))
			close = bar.Close
		}
		bars.Close()

		if vol.IsPositive() {
			results = append(results, result{
				symbol:   name,
				volume:   vol,
				notional: notional,
				close:    close,
			})
		}
	}

	slices.SortFunc(results, func(a, b result) int {
		return b.volume.Cmp(a.volume)
	})

	n := min(*nFlag, len(results))

	fmt.Printf("%-6s %12s %12s %10s\n",
		"SYMBOL", "VOLUME_K", "NOTIONAL_M", "CLOSE")

	for _, r := range results[:n] {
		fmt.Printf("%-6s %12s %12s %10s\n",
			r.symbol,
			r.volume.FormatThousand(0),
			r.notional.FormatThousand(0),
			r.close.FormatThousand(2))
	}
}
