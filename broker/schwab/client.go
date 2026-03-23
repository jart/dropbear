package schwab

import (
	"bytes"
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"syscall"
	"time"
)

const (
	apiBaseURL = "https://api.schwabapi.com/trader/v1"
)

// Client is a Schwab Trader API client.
type Client struct {
	TokenBucket netty.TokenBucket // rate limiter for mutating requests (POST/PUT/DELETE)
}

// NewClient creates a new Schwab API Client.
// The token bucket limits mutating requests to 120/minute per Schwab's throttle policy.
func NewClient() *Client {
	return &Client{
		TokenBucket: netty.NewTokenBucketPerMinute(120),
	}
}

// Close closes the client.
func (c *Client) Close() error {
	return nil
}

// RequestJSON performs an API request, marshaling the request body and unmarshaling the response.
// If requestBody is nil, no request body is sent. Otherwise requestBody is marshaled as JSON.
// The response body JSON is unmarshaled into responseBody if the return error is nil.
// Your responseBody must be nil for endpoints that return empty bodies (201 Created, 200 OK on DELETE).
// Any 404 errors are canonicalized to a ds.ErrNotFound return value.
// Other API error responses are unmarshaled into a schwab.Error.
func (c *Client) RequestJSON(client *http.Client, method, path string, requestBody, responseBody any) error {
	var requestBodyBytes []byte
	if requestBody != nil {
		var err error
		requestBodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
	}
	tries := 0
	for {
		resp, err := c.Request(client, method, path, requestBodyBytes)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			if responseBody == nil {
				return nil // e.g. DELETE returns 200 with empty body
			}
			err := json.NewDecoder(resp.Body).Decode(responseBody)
			if err == nil {
				return nil
			}
			if !errors.Is(err, syscall.ECONNRESET) {
				return fmt.Errorf("schwab: failed to decode json response from %s: %w", path, err)
			}
		case http.StatusCreated:
			// POST/PUT returns 201 with empty body
			return nil
		case http.StatusNoContent:
			return nil
		case http.StatusNotFound:
			return ds.ErrNotFound
		default:
			body, _ := io.ReadAll(resp.Body)
			log.Printf("schwab: %s %s returned %d: %s", method, path, resp.StatusCode, body)
			var apiErr Error
			if len(body) > 0 && json.Unmarshal(body, &apiErr) == nil && apiErr.HasContent() {
				return &apiErr
			}
			return fmt.Errorf("schwab: %s %s returned %d: %s", method, path, resp.StatusCode, body)
		}
		resp.Body.Close()
		delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
		log.Printf("schwab: got %d read reset, retrying in %v (attempt %d)", resp.StatusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}

// RequestJSONRaw is like RequestJSON but returns the raw *http.Response on success (2xx).
// The caller is responsible for closing the response body.
// Use this when you need access to response headers (e.g. Location header for order IDs).
func (c *Client) RequestJSONRaw(client *http.Client, method, path string, requestBody any) (*http.Response, error) {
	var requestBodyBytes []byte
	if requestBody != nil {
		var err error
		requestBodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}
	resp, err := c.Request(client, method, path, requestBodyBytes)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return resp, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ds.ErrNotFound
	default:
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("schwab: %s %s returned %d: %s", method, path, resp.StatusCode, body)
		var apiErr Error
		if len(body) > 0 && json.Unmarshal(body, &apiErr) == nil && apiErr.HasContent() {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("schwab: %s %s returned %d: %s", method, path, resp.StatusCode, body)
	}
}

// Request makes an authenticated API request with exponential backoff retry on 429/5xx.
func (c *Client) Request(client *http.Client, method, path string, body []byte) (*http.Response, error) {
	urlString := apiBaseURL + path

	// log request JSON for debugging
	if body != nil {
		var pretty bytes.Buffer
		json.Indent(&pretty, body, "", "  ")
		log.Printf("schwab: %s %s\n%s", method, path, pretty.String())
	}

	// retry loop
	tries := 0
	for {
		// get valid access token
		accessToken, err := getAccessToken()
		if err != nil {
			return nil, err
		}

		// send http request
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, urlString, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("schwab: creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)

		// handle network errors
		if err != nil {
			if !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
			log.Printf("schwab: %v, retrying in %v (attempt %d)", err, delay, tries)
			time.Sleep(delay)
			tries++
			continue
		}

		// handle http errors
		if !netty.IsRetryableHTTPStatusCode(resp.StatusCode) {
			return resp, nil
		}
		// schwab returns 500 with a JSON body for normal errors (e.g. bad order)
		// only retry 500 if it looks like a transient infrastructure failure
		if resp.StatusCode == http.StatusInternalServerError {
			if isSchwabApplicationError(resp) {
				return resp, nil
			}
		}
		statusCode := resp.StatusCode
		resp.Body.Close()

		// retry with exponential backoff
		delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
		log.Printf("schwab: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}

// isSchwabApplicationError returns true if a 500 response is a Schwab application
// error (JSON body with error message) rather than a transient infrastructure failure.
// These should not be retried — they indicate a problem with the request.
func isSchwabApplicationError(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return ct == "application/json" || len(ct) > 16 && ct[:16] == "application/json"
}

// logRetryableError logs headers and body for server errors to aid debugging.
func logRetryableError(method, path string, resp *http.Response) {
	if resp.StatusCode < 500 {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "schwab: %s %s returned %d\n", method, path, resp.StatusCode)
	for key, vals := range resp.Header {
		for _, val := range vals {
			fmt.Fprintf(&buf, "  %s: %s\n", key, val)
		}
	}
	if len(body) > 0 {
		buf.WriteString("  body: ")
		buf.Write(body)
	}
	log.Print(buf.String())
}
