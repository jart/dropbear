package kraken

import (
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.kraken.com"

// Client is a Kraken REST API client.
type Client struct {
	restLimiter    rateLimiter // for GetBalance, GetAssetPairs, etc.
	tradingLimiter rateLimiter // for AddOrder, CancelOrder, etc.
}

// NewClient creates a new Client.
func NewClient() *Client {
	return &Client{
		restLimiter:    newRateLimiter(),
		tradingLimiter: NewTradingRateLimiter(),
	}
}

// Close closes any resources used by the client.
func (c *Client) Close() error {
	c.tradingLimiter.Close()
	c.restLimiter.Close()
	return nil
}

// Response is the standard Kraken API response wrapper.
type Response struct {
	Error  []string        `json:"error"`
	Result json.RawMessage `json:"result"`
}

// Get makes a public GET request.
func (c *Client) Get(path string) (*http.Response, error) {
	return c.Request(netty.BulkHttpClient, "GET", path, nil, true)
}

// Post makes an authenticated POST request.
func (c *Client) Post(path string, values url.Values) (*http.Response, error) {
	return c.Request(netty.BulkHttpClient, "POST", path, values, true)
}

// PostFast makes an authenticated POST request using the fast HTTP client.
func (c *Client) PostFast(path string, values url.Values) (*http.Response, error) {
	return c.Request(netty.FastHTTPClient, "POST", path, values, false)
}

// Request makes an API request to Kraken with rate limiting and retries.
func (c *Client) Request(client *http.Client, method, path string, values url.Values, shouldRetry bool) (*http.Response, error) {
	if values == nil {
		values = url.Values{}
	}

	// add nonce for authenticated requests
	nonceValue := generateNonce()
	isPrivate := strings.Contains(path, "/private/")
	if isPrivate {
		values.Set("nonce", fmt.Sprintf("%d", nonceValue))
	}

	tries := 0
	for {
		reqURL := baseURL + path
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader(values.Encode())
		}

		req, err := http.NewRequest(method, reqURL, body)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		// add authentication headers for private endpoints
		if isPrivate {
			req.Header.Set("API-Key", getAPIKey())
			req.Header.Set("API-Sign", generateSignature(path, values, nonceValue))
		}

		resp, err := client.Do(req)

		// handle network errors
		if err != nil {
			if !shouldRetry || !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
			log.Printf("kraken: %v, retrying in %v (attempt %d)", err, delay, tries)
			time.Sleep(delay)
			tries++
			nonceValue = generateNonce()
			if isPrivate {
				values.Set("nonce", fmt.Sprintf("%d", nonceValue))
			}
			continue
		}

		// handle HTTP errors
		if !shouldRetry || !netty.IsRetryableHTTPStatusCode(resp.StatusCode) {
			if resp.StatusCode == http.StatusTooManyRequests {
				return nil, ds.ErrTooManyRequests
			}
			return resp, nil
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		// retry with exponential backoff
		delay := time.Duration(1<<min(tries, 10)) * time.Millisecond
		log.Printf("kraken: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
		nonceValue = generateNonce()
		if isPrivate {
			values.Set("nonce", fmt.Sprintf("%d", nonceValue))
		}
	}
}

// GetJSON makes a GET request and decodes the JSON response.
func (c *Client) GetJSON(path string, result any) error {
	c.restLimiter.Get()
	resp, err := c.Get(path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.decodeResponse(resp, result)
}

// PostJSON makes an authenticated POST request and decodes the JSON response.
func (c *Client) PostJSON(path string, values url.Values, result any) error {
	c.restLimiter.Get()
	resp, err := c.Post(path, values)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.decodeResponse(resp, result)
}

func (c *Client) decodeResponse(resp *http.Response, result any) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	if len(response.Error) > 0 {
		return fmt.Errorf("kraken error: %s", strings.Join(response.Error, ", "))
	}
	if result != nil && response.Result != nil {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decoding result: %w", err)
		}
	}
	return nil
}
