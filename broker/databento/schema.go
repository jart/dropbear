package databento

import "fmt"

type Schema uint16

const (
	SchemaMBO        Schema = 0  // market by order (L3)
	SchemaMBP1       Schema = 1  // top of book (L1)
	SchemaMBP10      Schema = 2  // 10 levels (L2)
	SchemaTBBO       Schema = 3  // trade events with bbo state immediately before trade executed
	SchemaTrades     Schema = 4  // trades only
	SchemaOHLCV1S    Schema = 5  // 1-second bars
	SchemaOHLCV1M    Schema = 6  // 1-minute bars
	SchemaOHLCV1H    Schema = 7  // 1-hour bars
	SchemaOHLCV1D    Schema = 8  // 1-day bars
	SchemaDefinition Schema = 9  // instrument definitions
	SchemaStatistics Schema = 10 // additional data disseminated by publishers
	SchemaStatus     Schema = 11 // trading status
	SchemaImbalance  Schema = 12 // auction imbalance events
	SchemaOHLCVEOD   Schema = 13 // open high low close volume at daily cadence based on end of trading session
	SchemaCMBP1      Schema = 14 // consolidated best bid and offer
	SchemaCMBP1S     Schema = 15 // consolidated best bid and offer subsampled at one second intervals, in addition to trades
	SchemaCMBP1M     Schema = 16 // consolidated best bid and offer subsampled at one minute intervals, in addition to trades
	SchemaTCBBO      Schema = 17 // trade events with cbbo state immediately before trade executed
	SchemaBBO1S      Schema = 18 // best bid and offer subsampled at one second intervals, in addition to trades
	SchemaBBO1M      Schema = 19 // best bid and offer subsampled at one minute intervals, in addition to trades
)

func (rt Schema) String() string {
	switch rt {
	case SchemaMBO:
		return "mbo"
	case SchemaMBP1:
		return "mbp1"
	case SchemaMBP10:
		return "mbp10"
	case SchemaTBBO:
		return "tbbo"
	case SchemaTrades:
		return "trades"
	case SchemaOHLCV1S:
		return "ohlcv1s"
	case SchemaOHLCV1M:
		return "ohlcv1m"
	case SchemaOHLCV1H:
		return "ohlcv1h"
	case SchemaOHLCV1D:
		return "ohlcv1d"
	case SchemaDefinition:
		return "definition"
	case SchemaStatistics:
		return "statistics"
	case SchemaStatus:
		return "status"
	case SchemaImbalance:
		return "imbalance"
	case SchemaOHLCVEOD:
		return "ohlcveod"
	case SchemaCMBP1:
		return "cmbp1"
	case SchemaCMBP1S:
		return "cmbp1s"
	case SchemaCMBP1M:
		return "cmbp1m"
	case SchemaTCBBO:
		return "tcbbo"
	case SchemaBBO1S:
		return "bbo1s"
	case SchemaBBO1M:
		return "bbo1m"
	default:
		return fmt.Sprintf("Schema(0x%02x)", uint8(rt))
	}
}

func (rt Schema) GoString() string {
	switch rt {
	case SchemaMBO:
		return "SchemaMBO"
	case SchemaMBP1:
		return "SchemaMBP1"
	case SchemaMBP10:
		return "SchemaMBP10"
	case SchemaTBBO:
		return "SchemaTBBO"
	case SchemaTrades:
		return "SchemaTrades"
	case SchemaOHLCV1S:
		return "SchemaOHLCV1S"
	case SchemaOHLCV1M:
		return "SchemaOHLCV1M"
	case SchemaOHLCV1H:
		return "SchemaOHLCV1H"
	case SchemaOHLCV1D:
		return "SchemaOHLCV1D"
	case SchemaDefinition:
		return "SchemaDefinition"
	case SchemaStatistics:
		return "SchemaStatistics"
	case SchemaStatus:
		return "SchemaStatus"
	case SchemaImbalance:
		return "SchemaImbalance"
	case SchemaOHLCVEOD:
		return "SchemaOHLCVEOD"
	case SchemaCMBP1:
		return "SchemaCMBP1"
	case SchemaCMBP1S:
		return "SchemaCMBP1S"
	case SchemaCMBP1M:
		return "SchemaCMBP1M"
	case SchemaTCBBO:
		return "SchemaTCBBO"
	case SchemaBBO1S:
		return "SchemaBBO1S"
	case SchemaBBO1M:
		return "SchemaBBO1M"
	default:
		return fmt.Sprintf("Schema(0x%02x)", uint8(rt))
	}
}
