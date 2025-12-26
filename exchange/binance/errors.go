package binance

import "errors"

// Binance Error Codes
// https://developers.binance.com/docs/binance-spot-api-docs/errors
// https://developers.binance.com/docs/derivatives/usds-margined-futures/error-code

var (

	// 10xx - General Server or Network issues
	ErrUnknown                 = errors.New("an unknown error occurred while processing the request")
	ErrDisconnected            = errors.New("internal error; unable to process your request")
	ErrUnauthorized            = errors.New("you are not authorized to execute this request")
	ErrTooManyRequests         = errors.New("too many requests queued")
	ErrDuplicateIP             = errors.New("this IP is already on the white list")
	ErrNoSuchIP                = errors.New("no such IP has been white listed")
	ErrUnexpectedResp          = errors.New("an unexpected response was received from the message bus")
	ErrTimeout                 = errors.New("timeout waiting for response from backend server")
	ErrServerBusy              = errors.New("server is currently overloaded with other requests")
	ErrErrorMsgReceived        = errors.New("error message received")
	ErrNonWhiteList            = errors.New("this IP cannot access this route")
	ErrInvalidMessage          = errors.New("the request is rejected by the API")
	ErrUnknownOrderComposition = errors.New("unsupported order combination")
	ErrTooManyOrders           = errors.New("too many new orders")
	ErrServiceShuttingDown     = errors.New("this service is no longer available")
	ErrUnsupportedOperation    = errors.New("this operation is not supported")
	ErrInvalidTimestamp        = errors.New("timestamp for this request is outside of the recvWindow")
	ErrInvalidSignature        = errors.New("signature for this request is not valid")
	ErrStartTimeGreaterThanEnd = errors.New("start time is greater than end time")
	ErrCompIdInUse             = errors.New("SenderCompId is currently in use")
	ErrTooManyConnections      = errors.New("too many concurrent connections")
	ErrLoggedOut               = errors.New("please send Logout message to close the session")
	ErrNotFound                = errors.New("not found, unauthenticated, or unauthorized")

	// 11xx - Request issues
	ErrIllegalChars                   = errors.New("illegal characters found in a parameter")
	ErrTooManyParameters              = errors.New("too many parameters sent for this endpoint")
	ErrMandatoryParamEmptyOrMalformed = errors.New("a mandatory parameter was not sent, was empty/null, or malformed")
	ErrUnknownParam                   = errors.New("an unknown parameter was sent")
	ErrUnreadParameters               = errors.New("not all sent parameters were read")
	ErrParamEmpty                     = errors.New("a parameter was empty")
	ErrParamNotRequired               = errors.New("a parameter was sent when not required")
	ErrBadAsset                       = errors.New("invalid asset")
	ErrBadAccount                     = errors.New("invalid account")
	ErrBadInstrumentType              = errors.New("invalid symbolType")
	ErrParamOverflow                  = errors.New("parameter overflowed")
	ErrBadPrecision                   = errors.New("parameter has too much precision")
	ErrNoDepth                        = errors.New("no orders on book for symbol")
	ErrWithdrawNotNegative            = errors.New("withdrawal amount must be negative")
	ErrTifNotRequired                 = errors.New("timeInForce parameter sent when not required")
	ErrInvalidTif                     = errors.New("invalid timeInForce")
	ErrInvalidOrderType               = errors.New("invalid orderType")
	ErrInvalidSide                    = errors.New("invalid side")
	ErrEmptyNewClOrdId                = errors.New("new client order ID was empty")
	ErrEmptyOrgClOrdId                = errors.New("original client order ID was empty")
	ErrBadInterval                    = errors.New("invalid interval")
	ErrBadSymbol                      = errors.New("invalid symbol")
	ErrInvalidSymbolStatus            = errors.New("invalid symbolStatus")
	ErrInvalidListenKey               = errors.New("this listenKey does not exist")
	ErrAssetNotSupported              = errors.New("this asset is not supported")
	ErrMoreThanXXHours                = errors.New("lookup interval is too big")
	ErrOptionalParamsBadCombo         = errors.New("combination of optional parameters invalid")
	ErrInvalidParameter               = errors.New("invalid data sent for a parameter")
	ErrBadStrategyType                = errors.New("strategyType was less than 1000000")
	ErrInvalidJson                    = errors.New("invalid JSON request")
	ErrInvalidNewOrderRespType        = errors.New("invalid newOrderRespType")
	ErrInvalidTickerType              = errors.New("invalid ticker type")
	ErrInvalidCancelRestrictions      = errors.New("cancelRestrictions has to be either ONLY_NEW or ONLY_PARTIALLY_FILLED")
	ErrDuplicateSymbols               = errors.New("symbol is present multiple times in the list")
	ErrInvalidSbeHeader               = errors.New("invalid X-MBX-SBE header")
	ErrUnsupportedSchemaId            = errors.New("unsupported SBE schema ID or version")
	ErrSbeDisabled                    = errors.New("SBE is not enabled")
	ErrOcoOrderTypeRejected           = errors.New("order type not supported in OCO")
	ErrOcoIcebergQtyTimeInForce       = errors.New("timeInForce must be GTC when icebergQty is used")
	ErrDeprecatedSchema               = errors.New("unable to encode the response in SBE schema")
	ErrBuyOcoLimitMustBeBelow         = errors.New("a limit order in a buy OCO must be below")
	ErrSellOcoLimitMustBeAbove        = errors.New("a limit order in a sell OCO must be above")
	ErrBothOcoOrdersCannotBeLimit     = errors.New("at least one OCO order must be contingent")
	ErrInvalidTagNumber               = errors.New("invalid tag number")
	ErrTagNotDefinedInMessage         = errors.New("tag not defined for this message type")
	ErrTagAppearsMoreThanOnce         = errors.New("tag appears more than once")
	ErrTagOutOfOrder                  = errors.New("tag specified out of required order")
	ErrGroupFieldsOutOfOrder          = errors.New("repeating group fields out of order")
	ErrInvalidComponent               = errors.New("component is incorrectly populated")
	ErrResetSeqNumSupport             = errors.New("sequence numbers must be reset for each new session")
	ErrAlreadyLoggedIn                = errors.New("Logon should only be sent once")
	ErrGarbledMessage                 = errors.New("garbled message")
	ErrBadSenderCompId                = errors.New("SenderCompId contains an incorrect value")
	ErrBadSeqNum                      = errors.New("MsgSeqNum contains an unexpected value")
	ErrExpectedLogon                  = errors.New("Logon must be the first message in the session")
	ErrTooManyMessages                = errors.New("too many messages")
	ErrParamsBadCombo                 = errors.New("conflicting fields")
	ErrNotAllowedInDropCopySessions   = errors.New("requested operation is not allowed in DropCopy sessions")
	ErrDropCopySessionNotAllowed      = errors.New("DropCopy sessions are not supported on this server")
	ErrDropCopySessionRequired        = errors.New("only DropCopy sessions are supported on this server")
	ErrNotAllowedInOrderEntrySessions = errors.New("requested operation is not allowed in order entry sessions")
	ErrNotAllowedInMarketDataSessions = errors.New("requested operation is not allowed in market data sessions")
	ErrIncorrectNumInGroupCount       = errors.New("incorrect NumInGroup count for repeating group")
	ErrDuplicateEntriesInGroup        = errors.New("group contains duplicate entries")
	ErrInvalidRequestId               = errors.New("invalid MDReqID")
	ErrTooManySubscriptions           = errors.New("too many subscriptions")
	ErrInvalidTimeUnit                = errors.New("invalid value for time unit")
	ErrBuyOcoStopLossMustBeAbove      = errors.New("a stop loss order in a buy OCO must be above")
	ErrSellOcoStopLossMustBeBelow     = errors.New("a stop loss order in a sell OCO must be below")
	ErrBuyOcoTakeProfitMustBeBelow    = errors.New("a take profit order in a buy OCO must be below")
	ErrSellOcoTakeProfitMustBeAbove   = errors.New("a take profit order in a sell OCO must be above")
	ErrInvalidPegPriceType            = errors.New("invalid pegPriceType")
	ErrInvalidPegOffsetType           = errors.New("invalid pegOffsetType")
	ErrSymbolDoesNotMatchStatus       = errors.New("the symbol's status does not match the requested symbolStatus")
	ErrInvalidSbeMessageField         = errors.New("invalid/missing field(s) in SBE message")
	ErrOpoWorkingMustBeBuy            = errors.New("working order in an OPO list must be a bid")
	ErrOpoPendingMustBeSell           = errors.New("pending orders in an OPO list must be asks")
	ErrWorkingParamRequired           = errors.New("working order must include required tag")
	ErrPendingParamNotRequired        = errors.New("pending orders should not include this tag")

	// 20xx - Processing issues
	ErrNewOrderRejected                = errors.New("new order rejected")
	ErrCancelRejected                  = errors.New("cancel rejected")
	ErrCancelAllFail                   = errors.New("batch cancel failure")
	ErrNoSuchOrder                     = errors.New("order does not exist")
	ErrBadApiKeyFmt                    = errors.New("API-key format invalid")
	ErrRejectedMbxKey                  = errors.New("invalid API-key, IP, or permissions for action")
	ErrNoTradingWindow                 = errors.New("no trading window could be found for the symbol")
	ErrApiKeysLocked                   = errors.New("API Keys are locked on this account")
	ErrBalanceNotSufficient            = errors.New("balance is insufficient")
	ErrMarginNotSufficient             = errors.New("margin is insufficient")
	ErrUnableToFill                    = errors.New("unable to fill")
	ErrOrderWouldImmediatelyTrigger    = errors.New("order would immediately trigger")
	ErrReduceOnlyReject                = errors.New("reduce only order is rejected")
	ErrUserInLiquidation               = errors.New("user in liquidation mode now")
	ErrPositionNotSufficient           = errors.New("position is not sufficient")
	ErrMaxOpenOrderExceeded            = errors.New("reach max open order limit")
	ErrOrderArchived                   = errors.New("order was canceled or expired with no executed qty over 90 days ago and has been archived")
	ErrReduceOnlyOrderTypeNotSupported = errors.New("this OrderType is not supported when reduceOnly")
	ErrMaxLeverageRatio                = errors.New("exceeded the maximum allowable position at current leverage")
	ErrMinLeverageRatio                = errors.New("leverage is smaller than permitted: insufficient margin balance")
	ErrSubscriptionActive              = errors.New("user data stream subscription already active")
	ErrSubscriptionInactive            = errors.New("user data stream subscription not active")
	ErrClientOrderIdInvalid            = errors.New("client order ID is not correct for this order ID")
	ErrMaximumSubscriptionIds          = errors.New("maximum subscription ID reached for this connection")

	// 40xx - Filters and other issues
	ErrInvalidOrderStatus                      = errors.New("invalid order status")
	ErrPriceLessThanZero                       = errors.New("price less than 0")
	ErrPriceGreaterThanMaxPrice                = errors.New("price greater than max price")
	ErrQtyLessThanZero                         = errors.New("quantity less than zero")
	ErrQtyLessThanMinQty                       = errors.New("quantity less than min quantity")
	ErrQtyGreaterThanMaxQty                    = errors.New("quantity greater than max quantity")
	ErrStopPriceLessThanZero                   = errors.New("stop price less than zero")
	ErrStopPriceGreaterThanMaxPrice            = errors.New("stop price greater than max price")
	ErrTickSizeLessThanZero                    = errors.New("tick size less than zero")
	ErrMaxPriceLessThanMinPrice                = errors.New("max price less than min price")
	ErrMaxQtyLessThanMinQty                    = errors.New("max qty less than min qty")
	ErrStepSizeLessThanZero                    = errors.New("step size less than zero")
	ErrMaxNumOrdersLessThanZero                = errors.New("max num orders less than zero")
	ErrPriceLessThanMinPrice                   = errors.New("price less than min price")
	ErrPriceNotIncreasedByTickSize             = errors.New("price not increased by tick size")
	ErrInvalidClOrdIdLen                       = errors.New("client order id is not valid")
	ErrPriceHigherThanMultiplierUp             = errors.New("price is higher than mark price multiplier cap")
	ErrMultiplierUpLessThanZero                = errors.New("multiplier up less than zero")
	ErrMultiplierDownLessThanZero              = errors.New("multiplier down less than zero")
	ErrCompositeScaleOverflow                  = errors.New("composite scale too large")
	ErrTargetStrategyInvalid                   = errors.New("target strategy invalid")
	ErrInvalidDepthLimit                       = errors.New("invalid depth limit")
	ErrWrongMarketStatus                       = errors.New("market status sent is not valid")
	ErrQtyNotIncreasedByStepSize               = errors.New("qty not increased by step size")
	ErrPriceLowerThanMultiplierDown            = errors.New("price is lower than mark price multiplier floor")
	ErrMultiplierDecimalLessThanZero           = errors.New("multiplier decimal less than zero")
	ErrCommissionInvalid                       = errors.New("commission invalid")
	ErrInvalidAccountType                      = errors.New("invalid account type")
	ErrInvalidLeverage                         = errors.New("invalid leverage")
	ErrInvalidTickSizePrecision                = errors.New("tick size precision is invalid")
	ErrInvalidStepSizePrecision                = errors.New("step size precision is invalid")
	ErrInvalidWorkingType                      = errors.New("invalid parameter working type")
	ErrExceedMaxCancelOrderSize                = errors.New("exceed maximum cancel order size")
	ErrInsuranceAccountNotFound                = errors.New("insurance account not found")
	ErrInvalidBalanceType                      = errors.New("balance type is invalid")
	ErrMaxStopOrderExceeded                    = errors.New("reach max stop order limit")
	ErrNoNeedToChangeMarginType                = errors.New("no need to change margin type")
	ErrThereExistsOpenOrders                   = errors.New("margin type cannot be changed if there exists open orders")
	ErrThereExistsQuantity                     = errors.New("margin type cannot be changed if there exists position")
	ErrAddIsolatedMarginReject                 = errors.New("add margin only support for isolated position")
	ErrCrossBalanceInsufficient                = errors.New("cross balance insufficient")
	ErrIsolatedBalanceInsufficient             = errors.New("isolated balance insufficient")
	ErrNoNeedToChangeAutoAddMargin             = errors.New("no need to change auto add margin")
	ErrAutoAddCrossedMarginReject              = errors.New("auto add margin only support for isolated position")
	ErrAddIsolatedMarginNoPositionReject       = errors.New("cannot add position margin: position is 0")
	ErrAmountMustBePositive                    = errors.New("amount must be positive")
	ErrInvalidApiKeyType                       = errors.New("invalid api key type")
	ErrInvalidRsaPublicKey                     = errors.New("invalid api public key")
	ErrMaxPriceTooLarge                        = errors.New("maxPrice and priceDecimal too large")
	ErrNoNeedToChangePositionSide              = errors.New("no need to change position side")
	ErrInvalidPositionSide                     = errors.New("invalid position side")
	ErrPositionSideNotMatch                    = errors.New("order's position side does not match user's setting")
	ErrReduceOnlyConflict                      = errors.New("invalid or improper reduceOnly value")
	ErrInvalidOptionsRequestType               = errors.New("invalid options request type")
	ErrInvalidOptionsTimeFrame                 = errors.New("invalid options time frame")
	ErrInvalidOptionsAmount                    = errors.New("invalid options amount")
	ErrInvalidOptionsEventType                 = errors.New("invalid options event type")
	ErrPositionSideChangeExistsOpenOrders      = errors.New("position side cannot be changed if there exists open orders")
	ErrPositionSideChangeExistsQuantity        = errors.New("position side cannot be changed if there exists position")
	ErrInvalidOptionsPremiumFee                = errors.New("invalid options premium fee")
	ErrInvalidClOptionsIdLen                   = errors.New("client options id is not valid")
	ErrInvalidOptionsDirection                 = errors.New("invalid options direction")
	ErrOptionsPremiumNotUpdate                 = errors.New("premium fee is not updated, reject order")
	ErrOptionsPremiumInputLessThanZero         = errors.New("input premium fee is less than 0, reject order")
	ErrOptionsAmountBiggerThanUpper            = errors.New("order amount is bigger than upper boundary or less than 0")
	ErrOptionsPremiumOutputZero                = errors.New("output premium fee is less than 0, reject order")
	ErrOptionsPremiumTooDiff                   = errors.New("original fee is too much higher than last fee")
	ErrOptionsPremiumReachLimit                = errors.New("place order amount has reached to limit, reject order")
	ErrOptionsCommonError                      = errors.New("options internal error")
	ErrInvalidOptionsId                        = errors.New("invalid options id")
	ErrOptionsUserNotFound                     = errors.New("user not found")
	ErrOptionsNotFound                         = errors.New("options not found")
	ErrInvalidBatchPlaceOrderSize              = errors.New("invalid number of batch place orders")
	ErrPlaceBatchOrdersFail                    = errors.New("fail to place batch orders")
	ErrUpcomingMethod                          = errors.New("method is not allowed currently, upcoming soon")
	ErrInvalidNotionalLimitCoef                = errors.New("invalid notional limit coefficient")
	ErrInvalidPriceSpreadThreshold             = errors.New("invalid price spread threshold")
	ErrReduceOnlyOrderPermission               = errors.New("user can only place reduce only order")
	ErrNoPlaceOrderPermission                  = errors.New("user can not place order currently")
	ErrInvalidContractType                     = errors.New("invalid contract type")
	ErrInactiveAccount                         = errors.New("inactive account")
	ErrInvalidClientTranIdLen                  = errors.New("clientTranId is not valid")
	ErrDuplicatedClientTranId                  = errors.New("clientTranId is duplicated")
	ErrDuplicatedClientOrderId                 = errors.New("clientOrderId is duplicated")
	ErrStopOrderTriggering                     = errors.New("stop order is triggering")
	ErrReduceOnlyMarginCheckFailed             = errors.New("reduce only order failed, please check your existing position and open orders")
	ErrStopOrderSwitchAlgo                     = errors.New("order type not supported for this endpoint, please use the Algo Order API endpoints instead")
	ErrMarketOrderReject                       = errors.New("the counterparty's best price does not meet the PERCENT_PRICE filter limit")
	ErrInvalidActivationPrice                  = errors.New("invalid activation price")
	ErrQuantityExistsWithClosePosition         = errors.New("quantity must be zero with closePosition equals true")
	ErrReduceOnlyMustBeTrue                    = errors.New("reduce only must be true with closePosition equals true")
	ErrOrderTypeCannotBeMkt                    = errors.New("order type can not be market if it's unable to cancel")
	ErrInvalidOpeningPositionStatus            = errors.New("invalid symbol status for opening position")
	ErrSymbolAlreadyClosed                     = errors.New("symbol is closed")
	ErrStrategyInvalidTriggerPrice             = errors.New("take profit or stop order will be triggered immediately")
	ErrInvalidPair                             = errors.New("invalid pair")
	ErrIsolatedLeverageRejectWithPosition      = errors.New("leverage reduction is not supported in Isolated Margin Mode with open positions")
	ErrMinNotional                             = errors.New("order's notional must be no smaller than 5.0 (unless you choose reduce only)")
	ErrInvalidTimeInterval                     = errors.New("invalid time interval")
	ErrIsolatedRejectWithJointMargin           = errors.New("unable to adjust to Multi-Assets mode with symbols under isolated-margin mode")
	ErrJointMarginRejectWithIsolated           = errors.New("unable to adjust to isolated-margin mode under the Multi-Assets mode")
	ErrJointMarginRejectWithMB                 = errors.New("unable to adjust Multi-Assets Mode with insufficient margin balance")
	ErrJointMarginRejectWithOpenOrder          = errors.New("unable to adjust Multi-Assets Mode with open orders")
	ErrNoNeedToChangeJointMargin               = errors.New("adjusted asset mode is currently set and does not need to be adjusted repeatedly")
	ErrJointMarginRejectWithNegativeBalance    = errors.New("unable to adjust Multi-Assets Mode with a negative wallet balance")
	ErrPriceHigherThanStopMultiplierUp         = errors.New("price is higher than stop price multiplier cap")
	ErrPriceLowerThanStopMultiplierDown        = errors.New("price is lower than stop price multiplier floor")
	ErrCoolingOffPeriod                        = errors.New("trade forbidden due to Cooling-off Period")
	ErrAdjustLeverageKycFailed                 = errors.New("intermediate Personal Verification is required for adjusting leverage over 20x")
	ErrAdjustLeverageOneMonthFailed            = errors.New("more than 20x leverage is available one month after account registration")
	ErrAdjustLeverageXDaysFailed               = errors.New("more than 20x leverage is available after Futures account registration")
	ErrAdjustLeverageKycLimit                  = errors.New("users in this country has limited adjust leverage")
	ErrAdjustLeverageAccountSymbolFailed       = errors.New("current symbol leverage cannot exceed 20 when using position limit adjustment service")
	ErrAdjustLeverageSymbolFailed              = errors.New("the max leverage of Symbol is 20x")
	ErrStopPriceHigherThanPriceMultiplierLimit = errors.New("stop price is higher than price multiplier cap")
	ErrStopPriceLowerThanPriceMultiplierLimit  = errors.New("stop price is lower than price multiplier floor")
	ErrTradingQuantitativeRule                 = errors.New("futures Trading Quantitative Rules violated, only reduceOnly order is allowed")
	ErrLargePositionSymRule                    = errors.New("futures Trading Risk Control Rules of large position holding violated, only reduceOnly order is allowed")
	ErrComplianceBlackSymbolRestriction        = errors.New("as per Terms of Use and compliance with local regulations, this feature is currently not available in your region")
	ErrAdjustLeverageComplianceFailed          = errors.New("as per Terms of Use and compliance with local regulations, the leverage is limited in your region")

	// 50xx - Order Execution issues
	ErrFokOrderReject                  = errors.New("due to the order could not be filled immediately, the FOK order has been rejected")
	ErrGtxOrderReject                  = errors.New("due to the order could not be executed as maker, the Post Only order will be rejected")
	ErrMoveOrderNotAllowedSymbolReason = errors.New("symbol is not in trading status, order amendment is not permitted")
	ErrLimitOrderOnly                  = errors.New("only limit order is supported")
	ErrExceedMaximumModifyOrderLimit   = errors.New("exceed maximum modify order limit")
	ErrSameOrder                       = errors.New("no need to modify the order")
	ErrMeRecvwindowReject              = errors.New("timestamp for this request is outside of the ME recvWindow")
	ErrModificationMinNotional         = errors.New("order's notional must be no smaller than minimum")
	ErrInvalidPriceMatch               = errors.New("invalid price match")
	ErrUnsupportedOrderTypePriceMatch  = errors.New("price match only supports order type: LIMIT, STOP AND TAKE_PROFIT")
	ErrInvalidSelfTradePreventionMode  = errors.New("invalid self trade prevention mode")
	ErrFutureGoodTillDate              = errors.New("the goodTillDate timestamp must be greater than current time plus 600 seconds")
	ErrBboOrderReject                  = errors.New("no depth matches this BBO order")
	ErrExistingPendingModification     = errors.New("a pending modification already exists for this order")
)

