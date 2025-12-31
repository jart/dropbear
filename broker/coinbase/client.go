package coinbase

import (
	"database/sql"
	"dropbear/db"
	"dropbear/ds"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Client is a Coinbase Advanced Trade API client.
type Client struct {
	db                  *sql.DB
	authKey             *Key
	rateLimiter         rateLimiter
	dayCandlesOnce      sync.Once
	dayCandlesUpsert    *sql.Stmt
	dayCandlesErr       error
	minuteCandlesOnce   sync.Once
	minuteCandlesUpsert *sql.Stmt
	minuteCandlesErr    error
}

// NewClient creates a new Client with the default parameters.
func NewClient() *Client {
	return NewClientWith(GetDefaultKey(), db.Get())
}

// NewClientWithKey creates a new Client.
func NewClientWith(authKey *Key, db *sql.DB) *Client {
	return &Client{
		db:          db,
		authKey:     authKey,
		rateLimiter: newRateLimiterForPrivateEndpoints(),
	}
}

// Close closes the client.
func (c *Client) Close() error {
	c.rateLimiter.Close()
	return nil
}

// Get makes an authenticated GET request.
func (c *Client) Get(url string) (*http.Response, error) {
	return c.Request(ds.BulkHttpClient, "GET", url, nil, true)
}

// Request makes an authenticated API request to Coinbase with rate limiting.
// Just because the returned error is nil doesn't mean the request succeeded.
func (c *Client) Request(client *http.Client, method, reqURL string, body io.Reader, shouldRetry bool) (*http.Response, error) {

	// read body into memory for retries
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
	}

	// setup url for coinbase
	u, err := url.Parse(reqURL)
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Host == "" {
		u.Host = "api.coinbase.com"
	}

	// auth tokens are keyed to path without parameters
	jwtURI := method + " " + u.Host + u.Path

	// retry loop
	tries := 0
	for {

		// send http request
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = io.NopCloser(ds.NewBytesReader(bodyBytes))
		}
		jwt := c.authKey.GenerateJWT(jwtURI, 60)
		req, err := http.NewRequest(method, u.String(), bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)

		// handle network errors
		if err != nil {
			if !shouldRetry || !ds.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
			log.Printf("coinbase: %v, retrying in %v (attempt %d)", err, delay, tries)
			time.Sleep(delay)
			tries++
			continue
		}

		// handle http errors
		if !shouldRetry || !ds.IsRetryableHTTPStatusCode(resp.StatusCode) {
			if resp.StatusCode == http.StatusTooManyRequests {
				return nil, ds.ErrTooManyRequests
			}
			return resp, nil
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		// retry with exponential backoff
		delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
		log.Printf("coinbase: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
