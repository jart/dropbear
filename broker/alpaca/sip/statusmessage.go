package sip

import (
	"bytes"
)

// StatusMessage is a fixed-size container for status message text.
// It stores up to 16 bytes and truncates longer strings.
type StatusMessage [16]byte

// ReasonMessage is a fixed-size container for reason message text.
// It stores up to 16 bytes and truncates longer strings.
type ReasonMessage [16]byte

// SetString copies s into m, truncating if necessary.
func (m *StatusMessage) SetString(s string) {
	clear(m[:])
	copy(m[:], s)
}

// SetBytes copies b into m, truncating if necessary.
func (m *StatusMessage) SetBytes(b []byte) {
	clear(m[:])
	copy(m[:], b)
}

// String returns the message as a string, trimmed of trailing nulls.
func (m StatusMessage) String() string {
	if i := bytes.IndexByte(m[:], 0); i >= 0 {
		return string(m[:i])
	}
	return string(m[:])
}

// Bytes returns the message bytes, trimmed of trailing nulls.
func (m StatusMessage) Bytes() []byte {
	if i := bytes.IndexByte(m[:], 0); i >= 0 {
		return m[:i]
	}
	return m[:]
}

// Len returns the length of the message (excluding trailing nulls).
func (m StatusMessage) Len() int {
	if i := bytes.IndexByte(m[:], 0); i >= 0 {
		return i
	}
	return len(m)
}

// IsEmpty returns true if the message is empty.
func (m StatusMessage) IsEmpty() bool {
	return m[0] == 0
}

// MarshalJSON implements json.Marshaler.
func (m StatusMessage) MarshalJSON() ([]byte, error) {
	s := m.String()
	buf := make([]byte, len(s)+2)
	buf[0] = '"'
	copy(buf[1:], s)
	buf[len(buf)-1] = '"'
	return buf, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *StatusMessage) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return ErrInvalidStatusMessage
	}
	m.SetBytes(data[1 : len(data)-1])
	return nil
}

// SetString copies s into m, truncating if necessary.
func (m *ReasonMessage) SetString(s string) {
	clear(m[:])
	copy(m[:], s)
}

// SetBytes copies b into m, truncating if necessary.
func (m *ReasonMessage) SetBytes(b []byte) {
	clear(m[:])
	copy(m[:], b)
}

// String returns the message as a string, trimmed of trailing nulls.
func (m ReasonMessage) String() string {
	if i := bytes.IndexByte(m[:], 0); i >= 0 {
		return string(m[:i])
	}
	return string(m[:])
}

// MarshalJSON implements json.Marshaler.
func (m ReasonMessage) MarshalJSON() ([]byte, error) {
	s := m.String()
	buf := make([]byte, len(s)+2)
	buf[0] = '"'
	copy(buf[1:], s)
	buf[len(buf)-1] = '"'
	return buf, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *ReasonMessage) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return ErrInvalidStatusMessage
	}
	m.SetBytes(data[1 : len(data)-1])
	return nil
}
