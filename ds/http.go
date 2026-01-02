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

// bulkTLSConfig is the shared TLS config for bulk HTTP clients
// Uses minimal settings to keep ClientHello under MSS (~1420 bytes) for TCP Fast Open.
var bulkTLSConfig = &tls.Config{
	SessionTicketsDisabled: false,
	ClientSessionCache:     tls.NewLRUClientSessionCache(64),
	NextProtos:             []string{"h2", "http/1.1"},
	MinVersion:             tls.VersionTLS13,          // TLS 1.3 for 1-RTT handshake
	CurvePreferences:       []tls.CurveID{tls.X25519}, // Smallest key share (32 bytes)
}

// bulkTLSDialer uses TCP Fast Open to send ClientHello in the SYN packet
var bulkTLSDialer = &tfoTLSDialer{config: bulkTLSConfig}

// BulkHttpClient is the general-purpose client for non-latency-critical requests.
// Uses HTTP/2 for efficient multiplexing of parallel requests.
// Uses TCP Fast Open to send TLS ClientHello in the SYN packet, saving one RTT.
var BulkHttpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           Dialer.DialContext,
		DialTLSContext:        bulkTLSDialer.DialTLSContext,
		MaxConnsPerHost:       20,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 0,
		ForceAttemptHTTP2:     true,
		DisableCompression:    false,
	},
	Timeout: 20 * time.Second,
}

// FastHTTPClient is optimized for latency-critical order submission.
// Uses HTTP/2 for connection reuse (avoids cold start latency spikes).
// Separate from BulkHttpClient to avoid head-of-line blocking from bulk traffic.
// Uses TCP Fast Open to send TLS ClientHello in the SYN packet, saving one RTT.
var FastHTTPClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           Dialer.DialContext,
		DialTLSContext:        bulkTLSDialer.DialTLSContext, // Reuse same TLS config (HTTP/2)
		MaxConnsPerHost:       4,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 0,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
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
