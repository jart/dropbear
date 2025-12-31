package coinbase

const (
	wsURL     = "wss://advanced-trade-ws.coinbase.com"
	wsUserURL = "wss://advanced-trade-ws-user.coinbase.com"
)

// WSURL returns the websocket URL.
// This requires no authentication but goes slower.
func WSURL() string {
	return wsURL
}

// WSUserURL returns the websocket URL for logged in user.
// This is the only way to get order updates.
// It's the fastest way to get market data.
func WSUserURL() string {
	return wsUserURL
}

// GenerateWSJWT creates a JWT for websocket authentication.
func GenerateWSJWT() string {
	return GetDefaultKey().GenerateJWT("", 60*3)
}
