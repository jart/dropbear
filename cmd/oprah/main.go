// Command oprah streams live 0DTE options NBBO data from OPRA via Databento.
//
// Phase 1: Fetches instrument definitions over HTTP, filters for 0DTE puts/calls.
// Phase 2: Opens LSG connection subscribing to just those instrument IDs for
// tick-level consolidated NBBO (CMBP1).
//
// Usage:
//
//	go run ./cmd/oprah
//	go run ./cmd/oprah -symbol SPY.OPT -start 2026-03-03T06:30:00
//	go run ./cmd/oprah -symbol SPXW.OPT -n 1000
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"strconv"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/loggy"
)

var (
	flagSymbol = flag.String("symbol", "SPY.OPT", "parent symbol to subscribe")
	flagStart  = clocky.TimeFlag("start", "2026-02-27T06:30:00", "start time in Pacific")
	flagN      = flag.Int("n", 0, "max NBBO records to print (0 = unlimited)")
)

// inst0DTE holds the fields we care about for a 0DTE option.
type inst0DTE struct {
	symbol string
	class  databento.InstrumentClass // 'C' or 'P'
	strike int64                     // fixed-point 1e-9
}

func main() {
	flag.Parse()
	loggy.Init()

	apiKey := databento.MustLoadDefaultKey()
	queryDateInt := flagStart.DateInt()

	// Phase 1: Fetch instrument definitions over HTTP and filter for 0DTE.
	log.Printf("fetching definitions: symbol=%s date=%d", *flagSymbol, queryDateInt)
	allInsts, err := databento.GetInstruments(apiKey, "OPRA.PILLAR", *flagSymbol, *flagStart)
	if err != nil {
		log.Fatal(err)
	}

	instruments := make(map[uint32]*inst0DTE)
	var iids []string
	for _, ref := range allInsts {
		if ref.Expiration.DateInt() != queryDateInt {
			continue
		}
		if ref.InstrumentClass != databento.InstrumentClassCall &&
			ref.InstrumentClass != databento.InstrumentClassPut {
			continue
		}
		iid := ref.Header.InstrumentID
		instruments[iid] = &inst0DTE{
			symbol: ref.GetRawSymbol(),
			class:  ref.InstrumentClass,
			strike: ref.StrikePrice,
		}
		iids = append(iids, strconv.FormatUint(uint64(iid), 10))
	}
	log.Printf("definitions: %d total, %d 0DTE puts/calls", len(allInsts), len(instruments))

	if len(iids) == 0 {
		log.Fatal("no 0DTE instruments found")
	}

	// Phase 2: Stream tick-level NBBO for just the 0DTE instruments.
	client, err := databento.Dial("OPRA.PILLAR", apiKey)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	err = client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeInstrumentID,
		Symbols: iids,
		Start:   *flagStart,
	})
	if err != nil {
		log.Fatal(err)
	}

	meta, err := client.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("streaming: version=%d schema=%s", meta.Version, meta.Schema)

	printed := 0
	ticks := 0
	for {
		rec, err := client.NextRecord()
		if err != nil {
			if err == io.EOF {
				log.Printf("stream ended")
				break
			}
			log.Fatalf("NextRecord: %v", err)
		}

		switch r := databento.CastRecord(rec).(type) {
		case *databento.CMBP1:
			ticks++
			info := instruments[r.Header.InstrumentID]
			if info == nil {
				continue
			}
			bid := r.Levels[0].BidPx
			ask := r.Levels[0].AskPx
			if bid == databento.UndefPrice || ask == databento.UndefPrice {
				continue
			}
			cp := "C"
			if info.class == databento.InstrumentClassPut {
				cp = "P"
			}
			log.Printf("%-30s %s strike=%s  bid=%s x%-5d  ask=%s x%-5d",
				info.symbol, cp,
				fmtPrice(info.strike),
				fmtPrice(bid), r.Levels[0].BidSz,
				fmtPrice(ask), r.Levels[0].AskSz)
			printed++
			if *flagN > 0 && printed >= *flagN {
				log.Printf("reached -n %d limit", *flagN)
				return
			}

		case databento.ErrorMsg:
			log.Printf("ERROR from gateway: %s", r.Err)

		case databento.SystemMsg:
			if !r.IsHeartbeat() {
				log.Printf("system: %s", r.Msg)
			}

		default:
			rtype := databento.RType(rec[1])
			hdr := (*databento.RecordHeader)(unsafe.Pointer(&rec[0]))
			log.Printf("unknown record type %s iid=%d ts=%s (%d bytes)",
				rtype, hdr.InstrumentID, hdr.TSEvent, len(rec))
		}

		if ticks > 0 && ticks%100000 == 0 {
			log.Printf("progress: ticks=%d printed=%d", ticks, printed)
		}
	}

	log.Printf("done: ticks=%d printed=%d", ticks, printed)
}

// fmtPrice formats a databento fixed-point price (1e-9 scale) as a decimal string.
func fmtPrice(p int64) string {
	if p == databento.UndefPrice {
		return "undef"
	}
	whole := p / databento.FixedPriceScale
	frac := p % databento.FixedPriceScale
	if frac < 0 {
		frac = -frac
	}
	cents := frac / 10000000
	return fmt.Sprintf("%d.%02d", whole, cents)
}
