package alpaca

// GetSIPMessageType extracts the message type from a JSON message without full parsing.
// Returns 0 if the type cannot be determined.
func GetSIPMessageType(data []byte) byte {
	// Look for "T":"X" pattern where X is a single-char message type
	for i := 0; i < len(data)-6; i++ {
		if data[i] == '"' && data[i+1] == 'T' && data[i+2] == '"' && data[i+3] == ':' && data[i+4] == '"' {
			// Check that it's a single character type (followed by closing quote)
			if data[i+6] == '"' {
				return data[i+5]
			}
		}
	}
	return 0
}
