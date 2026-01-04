package alpaca

import (
	"dropbear/ds"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

const (
	APIHost  = "api.alpaca.markets"
	DataHost = "data.alpaca.markets"
)

// Client is an Alpaca API client.
type Client struct{}

// NewClient creates a new Alpaca API Client.
func NewClient() *Client {
	return &Client{}
}

// Get makes an authenticated GET request.
func (c *Client) Get(path string) (*http.Response, error) {
	return c.Request(ds.BulkHttpClient, "GET", path, nil)
}

// Request makes an authenticated API request with exponential backoff retry on 429.
// Panics if called during backtesting - no network access allowed.
func (c *Client) Request(client *http.Client, method, urlString string, body io.Reader) (*http.Response, error) {

	// interpret url
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Host == "" {
		u.Host = APIHost
	}
	urlString = u.String()

	// schlep body into memory for retries
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
	}

	// retry loop
	tries := 0
	for {

		// send http request
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = io.NopCloser(ds.NewBytesReader(bodyBytes))
		}
		req, err := http.NewRequest(method, urlString, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		key, secret := GetKey()
		req.Header.Set("APCA-API-KEY-ID", key)
		req.Header.Set("APCA-API-SECRET-KEY", secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)

		// handle network errors
		if err != nil {
			if !ds.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(1<<min(tries, 15)) * time.Millisecond
			log.Printf("alpaca: %v, retrying in %v (attempt %d)", err, delay, tries)
			time.Sleep(delay)
			tries++
			continue
		}

		// handle http errors
		if !ds.IsRetryableHTTPStatusCode(resp.StatusCode) {
			return resp, nil
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		// retry with exponential backoff
		delay := time.Duration(1<<min(tries, 15)) * time.Millisecond
		log.Printf("alpaca: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
