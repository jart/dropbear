package coinbase

import "dropbear/netty"

// coinbase allows 15 requests/second with burst of 30 per profile
// https://docs.cdp.coinbase.com/exchange/rest-api/rate-limits
func newRateLimiterForPrivateEndpoints() netty.TokenBucket {
	return netty.NewTokenBucket(15, 30)
}
