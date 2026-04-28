package alpaca

import (
	"bytes"
	"dropbear/clocky"
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
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
		APITokenBucket:  netty.NewTokenBucketPerMinute(1000),
		DataTokenBucket: netty.NewTokenBucketPerMinute(1000),
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
func (c *Client) RequestJSON(client *http.Client, method, url string, logging bool, requestBody, responseBody any) error {
	var flog *os.File
	if logging {
		flog = getLog()
	}
	var requestBodyBytes []byte
	if requestBody != nil {
		var err error
		requestBodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		if flog != nil {
			var sb strings.Builder
			sb.WriteString(clocky.Now().String())
			sb.WriteString(" sending ")
			sb.WriteString(method)
			sb.WriteString(" ")
			sb.WriteString(url)
			sb.WriteString(" with body ")
			sb.Write(requestBodyBytes)
			sb.WriteRune('\n')
			flog.Write([]byte(sb.String()))
		}
	} else if flog != nil {
		var sb strings.Builder
		sb.WriteString(clocky.Now().String())
		sb.WriteString(" sending ")
		sb.WriteString(method)
		sb.WriteString(" ")
		sb.WriteString(url)
		sb.WriteRune('\n')
		flog.Write([]byte(sb.String()))
	}
	tries := 0
	for {
		resp, err := c.Request(client, method, url, requestBodyBytes)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			if !errors.Is(err, syscall.ECONNRESET) {
				return fmt.Errorf("reading response from %s: %w", url, err)
			}
			resp.Body.Close()
			delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
			log.Printf("alpaca: got %d read reset, retrying in %v (attempt %d)", resp.StatusCode, delay, tries)
			time.Sleep(delay)
			tries++
			continue
		}
		if flog != nil {
			var sb strings.Builder
			sb.WriteString(clocky.Now().String())
			sb.WriteString(" got ")
			sb.WriteString(strconv.Itoa(resp.StatusCode))
			sb.WriteString(" from ")
			sb.WriteString(method)
			sb.WriteString(" ")
			sb.WriteString(url)
			if len(bodyBytes) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
				sb.WriteString(" with body ")
				sb.Write(bodyBytes)
			}
			sb.WriteRune('\n')
			flog.Write([]byte(sb.String()))
		}
		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated:
			if responseBody == nil {
				panic("usage error: nil responseBody for 200 OK")
			}
			return json.Unmarshal(bodyBytes, responseBody)
		case http.StatusNoContent:
			if responseBody != nil {
				panic("usage error: non-nil responseBody for 204 No Content")
			}
			return nil
		case http.StatusNotFound:
			return ds.ErrNotFound
		default:
			var apiErr Error
			if json.Unmarshal(bodyBytes, &apiErr) == nil {
				return &apiErr
			}
			return fmt.Errorf("unexpected %d response from %s", resp.StatusCode, url)
		}
	}
}

// Request makes an authenticated API request with exponential backoff retry on 429.
// Panics if called during backtesting - no network access allowed.
func (c *Client) Request(client *http.Client, method, urlString string, body []byte) (*http.Response, error) {

	// interpret url
	key := GetKey()
	u, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Host == "" {
		u.Host = key.Host
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
		req.Header.Set("APCA-API-KEY-ID", key.Key)
		req.Header.Set("APCA-API-SECRET-KEY", key.Secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)

		// handle network errors
		if err != nil {
			if !netty.IsRetryableHTTPError(err) {
				return nil, err
			}
			delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
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
		delay := time.Duration(100<<min(tries, 8)) * time.Millisecond
		log.Printf("alpaca: got %d, retrying in %v (attempt %d)", statusCode, delay, tries)
		time.Sleep(delay)
		tries++
	}
}
