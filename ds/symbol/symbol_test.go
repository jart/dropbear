package symbol

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    Symbol
		wantErr bool
	}{
		{"simple", "AAPL", MustParse("AAPL"), false},
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
		{"simple", MustParse("AAPL"), "symbol.ParseString(\"AAPL\")"},
		{"max length", MustParse("ABCDEFG"), "symbol.ParseString(\"ABCDEFG\")"},
		{"single char", MustParse("Z"), "symbol.ParseString(\"Z\")"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.GoString(); got != tt.want {
				t.Errorf("Symbol.GoString() = %v, want %v", got, tt.want)
			}
		})
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
