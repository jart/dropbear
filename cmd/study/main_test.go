package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"os"
	"testing"
)

func init() {
	netty.SetOffline()
}

func TestStudy(t *testing.T) {
	paths := flag.Args()
	if len(paths) == 0 {
		t.Skip("no sip files specified (use: go test -run Study -timeout 0 -- file.sip)")
	}
	for _, path := range paths {
		study(path)
	}
}

func TestTick(t *testing.T) {
	tests := []struct {
		price decimal.Decimal
		want  decimal.Decimal
	}{
		{decimal.Parse("0.50"), decimal.Pip},
		{decimal.Parse("0.99"), decimal.Pip},
		{decimal.Parse("1.00"), decimal.Cent},
		{decimal.Parse("50.00"), decimal.Cent},
	}
	for _, tt := range tests {
		got := Tick(tt.price)
		if got != tt.want {
			t.Errorf("Tick(%s) = %s, want %s", tt.price, got, tt.want)
		}
	}
}

func TestTradeQuantity(t *testing.T) {
	*flagSize = decimal.Parse("1000")
	*flagFloor = decimal.Parse("2")
	tests := []struct {
		name   string
		price  decimal.Decimal
		maxQty decimal.Decimal
		want   decimal.Decimal
	}{
		{"normal $50", decimal.Parse("50"), decimal.Parse("1000"), decimal.Parse("20")},
		{"too expensive", decimal.Parse("5000"), decimal.Parse("1000"), decimal.Zero},
		{"$1 stock", decimal.Parse("1"), decimal.Parse("1000"), decimal.Parse("1000")},
		{"max limited", decimal.Parse("50"), decimal.Parse("10"), decimal.Parse("10")},
		{"$500 → 2 shares ok", decimal.Parse("500"), decimal.Parse("1000"), decimal.Parse("2")},
		{"$501 → 1 share < floor", decimal.Parse("501"), decimal.Parse("1000"), decimal.Zero},
	}
	for _, tt := range tests {
		got := tradeQuantity(tt.price, tt.maxQty)
		if got != tt.want {
			t.Errorf("tradeQuantity(%s) = %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestEstimateFee(t *testing.T) {
	qty := decimal.Parse("100")
	price := decimal.Parse("50")

	fee := estimateFee(1, qty, price, true)
	if !fee.IsNegative() {
		t.Errorf("maker buy fee should be negative (rebate), got %s", fee)
	}

	fee = estimateFee(-1, qty, price, true)
	t.Logf("maker sell fee: %s", fee)

	takerFee := estimateTakerFee(1, qty, price)
	makerFee := estimateFee(1, qty, price, true)
	if takerFee.Cmp(makerFee) <= 0 {
		t.Errorf("taker fee (%s) should exceed maker fee (%s)", takerFee, makerFee)
	}
}

func TestSignalDetection(t *testing.T) {
	*flagThreshold = decimal.Parse("0.3")
	*flagISO = decimal.Parse("200")

	gSymbols = make(map[symbol.Symbol]*symState)
	gSymQuote = make(map[symbol.Symbol]int64)
	gSymTrade = make(map[symbol.Symbol]int64)
	gTotalSignals = 0
	gBuySignals = 0
	gSellSignals = 0
	gHStats = [6]horizonStats{}
	gPriceBuckets = make([]bucketStats, len(priceBuckets))
	gSpreadBuckets = make([]bucketStats, len(spreadBuckets))

	st := &symState{
		hasQuote: true,
		quote: sip.Quote{
			BidPrice: decimal.Parse("50.00"),
			AskPrice: decimal.Parse("50.02"),
			BidSize:  800,
			AskSize:  200,
		},
	}
	testSym, _ := symbol.Parse("TEST")
	gSymbols[testSym] = st

	q := &sip.Quote{
		BidPrice:  decimal.Parse("50.00"),
		AskPrice:  decimal.Parse("50.02"),
		BidSize:   800,
		AskSize:   200,
		Timestamp: clocky.Time(1 * clocky.Second),
	}

	evaluateEntry(st, q)

	if gBuySignals != 1 {
		t.Errorf("expected 1 buy signal, got %d", gBuySignals)
	}
	if len(st.pending) != 1 {
		t.Errorf("expected 1 pending signal, got %d", len(st.pending))
	}
	if st.pending[0].direction != 1 {
		t.Errorf("expected bullish direction, got %d", st.pending[0].direction)
	}

	// balanced quote should not trigger
	gBuySignals = 0
	gSellSignals = 0
	st2 := &symState{
		hasQuote: true,
		quote: sip.Quote{
			BidPrice: decimal.Parse("50.00"),
			AskPrice: decimal.Parse("50.02"),
			BidSize:  500,
			AskSize:  500,
		},
	}
	testSym2, _ := symbol.Parse("TEST2")
	gSymbols[testSym2] = st2

	q2 := &sip.Quote{
		BidPrice:  decimal.Parse("50.00"),
		AskPrice:  decimal.Parse("50.02"),
		BidSize:   500,
		AskSize:   500,
		Timestamp: clocky.Time(1 * clocky.Second),
	}
	evaluateEntry(st2, q2)

	if gBuySignals != 0 || gSellSignals != 0 {
		t.Errorf("balanced quote should not trigger signal, got buy=%d sell=%d",
			gBuySignals, gSellSignals)
	}
}

func TestBucketLabels(t *testing.T) {
	// just make sure they don't panic
	for i := range priceBuckets {
		s := priceBucketLabel(i)
		if s == "" {
			t.Error("empty price bucket label")
		}
	}
	for i := range spreadBuckets {
		s := spreadBucketLabel(i)
		if s == "" {
			t.Error("empty spread bucket label")
		}
	}
}

func TestSipFileExists(t *testing.T) {
	path := os.ExpandEnv("$HOME/sip/sipdata-2026-01-07.sip")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("sip file not found: %s", path)
	}
	f, err := sip.OpenFile(path)
	if err != nil {
		t.Fatalf("failed to open sip file: %v", err)
	}
	defer f.Close()
	t.Logf("sip file has %d messages", f.Count())
	mFirst, _ := f.Get(0)
	t.Logf("first: type=%c symbol=%s time=%s",
		mFirst.Type, mFirst.Symbol, mFirst.Timestamp)
	mLast, _ := f.Get(f.Count() - 1)
	t.Logf("last:  type=%c symbol=%s time=%s",
		mLast.Type, mLast.Symbol, mLast.Timestamp)
}
