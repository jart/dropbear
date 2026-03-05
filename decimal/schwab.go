package decimal

import "fmt"

// unmarshalSchwabProto parses Schwab's protobuf decimal format:
//
//	{"lo": "18500000", "signScale": 12}
//
// The "lo" field is the value multiplied by 10^6, which matches our
// internal representation exactly. The "signScale" field is ignored.
// If "lo" is absent, the value is zero.
//
//go:noinline
func unmarshalSchwabProto(d *Decimal, data []byte) error {
	*d = Zero
	lo, ok := extractJSONStringField(data, "lo")
	if !ok {
		return nil
	}
	// lo is a plain integer string like "18500000" representing our raw int64
	var neg bool
	i := 0
	if i < len(lo) && lo[i] == '-' {
		neg = true
		i++
	}
	var v int64
	for i < len(lo) {
		c := lo[i]
		if c < '0' || c > '9' {
			return fmt.Errorf("decimal: invalid schwab proto lo: %q", lo)
		}
		v = v*10 + int64(c-'0')
		i++
	}
	if neg {
		v = -v
	}
	*d = Decimal(v)
	return nil
}

// extractJSONStringField finds a string-valued field in a flat JSON object.
// Returns the unquoted value and true, or ("", false) if not found.
func extractJSONStringField(data []byte, field string) ([]byte, bool) {
	// scan for "field" : "value"
	n := len(data)
	flen := len(field)
	for i := 0; i < n; i++ {
		if data[i] != '"' || i+flen+1 >= n {
			continue
		}
		// check field name
		match := true
		for j := 0; j < flen; j++ {
			if data[i+1+j] != field[j] {
				match = false
				break
			}
		}
		if !match || data[i+1+flen] != '"' {
			continue
		}
		// skip past closing quote, colon, whitespace
		i += flen + 2
		for i < n && (data[i] == ':' || data[i] == ' ' || data[i] == '\t') {
			i++
		}
		if i >= n {
			return nil, false
		}
		// extract value
		if data[i] == '"' {
			i++ // skip opening quote
			start := i
			for i < n && data[i] != '"' {
				i++
			}
			return data[start:i], true
		}
		// bare number/value
		start := i
		for i < n && data[i] != ',' && data[i] != '}' && data[i] != ' ' {
			i++
		}
		return data[start:i], true
	}
	return nil, false
}
