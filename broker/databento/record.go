package databento

// DBN record types for Databento Binary Encoding.
// These structs are designed for zero-copy mmap access.
//
// Reference: https://github.com/databento/dbn

// RecordHeader is the 16-byte header present in all DBN records.
type RecordHeader struct {
	// Length is the record length in 32-bit words (e.g., 14 = 56 bytes for MBO).
	Length uint8
	// RType identifies the record type (0xa0 = MBO, 0x01 = Mbp1, etc.).
	RType uint8
	// PublisherID identifies the data source.
	PublisherID uint16
	// InstrumentID is the numeric instrument identifier.
	InstrumentID uint32
	// TsEvent is the exchange timestamp in nanoseconds since Unix epoch.
	TsEvent uint64
}

// RType constants for DBN record types.
const (
	RTypeMbo        uint8 = 0xa0 // Market by Order (L3)
	RTypeMbp0       uint8 = 0x00 // Trades only
	RTypeMbp1       uint8 = 0x01 // Top of book (L1)
	RTypeMbp10      uint8 = 0x0a // 10 levels (L2)
	RTypeTbbo       uint8 = 0x05 // Trade + BBO
	RTypeOhlcv1S    uint8 = 0x20 // 1-second bars
	RTypeOhlcv1M    uint8 = 0x21 // 1-minute bars
	RTypeOhlcv1H    uint8 = 0x22 // 1-hour bars
	RTypeOhlcv1D    uint8 = 0x23 // 1-day bars
	RTypeStatus     uint8 = 0x12 // Trading status
	RTypeImbalance  uint8 = 0x14 // Auction imbalance
	RTypeStatistics uint8 = 0x13 // Statistics
	RTypeDefinition uint8 = 0x15 // Instrument definition
	RTypeError      uint8 = 0x16 // Error message
	RTypeSystem     uint8 = 0x17 // System message
	RTypeSymbolMap  uint8 = 0x18 // Symbol mapping
)

// MboMsg is a Market by Order (L3) record.
// Size: 56 bytes.
type MboMsg struct {
	Header    RecordHeader // 16 bytes
	OrderID   uint64       // Unique order identifier
	Price     int64        // Price in fixed-point (divide by 1e9)
	Size      uint32       // Order quantity
	Flags     uint8        // Bit flags
	ChannelID uint8        // Channel identifier
	Action    byte         // 'A'=Add, 'C'=Cancel, 'M'=Modify, 'R'=Clear, 'T'=Trade, 'F'=Fill
	Side      byte         // 'A'=Ask, 'B'=Bid, 'N'=None
	TsRecv    uint64       // Receive timestamp (nanoseconds)
	TsInDelta int32        // Delta from TsRecv to exchange timestamp
	Sequence  uint32       // Sequence number
}

// Action constants for order book updates.
const (
	ActionAdd    byte = 'A' // New order added
	ActionCancel byte = 'C' // Order canceled
	ActionModify byte = 'M' // Order modified
	ActionClear  byte = 'R' // Book cleared
	ActionTrade  byte = 'T' // Trade execution
	ActionFill   byte = 'F' // Order filled
)

// Side constants for order book entries.
const (
	SideAsk  byte = 'A' // Ask/offer side
	SideBid  byte = 'B' // Bid side
	SideNone byte = 'N' // No side (e.g., trade)
)

// Flag bits for MboMsg.Flags.
const (
	FlagLast      uint8 = 1 << 7 // Last message in event
	FlagTob       uint8 = 1 << 6 // Top of book changed
	FlagSnapshot  uint8 = 1 << 5 // Snapshot message
	FlagMbp       uint8 = 1 << 4 // MBP (aggregated) update
	FlagBadTsRecv uint8 = 1 << 3 // Invalid receive timestamp
)

