package decimal

// UnmarshalJSON implements json.Unmarshaler.
// Accepts JSON strings ("123.45"), numbers (123.45), null, and "".
// We assume the input is well-formed JSON. Don't use directly.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || (len(data) == 4 && data[0] == 'n') { // null
		*d = Zero
		return nil
	}
	if data[0] == '"' {
		if len(data) == 2 || data[1] == '"' { // "" or empty
			*d = Zero
			return nil
		}
		data = data[1 : len(data)-1]
	}
	v, err := ParseBytes(data)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// MarshalJSON implements json.Marshaler.
// Outputs as a JSON string to preserve precision.
func (d Decimal) MarshalJSON() ([]byte, error) {
	var b [24]byte
	buf := b[:0]
	buf = append(buf, '"')
	buf = d.Append(buf)
	buf = append(buf, '"')
	return buf, nil
}
