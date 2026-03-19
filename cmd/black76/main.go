// Black-76 real-time fair value tracker.
//
// Streams ES futures ticks alongside SPXW option quotes and measures how well
// Black-76 predicts fresh option quotes from the latest ES price. Compares two
// prediction methods:
//
//   - Delta: old_mid + delta * (new_ES - old_ES)  — fast, first-order
//   - Smile: Black76(new_ES, K, r, T, smile(K))   — full reprice via vol smile
//
// The smile is a quadratic in log-moneyness: σ(K) = a + b·x + c·x²
// where x = ln(K/F), fitted by least squares from recently observed IVs.
//
// Usage:
//
//	go run ./cmd/black76 -date 2026-03-13
//	go run ./cmd/black76 -date 2026-03-13 -time 09:30 -duration 1h
//	go run ./cmd/black76 -date 2026-03-13 -time 09:30 -duration 30m -range 100
//
// Reference: Fischer Black, "The pricing of commodity contracts",
// Journal of Financial Economics, 3(1-2), 1976, pp. 167-179.
//
// Black-76 formulas adapted from QuantLib's BlackFormula:
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
	"time"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/ds/black76"
)

var (
	dateFlag     = flag.String("date", "", "trading date (YYYY-MM-DD)")
	timeFlag     = flag.String("time", "09:30", "analysis start time in ET (HH:MM)")
	durationFlag = flag.Duration("duration", time.Hour, "analysis window duration")
	dirFlag      = flag.String("dir", "", "databento data directory (default ~/databento)")
	rangeFlag    = flag.Float64("range", 200, "strike range around ATM to analyze")
)

func dbnToFloat(p int64) float64 {
	if p == databento.UndefPrice {
		return 0
	}
	return float64(p) / 1e9
}

// smile fits σ(K) = A + B·d + C·d² where d = K - F (strike points from ATM).
type smile struct {
	F       float64 // forward price at calibration time
	A, B, C float64
}

// Vol returns the interpolated vol for strike K, re-centered on forward F.
// If F differs from the calibration forward, the smile shifts with it
// (sticky-strike behavior: same K gets same vol regardless of spot move).
func (s smile) Vol(K, F float64) float64 {
	d := K - F
	v := s.A + s.B*d + s.C*d*d
	if v < 0.01 {
		return 0.01
	}
	return v
}

// fitSmile fits a quadratic vol smile from (strike, iv) observations.
func fitSmile(F float64, strikes []float64, ivs []float64) smile {
	if len(strikes) < 3 {
		if len(strikes) > 0 {
			var sum float64
			for _, iv := range ivs {
				sum += iv
			}
			return smile{F: F, A: sum / float64(len(ivs))}
		}
		return smile{F: F}
	}
	var s0, s1, s2, s3, s4 float64
	var t0, t1, t2 float64
	for i, K := range strikes {
		d := K - F
		iv := ivs[i]
		d2 := d * d
		s0 += 1
		s1 += d
		s2 += d2
		s3 += d * d2
		s4 += d2 * d2
		t0 += iv
		t1 += d * iv
		t2 += d2 * iv
	}
	det := s0*(s2*s4-s3*s3) - s1*(s1*s4-s3*s2) + s2*(s1*s3-s2*s2)
	if math.Abs(det) < 1e-30 {
		return smile{F: F, A: t0 / s0}
	}
	a := (t0*(s2*s4-s3*s3) - s1*(t1*s4-s3*t2) + s2*(t1*s3-s2*t2)) / det
	b := (s0*(t1*s4-s3*t2) - t0*(s1*s4-s3*s2) + s2*(s1*t2-t1*s2)) / det
	c := (s0*(s2*t2-t1*s3) - s1*(s1*t2-t1*s2) + t0*(s1*s3-s2*s2)) / det
	return smile{F: F, A: a, B: b, C: c}
}

type optionDef struct {
	Strike float64
	IsCall bool
}

// optionState tracks the latest observed state per instrument.
type optionState struct {
	mid   float64     // last observed market mid
	es    float64     // ES price when last quote arrived
	iv    float64     // implied vol at last quote
	delta float64     // Black-76 delta at last quote
	ts    clocky.Time // timestamp of last quote
}

type priceSample struct {
	ts  clocky.Time
	mid float64
}

// bucket accumulates prediction errors.
type bucket struct {
	name     string
	count    int
	deltaSq  float64
	smileSq  float64
	deltaSum float64
	smileSum float64
}