// BidAskPair is a price level with bid and ask.
// Size: 32 bytes.
type BidAskPair struct {
	BidPx int64  // Bid price (fixed-point, divide by 1e9)
	AskPx int64  // Ask price (fixed-point, divide by 1e9)
	BidSz uint32 // Bid size
	AskSz uint32 // Ask size
	BidCt uint32 // Bid order count
	AskCt uint32 // Ask order count
}

// Mbp1Msg is a top-of-book (L1) record.
// Size: 80 bytes.
type Mbp1Msg struct {
	Header    RecordHeader // 16 bytes
	Price     int64        // Trade price or reference price
	Size      uint32       // Trade size
	Action    byte         // Action type
	Side      byte         // Side
	Flags     uint8        // Bit flags
	Depth     uint8        // Book depth (0 for L1)
	TsRecv    uint64       // Receive timestamp
	TsInDelta int32        // Delta to exchange timestamp
	Sequence  uint32       // Sequence number
	Level     BidAskPair   // Top of book (32 bytes)
}

// Mbp10Msg is a 10-level depth (L2) record.
// Size: 368 bytes.
type Mbp10Msg struct {
	Header    RecordHeader   // 16 bytes
	Price     int64          // Trade price or reference price
	Size      uint32         // Trade size
	Action    byte           // Action type
	Side      byte           // Side
	Flags     uint8          // Bit flags
	Depth     uint8          // Book depth
	TsRecv    uint64         // Receive timestamp
	TsInDelta int32          // Delta to exchange timestamp
	Sequence  uint32         // Sequence number
	Levels    [10]BidAskPair // 10 price levels (320 bytes)
}

// TradeMsg is a trade execution record (Mbp0).
// Size: 48 bytes.
type TradeMsg struct {
	Header    RecordHeader // 16 bytes
	Price     int64        // Trade price
	Size      uint32       // Trade size
	Action    byte         // Always 'T' for trade
	Side      byte         // Aggressor side
	Flags     uint8        // Bit flags
	Depth     uint8        // Always 0
	TsRecv    uint64       // Receive timestamp
	TsInDelta int32        // Delta to exchange timestamp
	Sequence  uint32       // Sequence number
}

// OhlcvMsg is an OHLCV bar record.
// Size: 56 bytes.
type OhlcvMsg struct {
	Header RecordHeader // 16 bytes
	Open   int64        // Open price (fixed-point)
	High   int64        // High price
	Low    int64        // Low price
	Close  int64        // Close price
	Volume uint64       // Volume
}

// PriceScale is the fixed-point scale for prices (1e9).
const PriceScale = 1_000_000_000

