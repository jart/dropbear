package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

// Instrument represents the definition of a financial instrument.
// This is the v3 (version three) databento memory layout.
type Instrument struct {
	Header                   RecordHeader          // 16 bytes
	TSRecv                   clocky.Time           // capture-server-received timestamp expressed as the number of nanoseconds since the UNIX epoch
	MinPriceIncrement        int64                 // minimum constant tick for the instrument in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	DisplayFactor            int64                 // multiplier to convert the venue’s display price to the conventional price in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	ExpirationWeird          clocky.Time           // [consider parsing expiry date from GetRawSymbol()] last eligible trade time expressed as a number of nanoseconds since the UNIX epoch. May only have date precision depending on the publisher
	Activation               clocky.Time           // time of instrument activation expressed as a number of nanoseconds since the UNIX epoch. May only have date precision depending on the publisher
	HighLimitPrice           int64                 // allowable high limit price for the trading day in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	LowLimitPrice            int64                 // allowable low limit price for the trading day in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	MaxPriceVariation        int64                 // differential value for price banding in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	UnitOfMeasureQty         int64                 // contract size for each instrument, in combination with unit_of_measure, in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	MinPriceIncrementAmount  int64                 // value currently under development by the venue. Converted to units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	PriceRatio               int64                 // value used for price calculation in spread and leg pricing in units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	StrikePrice              int64                 // exercise price if the instrument is an option. Converted to units of 1e-9, i.e. 1/1,000,000,000 or 0.000000001
	RawInstrumentID          uint64                // instrument ID assigned by the publisher. May be the same as instrument_id
	LegPrice                 int64                 //
	LegDelta                 int64                 //
	InstAttribValue          int32                 // bitmap of instrument eligibility attributes
	UnderlyingID             uint32                // instrument_id of the first underlying instrument
	MarketDepthImplied       int32                 // implied book depth on the price level data feed
	MarketDepth              int32                 // (outright) book depth on the price level data feed
	MarketSegmentID          uint32                // market segment of the instrument
	MaxTradeVol              uint32                // maximum trading volume for the instrument
	MinLotSize               int32                 // minimum order entry quantity for the instrument
	MinLotSizeBlock          int32                 // minimum quantity required for a block trade of the instrument
	MinLotSizeRoundLot       int32                 // minimum quantity required for a round lot of the instrument. Multiples of this quantity are also round lots
	MinTradeVol              uint32                // minimum trading volume for the instrument
	ContractMultiplier       int32                 // number of deliverables per instrument, i.e. peak days
	DecayQuantity            int32                 // quantity that a contract will decay daily, after decay_start_date has been reached
	OriginalContractSize     int32                 // fixed contract value assigned to each instrument
	LegInstrumentID          uint32                //
	LegRatioPriceNumerator   int32                 //
	LegRatioPriceDenominator int32                 //
	LegRatioQtyNumerator     int32                 //
	LegRatioQtyDenominator   int32                 //
	LegUnderlyingID          uint32                //
	ApplID                   int16                 // channel ID assigned at the venue
	MaturityYear             uint16                // calendar year reflected in the instrument symbol
	DecayStartDate           uint16                // date at which a contract will begin to decay
	ChannelID                uint16                // channel ID assigned by Databento as an incrementing integer starting at zero
	LegCount                 uint16                //
	LegIndex                 uint16                //
	Currency                 [4]byte               // currency used for price fields
	SettlCurrency            [4]byte               // currency used for settlement, if different from currency
	SecSubType               [6]byte               // strategy type of the spread
	RawSymbol                [SymbolCstrLen]byte   // instrument name (symbol) provided by the publisher, where symbol_cstr_len is specified in the Metadata
	Group                    [21]byte              // security group code of the instrument
	Exchange                 [5]byte               // exchange used to identify the instrument
	Asset                    [AssetCstrLen]byte    // underlying asset code (product code) of the instrument
	CFI                      [7]byte               // ISO standard instrument categorization code
	SecurityType             [7]byte               // type of the instrument, e.g. FUT for future or future spread
	UnitOfMeasure            [31]byte              // unit of measure for the instrument’s original contract size, e.g. USD or LBS
	Underlying               [21]byte              // symbol of the first underlying instrument
	StrikePriceCurrency      [4]byte               // currency used for strike_price
	LegRawSymbol             [SymbolCstrLen]byte   //
	InstrumentClass          InstrumentClass       // classification of the instrument, e.g. bond (B), call (C), etc.
	MatchAlgorithm           MatchAlgorithm        // matching algorithm used for the instrument, typically FIFO. See Matching algorithm
	MainFraction             uint8                 // price denominator of the main fraction
	PriceDisplayFormat       uint8                 // number of digits to the right of the tick mark, to display fractional prices
	SubFraction              uint8                 // price denominator of the sub fraction
	UnderlyingProduct        uint8                 // product complex of the instrument
	SecurityUpdateAction     SecurityUpdateAction  //
	MaturityMonth            uint8                 // calendar month reflected in the instrument symbol
	MaturityDay              uint8                 // calendar day reflected in the instrument symbol, or 0
	MaturityWeek             uint8                 // calendar week reflected in the instrument symbol, or 0
	UserDefinedInstrument    UserDefinedInstrument // Indicates if the instrument is user defined: Yes or No
	ContractMultiplierUnit   int8                  // type of contract_multiplier; either 1 for hours, or 2 for days
	FlowScheduleType         int8                  // schedule for delivering electricity
	TickRule                 uint8                 // tick rule of the spread
	LegInstrumentClass       InstrumentClass       //
	LegSide                  Side                  //
	reserved                 [17]byte              // reserved for future use
}

