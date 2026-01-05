package alpaca

import "fmt"

type OrderAlgorithm uint8

const (
	OrderAlgorithmNone OrderAlgorithm = iota // unspecified algorithm
	OrderAlgorithmDMA                        // send order directly to exchange
	OrderAlgorithmTWAP                       // execute orders over time to achieve time-weighted average price
	OrderAlgorithmVWAP                       // execute order over time to achieve volume-weighted average price
)

func ParseOrderAlgorithm(s string) (OrderAlgorithm, error) {
	switch s {
	case "":
		return OrderAlgorithmNone, nil
	case "DMA":
		return OrderAlgorithmDMA, nil
	case "TWAP":
		return OrderAlgorithmTWAP, nil
	case "VWAP":
		return OrderAlgorithmVWAP, nil
	default:
		return 0, fmt.Errorf("unknown order algorithm: %s", s)
	}
}

func (oa OrderAlgorithm) String() string {
	switch oa {
	case OrderAlgorithmNone:
		return ""
	case OrderAlgorithmDMA:
		return "DMA"
	case OrderAlgorithmTWAP:
		return "TWAP"
	case OrderAlgorithmVWAP:
		return "VWAP"
	default:
		panic("unknown order algorithm")
	}
}

func (oa OrderAlgorithm) GoString() string {
	switch oa {
	case OrderAlgorithmNone:
		return "OrderAlgorithmNone"
	case OrderAlgorithmDMA:
		return "OrderAlgorithmDMA"
	case OrderAlgorithmTWAP:
		return "OrderAlgorithmTWAP"
	case OrderAlgorithmVWAP:
		return "OrderAlgorithmVWAP"
	default:
		panic("unknown order algorithm")
	}
}

func (oa OrderAlgorithm) MarshalJSON() ([]byte, error) {
	return []byte(`"` + oa.String() + `"`), nil
}

func (oa *OrderAlgorithm) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*oa = OrderAlgorithmNone
		return nil
	}
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order algorithm: %s", data)
	}
	v, err := ParseOrderAlgorithm(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*oa = v
	return nil
}
