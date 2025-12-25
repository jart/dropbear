package ds

import "fmt"

type OrderType int

const (
	OrderTypeLimit OrderType = iota
	OrderTypeMarket
)

func (ot OrderType) String() string {
	switch ot {
	case OrderTypeLimit:
		return "limit"
	case OrderTypeMarket:
		return "market"
	default:
		panic("invalid order type")
	}
}

func ParseOrderType(s string) (OrderType, error) {
	switch s {
	case "limit", "Limit", "LIMIT":
		return OrderTypeLimit, nil
	case "market", "Market", "MARKET":
		return OrderTypeMarket, nil
	default:
		return 0, fmt.Errorf("invalid order type: %s", s)
	}
}

func MustParseOrderType(s string) OrderType {
	ot, err := ParseOrderType(s)
	if err != nil {
		panic(err)
	}
	return ot
}
