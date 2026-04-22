package alpaca

import "fmt"

type Exchange int

const (
	ExchangeUnknown Exchange = iota //
	ExchangeNASDAQ                  // National Association of Securities Dealers Automated Quotations (everything good)
	ExchangeNYSE                    // New York Stock Exchange (some old school stuff is listed here)
	ExchangeAMEX                    // New York Curb Exchange (older school stuff is listed here)
	ExchangeARCA                    // Archipelago Exchange (where most popular ETFs are listed)
	ExchangeBATS                    // Better Alternative Trading System (some stuff here)
	ExchangeCrypto                  // cryptography proofs
	ExchangeOTC                     // many penny stocks
)

func ParseExchange(s string) (Exchange, error) {
	switch s {
	case "", "UNKNOWN":
		return ExchangeUnknown, nil
	case "NYSE":
		return ExchangeNYSE, nil
	case "NASDAQ":
		return ExchangeNASDAQ, nil
	case "AMEX":
		return ExchangeAMEX, nil
	case "ARCA":
		return ExchangeARCA, nil
	case "BATS":
		return ExchangeBATS, nil
	case "CRYPTO":
		return ExchangeCrypto, nil
	case "OTC":
		return ExchangeOTC, nil
	default:
		return 0, fmt.Errorf("unknown exchange: %s", s)
	}
}

func (ex Exchange) String() string {
	switch ex {
	case ExchangeUnknown:
		return ""
	case ExchangeNYSE:
		return "NYSE"
	case ExchangeNASDAQ:
		return "NASDAQ"
	case ExchangeAMEX:
		return "AMEX"
	case ExchangeARCA:
		return "ARCA"
	case ExchangeBATS:
		return "BATS"
	case ExchangeCrypto:
		return "CRYPTO"
	case ExchangeOTC:
		return "OTC"
	default:
		panic("unknown exchange")
	}
}

func (ex Exchange) GoString() string {
	switch ex {
	case ExchangeUnknown:
		return "ExchangeUnknown"
	case ExchangeNYSE:
		return "ExchangeNYSE"
	case ExchangeNASDAQ:
		return "ExchangeNASDAQ"
	case ExchangeAMEX:
		return "ExchangeAMEX"
	case ExchangeARCA:
		return "ExchangeARCA"
	case ExchangeBATS:
		return "ExchangeBATS"
	case ExchangeCrypto:
		return "ExchangeCrypto"
	case ExchangeOTC:
		return "ExchangeOTC"
	default:
		panic("unknown exchange")
	}
}

func (ex Exchange) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ex.String() + `"`), nil
}

func (ex *Exchange) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid exchange: %s", data)
	}
	v, err := ParseExchange(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*ex = v
	return nil
}
