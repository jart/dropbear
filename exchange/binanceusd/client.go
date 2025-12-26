package binanceusd

import (
	"dropbear/ds"
	"dropbear/exchange/binance"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const baseURL = "https://nickel.justine.lol"

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
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(data); err != nil {
			return fmt.Errorf("failed to decode json response from %s: %w", url, err)
		}
		return nil
	} else {
		var apiErr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return fmt.Errorf("failed to decode error response from %s: %w", url, err)
		}
		canonicalError := binance.LookupError(apiErr.Code)
		if canonicalError == nil {
			return fmt.Errorf("binance api %d error from %s: %d %s", resp.StatusCode, url, apiErr.Code, apiErr.Msg)
		}
		return canonicalError
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
		resp, err := ds.BulkHttpClient.Do(req)

		// handle network errors
		if err != nil {
			if !ds.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Second * time.Duration(1<<tries)
			log.Printf("binance: %v, retrying in %v (attempt %d)", err, delay, tries)
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
		delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
		log.Printf("binance: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
