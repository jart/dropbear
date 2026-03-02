package alpaca

// TradingWSURL returns the trading websocket URL for live/paper trading.
func TradingWSURL() string {
	key := GetKey()
	return "wss://" + key.Host + "/stream"
}

// CryptoWSURL returns the crypto websocket URL.
func CryptoWSURL() string {
	return "wss://stream.data.alpaca.markets/v1beta3/crypto/us"
}
