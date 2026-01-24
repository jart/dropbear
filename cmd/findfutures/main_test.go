package main

import (
	"testing"
	"time"
)

func TestClassifyAsset(t *testing.T) {
	tests := []struct {
		asset string
		want  string
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
		{"VX", "vol"},
		{"BTC", "crypto"},
		{"ABC", "other"},
	}

	for _, tt := range tests {
		got := classifyAsset(tt.asset)
		if got != tt.want {
			t.Errorf("classifyAsset(%q) = %q, want %q", tt.asset, got, tt.want)
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

func TestAnalyzeCarry(t *testing.T) {
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
			SettlementPrice: 0.006388, // Contango
		},
	}

	opportunities := analyzeCarry(contracts)

	if len(opportunities) == 0 {
		t.Fatal("expected some opportunities")
	}

	opp := opportunities[0]

	// In contango (back > front), shorting is profitable
	if opp.Direction != "short" {
		t.Errorf("expected direction=short for contango, got %s", opp.Direction)
	}

	// Carry should be positive for contango
	if opp.AnnualizedCarry <= 0 {
		t.Errorf("expected positive carry for contango, got %f", opp.AnnualizedCarry)
	}

	// Currency futures should have implied rate calculated
	if opp.AssetType != "currency" {
		t.Errorf("expected currency asset type, got %s", opp.AssetType)
	}

	t.Logf("6J carry: %.2f%% annualized", opp.AnnualizedCarry*100)
	t.Logf("6J implied foreign rate: %.2f%%", opp.ImpliedForeignRat*100)
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
