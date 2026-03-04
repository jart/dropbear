package schwab

import (
	"dropbear/decimal"
	"dropbear/netty"
	"encoding/json"
	"testing"
)

func init() {
	netty.SetOffline()
}

func TestParseEnums(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		parse  func(string) (any, error)
		expect string
	}{
		{"Session", "NORMAL", func(s string) (any, error) { return ParseSession(s) }, "NORMAL"},
		{"Session", "PM", func(s string) (any, error) { return ParseSession(s) }, "PM"},
		{"Duration", "DAY", func(s string) (any, error) { return ParseDuration(s) }, "DAY"},
		{"Duration", "GOOD_TILL_CANCEL", func(s string) (any, error) { return ParseDuration(s) }, "GOOD_TILL_CANCEL"},
		{"OrderType", "LIMIT", func(s string) (any, error) { return ParseOrderType(s) }, "LIMIT"},
		{"OrderType", "TRAILING_STOP_LIMIT", func(s string) (any, error) { return ParseOrderType(s) }, "TRAILING_STOP_LIMIT"},
		{"OrderStatus", "WORKING", func(s string) (any, error) { return ParseOrderStatus(s) }, "WORKING"},
		{"OrderStatus", "FILLED", func(s string) (any, error) { return ParseOrderStatus(s) }, "FILLED"},
		{"OrderStrategyType", "SINGLE", func(s string) (any, error) { return ParseOrderStrategyType(s) }, "SINGLE"},
		{"OrderStrategyType", "OCO", func(s string) (any, error) { return ParseOrderStrategyType(s) }, "OCO"},
		{"Instruction", "BUY_TO_OPEN", func(s string) (any, error) { return ParseInstruction(s) }, "BUY_TO_OPEN"},
		{"Instruction", "SELL_TO_CLOSE", func(s string) (any, error) { return ParseInstruction(s) }, "SELL_TO_CLOSE"},
		{"AssetType", "OPTION", func(s string) (any, error) { return ParseAssetType(s) }, "OPTION"},
		{"PositionEffect", "OPENING", func(s string) (any, error) { return ParsePositionEffect(s) }, "OPENING"},
		{"ComplexStrategy", "NONE", func(s string) (any, error) { return ParseComplexStrategy(s) }, "NONE"},
		{"ComplexStrategy", "IRON_CONDOR", func(s string) (any, error) { return ParseComplexStrategy(s) }, "IRON_CONDOR"},
		{"ActivityType", "EXECUTION", func(s string) (any, error) { return ParseActivityType(s) }, "EXECUTION"},
		{"ExecutionType", "FILL", func(s string) (any, error) { return ParseExecutionType(s) }, "FILL"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.input, func(t *testing.T) {
			v, err := tt.parse(tt.input)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.input, err)
			}
			got := v.(interface{ String() string }).String()
			if got != tt.expect {
				t.Fatalf("String() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestEnumJSONRoundTrip(t *testing.T) {
	type wrapper struct {
		Session           Session           `json:"session"`
		Duration          Duration          `json:"duration"`
		OrderType         OrderType         `json:"orderType"`
		Instruction       Instruction       `json:"instruction"`
		OrderStrategyType OrderStrategyType `json:"orderStrategyType"`
	}
	w := wrapper{
		Session:           SessionNormal,
		Duration:          DurationGoodTillCancel,
		OrderType:         OrderTypeLimit,
		Instruction:       InstructionBuyToOpen,
		OrderStrategyType: OrderStrategyTypeSingle,
	}
	data, err := json.Marshal(&w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	expected := `{"session":"NORMAL","duration":"GOOD_TILL_CANCEL","orderType":"LIMIT","instruction":"BUY_TO_OPEN","orderStrategyType":"SINGLE"}`
	if string(data) != expected {
		t.Fatalf("got  %s\nwant %s", data, expected)
	}
	var w2 wrapper
	if err := json.Unmarshal(data, &w2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w2 != w {
		t.Fatalf("round trip mismatch: got %+v, want %+v", w2, w)
	}
}

func TestOrderStatusHelpers(t *testing.T) {
	if !OrderStatusFilled.IsFinal() {
		t.Fatal("FILLED should be final")
	}
	if !OrderStatusCanceled.IsFinal() {
		t.Fatal("CANCELED should be final")
	}
	if OrderStatusWorking.IsFinal() {
		t.Fatal("WORKING should not be final")
	}
	if !OrderStatusWorking.IsOpen() {
		t.Fatal("WORKING should be open")
	}
	if OrderStatusFilled.IsOpen() {
		t.Fatal("FILLED should not be open")
	}
}

func TestNewOptionLimitOrder(t *testing.T) {
	order := NewOptionLimitOrder(
		"AAPL  260620C00200000",
		InstructionBuyToOpen,
		10,
		decimal.Parse("6.45"),
	)
	data, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// verify it round-trips
	var got Order
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.OrderType != OrderTypeLimit {
		t.Fatalf("orderType = %v, want LIMIT", got.OrderType)
	}
	if got.Session != SessionNormal {
		t.Fatalf("session = %v, want NORMAL", got.Session)
	}
	if got.Duration != DurationDay {
		t.Fatalf("duration = %v, want DAY", got.Duration)
	}
	if got.OrderStrategyType != OrderStrategyTypeSingle {
		t.Fatalf("orderStrategyType = %v, want SINGLE", got.OrderStrategyType)
	}
	if got.Price.Cmp(decimal.Parse("6.45")) != 0 {
		t.Fatalf("price = %v, want 6.45", got.Price)
	}
	if len(got.OrderLegCollection) != 1 {
		t.Fatalf("legs = %d, want 1", len(got.OrderLegCollection))
	}
	leg := got.OrderLegCollection[0]
	if leg.Instruction != InstructionBuyToOpen {
		t.Fatalf("instruction = %v, want BUY_TO_OPEN", leg.Instruction)
	}
	if leg.Quantity.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("quantity = %v, want 10", leg.Quantity)
	}
	if leg.Instrument.Symbol != "AAPL  260620C00200000" {
		t.Fatalf("symbol = %q, want AAPL  260620C00200000", leg.Instrument.Symbol)
	}
	if leg.Instrument.Type != AssetTypeOption {
		t.Fatalf("assetType = %v, want OPTION", leg.Instrument.Type)
	}
}

func TestNewOptionLimitOrderJSON(t *testing.T) {
	// verify the JSON matches what Schwab expects
	order := NewOptionLimitOrder(
		"XYZ   240315C00500000",
		InstructionBuyToOpen,
		10,
		decimal.Parse("6.45"),
	)
	data, err := json.MarshalIndent(order, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("option limit order JSON:\n%s", data)
	// parse it back and check key fields are present
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw["orderType"] != "LIMIT" {
		t.Fatalf("orderType = %v", raw["orderType"])
	}
	if raw["session"] != "NORMAL" {
		t.Fatalf("session = %v", raw["session"])
	}
	if raw["orderStrategyType"] != "SINGLE" {
		t.Fatalf("orderStrategyType = %v", raw["orderStrategyType"])
	}
}

func TestErrorFormat(t *testing.T) {
	e := &Error{
		Message: "validation failed",
		Errors:  []string{"price must be > 0", "quantity required"},
	}
	got := e.Error()
	want := "validation failed: price must be > 0; quantity required"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestErrorFormatNoDetails(t *testing.T) {
	e := &Error{Message: "not found"}
	if e.Error() != "not found" {
		t.Fatalf("got %q", e.Error())
	}
}

func TestAuthorizeURL(t *testing.T) {
	// just verify it doesn't panic with no key file loaded
	// (in real usage, GetKey() would be initialized)
}

func BenchmarkEnumMarshal(b *testing.B) {
	for b.Loop() {
		OrderTypeLimit.MarshalJSON()
	}
}

func BenchmarkEnumUnmarshal(b *testing.B) {
	data := []byte(`"LIMIT"`)
	var ot OrderType
	for b.Loop() {
		ot.UnmarshalJSON(data)
	}
}
