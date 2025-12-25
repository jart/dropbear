package auth

import "encoding/json"

// ClientData represents the client data JSON structure
type ClientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin,omitempty"`
}

func parseClientData(data []byte) (*ClientData, error) {
	var cd ClientData
	if err := json.Unmarshal(data, &cd); err != nil {
		return nil, err
	}
	return &cd, nil
}
