package ds

import (
	"dropbear/decimal"
	"testing"
)

func TestBestBidAsk_EmptyBook(t *testing.T) {
	b := NewBook()
	bid, ask := b.BestBidAsk()
	if bid != decimal.Zero || ask != decimal.Zero {
		t.Errorf("expected zero for empty book, got bid %s ask %s", bid, ask)
	}
}

func TestPickAsk_EmptyBook(t *testing.T) {
	b := NewBook()
	result := b.PickAsk(decimal.Parse("50"))
	if result != decimal.Zero {
		t.Errorf("expected zero for empty book, got %s", result)
	}
}

func TestPickBid_EmptyBook(t *testing.T) {
	b := NewBook()
	result := b.PickBid(decimal.Parse("100"))
	if result != decimal.Zero {
		t.Errorf("expected zero for empty book, got %s", result)
	}
}

func TestPickAsk_BasicScenario(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("100"))
	b.UpdateAsk(decimal.Parse("10.02"), decimal.Parse("50"))
	b.UpdateAsk(decimal.Parse("10.03"), decimal.Parse("50"))
	b.Lock.Unlock()
	result := b.PickAsk(decimal.Parse("30"))
	expected := decimal.Parse("10.02")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestPickAsk_SkipSmallLevels(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("100"))
	b.UpdateAsk(decimal.Parse("10.02"), decimal.Parse("10"))
	b.UpdateAsk(decimal.Parse("10.03"), decimal.Parse("10"))
	b.UpdateAsk(decimal.Parse("10.04"), decimal.Parse("100"))
	b.Lock.Unlock()
	// Queue position 50, so we need cumulative competition > 50
	// 10.02: 10 (total: 10)
	// 10.03: 10 (total: 20)
	// 10.04: 100 (total: 120 > 50)
	// Level 100 > order 50, so undercut to 10.03
	result := b.PickAsk(decimal.Parse("50"))
	expected := decimal.Parse("10.04")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestPickBid_BasicScenario(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	// Best bid at 10.00, best ask at 10.02
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("50"))
	b.UpdateBid(decimal.Parse("9.99"), decimal.Parse("50"))
	b.UpdateAsk(decimal.Parse("10.02"), decimal.Parse("100"))
	b.Lock.Unlock()
	// Order size 100, queue position 30, increment 0.01
	// Competition at 10.00 is 50, which exceeds queue position 30
	// Level size 50 < order size 100, so we match at 10.00
	result := b.PickBid(decimal.Parse("30"))
	expected := decimal.Parse("10.00")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestPickBid_SkipSmallLevels(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("10"))
	b.UpdateBid(decimal.Parse("9.99"), decimal.Parse("10"))
	b.UpdateBid(decimal.Parse("9.98"), decimal.Parse("100"))
	b.UpdateAsk(decimal.Parse("10.02"), decimal.Parse("100"))
	b.Lock.Unlock()
	result := b.PickBid(decimal.Parse("50"))
	expected := decimal.Parse("9.98")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestPickBid_RespectHighestLimit(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	// Spread is tight: bid at 10.00, ask at 10.01
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("200"))
	b.UpdateAsk(decimal.Parse("10.01"), decimal.Parse("100"))
	b.Lock.Unlock()
	// Order size 50, queue position 30, increment 0.01
	// Highest possible = 10.01 - 0.01 = 10.00
	// Level 200 > order 50, wants to overcut but highest is 10.00
	result := b.PickBid(decimal.Parse("30"))
	expected := decimal.Parse("10.00")
	if result != expected {
		t.Errorf("expected %s (capped at highest), got %s", expected, result)
	}
}

func TestPickAsk_RespectLowestLimit(t *testing.T) {
	b := NewBook()
	b.Lock.Lock()
	// Spread is tight: bid at 10.00, ask at 10.01
	b.UpdateBid(decimal.Parse("10.00"), decimal.Parse("100"))
	b.UpdateAsk(decimal.Parse("10.01"), decimal.Parse("200"))
	b.Lock.Unlock()
	// Order size 50, queue position 30, increment 0.01
	// Lowest possible = 10.00 + 0.01 = 10.01
	// Level 200 > order 50, wants to undercut but lowest is 10.01
	result := b.PickAsk(decimal.Parse("30"))
	expected := decimal.Parse("10.01")
	if result != expected {
		t.Errorf("expected %s (capped at lowest), got %s", expected, result)
	}
}

