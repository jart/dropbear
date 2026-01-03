package ds

import "fmt"

// Broker identifies a trading broker.
type Broker uint8

const (
	BrokerAlpaca Broker = iota
	BrokerBinance
	BrokerBinanceusd
	BrokerCoinbase
	BrokerKraken
)

func MustParseBroker(s string) Broker {
	e, err := ParseBroker(s)
	if err != nil {
		panic(err)
	}
	return e
}

func ParseBroker(s string) (Broker, error) {
	switch s {
	case "alpaca":
		return BrokerAlpaca, nil
	case "binance":
		return BrokerBinance, nil
	case "binanceusd":
		return BrokerBinanceusd, nil
	case "coinbase":
		return BrokerCoinbase, nil
	case "kraken":
		return BrokerKraken, nil
	default:
		return 0, fmt.Errorf("unknown broker: %s", s)
	}
}

func (e Broker) String() string {
	switch e {
	case BrokerAlpaca:
		return "alpaca"
	case BrokerBinance:
		return "binance"
	case BrokerBinanceusd:
		return "binanceusd"
	case BrokerCoinbase:
		return "coinbase"
	case BrokerKraken:
		return "kraken"
	default:
		panic("invalid broker")
	}
}

func (e Broker) GoString() string {
	switch e {
	case BrokerAlpaca:
		return "BrokerAlpaca"
	case BrokerBinance:
		return "BrokerBinance"
	case BrokerBinanceusd:
		return "BrokerBinanceusd"
	case BrokerCoinbase:
		return "BrokerCoinbase"
	case BrokerKraken:
		return "BrokerKraken"
	default:
		panic("invalid broker")
	}
}
