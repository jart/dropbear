package alpaca

import (
	"flag"
	"fmt"
)

type OrderDestination uint8

const (
	OrderDestinationNone   OrderDestination = iota // unspecified destination
	OrderDestinationNYSE                           // route to New York Stock Exchange (does not support extended hours trading)
	OrderDestinationNASDAQ                         // route to NASDAQ exchange
	OrderDestinationARCA                           // route to NYSE Arca (electronic exchange, big for ETFs)
)

func ParseOrderDestination(s string) (OrderDestination, error) {
	switch s {
	case "", "smart":
		return OrderDestinationNone, nil
	case "NYSE", "nyse":
		return OrderDestinationNYSE, nil
	case "NASDAQ", "nasdaq":
		return OrderDestinationNASDAQ, nil
	case "ARCA", "arca":
		return OrderDestinationARCA, nil
	default:
		return 0, fmt.Errorf("unknown order destination: %s", s)
	}
}

func (od OrderDestination) String() string {
	switch od {
	case OrderDestinationNone:
		return "smart"
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

// OrderDestinationFlag defines an OrderDestination flag with specified name, default value, and usage string.
// The return value is the address of an OrderDestination variable that stores the value of the flag.
func OrderDestinationFlag(name string, value string, usage string) *OrderDestination {
	p := new(OrderDestination)
	var err error
	*p, err = ParseOrderDestination(value)
	if err != nil {
		panic(err)
	}
	flag.Var((*orderDestinationValue)(p), name, usage)
	return p
}

type orderDestinationValue OrderDestination

func (d *orderDestinationValue) Set(s string) error {
	v, err := ParseOrderDestination(s)
	if err != nil {
		return err
	}
	*d = orderDestinationValue(v)
	return nil
}

func (d *orderDestinationValue) Get() any {
	return OrderDestination(*d)
}

func (d *orderDestinationValue) String() string {
	return OrderDestination(*d).String()
}
