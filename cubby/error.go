package cubby

import "errors"

var (
	ErrUnknown           = errors.New("unknown symbol")
	ErrNotEquity         = errors.New("asset not equity")
	ErrIsRunning         = errors.New("cubby is running")
	ErrIsWarmingUp       = errors.New("cubby is warming up")
	ErrNotTradable       = errors.New("asset is not tradable")
	ErrNotShortable      = errors.New("asset is not shortable")
	ErrZeroQuantity      = errors.New("quantity cannot be zero")
	ErrMarketNotOpen     = errors.New("market is not open")
	ErrNegativePrice     = errors.New("price cannot be negative")
	ErrNotEasyToBorrow   = errors.New("asset is not easy to borrow")
	ErrPriceNotRounded   = errors.New("price is not rounded to cent")
	ErrFractionalShares  = errors.New("quantity cannot be fractional")
	ErrMissedLOODeadline = errors.New("missed market open auction deadline")
)
