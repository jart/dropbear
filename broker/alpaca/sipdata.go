package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"encoding/json"
)

// SIPTrade represents a trade execution from the SIP feed.
type SIPTrade struct {
	Type       SIPMessageType  `json:"T"` // SIPMessageTypeTrade
	Symbol     string          `json:"S"` // stock symbol
	TradeID    int64           `json:"i"` // unique trade identifier
	Exchange   SIPExchange     `json:"x"` // exchange code
	Price      decimal.Decimal `json:"p"` // execution price
	Size       int64           `json:"s"` // quantity traded
	Conditions SIPTradeCond    `json:"c"` // trade conditions (bitmask)
	Timestamp  clocky.Time     `json:"t"` // RFC-3339 timestamp
	Tape       SIPTape         `json:"z"` // tape designation (A, B, C)
}

// IsRegularSale returns true if this is a normal trade (no special conditions).
func (t *SIPTrade) IsRegularSale() bool {
	return t.Conditions.IsRegularSale() || t.Conditions == 0
}

// IsOddLot returns true if this is an odd lot trade.
func (t *SIPTrade) IsOddLot() bool {
	return t.Conditions.IsOddLot()
}

// IsExtendedHours returns true if this is an extended hours trade.
func (t *SIPTrade) IsExtendedHours() bool {
	return t.Conditions.IsExtendedHours()
}

// SIPQuote represents a quote update from the SIP feed.
type SIPQuote struct {
	Type        SIPMessageType  `json:"T"`  // SIPMessageTypeQuote
	Symbol      string          `json:"S"`  // stock symbol
	AskExchange SIPExchange     `json:"ax"` // ask exchange code
	AskPrice    decimal.Decimal `json:"ap"` // ask price
	AskSize     int64           `json:"as"` // ask size
	BidExchange SIPExchange     `json:"bx"` // bid exchange code
	BidPrice    decimal.Decimal `json:"bp"` // bid price
	BidSize     int64           `json:"bs"` // bid size
	Conditions  SIPQuoteCond    `json:"c"`  // quote conditions (bitmask)
	Timestamp   clocky.Time     `json:"t"`  // RFC-3339 timestamp
	Tape        SIPTape         `json:"z"`  // tape designation (A, B, C)
}

// Midpoint returns the midpoint between bid and ask prices.
func (q *SIPQuote) Midpoint() decimal.Decimal {
	return q.BidPrice.Add(q.AskPrice).DivInt(2)
}

// Spread returns the bid-ask spread.
func (q *SIPQuote) Spread() decimal.Decimal {
	return q.AskPrice.Sub(q.BidPrice)
}

// IsNonFirm returns true if any condition indicates a non-firm quote.
func (q *SIPQuote) IsNonFirm() bool {
	return q.Conditions.IsNonFirm()
}

// IsSlow returns true if any condition indicates a slow quote.
func (q *SIPQuote) IsSlow() bool {
	return q.Conditions.IsSlow()
}

// SIPBar represents an aggregated bar from the SIP feed.
type SIPBar struct {
	Type      SIPMessageType  `json:"T"`  // SIPMessageTypeBar, SIPMessageTypeDailyBar, or SIPMessageTypeUpdatedBar
	Symbol    string          `json:"S"`  // stock symbol
	Open      decimal.Decimal `json:"o"`  // opening price
	High      decimal.Decimal `json:"h"`  // highest price
	Low       decimal.Decimal `json:"l"`  // lowest price
	Close     decimal.Decimal `json:"c"`  // closing price
	Volume    int64           `json:"v"`  // total volume
	VWAP      decimal.Decimal `json:"vw"` // volume-weighted average price
	NumTrades int64           `json:"n"`  // number of trades
	Timestamp clocky.Time     `json:"t"`  // RFC-3339 timestamp
}

// SIPStatus represents a trading status message.
type SIPStatus struct {
	Type      SIPMessageType `json:"T"`  // SIPMessageTypeStatus
	Symbol    string         `json:"S"`  // stock symbol
	Code      SIPStatusCode  `json:"sc"` // status code
	Message   string         `json:"sm"` // status message
	Reason    SIPReasonCode  `json:"rc"` // reason code
	Timestamp clocky.Time    `json:"t"`  // RFC-3339 timestamp
}

// IsTradingHalt returns true if this is a trading halt status.
func (s *SIPStatus) IsTradingHalt() bool {
	return s.Code.IsTradingHalt()
}

// IsResume returns true if this is a trading resume status.
func (s *SIPStatus) IsResume() bool {
	return s.Code.IsResume()
}

// IsCircuitBreaker returns true if this is a circuit breaker event.
func (s *SIPStatus) IsCircuitBreaker() bool {
	return s.Reason.IsCircuitBreaker()
}

// ParseSIPTrade parses a raw JSON message into a SIPTrade.
func ParseSIPTrade(data []byte) (*SIPTrade, error) {
	var trade SIPTrade
	if err := json.Unmarshal(data, &trade); err != nil {
		return nil, err
	}
	return &trade, nil
}

// ParseSIPQuote parses a raw JSON message into a SIPQuote.
func ParseSIPQuote(data []byte) (*SIPQuote, error) {
	var quote SIPQuote
	if err := json.Unmarshal(data, &quote); err != nil {
		return nil, err
	}
	return &quote, nil
}

// ParseSIPBar parses a raw JSON message into a SIPBar.
func ParseSIPBar(data []byte) (*SIPBar, error) {
	var bar SIPBar
	if err := json.Unmarshal(data, &bar); err != nil {
		return nil, err
	}
	return &bar, nil
}

// ParseSIPStatus parses a raw JSON message into a SIPStatus.
func ParseSIPStatus(data []byte) (*SIPStatus, error) {
	var status SIPStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// GetSIPMessageType extracts the message type from a JSON message without full parsing.
// Returns 0 if the type cannot be determined.
func GetSIPMessageType(data []byte) byte {
	// Look for "T":" pattern
	for i := 0; i < len(data)-4; i++ {
		if data[i] == '"' && data[i+1] == 'T' && data[i+2] == '"' && data[i+3] == ':' && data[i+4] == '"' {
			if i+5 < len(data) {
				return data[i+5]
			}
		}
	}
	return 0
}
