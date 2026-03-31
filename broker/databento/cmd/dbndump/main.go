// dbndump prints the contents of DBN files in human-readable format.
//
// Usage:
//
//	go run ./broker/databento/cmd/dbndump ~/databento/SPXW/2026-03-13.definition.dbn
//	go run ./broker/databento/cmd/dbndump ~/databento/SPXW/2026-03-13.cmbp-1.dbn
//	go run ./broker/databento/cmd/dbndump -n 10 ~/databento/SPXW/2026-03-13.cmbp-1.dbn
//	go run ./broker/databento/cmd/dbndump -strike 302.5 -put -expiry 2026-03-20 ~/databento/GOOG/2026-03-20.dbn
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/osi"
)

var (
	nFlag      = flag.Int("n", 0, "max number of records to print (0 = all)")
	mFlag      = flag.Bool("m", false, "print metadata only")
	strikeFlag = decimal.Flag("strike", "0", "filter by strike price")
	putFlag    = flag.Bool("put", false, "filter for puts only")
	callFlag   = flag.Bool("call", false, "filter for calls only")
	expiryFlag = clocky.TimeFlag("expiry", "", "filter by expiration date")
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: dbndump [flags] <file.dbn> ...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		if err := dump(path); err != nil {
			log.Fatal(err)
		}
	}
}

func dump(path string) error {
	r, err := databento.OpenFileReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	fmt.Printf("%#v\n", &r.Metadata)
	if *mFlag {
		return nil
	}

	filtering := !strikeFlag.IsZero() || *putFlag || *callFlag || !expiryFlag.IsZero()
	wantIDs := map[uint32]bool{}

	count := 0
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if filtering {
			switch m := rec.(type) {
			case *databento.Instrument:
				if matchInstrument(m) {
					wantIDs[m.Header.InstrumentID] = true
				} else {
					continue
				}
			case *databento.CMBP1:
				if !wantIDs[m.Header.InstrumentID] {
					continue
				}
			case *databento.CBBO:
				if !wantIDs[m.Header.InstrumentID] {
					continue
				}
			case *databento.MBP1:
				if !wantIDs[m.Header.InstrumentID] {
					continue
				}
			case *databento.MBP10:
				if !wantIDs[m.Header.InstrumentID] {
					continue
				}
			}
		}

		if gs, ok := rec.(fmt.GoStringer); ok {
			fmt.Printf("%s\n", gs.GoString())
		} else {
			fmt.Printf("%#v\n", rec)
		}
		count++
		if *nFlag > 0 && count >= *nFlag {
			return nil
		}
	}
}

func matchInstrument(m *databento.Instrument) bool {
	if !strikeFlag.IsZero() {
		strike := decimal.Decimal(m.StrikePrice / 1000)
		if strike.Cmp(*strikeFlag) != 0 {
			return false
		}
	}
	if *putFlag && m.InstrumentClass != databento.InstrumentClassPut {
		return false
	}
	if *callFlag && m.InstrumentClass != databento.InstrumentClassCall {
		return false
	}
	if !expiryFlag.IsZero() {
		wantYear, wantMonth, wantDay := expiryFlag.Date()
		_, _, _, expYear, expMonth, expDay, _ := osi.Parse(m.GetRawSymbol())
		if expYear == wantYear && expMonth == wantMonth && expDay == wantDay {
			return false
		}
	}
	return true
}
