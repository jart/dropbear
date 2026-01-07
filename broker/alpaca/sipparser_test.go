package alpaca

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata_sip.jsonl
var sipTestData []byte

func TestSIPParserRoundtrip(t *testing.T) {
	scanner := bufio.NewScanner(bytes.NewReader(sipTestData))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	trades, quotes, bars, statuses := 0, 0, 0, 0
	tradeErrs, quoteErrs, barErrs, statusErrs := 0, 0, 0, 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msgType := GetSIPMessageType(line)

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
		case 'b', 'd', 'u':
			bars++
			if err := verifyBarRoundtrip(t, line, lineNum); err != nil {
				barErrs++
				t.Errorf("line %d: bar: %v\ndata: %s", lineNum, err, line)
				if barErrs > 10 {
					t.Fatal("too many bar errors")
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
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	t.Logf("verified %d messages (trades:%d quotes:%d bars:%d statuses:%d)",
		lineNum, trades, quotes, bars, statuses)

	if trades == 0 {
		t.Error("no trades found in test data")
	}
	if quotes == 0 {
		t.Error("no quotes found in test data")
	}
	if bars == 0 {
		t.Error("no bars found in test data")
	}
}

func verifyTradeRoundtrip(t *testing.T, data []byte, lineNum int) error {
	fast, err := ParseSIPTradeFast(data)
	if err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original SIPTrade
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

	var roundtrip SIPTrade
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyQuoteRoundtrip(t *testing.T, data []byte, lineNum int) error {
	fast, err := ParseSIPQuoteFast(data)
	if err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original SIPQuote
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

	var roundtrip SIPQuote
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyBarRoundtrip(t *testing.T, data []byte, lineNum int) error {
	fast, err := ParseSIPBarFast(data)
	if err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original SIPBar
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}

	if fast.Type != original.Type {
		t.Errorf("line %d: Type mismatch: fast=%v original=%v", lineNum, fast.Type, original.Type)
	}
	if fast.Symbol != original.Symbol {
		t.Errorf("line %d: Symbol mismatch: fast=%q original=%q", lineNum, fast.Symbol, original.Symbol)
	}
	if fast.Open != original.Open {
		t.Errorf("line %d: Open mismatch: fast=%v original=%v", lineNum, fast.Open, original.Open)
	}
	if fast.High != original.High {
		t.Errorf("line %d: High mismatch: fast=%v original=%v", lineNum, fast.High, original.High)
	}
	if fast.Low != original.Low {
		t.Errorf("line %d: Low mismatch: fast=%v original=%v", lineNum, fast.Low, original.Low)
	}
	if fast.Close != original.Close {
		t.Errorf("line %d: Close mismatch: fast=%v original=%v", lineNum, fast.Close, original.Close)
	}
	if fast.Volume != original.Volume {
		t.Errorf("line %d: Volume mismatch: fast=%d original=%d", lineNum, fast.Volume, original.Volume)
	}
	if fast.VWAP != original.VWAP {
		t.Errorf("line %d: VWAP mismatch: fast=%v original=%v", lineNum, fast.VWAP, original.VWAP)
	}
	if fast.NumTrades != original.NumTrades {
		t.Errorf("line %d: NumTrades mismatch: fast=%d original=%d", lineNum, fast.NumTrades, original.NumTrades)
	}
	if fast.Timestamp != original.Timestamp {
		t.Errorf("line %d: Timestamp mismatch: fast=%v original=%v", lineNum, fast.Timestamp, original.Timestamp)
	}

	var roundtrip SIPBar
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}

func verifyStatusRoundtrip(t *testing.T, data []byte, lineNum int) error {
	fast, err := ParseSIPStatusFast(data)
	if err != nil {
		return err
	}

	remarshaled, err := json.Marshal(&fast)
	if err != nil {
		return err
	}

	var original SIPStatus
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

	var roundtrip SIPStatus
	if err := json.Unmarshal(remarshaled, &roundtrip); err != nil {
		return err
	}

	return nil
}
