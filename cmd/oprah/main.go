// Command oprah backtests a short iron condor strategy on SPX 0DTE options
// using Databento CBBO-1s data. Sells OTM wings and collects theta decay.
//
// Usage:
//
//	go run ./cmd/oprah \
//	    -date 2026-02-27 \
//	    -defs ~/databento/SPX/2026-02-27.definition.dbn \
//	    -data ~/databento/SPX/2026-02-27.0dte.cbbo-1s.dbn \
//	    -v
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
)

type optionInfo struct {
	strike decimal.Decimal
	class  byte // 'C' or 'P'
	symbol string
}

type quote struct {
	bidPx, askPx decimal.Decimal
	bidSz, askSz uint32
}

type strikeLevel struct {
	strike        decimal.Decimal
	callID, putID uint32
}

type leg struct {
	id     uint32
	strike decimal.Decimal
	class  byte
	qty    int // +1 long, -1 short
}

type condor struct {
	legs      [4]leg
	entryMids [4]decimal.Decimal
	entryTime clocky.Time
	credit    decimal.Decimal
	exitTime  clocky.Time
	exitPnl   decimal.Decimal
	exitValue decimal.Decimal
	maxAverse decimal.Decimal
	reason    string
}

// dbnPrice converts a Databento int64 price (scale 1e9) to decimal.Decimal (scale 1e6).
func dbnPrice(p int64) decimal.Decimal {
	return decimal.Decimal(p / 1000)
}

func mid(q *quote) decimal.Decimal {
	return q.bidPx.Add(q.askPx).DivInt(2)
}

func intrinsic(class byte, strike, underlying decimal.Decimal) decimal.Decimal {
	if class == 'C' {
		v := underlying.Sub(strike)
		if v.IsPositive() {
			return v
		}
		return decimal.Zero
	}
	v := strike.Sub(underlying)
	if v.IsPositive() {
		return v
	}
	return decimal.Zero
}

func formatET(t clocky.Time) string {
	u := time.Unix(int64(t)/1e9, int64(t)%1e9).In(clocky.NYC)
	return u.Format("15:04:05")
}

func formatPnl(d decimal.Decimal) string {
	if d.IsNegative() {
		return "-$" + d.Neg().Format(2)
	}
	return "+$" + d.Format(2)
}

const (
	stateWaiting    = 0
	stateInPosition = 1
	stateSettled    = 2
)

