package decimal

// UnmarshalJSON implements json.Unmarshaler.
// Accepts both JSON strings ("123.45") and numbers (123.45).
// We assume the input is well-formed JSON. Don't use directly.
func (d *Decimal) UnmarshalJSON(data []byte) error {
	if data[0] == '"' {
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
