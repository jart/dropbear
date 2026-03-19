// Black-76 option pricing model for European options on futures.
//
// Reads ES futures, SR1 rate, and SPXW option data from databento DBN files,
// computes Black-76 fair values, and compares them to market midpoints.
//
// Usage:
//
//	go run ./cmd/black76 -date 2026-03-13
//	go run ./cmd/black76 -date 2026-03-13 -time 12:00
//	go run ./cmd/black76 -date 2026-03-13 -time 12:00 -range 100
//
// Reference: Fischer Black, "The pricing of commodity contracts",
// Journal of Financial Economics, 3(1-2), 1976, pp. 167-179.
//
// Black-76 formulas and implied vol solver adapted from QuantLib's BlackFormula:
// https://github.com/lballabio/QuantLib/blob/master/ql/pricingengines/blackformula.cpp
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/ds/black76"
)

var (
	dateFlag  = flag.String("date", "", "trading date (YYYY-MM-DD)")
	timeFlag  = flag.String("time", "12:00", "snapshot time in ET (HH:MM)")
	dirFlag   = flag.String("dir", "", "databento data directory (default ~/databento)")
	rangeFlag = flag.Float64("range", 200, "strike range around ATM to display")
)

// dbnToFloat converts a databento fixed-point price (1e-9 units) to float64.
func dbnToFloat(p int64) float64 {
	if p == databento.UndefPrice {
		return 0
	}
	return float64(p) / 1e9
}

type optionDef struct {
	Strike float64
	IsCall bool
	Expiry clocky.Time
}

type optionQuote struct {
	Bid float64
	Ask float64
}

type priceSample struct {
	ts  clocky.Time
	mid float64
}

