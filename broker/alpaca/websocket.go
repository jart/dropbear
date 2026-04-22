package alpaca

const (
	SIPWSURL        = "wss://stream.data.alpaca.markets/v2/sip"
	IEXWSURL        = "wss://stream.data.alpaca.markets/v2/iex"
	BOATSWSURL      = "wss://stream.data.alpaca.markets/v1beta1/boats"
	OvernightSWSURL = "wss://stream.data.alpaca.markets/v1beta1/overnight" // 15 minute delay boats
	DelayedSIPWSURL = "wss://stream.data.alpaca.markets/v2/delayed_sip"    // 15 minute delay sip
	CryptoWSURL     = "wss://stream.data.alpaca.markets/v1beta3/crypto/us"
)

// TradingWSURL returns the trading websocket URL for live/paper trading.
func TradingWSURL() string {
	key := GetKey()
	return "wss://" + key.Host + "/stream"
}
