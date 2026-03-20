package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CME publishes SPAN parameter files here (no auth required, public).
//
// File naming convention:
//
//	nymex.YYYYMMDD.i.zip  — intraday (published ~10:00, 12:00, 14:00 CT)
//	nymex.YYYYMMDD.s.zip  — settlement (published ~4:30 PM CT)
//
// SPAN 1.x "B" records contain the price scan range for each contract series.
// The "P" / type-2 combined commodity records hold the performance bond level.
// We parse "B" records: each line starts with "B", followed by exchange+product,
// and a 7-char integer price scan range in units of the contract's price basis
// (for CL: dollars * 100, so 500000 = $5000/contract).
//
// If CME ever switches to SPAN XML format for NYMEX energy, the file will have
// a .xml extension inside the zip — parseSPANXML (below) handles that.
const spanBaseURL = "https://www.cmegroup.com/ftp/span/data/nymex/"

// SPAN 1.x unpacked "B" record layout (0-indexed, fixed-width):
//
//	[0]     = 'B'  (record type)
//	[1..4]  = exchange acronym (4 chars, e.g. "NYM ")
//	[5..16] = combined commodity code (12 chars, e.g. "CL          ")
//	[17..23]= price scan range (7 chars, right-justified integer)
//
// The price scan range for CL is in hundredths of a dollar per contract
// (i.e., divide by 100 to get dollars). Verify against the margins page on
// first run — if the values look wrong, adjust spanScanRangeDivisor.
const (
	spanRecordType     = 0
	spanExchangeStart  = 1
	spanExchangeLen    = 4
	spanCombCommStart  = 5
	spanCombCommLen    = 12
	spanScanRangeStart = 17
	spanScanRangeLen   = 7
	spanMinRecordLen   = spanScanRangeStart + spanScanRangeLen

	// Divide raw scan range by this to get approximate dollar margin.
	// CL: 1 contract = 1000 bbl; scan range in cents/bbl → dollars
	// So $5000 margin ≈ raw value 500000. Divisor = 100.
	// Adjust per product if needed.
	spanScanRangeDivisor = 100
)

func watchSPAN(cfg Config, state *State, push *Pushover) {
	log.Println("span: starting SPAN file watcher")
	ticker := time.NewTicker(cfg.SPANInterval)
	defer ticker.Stop()

	checkSPAN(cfg, state, push)
	for range ticker.C {
		checkSPAN(cfg, state, push)
	}
}

func checkSPAN(cfg Config, state *State, push *Pushover) {
	scanRanges, err := fetchSPANScanRanges(cfg.WatchProducts)
	if err != nil {
		log.Printf("span: fetch error: %v", err)
		return
	}

	for product, newVal := range scanRanges {
		changed, oldVal := state.updateSPAN(product, newVal)
		dollars := newVal / spanScanRangeDivisor
		log.Printf("span: %s scan range = %d (≈$%d/contract)", product, newVal, dollars)

		if !changed {
			continue
		}

		dir := "↑ RAISED"
		if newVal < oldVal {
			dir = "↓ LOWERED"
		}
		oldDollars := oldVal / spanScanRangeDivisor
		err := push.Sendf(PriorityHigh,
			fmt.Sprintf("📊 CME SPAN Margin %s: %s", dir, product),
			"%s margin: $%d → $%d per contract\n(raw scan range: %d → %d)",
			product, oldDollars, dollars, oldVal, newVal,
		)
		if err != nil {
			log.Printf("span: pushover error: %v", err)
		}
		state.save()
	}
}