func main() {
	flag.Parse()
	if *dateFlag == "" {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/black76 -date 2026-03-13 [-time 12:00]\n")
		os.Exit(1)
	}

	dataDir := *dirFlag
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), "databento")
	}

	// Parse snapshot time in ET
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	snapshotTime, err := time.ParseInLocation("2006-01-02 15:04", *dateFlag+" "+*timeFlag, loc)
	if err != nil {
		log.Fatalf("parse time: %v", err)
	}
	snapshotTS := clocky.Time(snapshotTime.UnixNano())

	// Expiry is 4:00 PM ET on the target date (PM-settled SPXW)
	expiryTime := time.Date(snapshotTime.Year(), snapshotTime.Month(), snapshotTime.Day(), 16, 0, 0, 0, loc)
	expiryTS := clocky.Time(expiryTime.UnixNano())

	// 1. Load ES futures prices
	log.Printf("loading ES futures data...")
	esSamples := loadFuturesPrices(filepath.Join(dataDir, "ES", *dateFlag+".mbp-1.dbn"), snapshotTS)
	if len(esSamples) == 0 {
		log.Fatal("no ES price data found before snapshot time")
	}
	esPrice := esSamples[len(esSamples)-1].mid
	log.Printf("ES price at snapshot: %.2f (%d samples)", esPrice, len(esSamples))

	// 2. Load SR1 rate (SOFR 1-month future: price = 100 - annualized rate)
	log.Printf("loading SR1 rate data...")
	sr1Samples := loadFuturesPrices(filepath.Join(dataDir, "SR1", *dateFlag+".mbp-1.dbn"), snapshotTS)
	var rate float64
	if len(sr1Samples) > 0 {
		sr1Price := sr1Samples[len(sr1Samples)-1].mid
		rate = (100.0 - sr1Price) / 100.0
		log.Printf("SR1 price: %.4f -> rate: %.4f%%", sr1Price, rate*100)
	} else {
		rate = 0.04
		log.Printf("warning: no SR1 data, using default rate %.2f%%", rate*100)
	}

	// 3. Load SPXW option definitions (filter for 0DTE)
	log.Printf("loading SPXW definitions...")
	targetYear, targetMonth, targetDay := snapshotTime.Date()
	options := loadOptionDefs(
		filepath.Join(dataDir, "SPXW", *dateFlag+".definition.dbn"),
		loc, targetYear, targetMonth, targetDay,
	)
	log.Printf("loaded %d 0DTE option definitions", len(options))

	// 4. Stream SPXW quotes until snapshot time
	log.Printf("streaming SPXW quotes until %s ET...", snapshotTime.Format("15:04"))
	quotes := loadOptionQuotes(
		filepath.Join(dataDir, "SPXW", *dateFlag+".cmbp-1.dbn"),
		options, snapshotTS,
	)
	log.Printf("got quotes for %d options", len(quotes))

	// 5. Compute time to expiry
	T := float64(expiryTS-snapshotTS) / (365.25 * 24 * 3600 * 1e9)
	if T <= 0 {
		log.Fatal("snapshot time is at or after expiry")
	}

	// 6. Compute implied vol for each option
	type result struct {
		strike float64
		isCall bool
		mid    float64
		iv     float64
		b76    float64
	}
	var results []result
	for id, def := range options {
		q, ok := quotes[id]
		if !ok || q.Bid <= 0 || q.Ask <= 0 {
			continue
		}
		mid := (q.Bid + q.Ask) / 2
		iv := black76.IV(esPrice, def.Strike, rate, T, mid, def.IsCall)
		if iv <= 0.001 || iv > 5.0 {
			continue
		}
		results = append(results, result{
			strike: def.Strike,
			isCall: def.IsCall,
			mid:    mid,
			iv:     iv,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].strike != results[j].strike {
			return results[i].strike < results[j].strike
		}
		return results[i].isCall && !results[j].isCall
	})

	// 7. Find ATM implied vol (average of ATM call and put)
	var atmCallIV, atmPutIV float64
	bestCallDist := math.MaxFloat64
	bestPutDist := math.MaxFloat64
	for _, r := range results {
		dist := math.Abs(r.strike - esPrice)
		if r.isCall && dist < bestCallDist {
			bestCallDist = dist
			atmCallIV = r.iv
		}
		if !r.isCall && dist < bestPutDist {
			bestPutDist = dist
			atmPutIV = r.iv
		}
	}
	atmIV := (atmCallIV + atmPutIV) / 2
	if atmIV <= 0 {
		log.Fatal("could not determine ATM implied vol")
	}

	// 8. Compute B76 prices using flat ATM vol
	for i := range results {
		if results[i].isCall {
			results[i].b76 = black76.Call(esPrice, results[i].strike, rate, T, atmIV)
		} else {
			results[i].b76 = black76.Put(esPrice, results[i].strike, rate, T, atmIV)
		}
	}

	// 9. Print report
	minutes := T * 365.25 * 24 * 60
	fmt.Printf("\nBlack-76 Option Pricing Report\n")
	fmt.Printf("==============================\n")
	fmt.Printf("Date:            %s\n", *dateFlag)
	fmt.Printf("Snapshot:        %s ET\n", snapshotTime.Format("15:04:05"))
	fmt.Printf("ES Futures:      %.2f\n", esPrice)
	fmt.Printf("Risk-Free Rate:  %.4f%%\n", rate*100)
	fmt.Printf("Time to Expiry:  %.1f min (%.6f yr)\n", minutes, T)
	fmt.Printf("ATM Implied Vol: %.2f%% (call=%.2f%% put=%.2f%%)\n", atmIV*100, atmCallIV*100, atmPutIV*100)
	fmt.Printf("Strike Range:    %.0f-%.0f (ES ± %.0f)\n\n", esPrice-*rangeFlag, esPrice+*rangeFlag, *rangeFlag)

	fmt.Printf("%-8s %-4s %10s %10s %+9s %8s\n",
		"Strike", "Type", "Market", "B76(flat)", "Error", "IV")
	fmt.Printf("%-8s %-4s %10s %10s %9s %8s\n",
		"------", "----", "--------", "---------", "-------", "------")

	var sumErr, sumAbsErr, sumSqErr float64
	var count int
	for _, r := range results {
		if math.Abs(r.strike-esPrice) > *rangeFlag {
			continue
		}
		typ := "P"
		if r.isCall {
			typ = "C"
		}
		err := r.b76 - r.mid
		fmt.Printf("%-8.0f %-4s %10.2f %10.2f %+9.2f %7.1f%%\n",
			r.strike, typ, r.mid, r.b76, err, r.iv*100)
		sumErr += err
		sumAbsErr += math.Abs(err)
		sumSqErr += err * err
		count++
	}

	if count > 0 {
		fmt.Printf("\nSummary (%d options within ±%.0f of ATM):\n", count, *rangeFlag)
		fmt.Printf("  Mean Error:     %+.4f\n", sumErr/float64(count))
		fmt.Printf("  Mean Abs Error: %.4f\n", sumAbsErr/float64(count))
		fmt.Printf("  RMSE:           %.4f\n", math.Sqrt(sumSqErr/float64(count)))
	}
}

