package binance

import (
	"fmt"
)

// Error represents a Binance API error.
// https://developers.binance.com/docs/binance-spot-api-docs/errors
type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Msg)
}

var (

	// 10xx - general server or network issues
	ErrUnknown                 = Error{-1000, "unknown error occurred while processing request"}
	ErrDisconnected            = Error{-1001, "internal error; unable to process your request; please try again"}
	ErrUnauthorized            = Error{-1002, "you are not authorized to execute this request"}
	ErrTooManyRequests         = Error{-1003, "too many requests; please use the websocket for live updates to avoid polling api"}
	ErrUnexpectedResp          = Error{-1006, "an unexpected response was received from the message bus; execution status unknown"}
	ErrTimeout                 = Error{-1007, "timeout waiting for response from backend server; send status unknown; execution status unknown"}
	ErrServerBusy              = Error{-1008, "server is currently overloaded with other requests; try again in a few minutes"}
	ErrInvalidMessage          = Error{-1013, "request is rejected by the api"}
	ErrUnknownOrderComposition = Error{-1014, "unsupported order combination"}
	ErrTooManyOrders           = Error{-1015, "too many new orders"}
	ErrServiceShuttingDown     = Error{-1016, "service no longer available"}
	ErrUnsupportedOperation    = Error{-1020, "operation not supported"}
	ErrInvalidTimestamp        = Error{-1021, "timestamp for request outside recvWindow"}
	ErrInvalidSignature        = Error{-1022, "signature for request not valid"}
	ErrCompIDInUse             = Error{-1033, "senderCompId currently in use; concurrent use of the same SenderCompId in one account not allowed"}
	ErrTooManyConnections      = Error{-1034, "too many concurrent connections"}
	ErrLoggedOut               = Error{-1035, "send logout message to close the session"}

	// 11xx - request issues
	ErrIllegalChars                   = Error{-1100, "illegal characters found in parameter"}
	ErrTooManyParams                  = Error{-1101, "too many parameters"}
	ErrMandatoryParamEmptyOrMalformed = Error{-1102, "mandatory parameter not sent, empty/null, or malformed"}
	ErrUnknownParam                   = Error{-1103, "unknown parameter sent"}
	ErrUnreadParameters               = Error{-1104, "not all sent parameters read"}
	ErrParamEmpty                     = Error{-1105, "parameter was empty"}
	ErrParamNotRequired               = Error{-1106, "parameter sent when not required"}
	ErrParamOverflow                  = Error{-1108, "parameter overflowed"}
	ErrBadPrecision                   = Error{-1111, "parameter has too much precision"}
	ErrNoDepth                        = Error{-1112, "no orders on book for symbol"}
	ErrTIFNotRequired                 = Error{-1114, "timeInForce sent when not required"}
	ErrInvalidTIF                     = Error{-1115, "invalid timeInForce"}
	ErrInvalidOrderType               = Error{-1116, "invalid orderType"}
	ErrInvalidSide                    = Error{-1117, "invalid side"}
	ErrEmptyNewClOrdID                = Error{-1118, "new client order id was empty"}
	ErrEmptyOrigClOrdID               = Error{-1119, "original client order id was empty"}
	ErrBadInterval                    = Error{-1120, "invalid interval"}
	ErrBadSymbol                      = Error{-1121, "invalid symbol"}
	ErrInvalidSymbolStatus            = Error{-1122, "invalid symbolStatus"}
	ErrInvalidListenKey               = Error{-1125, "listenKey does not exist"}
	ErrMoreThanXXHours                = Error{-1127, "lookup interval too big"}
	ErrOptionalParamsBadCombo         = Error{-1128, "combination of optional parameters invalid"}
	ErrInvalidParameter               = Error{-1130, "invalid data sent for parameter"}
	ErrBadStrategyType                = Error{-1134, "bad strategy type"}
	ErrInvalidJSON                    = Error{-1135, "invalid json request"}
	ErrInvalidTickerType              = Error{-1139, "invalid ticker type"}
	ErrInvalidCancelRestrictions      = Error{-1145, "cancelRestrictions must be ONLY_NEW or ONLY_PARTIALLY_FILLED"}
	ErrDuplicateSymbols               = Error{-1151, "symbol present multiple times in list"}
	ErrInvalidSBEHeader               = Error{-1152, "invalid x-mbx-sbe header"}
	ErrUnsupportedSchemaID            = Error{-1153, "unsupported sbe schema id or version in x-mbx-sbe header"}
	ErrSBEDisabled                    = Error{-1155, "sbe not enabled"}
	ErrOCOOrderTypeRejected           = Error{-1158, "order type not supported in oco"}
	ErrOCOIcebergQtyTimeInForce       = Error{-1160, "parameter not supported if aboveTimeInForce/belowTimeInForce is not gtc"}
	ErrDeprecatedSchema               = Error{-1161, "unable to encode the response in sbe schema"}
	ErrBuyOCOLimitMustBeBelow         = Error{-1165, "limit order in buy oco must be below"}
	ErrSellOCOLimitMustBeAbove        = Error{-1166, "limit order in sell oco must be above"}
	ErrBothOCOOrdersCannotBeLimit     = Error{-1168, "at least one oco order must be contingent"}
	ErrInvalidTagNumber               = Error{-1169, "invalid tag number"}
	ErrTagNotDefinedInMessage         = Error{-1170, "tag not defined for message type"}
	ErrTagAppearsMoreThanOnce         = Error{-1171, "tag appears more than once"}
	ErrTagOutOfOrder                  = Error{-1172, "tag specified out of required order"}
	ErrGroupFieldsOutOfOrder          = Error{-1173, "repeating group fields out of order"}
	ErrInvalidComponent               = Error{-1174, "component incorrectly populated"}
	ErrResetSeqNumSupport             = Error{-1175, "reset sequence number support is currently unsupported"}
	ErrAlreadyLoggedIn                = Error{-1176, "logon should only be sent once"}
	ErrGarbledMessage                 = Error{-1177, "garbled message"}
	ErrBadSenderCompID                = Error{-1178, "senderCompId contains an incorrect value"}
	ErrBadSeqNum                      = Error{-1179, "msgSeqNum contains an unexpected value"}
	ErrExpectedLogon                  = Error{-1180, "logon must be the first message in the session"}
	ErrTooManyMessages                = Error{-1181, "too many messages"}
	ErrParamsBadCombo                 = Error{-1182, "conflicting fields"}
	ErrNotAllowedInDropCopySessions   = Error{-1183, "requested operation is not allowed in DropCopy sessions"}
	ErrDropCopySessionNotAllowed      = Error{-1184, "dropCopy sessions are not supported on this server. Please reconnect to a drop copy server"}
	ErrDropCopySessionRequired        = Error{-1185, "only DropCopy sessions are supported on this server; either reconnect to order entry server or send DropCopyFlag field"}
	ErrNotAllowedInOrderEntrySessions = Error{-1186, "requested operation is not allowed in order entry sessions"}
	ErrNotAllowedInMarketDataSessions = Error{-1187, "requested operation is not allowed in market data sessions"}
	ErrIncorrectNumInGroupCount       = Error{-1188, "incorrect NumInGroup count for repeating group"}
	ErrDuplicateEntriesInAGroup       = Error{-1189, "duplicate entries in a group"}
	ErrInvalidRequestID               = Error{-1190, "invalid request id"}
	ErrTooManySubscriptions           = Error{-1191, "too many subscriptions"}
	ErrInvalidTimeUnit                = Error{-1194, "invalid time unit; expected either MICROSECOND or MILLISECOND"}
	ErrBuyOCOStopLossMustBeAbove      = Error{-1196, "a stop loss order in a buy oco must be above"}
	ErrSellOCOStopLossMustBeBelow     = Error{-1197, "a stop loss order in a sell oco must be below"}
	ErrBuyOCOTakeProfitMustBeBelow    = Error{-1198, "a take profit order in a buy oco must be below"}
	ErrSellOCOTakeProfitMustBeAbove   = Error{-1199, "a take profit order in a sell oco must be above"}
	ErrInvalidPegPriceType            = Error{-1210, "invalid pegPriceType"}
	ErrInvalidPegOffsetType           = Error{-1211, "invalid pegOffsetType"}
	ErrSymbolDoesNotMatchStatus       = Error{-1220, "the symbol's status does not match the requested symbolStatus"}
	ErrInvalidSBEMessageField         = Error{-1221, "invalid/missing field(s) in SBE message"}
	ErrOPOWorkingMustBeBuy            = Error{-1222, "working order in an OPO list must be a bid"}
	ErrOPOPendingMustBeSell           = Error{-1223, "pending orders in an OPO list must be asks"}
	ErrWorkingParamRequired           = Error{-1224, "working order must include particular tag"}
	ErrPendingParamNotRequired        = Error{-1225, "pending orders should not include a particular param"}

	// 20xx - order issues
	ErrNewOrderRejected       = Error{-2010, "new order rejected"}
	ErrCancelRejected         = Error{-2011, "cancel rejected"}
	ErrNoSuchOrder            = Error{-2013, "no such order"}
	ErrBadAPIKeyFmt           = Error{-2014, "bad api key format"}
	ErrRejectedMBXKey         = Error{-2015, "rejected api-key, ip, or permissions for action"}
	ErrNoTradingWindow        = Error{-2016, "no trading window could be found for the symbol; try ticker/24hrs instead"}
	ErrOrderArchived          = Error{-2026, "order was canceled or expired with no executed qty over 90 days ago and has been archived"}
	ErrSubscriptionActive     = Error{-2035, "user data stream subscription already active"}
	ErrSubscriptionInactive   = Error{-2036, "user Data Stream subscription not active"}
	ErrClientOrderIDInvalid   = Error{-2039, "client order id is not correct for this order id"}
	ErrMaximumSubscriptionIDs = Error{-2042, "maximum subscription ID reached for this connection"}
)

