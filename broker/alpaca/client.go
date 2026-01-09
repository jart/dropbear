package alpaca

import (
	"bytes"
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
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
type Client struct {
	APITokenBucket  netty.TokenBucket
	DataTokenBucket netty.TokenBucket
}

// NewClient creates a new Alpaca API Client.
func NewClient() *Client {
	return &Client{
		APITokenBucket:  netty.NewTokenBucketPerMinute(200),
		DataTokenBucket: netty.NewTokenBucketPerMinute(10000),
	}
}

// Close closes the client.
func (c *Client) Close() error {
	return nil
}

// Get makes an authenticated GET request.
func (c *Client) Get(path string) (*http.Response, error) {
	return c.Request(netty.BulkHttpClient, "GET", path, nil)
}

// RequestJSON performs an API request, marshaling the request body and unmarshaling the response.
// If requestBody is nil, no request body is sent. Otherwise requestBody is marshaled as JSON.
// The response body JSON is unmarshaled into responseBody if the return error is nil.
// Your responseBody must be nil for endpoints that return 204 No Content.
// Any 404 errors are canonicalized to an ds.ErrNotFound return value.
// Other API error responses are unmarshaled into an alpaca.Error.
func (c *Client) RequestJSON(client *http.Client, method, url string, requestBody, responseBody any) error {
	var requestBodyBytes []byte
	if requestBody != nil {
		var err error
		requestBodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
	}
	resp, err := c.Request(client, method, url, requestBodyBytes)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		if responseBody == nil {
			panic("usage error: nil responseBody for 200 OK")
		}
		if err := decoder.Decode(responseBody); err != nil {
			return fmt.Errorf("failed to decode json response from %s: %w", url, err)
		}
		return nil
	case http.StatusNoContent:
		if responseBody != nil {
			panic("usage error: non-nil responseBody for 204 No Content")
		}
		return nil
	case http.StatusNotFound:
		return ds.ErrNotFound
	default:
		var apiErr Error
		if err := decoder.Decode(&apiErr); err != nil {
			return fmt.Errorf("failed to decode error response from %s: %w", url, err)
		}
		return &apiErr
	}
}

// Request makes an authenticated API request with exponential backoff retry on 429.
// Panics if called during backtesting - no network access allowed.
func (c *Client) Request(client *http.Client, method, urlString string, body []byte) (*http.Response, error) {

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

	// retry loop
	tries := 0
	for {

		// send http request
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
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
			if !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(1<<min(tries, 15)) * time.Millisecond
			log.Printf("alpaca: %v, retrying in %v (attempt %d)", err, delay, tries)
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
		delay := time.Duration(1<<min(tries, 15)) * time.Millisecond
		log.Printf("alpaca: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
