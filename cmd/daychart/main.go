package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	chartWidth  = 80
	chartHeight = 24
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: daychart SYMBOL YYYY-MM-DD\n")
		os.Exit(1)
	}
	symbol := os.Args[1]
	date := os.Args[2]

	bars, err := ds.OpenBars(filepath.Join(ds.EquityMinutesDir(), symbol))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening bars: %v\n", err)
		os.Exit(1)
	}
	defer bars.Close()

	start := clocky.MustParseTime(date + "T00:00:00-04:00")
	end := start.Add(24 * clocky.Hour)
	bars.Seek(start)

	var times []clocky.Time
	var prices []decimal.Decimal
	for bar := bars.Read(); bar != nil && bar.Timestamp < end; bar = bars.Read() {
		times = append(times, bar.Timestamp)
		prices = append(prices, bar.Close)
	}
	if len(prices) == 0 {
		fmt.Fprintf(os.Stderr, "no bars found for %s on %s\n", symbol, date)
		os.Exit(1)
	}

	// find price range
	lo := prices[0]
	hi := prices[0]
	for _, p := range prices[1:] {
		lo = lo.Min(p)
		hi = hi.Max(p)
	}
	if lo.Cmp(hi) == 0 {
		hi = lo.Add(decimal.Cent)
	}
	span := hi.Sub(lo)

	// build chart grid
	grid := make([][]byte, chartHeight)
	for i := range grid {
		grid[i] = make([]byte, chartWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// plot prices
	n := len(prices)
	for i, p := range prices {
		col := i * (chartWidth - 1) / (n - 1)
		row := max(chartHeight-1-int(p.Sub(lo).MulInt(chartHeight-1).Div(span).Int64()), 0)
		if row >= chartHeight {
			row = chartHeight - 1
		}
		grid[row][col] = '*'
	}

	// print header
	open := prices[0]
	close := prices[n-1]
	change := close.Sub(open)
	pct := change.MulInt(100).Div(open)
	fmt.Printf("%s %s  open=%s hi=%s lo=%s close=%s change=%s (%s%%)\n",
		symbol, date, open, hi, lo, close, change, pct.Format(2))
	fmt.Println()

	// print chart with y-axis labels
	for row := range chartHeight {
		price := hi.Sub(span.MulInt(row).Div(decimal.FromInt(chartHeight - 1)))
		label := price.Format(2)
		fmt.Printf("%10s |%s\n", label, string(grid[row]))
	}

	// print x-axis
	fmt.Printf("%10s +%s\n", "", strings.Repeat("-", chartWidth))
	fmt.Printf("%11s%-*s%s\n", "",
		chartWidth-len(times[n-1].Format("15:04")),
		times[0].Format("15:04"),
		times[n-1].Format("15:04"))
}
