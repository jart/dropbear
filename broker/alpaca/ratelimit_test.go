package alpaca

import (
	"flag"
	"net/http"
	"strconv"
	"testing"
	"time"
)

var flagRatelimitStudy = flag.Bool("ratelimit-study", false, "run alpaca rate limit study (makes real API calls)")

// TestRatelimitStudy analyzes Alpaca's rate limiting behavior to determine
// optimal TokenBucket configuration.
//
// Run with: go test ./broker/alpaca -v -ratelimit-study
//
// Alpaca rate limits (per docs):
//   - Free tier: 200 calls/min
//   - Algo Trader Plus: 200 calls/min (but unlimited via websocket)
//
// Headers returned:
//   - X-Ratelimit-Limit: max requests per window
//   - X-Ratelimit-Remaining: remaining requests in window
//   - X-Ratelimit-Reset: unix timestamp when window resets
func TestRatelimitStudy(t *testing.T) {
	if !*flagRatelimitStudy {
		t.Skip("skipping rate limit study (use -ratelimit-study to run)")
	}

	client := NewClient()
	dataURL := "https://data.alpaca.markets"

	type sample struct {
		time      time.Time
		limit     int
		remaining int
		reset     time.Time
		latency   time.Duration
	}

	var samples []sample

	// Make requests and collect rate limit info
	t.Log("Starting rate limit study...")
	t.Log("Making requests until we see rate limit behavior...")

	endpoint := "/v2/stocks/AAPL/bars?timeframe=1Min&start=2025-01-02T14:30:00Z&end=2025-01-02T14:31:00Z&feed=sip&limit=1"

	for i := 0; i < 250; i++ {
		start := time.Now()
		resp, err := dataRequestForTest(client, dataURL+endpoint)
		latency := time.Since(start)

		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		s := sample{
			time:    start,
			latency: latency,
		}

		if v := resp.Header.Get("X-Ratelimit-Limit"); v != "" {
			s.limit, _ = strconv.Atoi(v)
		}
		if v := resp.Header.Get("X-Ratelimit-Remaining"); v != "" {
			s.remaining, _ = strconv.Atoi(v)
		}
		if v := resp.Header.Get("X-Ratelimit-Reset"); v != "" {
			unix, _ := strconv.ParseInt(v, 10, 64)
			s.reset = time.Unix(unix, 0)
		}

		resp.Body.Close()

		samples = append(samples, s)

		if resp.StatusCode == 429 {
			t.Logf("Hit rate limit at request %d", i)
			retryAfter := resp.Header.Get("Retry-After")
			t.Logf("Retry-After: %s", retryAfter)
			break
		}

		// Log every 10 requests
		if i%10 == 0 {
			t.Logf("req=%3d status=%d remaining=%3d reset=%s latency=%v",
				i, resp.StatusCode, s.remaining, s.reset.Format("15:04:05"), s.latency)
		}

		// Small delay to not be too aggressive
		time.Sleep(10 * time.Millisecond)
	}

	// Analyze the data
	t.Log("\n=== Analysis ===")

	if len(samples) < 2 {
		t.Fatal("not enough samples")
	}

	// Find the limit
	limit := samples[0].limit
	t.Logf("Rate limit: %d requests per window", limit)

	// Calculate window duration by looking at reset times
	var windowDurations []time.Duration
	var lastReset time.Time
	for _, s := range samples {
		if !s.reset.IsZero() && !lastReset.IsZero() && s.reset != lastReset {
			windowDurations = append(windowDurations, s.reset.Sub(lastReset))
		}
		if !s.reset.IsZero() {
			lastReset = s.reset
		}
	}

	if len(windowDurations) > 0 {
		t.Logf("Observed window changes: %v", windowDurations)
	}

	// Calculate effective rate
	if len(samples) >= 2 {
		first := samples[0]
		last := samples[len(samples)-1]
		elapsed := last.time.Sub(first.time)
		requestsMade := len(samples)
		effectiveRate := float64(requestsMade) / elapsed.Seconds()
		t.Logf("Made %d requests in %v = %.2f req/sec", requestsMade, elapsed, effectiveRate)
	}

	// Look at remaining token depletion
	t.Log("\n=== Token Depletion Pattern ===")
	prevRemaining := -1
	for i, s := range samples {
		if s.remaining != prevRemaining {
			t.Logf("req=%3d remaining=%3d (delta=%d)", i, s.remaining, prevRemaining-s.remaining)
			prevRemaining = s.remaining
		}
	}

	// Calculate refill rate if we saw a refill
	t.Log("\n=== Recommended TokenBucket Config ===")
	// Alpaca uses 200/minute = 3.33/sec
	ratePerSec := float64(limit) / 60.0
	t.Logf("Limit: %d per minute", limit)
	t.Logf("Rate: %.2f per second", ratePerSec)
	t.Logf("Burst: %d", limit)
	t.Log("")
	t.Logf("Recommended config:")
	t.Logf("  limiter := ds.NewTokenBucket(%.2f, %d)", ratePerSec, limit)
	t.Log("")
	t.Log("Or more conservatively (90%% of limit):")
	t.Logf("  limiter := ds.NewTokenBucket(%.2f, %d)", ratePerSec*0.9, int(float64(limit)*0.9))
}

