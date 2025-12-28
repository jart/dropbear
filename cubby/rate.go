package cubby

import (
	"dropbear/clocky"
	"sync"
)

type rateLimiter struct {
	lock    sync.Mutex
	burst   int
	tokens  int
	refresh clocky.Duration
	nextone clocky.Time
	waiters []chan struct{}
}

// newRateLimiter creates a rate limiter for Alpaca.
// Alpaca has 200 requests per minute for trading endpoints.
func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		burst:   30,
		tokens:  30,
		refresh: clocky.Duration(1_000_000 / 3), // ~3 requests per second
	}
}

func (rl *rateLimiter) Close() error {
	return nil
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
