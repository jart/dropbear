package main

import (
	"testing"
	"time"
)

func init() {
	if err := loadContractSpecs(); err != nil {
		panic(err)
	}
}

func TestContractSpecsLoaded(t *testing.T) {
	tests := []struct {
		asset    string
		wantType string
	}{
		{"6J", "currency"},
		{"6E", "currency"},
		{"6B", "currency"},
		{"6A", "currency"},
		{"CNH", "currency"},
		{"M6E", "currency"},
		{"ES", "index"},
		{"NQ", "index"},
		{"YM", "index"},
		{"MES", "index"},
		{"CL", "energy"},
		{"NG", "energy"},
		{"GC", "metal"},
		{"SI", "metal"},
		{"ZN", "rate"},
		{"ZB", "rate"},
		{"SR3", "rate"},
		{"ZC", "ag"},
		{"LE", "ag"},
		{"BTC", "crypto"},
	}

	for _, tt := range tests {
		spec := getSpec(tt.asset)
		if spec.Asset == "" {
			t.Errorf("getSpec(%q) returned empty spec", tt.asset)
			continue
		}
		if spec.Type != tt.wantType {
			t.Errorf("getSpec(%q).Type = %q, want %q", tt.asset, spec.Type, tt.wantType)
		}
	}
}

func TestParseSymbol(t *testing.T) {
	tests := []struct {
		symbol    string
		wantAsset string
		wantMonth int
		wantYear  int
	}{
		{"6JH6", "6J", 3, 2026},
		{"ESZ5", "ES", 12, 2025},
		{"NQM7", "NQ", 6, 2027},
		{"RTYH6", "RTY", 3, 2026},
		{"invalid", "", 0, 0},
	}

	for _, tt := range tests {
		asset, expiry := parseSymbol(tt.symbol)
		if asset != tt.wantAsset {
			t.Errorf("parseSymbol(%q) asset = %q, want %q", tt.symbol, asset, tt.wantAsset)
		}
		if tt.wantAsset != "" {
			if expiry.Month() != time.Month(tt.wantMonth) {
				t.Errorf("parseSymbol(%q) month = %d, want %d", tt.symbol, expiry.Month(), tt.wantMonth)
			}
			if expiry.Year() != tt.wantYear {
				t.Errorf("parseSymbol(%q) year = %d, want %d", tt.symbol, expiry.Year(), tt.wantYear)
			}
		}
	}
}

func TestGenerateOutrightTrades(t *testing.T) {
	now := time.Now()
	contracts := []FuturesContract{
		{
			Symbol:          "6JH6",
			Asset:           "6J",
			Expiration:      now.Add(30 * 24 * time.Hour),
			SettlementPrice: 0.006337,
		},
		{
			Symbol:          "6JM6",
			Asset:           "6J",
			Expiration:      now.Add(122 * 24 * time.Hour),
			SettlementPrice: 0.006388, // Contango (back > front)
		},
	}

	trades := generateOutrightTrades(contracts)

	// Should have 2 trades (LONG and SHORT) for the one asset
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades (LONG and SHORT), got %d", len(trades))
	}

	// Find the SHORT trade
	var shortTrade, longTrade Trade
	for _, tr := range trades {
		if tr.Direction == "SHORT" {
			shortTrade = tr
		} else if tr.Direction == "LONG" {
			longTrade = tr
		}
	}

	// In contango (back > front), SHORT earns positive carry
	if shortTrade.Carry <= 0 {
		t.Errorf("expected positive carry for SHORT in contango, got %f", shortTrade.Carry)
	}

	// LONG pays negative carry in contango
	if longTrade.Carry >= 0 {
		t.Errorf("expected negative carry for LONG in contango, got %f", longTrade.Carry)
	}

	if shortTrade.CurveState != "contango" {
		t.Errorf("expected curveState=contango, got %s", shortTrade.CurveState)
	}

	if shortTrade.AssetType != "currency" {
		t.Errorf("expected currency asset type, got %s", shortTrade.AssetType)
	}

	t.Logf("6J SHORT carry: %+.2f%%, LONG carry: %+.2f%%", shortTrade.Carry*100, longTrade.Carry*100)
}

func TestGenerateSpreadTrades(t *testing.T) {
	now := time.Now()
	contracts := []FuturesContract{
		{
			Symbol:          "CLG6",
			Asset:           "CL",
			Expiration:      now.Add(20 * 24 * time.Hour),
			SettlementPrice: 60.53,
		},
		{
			Symbol:          "CLH6",
			Asset:           "CL",
			Expiration:      now.Add(51 * 24 * time.Hour),
			SettlementPrice: 59.59, // Backwardation (front > back)
		},
	}

	trades := generateSpreadTrades(contracts)

	if len(trades) == 0 {
		t.Fatal("expected some spread trades")
	}

	trade := trades[0]

	// Spread should be positive (front - back = 60.53 - 59.59 = 0.94)
	if trade.Price <= 0 {
		t.Errorf("expected positive spread price for backwardation, got %f", trade.Price)
	}

	// Backwardation
	if trade.CurveState != "backwrd" {
		t.Errorf("expected curveState=backwrd, got %s", trade.CurveState)
	}

	if trade.AssetType != "energy" {
		t.Errorf("expected energy asset type, got %s", trade.AssetType)
	}

	// Check that spread margin is less than outright margin
	clSpec := getSpec("CL")
	if trade.Margin >= clSpec.Margin {
		t.Errorf("spread margin %f should be less than outright %f", trade.Margin, clSpec.Margin)
	}

	t.Logf("CL spread: %.2f, margin: $%.0f, carry: %.2f%%", trade.Price, trade.Margin, trade.Carry*100)
}

func TestGenerateSymbols(t *testing.T) {
	symbols := generateSymbols()

	if len(symbols) == 0 {
		t.Fatal("expected some symbols")
	}

	// Check we have expected symbols
	found := make(map[string]bool)
	for _, sym := range symbols {
		found[sym] = true
	}

	// Should have currency futures
	hasYen := false
	for sym := range found {
		if len(sym) >= 2 && sym[:2] == "6J" {
			hasYen = true
			break
		}
	}
	if !hasYen {
		t.Error("expected Japanese Yen futures (6J*)")
	}

	// Should have index futures
	hasES := false
	for sym := range found {
		if len(sym) >= 2 && sym[:2] == "ES" {
			hasES = true
			break
		}
	}
	if !hasES {
		t.Error("expected S&P 500 futures (ES*)")
	}

	t.Logf("Generated %d symbols", len(symbols))
}

func TestCarryCalculation(t *testing.T) {
	// Test case: front = 100, back = 102, days between = 92
	// Carry = (102 - 100) / 100 * (365 / 92) = 0.02 * 3.967 ≈ 7.93%
	front := 100.0
	back := 102.0
	daysBetween := 92

	priceDiff := back - front
	carry := (priceDiff / front) * (365.0 / float64(daysBetween))

	expected := 0.0793 // approximately 7.93%
	tolerance := 0.001

	if carry < expected-tolerance || carry > expected+tolerance {
		t.Errorf("carry = %f, expected approximately %f", carry, expected)
	}

	t.Logf("Carry calculation: %.4f (%.2f%%)", carry, carry*100)
}
