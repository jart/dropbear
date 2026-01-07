package alpaca

import (
	"testing"
	"unsafe"
)

func internSymbol(b []byte) string {
	if asset, ok := Assets[string(b)]; ok {
		return asset.Symbol
	}
	return string(b)
}

var symbolBytes = []byte("AAPL")
var sink string

func BenchmarkInternSymbol(b *testing.B) {
	for b.Loop() {
		sink = internSymbol(symbolBytes)
	}
}

func BenchmarkMapLookupDirect(b *testing.B) {
	// Direct string lookup for comparison
	s := "AAPL"
	for b.Loop() {
		if asset, ok := Assets[s]; ok {
			sink = asset.Symbol
		}
	}
}

func BenchmarkStringConversion(b *testing.B) {
	// Just the conversion to see baseline
	for b.Loop() {
		sink = string(symbolBytes)
	}
}

func TestInternSymbol(t *testing.T) {
	b := []byte("AAPL")
	s := internSymbol(b)
	if s != "AAPL" {
		t.Errorf("got %q, want AAPL", s)
	}
	// Verify it's actually the interned string
	asset := Assets["AAPL"]
	if asset == nil {
		t.Fatal("AAPL not in Assets")
	}
	// Compare data pointers - they should be the same interned string
	sPtr := (*(*[2]uintptr)(unsafe.Pointer(&s)))[0]
	aPtr := (*(*[2]uintptr)(unsafe.Pointer(&asset.Symbol)))[0]
	if sPtr != aPtr {
		t.Error("not returning interned string")
	}
}