func BenchmarkPickAsk(b *testing.B) {
	book := NewBook()
	book.Lock.Lock()
	basePrice := decimal.Parse("100.00")
	increment := decimal.Parse("0.01")
	for i := 0; i < 100; i++ {
		bidPrice := basePrice.Sub(increment.MulInt(i + 1))
		askPrice := basePrice.Add(increment.MulInt(i + 1))
		size := decimal.FromInt(50).Add(increment.MulInt(i % 10 * 1000))
		book.UpdateBid(bidPrice, size)
		book.UpdateAsk(askPrice, size)
	}
	book.Lock.Unlock()
	depth := decimal.FromInt(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book.PickAsk(depth)
	}
}

func BenchmarkBestBidAsk(b *testing.B) {
	book := NewBook()
	book.Lock.Lock()
	basePrice := decimal.Parse("100.00")
	increment := decimal.Parse("0.01")
	for i := 0; i < 100; i++ {
		bidPrice := basePrice.Sub(increment.MulInt(i + 1))
		askPrice := basePrice.Add(increment.MulInt(i + 1))
		size := decimal.FromInt(50).Add(increment.MulInt(i % 10 * 1000))
		book.UpdateBid(bidPrice, size)
		book.UpdateAsk(askPrice, size)
	}
	book.Lock.Unlock()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book.BestBidAsk()
	}
}

func TestMarketBuyCostLimited(t *testing.T) {
	book := NewBook()

	// Set up ask side: 1 BTC @ $100, 2 BTC @ $101, 3 BTC @ $102
	book.UpdateAsk(decimal.Parse("100"), decimal.Parse("1"))
	book.UpdateAsk(decimal.Parse("101"), decimal.Parse("2"))
	book.UpdateAsk(decimal.Parse("102"), decimal.Parse("3"))

	tests := []struct {
		name     string
		qty      string
		limit    string
		wantCost string
		wantOk   bool
	}{
		{"buy 0.5 BTC limit $100", "0.5", "100", "50", true},
		{"buy 1 BTC limit $100", "1", "100", "100", true},
		{"buy 1.5 BTC limit $100", "1.5", "100", "", false},     // not enough at $100
		{"buy 1.5 BTC limit $101", "1.5", "101", "150.5", true}, // 1@100 + 0.5@101
		{"buy 3 BTC limit $101", "3", "101", "302", true},       // 1@100 + 2@101
		{"buy 4 BTC limit $101", "4", "101", "", false},         // only 3 BTC available at or below $101
		{"buy 6 BTC limit $102", "6", "102", "608", true},       // 1@100 + 2@101 + 3@102
		{"buy 7 BTC limit $102", "7", "102", "", false},         // only 6 BTC total
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, ok := book.MarketBuyCostLimited(decimal.Parse(tt.qty), decimal.Parse(tt.limit))
			if ok != tt.wantOk {
				t.Errorf("got ok=%v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk && cost.String() != tt.wantCost {
				t.Errorf("got cost=%s, want %s", cost.String(), tt.wantCost)
			}
		})
	}
}

func TestMarketSellProceedsLimited(t *testing.T) {
	book := NewBook()

	// Set up bid side: 3 BTC @ $98, 2 BTC @ $99, 1 BTC @ $100
	book.UpdateBid(decimal.Parse("98"), decimal.Parse("3"))
	book.UpdateBid(decimal.Parse("99"), decimal.Parse("2"))
	book.UpdateBid(decimal.Parse("100"), decimal.Parse("1"))

	tests := []struct {
		name         string
		qty          string
		limit        string
		wantProceeds string
		wantOk       bool
	}{
		{"sell 0.5 BTC limit $100", "0.5", "100", "50", true},
		{"sell 1 BTC limit $100", "1", "100", "100", true},
		{"sell 1.5 BTC limit $100", "1.5", "100", "", false},   // not enough at $100
		{"sell 1.5 BTC limit $99", "1.5", "99", "149.5", true}, // 1@100 + 0.5@99
		{"sell 3 BTC limit $99", "3", "99", "298", true},       // 1@100 + 2@99
		{"sell 4 BTC limit $99", "4", "99", "", false},         // only 3 BTC available at or above $99
		{"sell 6 BTC limit $98", "6", "98", "592", true},       // 1@100 + 2@99 + 3@98
		{"sell 7 BTC limit $98", "7", "98", "", false},         // only 6 BTC total
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proceeds, ok := book.MarketSellProceedsLimited(decimal.Parse(tt.qty), decimal.Parse(tt.limit))
			if ok != tt.wantOk {
				t.Errorf("got ok=%v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk && proceeds.String() != tt.wantProceeds {
				t.Errorf("got proceeds=%s, want %s", proceeds.String(), tt.wantProceeds)
			}
		})
	}
}
