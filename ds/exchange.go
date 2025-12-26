package ds

// Exchange identifies a trading exchange.
type Exchange int

const (
	ExchangeAlpaca Exchange = iota
	ExchangeBinance
	ExchangeBinanceUSD
	ExchangeCoinbase
)

func (e Exchange) String() string {
	switch e {
	case ExchangeAlpaca:
		return "alpaca"
	case ExchangeBinance:
		return "binance"
	case ExchangeBinanceUSD:
		return "binanceusd"
	case ExchangeCoinbase:
		return "coinbase"
	default:
		panic("invalid exchange")
	}
}
