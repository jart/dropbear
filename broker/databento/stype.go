package databento

type SType uint8

const (
	STypeInstrumentID  SType = 0  // symbology using unique numeric id
	STypeRawSymbol     SType = 1  // symbology using original symbols provided by publisher
	STypeSmart         SType = 2  // set of databento-specific symbologies for referring to groups of symbols
	STypeContinuous    SType = 3  // databento-specific symbology where one symbol may point to different instruments at different points of time, e.g. to always refer to the front month future
	STypeParent        SType = 4  // databento-specific symbology for referring to a group of symbols by one "parent" symbol, e.g. ES.FUT to refer to all ES futures
	STypeNasdaqSymbol  SType = 5  // Symbology for US equities using NASDAQ Integrated suffix conventions
	STypeCMSSymbol     SType = 6  // Symbology for US equities using CMS suffix conventions
	STypeISIN          SType = 7  // Symbology using International Security Identification Numbers (ISIN) - ISO 6166
	STypeUSCode        SType = 8  // Symbology using US domestic Committee on Uniform Securities Identification Procedure (CUSIP) codes
	STypeBBGCompID     SType = 9  // Symbology using Bloomberg composite global IDs
	STypeBBGCompTicker SType = 10 // Symbology using Bloomberg composite tickers
	STypeFIGI          SType = 11 // Symbology using Bloomberg FIGI exchange level IDs
	STypeFIGITicker    SType = 12 // Symbology using Bloomberg exchange level tickers
)

func (st SType) String() string {
	switch st {
	case STypeInstrumentID:
		return "instrument_id"
	case STypeRawSymbol:
		return "raw_symbol"
	case STypeSmart:
		return "smart"
	case STypeContinuous:
		return "continuous"
	case STypeParent:
		return "parent"
	case STypeNasdaqSymbol:
		return "nasdaq_symbol"
	case STypeCMSSymbol:
		return "cms_symbol"
	case STypeISIN:
		return "isin"
	case STypeUSCode:
		return "us_code"
	case STypeBBGCompID:
		return "bbg_comp_id"
	case STypeBBGCompTicker:
		return "bbg_comp_ticker"
	case STypeFIGI:
		return "figi"
	case STypeFIGITicker:
		return "figi_ticker"
	default:
		return "unknown"
	}
}

func (st SType) GoString() string {
	switch st {
	case STypeInstrumentID:
		return "STypeInstrumentID"
	case STypeRawSymbol:
		return "STypeRawSymbol"
	case STypeSmart:
		return "STypeSmart"
	case STypeContinuous:
		return "STypeContinuous"
	case STypeParent:
		return "STypeParent"
	case STypeNasdaqSymbol:
		return "STypeNasdaqSymbol"
	case STypeCMSSymbol:
		return "STypeCMSSymbol"
	case STypeISIN:
		return "STypeISIN"
	case STypeUSCode:
		return "STypeUSCode"
	case STypeBBGCompID:
		return "STypeBBGCompID"
	case STypeBBGCompTicker:
		return "STypeBBGCompTicker"
	case STypeFIGI:
		return "STypeFIGI"
	case STypeFIGITicker:
		return "STypeFIGITicker"
	default:
		return "SType(" + string(st) + ")"
	}
}