// fetchSPANScanRanges tries intraday then settlement SPAN file for today.
// Returns map[productCode]scanRange.
func fetchSPANScanRanges(products []string) (map[string]int64, error) {
	today := time.Now()

	// Also try yesterday in case today's file isn't published yet (pre-open)
	for _, t := range []time.Time{today, today.Add(-24 * time.Hour)} {
		dateStr := t.Format("20060102")
		for _, suffix := range []string{"i2", "i1", "i", "s"} {
			filename := fmt.Sprintf("nymex.%s.%s.zip", dateStr, suffix)
			url := spanBaseURL + filename

			data, err := httpGetBytes(url)
			if err != nil {
				continue // file doesn't exist for this time slot
			}
			log.Printf("span: parsing %s", filename)

			ranges, err := parseSPANZip(data, products)
			if err != nil {
				log.Printf("span: parse error for %s: %v", filename, err)
				continue
			}
			if len(ranges) > 0 {
				return ranges, nil
			}
		}
	}
	return nil, fmt.Errorf("no SPAN file found for %s or %s",
		today.Format("2006-01-02"),
		today.Add(-24*time.Hour).Format("2006-01-02"))
}

// parseSPANZip unzips and parses the SPAN file, returning scan ranges for wanted products.
func parseSPANZip(zipData []byte, products []string) (map[string]int64, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("unzip: %w", err)
	}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, ".xml") {
			// SPAN XML format (newer)
			return parseSPANXML(string(content), products)
		}
		// Classic positional format
		return parseSPANPositional(string(content), products)
	}
	return nil, fmt.Errorf("zip contained no usable files")
}

// parseSPANPositional parses SPAN 1.x unpacked positional format.
// Type "B" records hold per-series parameters including price scan range.
// We use Type "P" (combined commodity) records if present; fall back to "B".
func parseSPANPositional(content string, products []string) (map[string]int64, error) {
	productSet := make(map[string]bool)
	for _, p := range products {
		productSet[p] = true
	}

	result := make(map[string]int64)

	for _, line := range strings.Split(content, "\n") {
		if len(line) < spanMinRecordLen {
			continue
		}
		if line[spanRecordType] != 'B' && line[spanRecordType] != 'P' {
			continue
		}

		exchange := strings.TrimSpace(line[spanExchangeStart : spanExchangeStart+spanExchangeLen])
		combComm := strings.TrimSpace(line[spanCombCommStart : spanCombCommStart+spanCombCommLen])

		// NYMEX products: exchange code is "NYM " or "NYMX" or similar
		if !strings.HasPrefix(exchange, "NYM") && exchange != "NYMX" {
			continue
		}
		if !productSet[combComm] {
			continue
		}

		rawStr := strings.TrimSpace(line[spanScanRangeStart : spanScanRangeStart+spanScanRangeLen])
		val, err := strconv.ParseInt(rawStr, 10, 64)
		if err != nil {
			continue
		}
		if val > 0 {
			// Keep the highest scan range seen for this product (spot month is largest)
			if existing, ok := result[combComm]; !ok || val > existing {
				result[combComm] = val
			}
		}
	}

	return result, nil
}

// parseSPANXML handles SPAN XML format files (used by some newer CME products).
// Looks for <combinedCommodity> elements with productCode and priceScanRange attributes.
func parseSPANXML(content string, products []string) (map[string]int64, error) {
	productSet := make(map[string]bool)
	for _, p := range products {
		productSet[p] = true
	}

	result := make(map[string]int64)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		// Simple attribute extraction — avoids XML decoder overhead
		if !strings.Contains(line, "priceScanRange") {
			continue
		}

		code := xmlAttr(line, "productCode")
		if code == "" {
			code = xmlAttr(line, "ccCode")
		}
		if !productSet[code] {
			continue
		}

		rawStr := xmlAttr(line, "priceScanRange")
		val, err := strconv.ParseInt(rawStr, 10, 64)
		if err != nil {
			continue
		}
		if val > 0 {
			if existing, ok := result[code]; !ok || val > existing {
				result[code] = val
			}
		}
	}

	return result, nil
}

// xmlAttr extracts the value of a named XML attribute from a line of text.
// e.g. xmlAttr(`<foo bar="123">`, "bar") returns "123".
func xmlAttr(line, attr string) string {
	needle := attr + `="`
	idx := strings.Index(line, needle)
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := strings.Index(line[start:], `"`)
	if end == -1 {
		return ""
	}
	return line[start : start+end]
}

func httpGetBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("404 not found")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
