package alpaca

import "fmt"

type Exchange int

const (
	ExchangeNYSE Exchange = iota
	ExchangeNASDAQ
	ExchangeAMEX
	ExchangeARCA
	ExchangeASCX
	ExchangeBATS
	ExchangeNYSEARCA
	ExchangeCrypto
	ExchangeOTC
)

func ParseExchange(s string) (Exchange, error) {
	switch s {
	case "NYSE":
		return ExchangeNYSE, nil
	case "NASDAQ":
		return ExchangeNASDAQ, nil
	case "AMEX":
		return ExchangeAMEX, nil
	case "ARCA":
		return ExchangeARCA, nil
	case "ASCX":
		return ExchangeASCX, nil
	case "BATS":
		return ExchangeBATS, nil
	case "NYSEARCA":
		return ExchangeNYSEARCA, nil
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
	case ExchangeNYSE:
		return "NYSE"
	case ExchangeNASDAQ:
		return "NASDAQ"
	case ExchangeAMEX:
		return "AMEX"
	case ExchangeARCA:
		return "ARCA"
	case ExchangeASCX:
		return "ASCX"
	case ExchangeBATS:
		return "BATS"
	case ExchangeNYSEARCA:
		return "NYSEARCA"
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
	case ExchangeNYSE:
		return "ExchangeNYSE"
	case ExchangeNASDAQ:
		return "ExchangeNASDAQ"
	case ExchangeAMEX:
		return "ExchangeAMEX"
	case ExchangeARCA:
		return "ExchangeARCA"
	case ExchangeASCX:
		return "ExchangeASCX"
	case ExchangeBATS:
		return "ExchangeBATS"
	case ExchangeNYSEARCA:
		return "ExchangeNYSEARCA"
	case ExchangeCrypto:
		return "ExchangeCrypto"
	case ExchangeOTC:
		return "ExchangeOTC"
	default:
		panic("unknown exchange")
	}
}
