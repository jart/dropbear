package symbol

import (
	"database/sql/driver"
	"fmt"
	"unsafe"
)

// Symbol represents a stock symbol.
type Symbol int64

func Parse(ticker string) (Symbol, error) {
	return ParseBytes(unsafe.Slice(unsafe.StringData(ticker), len(ticker)))
}

func ParseBytes(ticker []byte) (Symbol, error) {
	switch len(ticker) {
	case 1:
		return Symbol(uint64(ticker[0])), nil
	case 2:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8), nil
	case 3:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16), nil
	case 4:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16 | uint64(ticker[3])<<24), nil
	case 5:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16 | uint64(ticker[3])<<24 | uint64(ticker[4])<<32), nil
	case 6:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16 | uint64(ticker[3])<<24 | uint64(ticker[4])<<32 | uint64(ticker[5])<<40), nil
	case 7:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16 | uint64(ticker[3])<<24 | uint64(ticker[4])<<32 | uint64(ticker[5])<<40 | uint64(ticker[6])<<48), nil
	case 8:
		return Symbol(uint64(ticker[0]) | uint64(ticker[1])<<8 | uint64(ticker[2])<<16 | uint64(ticker[3])<<24 | uint64(ticker[4])<<32 | uint64(ticker[5])<<40 | uint64(ticker[6])<<48 | uint64(ticker[7])<<56), nil
	default:
		return 0, fmt.Errorf("invalid stock symbol: %q", ticker)
	}
}

func MustParse(ticker string) Symbol {
	symbol, err := Parse(ticker)
	if err != nil {
		panic(err)
	}
	return symbol
}

func (s Symbol) String() string {
	var buf [8]byte
	n := 0
	v := uint64(s)
	for v != 0 {
		buf[n] = byte(v & 0xFF)
		v >>= 8
		n++
	}
	return string(buf[:n])
}

// Value ensures SQLite treats Symbol as string.
func (s Symbol) Value() (driver.Value, error) {
	return s.String(), nil
}

func (s Symbol) GoString() string {
	return "symbol.MustParse(\"" + s.String() + "\")"
}

func (s Symbol) Format(f fmt.State, verb rune) {
	// reconstruct format string from flags
	var format []byte
	format = append(format, '%')
	for _, flag := range [...]int{'-', '+', '#', ' ', '0'} {
		if f.Flag(flag) {
			format = append(format, byte(flag))
		}
	}
	if w, ok := f.Width(); ok {
		format = append(format, fmt.Sprint(w)...)
	}
	if p, ok := f.Precision(); ok {
		format = append(format, '.')
		format = append(format, fmt.Sprint(p)...)
	}
	format = append(format, byte(verb))
	switch verb {
	case 's', 'q':
		fmt.Fprintf(f, string(format), s.String())
	case 'v':
		if f.Flag('#') {
			fmt.Fprint(f, s.GoString())
		} else {
			fmt.Fprint(f, s.String())
		}
	default:
		fmt.Fprintf(f, string(format), uint64(s))
	}
}

func (s Symbol) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

func (s *Symbol) UnmarshalJSON(data []byte) error {
	if data[0] != '"' {
		return fmt.Errorf("invalid stock symbol: %q", data)
	}
	switch len(data) {
	case 3:
		*s = Symbol(uint64(data[1]))
		return nil
	case 4:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8)
		return nil
	case 5:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16)
		return nil
	case 6:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24)
		return nil
	case 7:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24 | uint64(data[5])<<32)
		return nil
	case 8:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24 | uint64(data[5])<<32 | uint64(data[6])<<40)
		return nil
	case 9:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24 | uint64(data[5])<<32 | uint64(data[6])<<40 | uint64(data[7])<<48)
		return nil
	case 10:
		*s = Symbol(uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16 | uint64(data[4])<<24 | uint64(data[5])<<32 | uint64(data[6])<<40 | uint64(data[7])<<48 | uint64(data[8])<<56)
		return nil
	default:
		return fmt.Errorf("invalid stock symbol: %q", data)
	}
}

func (s Symbol) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *Symbol) UnmarshalText(text []byte) error {
	sym, err := ParseBytes(text)
	if err != nil {
		return err
	}
	*s = sym
	return nil
}
