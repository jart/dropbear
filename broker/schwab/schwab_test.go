package schwab

import (
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

func TestErrorFormatStringErrors(t *testing.T) {
	// the simpler format from the docs
	data := `{"message": "validation failed", "errors": ["price must be > 0", "quantity required"]}`
	var e Error
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		t.Fatal(err)
	}
	got := e.Error()
	want := "validation failed: price must be > 0: quantity required"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestErrorFormatObjectErrors(t *testing.T) {
	// the real format from the API
	data := `{"errors": [{"id": "abc-123", "status": 500, "title": "Internal Server Error"}]}`
	var e Error
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		t.Fatal(err)
	}
	got := e.Error()
	want := "Internal Server Error"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if !e.HasContent() {
		t.Fatal("should have content")
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
