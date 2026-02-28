package databento

import "fmt"

type RType uint8

const (
	RTypeMBP0          RType = 0x00 // trades only
	RTypeMBP1          RType = 0x01 // top of book (L1) market by price record
	RTypeMBP10         RType = 0x0a // ten levels (L2) market by price record that's ten levels deep
	RTypeOHLCV1S       RType = 0x20 // 1-second bars (open high low close volume)
	RTypeOHLCV1M       RType = 0x21 // 1-minute bars (open high low close volume)
	RTypeOHLCV1H       RType = 0x22 // 1-hour bars (open high low close volume)
	RTypeOHLCV1D       RType = 0x23 // 1-day bars (open high low close volume)
	RTypeOHLCVEOD      RType = 0x24 // end-of-day bars (open high low close volume)
	RTypeStatus        RType = 0x12 // exchange status record
	RTypeInstrumentDef RType = 0x13 // instrument definition record
	RTypeImbalance     RType = 0x14 // auction order imbalance record
	RTypeError         RType = 0x15 // denotes error from gateway
	RTypeSymbolMapping RType = 0x16 // symbol mapping record
	RTypeSystem        RType = 0x17 // non-error message from gateway (e.g. heartbeats)
	RTypeStatistics    RType = 0x18 // statistics record from publisher (not calculated by databento)
	RTypeMBO           RType = 0xa0 // market by order (L3) record
	RTypeCMBP1         RType = 0xb1 // consolidated market by price (NBBO) record
	RTypeCMBP1S        RType = 0xc0 // consolidated market by price (NBBO) record (subsampled on one second interval)
	RTypeCMBP1M        RType = 0xc1 // consolidated market by price (NBBO) record (subsampled on one minute interval)
	RTypeTCBBO         RType = 0xc2 // consolidated best bid and offer trade record containing consolidated BBO before trade
	RTypeBBO1S         RType = 0xc3 // best bid offer record subsampled on one second interval
	RTypeBBO1M         RType = 0xc4 // best bid offer record subsampled on one minute interval
)

func (rt RType) String() string {
	switch rt {
	case RTypeMBP0:
		return "RTypeMBP0"
	case RTypeMBP1:
		return "RTypeMBP1"
	case RTypeMBP10:
		return "RTypeMBP10"
	case RTypeOHLCV1S:
		return "RTypeOHLCV1S"
	case RTypeOHLCV1M:
		return "RTypeOHLCV1M"
	case RTypeOHLCV1H:
		return "RTypeOHLCV1H"
	case RTypeOHLCV1D:
		return "RTypeOHLCV1D"
	case RTypeOHLCVEOD:
		return "RTypeOHLCVEOD"
	case RTypeStatus:
		return "RTypeStatus"
	case RTypeInstrumentDef:
		return "RTypeInstrumentDef"
	case RTypeImbalance:
		return "RTypeImbalance"
	case RTypeError:
		return "RTypeError"
	case RTypeSymbolMapping:
		return "RTypeSymbolMapping"
	case RTypeSystem:
		return "RTypeSystem"
	case RTypeStatistics:
		return "RTypeStatistics"
	case RTypeMBO:
		return "RTypeMBO"
	case RTypeCMBP1:
		return "RTypeCMBP1"
	case RTypeCMBP1S:
		return "RTypeCMBP1S"
	case RTypeCMBP1M:
		return "RTypeCMBP1M"
	case RTypeTCBBO:
		return "RTypeTCBBO"
	case RTypeBBO1S:
		return "RTypeBBO1S"
	case RTypeBBO1M:
		return "RTypeBBO1M"
	default:
		return fmt.Sprintf("RType(0x%02x)", uint8(rt))
	}
}
