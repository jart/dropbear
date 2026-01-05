package decimal

import (
	"encoding/json"
	"testing"
)

func TestDecimalUnmarshalJSON(t *testing.T) {
	tests := []struct {
		input string
		want  Decimal
	}{
		// JSON string values
		{`"123.45"`, Parse("123.45")},
		{`"0"`, Zero},
		{`"-99.99"`, Parse("-99.99")},
		{`"0.00000001"`, Satoshi},

		// JSON number values
		{`123.45`, Parse("123.45")},
		{`0`, Zero},
		{`-99.99`, Parse("-99.99")},
		{`1e-8`, Satoshi},
		{`1.5e2`, Parse("150")},
	}

	for _, tt := range tests {
		var d Decimal
		if err := json.Unmarshal([]byte(tt.input), &d); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", tt.input, err)
			continue
		}
		if d != tt.want {
			t.Errorf("Unmarshal(%s) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestDecimalMarshalJSON(t *testing.T) {
	tests := []struct {
		input Decimal
		want  string
	}{
		{Parse("123.45"), `"123.45"`},
		{Zero, `"0"`},
		{Parse("-99.99"), `"-99.99"`},
		{Satoshi, `"0.00000001"`},
		{Parse("1000000"), `"1000000"`},
	}

	for _, tt := range tests {
		got, err := json.Marshal(tt.input)
		if err != nil {
			t.Errorf("Marshal(%v) error: %v", tt.input, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("Marshal(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestDecimalJSONRoundTrip(t *testing.T) {
	type Order struct {
		Price    Decimal `json:"price"`
		Quantity Decimal `json:"quantity"`
	}

	original := Order{
		Price:    Parse("12345.67890123"),
		Quantity: Parse("0.00000001"),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Order
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Price != original.Price {
		t.Errorf("Price = %v, want %v", decoded.Price, original.Price)
	}
	if decoded.Quantity != original.Quantity {
		t.Errorf("Quantity = %v, want %v", decoded.Quantity, original.Quantity)
	}
}

func TestDecimalUnmarshalJSONNullAndEmpty(t *testing.T) {
	// null and "" should parse as zero (for Alpaca API compatibility)
	tests := []struct {
		input string
		want  Decimal
	}{
		{`null`, Zero},
		{`""`, Zero},
	}

	for _, tt := range tests {
		var d Decimal
		if err := json.Unmarshal([]byte(tt.input), &d); err != nil {
			t.Errorf("Unmarshal(%s) error: %v", tt.input, err)
			continue
		}
		if d != tt.want {
			t.Errorf("Unmarshal(%s) = %v, want %v", tt.input, d, tt.want)
		}
	}
}

func TestDecimalUnmarshalJSONErrors(t *testing.T) {
	tests := []struct {
		input string
	}{
		{`"`},         // unclosed quote
		{`"abc"`},     // not a number
		{`"123.abc"`}, // partial garbage
	}

	for _, tt := range tests {
		var d Decimal
		if err := json.Unmarshal([]byte(tt.input), &d); err == nil {
			t.Errorf("Unmarshal(%s) should have failed", tt.input)
		}
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	values := []Decimal{
		Zero,
		Satoshi,
		Parse("123.456"),
		Parse("-9999999.12345678"),
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, d := range values {
			d.MarshalJSON()
		}
	}
}
