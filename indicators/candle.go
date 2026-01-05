package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// Candle represents a single OHLC candlestick.
type Candle struct {
	Start  clocky.Time     // microsecond unix timestamp
	Open   decimal.Decimal // first price in period
	High   decimal.Decimal // highest price in period
	Low    decimal.Decimal // lowest price in period
	Close  decimal.Decimal // last price in period
	Volume decimal.Decimal // volume in period
}
