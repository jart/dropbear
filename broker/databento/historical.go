// HTTP client for Databento's historical API.
// Source: ~/vendor/databento-cpp/src/historical.cpp
// Source: ~/vendor/databento-cpp/src/detail/http_client.cpp
package databento

import (
	"dropbear/clocky"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

const historicalGateway = "https://hist.databento.com"

// HistoricalClient accesses Databento's historical market data API.
// Rate-limits outgoing connections to comply with Databento's policy
// of at most 5 new connections per second per IP.
type HistoricalClient struct {
	apiKey  ApiKey
	limiter netty.TokenBucket
}

// NewHistoricalClient creates a new historical API client.
func NewHistoricalClient(apiKey ApiKey) *HistoricalClient {
	return &HistoricalClient{
		apiKey:  apiKey,
		limiter: netty.NewTokenBucket(5, 5),
	}
}

// do executes an HTTP request with client-side rate limiting and
// exponential backoff retry on 429 and other retryable status codes.
// The caller is responsible for closing the response body on success.
// For POST requests, set req.GetBody so the body can be re-read on retry.
func (c *HistoricalClient) do(req *http.Request) (*http.Response, error) {
	for tries := 0; ; tries++ {
		if tries > 0 && req.GetBody != nil {
			var err error
			req.Body, err = req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("databento: reset body: %w", err)
			}
		}
		c.limiter.Get()
		resp, err := netty.BulkHttpClient.Do(req)
		if err != nil {
			if !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := clocky.Second << min(tries, 5)
			log.Printf("databento: %v, retrying in %v (attempt %d)", err, delay, tries+1)
			clocky.Sleep(delay)
			continue
		}
		if !netty.IsRetryableHTTPStatusCode(resp.StatusCode) {
			return resp, nil
		}
		resp.Body.Close()
		delay := clocky.Second << min(tries, 5)
		log.Printf("databento: got %d, retrying in %v (attempt %d)", resp.StatusCode, delay, tries+1)
		clocky.Sleep(delay)
	}
}

// getMetadataJSON performs a GET request to a metadata endpoint and
// decodes the JSON response into dest.
func (c *HistoricalClient) getMetadataJSON(path string, params map[string]string, dest any) error {
	req, err := http.NewRequest("GET", historicalGateway+path, nil)
	if err != nil {
		return err
	}
	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	req.SetBasicAuth(string(c.apiKey), "")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("databento: http %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
