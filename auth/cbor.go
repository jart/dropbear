package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Minimal CBOR decoder for WebAuthn
// Only supports the subset needed: maps, byte strings, text strings, integers

var (
	errCBORTruncated   = errors.New("cbor: truncated")
	errCBORUnsupported = errors.New("cbor: unsupported type")
)

type cborReader struct {
	data []byte
	pos  int
}

func (r *cborReader) remaining() int {
	return len(r.data) - r.pos
}

func (r *cborReader) readByte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errCBORTruncated
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *cborReader) readBytes(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, errCBORTruncated
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *cborReader) readUint(additionalInfo byte) (uint64, error) {
	if additionalInfo < 24 {
		return uint64(additionalInfo), nil
	}
	switch additionalInfo {
	case 24:
		b, err := r.readByte()
		return uint64(b), err
	case 25:
		b, err := r.readBytes(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint16(b)), nil
	case 26:
		b, err := r.readBytes(4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint32(b)), nil
	case 27:
		b, err := r.readBytes(8)
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint64(b), nil
	default:
		return 0, errCBORUnsupported
	}
}

// readValue reads a CBOR value and returns it as a Go type
// Returns: int64, []byte, string, map[any]any, or []any
func (r *cborReader) readValue() (any, error) {
	b, err := r.readByte()
	if err != nil {
		return nil, err
	}

	majorType := b >> 5
	additionalInfo := b & 0x1f

	switch majorType {
	case 0: // unsigned integer
		v, err := r.readUint(additionalInfo)
		return int64(v), err

	case 1: // negative integer
		v, err := r.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		return int64(-1 - int64(v)), err

	case 2: // byte string
		length, err := r.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		return r.readBytes(int(length))

	case 3: // text string
		length, err := r.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		b, err := r.readBytes(int(length))
		if err != nil {
			return nil, err
		}
		return string(b), nil

	case 4: // array
		length, err := r.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		arr := make([]any, length)
		for i := range arr {
			arr[i], err = r.readValue()
			if err != nil {
				return nil, err
			}
		}
		return arr, nil

	case 5: // map
		length, err := r.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		m := make(map[any]any, length)
		for i := uint64(0); i < length; i++ {
			key, err := r.readValue()
			if err != nil {
				return nil, err
			}
			value, err := r.readValue()
			if err != nil {
				return nil, err
			}
			m[key] = value
		}
		return m, nil

	default:
		return nil, fmt.Errorf("cbor: unsupported major type %d", majorType)
	}
}

func cborDecode(data []byte) (any, error) {
	r := &cborReader{data: data}
	return r.readValue()
}

// Helper to get int64 from map with int64 key
func cborMapGetInt(m map[any]any, key int64) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	i, ok := v.(int64)
	return i, ok
}

// Helper to get []byte from map with int64 key
func cborMapGetBytes(m map[any]any, key int64) ([]byte, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	b, ok := v.([]byte)
	return b, ok
}

// Helper to get string from map with string key
func cborMapGetString(m map[any]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Helper to get []byte from map with string key
func cborMapGetBytesStr(m map[any]any, key string) ([]byte, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	b, ok := v.([]byte)
	return b, ok
}