// loadFuturesPrices reads MBP1 records from a DBN file, downsamples to one
// price per second, and returns all samples up to the given timestamp.
func loadFuturesPrices(path string, until clocky.Time) []priceSample {
	r, err := databento.OpenFile(path)
	if err != nil {
		log.Printf("warning: cannot open %s: %v", path, err)
		return nil
	}
	defer r.Close()
	var samples []priceSample
	var lastSec int64
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read %s: %v", path, err)
		}
		mbp, ok := rec.(*databento.MBP1)
		if !ok {
			continue
		}
		if mbp.TSRecv > until {
			break
		}
		bid := dbnToFloat(mbp.Levels[0].BidPx)
		ask := dbnToFloat(mbp.Levels[0].AskPx)
		if bid <= 0 || ask <= 0 {
			continue
		}
		mid := (bid + ask) / 2
		sec := int64(mbp.TSRecv) / 1_000_000_000
		if sec == lastSec {
			samples[len(samples)-1].mid = mid
			continue
		}
		lastSec = sec
		samples = append(samples, priceSample{ts: mbp.TSRecv, mid: mid})
	}
	return samples
}

// loadOptionDefs reads Instrument records from a definition DBN file and
// returns only the 0DTE options matching the target date.
func loadOptionDefs(path string, loc *time.Location, year int, month time.Month, day int) map[uint32]*optionDef {
	r, err := databento.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer r.Close()
	// Match date in the RawSymbol field: "SPXW  YYMMDD..."
	dateStr := fmt.Sprintf("%02d%02d%02d", year%100, month, day)
	options := make(map[uint32]*optionDef)
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read %s: %v", path, err)
		}
		inst, ok := rec.(*databento.Instrument)
		if !ok {
			continue
		}
		// Filter by date in RawSymbol: "SPXW  YYMMDDX..."
		sym := inst.GetRawSymbol()
		if len(sym) < 12 || sym[6:12] != dateStr {
			continue
		}
		strike := dbnToFloat(inst.StrikePrice)
		if strike <= 0 {
			continue
		}
		isCall := inst.InstrumentClass == databento.InstrumentClassCall
		options[inst.Header.InstrumentID] = &optionDef{
			Strike: strike,
			IsCall: isCall,
			Expiry: inst.Expiration,
		}
	}
	return options
}

// loadOptionQuotes streams CMBP1 records from a DBN file, tracking the latest
// bid/ask for each instrument in the defs set, stopping at the given timestamp.
func loadOptionQuotes(path string, defs map[uint32]*optionDef, until clocky.Time) map[uint32]*optionQuote {
	r, err := databento.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer r.Close()
	quotes := make(map[uint32]*optionQuote)
	n := 0
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read %s: %v", path, err)
		}
		n++
		if n%50_000_000 == 0 {
			log.Printf("  processed %dM records...", n/1_000_000)
		}
		cmbp, ok := rec.(*databento.CMBP1)
		if !ok {
			continue
		}
		if cmbp.TSRecv > until {
			break
		}
		if _, ok := defs[cmbp.Header.InstrumentID]; !ok {
			continue
		}
		bid := dbnToFloat(cmbp.Levels[0].BidPx)
		ask := dbnToFloat(cmbp.Levels[0].AskPx)
		if bid <= 0 || ask <= 0 {
			continue
		}
		q := quotes[cmbp.Header.InstrumentID]
		if q == nil {
			q = &optionQuote{}
			quotes[cmbp.Header.InstrumentID] = q
		}
		q.Bid = bid
		q.Ask = ask
	}
	return quotes
}
