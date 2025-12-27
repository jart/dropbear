package ds

import "fmt"

// Exchange identifies a trading exchange.
type Exchange int

const (
	ExchangeAlpaca Exchange = iota
	ExchangeBinance
	ExchangeBinanceusd
	ExchangeCoinbase
)

func MustParseExchange(s string) Exchange {
	e, err := ParseExchange(s)
	if err != nil {
		panic(err)
	}
	return e
}

func ParseExchange(s string) (Exchange, error) {
	switch s {
	case "alpaca":
		return ExchangeAlpaca, nil
	case "binance":
		return ExchangeBinance, nil
	case "binanceusd":
		return ExchangeBinanceusd, nil
	case "coinbase":
		return ExchangeCoinbase, nil
	default:
		return 0, fmt.Errorf("unknown exchange: %s", s)
	}
}

func (e Exchange) String() string {
	switch e {
	case ExchangeAlpaca:
		return "alpaca"
	case ExchangeBinance:
		return "binance"
	case ExchangeBinanceusd:
		return "binanceusd"
	case ExchangeCoinbase:
		return "coinbase"
	default:
		panic("invalid exchange")
	}
}
