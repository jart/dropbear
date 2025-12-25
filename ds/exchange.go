package ds

// Exchange identifies a trading exchange.
type Exchange int

const (
	ExchangeAlpaca Exchange = iota
	ExchangeBinance
	ExchangeCoinbase
)

func (e Exchange) String() string {
	switch e {
	case ExchangeAlpaca:
		return "alpaca"
	case ExchangeBinance:
		return "binance"
	case ExchangeCoinbase:
		return "coinbase"
	default:
		panic("invalid exchange")
	}
}
