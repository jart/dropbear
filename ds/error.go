package ds

import "errors"

var (
	ErrNotFound            = errors.New("not found")
	ErrSelfTrade           = errors.New("self trading")
	ErrOrderNotOpen        = errors.New("order not open")
	ErrOrderPendingReplace = errors.New("order pending replace")
	ErrOrderPendingCancel  = errors.New("order pending cancel")
	ErrPostOnly            = errors.New("order would cross spread")
	ErrRFQCannotEdit       = errors.New("rfq orders cannot be edited")
	ErrInsufficientFunds   = errors.New("we require more vespene gas")
	ErrTooManyRequests     = errors.New("too many requests")
	ErrBusy                = errors.New("resource busy")
)
