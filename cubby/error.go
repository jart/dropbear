package cubby

import "errors"

var (
	ErrUnknown           = errors.New("unknown symbol")
	ErrRunning           = errors.New("cubby is running")
	ErrNotEquity         = errors.New("asset not an equity")
	ErrIsRunning         = errors.New("cubby is running")
	ErrIsWarmingUp       = errors.New("cubby is warming up")
	ErrNotTradable       = errors.New("asset is not tradable")
	ErrNotShortable      = errors.New("asset is not shortable")
	ErrZeroQuantity      = errors.New("quantity cannot be zero")
	ErrNegativePrice     = errors.New("price cannot be negative")
	ErrNotEasyToBorrow   = errors.New("asset is not easy to borrow")
	ErrFractionalShares  = errors.New("quantity cannot be fractional")
	ErrOrderOverlapsZero = errors.New("order quantity would flip position")
)
