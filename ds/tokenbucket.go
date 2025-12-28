package ds

import (
	"sync"
	"time"
)

// TokenBucket is a rate limiter that controls the rate of operations.
//
// It uses a token bucket algorithm where tokens are added at a fixed rate
// up to a maximum burst capacity. Each operation consumes one token.
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

	// Close shuts down the token bucket and releases resources.
	Close() error
}

// tokenBucket implements the TokenBucket interface.
type tokenBucket struct {
	lock    sync.Mutex
	burst   int             // max tokens allowed at once
	tokens  int             // current number of tokens
	refresh time.Duration   // time between adding tokens
	waiters []chan struct{} // channels waiting for tokens
	done    chan struct{}   // signals shutdown
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
//
// Example for Kraken Pro trading (max 180, decay 3.75/sec):
//
//	limiter := NewTokenBucket(3.75, 180)
func NewTokenBucket(ratePerSecond float64, burst int) TokenBucket {
	tb := &tokenBucket{
		burst:   burst,
		tokens:  burst,
		refresh: time.Duration(float64(time.Second) / ratePerSecond),
		done:    make(chan struct{}),
	}
	go tb.run()
	return tb
}

func (tb *tokenBucket) Close() error {
	close(tb.done)
	return nil
}

func (tb *tokenBucket) Try() bool {
	tb.lock.Lock()
	defer tb.lock.Unlock()
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *tokenBucket) Get() {
	tb.lock.Lock()
	if tb.tokens > 0 {
		tb.tokens--
		tb.lock.Unlock()
		return
	}
	ch := make(chan struct{})
	tb.waiters = append(tb.waiters, ch)
	tb.lock.Unlock()
	<-ch
}

func (tb *tokenBucket) run() {
	ticker := time.NewTicker(tb.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tb.lock.Lock()
			if len(tb.waiters) > 0 {
				close(tb.waiters[0])
				tb.waiters = tb.waiters[1:]
			} else if tb.tokens < tb.burst {
				tb.tokens++
			}
			tb.lock.Unlock()
		case <-tb.done:
			return
		}
	}
}