func (m *Instrument) InstrumentID() uint32 {
	return m.Header.InstrumentID
}

func (m *Instrument) GetTSRecv() clocky.Time {
	return m.TSRecv
}

// GetRawSymbol returns the raw symbol as a string.
func (m *Instrument) GetRawSymbol() string {
	return convertBytesToString(m.RawSymbol[:])
}

// GetAsset returns the asset/product root as a string.
func (m *Instrument) GetAsset() string {
	return convertBytesToString(m.Asset[:])
}

// GetUnderlying returns the underlying symbol as a string.
func (m *Instrument) GetUnderlying() string {
	return convertBytesToString(m.Underlying[:])
}

// GetExchange returns the exchange as a string.
func (m *Instrument) GetExchange() string {
	return convertBytesToString(m.Exchange[:])
}

// GetCurrency returns the price currency as a string.
func (m *Instrument) GetCurrency() string {
	return convertBytesToString(m.Currency[:])
}

// GetSecurityType returns the security type as a string.
func (m *Instrument) GetSecurityType() string {
	return convertBytesToString(m.SecurityType[:])
}

// GetGroup returns the product group as a string.
func (m *Instrument) GetGroup() string {
	return convertBytesToString(m.Group[:])
}

func (m *Instrument) GoString() string {
	var b strings.Builder
	b.WriteString("Instrument{\n")
	appendName(&b, "Header")
	appendField(&b, &m.Header)
	appendName(&b, "TSRecv")
	appendTimestamp(&b, m.TSRecv)
	appendName(&b, "MinPriceIncrement")
	appendPrice(&b, m.MinPriceIncrement)
	appendName(&b, "DisplayFactor")
	appendPrice(&b, m.DisplayFactor)
	appendName(&b, "ExpirationWeird")
	appendTimestamp(&b, m.ExpirationWeird)
	appendName(&b, "Activation")
	appendTimestamp(&b, m.Activation)
	appendName(&b, "HighLimitPrice")
	appendPrice(&b, m.HighLimitPrice)
	appendName(&b, "LowLimitPrice")
	appendPrice(&b, m.LowLimitPrice)
	appendName(&b, "MaxPriceVariation")
	appendPrice(&b, m.MaxPriceVariation)
	appendName(&b, "UnitOfMeasureQty")
	appendPrice(&b, m.UnitOfMeasureQty)
	appendName(&b, "MinPriceIncrementAmount")
	appendPrice(&b, m.MinPriceIncrementAmount)
	appendName(&b, "PriceRatio")
	appendPrice(&b, m.PriceRatio)
	appendName(&b, "StrikePrice")
	appendPrice(&b, m.StrikePrice)
	appendName(&b, "RawInstrumentID")
	appendField(&b, m.RawInstrumentID)
	appendName(&b, "LegPrice")
	appendPrice(&b, m.LegPrice)
	appendName(&b, "LegDelta")
	appendPrice(&b, m.LegDelta)
	appendName(&b, "InstAttribValue")
	appendInt32(&b, m.InstAttribValue)
	appendName(&b, "UnderlyingID")
	appendUint32(&b, m.UnderlyingID)
	appendName(&b, "MarketDepthImplied")
	appendInt32(&b, m.MarketDepthImplied)
	appendName(&b, "MarketDepth")
	appendInt32(&b, m.MarketDepth)
	appendName(&b, "MarketSegmentID")
	appendUint32(&b, m.MarketSegmentID)
	appendName(&b, "MaxTradeVol")
	appendUint32(&b, m.MaxTradeVol)
	appendName(&b, "MinLotSize")
	appendInt32(&b, m.MinLotSize)
	appendName(&b, "MinLotSizeBlock")
	appendInt32(&b, m.MinLotSizeBlock)
	appendName(&b, "MinLotSizeRoundLot")
	appendInt32(&b, m.MinLotSizeRoundLot)
	appendName(&b, "MinTradeVol")
	appendUint32(&b, m.MinTradeVol)
	appendName(&b, "ContractMultiplier")
	appendInt32(&b, m.ContractMultiplier)
	appendName(&b, "DecayQuantity")
	appendInt32(&b, m.DecayQuantity)
	appendName(&b, "OriginalContractSize")
	appendInt32(&b, m.OriginalContractSize)
	appendName(&b, "LegInstrumentID")
	appendUint32(&b, m.LegInstrumentID)
	appendName(&b, "LegRatioPriceNumerator")
	appendInt32(&b, m.LegRatioPriceNumerator)
	appendName(&b, "LegRatioPriceDenominator")
	appendInt32(&b, m.LegRatioPriceDenominator)
	appendName(&b, "LegRatioQtyNumerator")
	appendInt32(&b, m.LegRatioQtyNumerator)
	appendName(&b, "LegRatioQtyDenominator")
	appendInt32(&b, m.LegRatioQtyDenominator)
	appendName(&b, "LegUnderlyingID")
	appendUint32(&b, m.LegUnderlyingID)
	appendName(&b, "ApplID")
	appendField(&b, m.ApplID)
	appendName(&b, "MaturityYear")
	appendField(&b, m.MaturityYear)
	appendName(&b, "DecayStartDate")
	appendField(&b, m.DecayStartDate)
	appendName(&b, "ChannelID")
	appendField(&b, m.ChannelID)
	appendName(&b, "LegCount")
	appendField(&b, m.LegCount)
	appendName(&b, "LegIndex")
	appendField(&b, m.LegIndex)
	appendName(&b, "Currency")
	appendByteString(&b, m.Currency[:])
	appendName(&b, "SettlCurrency")
	appendByteString(&b, m.SettlCurrency[:])
	appendName(&b, "SecSubType")
	appendByteString(&b, m.SecSubType[:])
	appendName(&b, "RawSymbol")
	appendByteString(&b, m.RawSymbol[:])
	appendName(&b, "Group")
	appendByteString(&b, m.Group[:])
	appendName(&b, "Exchange")
	appendByteString(&b, m.Exchange[:])
	appendName(&b, "Asset")
	appendByteString(&b, m.Asset[:])
	appendName(&b, "CFI")
	appendByteString(&b, m.CFI[:])
	appendName(&b, "SecurityType")
	appendByteString(&b, m.SecurityType[:])
	appendName(&b, "UnitOfMeasure")
	appendByteString(&b, m.UnitOfMeasure[:])
	appendName(&b, "Underlying")
	appendByteString(&b, m.Underlying[:])
	appendName(&b, "StrikePriceCurrency")
	appendByteString(&b, m.StrikePriceCurrency[:])
	appendName(&b, "LegRawSymbol")
	appendByteString(&b, m.LegRawSymbol[:])
	appendName(&b, "InstrumentClass")
	appendField(&b, m.InstrumentClass)
	appendName(&b, "MatchAlgorithm")
	appendField(&b, m.MatchAlgorithm)
	appendName(&b, "MainFraction")
	appendField(&b, m.MainFraction)
	appendName(&b, "PriceDisplayFormat")
	appendField(&b, m.PriceDisplayFormat)
	appendName(&b, "SubFraction")
	appendField(&b, m.SubFraction)
	appendName(&b, "UnderlyingProduct")
	appendField(&b, m.UnderlyingProduct)
	appendName(&b, "SecurityUpdateAction")
	appendField(&b, m.SecurityUpdateAction)
	appendName(&b, "MaturityMonth")
	appendField(&b, m.MaturityMonth)
	appendName(&b, "MaturityDay")
	appendField(&b, m.MaturityDay)
	appendName(&b, "MaturityWeek")
	appendField(&b, m.MaturityWeek)
	appendName(&b, "UserDefinedInstrument")
	appendField(&b, m.UserDefinedInstrument)
	appendName(&b, "ContractMultiplierUnit")
	appendField(&b, m.ContractMultiplierUnit)
	appendName(&b, "FlowScheduleType")
	appendField(&b, m.FlowScheduleType)
	appendName(&b, "TickRule")
	appendField(&b, m.TickRule)
	appendName(&b, "LegInstrumentClass")
	appendField(&b, m.LegInstrumentClass)
	appendName(&b, "LegSide")
	appendField(&b, m.LegSide)
	b.WriteString("}")
	return b.String()
}

func (m *Instrument) Encode() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(m)), unsafe.Sizeof(*m))
}
