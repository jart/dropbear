// study2 measures how often quote imbalance exceeds a threshold
// across the full SIP feed, broken down by price range.
//
// usage: go test -run Study -timeout 0 -- ~/sip/sipdata-2026-01-07.sip
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
)

func init() {
	netty.SetOffline()
}

var (
	flagDayOnly = flag.Bool("dayonly", false, "restrict to day session only")
)

var priceBuckets = []decimal.Decimal{
	decimal.Parse("1"),
	decimal.Parse("5"),
	decimal.Parse("10"),
	decimal.Parse("25"),
	decimal.Parse("50"),
	decimal.Parse("100"),
	decimal.Parse("250"),
	decimal.Parse("500"),
	decimal.Parse("99999"),
}

var thresholds = []decimal.Decimal{
	decimal.Parse("0.1"),
	decimal.Parse("0.2"),
	decimal.Parse("0.3"),
	decimal.Parse("0.4"),
	decimal.Parse("0.5"),
	decimal.Parse("0.6"),
	decimal.Parse("0.7"),
	decimal.Parse("0.8"),
	decimal.Parse("0.9"),
	decimal.Parse("0.925"),
	decimal.Parse("0.95"),
	decimal.Parse("0.975"),
	decimal.Parse("0.99"),
}

type bucket struct {
	quotes int64
	above  [13]int64 // count of quotes exceeding each threshold
}

var (
	gSymQuote map[symbol.Symbol]int64
	gTotal    bucket
	gByPrice  []bucket
	gFirst    clocky.Time
	gLast     clocky.Time
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: go test -run Study -timeout 0 -- <sipfile>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		study(path)
	}
}

func study(path string) {
	f, err := sip.OpenFile(path)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	defer f.Close()

	gSymQuote = make(map[symbol.Symbol]int64, 16384)
	gTotal = bucket{}
	gByPrice = make([]bucket, len(priceBuckets))

	count := f.Count()
	gFirst = f.Get(0).Timestamp
	gLast = f.Get(count - 1).Timestamp

	pctStep := count / 100
	if pctStep == 0 {
		pctStep = 1
	}

	for i := 0; i < count; i++ {
		if i%pctStep == 0 {
			fmt.Fprintf(os.Stderr, "\r%d%%", i*100/count)
		}
		msg := f.Get(i)
		if msg.Type != sip.MessageTypeQuote {
			continue
		}
		q := msg.Quote()
		if q.BidPrice.IsZero() || q.AskPrice.IsZero() {
			continue
		}
		if q.BidPrice.Cmp(q.AskPrice) >= 0 {
			continue
		}
		if q.Indicative() {
			continue
		}
		if q.DangerousAsk() || q.DangerousBid() {
			continue
		}

		bidSize := decimal.FromInt(int(q.BidSize))
		askSize := decimal.FromInt(int(q.AskSize))
		total := bidSize.Add(askSize)
		if total.IsZero() {
			continue
		}

		imbalance := bidSize.Sub(askSize).Div(total).Abs()
		mid := q.Midpoint()
		pbkt := priceBucket(mid)

		gTotal.quotes++
		gByPrice[pbkt].quotes++
		gSymQuote[msg.Symbol]++

		for t, thresh := range thresholds {
			if imbalance.Cmp(thresh) > 0 {
				gTotal.above[t]++
				gByPrice[pbkt].above[t]++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "\r     \r")

	printResults(path, count)
}

func priceBucket(mid decimal.Decimal) int {
	for i, bound := range priceBuckets {
		if mid.Cmp(bound) < 0 {
			return i
		}
	}
	return len(priceBuckets) - 1
}

func priceBucketLabel(i int) string {
	if i == 0 {
		return fmt.Sprintf("< $%s", priceBuckets[0])
	}
	if i == len(priceBuckets)-1 {
		return fmt.Sprintf(">= $%s", priceBuckets[i-1])
	}
	return fmt.Sprintf("$%s-$%s", priceBuckets[i-1], priceBuckets[i])
}

func printResults(path string, count int) {
	fmt.Printf("\n")
	fmt.Printf("════════════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("  IMBALANCE FREQUENCY: %s\n", path)
	fmt.Printf("════════════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("  messages: %d  valid quotes: %d  symbols: %d\n", count, gTotal.quotes, len(gSymQuote))
	fmt.Printf("  time range: %s to %s\n\n", gFirst, gLast)

	// overall
	fmt.Printf("── OVERALL ─────────────────────────────────────────────────────────────────\n")
	printBucket("all", gTotal)

	// by price
	fmt.Printf("\n── BY PRICE ────────────────────────────────────────────────────────────────\n")
	fmt.Printf("  %-14s %12s", "PRICE", "QUOTES")
	for _, t := range thresholds {
		fmt.Printf(" %6s", ">"+t.String())
	}
	fmt.Printf("\n")
	for i, b := range gByPrice {
		if b.quotes == 0 {
			continue
		}
		fmt.Printf("  %-14s %12d", priceBucketLabel(i), b.quotes)
		for t := range thresholds {
			pct := float64(b.above[t]) / float64(b.quotes) * 100
			fmt.Printf(" %5.1f%%", pct)
		}
		fmt.Printf("\n")
	}
	fmt.Printf("\n")
}

func printBucket(label string, b bucket) {
	fmt.Printf("  %s: %d valid quotes\n", label, b.quotes)
	fmt.Printf("  %-12s %12s %8s\n", "THRESHOLD", "QUOTES", "RATE")
	for t, thresh := range thresholds {
		pct := float64(b.above[t]) / float64(b.quotes) * 100
		fmt.Printf("  > %-10s %12d %7.1f%%\n", thresh, b.above[t], pct)
	}
}