func (b *bucket) add(deltaErr, smileErr float64) {
	b.count++
	b.deltaSq += deltaErr * deltaErr
	b.smileSq += smileErr * smileErr
	b.deltaSum += math.Abs(deltaErr)
	b.smileSum += math.Abs(smileErr)
}

func main() {
	flag.Parse()
	if *dateFlag == "" {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/black76 -date 2026-03-13 [-time 09:30] [-duration 1h]\n")
		os.Exit(1)
	}

	dataDir := *dirFlag
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), "databento")
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("load timezone: %v", err)
	}
	startTime, err := time.ParseInLocation("2006-01-02 15:04", *dateFlag+" "+*timeFlag, loc)
	if err != nil {
		log.Fatalf("parse time: %v", err)
	}
	startTS := clocky.Time(startTime.UnixNano())
	endTime := startTime.Add(*durationFlag)
	endTS := clocky.Time(endTime.UnixNano())
	expiryTime := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 16, 0, 0, 0, loc)
	expiryTS := clocky.Time(expiryTime.UnixNano())

	// 1. Load ES futures prices (downsampled to 1/sec)
	log.Printf("loading ES futures data...")
	esSamples := loadFuturesPrices(filepath.Join(dataDir, "ES", *dateFlag+".mbp-1.dbn"), endTS)
	if len(esSamples) == 0 {
		log.Fatal("no ES price data found")
	}
	log.Printf("loaded %d ES samples", len(esSamples))

	// 2. Load SR1 rate
	log.Printf("loading SR1 rate data...")
	sr1Samples := loadFuturesPrices(filepath.Join(dataDir, "SR1", *dateFlag+".mbp-1.dbn"), endTS)
	var rate float64
	if len(sr1Samples) > 0 {
		sr1Price := sr1Samples[len(sr1Samples)-1].mid
		rate = (100.0 - sr1Price) / 100.0
		log.Printf("SR1 rate: %.4f%%", rate*100)
	} else {
		rate = 0.04
		log.Printf("warning: no SR1 data, using default rate %.2f%%", rate*100)
	}

	// 3. Load SPXW definitions (0DTE only)
	log.Printf("loading SPXW definitions...")
	targetYear, targetMonth, targetDay := startTime.Date()
	defs := loadOptionDefs(
		filepath.Join(dataDir, "SPXW", *dateFlag+".definition.dbn"),
		targetYear, targetMonth, targetDay,
	)
	log.Printf("loaded %d 0DTE option definitions", len(defs))

	// 4. Stream SPXW quotes and track predictions
	log.Printf("streaming SPXW quotes %s - %s ET...", startTime.Format("15:04"), endTime.Format("15:04"))

	states := make(map[uint32]*optionState) // per-instrument state
	var currentSmile smile                   // current fitted smile
	var esIdx int                            // cursor into esSamples
	var esPrice float64                      // latest ES price
	var esMin, esMax float64                 // ES range during window

	// Staleness buckets
	staleBuckets := []*bucket{
		{name: "< 100ms"},
		{name: "100-500ms"},
		{name: "500ms-2s"},
		{name: "2-10s"},
		{name: "> 10s"},
	}

	// Moneyness buckets (|ln(K/F)|)
	moneyBuckets := []*bucket{
		{name: "ATM (< 0.5%)"},
		{name: "0.5-1%"},
		{name: "1-2%"},
		{name: "2-5%"},
		{name: "> 5%"},
	}

	staleBucket := func(d time.Duration) *bucket {
		switch {
		case d < 100*time.Millisecond:
			return staleBuckets[0]
		case d < 500*time.Millisecond:
			return staleBuckets[1]
		case d < 2*time.Second:
			return staleBuckets[2]
		case d < 10*time.Second:
			return staleBuckets[3]
		default:
			return staleBuckets[4]
		}
	}

	moneyBucket := func(absLogMoney float64) *bucket {
		switch {
		case absLogMoney < 0.005:
			return moneyBuckets[0]
		case absLogMoney < 0.01:
			return moneyBuckets[1]
		case absLogMoney < 0.02:
			return moneyBuckets[2]
		case absLogMoney < 0.05:
			return moneyBuckets[3]
		default:
			return moneyBuckets[4]
		}
	}

	// Advance ES cursor to the given timestamp.
	advanceES := func(ts clocky.Time) {
		for esIdx < len(esSamples)-1 && esSamples[esIdx+1].ts <= ts {
			esIdx++
		}
		if esIdx < len(esSamples) {
			esPrice = esSamples[esIdx].mid
		}
	}

	// Recalibrate smile: recompute IVs at the CURRENT ES price from
	// last observed market mids, then fit the quadratic.
	var smileStrikes, smileIVs []float64
	recalibrate := func(now clocky.Time) {
		smileStrikes = smileStrikes[:0]
		smileIVs = smileIVs[:0]
		T := float64(expiryTS-now) / (365.25 * 24 * 3600 * 1e9)
		if T <= 0 {
			return
		}
		for id, st := range states {
			def := defs[id]
			if st.mid <= 0 {
				continue
			}
			if math.Abs(def.Strike-esPrice) > *rangeFlag {
				continue
			}
			// OTM only: puts below ATM, calls above ATM
			otmCall := def.IsCall && def.Strike >= esPrice
			otmPut := !def.IsCall && def.Strike <= esPrice
			if !otmCall && !otmPut {
				continue
			}
			// Recompute IV at current ES price from last observed mid
			// Adjust mid for ES move: mid + delta * (currentES - ES_at_quote)
			adjMid := st.mid + st.delta*(esPrice-st.es)
			if adjMid <= 0 {
				continue
			}
			iv := black76.IV(esPrice, def.Strike, rate, T, adjMid, def.IsCall)
			if iv <= 0.01 || iv > 3.0 {
				continue
			}
			smileStrikes = append(smileStrikes, def.Strike)
			smileIVs = append(smileIVs, iv)
		}
		currentSmile = fitSmile(esPrice, smileStrikes, smileIVs)
	}

	// Stream SPXW
	r, err := databento.OpenFile(filepath.Join(dataDir, "SPXW", *dateFlag+".cmbp-1.dbn"))
	if err != nil {
		log.Fatalf("open SPXW cmbp-1: %v", err)
	}
	defer r.Close()

	var nRecords, nUpdates, nPredictions int
	var recalibN int
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("read SPXW: %v", err)
		}
		nRecords++
		if nRecords%50_000_000 == 0 {
			log.Printf("  processed %dM records, %d predictions...", nRecords/1_000_000, nPredictions)
		}

		cmbp, ok := rec.(*databento.CMBP1)
		if !ok {
			continue
		}
		if cmbp.TSRecv > endTS {
			break
		}
		if cmbp.TSRecv < startTS {
			// Before analysis window: still update states for warm-up
			def, ok := defs[cmbp.Header.InstrumentID]
			if !ok {
				continue
			}
			bid := dbnToFloat(cmbp.Levels[0].BidPx)
			ask := dbnToFloat(cmbp.Levels[0].AskPx)
			if bid <= 0 || ask <= 0 {
				continue
			}
			advanceES(cmbp.TSRecv)
			mid := (bid + ask) / 2
			T := float64(expiryTS-cmbp.TSRecv) / (365.25 * 24 * 3600 * 1e9)
			if T <= 0 {
				continue
			}
			iv := black76.IV(esPrice, def.Strike, rate, T, mid, def.IsCall)
			var delta float64
			if def.IsCall {
				delta = black76.CallDelta(esPrice, def.Strike, rate, T, iv)
			} else {
				delta = black76.PutDelta(esPrice, def.Strike, rate, T, iv)
			}
			st := states[cmbp.Header.InstrumentID]
			if st == nil {
				st = &optionState{}
				states[cmbp.Header.InstrumentID] = st
			}
			st.mid = mid
			st.es = esPrice
			st.iv = iv
			st.delta = delta
			st.ts = cmbp.TSRecv
			continue
		}

		// Inside analysis window
		def, ok := defs[cmbp.Header.InstrumentID]
		if !ok {
			continue
		}
		bid := dbnToFloat(cmbp.Levels[0].BidPx)
		ask := dbnToFloat(cmbp.Levels[0].AskPx)
		if bid <= 0 || ask <= 0 {
			continue
		}

		advanceES(cmbp.TSRecv)
		if esPrice <= 0 {
			continue
		}
		if esMin == 0 || esPrice < esMin {
			esMin = esPrice
		}
		if esPrice > esMax {
			esMax = esPrice
		}

		mid := (bid + ask) / 2
		T := float64(expiryTS-cmbp.TSRecv) / (365.25 * 24 * 3600 * 1e9)
		if T <= 0 {
			continue
		}

		// Skip strikes outside range
		if math.Abs(def.Strike-esPrice) > *rangeFlag {
			continue
		}

		nUpdates++

		// If we have a previous state, compute prediction errors
		st := states[cmbp.Header.InstrumentID]
		if st != nil && st.mid > 0 && st.ts > 0 {
			staleness := time.Duration(cmbp.TSRecv-st.ts) * time.Nanosecond

			// Delta prediction: old_mid + delta * (new_ES - old_ES)
			deltaPred := st.mid + st.delta*(esPrice-st.es)
			deltaErr := deltaPred - mid

			// Smile prediction: Black76(new_ES, K, r, T, smile_vol)
			smileVol := currentSmile.Vol(def.Strike, esPrice)
			var smilePred float64
			if smileVol > 0 && currentSmile.A > 0 {
				if def.IsCall {
					smilePred = black76.Call(esPrice, def.Strike, rate, T, smileVol)
				} else {
					smilePred = black76.Put(esPrice, def.Strike, rate, T, smileVol)
				}
			} else {
				smilePred = deltaPred // fall back to delta if no smile yet
			}
			smileErr := smilePred - mid

			staleBucket(staleness).add(deltaErr, smileErr)
			absLogMoney := math.Abs(math.Log(def.Strike / esPrice))
			moneyBucket(absLogMoney).add(deltaErr, smileErr)
			nPredictions++
		}

		// Update state
		iv := black76.IV(esPrice, def.Strike, rate, T, mid, def.IsCall)
		var delta float64
		if def.IsCall {
			delta = black76.CallDelta(esPrice, def.Strike, rate, T, iv)
		} else {
			delta = black76.PutDelta(esPrice, def.Strike, rate, T, iv)
		}
		if st == nil {
			st = &optionState{}
			states[cmbp.Header.InstrumentID] = st
		}
		st.mid = mid
		st.es = esPrice
		st.iv = iv
		st.delta = delta
		st.ts = cmbp.TSRecv

		// Recalibrate smile periodically
		recalibN++
		if recalibN%10000 == 0 {
			recalibrate(cmbp.TSRecv)
		}
	}

	// Final calibration for reporting
	recalibrate(endTS)

	// Report
	fmt.Printf("\nBlack-76 Real-Time Fair Value Analysis\n")
	fmt.Printf("======================================\n")
	fmt.Printf("Date:            %s\n", *dateFlag)
	fmt.Printf("Window:          %s - %s ET\n", startTime.Format("15:04"), endTime.Format("15:04"))
	fmt.Printf("ES Range:        %.2f - %.2f\n", esMin, esMax)
	fmt.Printf("SR1 Rate:        %.4f%%\n", rate*100)
	fmt.Printf("0DTE Options:    %d defined\n", len(defs))
	fmt.Printf("Quote Updates:   %d (within ±%.0f of ATM)\n", nUpdates, *rangeFlag)
	fmt.Printf("Predictions:     %d\n", nPredictions)
	fmt.Printf("Smile (final):   σ(K) = %.4f %+.6f·d %+.8f·d²  (d = K - F)\n",
		currentSmile.A, currentSmile.B, currentSmile.C)

	fmt.Printf("\nPrediction Accuracy by Quote Staleness:\n")
	printBucketTable(staleBuckets)

	fmt.Printf("\nPrediction Accuracy by Moneyness |ln(K/F)|:\n")
	printBucketTable(moneyBuckets)
}

