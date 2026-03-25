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
		return "mbp-1"
	case SchemaMBP10:
		return "mbp-10"
	case SchemaTBBO:
		return "tbbo"
	case SchemaTrades:
		return "trades"
	case SchemaOHLCV1S:
		return "ohlcv-1s"
	case SchemaOHLCV1M:
		return "ohlcv-1m"
	case SchemaOHLCV1H:
		return "ohlcv-1h"
	case SchemaOHLCV1D:
		return "ohlcv-1d"
	case SchemaDefinition:
		return "definition"
	case SchemaStatistics:
		return "statistics"
	case SchemaStatus:
		return "status"
	case SchemaImbalance:
		return "imbalance"
	case SchemaOHLCVEOD:
		return "ohlcv-eod"
	case SchemaCMBP1:
		return "cmbp-1"
	case SchemaCMBP1S:
		return "cbbo-1s"
	case SchemaCMBP1M:
		return "cbbo-1m"
	case SchemaTCBBO:
		return "tcbbo"
	case SchemaBBO1S:
		return "bbo-1s"
	case SchemaBBO1M:
		return "bbo-1m"
	default:
		return fmt.Sprintf("%d", rt)
	}
}

// ParseSchema parses a schema string like "mbo" or "ohlcv-1m" into a Schema.
func ParseSchema(s string) (Schema, error) {
	switch s {
	case "mbo":
		return SchemaMBO, nil
	case "mbp-1":
		return SchemaMBP1, nil
	case "mbp-10":
		return SchemaMBP10, nil
	case "tbbo":
		return SchemaTBBO, nil
	case "trades":
		return SchemaTrades, nil
	case "ohlcv-1s":
		return SchemaOHLCV1S, nil
	case "ohlcv-1m":
		return SchemaOHLCV1M, nil
	case "ohlcv-1h":
		return SchemaOHLCV1H, nil
	case "ohlcv-1d":
		return SchemaOHLCV1D, nil
	case "definition":
		return SchemaDefinition, nil
	case "statistics":
		return SchemaStatistics, nil
	case "status":
		return SchemaStatus, nil
	case "imbalance":
		return SchemaImbalance, nil
	case "ohlcv-eod":
		return SchemaOHLCVEOD, nil
	case "cmbp-1":
		return SchemaCMBP1, nil
	case "cbbo-1s":
		return SchemaCMBP1S, nil
	case "cbbo-1m":
		return SchemaCMBP1M, nil
	case "tcbbo":
		return SchemaTCBBO, nil
	case "bbo-1s":
		return SchemaBBO1S, nil
	case "bbo-1m":
		return SchemaBBO1M, nil
	default:
		return 0, fmt.Errorf("databento: unknown schema %q", s)
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