var binanceErrorCodes = map[int]error{
	// 10xx - General Server or Network issues
	-1000: ErrUnknown,
	-1001: ErrDisconnected,
	-1002: ErrUnauthorized,
	-1003: ErrTooManyRequests,
	-1004: ErrDuplicateIP,
	-1005: ErrNoSuchIP,
	-1006: ErrUnexpectedResp,
	-1007: ErrTimeout,
	-1008: ErrServerBusy,
	-1010: ErrErrorMsgReceived,
	-1011: ErrNonWhiteList,
	-1013: ErrInvalidMessage,
	-1014: ErrUnknownOrderComposition,
	-1015: ErrTooManyOrders,
	-1016: ErrServiceShuttingDown,
	-1020: ErrUnsupportedOperation,
	-1021: ErrInvalidTimestamp,
	-1022: ErrInvalidSignature,
	-1023: ErrStartTimeGreaterThanEnd,
	-1033: ErrCompIdInUse,
	-1034: ErrTooManyConnections,
	-1035: ErrLoggedOut,
	-1099: ErrNotFound,

	// 11xx - Request issues
	-1100: ErrIllegalChars,
	-1101: ErrTooManyParameters,
	-1102: ErrMandatoryParamEmptyOrMalformed,
	-1103: ErrUnknownParam,
	-1104: ErrUnreadParameters,
	-1105: ErrParamEmpty,
	-1106: ErrParamNotRequired,
	-1108: ErrBadAsset,
	-1109: ErrBadAccount,
	-1110: ErrBadInstrumentType,
	-1111: ErrBadPrecision,
	-1112: ErrNoDepth,
	-1113: ErrWithdrawNotNegative,
	-1114: ErrTifNotRequired,
	-1115: ErrInvalidTif,
	-1116: ErrInvalidOrderType,
	-1117: ErrInvalidSide,
	-1118: ErrEmptyNewClOrdId,
	-1119: ErrEmptyOrgClOrdId,
	-1120: ErrBadInterval,
	-1121: ErrBadSymbol,
	-1122: ErrInvalidSymbolStatus,
	-1125: ErrInvalidListenKey,
	-1126: ErrAssetNotSupported,
	-1127: ErrMoreThanXXHours,
	-1128: ErrOptionalParamsBadCombo,
	-1130: ErrInvalidParameter,
	-1134: ErrBadStrategyType,
	-1135: ErrInvalidJson,
	-1136: ErrInvalidNewOrderRespType,
	-1139: ErrInvalidTickerType,
	-1145: ErrInvalidCancelRestrictions,
	-1151: ErrDuplicateSymbols,
	-1152: ErrInvalidSbeHeader,
	-1153: ErrUnsupportedSchemaId,
	-1155: ErrSbeDisabled,
	-1158: ErrOcoOrderTypeRejected,
	-1160: ErrOcoIcebergQtyTimeInForce,
	-1161: ErrDeprecatedSchema,
	-1165: ErrBuyOcoLimitMustBeBelow,
	-1166: ErrSellOcoLimitMustBeAbove,
	-1168: ErrBothOcoOrdersCannotBeLimit,
	-1169: ErrInvalidTagNumber,
	-1170: ErrTagNotDefinedInMessage,
	-1171: ErrTagAppearsMoreThanOnce,
	-1172: ErrTagOutOfOrder,
	-1173: ErrGroupFieldsOutOfOrder,
	-1174: ErrInvalidComponent,
	-1175: ErrResetSeqNumSupport,
	-1176: ErrAlreadyLoggedIn,
	-1177: ErrGarbledMessage,
	-1178: ErrBadSenderCompId,
	-1179: ErrBadSeqNum,
	-1180: ErrExpectedLogon,
	-1181: ErrTooManyMessages,
	-1182: ErrParamsBadCombo,
	-1183: ErrNotAllowedInDropCopySessions,
	-1184: ErrDropCopySessionNotAllowed,
	-1185: ErrDropCopySessionRequired,
	-1186: ErrNotAllowedInOrderEntrySessions,
	-1187: ErrNotAllowedInMarketDataSessions,
	-1188: ErrIncorrectNumInGroupCount,
	-1189: ErrDuplicateEntriesInGroup,
	-1190: ErrInvalidRequestId,
	-1191: ErrTooManySubscriptions,
	-1194: ErrInvalidTimeUnit,
	-1196: ErrBuyOcoStopLossMustBeAbove,
	-1197: ErrSellOcoStopLossMustBeBelow,
	-1198: ErrBuyOcoTakeProfitMustBeBelow,
	-1199: ErrSellOcoTakeProfitMustBeAbove,
	-1210: ErrInvalidPegPriceType,
	-1211: ErrInvalidPegOffsetType,
	-1220: ErrSymbolDoesNotMatchStatus,
	-1221: ErrInvalidSbeMessageField,
	-1222: ErrOpoWorkingMustBeBuy,
	-1223: ErrOpoPendingMustBeSell,
	-1224: ErrWorkingParamRequired,
	-1225: ErrPendingParamNotRequired,

	// 20xx - Processing issues
	-2010: ErrNewOrderRejected,
	-2011: ErrCancelRejected,
	-2012: ErrCancelAllFail,
	-2013: ErrNoSuchOrder,
	-2014: ErrBadApiKeyFmt,
	-2015: ErrRejectedMbxKey,
	-2016: ErrNoTradingWindow,
	-2017: ErrApiKeysLocked,
	-2018: ErrBalanceNotSufficient,
	-2019: ErrMarginNotSufficient,
	-2020: ErrUnableToFill,
	-2021: ErrOrderWouldImmediatelyTrigger,
	-2022: ErrReduceOnlyReject,
	-2023: ErrUserInLiquidation,
	-2024: ErrPositionNotSufficient,
	-2025: ErrMaxOpenOrderExceeded,
	-2026: ErrReduceOnlyOrderTypeNotSupported,
	-2027: ErrMaxLeverageRatio,
	-2028: ErrMinLeverageRatio,
	-2035: ErrSubscriptionActive,
	-2036: ErrSubscriptionInactive,
	-2039: ErrClientOrderIdInvalid,
	-2042: ErrMaximumSubscriptionIds,

	// 40xx - Filters and other issues
	-4000: ErrInvalidOrderStatus,
	-4001: ErrPriceLessThanZero,
	-4002: ErrPriceGreaterThanMaxPrice,
	-4003: ErrQtyLessThanZero,
	-4004: ErrQtyLessThanMinQty,
	-4005: ErrQtyGreaterThanMaxQty,
	-4006: ErrStopPriceLessThanZero,
	-4007: ErrStopPriceGreaterThanMaxPrice,
	-4008: ErrTickSizeLessThanZero,
	-4009: ErrMaxPriceLessThanMinPrice,
	-4010: ErrMaxQtyLessThanMinQty,
	-4011: ErrStepSizeLessThanZero,
	-4012: ErrMaxNumOrdersLessThanZero,
	-4013: ErrPriceLessThanMinPrice,
	-4014: ErrPriceNotIncreasedByTickSize,
	-4015: ErrInvalidClOrdIdLen,
	-4016: ErrPriceHigherThanMultiplierUp,
	-4017: ErrMultiplierUpLessThanZero,
	-4018: ErrMultiplierDownLessThanZero,
	-4019: ErrCompositeScaleOverflow,
	-4020: ErrTargetStrategyInvalid,
	-4021: ErrInvalidDepthLimit,
	-4022: ErrWrongMarketStatus,
	-4023: ErrQtyNotIncreasedByStepSize,
	-4024: ErrPriceLowerThanMultiplierDown,
	-4025: ErrMultiplierDecimalLessThanZero,
	-4026: ErrCommissionInvalid,
	-4027: ErrInvalidAccountType,
	-4028: ErrInvalidLeverage,
	-4029: ErrInvalidTickSizePrecision,
	-4030: ErrInvalidStepSizePrecision,
	-4031: ErrInvalidWorkingType,
	-4032: ErrExceedMaxCancelOrderSize,
	-4033: ErrInsuranceAccountNotFound,
	-4044: ErrInvalidBalanceType,
	-4045: ErrMaxStopOrderExceeded,
	-4046: ErrNoNeedToChangeMarginType,
	-4047: ErrThereExistsOpenOrders,
	-4048: ErrThereExistsQuantity,
	-4049: ErrAddIsolatedMarginReject,
	-4050: ErrCrossBalanceInsufficient,
	-4051: ErrIsolatedBalanceInsufficient,
	-4052: ErrNoNeedToChangeAutoAddMargin,
	-4053: ErrAutoAddCrossedMarginReject,
	-4054: ErrAddIsolatedMarginNoPositionReject,
	-4055: ErrAmountMustBePositive,
	-4056: ErrInvalidApiKeyType,
	-4057: ErrInvalidRsaPublicKey,
	-4058: ErrMaxPriceTooLarge,
	-4059: ErrNoNeedToChangePositionSide,
	-4060: ErrInvalidPositionSide,
	-4061: ErrPositionSideNotMatch,
	-4062: ErrReduceOnlyConflict,
	-4063: ErrInvalidOptionsRequestType,
	-4064: ErrInvalidOptionsTimeFrame,
	-4065: ErrInvalidOptionsAmount,
	-4066: ErrInvalidOptionsEventType,
	-4067: ErrPositionSideChangeExistsOpenOrders,
	-4068: ErrPositionSideChangeExistsQuantity,
	-4069: ErrInvalidOptionsPremiumFee,
	-4070: ErrInvalidClOptionsIdLen,
	-4071: ErrInvalidOptionsDirection,
	-4072: ErrOptionsPremiumNotUpdate,
	-4073: ErrOptionsPremiumInputLessThanZero,
	-4074: ErrOptionsAmountBiggerThanUpper,
	-4075: ErrOptionsPremiumOutputZero,
	-4076: ErrOptionsPremiumTooDiff,
	-4077: ErrOptionsPremiumReachLimit,
	-4078: ErrOptionsCommonError,
	-4079: ErrInvalidOptionsId,
	-4080: ErrOptionsUserNotFound,
	-4081: ErrOptionsNotFound,
	-4082: ErrInvalidBatchPlaceOrderSize,
	-4083: ErrPlaceBatchOrdersFail,
	-4084: ErrUpcomingMethod,
	-4085: ErrInvalidNotionalLimitCoef,
	-4086: ErrInvalidPriceSpreadThreshold,
	-4087: ErrReduceOnlyOrderPermission,
	-4088: ErrNoPlaceOrderPermission,
	-4104: ErrInvalidContractType,
	-4109: ErrInactiveAccount,
	-4114: ErrInvalidClientTranIdLen,
	-4115: ErrDuplicatedClientTranId,
	-4116: ErrDuplicatedClientOrderId,
	-4117: ErrStopOrderTriggering,
	-4118: ErrReduceOnlyMarginCheckFailed,
	-4120: ErrStopOrderSwitchAlgo,
	-4131: ErrMarketOrderReject,
	-4135: ErrInvalidActivationPrice,
	-4137: ErrQuantityExistsWithClosePosition,
	-4138: ErrReduceOnlyMustBeTrue,
	-4139: ErrOrderTypeCannotBeMkt,
	-4140: ErrInvalidOpeningPositionStatus,
	-4141: ErrSymbolAlreadyClosed,
	-4142: ErrStrategyInvalidTriggerPrice,
	-4144: ErrInvalidPair,
	-4161: ErrIsolatedLeverageRejectWithPosition,
	-4164: ErrMinNotional,
	-4165: ErrInvalidTimeInterval,
	-4167: ErrIsolatedRejectWithJointMargin,
	-4168: ErrJointMarginRejectWithIsolated,
	-4169: ErrJointMarginRejectWithMB,
	-4170: ErrJointMarginRejectWithOpenOrder,
	-4171: ErrNoNeedToChangeJointMargin,
	-4172: ErrJointMarginRejectWithNegativeBalance,
	-4183: ErrPriceHigherThanStopMultiplierUp,
	-4184: ErrPriceLowerThanStopMultiplierDown,
	-4192: ErrCoolingOffPeriod,
	-4202: ErrAdjustLeverageKycFailed,
	-4203: ErrAdjustLeverageOneMonthFailed,
	-4205: ErrAdjustLeverageXDaysFailed,
	-4206: ErrAdjustLeverageKycLimit,
	-4208: ErrAdjustLeverageAccountSymbolFailed,
	-4209: ErrAdjustLeverageSymbolFailed,
	-4210: ErrStopPriceHigherThanPriceMultiplierLimit,
	-4211: ErrStopPriceLowerThanPriceMultiplierLimit,
	-4400: ErrTradingQuantitativeRule,
	-4401: ErrLargePositionSymRule,
	-4402: ErrComplianceBlackSymbolRestriction,
	-4403: ErrAdjustLeverageComplianceFailed,

	// 50xx - Order Execution issues
	-5021: ErrFokOrderReject,
	-5022: ErrGtxOrderReject,
	-5024: ErrMoveOrderNotAllowedSymbolReason,
	-5025: ErrLimitOrderOnly,
	-5026: ErrExceedMaximumModifyOrderLimit,
	-5027: ErrSameOrder,
	-5028: ErrMeRecvwindowReject,
	-5029: ErrModificationMinNotional,
	-5037: ErrInvalidPriceMatch,
	-5038: ErrUnsupportedOrderTypePriceMatch,
	-5039: ErrInvalidSelfTradePreventionMode,
	-5040: ErrFutureGoodTillDate,
	-5041: ErrBboOrderReject,
	-5043: ErrExistingPendingModification,
}

// LookupError returns the error for a Binance error code, or nil if unknown.
func LookupError(code int) error {
	return binanceErrorCodes[code]
}
