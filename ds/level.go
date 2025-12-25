package ds

import (
	"dropbear/decimal"
	"io"
)

// Level represents the sum of bids or asks at a price level in an order book.
type Level struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

func (t Level) Encode(b []byte) []byte {
	b = t.Price.Encode(b)
	b = t.Size.Encode(b)
	return b
}

func (t *Level) Decode(b []byte) []byte {
	b = t.Price.Decode(b)
	b = t.Size.Decode(b)
	return b
}

func (t *Level) Deserialize(reader io.Reader) error {
	err := t.Price.Deserialize(reader)
	if err != nil {
		return err
	}
	err = t.Size.Deserialize(reader)
	if err != nil {
		return err
	}
	return nil
}
