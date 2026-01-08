package alpaca

import "fmt"

// Error represents an Alpaca error repsonse.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

var (
	ErrIOCOrdersNotSupportedForAlgoDestinations = &Error{Code: 42210000, Message: "IOC orders are not supported for algo destinations"}
)

func canonicalizeError(code int) error {
	switch code {
	default:
		return nil
	}
}
