package kraken

import (
	"dropbear/decimal"
	"dropbear/ds"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Manual rate limit probing tests for Kraken.
//
// These tests hit the live Kraken API to determine actual rate limits.
// They require valid API credentials in kraken.json.
//
// Run with:
//   go test -v ./broker/kraken -run TestProbe -kraken-key=kraken.json
//
// Individual probes:
//   go test -v ./broker/kraken -run TestProbeRESTRateLimit
//   go test -v ./broker/kraken -run TestProbeTradingRateLimit
//   go test -v ./broker/kraken -run TestProbePublicLatency

var probeFlag = flag.Bool("probe", false, "run live rate limit probing tests")

// unlimitedBucket bypasses rate limiting for probing tests.
type unlimitedBucket struct{}

func (unlimitedBucket) Close() error { return nil }
func (unlimitedBucket) Try() bool    { return true }
func (unlimitedBucket) Get()         {}

func skipIfNotProbing(t *testing.T) {
	if !*probeFlag {
		t.Skip("skipping live probe test; use -probe flag to enable")
	}
}

// TestProbePublicLatency measures baseline latency to Kraken's public API.
// This helps establish network conditions before probing rate limits.
func TestProbePublicLatency(t *testing.T) {
	skipIfNotProbing(t)

	url := "https://api.kraken.com/0/public/Time"
	var latencies []time.Duration

	for i := 0; i < 10; i++ {
		start := time.Now()
		resp, err := http.Get(url)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
		latencies = append(latencies, elapsed)
		t.Logf("request %d: %v", i, elapsed)
	}

	var total time.Duration
	min, max := latencies[0], latencies[0]
	for _, l := range latencies {
		total += l
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
	}
	avg := total / time.Duration(len(latencies))
	t.Logf("latency: min=%v avg=%v max=%v", min, avg, max)
}

// TestProbeRESTRateLimit probes the REST API rate limit by making rapid requests.
// For Pro tier, expected: max 20 counter, decay 1/sec.
func TestProbeRESTRateLimit(t *testing.T) {
	skipIfNotProbing(t)

	// Create client without rate limiting so we can probe Kraken's actual limits
	client := &Client{restLimiter: unlimitedBucket{}, tradingLimiter: unlimitedBucket{}}

	// Phase 1: Burst until rate limited
	t.Log("Phase 1: Bursting to find limit...")
	var successCount int
	var rateLimitedAt time.Time

	for i := 0; i < 100; i++ {
		start := time.Now()
		_, err := client.GetBalance()
		elapsed := time.Since(start)

		if err != nil {
			if strings.Contains(err.Error(), "Rate limit") ||
				strings.Contains(err.Error(), "EAPI:Rate limit exceeded") {
				rateLimitedAt = time.Now()
				t.Logf("rate limited after %d requests (elapsed: %v)", successCount, elapsed)
				break
			}
			t.Logf("request %d error: %v", i, err)
		} else {
			successCount++
			t.Logf("request %d: ok (%v)", i, elapsed)
		}
	}

	if rateLimitedAt.IsZero() {
		t.Log("never rate limited in 100 requests - burst limit is at least 100")
		return
	}

	t.Logf("burst capacity: ~%d requests", successCount)

	// Phase 2: Measure decay rate
	t.Log("\nPhase 2: Measuring decay rate...")
	for wait := 1; wait <= 30; wait++ {
		time.Sleep(time.Duration(wait) * time.Second)
		elapsed := time.Since(rateLimitedAt)

		_, err := client.GetBalance()
		if err == nil {
			t.Logf("recovered after %v (decay rate: ~%.2f/sec)", elapsed, float64(successCount)/elapsed.Seconds())
			break
		}
		if !strings.Contains(err.Error(), "Rate limit") {
			t.Logf("different error after %v: %v", elapsed, err)
			break
		}
		t.Logf("still limited after %v", elapsed)
	}
}

// TestProbeTradingRateLimit probes the trading rate limit by placing/canceling orders.
// For Pro tier, expected: max 180 counter, decay 3.75/sec per pair.
//
// WARNING: This places real orders (far off-market limit orders that won't fill).
// Use with caution and monitor your account.
func TestProbeTradingRateLimit(t *testing.T) {
	skipIfNotProbing(t)

	// Create client without rate limiting so we can probe Kraken's actual limits
	client := &Client{restLimiter: unlimitedBucket{}, tradingLimiter: unlimitedBucket{}}

	// Get current BTC price to place orders far away from market
	ticker, err := client.GetTicker("XBTUSD")
	if err != nil {
		t.Fatalf("failed to get ticker: %v", err)
	}
	t.Logf("current BTC bid: %s", ticker.Bid[0])

	// We'll place limit buy orders far below market (won't fill)
	// Kraken minimum cost is ~$5, so we need volume * price >= 5
	// 0.001 BTC * $10000 = $10 (safe margin above minimum)
	pair := "XBTUSD"
	volume := "0.001"
	price := "10000" // $10k BTC buy order - won't fill at current ~$94k

	t.Log("Phase 1: Bursting AddOrder to find limit...")
	var orderIDs []string
	var successCount int
	var rateLimitedAt time.Time

	for i := 0; i < 250; i++ {
		start := time.Now()
		orderID, err := client.LimitOrder(pair, ds.SideBuy, decimal.Parse(volume), decimal.Parse(price), "", ds.OrderStrategyPostOnly)
		elapsed := time.Since(start)

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Rate limit") ||
				strings.Contains(errStr, "EOrder:Rate limit exceeded") {
				rateLimitedAt = time.Now()
				t.Logf("rate limited after %d orders (elapsed: %v)", successCount, elapsed)
				break
			}
			t.Logf("order %d error: %v", i, err)
			// Don't count API errors as rate limits
			continue
		}
		successCount++
		orderIDs = append(orderIDs, orderID)
		if successCount%10 == 0 {
			t.Logf("placed %d orders...", successCount)
		}
	}

	t.Logf("trading burst capacity: ~%d orders", successCount)

	// Cancel all orders we placed
	t.Logf("\nCanceling %d orders...", len(orderIDs))
	if len(orderIDs) > 0 {
		// Wait a bit to avoid cancel penalties
		time.Sleep(5 * time.Second)
		err := client.CancelAllOrders()
		if err != nil {
			t.Logf("cancel all orders error: %v", err)
		} else {
			t.Log("all orders canceled")
		}
	}

	if !rateLimitedAt.IsZero() {
		// Phase 2: Measure decay
		t.Log("\nPhase 2: Measuring trading decay rate...")
		for wait := 5; wait <= 60; wait += 5 {
			time.Sleep(time.Duration(wait) * time.Second)
			elapsed := time.Since(rateLimitedAt)

			orderID, err := client.LimitOrder(pair, ds.SideBuy, decimal.Parse(volume), decimal.Parse(price), "", ds.OrderStrategyPostOnly)
			if err == nil {
				t.Logf("recovered after %v (decay rate: ~%.2f/sec)", elapsed, float64(successCount)/elapsed.Seconds())
				// Cancel this order
				client.CancelOrder(orderID)
				break
			}
			if !strings.Contains(err.Error(), "Rate limit") {
				t.Logf("different error after %v: %v", elapsed, err)
				break
			}
			t.Logf("still limited after %v", elapsed)
		}
	}
}

