package alpaca

import (
	"encoding/json"
)

// ParseSIPTrade parses a raw JSON message into a SIPTrade.
func ParseSIPTrade(data []byte) (*SIPTrade, error) {
	var trade SIPTrade
	if err := json.Unmarshal(data, &trade); err != nil {
		return nil, err
	}
	return &trade, nil
}

// ParseSIPQuote parses a raw JSON message into a SIPQuote.
func ParseSIPQuote(data []byte) (*SIPQuote, error) {
	var quote SIPQuote
	if err := json.Unmarshal(data, &quote); err != nil {
		return nil, err
	}
	return &quote, nil
}

// ParseSIPBar parses a raw JSON message into a SIPBar.
func ParseSIPBar(data []byte) (*SIPBar, error) {
	var bar SIPBar
	if err := json.Unmarshal(data, &bar); err != nil {
		return nil, err
	}
	return &bar, nil
}

// ParseSIPStatus parses a raw JSON message into a SIPStatus.
func ParseSIPStatus(data []byte) (*SIPStatus, error) {
	var status SIPStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

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
