package sip

// CancelError is a message type that isn't supported yet.
// Subscribing to Trades channel implies this so it's needed.
// https://docs.alpaca.markets/docs/real-time-stock-pricing-data
type CancelError struct {
	Type MessageType
	_    [55]byte
}

func (t *CancelError) Parse(data []byte) (int, error) {
	t.Type = MessageTypeCancelError
	i := 1
	n := len(data)
	if n < 2 || data[0] != '{' {
		return 0, ErrParsingError
	}
	for i < n {
		b := data[i]
		i++
		switch b {
		case '}':
			return i, nil
		}
	}
	return 0, ErrParsingError
}
