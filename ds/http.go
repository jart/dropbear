package ds

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// BulkHttpClient is the general-purpose client for non-latency-critical requests.
// Uses HTTP/2 for efficient multiplexing of parallel requests.
var BulkHttpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           Dialer.DialContext,
		MaxConnsPerHost:       20,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: 0,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		TLSClientConfig: &tls.Config{
			SessionTicketsDisabled: false,
			ClientSessionCache:     tls.NewLRUClientSessionCache(32),
			NextProtos:             []string{"h2", "http/1.1"},
		},
	},
	Timeout: 20 * time.Second,
}

// FastHTTPClient is optimized for latency-critical order submission.
// Uses HTTP/1.1 to avoid head-of-line blocking from concurrent requests.
var FastHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           Dialer.DialContext,
		MaxConnsPerHost:       4,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: 0,
		ForceAttemptHTTP2:     false, // HTTP/1.1 for dedicated connections
		DisableCompression:    true,
		TLSClientConfig: &tls.Config{
			SessionTicketsDisabled: false,
			ClientSessionCache:     tls.NewLRUClientSessionCache(32),
			NextProtos:             []string{"http/1.1"},
		},
	},
	Timeout: 20 * time.Second,
}

// IsRetryableHTTPError returns true if the error is a retryable network error.
func IsRetryableHTTPError(err error) bool {
	// connection errors
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	// timeouts
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// url.Error wraps underlying network errors
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsRetryableHTTPError(urlErr.Err)
	}
	// server closed connection
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}

// IsRetryableHTTPStatusCode returns true if an HTTP status code is conducive to retrying with exponential backoff.
func IsRetryableHTTPStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
