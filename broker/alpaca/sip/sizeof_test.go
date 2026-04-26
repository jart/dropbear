package sip

import (
	"testing"
	"unsafe"
)

// test that all sip.Message structs are same size in binary
func TestMessageSize(t *testing.T) {
	if got := unsafe.Sizeof(Message{}); got != 56 {
		t.Errorf("sizeof(Message) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Trade{}); got != 56 {
		t.Errorf("sizeof(Trade) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Quote{}); got != 56 {
		t.Errorf("sizeof(Quote) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Status{}); got != 56 {
		t.Errorf("sizeof(Status) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(LULD{}); got != 56 {
		t.Errorf("sizeof(LULD) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Imbalance{}); got != 56 {
		t.Errorf("sizeof(Imbalance) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Correction{}); got != 56 {
		t.Errorf("sizeof(Correction) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(CancelError{}); got != 56 {
		t.Errorf("sizeof(CancelError) = %d, want 56", got)
	}
	if got := unsafe.Sizeof(Bar{}); got != 56 {
		t.Errorf("sizeof(Bar) = %d, want 56", got)
	}
}
