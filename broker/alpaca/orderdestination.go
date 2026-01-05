package alpaca

import "fmt"

type OrderDestination uint8

const (
	OrderDestinationNone   OrderDestination = iota // unspecified destination
	OrderDestinationNYSE                           // route to New York Stock Exchange
	OrderDestinationNASDAQ                         // route to NASDAQ exchange
	OrderDestinationARCA                           // route to NYSE Arca (electronic exchange, big for ETFs)
)

func ParseOrderDestination(s string) (OrderDestination, error) {
	switch s {
	case "":
		return OrderDestinationNone, nil
	case "NYSE":
		return OrderDestinationNYSE, nil
	case "NASDAQ":
		return OrderDestinationNASDAQ, nil
	case "ARCA":
		return OrderDestinationARCA, nil
	default:
		return 0, fmt.Errorf("unknown order destination: %s", s)
	}
}

func (od OrderDestination) String() string {
	switch od {
	case OrderDestinationNone:
		return ""
	case OrderDestinationNYSE:
		return "NYSE"
	case OrderDestinationNASDAQ:
		return "NASDAQ"
	case OrderDestinationARCA:
		return "ARCA"
	default:
		panic("unknown order destination")
	}
}

func (od OrderDestination) GoString() string {
	switch od {
	case OrderDestinationNone:
		return "OrderDestinationNone"
	case OrderDestinationNYSE:
		return "OrderDestinationNYSE"
	case OrderDestinationNASDAQ:
		return "OrderDestinationNASDAQ"
	case OrderDestinationARCA:
		return "OrderDestinationARCA"
	default:
		panic("unknown order destination")
	}
}

func (od OrderDestination) MarshalJSON() ([]byte, error) {
	return []byte(`"` + od.String() + `"`), nil
}

func (od *OrderDestination) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		*od = OrderDestinationNone
		return nil
	}
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid order destination: %s", data)
	}
	v, err := ParseOrderDestination(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*od = v
	return nil
}
