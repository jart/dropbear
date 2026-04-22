package sip

import (
	"bufio"
	"bytes"
	"dropbear/decimal"
	"dropbear/symbol"
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed test.jsonl
var sipTestData []byte

func TestSIPParserRoundtrip(t *testing.T) {
	scanner := bufio.NewScanner(bytes.NewReader(sipTestData))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	trades, quotes, statuses, lulds := 0, 0, 0, 0
	tradeErrs, quoteErrs, statusErrs, luldErrs := 0, 0, 0, 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msgType := GetMessageType(line)

		switch msgType {
		case 't':
			trades++
			if err := verifyTradeRoundtrip(t, line, lineNum); err != nil {
				tradeErrs++
				t.Errorf("line %d: trade: %v\ndata: %s", lineNum, err, line)
				if tradeErrs > 10 {
					t.Fatal("too many trade errors")
				}
			}
		case 'q':
			quotes++
			if err := verifyQuoteRoundtrip(t, line, lineNum); err != nil {
				quoteErrs++
				t.Errorf("line %d: quote: %v\ndata: %s", lineNum, err, line)
				if quoteErrs > 10 {
					t.Fatal("too many quote errors")
				}
			}
		case 's':
			statuses++
			if err := verifyStatusRoundtrip(t, line, lineNum); err != nil {
				statusErrs++
				t.Errorf("line %d: status: %v\ndata: %s", lineNum, err, line)
				if statusErrs > 10 {
					t.Fatal("too many status errors")
				}
			}
		case 'l':
			lulds++
			if err := verifyLULDRoundtrip(t, line, lineNum); err != nil {
				luldErrs++
				t.Errorf("line %d: luld: %v\ndata: %s", lineNum, err, line)
				if luldErrs > 10 {
					t.Fatal("too many luld errors")
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	t.Logf("verified %d messages (trades:%d quotes:%d statuses:%d lulds:%d)",
		lineNum, trades, quotes, statuses, lulds)

	if trades == 0 {
		t.Error("no trades found in test data")
	}
	if quotes == 0 {
		t.Error("no quotes found in test data")
	}
}

func verifyTradeRoundtrip(t *testing.T, data []byte, lineNum int) error {
	var fast Trade
	if _, err := fast.Parse(data); err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original Trade
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}

	if fast.Type != original.Type {
		t.Errorf("line %d: Type mismatch: fast=%v original=%v", lineNum, fast.Type, original.Type)
	}
	if fast.Symbol != original.Symbol {
		t.Errorf("line %d: Symbol mismatch: fast=%q original=%q", lineNum, fast.Symbol, original.Symbol)
	}
	if fast.TradeID != original.TradeID {
		t.Errorf("line %d: TradeID mismatch: fast=%d original=%d", lineNum, fast.TradeID, original.TradeID)
	}
	if fast.Exchange != original.Exchange {
		t.Errorf("line %d: Exchange mismatch: fast=%v original=%v", lineNum, fast.Exchange, original.Exchange)
	}
	if fast.Price != original.Price {
		t.Errorf("line %d: Price mismatch: fast=%v original=%v", lineNum, fast.Price, original.Price)
	}
	if fast.Size != original.Size {
		t.Errorf("line %d: Size mismatch: fast=%d original=%d", lineNum, fast.Size, original.Size)
	}
	if fast.Conditions != original.Conditions {
		t.Errorf("line %d: Conditions mismatch: fast=%v original=%v", lineNum, fast.Conditions, original.Conditions)
	}
	if fast.Timestamp != original.Timestamp {
		t.Errorf("line %d: Timestamp mismatch: fast=%v original=%v", lineNum, fast.Timestamp, original.Timestamp)
	}
	if fast.Tape != original.Tape {
		t.Errorf("line %d: Tape mismatch: fast=%v original=%v", lineNum, fast.Tape, original.Tape)
	}

	var roundtrip Trade
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyQuoteRoundtrip(t *testing.T, data []byte, lineNum int) error {
	var fast Quote
	if _, err := fast.Parse(data); err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original Quote
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}

	if fast.Type != original.Type {
		t.Errorf("line %d: Type mismatch: fast=%v original=%v", lineNum, fast.Type, original.Type)
	}
	if fast.Symbol != original.Symbol {
		t.Errorf("line %d: Symbol mismatch: fast=%q original=%q", lineNum, fast.Symbol, original.Symbol)
	}
	if fast.AskExchange != original.AskExchange {
		t.Errorf("line %d: AskExchange mismatch: fast=%v original=%v", lineNum, fast.AskExchange, original.AskExchange)
	}
	if fast.AskPrice != original.AskPrice {
		t.Errorf("line %d: AskPrice mismatch: fast=%v original=%v", lineNum, fast.AskPrice, original.AskPrice)
	}
	if fast.AskSize != original.AskSize {
		t.Errorf("line %d: AskSize mismatch: fast=%d original=%d", lineNum, fast.AskSize, original.AskSize)
	}
	if fast.BidExchange != original.BidExchange {
		t.Errorf("line %d: BidExchange mismatch: fast=%v original=%v", lineNum, fast.BidExchange, original.BidExchange)
	}
	if fast.BidPrice != original.BidPrice {
		t.Errorf("line %d: BidPrice mismatch: fast=%v original=%v", lineNum, fast.BidPrice, original.BidPrice)
	}
	if fast.BidSize != original.BidSize {
		t.Errorf("line %d: BidSize mismatch: fast=%d original=%d", lineNum, fast.BidSize, original.BidSize)
	}
	if fast.Conditions != original.Conditions {
		t.Errorf("line %d: Conditions mismatch: fast=%v original=%v", lineNum, fast.Conditions, original.Conditions)
	}
	if fast.Timestamp != original.Timestamp {
		t.Errorf("line %d: Timestamp mismatch: fast=%v original=%v", lineNum, fast.Timestamp, original.Timestamp)
	}
	if fast.Tape != original.Tape {
		t.Errorf("line %d: Tape mismatch: fast=%v original=%v", lineNum, fast.Tape, original.Tape)
	}

	var roundtrip Quote
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyStatusRoundtrip(t *testing.T, data []byte, lineNum int) error {
	var fast Status
	if _, err := fast.Parse(data); err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original Status
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}

	if fast.Type != original.Type {
		t.Errorf("line %d: Type mismatch: fast=%v original=%v", lineNum, fast.Type, original.Type)
	}
	if fast.Symbol != original.Symbol {
		t.Errorf("line %d: Symbol mismatch: fast=%q original=%q", lineNum, fast.Symbol, original.Symbol)
	}
	if fast.Code != original.Code {
		t.Errorf("line %d: Code mismatch: fast=%v original=%v", lineNum, fast.Code, original.Code)
	}
	if fast.Message != original.Message {
		t.Errorf("line %d: Message mismatch: fast=%q original=%q", lineNum, fast.Message, original.Message)
	}
	if fast.Reason != original.Reason {
		t.Errorf("line %d: Reason mismatch: fast=%v original=%v", lineNum, fast.Reason, original.Reason)
	}
	if fast.Timestamp != original.Timestamp {
		t.Errorf("line %d: Timestamp mismatch: fast=%v original=%v", lineNum, fast.Timestamp, original.Timestamp)
	}

	var roundtrip Status
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyLULDRoundtrip(t *testing.T, data []byte, lineNum int) error {
	var fast LULD
	if _, err := fast.Parse(data); err != nil {
		return err
	}

	var original LULD
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}

	if fast.Type != original.Type {
		t.Errorf("line %d: Type mismatch: fast=%v original=%v", lineNum, fast.Type, original.Type)
	}
	if fast.Symbol != original.Symbol {
		t.Errorf("line %d: Symbol mismatch: fast=%q original=%q", lineNum, fast.Symbol, original.Symbol)
	}
	if fast.UpperLimit != original.UpperLimit {
		t.Errorf("line %d: UpperLimit mismatch: fast=%v original=%v", lineNum, fast.UpperLimit, original.UpperLimit)
	}
	if fast.LowerLimit != original.LowerLimit {
		t.Errorf("line %d: LowerLimit mismatch: fast=%v original=%v", lineNum, fast.LowerLimit, original.LowerLimit)
	}
	if fast.Indicator != original.Indicator {
		t.Errorf("line %d: Indicator mismatch: fast=%v original=%v", lineNum, fast.Indicator, original.Indicator)
	}
	if fast.Timestamp != original.Timestamp {
		t.Errorf("line %d: Timestamp mismatch: fast=%v original=%v", lineNum, fast.Timestamp, original.Timestamp)
	}
	if fast.Tape != original.Tape {
		t.Errorf("line %d: Tape mismatch: fast=%v original=%v", lineNum, fast.Tape, original.Tape)
	}

	return nil
}

func TestSIPParserTrade(t *testing.T) {
	line := []byte(`{"T":"t","S":"TSLA","i":493818,"x":"D","p":390.4499,"s":25,"c":["@","I"],"z":"C","t":"2026-04-22T16:28:55.110808386Z"}`)
	var trade Trade
	if _, err := trade.Parse(line); err != nil {
		t.Fatalf("failed to parse trade: %v", err)
	}
	if trade.Type != MessageTypeTrade {
		t.Errorf("expected Type %v, got %v", MessageTypeTrade, trade.Type)
	}
	if trade.Symbol != symbol.MustParse("TSLA") {
		t.Errorf("expected Symbol 'TSLA', got '%s'", trade.Symbol)
	}
	if trade.TradeID != 493818 {
		t.Errorf("expected TradeID 493818, got %d", trade.TradeID)
	}
	if trade.Exchange != ExchangeADF {
		t.Errorf("expected Exchange 'D', got '%c'", trade.Exchange)
	}
	if trade.Price.Cmp(decimal.Parse("390.4499")) != 0 {
		t.Errorf("expected Price 390.4499, got %v", trade.Price)
	}
	if trade.Size != 25 {
		t.Errorf("expected Size 25, got %d", trade.Size)
	}
	if trade.Conditions != (TradeCondRegularSale | TradeCondOddLot) {
		t.Errorf("expected Conditions %v, got %v", TradeCondRegularSale|TradeCondOddLot, trade.Conditions)
	}
	if trade.Timestamp.RFC3339() != "2026-04-22T16:28:55.110808386Z" {
		t.Errorf("expected Timestamp '2026-04-22T16:28:55.110808386Z', got '%s'", trade.Timestamp.RFC3339())
	}
	if trade.Tape != 'C' {
		t.Errorf("expected Tape 'C', got '%c'", trade.Tape)
	}
}
