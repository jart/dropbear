package binance

import (
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const baseURL = "https://nickel.justinestreet.capital"

// Client is a Binance REST API client.
type Client struct{}

// NewClient creates a new Client.
func NewClient() *Client {
	return &Client{}
}

// GetJSON performs a GET request and decodes the JSON response into data.
func (c *Client) GetJSON(url string, data any) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	if resp.StatusCode == http.StatusOK {
		if err := decoder.Decode(data); err != nil {
			return fmt.Errorf("failed to decode json response from %s: %w", url, err)
		}
		return nil
	} else {
		var err Error
		if err := decoder.Decode(&err); err != nil {
			return fmt.Errorf("failed to decode error response from %s: %w", url, err)
		}
		canonicalAPIErrorObject := canonicalizeError(err.Code)
		if canonicalAPIErrorObject != nil {
			return canonicalAPIErrorObject
		}
		return &err
	}
}

// Get makes a GET request.
func (c *Client) Get(path string) (*http.Response, error) {
	return c.Request("GET", path, nil)
}

// Request makes an API request to Binance with retries.
// Panics if called during backtesting - no network access allowed.
func (c *Client) Request(method, path string, body io.Reader) (*http.Response, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
	}
	tries := 0
	url := baseURL + path
	for {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = io.NopCloser(ds.NewBytesReader(bodyBytes))
		}
		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := netty.BulkHttpClient.Do(req)

		// handle network errors
		if err != nil {
			if !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Second * time.Duration(1<<tries)
			log.Printf("binance: %v, retrying in %v (attempt %d)", err, delay, tries)
			time.Sleep(delay)
			tries++
			continue
		}

		// handle http errors
		if !netty.IsRetryableHTTPStatusCode(resp.StatusCode) {
			return resp, nil
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		// retry with exponential backoff
		delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
		log.Printf("binance: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
