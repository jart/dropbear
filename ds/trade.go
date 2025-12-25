package ds

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"io"
)

// Trade represents a trade that happened in a market data feed.
type Trade struct {
	Side     Side // indicates what taker is doing
	Time     clocky.Time
	Price    decimal.Decimal
	Quantity decimal.Decimal
}

func (t Trade) Encode(b []byte) []byte {
	b = t.Side.Encode(b)
	b = t.Time.Encode(b)
	b = t.Price.Encode(b)
	b = t.Quantity.Encode(b)
	return b
}

func (t *Trade) Decode(b []byte) []byte {
	b = t.Side.Decode(b)
	b = t.Time.Decode(b)
	b = t.Price.Decode(b)
	b = t.Quantity.Decode(b)
	return b
}

func (t *Trade) Deserialize(reader io.Reader) error {
	err := t.Side.Deserialize(reader)
	if err != nil {
		return err
	}
	err = t.Time.Deserialize(reader)
	if err != nil {
		return err
	}
	err = t.Price.Deserialize(reader)
	if err != nil {
		return err
	}
	err = t.Quantity.Deserialize(reader)
	if err != nil {
		return err
	}
	return nil
}
