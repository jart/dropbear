package decimal

import (
	"encoding/binary"
	"io"
)

// Decimal represents a fixed-point decimal number with 9 decimal places.
type Decimal int64

const (
	Zero    = Decimal(0)
	One     = Decimal(Scale)
	NegOne  = Decimal(-Scale)
	Two     = Decimal(Scale * 2)
	Half    = Decimal(Scale / 2)
	Tenth   = Decimal(Scale / 10)
	Max     = Decimal(9_000_000_000 * Scale)
	Min     = Decimal(-9_000_000_000 * Scale)
	Satoshi = Decimal(10) // 0.00000001
	Scale   = 1_000_000_000
	Places  = 9
)

// FromInt converts int to Decimal.
func FromInt(n int) Decimal {
	return Decimal(int64(n) * Scale)
}

// FromFloat64 converts float64 to Decimal.
func FromFloat64(n float64) Decimal {
	return Decimal(int64(n * float64(Scale)))
}

// FromBPS converts basis points to a Decimal fraction. FromBPS(100) returns 0.01.
func FromBPS(n int) Decimal {
	return Decimal(int64(n) * (Scale / 10000))
}

// ParseBPS parses a string representing basis points into a Decimal fraction.
// For example, "6.5" becomes 0.00065.
func ParseBPS(s string) Decimal {
	return Parse(s).DivInt(10000)
}

func (d Decimal) Neg() Decimal          { return -d }
func (d Decimal) IsPositive() bool      { return d > 0 }
func (d Decimal) IsNegative() bool      { return d < 0 }
func (d Decimal) Add(o Decimal) Decimal { return d + o }
func (d Decimal) Sub(o Decimal) Decimal { return d - o }
func (d Decimal) IsZero() bool          { return d == 0 }
func (d Decimal) Sqr() Decimal          { return d.Mul(d) }
func (d Decimal) BPS() Decimal          { return d.MulInt(10000) }
func (d Decimal) Int64() int64          { return int64(d) / Scale }
func (d Decimal) Float64() float64      { return float64(d) / Scale }
func (d Decimal) Int() int              { return int(int64(d) / Scale) }
func (d Decimal) MulInt(n int) Decimal  { return Decimal(int64(d) * int64(n)) }
func (d Decimal) DivInt(n int) Decimal  { return Decimal(int64(d) / int64(n)) }

// Cmp compares d and o and returns -1 if d < o, 0 if d == o, 1 if d > o.
func (d Decimal) Cmp(o Decimal) int {
	if d < o {
		return -1
	}
	if d > o {
		return 1
	}
	return 0
}

func (d Decimal) Abs() Decimal {
	if d < 0 {
		return -d
	}
	return d
}

func (d Decimal) Min(o Decimal) Decimal {
	if d < o {
		return d
	}
	return o
}

func (d Decimal) Max(o Decimal) Decimal {
	if d > o {
		return d
	}
	return o
}

// Truncate removes the fractional part, rounding toward zero.
func (d Decimal) Truncate() Decimal {
	return Decimal(int64(d) / Scale * Scale)
}

func (d Decimal) Encode(b []byte) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(d))
}

func (d *Decimal) Decode(b []byte) []byte {
	*d = Decimal(int64(binary.LittleEndian.Uint64(b)))
	return b[8:]
}

func (d *Decimal) Deserialize(reader io.Reader) error {
	var b [8]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return err
	}
	d.Decode(b[:])
	return nil
}
