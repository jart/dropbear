package schwab

import (
	"fmt"
	"strings"
)

// Error represents a Schwab API error response.
//
// Schwab returns errors as JSON with a message string and an array of error detail strings.
// HTTP status codes indicate the error category:
//
// 400 - Bad Request: validation problem with the request
// 401 - Unauthorized: invalid token or no authorized accounts
// 403 - Forbidden: access denied
// 404 - Not Found: resource does not exist
// 500 - Internal Server Error: unexpected server error
// 503 - Service Unavailable: temporary server problem
type Error struct {
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}

func (e *Error) Error() string {
	if len(e.Errors) == 0 {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Message, strings.Join(e.Errors, "; "))
}
