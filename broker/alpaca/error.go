package alpaca

import (
	"fmt"
)

// Error represents an Alpaca error response.
//
// Code format is HHHCCCCC where HHH is HTTP status, CCCCC is sub-code. Alpaca reuses codes
// for different kinds of errors, so only the HTTP status really matters. The only statuses
// that reliably tell you something without needing to do substring matching are 422, 404,
// and 429 (which is handled automatically). Other ones like 403 and 422 could mean you
// are misusing the api, you lost a race condition, and various other things.
//
// So pretty much the only error that's worthwhile and possible to cheaply canonicalize
// is the 404 Not Found error which should be returned as ds.ErrNotFound instead.
//
// 40010001 (422) - Parameter validation errors:
//   - "qty must be > 0"
//   - "limit price must be > 0"
//   - "limit orders require a limit price"
//   - "stop orders require a stop price"
//   - "stop limit orders require both stop and limit price"
//   - "only one of qty or notional is accepted"
//   - "trailing stop orders must specify one of trail_price or trail_percent"
//   - "trail_percent must be greater than or equal to 0.1"
//   - "order_id is missing"
//   - "client_order_id must be no more than 128 characters"
//
// 40310000 (403) - Forbidden errors (many different scenarios!):
//   - "opg orders must be submitted after 7:00pm and before 9:28am"
//   - "cls orders must be submitted after 7:00pm and before 3:55pm"
//   - "insufficient buying power" (includes buying_power and cost_basis fields)
//   - "cost basis must be >= minimal amount of order 1" (crypto minimum)
//   - "fractional trading is disabled for this account"
//   - wash trade prevention (crossing own order)
//
// 40410000 (404) - Not found errors:
//   - "symbol not found: XXX"
//   - "position does not exist"
//   - "order not found for XXX"
//
// 42210000 (422) - Business logic / unprocessable errors:
//   - "asset \"XXX\" not found"
//   - "extended hours order must be DAY limit orders"
//   - "advanced instructions only support DAY time_in_force"
//   - "position intent mismatch, inferred: X, specified: Y"
//   - "unable to replace order, order is not open"
//   - "FOK orders are not supported for algo destinations"
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// Status returns the HTTP status code portion of the error code.
func (e *Error) Status() int {
	return e.Code / 1000000
}

// SubCode returns the sub-code portion of the error code.
func (e *Error) SubCode() int {
	return e.Code % 1000000
}