func main() {
	dateStr := flag.String("date", "2026-02-27", "trading date")
	defsPath := flag.String("defs", "", "path to definitions .dbn")
	dataPath := flag.String("data", "", "path to 0DTE CBBO-1s .dbn")
	entryStr := flag.String("entry", "09:45", "entry time ET (HH:MM)")
	width := flag.Int("width", 5, "wing width in SPX points")
	stopMult := flag.Int("stop", 2, "stop loss multiplier of credit")
	verbose := flag.Bool("v", false, "verbose output")
	flag.Parse()

	if *defsPath == "" || *dataPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse date.
	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("bad date %q: %v", *dateStr, err)
	}
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	// Parse entry time.
	et, err := time.Parse("15:04", *entryStr)
	if err != nil {
		log.Fatalf("bad entry time %q: %v", *entryStr, err)
	}
	entryET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		et.Hour(), et.Minute(), 0, 0, clocky.NYC).UnixNano())
	profitET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		15, 0, 0, 0, clocky.NYC).UnixNano())
	expiryET := clocky.Time(time.Date(date.Year(), date.Month(), date.Day(),
		16, 0, 0, 0, clocky.NYC).UnixNano())

	widthDec := decimal.FromInt(*width)

	// Phase 1: Load definitions.
	instruments, strikes, err := loadDefinitions(*defsPath, queryDateInt)
	if err != nil {
		log.Fatalf("load definitions: %v", err)
	}
	if *verbose {
		fmt.Printf("loaded %d instruments, %d strikes\n", len(instruments), len(strikes))
	}

	// Phase 2: Stream CBBO and run strategy.
	quotes := make(map[uint32]*quote)
	state := stateWaiting
	var c condor
	var atmStrike decimal.Decimal
	var atmCallID, atmPutID uint32

	f, err := os.Open(*dataPath)
	if err != nil {
		log.Fatalf("open data: %v", err)
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		log.Fatalf("decode metadata: %v", err)
	}

	br := bufio.NewReaderSize(f, 1<<20)
	records := 0
	cbboSize := int(unsafe.Sizeof(databento.CBBO{}))

	for {
		rec, err := databento.DecodeRecord(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("decode record: %v", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			log.Fatalf("upgrade record: %v", err)
		}
		if len(rec) < cbboSize {
			continue
		}
		cbbo := (*databento.CBBO)(unsafe.Pointer(&rec[0]))
		records++

		// Skip unknown instruments.
		if _, ok := instruments[cbbo.Header.InstrumentID]; !ok {
			continue
		}

		// Update quote (convert prices at data boundary).
		bidPx := cbbo.Levels[0].BidPx
		askPx := cbbo.Levels[0].AskPx
		if bidPx == databento.UndefPrice || askPx == databento.UndefPrice {
			continue
		}
		q, ok := quotes[cbbo.Header.InstrumentID]
		if !ok {
			q = &quote{}
			quotes[cbbo.Header.InstrumentID] = q
		}
		q.bidPx = dbnPrice(bidPx)
		q.askPx = dbnPrice(askPx)
		q.bidSz = cbbo.Levels[0].BidSz
		q.askSz = cbbo.Levels[0].AskSz

		ts := cbbo.Header.TSEvent

		switch state {
		case stateWaiting:
			if ts.Before(entryET) {
				continue
			}

			// Find ATM strike: minimize |callMid - putMid|.
			bestIdx := -1
			bestDiff := decimal.Max
			for i, sl := range strikes {
				cq := quotes[sl.callID]
				pq := quotes[sl.putID]
				if cq == nil || pq == nil {
					continue
				}
				if cq.bidPx.IsZero() || pq.bidPx.IsZero() {
					continue
				}
				diff := mid(cq).Sub(mid(pq)).Abs()
				if diff.Cmp(bestDiff) < 0 {
					bestDiff = diff
					bestIdx = i
				}
			}
			if bestIdx < 0 {
				continue
			}

			atm := strikes[bestIdx]
			atmStrike = atm.strike
			atmCallID = atm.callID
			atmPutID = atm.putID
			atmCallMid := mid(quotes[atm.callID])
			atmPutMid := mid(quotes[atm.putID])
			straddle := atmCallMid.Add(atmPutMid)
			otmDist := straddle.MulInt(13).DivInt(10)

			if *verbose {
				fmt.Printf("ATM: %d  call mid: %s  put mid: %s  straddle: %s  dist: %s\n",
					atm.strike.Int(), atmCallMid.Format(2), atmPutMid.Format(2),
					straddle.Format(2), otmDist.Format(2))
			}

			// Short put: nearest strike <= ATM - otmDist.
			shortPutTarget := atm.strike.Sub(otmDist)
			shortPutIdx := sort.Search(len(strikes), func(i int) bool {
				return strikes[i].strike.Cmp(shortPutTarget) > 0
			}) - 1
			if shortPutIdx < 0 {
				log.Fatalf("no short put strike below %s", shortPutTarget.String())
			}

			// Short call: nearest strike >= ATM + otmDist.
			shortCallTarget := atm.strike.Add(otmDist)
			shortCallIdx := sort.Search(len(strikes), func(i int) bool {
				return strikes[i].strike.Cmp(shortCallTarget) >= 0
			})
			if shortCallIdx >= len(strikes) {
				log.Fatalf("no short call strike above %s", shortCallTarget.String())
			}

			// Long put: nearest strike <= shortPut - width.
			longPutTarget := strikes[shortPutIdx].strike.Sub(widthDec)
			longPutIdx := sort.Search(len(strikes), func(i int) bool {
				return strikes[i].strike.Cmp(longPutTarget) > 0
			}) - 1
			if longPutIdx < 0 {
				log.Fatalf("no long put strike below %s", longPutTarget.String())
			}

			// Long call: nearest strike >= shortCall + width.
			longCallTarget := strikes[shortCallIdx].strike.Add(widthDec)
			longCallIdx := sort.Search(len(strikes), func(i int) bool {
				return strikes[i].strike.Cmp(longCallTarget) >= 0
			})
			if longCallIdx >= len(strikes) {
				log.Fatalf("no long call strike above %s", longCallTarget.String())
			}

			sp := strikes[shortPutIdx]
			lp := strikes[longPutIdx]
			sc := strikes[shortCallIdx]
			lc := strikes[longCallIdx]

			// Verify all legs have quotes.
			if quotes[sp.putID] == nil || quotes[lp.putID] == nil ||
				quotes[sc.callID] == nil || quotes[lc.callID] == nil {
				continue
			}

			c.legs = [4]leg{
				{id: sp.putID, strike: sp.strike, class: 'P', qty: -1},  // short put
				{id: lp.putID, strike: lp.strike, class: 'P', qty: +1},  // long put
				{id: sc.callID, strike: sc.strike, class: 'C', qty: -1}, // short call
				{id: lc.callID, strike: lc.strike, class: 'C', qty: +1}, // long call
			}
			c.entryTime = ts
			for i, l := range c.legs {
				c.entryMids[i] = mid(quotes[l.id])
			}
			c.credit = c.entryMids[0].Add(c.entryMids[2]).Sub(c.entryMids[1]).Sub(c.entryMids[3])

			if c.credit.IsZero() || c.credit.IsNegative() {
				if *verbose {
					fmt.Printf("skip: non-positive credit %s\n", c.credit.Format(2))
				}
				continue
			}

			state = stateInPosition

			if *verbose {
				fmt.Printf("ENTRY at %s ET\n", formatET(ts))
				fmt.Printf("  short %dP mid %s    long %dP mid %s\n",
					sp.strike.Int(), c.entryMids[0].Format(2),
					lp.strike.Int(), c.entryMids[1].Format(2))
				fmt.Printf("  short %dC mid %s    long %dC mid %s\n",
					sc.strike.Int(), c.entryMids[2].Format(2),
					lc.strike.Int(), c.entryMids[3].Format(2))
				fmt.Printf("  credit: $%s ($%s/contract)\n",
					c.credit.Format(2), c.credit.MulInt(100).Format(2))
			}

		case stateInPosition:
			// Mark to market.
			value := decimal.Zero
			for _, l := range c.legs {
				lq := quotes[l.id]
				if lq == nil {
					continue
				}
				m := mid(lq)
				if l.qty < 0 {
					value = value.Add(m)
				} else {
					value = value.Sub(m)
				}
			}
			pnl := c.credit.Sub(value)
			c.maxAverse = c.maxAverse.Min(pnl)

			// Stop loss: value > credit + credit * stopMult.
			stopThreshold := c.credit.Add(c.credit.MulInt(*stopMult))
			if value.Cmp(stopThreshold) > 0 {
				c.exitTime = ts
				c.exitPnl = pnl
				c.exitValue = value
				c.reason = "stop"
				state = stateSettled
				break
			}

			// Take profit at 3:00 PM ET.
			if !ts.Before(profitET) && value.Cmp(c.credit) < 0 {
				c.exitTime = ts
				c.exitPnl = pnl
				c.exitValue = value
				c.reason = "profit"
				state = stateSettled
				break
			}

			// Expiration at 4:00 PM ET: cash settle at intrinsic.
			if !ts.Before(expiryET) {
				underlying := atmStrike
				if acq := quotes[atmCallID]; acq != nil {
					if apq := quotes[atmPutID]; apq != nil {
						underlying = atmStrike.Add(mid(acq)).Sub(mid(apq))
					}
				}
				value = decimal.Zero
				for _, l := range c.legs {
					iv := intrinsic(l.class, l.strike, underlying)
					if l.qty < 0 {
						value = value.Add(iv)
					} else {
						value = value.Sub(iv)
					}
				}
				c.exitTime = ts
				c.exitPnl = c.credit.Sub(value)
				c.exitValue = value
				c.reason = "expiration"
				state = stateSettled
				break
			}
		}

		if state == stateSettled {
			break
		}
	}

	if *verbose {
		fmt.Printf("processed %d records\n", records)
	}

	if state == stateWaiting {
		log.Fatal("never entered position")
	}
	if state == stateInPosition {
		log.Fatal("data ended while in position (no settlement)")
	}

	// Print results.
	result := "WIN"
	if c.exitPnl.IsNegative() {
		result = "LOSS"
	}
	maxAE := c.maxAverse.Neg().Max(decimal.Zero)

	fmt.Printf("%s 0DTE iron condor\n", *dateStr)
	fmt.Printf("  entry:  %s ET\n", formatET(c.entryTime))
	fmt.Printf("    short %dP  mid %s    long %dP  mid %s\n",
		c.legs[0].strike.Int(), c.entryMids[0].Format(2),
		c.legs[1].strike.Int(), c.entryMids[1].Format(2))
	fmt.Printf("    short %dC  mid %s    long %dC  mid %s\n",
		c.legs[2].strike.Int(), c.entryMids[2].Format(2),
		c.legs[3].strike.Int(), c.entryMids[3].Format(2))
	fmt.Printf("  credit: $%s ($%s/contract)\n",
		c.credit.Format(2), c.credit.MulInt(100).Format(2))
	fmt.Printf("  exit:   %s ET (%s)\n", formatET(c.exitTime), c.reason)
	fmt.Printf("  value:  $%s\n", c.exitValue.Format(2))
	fmt.Printf("  P&L:    %s (%s/contract)\n",
		formatPnl(c.exitPnl), formatPnl(c.exitPnl.MulInt(100)))
	fmt.Printf("  max AE: $%s\n", maxAE.Format(2))
	fmt.Printf("  result: %s\n", result)
}

