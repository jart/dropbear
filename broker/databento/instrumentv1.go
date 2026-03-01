// InstrumentV1 is a definition of an instrument in DBN version 1 (360 bytes).
// Source: ~/vendor/databento-cpp/include/databento/v1.hpp lines 63-154
package databento

import "dropbear/clocky"

const (
	SymbolCstrLenV1 = 22
)

type InstrumentV1 struct {
	Header                  RecordHeader          // 16 bytes
	TSRecv                  clocky.Time           // 8
	MinPriceIncrement       int64                 // 8
	DisplayFactor           int64                 // 8
	Expiration              clocky.Time           // 8
	Activation              clocky.Time           // 8
	HighLimitPrice          int64                 // 8
	LowLimitPrice           int64                 // 8
	MaxPriceVariation       int64                 // 8
	TradingReferencePrice   int64                 // 8
	UnitOfMeasureQty        int64                 // 8
	MinPriceIncrementAmount int64                 // 8
	PriceRatio              int64                 // 8
	InstAttribValue         int32                 // 4
	UnderlyingID            uint32                // 4
	RawInstrumentID         uint32                // 4 (uint32 in v1, not uint64)
	MarketDepthImplied      int32                 // 4
	MarketDepth             int32                 // 4
	MarketSegmentID         uint32                // 4
	MaxTradeVol             uint32                // 4
	MinLotSize              int32                 // 4
	MinLotSizeBlock         int32                 // 4
	MinLotSizeRoundLot      int32                 // 4
	MinTradeVol             uint32                // 4
	reserved2               [4]byte               // 4
	ContractMultiplier      int32                 // 4
	DecayQuantity           int32                 // 4
	OriginalContractSize    int32                 // 4
	reserved3               [4]byte               // 4
	TradingReferenceDate    uint16                // 2
	ApplID                  int16                 // 2
	MaturityYear            uint16                // 2
	DecayStartDate          uint16                // 2
	ChannelID               uint16                // 2
	Currency                [4]byte               // 4
	SettlCurrency           [4]byte               // 4
	SecSubType              [6]byte               // 6
	RawSymbol               [SymbolCstrLenV1]byte // 22
	Group                   [21]byte              // 21
	Exchange                [5]byte               // 5
	Asset                   [AssetCstrLenV2]byte  // 7
	CFI                     [7]byte               // 7
	SecurityType            [7]byte               // 7
	UnitOfMeasure           [31]byte              // 31
	Underlying              [21]byte              // 21
	StrikePriceCurrency     [4]byte               // 4
	InstrumentClass         InstrumentClass       // 1
	reserved4               [2]byte               // 2
	StrikePrice             int64                 // 8
	reserved5               [6]byte               // 6
	MatchAlgorithm          MatchAlgorithm        // 1
	MDSecurityTradingStatus uint8                 // 1
	MainFraction            uint8                 // 1
	PriceDisplayFormat      uint8                 // 1
	SettlPriceType          uint8                 // 1
	SubFraction             uint8                 // 1
	UnderlyingProduct       uint8                 // 1
	SecurityUpdateAction    SecurityUpdateAction  // 1
	MaturityMonth           uint8                 // 1
	MaturityDay             uint8                 // 1
	MaturityWeek            uint8                 // 1
	UserDefinedInstrument   UserDefinedInstrument // 1
	ContractMultiplierUnit  int8                  // 1
	FlowScheduleType        int8                  // 1
	TickRule                uint8                 // 1
	dummy                   [3]byte               // 3
} // total: 360

