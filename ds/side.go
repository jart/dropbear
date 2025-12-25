package ds

import (
	"dropbear/decimal"
	"dropbear/loggy"
	"fmt"
	"io"
)

type Side decimal.Decimal

const (
	SideBuy  = Side(decimal.One)
	SideSell = Side(decimal.NegOne)
)

func ParseSide(s string) (Side, error) {
	switch s {
	case "buy", "BUY", "bid", "BID":
		return SideBuy, nil
	case "sell", "SELL", "ask", "ASK":
		return SideSell, nil
	default:
		return 0, fmt.Errorf("invalid side: %s", s)
	}
}

func MustParseSide(s string) Side {
	side, err := ParseSide(s)
	if err != nil {
		loggy.Fatalf("%v", err)
	}
	return side
}

func (s Side) Flip() Side {
	switch s {
	case SideBuy:
		return SideSell
	case SideSell:
		return SideBuy
	default:
		panic("invalid side")
	}
}

func (s Side) String() string {
	switch s {
	case SideSell:
		return "sell"
	case SideBuy:
		return "buy"
	default:
		panic("invalid side")
	}
}

func (s Side) ToUpper() string {
	switch s {
	case SideSell:
		return "SELL"
	case SideBuy:
		return "BUY"
	default:
		panic("invalid side")
	}
}

func (s Side) Encode(b []byte) []byte {
	switch s {
	case SideBuy:
		return append(b, 0)
	case SideSell:
		return append(b, 1)
	default:
		panic("invalid side")
	}
}

func (s *Side) Decode(b []byte) []byte {
	switch b[0] {
	case 0:
		*s = SideBuy
		return b[1:]
	case 1:
		*s = SideSell
		return b[1:]
	default:
		panic("failed to deserialize side")
	}
}

func (s *Side) Deserialize(reader io.Reader) error {
	var b [1]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return err
	}
	s.Decode(b[:])
	return nil
}