// loadDefinitions reads a definitions .dbn file and returns instruments and
// sorted strike levels for options expiring on the given date.
func loadDefinitions(path string, queryDateInt int) (map[uint32]optionInfo, []strikeLevel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	meta, err := databento.DecodeMetadata(f)
	if err != nil {
		return nil, nil, fmt.Errorf("decode metadata: %w", err)
	}

	instruments := make(map[uint32]optionInfo)
	strikeMap := make(map[decimal.Decimal]*strikeLevel)
	instSize := int(unsafe.Sizeof(databento.Instrument{}))

	for {
		rec, err := databento.DecodeRecord(f)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, fmt.Errorf("decode record: %w", err)
		}
		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			return nil, nil, fmt.Errorf("upgrade record: %w", err)
		}
		if len(rec) < instSize {
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))

		// Filter by instrument class.
		var class byte
		switch inst.InstrumentClass {
		case databento.InstrumentClassCall:
			class = 'C'
		case databento.InstrumentClassPut:
			class = 'P'
		default:
			continue
		}

		// Filter by expiration date.
		expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
		expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
		if expDateInt != queryDateInt {
			continue
		}

		// Skip undefined strike price.
		if inst.StrikePrice == databento.UndefPrice {
			continue
		}

		strike := dbnPrice(inst.StrikePrice)
		id := inst.Header.InstrumentID
		instruments[id] = optionInfo{
			strike: strike,
			class:  class,
			symbol: inst.GetRawSymbol(),
		}

		// Build strike levels.
		sl, ok := strikeMap[strike]
		if !ok {
			sl = &strikeLevel{strike: strike}
			strikeMap[strike] = sl
		}
		if class == 'C' {
			sl.callID = id
		} else {
			sl.putID = id
		}
	}

	// Build sorted strikes (only those with both call and put).
	strikes := make([]strikeLevel, 0, len(strikeMap))
	for _, sl := range strikeMap {
		if sl.callID != 0 && sl.putID != 0 {
			strikes = append(strikes, *sl)
		}
	}
	sort.Slice(strikes, func(i, j int) bool {
		return strikes[i].strike.Cmp(strikes[j].strike) < 0
	})

	return instruments, strikes, nil
}
