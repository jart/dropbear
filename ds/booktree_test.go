package ds

import (
	"dropbear/decimal"
	"math/rand"
	"testing"
)

func TestBook_BasicOperations(t *testing.T) {
	b := NewBook()

	// Insert some bids
	b.UpdateBid(decimal.Parse("100"), decimal.Parse("1"))
	b.UpdateBid(decimal.Parse("99"), decimal.Parse("2"))
	b.UpdateBid(decimal.Parse("101"), decimal.Parse("3"))

	// Insert some asks
	b.UpdateAsk(decimal.Parse("102"), decimal.Parse("1"))
	b.UpdateAsk(decimal.Parse("103"), decimal.Parse("2"))
	b.UpdateAsk(decimal.Parse("101.5"), decimal.Parse("3"))

	bid, ask := b.BestBidAsk()
	if bid != decimal.Parse("101") {
		t.Errorf("best bid = %v, want 101", bid)
	}
	if ask != decimal.Parse("101.5") {
		t.Errorf("best ask = %v, want 101.5", ask)
	}

	// Update existing level
	b.UpdateBid(decimal.Parse("101"), decimal.Parse("5"))
	bid, _ = b.BestBidAsk()
	if bid != decimal.Parse("101") {
		t.Errorf("best bid after update = %v, want 101", bid)
	}

	// Remove level
	b.UpdateBid(decimal.Parse("101"), decimal.Zero)
	bid, _ = b.BestBidAsk()
	if bid != decimal.Parse("100") {
		t.Errorf("best bid after remove = %v, want 100", bid)
	}
}

func TestBook_Iteration(t *testing.T) {
	b := NewBook()

	// Insert bids in random order
	prices := []string{"100", "99", "101", "98", "102", "97"}
	for _, p := range prices {
		b.UpdateBid(decimal.Parse(p), decimal.Parse("1"))
	}

	// Should iterate in descending order
	expected := []string{"102", "101", "100", "99", "98", "97"}
	i := 0
	for idx := b.bids.first(); idx != -1; idx = b.bids.next(idx) {
		n := b.arena.get(idx)
		if n.price != decimal.Parse(expected[i]) {
			t.Errorf("bid[%d] = %v, want %s", i, n.price, expected[i])
		}
		i++
	}
	if i != len(expected) {
		t.Errorf("iterated %d bids, want %d", i, len(expected))
	}

	// Insert asks in random order
	b.UpdateAsk(decimal.Parse("103"), decimal.Parse("1"))
	b.UpdateAsk(decimal.Parse("105"), decimal.Parse("1"))
	b.UpdateAsk(decimal.Parse("104"), decimal.Parse("1"))

	// Should iterate in ascending order
	expectedAsks := []string{"103", "104", "105"}
	i = 0
	for idx := b.asks.first(); idx != -1; idx = b.asks.next(idx) {
		n := b.arena.get(idx)
		if n.price != decimal.Parse(expectedAsks[i]) {
			t.Errorf("ask[%d] = %v, want %s", i, n.price, expectedAsks[i])
		}
		i++
	}
}

func TestBook_MarketBuyCostLimited(t *testing.T) {
	b := NewBook()
	b.UpdateAsk(decimal.Parse("100"), decimal.Parse("1"))
	b.UpdateAsk(decimal.Parse("101"), decimal.Parse("2"))
	b.UpdateAsk(decimal.Parse("102"), decimal.Parse("3"))

	cost, ok := b.MarketBuyCostLimited(decimal.Parse("1.5"), decimal.Parse("101"))
	if !ok {
		t.Error("expected ok")
	}
	// 1 @ 100 + 0.5 @ 101 = 100 + 50.5 = 150.5
	if cost != decimal.Parse("150.5") {
		t.Errorf("cost = %v, want 150.5", cost)
	}
}

func TestBook_MarketSellProceedsLimited(t *testing.T) {
	b := NewBook()
	b.UpdateBid(decimal.Parse("100"), decimal.Parse("1"))
	b.UpdateBid(decimal.Parse("99"), decimal.Parse("2"))
	b.UpdateBid(decimal.Parse("98"), decimal.Parse("3"))

	proceeds, ok := b.MarketSellProceedsLimited(decimal.Parse("1.5"), decimal.Parse("99"))
	if !ok {
		t.Error("expected ok")
	}
	// 1 @ 100 + 0.5 @ 99 = 100 + 49.5 = 149.5
	if proceeds != decimal.Parse("149.5") {
		t.Errorf("proceeds = %v, want 149.5", proceeds)
	}
}

func BenchmarkBook_Update(b *testing.B) {
	book := NewBook()
	rng := rand.New(rand.NewSource(42))
	prices := make([]decimal.Decimal, 10000)
	for i := range prices {
		prices[i] = decimal.FromInt(rng.Intn(10000) + 1000)
	}
	for i := 0; b.Loop(); i++ {
		price := prices[i%len(prices)]
		size := decimal.FromInt(rng.Intn(100) + 1)
		if rng.Intn(2) == 0 {
			book.UpdateBid(price, size)
		} else {
			book.UpdateAsk(price, size)
		}
	}
}

func BenchmarkBook_UpdateAndRemove(b *testing.B) {
	book := NewBook()
	rng := rand.New(rand.NewSource(42))
	prices := make([]decimal.Decimal, 10000)
	for i := range prices {
		prices[i] = decimal.FromInt(rng.Intn(10000) + 1000)
	}
	for i := 0; b.Loop(); i++ {
		price := prices[i%len(prices)]
		if rng.Intn(3) == 0 {
			book.UpdateBid(price, decimal.Zero)
			book.UpdateAsk(price, decimal.Zero)
		} else {
			size := decimal.FromInt(rng.Intn(100) + 1)
			if rng.Intn(2) == 0 {
				book.UpdateBid(price, size)
			} else {
				book.UpdateAsk(price, size)
			}
		}
	}
}
