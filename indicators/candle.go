package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"io"
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

func (c Candle) Encode(b []byte) []byte {
	b = c.Start.Encode(b)
	b = c.Open.Encode(b)
	b = c.High.Encode(b)
	b = c.Low.Encode(b)
	b = c.Close.Encode(b)
	b = c.Volume.Encode(b)
	return b
}

func (c *Candle) Decode(b []byte) []byte {
	b = c.Start.Decode(b)
	b = c.Open.Decode(b)
	b = c.High.Decode(b)
	b = c.Low.Decode(b)
	b = c.Close.Decode(b)
	b = c.Volume.Decode(b)
	return b
}

func (c *Candle) Serialize(w io.Writer) error {
	_, err := w.Write(c.Encode(nil))
	return err
}

func (c *Candle) Deserialize(r io.Reader) error {
	var b [48]byte
	_, err := io.ReadFull(r, b[:])
	if err != nil {
		return err
	}
	c.Decode(b[:])
	return nil
}
