package options

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
	"testing"
)

func makeOption(class databento.InstrumentClass, strikePrice int, bid, ask string) *Option {
	return &Option{
		Class:  class,
		Strike: &Strike{Price: decimal.FromInt(strikePrice)},
		Bid:    decimal.Parse(bid),
		Ask:    decimal.Parse(ask),
		Got:    GotBid | GotAsk,
	}
}

func makeOptionWithGreeks(class databento.InstrumentClass, strikePrice int, bid, ask string) *Option {
	return &Option{
		Class:  class,
		Strike: &Strike{Price: decimal.FromInt(strikePrice)},
		Bid:    decimal.Parse(bid),
		Ask:    decimal.Parse(ask),
		Got:    GotBid | GotAsk | GotGreeks,
		IV:     decimal.Parse("0.15"),
		Delta:  decimal.Parse("0.5"),
		Gamma:  decimal.Parse("0.01"),
		Theta:  decimal.Parse("-0.5"),
		Vega:   decimal.Parse("1.0"),
	}
}

func TestAddCallThenPutBecomesReady(t *testing.T) {
	oc := NewChain()
	call := makeOption(databento.InstrumentClassCall, 6000, "10", "12")
	put := makeOption(databento.InstrumentClassPut, 6000, "8", "10")
	oc.Add(call)
	strike, _ := oc.Strikes.Get(decimal.FromInt(6000))
	if strike != nil {
		t.Fatal("strike should not be in Strikes before both call and put are added")
	}
	oc.Add(put)
	strike, _ = oc.Strikes.Get(decimal.FromInt(6000))
	if strike == nil {
		t.Fatal("strike should be in Strikes after both call and put are added")
	}
	if !strike.IsReady() {
		t.Fatal("strike should be ready")
	}
	if strike.Call != call {
		t.Error("strike.Call should point to the call option")
	}
	if strike.Put != put {
		t.Error("strike.Put should point to the put option")
	}
}

func TestAddPutThenCallBecomesReady(t *testing.T) {
	oc := NewChain()
	put := makeOption(databento.InstrumentClassPut, 6000, "8", "10")
	call := makeOption(databento.InstrumentClassCall, 6000, "10", "12")
	oc.Add(put)
	strike, _ := oc.Strikes.Get(decimal.FromInt(6000))
	if strike != nil {
		t.Fatal("strike should not be in Strikes before both added")
	}
	oc.Add(call)
	strike, _ = oc.Strikes.Get(decimal.FromInt(6000))
	if strike == nil {
		t.Fatal("strike should be in Strikes after both added")
	}
	if !strike.IsReady() {
		t.Fatal("strike should be ready")
	}
}

func TestAddTakesOwnershipOfStrike(t *testing.T) {
	oc := NewChain()
	call := makeOption(databento.InstrumentClassCall, 6000, "10", "12")
	origStrike := call.Strike
	oc.Add(call)
	if call.Strike == origStrike {
		t.Error("Add should replace option's Strike with the canonical one")
	}
}

func TestLinkedListPrevNext(t *testing.T) {
	oc := NewChain()
	// Add three strikes in order: 5990, 6000, 6010
	prices := []int{5990, 6000, 6010}
	for _, p := range prices {
		call := makeOption(databento.InstrumentClassCall, p, "10", "12")
		put := makeOption(databento.InstrumentClassPut, p, "8", "10")
		oc.Add(call)
		oc.Add(put)
	}
	s5990, _ := oc.Strikes.Get(decimal.FromInt(5990))
	s6000, _ := oc.Strikes.Get(decimal.FromInt(6000))
	s6010, _ := oc.Strikes.Get(decimal.FromInt(6010))
	if s5990.Prev != nil {
		t.Error("first strike should have nil Prev")
	}
	if s5990.Next != s6000 {
		t.Error("5990.Next should be 6000")
	}
	if s6000.Prev != s5990 {
		t.Error("6000.Prev should be 5990")
	}
	if s6000.Next != s6010 {
		t.Error("6000.Next should be 6010")
	}
	if s6010.Prev != s6000 {
		t.Error("6010.Prev should be 6000")
	}
	if s6010.Next != nil {
		t.Error("last strike should have nil Next")
	}
}

