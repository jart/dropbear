// dump prints Databento MBO files in human-readable format.
//
// Usage:
//
//	go run ./broker/databento/cmd/dump ~/databento/GOOG/2024-12-20.mbo
//	go run ./broker/databento/cmd/dump -n 100 ~/databento/GOOG/2024-12-20.mbo
//	go run ./broker/databento/cmd/dump -trades ~/databento/GOOG/2024-12-20.mbo
package main

import (
	"dropbear/broker/databento"
	"flag"
	"fmt"
	"os"
	"time"
)

var (
	flagN      = flag.Int("n", 0, "limit output to first N records (0 = all)")
	flagTrades = flag.Bool("trades", false, "only show trades")
	flagAdds   = flag.Bool("adds", false, "only show order additions")
	flagBids   = flag.Bool("bids", false, "only show bid side")
	flagAsks   = flag.Bool("asks", false, "only show ask side")
	flagCSV    = flag.Bool("csv", false, "output as CSV")
)

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "usage: dump [flags] <file.mbo>\n")
		os.Exit(1)
	}

	path := flag.Arg(0)
	mbo, err := databento.OpenMboFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer mbo.Close()

	if *flagCSV {
		fmt.Println("timestamp,action,side,price,size,order_id,flags,sequence")
	}

	count := 0
	for i := 0; i < mbo.Len(); i++ {
		r := &mbo.Records[i]

		// Apply filters
		if *flagTrades && r.Action != databento.ActionTrade {
			continue
		}
		if *flagAdds && r.Action != databento.ActionAdd {
			continue
		}
		if *flagBids && r.Side != databento.SideBid {
			continue
		}
		if *flagAsks && r.Side != databento.SideAsk {
			continue
		}

		if *flagCSV {
			printCSV(r)
		} else {
			printRecord(r)
		}

		count++
		if *flagN > 0 && count >= *flagN {
			break
		}
	}

	if !*flagCSV {
		fmt.Fprintf(os.Stderr, "# %d records\n", count)
	}
}

func printRecord(r *databento.MboMsg) {
	ts := time.Unix(0, int64(r.Header.TsEvent)).UTC()
	price := float64(r.Price) / databento.PriceScale

	action := actionName(r.Action)
	side := sideName(r.Side)

	// Format: timestamp action side price size [order_id] [flags]
	switch r.Action {
	case databento.ActionTrade:
		fmt.Printf("%s %-5s %4s %10.2f x %-6d seq=%d\n",
			ts.Format("15:04:05.000000000"), action, side, price, r.Size, r.Sequence)
	case databento.ActionClear:
		fmt.Printf("%s %-5s (book cleared)\n",
			ts.Format("15:04:05.000000000"), action)
	default:
		fmt.Printf("%s %-5s %4s %10.2f x %-6d order=%d seq=%d\n",
			ts.Format("15:04:05.000000000"), action, side, price, r.Size, r.OrderID, r.Sequence)
	}
}

func printCSV(r *databento.MboMsg) {
	ts := time.Unix(0, int64(r.Header.TsEvent)).UTC()
	price := float64(r.Price) / databento.PriceScale

	fmt.Printf("%s,%c,%c,%.9f,%d,%d,%d,%d\n",
		ts.Format(time.RFC3339Nano),
		r.Action, r.Side, price, r.Size, r.OrderID, r.Flags, r.Sequence)
}

func actionName(a byte) string {
	switch a {
	case databento.ActionAdd:
		return "ADD"
	case databento.ActionCancel:
		return "CXL"
	case databento.ActionModify:
		return "MOD"
	case databento.ActionTrade:
		return "TRADE"
	case databento.ActionFill:
		return "FILL"
	case databento.ActionClear:
		return "CLEAR"
	default:
		return fmt.Sprintf("?%c", a)
	}
}

func sideName(s byte) string {
	switch s {
	case databento.SideBid:
		return "BID"
	case databento.SideAsk:
		return "ASK"
	case databento.SideNone:
		return ""
	default:
		return fmt.Sprintf("?%c", s)
	}
}
