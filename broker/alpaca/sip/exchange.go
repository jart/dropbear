package sip

import "fmt"

// Exchange represents a market center in the SIP consolidated feed.
// These are single-letter codes defined by the CTA and UTP plans.
type Exchange byte

const (
	ExchangeNone   Exchange = 0
	ExchangeAMEX   Exchange = 'A' // New York Curb Exchange (NYSE American)
	ExchangeBSE    Exchange = 'B' // Boston Stock Exchange (NASDAQ OMX BX)
	ExchangeNSE    Exchange = 'C' // National Stock Exchange
	ExchangeADF    Exchange = 'D' // FINRA ADF
	ExchangeIndy   Exchange = 'E' // Market Independent
	ExchangeGMX    Exchange = 'F' // Nasdaq Global/Select Market (?)
	ExchangePearl  Exchange = 'G' // MIAX Pearl (?)
	ExchangeMIAX   Exchange = 'H' // MIAX
	ExchangeISE    Exchange = 'I' // International Securities Exchange
	ExchangeEDGA   Exchange = 'J' // Cboe EDGA
	ExchangeEDGX   Exchange = 'K' // Cboe EDGX
	ExchangeLTSE   Exchange = 'L' // Long Term Stock Exchange
	ExchangeCSE    Exchange = 'M' // Chicago Stock Exchange
	ExchangeNYSE   Exchange = 'N' // New York Stock Exchange
	ExchangeARCA   Exchange = 'P' // NYSE Arca
	ExchangeNASDAQ Exchange = 'Q' // NASDAQ OMX
	ExchangePOG    Exchange = 'S' // NASDAQ Small Cap
	ExchangeINT    Exchange = 'T' // NASDAQ International
	ExchangeMEMX   Exchange = 'U' // Members Exchange
	ExchangeIEX    Exchange = 'V' // IEX
	ExchangeCBOE   Exchange = 'W' // CBOE
	ExchangePSX    Exchange = 'X' // NASDAQ OMX PSX
	ExchangeBYX    Exchange = 'Y' // Cboe BYX
	ExchangeBZX    Exchange = 'Z' // Cboe BZX
)

// Pre-computed JSON representations (no allocation on marshal)
var exchangeJSON [256][]byte

func init() {
	for i := range exchangeJSON {
		exchangeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (e Exchange) String() string {
	switch e {
	case ExchangeNone:
		return "None"
	case ExchangeAMEX:
		return "AMEX"
	case ExchangeBSE:
		return "BSE"
	case ExchangeNSE:
		return "NSE"
	case ExchangeADF:
		return "ADF"
	case ExchangeIndy:
		return "Indy"
	case ExchangeGMX:
		return "GMX"
	case ExchangePearl:
		return "Pearl"
	case ExchangeMIAX:
		return "MIAX"
	case ExchangeISE:
		return "ISE"
	case ExchangeEDGA:
		return "EDGA"
	case ExchangeEDGX:
		return "EDGX"
	case ExchangeLTSE:
		return "LTSE"
	case ExchangeCSE:
		return "CSE"
	case ExchangeNYSE:
		return "NYSE"
	case ExchangeARCA:
		return "ARCA"
	case ExchangeNASDAQ:
		return "NASDAQ"
	case ExchangePOG:
		return "POG"
	case ExchangeINT:
		return "INT"
	case ExchangeMEMX:
		return "MEMX"
	case ExchangeIEX:
		return "IEX"
	case ExchangeCBOE:
		return "CBOE"
	case ExchangePSX:
		return "PSX"
	case ExchangeBYX:
		return "BYX"
	case ExchangeBZX:
		return "BZX"
	default:
		return string(byte(e))
	}
}

func (e Exchange) GoString() string {
	switch e {
	case ExchangeNone:
		return "sip.ExchangeNone"
	case ExchangeAMEX:
		return "sip.ExchangeAMEX"
	case ExchangeBSE:
		return "sip.ExchangeBSE"
	case ExchangeNSE:
		return "sip.ExchangeNSE"
	case ExchangeADF:
		return "sip.ExchangeADF"
	case ExchangeIndy:
		return "sip.ExchangeIndy"
	case ExchangeGMX:
		return "sip.ExchangeGMX"
	case ExchangePearl:
		return "sip.ExchangePearl"
	case ExchangeMIAX:
		return "sip.ExchangeMIAX"
	case ExchangeISE:
		return "sip.ExchangeISE"
	case ExchangeEDGA:
		return "sip.ExchangeEDGA"
	case ExchangeEDGX:
		return "sip.ExchangeEDGX"
	case ExchangeLTSE:
		return "sip.ExchangeLTSE"
	case ExchangeCSE:
		return "sip.ExchangeCSE"
	case ExchangeNYSE:
		return "sip.ExchangeNYSE"
	case ExchangeARCA:
		return "sip.ExchangeARCA"
	case ExchangeNASDAQ:
		return "sip.ExchangeNASDAQ"
	case ExchangePOG:
		return "sip.ExchangePOG"
	case ExchangeINT:
		return "sip.ExchangeINT"
	case ExchangeMEMX:
		return "sip.ExchangeMEMX"
	case ExchangeIEX:
		return "sip.ExchangeIEX"
	case ExchangeCBOE:
		return "sip.ExchangeCBOE"
	case ExchangePSX:
		return "sip.ExchangePSX"
	case ExchangeBYX:
		return "sip.ExchangeBYX"
	case ExchangeBZX:
		return "sip.ExchangeBZX"
	default:
		return fmt.Sprintf("sip.Exchange('%c')", e)
	}
}

func (e Exchange) Code() string {
	return string(byte(e))
}

func (e Exchange) MarshalJSON() ([]byte, error) {
	return exchangeJSON[e], nil
}

func (e *Exchange) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*e = Exchange(data[1])
		return nil
	}
	return ErrInvalidExchange
}
