package netty

import (
	"dropbear/clocky"
	"sync"
)

// TokenBucket is a rate limiter that controls the rate of operations.
//
// It uses a token bucket algorithm where tokens are added at a fixed rate
// up to a maximum burst capacity. Each operation consumes one token.
// Tokens are calculated lazily on access, so there's no background goroutine.
type TokenBucket interface {

	// Try attempts to consume a token without blocking.
	// Returns true if a token was available, false otherwise.
	//
	// Use Try for latency-sensitive operations like order submission where
	// you'd rather fail fast with ErrTooManyRequests than block. This lets
	// the caller decide how to handle rate limiting (retry, queue, abort).
	//
	// Example:
	//   if !limiter.Try() {
	//       return "", ErrTooManyRequests
	//   }
	//   return c.submitOrder(order)
	Try() bool

	// Get blocks until a token is available, then consumes it.
	//
	// Use Get for operations where blocking is acceptable, such as fetching
	// account balances, market data snapshots, or other non-urgent requests.
	// This simplifies code by handling rate limiting transparently.
	//
	// Example:
	//   limiter.Get() // blocks if necessary
	//   return c.getBalance()
	Get()
}

// NewTokenBucketPerMinute creates a token bucket from a per-minute limit.
//
// Burst is set to 10% of the limit, and rate is calculated to ensure
// you never exceed the limit in any 60-second period.
//
// Example for Alpaca data API (10,000/min):
//
//	limiter := NewTokenBucketPerMinute(10000)  // ~150/sec, burst 1000
//
// Example for Alpaca trading API (200/min):
//
//	limiter := NewTokenBucketPerMinute(200)  // ~3/sec, burst 20
func NewTokenBucketPerMinute(limitPerMinute int) TokenBucket {
	burst := limitPerMinute / 10
	ratePerSecond := float64(limitPerMinute-burst) / 60.0
	return newTokenBucket(ratePerSecond, burst)
}

// NewTokenBucket creates a new token bucket rate limiter.
//
// Parameters:
//   - ratePerSecond: tokens added per second (sustained rate)
//   - burst: maximum tokens that can accumulate (burst capacity)
//
// The bucket starts full (at burst capacity).
//
// Example for Kraken Pro REST API (max 20, decay 1/sec):
//
//	limiter := NewTokenBucket(1, 20)
func NewTokenBucket(ratePerSecond, burst int) TokenBucket {
	return newTokenBucket(float64(ratePerSecond), burst)
}

func newTokenBucket(ratePerSecond float64, burst int) TokenBucket {
	return &tokenBucket{
		tokens:   float64(burst),
		burst:    float64(burst),
		rate:     ratePerSecond / 1e9, // convert to per-nanosecond
		lastTime: clocky.Now(),
	}
}

// tokenBucket implements the TokenBucket interface using lazy refill.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64     // current number of tokens
	burst    float64     // max tokens allowed
	rate     float64     // tokens per nanosecond
	lastTime clocky.Time // last refill time
}

// refill adds tokens based on elapsed time since last access.
// Must be called with mu held.
func (tb *tokenBucket) refill() {
	now := clocky.Now()
	elapsed := now.Sub(tb.lastTime)
	if elapsed > 0 {
		tb.tokens += float64(elapsed.Nanoseconds()) * tb.rate
		if tb.tokens > tb.burst {
			tb.tokens = tb.burst
		}
		tb.lastTime = now
	}
}

func (tb *tokenBucket) Try() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *tokenBucket) Get() {
	waitFor := tb.getImpl()
	if waitFor > 0 {
		clocky.Sleep(waitFor)
	}
}

func (tb *tokenBucket) getImpl() clocky.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	if tb.tokens >= 1 {
		tb.tokens--
		return 0
	}
	// calculate how long to wait for 1 token
	need := 1 - tb.tokens
	wait := clocky.Duration(need / tb.rate)
	tb.tokens = 0
	tb.lastTime = clocky.Now()
	return wait
}
