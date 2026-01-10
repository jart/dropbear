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
