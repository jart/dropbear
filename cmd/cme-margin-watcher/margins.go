package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CME's internal API endpoint used by their website's margin tables.
// Returns a JSON array of margin rows for a given product code.
// This is undocumented/unofficial — if it breaks, SPAN monitoring still works.
//
// Alternative scrape target if this endpoint stops working:
//
//	https://www.cmegroup.com/markets/energy/crude-oil/light-sweet-crude.margins.html
//
// (but that page is JS-rendered, so you'd need chromedp)
// CME product codes → exchange mapping for the margins API.
// Most NYMEX energy products live under exchange=NYMEX.
// Gold/Silver are COMEX. Adjust if needed.
var productExchanges = map[string]string{
	"CL":  "NYMEX",
	"BRN": "NYMEX",
	"NG":  "NYMEX",
	"GC":  "COMEX",
	"SI":  "COMEX",
}

// marginRow mirrors what CME's API returns per contract month.
// Field names may need adjustment if CME changes their JSON schema.
type marginRow struct {
	ContractMonth  string `json:"contractMonth"`
	MaintenanceAmt string `json:"maintenanceAmt"` // e.g. "8500"
	InitialAmt     string `json:"initialAmt"`
	ProductCode    string `json:"productCode"`
}

type marginResponse struct {
	Rows []marginRow `json:"rows"`
}

func watchMargins(cfg Config, state *State, push *Pushover) {
	log.Println("margins: starting CME margins API watcher")
	ticker := time.NewTicker(cfg.MarginsInterval)
	defer ticker.Stop()

	checkMargins(cfg, state, push)
	for range ticker.C {
		checkMargins(cfg, state, push)
	}
}

func checkMargins(cfg Config, state *State, push *Pushover) {
	for _, product := range cfg.WatchProducts {
		maint, err := fetchMaintenanceMargin(product)
		if err != nil {
			log.Printf("margins: %s fetch error: %v", product, err)
			continue
		}
		if maint == "" {
			continue
		}

		changed, oldVal := state.updateMargin(product, maint)
		log.Printf("margins: %s maintenance = %s (was %s)", product, maint, oldVal)

		if !changed {
			continue
		}

		dir := "↑ RAISED"
		if dollarLess(maint, oldVal) {
			dir = "↓ LOWERED"
		}
		err = push.Sendf(PriorityHigh,
			fmt.Sprintf("💰 CME Margin %s: %s", dir, product),
			"%s spot-month maintenance margin:\n$%s → $%s per contract",
			product, oldVal, maint,
		)
		if err != nil {
			log.Printf("margins: pushover error: %v", err)
		}
		state.save()
	}
}

// fetchMaintenanceMargin returns the spot-month maintenance margin for a product.
// Returns "" if the API is unavailable or the data can't be parsed.
func fetchMaintenanceMargin(product string) (string, error) {
	exchange, ok := productExchanges[product]
	if !ok {
		exchange = "NYMEX"
	}

	url := fmt.Sprintf("https://www.cmegroup.com/CmeWS/mvc/Margins/Initial?productCode=%s&exchange=%s",
		product, exchange)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	// Mimic a browser to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", "https://www.cmegroup.com/")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 403 {
		// API unavailable — not an error, just silent fallback to SPAN
		return "", nil
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Try standard JSON response
	var apiResp marginResponse
	if err := json.Unmarshal(body, &apiResp); err == nil && len(apiResp.Rows) > 0 {
		// Return spot month (first row) maintenance margin
		return strings.TrimSpace(apiResp.Rows[0].MaintenanceAmt), nil
	}

	// Sometimes CME returns a bare JSON array
	var rows []marginRow
	if err := json.Unmarshal(body, &rows); err == nil && len(rows) > 0 {
		return strings.TrimSpace(rows[0].MaintenanceAmt), nil
	}

	// Log what we actually got for debugging
	preview := string(body)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	log.Printf("margins: %s unexpected response: %s", product, preview)
	return "", nil
}

// dollarLess returns true if the dollar string a < b (simple numeric compare).
func dollarLess(a, b string) bool {
	var fa, fb float64
	fmt.Sscanf(strings.ReplaceAll(a, ",", ""), "%f", &fa)
	fmt.Sscanf(strings.ReplaceAll(b, ",", ""), "%f", &fb)
	return fa < fb
}
