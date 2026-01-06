package kraken

import "dropbear/netty"

// Kraken has two separate rate limit systems:
//
//  1. REST API counter (for general calls like GetBalance, GetAssetPairs):
//     Documented (Pro): max 20, decay 1/sec
//     Empirical (Pro):  max 59, decay ~9/sec
//
//  2. Trading rate counter (for AddOrder, CancelOrder, per currency pair):
//     Documented (Pro): max 180, decay 3.75/sec
//     Empirical (Pro):  max 225 open orders per pair (hit order limit before rate limit)
//
// https://docs.kraken.com/api/docs/guides/spot-rest-ratelimits/
// https://docs.kraken.com/api/docs/guides/spot-ratelimits/

// NewRateLimiter creates a rate limiter for REST API calls.
// Empirically measured: burst 59, decay ~9/sec. We use conservative values.
func NewRateLimiter() netty.TokenBucket {
	return netty.NewTokenBucket(8, 50)
}

// NewTradingRateLimiter creates a rate limiter for trading operations.
// Empirically we hit 225 open order limit before rate limiting kicked in.
// This should be used per currency pair for accurate limiting.
func NewTradingRateLimiter() netty.TokenBucket {
	return netty.NewTokenBucket(10, 200)
}
