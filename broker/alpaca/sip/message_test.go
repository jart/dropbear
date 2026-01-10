package sip

import (
	"testing"
	"unsafe"
)

func TestStatusMessageSize(t *testing.T) {
	if got := unsafe.Sizeof(StatusMessage{}); got != 16 {
		t.Errorf("sizeof(StatusMessage) = %d, want 16", got)
	}
}

func TestStatusMessageSetString(t *testing.T) {
	var m StatusMessage
	m.SetString("Trading Halt")
	if got := m.String(); got != "Trading Halt" {
		t.Errorf("String() = %q, want %q", got, "Trading Halt")
	}
	if got := m.Len(); got != 12 {
		t.Errorf("Len() = %d, want 12", got)
	}
}

func TestStatusMessageTruncation(t *testing.T) {
	var m StatusMessage
	long := "This is a very long message that exceeds sixteen characters"
	m.SetString(long)
	if got := m.Len(); got != 16 {
		t.Errorf("Len() = %d, want 16", got)
	}
	if got := m.String(); got != long[:16] {
		t.Errorf("String() = %q, want %q", got, long[:16])
	}
}

func TestStatusMessageEmpty(t *testing.T) {
	var m StatusMessage
	if !m.IsEmpty() {
		t.Error("zero StatusMessage should be empty")
	}
	if got := m.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
	if got := m.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
}

func TestStatusMessageOverwrite(t *testing.T) {
	var m StatusMessage
	m.SetString("First message that is long")
	m.SetString("Short")
	if got := m.String(); got != "Short" {
		t.Errorf("String() = %q, want %q", got, "Short")
	}
	if got := m.Len(); got != 5 {
		t.Errorf("Len() = %d, want 5", got)
	}
}

func TestStatusMessageJSON(t *testing.T) {
	var m StatusMessage
	m.SetString("Trading Halt")
	data, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != `"Trading Halt"` {
		t.Errorf("MarshalJSON() = %s, want %q", got, `"Trading Halt"`)
	}

	var m2 StatusMessage
	err = m2.UnmarshalJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.String(); got != "Trading Halt" {
		t.Errorf("after unmarshal String() = %q, want %q", got, "Trading Halt")
	}
}

func TestStatusMessageUnmarshalTruncates(t *testing.T) {
	var m StatusMessage
	long := `"This is a very long message that exceeds sixteen characters"`
	err := m.UnmarshalJSON([]byte(long))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Len(); got != 16 {
		t.Errorf("Len() = %d, want 16", got)
	}
}

func TestStatusMessageUnmarshalError(t *testing.T) {
	var m StatusMessage
	if err := m.UnmarshalJSON([]byte(`not quoted`)); err != ErrInvalidStatusMessage {
		t.Errorf("UnmarshalJSON(not quoted) = %v, want ErrInvalidStatusMessage", err)
	}
	if err := m.UnmarshalJSON([]byte(`"unclosed`)); err != ErrInvalidStatusMessage {
		t.Errorf("UnmarshalJSON(unclosed) = %v, want ErrInvalidStatusMessage", err)
	}
}

func BenchmarkStatusMessageSetString(b *testing.B) {
	var m StatusMessage
	for b.Loop() {
		m.SetString("Trading Halt")
	}
}

func BenchmarkStatusMessageString(b *testing.B) {
	var m StatusMessage
	m.SetString("Trading Halt")
	for b.Loop() {
		_ = m.String()
	}
}

func BenchmarkStatusMessageMarshalJSON(b *testing.B) {
	var m StatusMessage
	m.SetString("Trading Halt")
	for b.Loop() {
		_, _ = m.MarshalJSON()
	}
}

func BenchmarkStatusMessageUnmarshalJSON(b *testing.B) {
	data := []byte(`"Trading Halt"`)
	var m StatusMessage
	for b.Loop() {
		_ = m.UnmarshalJSON(data)
	}
}
