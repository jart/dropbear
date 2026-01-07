package alpaca

import "fmt"

// SIPExchange represents a market center in the SIP consolidated feed.
// These are single-letter codes defined by the CTA and UTP plans.
type SIPExchange byte

const (
	SIPExchangeUnknown        SIPExchange = 0
	SIPExchangeNYSEAmerican   SIPExchange = 'A' // NYSE American (AMEX)
	SIPExchangeNASDAQBX       SIPExchange = 'B' // NASDAQ OMX BX
	SIPExchangeNationalStock  SIPExchange = 'C' // National Stock Exchange
	SIPExchangeFINRAADF       SIPExchange = 'D' // FINRA ADF
	SIPExchangeMarketIndep    SIPExchange = 'E' // Market Independent
	SIPExchangeNasdaqGMX      SIPExchange = 'F' // Nasdaq Global/Select Market (?)
	SIPExchangeMIAXPearl      SIPExchange = 'G' // MIAX Pearl (?)
	SIPExchangeMIAX           SIPExchange = 'H' // MIAX
	SIPExchangeISE            SIPExchange = 'I' // International Securities Exchange
	SIPExchangeCboeEDGA       SIPExchange = 'J' // Cboe EDGA
	SIPExchangeCboeEDGX       SIPExchange = 'K' // Cboe EDGX
	SIPExchangeLTSE           SIPExchange = 'L' // Long Term Stock Exchange
	SIPExchangeChicago        SIPExchange = 'M' // Chicago Stock Exchange
	SIPExchangeNYSE           SIPExchange = 'N' // New York Stock Exchange
	SIPExchangeNYSEArca       SIPExchange = 'P' // NYSE Arca
	SIPExchangeNASDAQ         SIPExchange = 'Q' // NASDAQ OMX
	SIPExchangeNASDAQSmallCap SIPExchange = 'S' // NASDAQ Small Cap
	SIPExchangeNASDAQInt      SIPExchange = 'T' // NASDAQ Int
	SIPExchangeMEMX           SIPExchange = 'U' // Members Exchange
	SIPExchangeIEX            SIPExchange = 'V' // IEX
	SIPExchangeCBOE           SIPExchange = 'W' // CBOE
	SIPExchangeNASDAQPSX      SIPExchange = 'X' // NASDAQ OMX PSX
	SIPExchangeCboeBYX        SIPExchange = 'Y' // Cboe BYX
	SIPExchangeCboeBZX        SIPExchange = 'Z' // Cboe BZX
)

// Pre-computed JSON representations (no allocation on marshal)
var sipExchangeJSON [256][]byte

func init() {
	for i := range sipExchangeJSON {
		sipExchangeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (e SIPExchange) GoString() string {
	switch e {
	case SIPExchangeNYSEAmerican:
		return "alpaca.SIPExchangeNYSEAmerican"
	case SIPExchangeNASDAQBX:
		return "alpaca.SIPExchangeNASDAQBX"
	case SIPExchangeNationalStock:
		return "alpaca.SIPExchangeNationalStock"
	case SIPExchangeFINRAADF:
		return "alpaca.SIPExchangeFINRAADF"
	case SIPExchangeMarketIndep:
		return "alpaca.SIPExchangeMarketIndep"
	case SIPExchangeNasdaqGMX:
		return "alpaca.SIPExchangeNasdaqGMX"
	case SIPExchangeMIAXPearl:
		return "alpaca.SIPExchangeMIAXPearl"
	case SIPExchangeMIAX:
		return "alpaca.SIPExchangeMIAX"
	case SIPExchangeISE:
		return "alpaca.SIPExchangeISE"
	case SIPExchangeCboeEDGA:
		return "alpaca.SIPExchangeCboeEDGA"
	case SIPExchangeCboeEDGX:
		return "alpaca.SIPExchangeCboeEDGX"
	case SIPExchangeLTSE:
		return "alpaca.SIPExchangeLTSE"
	case SIPExchangeChicago:
		return "alpaca.SIPExchangeChicago"
	case SIPExchangeNYSE:
		return "alpaca.SIPExchangeNYSE"
	case SIPExchangeNYSEArca:
		return "alpaca.SIPExchangeNYSEArca"
	case SIPExchangeNASDAQ:
		return "alpaca.SIPExchangeNASDAQ"
	case SIPExchangeNASDAQSmallCap:
		return "alpaca.SIPExchangeNASDAQSmallCap"
	case SIPExchangeNASDAQInt:
		return "alpaca.SIPExchangeNASDAQInt"
	case SIPExchangeMEMX:
		return "alpaca.SIPExchangeMEMX"
	case SIPExchangeIEX:
		return "alpaca.SIPExchangeIEX"
	case SIPExchangeCBOE:
		return "alpaca.SIPExchangeCBOE"
	case SIPExchangeNASDAQPSX:
		return "alpaca.SIPExchangeNASDAQPSX"
	case SIPExchangeCboeBYX:
		return "alpaca.SIPExchangeCboeBYX"
	case SIPExchangeCboeBZX:
		return "alpaca.SIPExchangeCboeBZX"
	default:
		return fmt.Sprintf("alpaca.SIPExchange(%d)", byte(e))
	}
}

func (e SIPExchange) String() string {
	return e.GoString()
}

func (e SIPExchange) Code() string {
	return string(byte(e))
}

func (e SIPExchange) MarshalJSON() ([]byte, error) {
	return sipExchangeJSON[e], nil
}

func (e *SIPExchange) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*e = SIPExchange(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*e = SIPExchangeUnknown
		return nil
	}
	return fmt.Errorf("invalid SIPExchange: %s", data)
}

func ParseSIPExchange(code string) SIPExchange {
	if len(code) == 0 {
		return SIPExchangeUnknown
	}
	return SIPExchange(code[0])
}