// ToV3 upgrades a v1 instrument definition to v3 layout.
// Source: ~/vendor/databento-cpp/src/v1.cpp lines 100-186
func (m *InstrumentV1) ToV3() Instrument {
	var inst Instrument
	inst.Header = m.Header
	inst.Header.Length = uint8(520 / 4) // update length to v3 size
	inst.TSRecv = m.TSRecv
	inst.MinPriceIncrement = m.MinPriceIncrement
	inst.DisplayFactor = m.DisplayFactor
	inst.Expiration = m.Expiration
	inst.Activation = m.Activation
	inst.HighLimitPrice = m.HighLimitPrice
	inst.LowLimitPrice = m.LowLimitPrice
	inst.MaxPriceVariation = m.MaxPriceVariation
	inst.UnitOfMeasureQty = m.UnitOfMeasureQty
	inst.MinPriceIncrementAmount = m.MinPriceIncrementAmount
	inst.PriceRatio = m.PriceRatio
	inst.StrikePrice = m.StrikePrice
	inst.RawInstrumentID = uint64(m.RawInstrumentID)
	inst.LegPrice = UndefPrice
	inst.LegDelta = UndefPrice
	inst.InstAttribValue = m.InstAttribValue
	inst.UnderlyingID = m.UnderlyingID
	inst.MarketDepthImplied = m.MarketDepthImplied
	inst.MarketDepth = m.MarketDepth
	inst.MarketSegmentID = m.MarketSegmentID
	inst.MaxTradeVol = m.MaxTradeVol
	inst.MinLotSize = m.MinLotSize
	inst.MinLotSizeBlock = m.MinLotSizeBlock
	inst.MinLotSizeRoundLot = m.MinLotSizeRoundLot
	inst.MinTradeVol = m.MinTradeVol
	inst.ContractMultiplier = m.ContractMultiplier
	inst.DecayQuantity = m.DecayQuantity
	inst.OriginalContractSize = m.OriginalContractSize
	inst.ApplID = m.ApplID
	inst.MaturityYear = m.MaturityYear
	inst.DecayStartDate = m.DecayStartDate
	inst.ChannelID = m.ChannelID
	inst.Currency = m.Currency
	inst.SettlCurrency = m.SettlCurrency
	copy(inst.SecSubType[:], m.SecSubType[:])
	copy(inst.RawSymbol[:], m.RawSymbol[:])
	copy(inst.Group[:], m.Group[:])
	copy(inst.Exchange[:], m.Exchange[:])
	copy(inst.Asset[:], m.Asset[:])
	copy(inst.CFI[:], m.CFI[:])
	copy(inst.SecurityType[:], m.SecurityType[:])
	copy(inst.UnitOfMeasure[:], m.UnitOfMeasure[:])
	copy(inst.Underlying[:], m.Underlying[:])
	copy(inst.StrikePriceCurrency[:], m.StrikePriceCurrency[:])
	inst.InstrumentClass = m.InstrumentClass
	inst.MatchAlgorithm = m.MatchAlgorithm
	inst.MainFraction = m.MainFraction
	inst.PriceDisplayFormat = m.PriceDisplayFormat
	inst.SubFraction = m.SubFraction
	inst.UnderlyingProduct = m.UnderlyingProduct
	inst.SecurityUpdateAction = m.SecurityUpdateAction
	inst.MaturityMonth = m.MaturityMonth
	inst.MaturityDay = m.MaturityDay
	inst.MaturityWeek = m.MaturityWeek
	inst.UserDefinedInstrument = m.UserDefinedInstrument
	inst.ContractMultiplierUnit = m.ContractMultiplierUnit
	inst.FlowScheduleType = m.FlowScheduleType
	inst.TickRule = m.TickRule
	inst.LegSide = SideNone
	return inst
}

func (m *InstrumentV1) GetRawSymbol() string {
	return convertBytesToString(m.RawSymbol[:])
}

func (m *InstrumentV1) GetAsset() string {
	return convertBytesToString(m.Asset[:])
}

func (m *InstrumentV1) GetExchange() string {
	return convertBytesToString(m.Exchange[:])
}

func (m *InstrumentV1) GetCurrency() string {
	return convertBytesToString(m.Currency[:])
}

func (m *InstrumentV1) GetSecurityType() string {
	return convertBytesToString(m.SecurityType[:])
}

func (m *InstrumentV1) GetGroup() string {
	return convertBytesToString(m.Group[:])
}
