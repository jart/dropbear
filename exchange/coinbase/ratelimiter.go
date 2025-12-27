package coinbase

import (
	"sync"
	"time"
)

// RateLimiter implements Coinbase's rate limiting policy.
// This helps us not waste even more tokens on 429 backoff.
// https://docs.cdp.coinbase.com/exchange/rest-api/rate-limits
type rateLimiter struct {
	lock    sync.Mutex
	burst   int           // max tokens allowed at once
	tokens  int           // current number of tokens
	refresh time.Duration // time between adding tokens
	waiters []chan struct{}
}

func newRateLimiterForPrivateEndpoints() *rateLimiter {
	return newRateLimiter(15, 30) // per profile
}

func newRateLimiter(requestsPerSecond, burst int) *rateLimiter {
	rl := &rateLimiter{
		burst:   burst,
		tokens:  burst,
		refresh: time.Duration(1_000_000_000 / requestsPerSecond),
	}
	go rl.run()
	return rl
}

func (rl *rateLimiter) Try() bool {
	rl.lock.Lock()
	defer rl.lock.Unlock()
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) Get() {
	rl.lock.Lock()
	if rl.tokens > 0 {
		rl.tokens--
		rl.lock.Unlock()
		return
	}
	ch := make(chan struct{})
	rl.waiters = append(rl.waiters, ch)
	rl.lock.Unlock()
	<-ch
}

func (rl *rateLimiter) run() {
	ticker := time.NewTicker(rl.refresh)
	defer ticker.Stop()
	for range ticker.C {
		rl.lock.Lock()
		if len(rl.waiters) > 0 {
			close(rl.waiters[0])
			rl.waiters = rl.waiters[1:]
		} else if rl.tokens < rl.burst {
			rl.tokens++
		}
		rl.lock.Unlock()
	}
}