// TestRatelimitBurst tests burst behavior - how many requests can we make instantly
func TestRatelimitBurst(t *testing.T) {
	if !*flagRatelimitStudy {
		t.Skip("skipping rate limit study (use -ratelimit-study to run)")
	}

	client := NewClient()
	dataURL := "https://data.alpaca.markets"
	endpoint := "/v2/stocks/AAPL/bars?timeframe=1Min&start=2025-01-02T14:30:00Z&end=2025-01-02T14:31:00Z&feed=sip&limit=1"

	t.Log("Testing burst capacity - making rapid requests...")

	start := time.Now()
	var lastRemaining int
	var hitLimit bool

	for i := 0; i < 300; i++ {
		resp, err := dataRequestForTest(client, dataURL+endpoint)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		remaining, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
		resp.Body.Close()

		if resp.StatusCode == 429 {
			elapsed := time.Since(start)
			t.Logf("Hit 429 at request %d after %v (%.1f req/sec)", i, elapsed, float64(i)/elapsed.Seconds())
			hitLimit = true
			break
		}

		lastRemaining = remaining

		// No delay - burst as fast as possible
	}

	if !hitLimit {
		elapsed := time.Since(start)
		t.Logf("Made 300 requests in %v without hitting limit", elapsed)
		t.Logf("Final remaining: %d", lastRemaining)
	}
}

// TestRatelimitRecovery tests how quickly tokens recover after depletion
func TestRatelimitRecovery(t *testing.T) {
	if !*flagRatelimitStudy {
		t.Skip("skipping rate limit study (use -ratelimit-study to run)")
	}

	client := NewClient()
	dataURL := "https://data.alpaca.markets"
	endpoint := "/v2/stocks/AAPL/bars?timeframe=1Min&start=2025-01-02T14:30:00Z&end=2025-01-02T14:31:00Z&feed=sip&limit=1"

	t.Log("Testing recovery rate...")

	// First, make some requests to deplete tokens
	t.Log("Phase 1: Depleting tokens...")
	for i := 0; i < 50; i++ {
		resp, err := dataRequestForTest(client, dataURL+endpoint)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		remaining, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
		resp.Body.Close()
		if i%10 == 0 {
			t.Logf("After %d requests: remaining=%d", i+1, remaining)
		}
	}

	// Now wait and check recovery
	t.Log("\nPhase 2: Monitoring recovery...")
	for i := 0; i < 12; i++ {
		time.Sleep(5 * time.Second)

		resp, err := dataRequestForTest(client, dataURL+endpoint)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		remaining, _ := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
		reset := resp.Header.Get("X-Ratelimit-Reset")
		resp.Body.Close()

		t.Logf("After %ds wait: remaining=%d reset=%s", (i+1)*5, remaining, reset)
	}
}

func dataRequestForTest(_ *Client, url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	key, secret := GetKey()
	req.Header.Set("APCA-API-KEY-ID", key)
	req.Header.Set("APCA-API-SECRET-KEY", secret)
	return http.DefaultClient.Do(req)
}
