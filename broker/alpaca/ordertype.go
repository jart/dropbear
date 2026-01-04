package alpaca

import "fmt"

type OrderType uint8

const (
	OrderTypeMarket       OrderType = iota // execute immediately at best available price
	OrderTypeLimit                         // execute at specified price or better
	OrderTypeStop                          // becomes market order when stop price is reached
	OrderTypeStopLimit                     // becomes limit order when stop price is reached
	OrderTypeTrailingStop                  // stop price trails market by specified amount
)

func ParseOrderType(s string) (OrderType, error) {
	switch s {
	case "market":
		return OrderTypeMarket, nil
	case "limit":
		return OrderTypeLimit, nil
	case "stop":
		return OrderTypeStop, nil
	case "stop_limit":
		return OrderTypeStopLimit, nil
	case "trailing_stop":
		return OrderTypeTrailingStop, nil
	default:
		return 0, fmt.Errorf("unknown order type: %s", s)
	}
}

func (ot OrderType) String() string {
	switch ot {
	case OrderTypeMarket:
		return "market"
	case OrderTypeLimit:
		return "limit"
	case OrderTypeStop:
		return "stop"
	case OrderTypeStopLimit:
		return "stop_limit"
	case OrderTypeTrailingStop:
		return "trailing_stop"
	default:
		panic("unknown order type")
	}
}

func (ot OrderType) GoString() string {
	switch ot {
	case OrderTypeMarket:
		return "OrderTypeMarket"
	case OrderTypeLimit:
		return "OrderTypeLimit"
	case OrderTypeStop:
		return "OrderTypeStop"
	case OrderTypeStopLimit:
		return "OrderTypeStopLimit"
	case OrderTypeTrailingStop:
		return "OrderTypeTrailingStop"
	default:
		panic("unknown order type")
	}
}

func (ot OrderType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ot.String() + `"`), nil
}

func (ot *OrderType) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order type: %s", data)
	}
	v, err := ParseOrderType(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*ot = v
	return nil
}