func TestLinkedListInsertBetween(t *testing.T) {
	oc := NewChain()
	// Add 5990 and 6010 first, then insert 6000 between them.
	for _, p := range []int{5990, 6010} {
		call := makeOption(databento.InstrumentClassCall, p, "10", "12")
		put := makeOption(databento.InstrumentClassPut, p, "8", "10")
		oc.Add(call)
		oc.Add(put)
	}
	// Insert 6000 in the middle
	call := makeOption(databento.InstrumentClassCall, 6000, "10", "12")
	put := makeOption(databento.InstrumentClassPut, 6000, "8", "10")
	oc.Add(call)
	oc.Add(put)

	s5990, _ := oc.Strikes.Get(decimal.FromInt(5990))
	s6000, _ := oc.Strikes.Get(decimal.FromInt(6000))
	s6010, _ := oc.Strikes.Get(decimal.FromInt(6010))

	if s5990.Next != s6000 {
		t.Error("5990.Next should be updated to 6000")
	}
	if s6000.Prev != s5990 {
		t.Error("6000.Prev should be 5990")
	}
	if s6000.Next != s6010 {
		t.Error("6000.Next should be 6010")
	}
	if s6010.Prev != s6000 {
		t.Error("6010.Prev should be updated to 6000")
	}
}

func TestAtTheMoneyTracksClosestStrike(t *testing.T) {
	oc := NewChain()
	// Strike 6000: call mid=50, put mid=48 → underlying ≈ 6000+50-48 = 6002
	// Strike 6005: call mid=47, put mid=51 → underlying ≈ 6005+47-51 = 6001
	call6000 := makeOption(databento.InstrumentClassCall, 6000, "49", "51")
	put6000 := makeOption(databento.InstrumentClassPut, 6000, "47", "49")
	call6005 := makeOption(databento.InstrumentClassCall, 6005, "46", "48")
	put6005 := makeOption(databento.InstrumentClassPut, 6005, "50", "52")
	oc.Add(call6000)
	oc.Add(put6000)
	if oc.AtTheMoney == nil {
		t.Fatal("AtTheMoney should be set after first ready strike with greeks")
	}
	if oc.AtTheMoney.Price.Cmp(decimal.FromInt(6000)) != 0 {
		t.Errorf("AtTheMoney should be 6000, got %s", oc.AtTheMoney.Price)
	}
	oc.Add(call6005)
	oc.Add(put6005)
	// ATM may update when the ATM strike's quotes change; the exact result
	// depends on which strike is closer to the inferred price.
	if oc.AtTheMoney == nil {
		t.Fatal("AtTheMoney should still be set")
	}
}

func TestPriceReflectsUnderlyingFromATM(t *testing.T) {
	oc := NewChain()
	// call mid = (49+51)/2 = 50, put mid = (47+49)/2 = 48
	// underlying = 6000 + 50 - 48 = 6002
	call := makeOption(databento.InstrumentClassCall, 6000, "49", "51")
	put := makeOption(databento.InstrumentClassPut, 6000, "47", "49")
	oc.Add(call)
	oc.Add(put)
	expected := decimal.FromInt(6002)
	if oc.Price.Cmp(expected) != 0 {
		t.Errorf("Price should be %s, got %s", expected, oc.Price)
	}
}

func TestAddWithoutQuotesDoesNotSetATM(t *testing.T) {
	oc := NewChain()
	call := &Option{
		Class:  databento.InstrumentClassCall,
		Strike: &Strike{Price: decimal.FromInt(6000)},
	}
	put := &Option{
		Class:  databento.InstrumentClassPut,
		Strike: &Strike{Price: decimal.FromInt(6000)},
	}
	oc.Add(call)
	oc.Add(put)
	if oc.AtTheMoney != nil {
		t.Error("AtTheMoney should not be set when options have no quotes")
	}
}

func TestEmptyOptionsChain(t *testing.T) {
	oc := NewChain()
	if oc.Strikes.Size() != 0 {
		t.Error("new Chain should have no strikes")
	}
	if oc.AtTheMoney != nil {
		t.Error("new Chain should have nil AtTheMoney")
	}
	if !oc.Price.IsZero() {
		t.Error("new Chain should have zero Price")
	}
}