func printBucketTable(buckets []*bucket) {
	fmt.Printf("  %-14s %7s %11s %11s %9s\n", "Bucket", "Count", "Delta RMSE", "Smile RMSE", "Winner")
	fmt.Printf("  %-14s %7s %11s %11s %9s\n", "--------------", "-------", "-----------", "-----------", "---------")
	for _, b := range buckets {
		if b.count == 0 {
			continue
		}
		n := float64(b.count)
		dRMSE := math.Sqrt(b.deltaSq / n)
		sRMSE := math.Sqrt(b.smileSq / n)
		winner := "delta"
		if sRMSE < dRMSE {
			winner = "smile"
		}
		fmt.Printf("  %-14s %7d %11.4f %11.4f %9s\n", b.name, b.count, dRMSE, sRMSE, winner)
	}
}

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

func loadOptionDefs(path string, year int, month time.Month, day int) map[uint32]*optionDef {
	r, err := databento.OpenFile(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer r.Close()
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
		sym := inst.GetRawSymbol()
		if len(sym) < 12 || sym[6:12] != dateStr {
			continue
		}
		strike := dbnToFloat(inst.StrikePrice)
		if strike <= 0 {
			continue
		}
		options[inst.Header.InstrumentID] = &optionDef{
			Strike: strike,
			IsCall: inst.InstrumentClass == databento.InstrumentClassCall,
		}
	}
	return options
}
