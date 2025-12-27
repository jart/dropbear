package teddy

import (
	"dropbear/clocky"
	"sync"
)

type rateLimiter struct {
	lock    sync.Mutex
	burst   int             // max tokens allowed at once
	tokens  int             // current number of tokens
	refresh clocky.Duration // time between adding tokens
	nextone clocky.Time     // time next token is created
	waiters []chan struct{}
}

// newRateLimiterCoinbase mimics Coinbase rate limits.
// This helps us know how to simulate 429 backoff responses.
// https://docs.cdp.coinbase.com/exchange/rest-api/rate-limits
func newRateLimiterCoinbase() *rateLimiter {
	return newRateLimiter(15, 30) // per profile for private endpoints
}

func newRateLimiter(requestsPerSecond, burst int) *rateLimiter {
	return &rateLimiter{
		burst:   burst,
		tokens:  burst,
		refresh: clocky.Duration(1_000_000 / requestsPerSecond),
	}
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

func (rl *rateLimiter) Pulse(now clocky.Time) {
	rl.lock.Lock()
	defer rl.lock.Unlock()
	if rl.nextone.IsZero() {
		rl.nextone = now.Add(rl.refresh)
	}
	if !now.Before(rl.nextone) {
		rl.nextone = rl.nextone.Add(rl.refresh)
		if len(rl.waiters) > 0 {
			close(rl.waiters[0])
			rl.waiters = rl.waiters[1:]
		} else if rl.tokens < rl.burst {
			rl.tokens++
		}
	}
}
