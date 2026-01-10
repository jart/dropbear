package sip

import (
	"testing"
	"unsafe"
)

// TestStructLayout verifies that the core SIP data structures have compatible
// memory layouts in their common header fields. This enables type punning to
// efficiently read Type, Tape, Timestamp, and Symbol without knowing the message type.
//
// All structs must have:
//   - Type at offset 0
//   - Tape at offset 1
//   - Timestamp at offset 8
//   - Symbol at offset 16
func TestStructLayout(t *testing.T) {
	// Verify Type field is at offset 0 for all structs
	if off := unsafe.Offsetof(Message{}.Type); off != 0 {
		t.Errorf("Message.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(Trade{}.Type); off != 0 {
		t.Errorf("Trade.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(Quote{}.Type); off != 0 {
		t.Errorf("Quote.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(Status{}.Type); off != 0 {
		t.Errorf("Status.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(LULD{}.Type); off != 0 {
		t.Errorf("LULD.Type offset = %d, want 0", off)
	}

	// Verify Tape field is at offset 1 for all structs
	if off := unsafe.Offsetof(Message{}.Tape); off != 1 {
		t.Errorf("Message.Tape offset = %d, want 1", off)
	}
	if off := unsafe.Offsetof(Trade{}.Tape); off != 1 {
		t.Errorf("Trade.Tape offset = %d, want 1", off)
	}
	if off := unsafe.Offsetof(Quote{}.Tape); off != 1 {
		t.Errorf("Quote.Tape offset = %d, want 1", off)
	}
	if off := unsafe.Offsetof(Status{}.Tape); off != 1 {
		t.Errorf("Status.Tape offset = %d, want 1", off)
	}
	if off := unsafe.Offsetof(LULD{}.Tape); off != 1 {
		t.Errorf("LULD.Tape offset = %d, want 1", off)
	}

	// Verify Timestamp field is at offset 8 for all structs
	if off := unsafe.Offsetof(Message{}.Timestamp); off != 8 {
		t.Errorf("Message.Timestamp offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(Trade{}.Timestamp); off != 8 {
		t.Errorf("Trade.Timestamp offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(Quote{}.Timestamp); off != 8 {
		t.Errorf("Quote.Timestamp offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(Status{}.Timestamp); off != 8 {
		t.Errorf("Status.Timestamp offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(LULD{}.Timestamp); off != 8 {
		t.Errorf("LULD.Timestamp offset = %d, want 8", off)
	}

	// Verify Symbol field is at offset 16 for all structs
	if off := unsafe.Offsetof(Message{}.Symbol); off != 16 {
		t.Errorf("Message.Symbol offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(Trade{}.Symbol); off != 16 {
		t.Errorf("Trade.Symbol offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(Quote{}.Symbol); off != 16 {
		t.Errorf("Quote.Symbol offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(Status{}.Symbol); off != 16 {
		t.Errorf("Status.Symbol offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(LULD{}.Symbol); off != 16 {
		t.Errorf("LULD.Symbol offset = %d, want 16", off)
	}
}

// TestStructSizes verifies all message structs are the same size (56 bytes).
func TestStructSizes(t *testing.T) {
	tests := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"Message", unsafe.Sizeof(Message{}), 56},
		{"Trade", unsafe.Sizeof(Trade{}), 56},
		{"Quote", unsafe.Sizeof(Quote{}), 56},
		{"Status", unsafe.Sizeof(Status{}), 56},
		{"LULD", unsafe.Sizeof(LULD{}), 56},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("sizeof(%s) = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

// TestHeaderOverlap verifies that we can safely read the common header fields
// from any message type by casting to a common header struct.
func TestHeaderOverlap(t *testing.T) {
	// The header struct that can be overlaid on any message
	type Header struct {
		Type      MessageType
		Tape      Tape
		_         [6]byte // padding
		Timestamp int64
		Symbol    int64
	}

	if unsafe.Sizeof(Header{}) != 24 {
		t.Errorf("sizeof(Header) = %d, want 24", unsafe.Sizeof(Header{}))
	}

	// Verify all message types are at least as large as the header
	if unsafe.Sizeof(Message{}) < unsafe.Sizeof(Header{}) {
		t.Error("Message is smaller than Header")
	}
	if unsafe.Sizeof(Trade{}) < unsafe.Sizeof(Header{}) {
		t.Error("Trade is smaller than Header")
	}
	if unsafe.Sizeof(Quote{}) < unsafe.Sizeof(Header{}) {
		t.Error("Quote is smaller than Header")
	}
	if unsafe.Sizeof(Status{}) < unsafe.Sizeof(Header{}) {
		t.Error("Status is smaller than Header")
	}
	if unsafe.Sizeof(LULD{}) < unsafe.Sizeof(Header{}) {
		t.Error("LULD is smaller than Header")
	}
}
