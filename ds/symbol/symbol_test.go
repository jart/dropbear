package symbol

import (
	"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    Symbol
		wantErr bool
	}{
		{"simple", "AAPL", Symbol('A' | 'A'<<8 | 'P'<<16 | 'L'<<24), false},
		{"simple", "AAPL", MustParse("AAPL"), false},
		{"omfg", "SOXL", Symbol(0x4c584f53), false},
		{"max length", "ABCDEFG", MustParse("ABCDEFG"), false},
		{"too long", "ABCDEFGH", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.arg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSymbol_String(t *testing.T) {
	tests := []struct {
		name string
		s    Symbol
		want string
	}{
		{"math", Symbol('A' | 'A'<<8 | 'P'<<16 | 'L'<<24), "AAPL"},
		{"simple", MustParse("AAPL"), "AAPL"},
		{"max length", MustParse("ABCDEFG"), "ABCDEFG"},
		{"single char", MustParse("Z"), "Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("Symbol.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSymbol_GoString(t *testing.T) {
	tests := []struct {
		name string
		s    Symbol
		want string
	}{
		{"simple", MustParse("AAPL"), "symbol.MustParse(\"AAPL\")"},
		{"max length", MustParse("ABCDEFG"), "symbol.MustParse(\"ABCDEFG\")"},
		{"single char", MustParse("Z"), "symbol.MustParse(\"Z\")"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.GoString(); got != tt.want {
				t.Errorf("Symbol.GoString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSymbol_Format(t *testing.T) {
	sym := MustParse("SOXL")

	// %x should print the integer value in hex, not the string bytes
	// SOXL packed little-endian: 'S'=0x53, 'O'=0x4f, 'X'=0x58, 'L'=0x4c
	// = 0x4c584f53 (L in high byte, S in low byte)
	gotX := fmt.Sprintf("%x", sym)
	wantX := "4c584f53"
	if gotX != wantX {
		t.Errorf("%%x = %q, want %q", gotX, wantX)
	}

	// %s should print the string representation
	gotS := fmt.Sprintf("%s", sym)
	wantS := "SOXL"
	if gotS != wantS {
		t.Errorf("%%s = %q, want %q", gotS, wantS)
	}

	// %v should also print the string representation
	gotV := fmt.Sprintf("%v", sym)
	if gotV != wantS {
		t.Errorf("%%v = %q, want %q", gotV, wantS)
	}

	// %d should print the decimal integer value
	gotD := fmt.Sprintf("%d", sym)
	wantD := "1280855891" // 0x4c584f53 in decimal
	if gotD != wantD {
		t.Errorf("%%d = %q, want %q", gotD, wantD)
	}

	// verify low byte is first char
	if sym&0xff != 'S' {
		t.Errorf("low byte = 0x%x, want 0x%x ('S')", sym&0xff, 'S')
	}

	// test width and flags are respected
	got8s := fmt.Sprintf("[%8s]", sym)
	want8s := "[    SOXL]"
	if got8s != want8s {
		t.Errorf("%%8s = %q, want %q", got8s, want8s)
	}

	gotLeft := fmt.Sprintf("[%-8s]", sym)
	wantLeft := "[SOXL    ]"
	if gotLeft != wantLeft {
		t.Errorf("%%-8s = %q, want %q", gotLeft, wantLeft)
	}

	gotHash := fmt.Sprintf("%#x", sym)
	wantHash := "0x4c584f53"
	if gotHash != wantHash {
		t.Errorf("%%#x = %q, want %q", gotHash, wantHash)
	}
}

func BenchmarkMustParse(b *testing.B) {
	for b.Loop() {
		_ = MustParse("AAPL")
	}
}

func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		_, _ = Parse("AAPL")
	}
}

func BenchmarkParseBytes(b *testing.B) {
	data := []byte("AAPL")
	for b.Loop() {
		_, _ = ParseBytes(data)
	}
}
