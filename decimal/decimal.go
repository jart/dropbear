package decimal

import (
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"
)

// Decimal represents a fixed-point decimal number with 9 decimal places on each side.
type Decimal int64

const (
	Zero    = Decimal(0)
	One     = Decimal(Scale)
	NegOne  = Decimal(-Scale)
	Two     = Decimal(Scale * 2)
	Half    = Decimal(Scale / 2)
	Tenth   = Decimal(Scale / 10)
	Max     = Decimal(math.MaxInt64) // 9,223,372,036.854_775_807
	Min     = Decimal(math.MinInt64) // -9,223,372,036.854_775_808
	Epsilon = Decimal(1)             // 0.000000001
	Satoshi = Decimal(10)            // 0.00000001
	Cent    = Decimal(Scale / 100)   // 0.01
	Scale   = 1_000_000_000
	Places  = 9
)

// FromInt converts int to Decimal.
func FromInt(n int) Decimal {
	if n > math.MaxInt64/Scale || n < math.MinInt64/Scale {
		panicOverflow()
	}
	return Decimal(int64(n) * Scale)
}

// maxSafeFloat is the largest float64 value that can be converted to Decimal
// without precision loss. Beyond this, float64 can't represent values with
// nanosecond precision (our smallest unit). This is 2^53 / Scale.
const maxSafeFloat = float64(1<<53) / Scale // ~9,007,199

// FromFloat64 converts float64 to Decimal.
// Panics on NaN, infinity, or values where float64 lacks sufficient precision.
// For large values (above ~9 million), use Parse() with a string instead.
func FromFloat64(n float64) Decimal {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		panic("decimal: NaN or infinity")
	}
	if n > maxSafeFloat || n < -maxSafeFloat {
		panic("decimal: float64 lacks precision at this scale")
	}
	return Decimal(math.Round(n * Scale))
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

// Add returns d + o, panicking on overflow.
func (x Decimal) Add(y Decimal) Decimal {
	z := x + y
	if ((z ^ x) & (z ^ y)) < 0 {
		panicOverflow()
	}
	return z
}

// Sub returns d - o, panicking on overflow.
func (x Decimal) Sub(y Decimal) Decimal {
	z := x - y
	if ((x ^ y) & (z ^ x)) < 0 {
		panicOverflow()
	}
	return z
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	if d == Min {
		panicOverflow()
	}
	return -d
}

func (d Decimal) IsPositive() bool { return d > 0 }
func (d Decimal) IsNegative() bool { return d < 0 }
func (d Decimal) IsZero() bool     { return d == 0 }
func (d Decimal) Sqr() Decimal     { return d.Mul(d) }
func (d Decimal) BPS() Decimal     { return d.MulInt(10000) }
func (d Decimal) Int64() int64     { return int64(d) / Scale }
func (d Decimal) Float64() float64 { return float64(d) / Scale }
func (d Decimal) Int() int         { return int(int64(d) / Scale) }

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
		if d == Min {
			panicOverflow()
		}
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

// Store atomically stores v into d.
func (d *Decimal) Store(v Decimal) {
	atomic.StoreInt64((*int64)(d), int64(v))
}

// Load atomically loads and returns the value of d.
func (d *Decimal) Load() Decimal {
	return Decimal(atomic.LoadInt64((*int64)(d)))
}

// AtomicAdd atomically adds v to d.
func (d *Decimal) AtomicAdd(v Decimal) Decimal {
	return Decimal(atomic.AddInt64((*int64)(d), int64(v)))
}

// Swap atomically replaces v into d.
func (d *Decimal) Swap(v Decimal) Decimal {
	return Decimal(atomic.SwapInt64((*int64)(d), int64(v)))
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

//go:noinline
func panicOverflow() {
	panic("decimal overflow")
}
