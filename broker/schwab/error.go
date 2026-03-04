package schwab

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Error represents a Schwab API error response.
//
// Schwab returns two different error formats depending on the endpoint:
//
//	{"message": "string", "errors": ["string"]}
//	{"errors": [{"id": "...", "status": 500, "title": "Internal Server Error"}]}
//
// HTTP status codes indicate the error category:
//
// 400 - Bad Request: validation problem with the request
// 401 - Unauthorized: invalid token or no authorized accounts
// 403 - Forbidden: access denied
// 404 - Not Found: resource does not exist
// 500 - Internal Server Error: unexpected server error
// 503 - Service Unavailable: temporary server problem
type Error struct {
	Message string        `json:"message"`
	Errors  []ErrorDetail `json:"errors"`
}

// ErrorDetail represents a single error in the errors array.
// Schwab sometimes returns these as plain strings and sometimes as objects.
type ErrorDetail struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Raw    string // populated when the error is a plain string
}

func (d *ErrorDetail) UnmarshalJSON(data []byte) error {
	// try as string first (the simpler format from docs)
	if len(data) >= 2 && data[0] == '"' {
		d.Raw = string(data[1 : len(data)-1])
		return nil
	}
	// otherwise parse as object (the real format from the API)
	var obj struct {
		ID     string `json:"id"`
		Status int    `json:"status"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	d.ID = obj.ID
	d.Status = obj.Status
	d.Title = obj.Title
	d.Detail = obj.Detail
	return nil
}

func (d ErrorDetail) String() string {
	if d.Raw != "" {
		return d.Raw
	}
	if d.Title != "" {
		if d.Detail != "" {
			return fmt.Sprintf("%s: %s", d.Title, d.Detail)
		}
		return d.Title
	}
	return fmt.Sprintf("status %d", d.Status)
}

func (e *Error) Error() string {
	parts := make([]string, 0, len(e.Errors)+1)
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	for _, d := range e.Errors {
		parts = append(parts, d.String())
	}
	if len(parts) == 0 {
		return "unknown schwab error"
	}
	return strings.Join(parts, ": ")
}

// HasContent returns true if the error has any useful information.
func (e *Error) HasContent() bool {
	return e.Message != "" || len(e.Errors) > 0
}