// TestProbeOpenOrderLimit probes the maximum open orders per pair.
// For Pro tier, expected: max 225 open orders per pair.
func TestProbeOpenOrderLimit(t *testing.T) {
	skipIfNotProbing(t)

	// Create client without rate limiting so we can probe Kraken's actual limits
	client := &Client{restLimiter: unlimitedBucket{}, tradingLimiter: unlimitedBucket{}}

	// 0.001 BTC * $10000 = $10 (safe margin above ~$5 minimum)
	pair := "XBTUSD"
	volume := "0.001"
	basePrice := 10000.0 // $10k starting price - won't fill

	t.Log("Placing orders to find open order limit...")
	var orderIDs []string

	for i := 0; i < 300; i++ {
		// Vary price slightly to avoid duplicate order rejection
		price := fmt.Sprintf("%.1f", basePrice+float64(i)*0.1)
		orderID, err := client.LimitOrder(pair, ds.SideBuy, decimal.Parse(volume), decimal.Parse(price), "", ds.OrderStrategyPostOnly)

		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Orders limit exceeded") ||
				strings.Contains(errStr, "EOrder:Orders limit exceeded") {
				t.Logf("open order limit reached at %d orders", len(orderIDs))
				break
			}
			if strings.Contains(errStr, "Rate limit") {
				t.Logf("rate limited at %d orders, waiting...", len(orderIDs))
				time.Sleep(10 * time.Second)
				i-- // retry
				continue
			}
			t.Logf("order %d error: %v", i, err)
			continue
		}
		orderIDs = append(orderIDs, orderID)
		if len(orderIDs)%25 == 0 {
			t.Logf("placed %d orders...", len(orderIDs))
		}
	}

	t.Logf("max open orders: ~%d", len(orderIDs))

	// Cleanup
	t.Logf("\nCanceling %d orders...", len(orderIDs))
	time.Sleep(5 * time.Second) // avoid cancel penalty
	err := client.CancelAllOrders()
	if err != nil {
		t.Logf("cancel error: %v", err)
	}
}
