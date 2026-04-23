package alpaca

// TradingWSURL returns the trading websocket URL for live/paper trading.
func TradingWSURL() string {
	key := GetKey()
	return "wss://" + key.Host + "/stream"
}
