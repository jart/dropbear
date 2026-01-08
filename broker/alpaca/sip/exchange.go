package sip

// Exchange represents a market center in the SIP consolidated feed.
// These are single-letter codes defined by the CTA and UTP plans.
type Exchange byte

const (
	ExchangeNYSEAmerican   Exchange = 'A' // NYSE American (AMEX)
	ExchangeNASDAQBX       Exchange = 'B' // NASDAQ OMX BX
	ExchangeNationalStock  Exchange = 'C' // National Stock Exchange
	ExchangeFINRAADF       Exchange = 'D' // FINRA ADF
	ExchangeMarketIndep    Exchange = 'E' // Market Independent
	ExchangeNasdaqGMX      Exchange = 'F' // Nasdaq Global/Select Market (?)
	ExchangeMIAXPearl      Exchange = 'G' // MIAX Pearl (?)
	ExchangeMIAX           Exchange = 'H' // MIAX
	ExchangeISE            Exchange = 'I' // International Securities Exchange
	ExchangeCboeEDGA       Exchange = 'J' // Cboe EDGA
	ExchangeCboeEDGX       Exchange = 'K' // Cboe EDGX
	ExchangeLTSE           Exchange = 'L' // Long Term Stock Exchange
	ExchangeChicago        Exchange = 'M' // Chicago Stock Exchange
	ExchangeNYSE           Exchange = 'N' // New York Stock Exchange
	ExchangeNYSEArca       Exchange = 'P' // NYSE Arca
	ExchangeNASDAQ         Exchange = 'Q' // NASDAQ OMX
	ExchangeNASDAQSmallCap Exchange = 'S' // NASDAQ Small Cap
	ExchangeNASDAQInt      Exchange = 'T' // NASDAQ Int
	ExchangeMEMX           Exchange = 'U' // Members Exchange
	ExchangeIEX            Exchange = 'V' // IEX
	ExchangeCBOE           Exchange = 'W' // CBOE
	ExchangeNASDAQPSX      Exchange = 'X' // NASDAQ OMX PSX
	ExchangeCboeBYX        Exchange = 'Y' // Cboe BYX
	ExchangeCboeBZX        Exchange = 'Z' // Cboe BZX
)

// Pre-computed JSON representations (no allocation on marshal)
var exchangeJSON [256][]byte

func init() {
	for i := range exchangeJSON {
		exchangeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (e Exchange) GoString() string {
	switch e {
	case ExchangeNYSEAmerican:
		return "sip.ExchangeNYSEAmerican"
	case ExchangeNASDAQBX:
		return "sip.ExchangeNASDAQBX"
	case ExchangeNationalStock:
		return "sip.ExchangeNationalStock"
	case ExchangeFINRAADF:
		return "sip.ExchangeFINRAADF"
	case ExchangeMarketIndep:
		return "sip.ExchangeMarketIndep"
	case ExchangeNasdaqGMX:
		return "sip.ExchangeNasdaqGMX"
	case ExchangeMIAXPearl:
		return "sip.ExchangeMIAXPearl"
	case ExchangeMIAX:
		return "sip.ExchangeMIAX"
	case ExchangeISE:
		return "sip.ExchangeISE"
	case ExchangeCboeEDGA:
		return "sip.ExchangeCboeEDGA"
	case ExchangeCboeEDGX:
		return "sip.ExchangeCboeEDGX"
	case ExchangeLTSE:
		return "sip.ExchangeLTSE"
	case ExchangeChicago:
		return "sip.ExchangeChicago"
	case ExchangeNYSE:
		return "sip.ExchangeNYSE"
	case ExchangeNYSEArca:
		return "sip.ExchangeNYSEArca"
	case ExchangeNASDAQ:
		return "sip.ExchangeNASDAQ"
	case ExchangeNASDAQSmallCap:
		return "sip.ExchangeNASDAQSmallCap"
	case ExchangeNASDAQInt:
		return "sip.ExchangeNASDAQInt"
	case ExchangeMEMX:
		return "sip.ExchangeMEMX"
	case ExchangeIEX:
		return "sip.ExchangeIEX"
	case ExchangeCBOE:
		return "sip.ExchangeCBOE"
	case ExchangeNASDAQPSX:
		return "sip.ExchangeNASDAQPSX"
	case ExchangeCboeBYX:
		return "sip.ExchangeCboeBYX"
	case ExchangeCboeBZX:
		return "sip.ExchangeCboeBZX"
	default:
		panic("invalid exchange")
	}
}

func (e Exchange) String() string {
	return e.GoString()
}

func (e Exchange) Code() string {
	return string(byte(e))
}

func (e Exchange) MarshalJSON() ([]byte, error) {
	return exchangeJSON[e], nil
}

func (e *Exchange) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		x := Exchange(data[1])
		switch x {
		case ExchangeNYSEAmerican, ExchangeNASDAQBX, ExchangeNationalStock,
			ExchangeFINRAADF, ExchangeMarketIndep, ExchangeNasdaqGMX,
			ExchangeMIAXPearl, ExchangeMIAX, ExchangeISE,
			ExchangeCboeEDGA, ExchangeCboeEDGX, ExchangeLTSE,
			ExchangeChicago, ExchangeNYSE, ExchangeNYSEArca,
			ExchangeNASDAQ, ExchangeNASDAQSmallCap, ExchangeNASDAQInt,
			ExchangeMEMX, ExchangeIEX, ExchangeCBOE,
			ExchangeNASDAQPSX, ExchangeCboeBYX, ExchangeCboeBZX:
			*e = x
			return nil
		default:
			return ErrInvalidExchange
		}
	}
	return ErrInvalidExchange
}
