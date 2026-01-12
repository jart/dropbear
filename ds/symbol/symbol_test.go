package symbol

import (
	"encoding/json"
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
		{"too long", "ABCDEFGHI", 0, true},
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
	tests := []struct {
		name string
		sym  string
		fmt  string
		want string
	}{
		{"string", "AAPL", "%s", "AAPL"},
		{"string aligned left", "AAPL", "%-8s", "AAPL    "},
		{"string aligned right", "AAPL", "%8s", "    AAPL"},
		{"hex", "AAPL", "%x", "4c504141"},
		{"hex repr", "AAPL", "%#x", "0x4c504141"},
		{"v", "AAPL", "%v", "AAPL"},
		{"v repr", "AAPL", "%#v", "symbol.MustParse(\"AAPL\")"},
		{"q", "AAPL", "%q", "\"AAPL\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmt.Sprintf(tt.fmt, MustParse(tt.sym)); got != tt.want {
				t.Errorf("Symbol.Format(%v) = %v, want %v", tt.fmt, got, tt.want)
			}
		})
	}
}

func TestTextMarshal(t *testing.T) {
	sym := MustParse("GOOG")
	text, err := sym.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(text) != "GOOG" {
		t.Errorf("MarshalText() = %v, want %v", string(text), "GOOG")
	}
	var sym2 Symbol
	if err := sym2.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if sym2 != sym {
		t.Errorf("UnmarshalText() = %v, want %v", sym2, sym)
	}
}

func TestMapKeyUnmarshal(t *testing.T) {
	// This tests the actual bug that was fixed: using Symbol as a map key in JSON
	jsonData := []byte(`{"GOOG": [1, 2, 3], "AAPL": [4, 5, 6]}`)
	var result map[Symbol][]int
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
	goog := MustParse("GOOG")
	if vals, ok := result[goog]; !ok {
		t.Errorf("missing GOOG key")
	} else if len(vals) != 3 || vals[0] != 1 {
		t.Errorf("wrong values for GOOG: %v", vals)
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
