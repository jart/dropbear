package coinbase

import "dropbear/ds"

// rateLimiter wraps ds.TokenBucket for Coinbase rate limiting.
// https://docs.cdp.coinbase.com/exchange/rest-api/rate-limits
type rateLimiter = ds.TokenBucket

func newRateLimiterForPrivateEndpoints() rateLimiter {
	// Coinbase allows 15 requests/second with burst of 30 per profile
	return ds.NewTokenBucket(15, 30)
}