// InstrumentDefMsg is an instrument definition record.
// Size: 440 bytes (header + 424 bytes payload).
type InstrumentDefMsg struct {
	Header                  RecordHeader // 16 bytes
	TsRecv                  uint64       // Receive timestamp
	MinPriceIncrement       int64        // Tick size (fixed-point, divide by 1e9)
	DisplayFactor           int64        // Multiplier for venue display price
	Expiration              uint64       // Last eligible trade time (nanoseconds since epoch)
	Activation              uint64       // Instrument activation time
	HighLimitPrice          int64        // Daily high limit price
	LowLimitPrice           int64        // Daily low limit price
	MaxPriceVariation       int64        // Max price variation
	TradingReferencePrice   int64        // Settlement price
	UnitOfMeasureQty        int64        // Contract size (fixed-point)
	MinPriceIncrementAmount int64        // Min price increment amount
	PriceRatio              int64        // Price ratio
	StrikePrice             int64        // Option strike price
	InstAttribValue         int32        // Instrument attributes
	UnderlyingID            uint32       // Underlying instrument ID
	RawInstrumentID         uint32       // Publisher's instrument ID
	MarketDepthImplied      int32        // Implied market depth
	MarketDepth             int32        // Market depth
	MarketSegmentID         uint32       // Market segment ID
	MaxTradeVol             uint32       // Max trade volume
	MinLotSize              int32        // Min lot size
	MinLotSizeBlock         int32        // Min lot size for block trades
	MinLotSizeRoundLot      int32        // Min lot size for round lots
	MinTradeVol             uint32       // Min trade volume
	ContractMultiplier      int32        // Contract multiplier (e.g., 125000 for /6J)
	DecayQuantity           int32        // Decay quantity
	OriginalContractSize    int32        // Original contract size
	TradingReferenceDate    uint16       // Trading reference date (days since epoch)
	ApplID                  int16        // Application ID
	MaturityYear            uint16       // Maturity year
	DecayStartDate          uint16       // Decay start date
	ChannelID               uint16       // Channel ID
	Currency                [4]byte      // Price currency (e.g., "USD")
	SettlCurrency           [4]byte      // Settlement currency
	Secsubtype              [6]byte      // Security subtype
	RawSymbol               [72]byte     // Raw symbol (e.g., "6JH5")
	Group                   [21]byte     // Product group
	Exchange                [5]byte      // Exchange (e.g., "CME")
	Asset                   [7]byte      // Asset/product root (e.g., "6J")
	Cfi                     [7]byte      // CFI code
	SecurityType            [7]byte      // Security type
	UnitOfMeasure           [31]byte     // Unit of measure
	Underlying              [21]byte     // Underlying symbol
	StrikePriceCurrency     [4]byte      // Strike price currency
	InstrumentClass         byte         // 'F'=Future, 'O'=Option, 'S'=Spread
	MatchAlgorithm          byte         // Matching algorithm
	MdSecurityTradingStatus uint8        // Trading status
	MainFraction            uint8        // Main fraction
	PriceDisplayFormat      uint8        // Price display format
	SettlPriceType          uint8        // Settlement price type
	SubFraction             uint8        // Sub fraction
	UnderlyingProduct       uint8        // Underlying product
	SecurityUpdateAction    byte         // 'A'=Add, 'D'=Delete, 'M'=Modify
	MaturityMonth           uint8        // Maturity month (1-12)
	MaturityDay             uint8        // Maturity day
	MaturityWeek            uint8        // Maturity week
	UserDefinedInstrument   byte         // 'Y' or 'N'
	ContractMultiplierUnit  int8         // Multiplier unit
	FlowScheduleType        int8         // Flow schedule type
	TickRule                uint8        // Tick rule
	Reserved                [10]byte     // Reserved padding
}

// GetRawSymbol returns the raw symbol as a string.
func (m *InstrumentDefMsg) GetRawSymbol() string {
	return nullTermString(m.RawSymbol[:])
}

// GetAsset returns the asset/product root as a string.
func (m *InstrumentDefMsg) GetAsset() string {
	return nullTermString(m.Asset[:])
}

// GetExchange returns the exchange as a string.
func (m *InstrumentDefMsg) GetExchange() string {
	return nullTermString(m.Exchange[:])
}

// GetCurrency returns the price currency as a string.
func (m *InstrumentDefMsg) GetCurrency() string {
	return nullTermString(m.Currency[:])
}

// GetSecurityType returns the security type as a string.
func (m *InstrumentDefMsg) GetSecurityType() string {
	return nullTermString(m.SecurityType[:])
}

// GetGroup returns the product group as a string.
func (m *InstrumentDefMsg) GetGroup() string {
	return nullTermString(m.Group[:])
}

// StatisticsMsg is a statistics record (open interest, settlement, etc).
// Size: 80 bytes.
type StatisticsMsg struct {
	Header     RecordHeader // 16 bytes
	TsRecv     uint64       // Receive timestamp
	TsRef      uint64       // Reference timestamp
	Price      int64        // Statistic price (fixed-point)
	Quantity   int32        // Quantity (for open interest)
	Sequence   uint32       // Sequence number
	TsInDelta  int32        // Delta to exchange timestamp
	StatType   uint16       // Statistic type (1=OpeningPrice, 5=SettlementPrice, etc)
	ChannelID  uint16       // Channel ID
	UpdateMode uint8        // 1=new, 2=delete
	StatFlags  uint8        // Statistic flags
	Reserved   [2]byte      // Padding
}