func canonicalizeError(code int) error {
	switch code {
	case -1000:
		return &ErrUnknown
	case -1001:
		return &ErrDisconnected
	case -1002:
		return &ErrUnauthorized
	case -1003:
		return &ErrTooManyRequests
	case -1006:
		return &ErrUnexpectedResp
	case -1007:
		return &ErrTimeout
	case -1008:
		return &ErrServerBusy
	case -1013:
		return &ErrInvalidMessage
	case -1014:
		return &ErrUnknownOrderComposition
	case -1015:
		return &ErrTooManyOrders
	case -1016:
		return &ErrServiceShuttingDown
	case -1020:
		return &ErrUnsupportedOperation
	case -1021:
		return &ErrInvalidTimestamp
	case -1022:
		return &ErrInvalidSignature
	case -1033:
		return &ErrCompIDInUse
	case -1034:
		return &ErrTooManyConnections
	case -1035:
		return &ErrLoggedOut
	case -1100:
		return &ErrIllegalChars
	case -1101:
		return &ErrTooManyParams
	case -1102:
		return &ErrMandatoryParamEmptyOrMalformed
	case -1103:
		return &ErrUnknownParam
	case -1104:
		return &ErrUnreadParameters
	case -1105:
		return &ErrParamEmpty
	case -1106:
		return &ErrParamNotRequired
	case -1108:
		return &ErrParamOverflow
	case -1111:
		return &ErrBadPrecision
	case -1112:
		return &ErrNoDepth
	case -1114:
		return &ErrTIFNotRequired
	case -1115:
		return &ErrInvalidTIF
	case -1116:
		return &ErrInvalidOrderType
	case -1117:
		return &ErrInvalidSide
	case -1118:
		return &ErrEmptyNewClOrdID
	case -1119:
		return &ErrEmptyOrigClOrdID
	case -1120:
		return &ErrBadInterval
	case -1121:
		return &ErrBadSymbol
	case -1122:
		return &ErrInvalidSymbolStatus
	case -1125:
		return &ErrInvalidListenKey
	case -1127:
		return &ErrMoreThanXXHours
	case -1128:
		return &ErrOptionalParamsBadCombo
	case -1130:
		return &ErrInvalidParameter
	case -1134:
		return &ErrBadStrategyType
	case -1135:
		return &ErrInvalidJSON
	case -1139:
		return &ErrInvalidTickerType
	case -1145:
		return &ErrInvalidCancelRestrictions
	case -1151:
		return &ErrDuplicateSymbols
	case -1152:
		return &ErrInvalidSBEHeader
	case -1153:
		return &ErrUnsupportedSchemaID
	case -1155:
		return &ErrSBEDisabled
	case -1158:
		return &ErrOCOOrderTypeRejected
	case -1160:
		return &ErrOCOIcebergQtyTimeInForce
	case -1161:
		return &ErrDeprecatedSchema
	case -1165:
		return &ErrBuyOCOLimitMustBeBelow
	case -1166:
		return &ErrSellOCOLimitMustBeAbove
	case -1168:
		return &ErrBothOCOOrdersCannotBeLimit
	case -1169:
		return &ErrInvalidTagNumber
	case -1170:
		return &ErrTagNotDefinedInMessage
	case -1171:
		return &ErrTagAppearsMoreThanOnce
	case -1172:
		return &ErrTagOutOfOrder
	case -1173:
		return &ErrGroupFieldsOutOfOrder
	case -1174:
		return &ErrInvalidComponent
	case -1175:
		return &ErrResetSeqNumSupport
	case -1176:
		return &ErrAlreadyLoggedIn
	case -1177:
		return &ErrGarbledMessage
	case -1178:
		return &ErrBadSenderCompID
	case -1179:
		return &ErrBadSeqNum
	case -1180:
		return &ErrExpectedLogon
	case -1181:
		return &ErrTooManyMessages
	case -1182:
		return &ErrParamsBadCombo
	case -1183:
		return &ErrNotAllowedInDropCopySessions
	case -1184:
		return &ErrDropCopySessionNotAllowed
	case -1185:
		return &ErrDropCopySessionRequired
	case -1186:
		return &ErrNotAllowedInOrderEntrySessions
	case -1187:
		return &ErrNotAllowedInMarketDataSessions
	case -1188:
		return &ErrIncorrectNumInGroupCount
	case -1189:
		return &ErrDuplicateEntriesInAGroup
	case -1190:
		return &ErrInvalidRequestID
	case -1191:
		return &ErrTooManySubscriptions
	case -1194:
		return &ErrInvalidTimeUnit
	case -1196:
		return &ErrBuyOCOStopLossMustBeAbove
	case -1197:
		return &ErrSellOCOStopLossMustBeBelow
	case -1198:
		return &ErrBuyOCOTakeProfitMustBeBelow
	case -1199:
		return &ErrSellOCOTakeProfitMustBeAbove
	case -1210:
		return &ErrInvalidPegPriceType
	case -1211:
		return &ErrInvalidPegOffsetType
	case -1220:
		return &ErrSymbolDoesNotMatchStatus
	case -1221:
		return &ErrInvalidSBEMessageField
	case -1222:
		return &ErrOPOWorkingMustBeBuy
	case -1223:
		return &ErrOPOPendingMustBeSell
	case -1224:
		return &ErrWorkingParamRequired
	case -1225:
		return &ErrPendingParamNotRequired
	case -2010:
		return &ErrNewOrderRejected
	case -2011:
		return &ErrCancelRejected
	case -2013:
		return &ErrNoSuchOrder
	case -2014:
		return &ErrBadAPIKeyFmt
	case -2015:
		return &ErrRejectedMBXKey
	case -2016:
		return &ErrNoTradingWindow
	case -2026:
		return &ErrOrderArchived
	case -2035:
		return &ErrSubscriptionActive
	case -2036:
		return &ErrSubscriptionInactive
	case -2039:
		return &ErrClientOrderIDInvalid
	case -2042:
		return &ErrMaximumSubscriptionIDs
	default:
		return nil
	}
}
