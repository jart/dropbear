package sip

import "errors"

var (
	ErrOverflow             = errors.New("sip overflow")
	ErrInvalidTape          = errors.New("invalid sip tape")
	ErrInvalidExchange      = errors.New("invalid sip exchange")
	ErrInvalidQuoteCond     = errors.New("invalid sip quote condition")
	ErrInvalidTradeCond     = errors.New("invalid sip trade condition")
	ErrInvalidStatusCode    = errors.New("invalid sip status code")
	ErrInvalidReasonCode    = errors.New("invalid sip reason code")
	ErrInvalidMessageType   = errors.New("invalid sip message type")
	ErrInvalidLULDIndicator = errors.New("invalid sip luld indicator")
	ErrInvalidStatusMessage = errors.New("invalid sip status message")
	ErrParsingError         = errors.New("sip parsing error")
	ErrTypeMismatch         = errors.New("sip parsing error: type mismatch")
	ErrMissingField         = errors.New("sip parsing error: missing field")
	ErrUnrecognizedField    = errors.New("sip parsing error: unrecognized field")
	ErrNegativeQuoteSize    = errors.New("sip parsing error: negative quote size")
	ErrNegativeTradeSize    = errors.New("sip parsing error: negative trade size")
)
