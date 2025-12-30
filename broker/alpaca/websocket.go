package alpaca

const (
	cryptoWSURL       = "wss://stream.data.alpaca.markets/v1beta3/crypto/us"
	tradingWSURL      = "wss://api.alpaca.markets/stream"
	tradingWSURLPaper = "wss://paper-api.alpaca.markets/stream"
)

// CryptoWSURL returns the crypto websocket URL.
func CryptoWSURL() string {
	return cryptoWSURL
}

// TradingWSURL returns the trading websocket URL for live trading.
func TradingWSURL() string {
	return tradingWSURL
}

// TradingWSURLPaper returns the trading websocket URL for paper trading.
func TradingWSURLPaper() string {
	return tradingWSURLPaper
}
