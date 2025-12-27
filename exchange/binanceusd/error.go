package binanceusd

import (
	"fmt"
)

// Error represents a Binance API error.
// https://developers.binance.com/docs/derivatives/usds-margined-futures/error-code
type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Msg)
}

var (

	// 10xx - general server or network issues
	ErrUnknown                     = Error{-1000, "unknown error occurred while processing request"}
	ErrDisconnected                = Error{-1001, "internal error; unable to process your request; please try again"}
	ErrUnauthorized                = Error{-1002, "you are not authorized to execute this request"}
	ErrTooManyRequests             = Error{-1003, "too many requests; please use the websocket for live updates to avoid polling api"}
	ErrDuplicateIP                 = Error{-1004, "ip already on white list"}
	ErrNoSuchIP                    = Error{-1005, "no such ip on white list"}
	ErrUnexpectedResp              = Error{-1006, "an unexpected response was received from the message bus; execution status unknown"}
	ErrTimeout                     = Error{-1007, "timeout waiting for response from backend server; send status unknown; execution status unknown"}
	ErrRequestThrottled            = Error{-1008, "server is currently overloaded with other requests; try again in a few minutes"}
	ErrMsgReceived                 = Error{-1010, "message received"}
	ErrNonWhiteList                = Error{-1011, "ip cannot access route"}
	ErrInvalidMessage              = Error{-1013, "request is rejected by the api"}
	ErrUnknownOrderComposition     = Error{-1014, "unsupported order combination"}
	ErrTooManyOrders               = Error{-1015, "too many new orders"}
	ErrServiceShuttingDown         = Error{-1016, "service no longer available"}
	ErrUnsupportedOperation        = Error{-1020, "operation not supported"}
	ErrInvalidTimestamp            = Error{-1021, "timestamp for request outside recvWindow"}
	ErrInvalidSignature            = Error{-1022, "signature for request not valid"}
	ErrStartTimeGreaterThanEndTime = Error{-1023, "start time greater than end time"}
	ErrNotFound                    = Error{-1099, "not found"}

	// 11xx - request issues
	ErrIllegalChars                   = Error{-1100, "illegal characters found in parameter"}
	ErrTooManyParams                  = Error{-1101, "too many parameters"}
	ErrMandatoryParamEmptyOrMalformed = Error{-1102, "mandatory parameter not sent, empty/null, or malformed"}
	ErrUnknownParam                   = Error{-1103, "unknown parameter sent"}
	ErrUnreadParameters               = Error{-1104, "not all sent parameters read"}
	ErrParamEmpty                     = Error{-1105, "parameter was empty"}
	ErrParamNotRequired               = Error{-1106, "parameter sent when not required"}
	ErrBadAsset                       = Error{-1108, "invalid asset"}
	ErrBadAccount                     = Error{-1109, "invalid account"}
	ErrBadInstrumentType              = Error{-1110, "invalid symbolType"}
	ErrBadPrecisen                    = Error{-1111, "precision is over maximum defined for asset"}
	ErrNoDepth                        = Error{-1112, "no orders on book for symbol"}
	ErrWithdrawNotNegative            = Error{-1113, "withdrawal amount must be negative"}
	ErrTIFNotRequired                 = Error{-1114, "timeInForce sent when not required"}
	ErrInvalidTIF                     = Error{-1115, "invalid timeInForce"}
	ErrInvalidOrderType               = Error{-1116, "invalid orderType"}
	ErrInvalidSide                    = Error{-1117, "invalid side"}
	ErrEmptyNewClOrdID                = Error{-1118, "new client order id was empty"}
	ErrEmptyOrigClOrdID               = Error{-1119, "original client order id was empty"}
	ErrBadInterval                    = Error{-1120, "invalid interval"}
	ErrBadSymbol                      = Error{-1121, "invalid symbol"}
	ErrInvalidSymbolStatus            = Error{-1122, "invalid symbolStatus"}
	ErrInvalidListenKey               = Error{-1125, "listenKey does not exist; please use post /fapi/v1/listenKey to recreate listenKey"}
	ErrAssetNotSupported              = Error{-1126, "asset not supported"}
	ErrMoreThanXXHours                = Error{-1127, "lookup interval too big or too many hours between startTime and endTime"}
	ErrOptionalParamsBadCombo         = Error{-1128, "combination of optional parameters invalid"}
	ErrInvalidParameter               = Error{-1130, "invalid data sent for parameter"}
	ErrInvalidNewOrderRespType        = Error{-1136, "invalid newOrderRespType"}

	// 20xx - processing issues
	ErrNewOrderRejected                = Error{-2010, "new order rejected"}
	ErrCancelRejected                  = Error{-2011, "cancel rejected"}
	ErrCancelAllFail                   = Error{-2012, "batch cancel failure"}
	ErrNoSuchOrder                     = Error{-2013, "no such order"}
	ErrBadAPIKeyFmt                    = Error{-2014, "bad api key format"}
	ErrRejectedMBXKey                  = Error{-2015, "rejected api-key, ip, or permissions for action"}
	ErrNoTradingWindow                 = Error{-2016, "no trading window could be found for the symbol; try ticker/24hrs instead"}
	ErrAPIKeysLocked                   = Error{-2017, "api keys are locked on this account"}
	ErrBalanceNotSufficient            = Error{-2018, "balance not sufficient"}
	ErrMarginNotSufficient             = Error{-2019, "margin not sufficient"}
	ErrUnableToFill                    = Error{-2020, "unable to fill"}
	ErrOrderWouldImmediatelyTrigger    = Error{-2021, "order would immediately trigger"}
	ErrReduceOnlyReject                = Error{-2022, "reduceOnly order is rejected; this indicates the new reduce-only order conflicts with existing open orders; cancel the existing order and resubmit the reduce-only order"}
	ErrUserInLiquidation               = Error{-2023, "user in liquidation mode now"}
	ErrPositionNotSufficient           = Error{-2024, "position not sufficient"}
	ErrMaxOpenOrdersExceeded           = Error{-2025, "max open order limit exceeded"}
	ErrReduceOnlyOrderTypeNotSupported = Error{-2026, "order type not supported when reduceOnly"}
	ErrMaxLeverageRatio                = Error{-2027, "exceeded maximum allowable position at current leverage"}
	ErrMinLeverageRatio                = Error{-2028, "leverage is smaller than permitted: insufficient margin balance"}

	// 40xx - filters and other issues
	ErrInvalidOrderStatus            = Error{-4000, "invalid order status"}
	ErrPriceLessThanZero             = Error{-4001, "price less than zero"}
	ErrPriceGreaterThanMaxPrice      = Error{-4002, "price greater than max price"}
	ErrQtyLessThanZero               = Error{-4003, "quantity less than zero"}
	ErrQtyLessThanMinQty             = Error{-4004, "quantity less than min quantity"}
	ErrQtyGreaterThanMaxQty          = Error{-4005, "quantity greater than max quantity"}
	ErrStopPriceLessThanZero         = Error{-4006, "stop price less than zero"}
	ErrStopPriceGreaterThanMaxPrice  = Error{-4007, "stop price greater than max price"}
	ErrTickSizeLessThanZero          = Error{-4008, "tick size less than zero"}
	ErrMaxPriceLessThanMinPrice      = Error{-4009, "max price less than min price"}
	ErrMaxQtyLessThanMinQty          = Error{-4010, "max qty less than min qty"}
	ErrStepSizeLessThanZero          = Error{-4011, "step size less than zero"}
	ErrMaxNumOrdersLessThanZero      = Error{-4012, "max mum orders less than zero"}
	ErrPriceLessThanMinPrice         = Error{-4013, "price less than min price"}
	ErrPriceNotIncreasedByTickSize   = Error{-4014, "price not increased by tick size"}
	ErrInvalidClOrdIDLen             = Error{-4015, "client order id is not valid; must be no more than 36 characters"}
	ErrPriceHighterThanMultiplierUp  = Error{-4016, "price is higher than mark price multiplier cap"}
	ErrMultiplierUpLessThanZero      = Error{-4017, "multiplier up less than zero"}
	ErrMultiplierDownLessThanZero    = Error{-4018, "multiplier down less than zero"}
	ErrCompositeScaleOverflow        = Error{-4019, "composite scale too large"}
	ErrTargetStrategyInvalid         = Error{-4020, "target strategy invalid"}
	ErrInvalidDepthLimit             = Error{-4021, "invalid depth limit"}
	ErrWrongMarketStatus             = Error{-4022, "market status sent is not valid"}
	ErrQtyNotIncreasedByStepSize     = Error{-4023, "qty not increased by step size"}
	ErrPriceLowerThanMultiplierDown  = Error{-4024, "price is lower than mark price multiplier floor"}
	ErrMultiplierDecimalLessThanZero = Error{-4025, "multiplier decimal less than zero"}
	ErrCommissionInvalid             = Error{-4026, "commission invalid"}
	ErrInvalidAccountType            = Error{-4027, "invalid account type"}
	ErrInvalidLeverage               = Error{-4028, "invalid leverage"}
	ErrTickSizePrecision             = Error{-4029, "tick size precision is invalid"}
	ErrStepSizePrecision             = Error{-4030, "step size precision is invalid"}
	ErrWorkingType                   = Error{-4031, "invalid parameter working type"}

	// 40xx - filters and other issues
	ErrExceedMaxCancelOrderSize                = Error{-4032, "exceed maximum cancel order size"}
	ErrInsuranceAccountNotFound                = Error{-4033, "insurance account not found"}
	ErrInvalidBalanceType                      = Error{-4044, "balance type is invalid"}
	ErrMaxStopOrderExceeded                    = Error{-4045, "reach max stop order limit"}
	ErrNoNeedToChangeMarginType                = Error{-4046, "no need to change margin type"}
	ErrThereExistsOpenOrders                   = Error{-4047, "margin type cannot be changed if there exists open orders"}
	ErrThereExistsQuantity                     = Error{-4048, "margin type cannot be changed if there exists position"}
	ErrAddIsolatedMarginReject                 = Error{-4049, "add margin only support for isolated position"}
	ErrCrossBalanceInsufficient                = Error{-4050, "cross balance insufficient"}
	ErrIsolatedBalanceInsufficient             = Error{-4051, "isolated balance insufficient"}
	ErrNoNeedToChangeAutoAddMargin             = Error{-4052, "no need to change auto add margin"}
	ErrAutoAddCrossedMarginReject              = Error{-4053, "auto add margin only support for isolated position"}
	ErrAddIsolatedMarginNoPositionReject       = Error{-4054, "cannot add position margin: position is 0"}
	ErrAmountMustBePositive                    = Error{-4055, "amount must be positive"}
	ErrInvalidAPIKeyType                       = Error{-4056, "invalid api key type"}
	ErrInvalidRSAPublicKey                     = Error{-4057, "invalid api public key"}
	ErrMaxPriceTooLarge                        = Error{-4058, "maxPrice and priceDecimal too large"}
	ErrNoNeedToChangePositionSide              = Error{-4059, "no need to change position side"}
	ErrInvalidPositionSide                     = Error{-4060, "invalid position side"}
	ErrPositionSideNotMatch                    = Error{-4061, "order's position side does not match user's setting"}
	ErrReduceOnlyConflict                      = Error{-4062, "invalid or improper reduceOnly value"}
	ErrInvalidOptionsRequestType               = Error{-4063, "invalid options request type"}
	ErrInvalidOptionsTimeFrame                 = Error{-4064, "invalid options time frame"}
	ErrInvalidOptionsAmount                    = Error{-4065, "invalid options amount"}
	ErrInvalidOptionsEventType                 = Error{-4066, "invalid options event type"}
	ErrPositionSideChangeExistsOpenOrders      = Error{-4067, "position side cannot be changed if there exists open orders"}
	ErrPositionSideChangeExistsQuantity        = Error{-4068, "position side cannot be changed if there exists position"}
	ErrInvalidOptionsPremiumFee                = Error{-4069, "invalid options premium fee"}
	ErrInvalidClOptionsIDLen                   = Error{-4070, "client options id is not valid"}
	ErrInvalidOptionsDirection                 = Error{-4071, "invalid options direction"}
	ErrOptionsPremiumNotUpdate                 = Error{-4072, "premium fee is not updated, reject order"}
	ErrOptionsPremiumInputLessThanZero         = Error{-4073, "input premium fee is less than 0, reject order"}
	ErrOptionsAmountBiggerThanUpper            = Error{-4074, "order amount is bigger than upper boundary or less than 0, reject order"}
	ErrOptionsPremiumOutputZero                = Error{-4075, "output premium fee is less than 0, reject order"}
	ErrOptionsPremiumTooDiff                   = Error{-4076, "original fee is too much higher than last fee"}
	ErrOptionsPremiumReachLimit                = Error{-4077, "place order amount has reached to limit, reject order"}
	ErrOptionsCommonError                      = Error{-4078, "options internal error"}
	ErrInvalidOptionsID                        = Error{-4079, "invalid options id"}
	ErrOptionsUserNotFound                     = Error{-4080, "user not found"}
	ErrOptionsNotFound                         = Error{-4081, "options not found"}
	ErrInvalidBatchPlaceOrderSize              = Error{-4082, "invalid number of batch place orders"}
	ErrPlaceBatchOrdersFail                    = Error{-4083, "fail to place batch orders"}
	ErrUpcomingMethod                          = Error{-4084, "method is not allowed currently; upcoming soon"}
	ErrInvalidNotionalLimitCoef                = Error{-4085, "invalid notional limit coefficient"}
	ErrInvalidPriceSpreadThreshold             = Error{-4086, "invalid price spread threshold"}
	ErrReduceOnlyOrderPermission               = Error{-4087, "user can only place reduce only order"}
	ErrNoPlaceOrderPermission                  = Error{-4088, "user can not place order currently"}
	ErrInvalidContractType                     = Error{-4104, "invalid contract type"}
	ErrInactiveAccount                         = Error{-4109, "inactive account"}
	ErrInvalidClientTranIDLen                  = Error{-4114, "clientTranId is not valid"}
	ErrDuplicatedClientTranID                  = Error{-4115, "clientTranId is duplicated"}
	ErrDuplicatedClientOrderID                 = Error{-4116, "clientOrderId is duplicated"}
	ErrStopOrderTriggering                     = Error{-4117, "stop order is triggering"}
	ErrReduceOnlyMarginCheckFailed             = Error{-4118, "reduceOnly order failed; please check your existing position and open orders"}
	ErrStopOrderSwitchAlgo                     = Error{-4120, "order type not supported for this endpoint; please use the Algo Order API endpoints instead"}
	ErrMarketOrderReject                       = Error{-4131, "the counterparty's best price does not meet the PERCENT_PRICE filter limit"}
	ErrInvalidActivationPrice                  = Error{-4135, "invalid activation price"}
	ErrQuantityExistsWithClosePosition         = Error{-4137, "quantity must be zero with closePosition equals true"}
	ErrReduceOnlyMustBeTrue                    = Error{-4138, "reduce only must be true with closePosition equals true"}
	ErrOrderTypeCannotBeMkt                    = Error{-4139, "order type can not be market if it's unable to cancel"}
	ErrInvalidOpeningPositionStatus            = Error{-4140, "invalid symbol status for opening position"}
	ErrSymbolAlreadyClosed                     = Error{-4141, "symbol is closed"}
	ErrStrategyInvalidTriggerPrice             = Error{-4142, "take profit or stop order will be triggered immediately"}
	ErrInvalidPair                             = Error{-4144, "invalid pair"}
	ErrIsolatedLeverageRejectWithPosition      = Error{-4161, "leverage reduction is not supported in Isolated Margin Mode with open positions"}
	ErrMinNotional                             = Error{-4164, "order's notional must be no smaller than 5.0 (unless you choose reduce only)"}
	ErrInvalidTimeInterval                     = Error{-4165, "invalid time interval"}
	ErrIsolatedRejectWithJointMargin           = Error{-4167, "unable to adjust to Multi-Assets mode with symbols of USDⓈ-M Futures under isolated-margin mode"}
	ErrJointMarginRejectWithIsolated           = Error{-4168, "unable to adjust to isolated-margin mode under the Multi-Assets mode"}
	ErrJointMarginRejectWithMB                 = Error{-4169, "unable to adjust Multi-Assets Mode with insufficient margin balance in USDⓈ-M Futures"}
	ErrJointMarginRejectWithOpenOrder          = Error{-4170, "unable to adjust Multi-Assets Mode with open orders in USDⓈ-M Futures"}
	ErrNoNeedToChangeJointMargin               = Error{-4171, "adjusted asset mode is currently set and does not need to be adjusted repeatedly"}
	ErrJointMarginRejectWithNegativeBalance    = Error{-4172, "unable to adjust Multi-Assets Mode with a negative wallet balance of margin available asset in USDⓈ-M Futures account"}
	ErrPriceHigherThanStopMultiplierUp         = Error{-4183, "price is higher than stop price multiplier cap"}
	ErrPriceLowerThanStopMultiplierDown        = Error{-4184, "price is lower than stop price multiplier floor"}
	ErrCoolingOffPeriod                        = Error{-4192, "trade forbidden due to Cooling-off Period"}
	ErrAdjustLeverageKYCFailed                 = Error{-4202, "intermediate Personal Verification is required for adjusting leverage over 20x"}
	ErrAdjustLeverageOneMonthFailed            = Error{-4203, "more than 20x leverage is available one month after account registration"}
	ErrAdjustLeverageXDaysFailed               = Error{-4205, "more than 20x leverage is available after Futures account registration"}
	ErrAdjustLeverageKYCLimit                  = Error{-4206, "users in this country has limited adjust leverage"}
	ErrAdjustLeverageAccountSymbolFailed       = Error{-4208, "current symbol leverage cannot exceed 20 when using position limit adjustment service"}
	ErrAdjustLeverageSymbolFailed              = Error{-4209, "the max leverage of Symbol is 20x"}
	ErrStopPriceHigherThanPriceMultiplierLimit = Error{-4210, "stop price is higher than price multiplier cap"}
	ErrStopPriceLowerThanPriceMultiplierLimit  = Error{-4211, "stop price is lower than price multiplier floor"}
	ErrTradingQuantitativeRule                 = Error{-4400, "Futures Trading Quantitative Rules violated, only reduceOnly order is allowed"}
	ErrLargePositionSymRule                    = Error{-4401, "Futures Trading Risk Control Rules of large position holding violated, only reduceOnly order is allowed"}
	ErrComplianceBlackSymbolRestriction        = Error{-4402, "this feature is currently not available in your region"}
	ErrAdjustLeverageComplianceFailed          = Error{-4403, "the leverage can only up to 10x in your region"}

	// 50xx - order execution issues
	ErrFOKOrderReject                  = Error{-5021, "the FOK order has been rejected because it could not be filled immediately"}
	ErrGTXOrderReject                  = Error{-5022, "the Post Only order will be rejected because it could not be executed as maker"}
	ErrMoveOrderNotAllowedSymbolReason = Error{-5024, "symbol is not in trading status; order amendment is not permitted"}
	ErrLimitOrderOnly                  = Error{-5025, "only limit order is supported"}
	ErrExceedMaximumModifyOrderLimit   = Error{-5026, "exceed maximum modify order limit"}
	ErrSameOrder                       = Error{-5027, "no need to modify the order"}
	ErrMERecvWindowReject              = Error{-5028, "timestamp for this request is outside of the ME recvWindow"}
	ErrModificationMinNotional         = Error{-5029, "order's notional must be no smaller than minimum"}
	ErrInvalidPriceMatch               = Error{-5037, "invalid price match"}
	ErrUnsupportedOrderTypePriceMatch  = Error{-5038, "price match only supports order type: LIMIT, STOP AND TAKE_PROFIT"}
	ErrInvalidSelfTradePreventionMode  = Error{-5039, "invalid self trade prevention mode"}
	ErrFutureGoodTillDate              = Error{-5040, "the goodTillDate timestamp must be greater than the current time plus 600 seconds and smaller than 253402300799000"}
	ErrBBOOrderReject                  = Error{-5041, "no depth matches this BBO order"}
	ErrExistingPendingModification     = Error{-5043, "a pending modification already exists for this order"}
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
	case -1004:
		return &ErrDuplicateIP
	case -1005:
		return &ErrNoSuchIP
	case -1006:
		return &ErrUnexpectedResp
	case -1007:
		return &ErrTimeout
	case -1008:
		return &ErrRequestThrottled
	case -1010:
		return &ErrMsgReceived
	case -1011:
		return &ErrNonWhiteList
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
	case -1023:
		return &ErrStartTimeGreaterThanEndTime
	case -1099:
		return &ErrNotFound
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
		return &ErrBadAsset
	case -1109:
		return &ErrBadAccount
	case -1110:
		return &ErrBadInstrumentType
	case -1111:
		return &ErrBadPrecisen
	case -1112:
		return &ErrNoDepth
	case -1113:
		return &ErrWithdrawNotNegative
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
	case -1126:
		return &ErrAssetNotSupported
	case -1127:
		return &ErrMoreThanXXHours
	case -1128:
		return &ErrOptionalParamsBadCombo
	case -1130:
		return &ErrInvalidParameter
	case -1136:
		return &ErrInvalidNewOrderRespType
	case -2010:
		return &ErrNewOrderRejected
	case -2011:
		return &ErrCancelRejected
	case -2012:
		return &ErrCancelAllFail
	case -2013:
		return &ErrNoSuchOrder
	case -2014:
		return &ErrBadAPIKeyFmt
	case -2015:
		return &ErrRejectedMBXKey
	case -2016:
		return &ErrNoTradingWindow
	case -2017:
		return &ErrAPIKeysLocked
	case -2018:
		return &ErrBalanceNotSufficient
	case -2019:
		return &ErrMarginNotSufficient
	case -2020:
		return &ErrUnableToFill
	case -2021:
		return &ErrOrderWouldImmediatelyTrigger
	case -2022:
		return &ErrReduceOnlyReject
	case -2023:
		return &ErrUserInLiquidation
	case -2024:
		return &ErrPositionNotSufficient
	case -2025:
		return &ErrMaxOpenOrdersExceeded
	case -2026:
		return &ErrReduceOnlyOrderTypeNotSupported
	case -2027:
		return &ErrMaxLeverageRatio
	case -2028:
		return &ErrMinLeverageRatio
	case -4000:
		return &ErrInvalidOrderStatus
	case -4001:
		return &ErrPriceLessThanZero
	case -4002:
		return &ErrPriceGreaterThanMaxPrice
	case -4003:
		return &ErrQtyLessThanZero
	case -4004:
		return &ErrQtyLessThanMinQty
	case -4005:
		return &ErrQtyGreaterThanMaxQty
	case -4006:
		return &ErrStopPriceLessThanZero
	case -4007:
		return &ErrStopPriceGreaterThanMaxPrice
	case -4008:
		return &ErrTickSizeLessThanZero
	case -4009:
		return &ErrMaxPriceLessThanMinPrice
	case -4010:
		return &ErrMaxQtyLessThanMinQty
	case -4011:
		return &ErrStepSizeLessThanZero
	case -4012:
		return &ErrMaxNumOrdersLessThanZero
	case -4013:
		return &ErrPriceLessThanMinPrice
	case -4014:
		return &ErrPriceNotIncreasedByTickSize
	case -4015:
		return &ErrInvalidClOrdIDLen
	case -4016:
		return &ErrPriceHighterThanMultiplierUp
	case -4017:
		return &ErrMultiplierUpLessThanZero
	case -4018:
		return &ErrMultiplierDownLessThanZero
	case -4019:
		return &ErrCompositeScaleOverflow
	case -4020:
		return &ErrTargetStrategyInvalid
	case -4021:
		return &ErrInvalidDepthLimit
	case -4022:
		return &ErrWrongMarketStatus
	case -4023:
		return &ErrQtyNotIncreasedByStepSize
	case -4024:
		return &ErrPriceLowerThanMultiplierDown
	case -4025:
		return &ErrMultiplierDecimalLessThanZero
	case -4026:
		return &ErrCommissionInvalid
	case -4027:
		return &ErrInvalidAccountType
	case -4028:
		return &ErrInvalidLeverage
	case -4029:
		return &ErrTickSizePrecision
	case -4030:
		return &ErrStepSizePrecision
	case -4031:
		return &ErrWorkingType
	case -4032:
		return &ErrExceedMaxCancelOrderSize
	case -4033:
		return &ErrInsuranceAccountNotFound
	case -4044:
		return &ErrInvalidBalanceType
	case -4045:
		return &ErrMaxStopOrderExceeded
	case -4046:
		return &ErrNoNeedToChangeMarginType
	case -4047:
		return &ErrThereExistsOpenOrders
	case -4048:
		return &ErrThereExistsQuantity
	case -4049:
		return &ErrAddIsolatedMarginReject
	case -4050:
		return &ErrCrossBalanceInsufficient
	case -4051:
		return &ErrIsolatedBalanceInsufficient
	case -4052:
		return &ErrNoNeedToChangeAutoAddMargin
	case -4053:
		return &ErrAutoAddCrossedMarginReject
	case -4054:
		return &ErrAddIsolatedMarginNoPositionReject
	case -4055:
		return &ErrAmountMustBePositive
	case -4056:
		return &ErrInvalidAPIKeyType
	case -4057:
		return &ErrInvalidRSAPublicKey
	case -4058:
		return &ErrMaxPriceTooLarge
	case -4059:
		return &ErrNoNeedToChangePositionSide
	case -4060:
		return &ErrInvalidPositionSide
	case -4061:
		return &ErrPositionSideNotMatch
	case -4062:
		return &ErrReduceOnlyConflict
	case -4063:
		return &ErrInvalidOptionsRequestType
	case -4064:
		return &ErrInvalidOptionsTimeFrame
	case -4065:
		return &ErrInvalidOptionsAmount
	case -4066:
		return &ErrInvalidOptionsEventType
	case -4067:
		return &ErrPositionSideChangeExistsOpenOrders
	case -4068:
		return &ErrPositionSideChangeExistsQuantity
	case -4069:
		return &ErrInvalidOptionsPremiumFee
	case -4070:
		return &ErrInvalidClOptionsIDLen
	case -4071:
		return &ErrInvalidOptionsDirection
	case -4072:
		return &ErrOptionsPremiumNotUpdate
	case -4073:
		return &ErrOptionsPremiumInputLessThanZero
	case -4074:
		return &ErrOptionsAmountBiggerThanUpper
	case -4075:
		return &ErrOptionsPremiumOutputZero
	case -4076:
		return &ErrOptionsPremiumTooDiff
	case -4077:
		return &ErrOptionsPremiumReachLimit
	case -4078:
		return &ErrOptionsCommonError
	case -4079:
		return &ErrInvalidOptionsID
	case -4080:
		return &ErrOptionsUserNotFound
	case -4081:
		return &ErrOptionsNotFound
	case -4082:
		return &ErrInvalidBatchPlaceOrderSize
	case -4083:
		return &ErrPlaceBatchOrdersFail
	case -4084:
		return &ErrUpcomingMethod
	case -4085:
		return &ErrInvalidNotionalLimitCoef
	case -4086:
		return &ErrInvalidPriceSpreadThreshold
	case -4087:
		return &ErrReduceOnlyOrderPermission
	case -4088:
		return &ErrNoPlaceOrderPermission
	case -4104:
		return &ErrInvalidContractType
	case -4109:
		return &ErrInactiveAccount
	case -4114:
		return &ErrInvalidClientTranIDLen
	case -4115:
		return &ErrDuplicatedClientTranID
	case -4116:
		return &ErrDuplicatedClientOrderID
	case -4117:
		return &ErrStopOrderTriggering
	case -4118:
		return &ErrReduceOnlyMarginCheckFailed
	case -4120:
		return &ErrStopOrderSwitchAlgo
	case -4131:
		return &ErrMarketOrderReject
	case -4135:
		return &ErrInvalidActivationPrice
	case -4137:
		return &ErrQuantityExistsWithClosePosition
	case -4138:
		return &ErrReduceOnlyMustBeTrue
	case -4139:
		return &ErrOrderTypeCannotBeMkt
	case -4140:
		return &ErrInvalidOpeningPositionStatus
	case -4141:
		return &ErrSymbolAlreadyClosed
	case -4142:
		return &ErrStrategyInvalidTriggerPrice
	case -4144:
		return &ErrInvalidPair
	case -4161:
		return &ErrIsolatedLeverageRejectWithPosition
	case -4164:
		return &ErrMinNotional
	case -4165:
		return &ErrInvalidTimeInterval
	case -4167:
		return &ErrIsolatedRejectWithJointMargin
	case -4168:
		return &ErrJointMarginRejectWithIsolated
	case -4169:
		return &ErrJointMarginRejectWithMB
	case -4170:
		return &ErrJointMarginRejectWithOpenOrder
	case -4171:
		return &ErrNoNeedToChangeJointMargin
	case -4172:
		return &ErrJointMarginRejectWithNegativeBalance
	case -4183:
		return &ErrPriceHigherThanStopMultiplierUp
	case -4184:
		return &ErrPriceLowerThanStopMultiplierDown
	case -4192:
		return &ErrCoolingOffPeriod
	case -4202:
		return &ErrAdjustLeverageKYCFailed
	case -4203:
		return &ErrAdjustLeverageOneMonthFailed
	case -4205:
		return &ErrAdjustLeverageXDaysFailed
	case -4206:
		return &ErrAdjustLeverageKYCLimit
	case -4208:
		return &ErrAdjustLeverageAccountSymbolFailed
	case -4209:
		return &ErrAdjustLeverageSymbolFailed
	case -4210:
		return &ErrStopPriceHigherThanPriceMultiplierLimit
	case -4211:
		return &ErrStopPriceLowerThanPriceMultiplierLimit
	case -4400:
		return &ErrTradingQuantitativeRule
	case -4401:
		return &ErrLargePositionSymRule
	case -4402:
		return &ErrComplianceBlackSymbolRestriction
	case -4403:
		return &ErrAdjustLeverageComplianceFailed
	case -5021:
		return &ErrFOKOrderReject
	case -5022:
		return &ErrGTXOrderReject
	case -5024:
		return &ErrMoveOrderNotAllowedSymbolReason
	case -5025:
		return &ErrLimitOrderOnly
	case -5026:
		return &ErrExceedMaximumModifyOrderLimit
	case -5027:
		return &ErrSameOrder
	case -5028:
		return &ErrMERecvWindowReject
	case -5029:
		return &ErrModificationMinNotional
	case -5037:
		return &ErrInvalidPriceMatch
	case -5038:
		return &ErrUnsupportedOrderTypePriceMatch
	case -5039:
		return &ErrInvalidSelfTradePreventionMode
	case -5040:
		return &ErrFutureGoodTillDate
	case -5041:
		return &ErrBBOOrderReject
	case -5043:
		return &ErrExistingPendingModification
	default:
		return nil
	}
}
