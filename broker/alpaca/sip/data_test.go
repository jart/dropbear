package sip

import (
	"dropbear/ds/symbol"
	"testing"
)

// Sample JSON messages for benchmarking
var (
	AAPL      = symbol.MustParse("AAPL")
	tradeJSON = []byte(`{"T":"t","S":"AAPL","i":12345,"x":"Q","p":"150.25","s":100,"c":["@","F"],"t":"2024-01-15T14:30:00.123456789Z","z":"C"}`)
	quoteJSON = []byte(`{"T":"q","S":"AAPL","ax":"Q","ap":"150.26","as":200,"bx":"P","bp":"150.24","bs":300,"c":["R"],"t":"2024-01-15T14:30:00.123456789Z","z":"C"}`)
	barJSON   = []byte(`{"T":"b","S":"AAPL","o":"150.00","h":"151.00","l":"149.50","c":"150.50","v":1000000,"vw":"150.25","n":5000,"t":"2024-01-15T14:30:00Z"}`)
)

func BenchmarkGetMessageType(b *testing.B) {
	for b.Loop() {
		_ = GetMessageType(tradeJSON)
	}
}

func BenchmarkParseTrade(b *testing.B) {
	for b.Loop() {
		_, err := ParseTrade(tradeJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseQuote(b *testing.B) {
	for b.Loop() {
		_, err := ParseQuote(quoteJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseBar(b *testing.B) {
	for b.Loop() {
		_, err := ParseBar(barJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExchange_MarshalJSON(b *testing.B) {
	e := ExchangeNASDAQ
	for b.Loop() {
		_, err := e.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExchange_UnmarshalJSON(b *testing.B) {
	data := []byte(`"Q"`)
	var e Exchange
	for b.Loop() {
		err := e.UnmarshalJSON(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExchange_String(b *testing.B) {
	e := ExchangeNASDAQ
	for b.Loop() {
		_ = e.String()
	}
}

func BenchmarkExchange_GoString(b *testing.B) {
	e := ExchangeNASDAQ
	for b.Loop() {
		_ = e.GoString()
	}
}

func BenchmarkTradeCond_MarshalJSON_Single(b *testing.B) {
	c := TradeCondRegularSale
	for b.Loop() {
		_, err := c.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTradeCond_MarshalJSON_Multi(b *testing.B) {
	c := TradeCondRegularSale | TradeCondISO
	for b.Loop() {
		_, err := c.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTradeCond_UnmarshalJSON(b *testing.B) {
	data := []byte(`["@","F"]`)
	var c TradeCond
	for b.Loop() {
		err := c.UnmarshalJSON(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTradeCond_IsRegularSale(b *testing.B) {
	c := TradeCondRegularSale | TradeCondISO
	for b.Loop() {
		_ = c.IsRegularSale()
	}
}

func BenchmarkQuoteCond_IsSlow(b *testing.B) {
	c := QuoteCondB | QuoteCondA
	for b.Loop() {
		_ = c.IsSlow()
	}
}

func BenchmarkQuoteCond_MarshalJSON_Single(b *testing.B) {
	c := QuoteCondR
	for b.Loop() {
		_, err := c.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuoteCond_MarshalJSON_Multi(b *testing.B) {
	c := QuoteCondR | QuoteCondNonFirm
	for b.Loop() {
		_, err := c.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTape_MarshalJSON(b *testing.B) {
	t := TapeC
	for b.Loop() {
		_, err := t.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTape_IsCTA(b *testing.B) {
	t := TapeA
	for b.Loop() {
		_ = t.IsCTA()
	}
}

func BenchmarkTape_IsUTP(b *testing.B) {
	t := TapeC
	for b.Loop() {
		_ = t.IsUTP()
	}
}

func TestGetMessageType(t *testing.T) {
	tests := []struct {
		data []byte
		want byte
	}{
		{tradeJSON, 't'},
		{quoteJSON, 'q'},
		{barJSON, 'b'},
		{[]byte(`{}`), 0},
		{[]byte(`{"T":"s"}`), 's'},
	}
	for _, tt := range tests {
		got := GetMessageType(tt.data)
		if got != tt.want {
			t.Errorf("GetMessageType(%s) = %c, want %c", tt.data, got, tt.want)
		}
	}
}

func TestTradeCond_UnmarshalJSON(t *testing.T) {
	var c TradeCond
	err := c.UnmarshalJSON([]byte(`["@","F"]`))
	if err != nil {
		t.Fatal(err)
	}
	if c != TradeCondRegularSale|TradeCondISO {
		t.Errorf("got %v, want %v", c, TradeCondRegularSale|TradeCondISO)
	}
}

func TestTradeCond_IsRegularSale(t *testing.T) {
	if !TradeCondRegularSaleCTA.IsRegularSale() {
		t.Error("TradeCondRegularSaleCTA should be regular sale")
	}
	if !TradeCondRegularSale.IsRegularSale() {
		t.Error("TradeCondRegularSale should be regular sale")
	}
	if TradeCondOddLot.IsRegularSale() {
		t.Error("TradeCondOddLot should not be regular sale")
	}
	// Multiple conditions
	c := TradeCondRegularSale | TradeCondISO
	if !c.IsRegularSale() {
		t.Error("@,F should be regular sale")
	}
}

func TestTradeCond_IsExtendedHours(t *testing.T) {
	if !TradeCondExtendedHours.IsExtendedHours() {
		t.Error("TradeCondExtendedHours should be extended hours")
	}
	if !TradeCondExtendedHoursOOS.IsExtendedHours() {
		t.Error("TradeCondExtendedHoursOOS should be extended hours")
	}
	if TradeCondRegularSaleCTA.IsExtendedHours() {
		t.Error("TradeCondRegularSaleCTA should not be extended hours")
	}
}

func TestQuoteCond_IsSlow(t *testing.T) {
	if !QuoteCondB.IsSlow() {
		t.Error("QuoteCondB should be slow")
	}
	if !QuoteCondA.IsSlow() {
		t.Error("QuoteCondA should be slow")
	}
	if QuoteCondNonFirm.IsSlow() {
		t.Error("QuoteCondNonFirm should not be slow")
	}
}

func TestTape_IsCTA(t *testing.T) {
	if !TapeA.IsCTA() {
		t.Error("TapeA should be CTA")
	}
	if !TapeB.IsCTA() {
		t.Error("TapeB should be CTA")
	}
	if TapeC.IsCTA() {
		t.Error("TapeC should not be CTA")
	}
}

func TestTape_IsUTP(t *testing.T) {
	if !TapeC.IsUTP() {
		t.Error("TapeC should be UTP")
	}
	if TapeA.IsUTP() {
		t.Error("TapeA should not be UTP")
	}
}

func TestParseQuote(t *testing.T) {
	quote, err := ParseQuote(quoteJSON)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Symbol != AAPL {
		t.Errorf("Symbol = %s, want AAPL", quote.Symbol)
	}
	if quote.Conditions != QuoteCondR {
		t.Errorf("Conditions = %v, want %v", quote.Conditions, QuoteCondR)
	}
}

func TestParseTrade(t *testing.T) {
	trade, err := ParseTrade(tradeJSON)
	if err != nil {
		t.Fatal(err)
	}
	if trade.Type != MessageTypeTrade {
		t.Errorf("Type = %v, want %v", trade.Type, MessageTypeTrade)
	}
	if trade.Symbol != AAPL {
		t.Errorf("Symbol = %s, want AAPL", trade.Symbol)
	}
	if trade.TradeID != 12345 {
		t.Errorf("TradeID = %d, want 12345", trade.TradeID)
	}
	if trade.Exchange != ExchangeNASDAQ {
		t.Errorf("Exchange = %v, want %v", trade.Exchange, ExchangeNASDAQ)
	}
	if trade.Price.String() != "150.25" {
		t.Errorf("Price = %s, want 150.25", trade.Price)
	}
	if trade.Size != 100 {
		t.Errorf("Size = %d, want 100", trade.Size)
	}
	if trade.Conditions != TradeCondRegularSale|TradeCondISO {
		t.Errorf("Conditions = %v, want %v", trade.Conditions, TradeCondRegularSale|TradeCondISO)
	}
	if trade.Tape != TapeC {
		t.Errorf("Tape = %v, want %v", trade.Tape, TapeC)
	}
}

func TestParseQuote2(t *testing.T) {
	quote, err := ParseQuote(quoteJSON)
	if err != nil {
		t.Fatal(err)
	}
	if quote.Type != MessageTypeQuote {
		t.Errorf("Type = %v, want %v", quote.Type, MessageTypeQuote)
	}
	if quote.Symbol != AAPL {
		t.Errorf("Symbol = %s, want AAPL", quote.Symbol)
	}
	if quote.AskExchange != ExchangeNASDAQ {
		t.Errorf("AskExchange = %v, want %v", quote.AskExchange, ExchangeNASDAQ)
	}
	if quote.AskPrice.String() != "150.26" {
		t.Errorf("AskPrice = %s, want 150.26", quote.AskPrice)
	}
	if quote.AskSize != 200 {
		t.Errorf("AskSize = %d, want 200", quote.AskSize)
	}
	if quote.BidExchange != ExchangeNYSEArca {
		t.Errorf("BidExchange = %v, want %v", quote.BidExchange, ExchangeNYSEArca)
	}
	if quote.BidPrice.String() != "150.24" {
		t.Errorf("BidPrice = %s, want 150.24", quote.BidPrice)
	}
	if quote.BidSize != 300 {
		t.Errorf("BidSize = %d, want 300", quote.BidSize)
	}
	if quote.Conditions != QuoteCondR {
		t.Errorf("Conditions = %v, want %v", quote.Conditions, QuoteCondR)
	}
	if quote.Tape != TapeC {
		t.Errorf("Tape = %v, want %v", quote.Tape, TapeC)
	}
}

func TestParseBar(t *testing.T) {
	bar, err := ParseBar(barJSON)
	if err != nil {
		t.Fatal(err)
	}
	if bar.Type != MessageTypeBar {
		t.Errorf("Type = %v, want %v", bar.Type, MessageTypeBar)
	}
	if bar.Symbol != AAPL {
		t.Errorf("Symbol = %s, want AAPL", bar.Symbol)
	}
	if bar.Open.String() != "150" {
		t.Errorf("Open = %s, want 150", bar.Open)
	}
	if bar.High.String() != "151" {
		t.Errorf("High = %s, want 151", bar.High)
	}
	if bar.Low.String() != "149.5" {
		t.Errorf("Low = %s, want 149.5", bar.Low)
	}
	if bar.Close.String() != "150.5" {
		t.Errorf("Close = %s, want 150.5", bar.Close)
	}
	if bar.Volume != 1000000 {
		t.Errorf("Volume = %d, want 1000000", bar.Volume)
	}
	if bar.VWAP.String() != "150.25" {
		t.Errorf("VWAP = %s, want 150.25", bar.VWAP)
	}
	if bar.NumTrades != 5000 {
		t.Errorf("NumTrades = %d, want 5000", bar.NumTrades)
	}
}

func TestStatusCode_IsTradingHalt(t *testing.T) {
	if !StatusCodeTradingHaltCTA.IsTradingHalt() {
		t.Error("StatusCodeTradingHaltCTA should be trading halt")
	}
	if !StatusCodeTradingHaltUTP.IsTradingHalt() {
		t.Error("StatusCodeTradingHaltUTP should be trading halt")
	}
	if StatusCodeResumeCTA.IsTradingHalt() {
		t.Error("StatusCodeResumeCTA should not be trading halt")
	}
}

func TestStatusCode_IsResume(t *testing.T) {
	if !StatusCodeResumeCTA.IsResume() {
		t.Error("StatusCodeResumeCTA should be resume")
	}
	if !StatusCodeTradingResumptionUTP.IsResume() {
		t.Error("StatusCodeTradingResumptionUTP should be resume")
	}
	if StatusCodeTradingHaltCTA.IsResume() {
		t.Error("StatusCodeTradingHaltCTA should not be resume")
	}
}

func TestReasonCode_IsCircuitBreaker(t *testing.T) {
	if !ReasonCodeCircuitLvl1.IsCircuitBreaker() {
		t.Error("ReasonCodeCircuitLvl1 should be circuit breaker")
	}
	if !ReasonCodeCircuitLvl2.IsCircuitBreaker() {
		t.Error("ReasonCodeCircuitLvl2 should be circuit breaker")
	}
	if !ReasonCodeCircuitLvl3.IsCircuitBreaker() {
		t.Error("ReasonCodeCircuitLvl3 should be circuit breaker")
	}
	if ReasonCodeNewsPending.IsCircuitBreaker() {
		t.Error("ReasonCodeNewsPending should not be circuit breaker")
	}
}

func TestReasonCode_ParseReasonCode_Unknown(t *testing.T) {
	// Known code should succeed
	rc, err := ParseReasonCode([]byte("T1"))
	if err != nil {
		t.Errorf("ParseReasonCode(T1) error: %v", err)
	}
	if rc != ReasonCodeHaltNewsPending {
		t.Errorf("ParseReasonCode(T1) = %v, want %v", rc, ReasonCodeHaltNewsPending)
	}

	// Unknown code should return error
	_, err = ParseReasonCode([]byte("BOGUS"))
	if err != ErrInvalidReasonCode {
		t.Errorf("ParseReasonCode(BOGUS) error = %v, want %v", err, ErrInvalidReasonCode)
	}
}

func TestParseStatus(t *testing.T) {
	statusJSON := []byte(`{"T":"s","S":"AAPL","sc":"H","sm":"Trading Halted","rc":"T1","t":"2024-01-15T14:30:00Z"}`)
	status, err := ParseStatus(statusJSON)
	if err != nil {
		t.Fatal(err)
	}
	if status.Type != MessageTypeStatus {
		t.Errorf("Type = %v, want %v", status.Type, MessageTypeStatus)
	}
	if status.Symbol != AAPL {
		t.Errorf("Symbol = %s, want AAPL", status.Symbol)
	}
	if status.Code != StatusCodeTradingHaltUTP {
		t.Errorf("Code = %v, want %v", status.Code, StatusCodeTradingHaltUTP)
	}
	if status.Message != "Trading Halted" {
		t.Errorf("Message = %s, want 'Trading Halted'", status.Message)
	}
	if status.Reason != ReasonCodeHaltNewsPending {
		t.Errorf("Reason = %v, want %v", status.Reason, ReasonCodeHaltNewsPending)
	}
	if !status.Code.IsTradingHalt() {
		t.Error("should be trading halt")
	}
}

func BenchmarkReasonCode_IsCircuitBreaker(b *testing.B) {
	rc := ReasonCodeCircuitLvl1
	for b.Loop() {
		_ = rc.IsCircuitBreaker()
	}
}

func BenchmarkReasonCode_ParseReasonCode(b *testing.B) {
	data := []byte("T12")
	for b.Loop() {
		_, err := ParseReasonCode(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseStatus(b *testing.B) {
	statusJSON := []byte(`{"T":"s","S":"AAPL","sc":"H","sm":"Trading Halted","rc":"T1","t":"2024-01-15T14:30:00Z"}`)
	for b.Loop() {
		_, err := ParseStatus(statusJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}
