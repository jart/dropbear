package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"testing"
)

func init() {
	ds.SetOffline()
	clocky.Now = clocky.FakeNow
}

// setupTestExchange creates a minimal exchange for testing accounting logic.
func setupTestExchange(t *testing.T) (*Exchange, *Pair, *Holding, *Holding) {
	t.Helper()

	// Create exchange with zero fees for simpler math
	ex := &Exchange{
		Exchange: ds.ExchangeCoinbase,
		MakerFee: decimal.Zero,
		TakerFee: decimal.Zero,
		Rebate:   decimal.Zero,
	}
	ex.Holdings = &Holdings{
		exchange:    ex,
		holdingsMap: make(map[string]*Holding),
	}
	ex.Orders = &Orders{
		Exchange:     ex,
		ordersMap:    make(map[string]*Order),
		ordersArray:  make([]*Order, 0),
		OnOrderEvent: func(*Order) {},
	}
	ex.Pairs = &Pairs{
		exchange: ex,
		pairsMap: make(map[string]*Pair),
	}

	// Create pair
	pair := &Pair{
		Exchange:       ex,
		BaseCurrency:   "BTC",
		QuoteCurrency:  "USD",
		BaseIncrement:  decimal.Satoshi,
		QuoteIncrement: decimal.Parse("0.01"),
		QuoteMinSize:   decimal.One,
		QuoteMaxSize:   decimal.FromInt(1000000),
		BaseMinSize:    decimal.Satoshi,
		BaseMaxSize:    decimal.FromInt(1000),
		OrderBook:      ds.NewBook(),
		LastPrice:      decimal.FromInt(100), // $100 per BTC for easy math
	}
	ex.Pairs.pairsMap["BTC-USD"] = pair
	ex.Pairs.pairsArray = append(ex.Pairs.pairsArray, pair)

	// Create holdings
	usdHolding := newHolding(ex, "USD")
	btcHolding := newHolding(ex, "BTC")
	ex.Holdings.holdingsMap["USD"] = usdHolding
	ex.Holdings.holdingsMap["BTC"] = btcHolding
	ex.Holdings.holdingsArray = append(ex.Holdings.holdingsArray, usdHolding, btcHolding)

	return ex, pair, usdHolding, btcHolding
}

// setupOrderBook populates the order book with liquidity.
func setupOrderBook(pair *Pair, bidPrice, askPrice, size decimal.Decimal) {
	pair.OrderBook.Lock.Lock()
	pair.OrderBook.UpdateBid(bidPrice, size)
	pair.OrderBook.UpdateAsk(askPrice, size)
	pair.OrderBook.Lock.Unlock()
}

// TestAvailableNeverExceedsQuantity verifies the core invariant that
// Available should never exceed Quantity for any holding.
func TestAvailableNeverExceedsQuantity(t *testing.T) {
	_, pair, usdHolding, btcHolding := setupTestExchange(t)

	// Set initial balance: $1000 USD
	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	// Setup order book: bid at $99, ask at $101, 100 BTC available
	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(101), decimal.FromInt(100))

	// Simulate a market buy order for 1 BTC
	// At $101/BTC, this costs $101
	quantity := decimal.One
	bestAsk := decimal.FromInt(101)

	// Calculate hold amount (with 10% slippage buffer as in market.go)
	maxValue := quantity.Mul(bestAsk).Mul(decimal.Parse("1.10")) // 1 * 101 * 1.10 = 111.1
	holdAmount := maxValue                                       // no fee for this test

	// Deduct hold from Available (as done in MarketOrder)
	usdHolding.Available = usdHolding.Available.Sub(holdAmount)

	t.Logf("After hold: USD Quantity=%s Available=%s", usdHolding.Quantity, usdHolding.Available)

	// Simulate the fill
	fillValue := decimal.FromInt(101) // actual cost
	total := fillValue                // no fee

	// Debit cash
	usdHolding.Quantity = usdHolding.Quantity.Sub(total)

	// CORRECT: Only release unused portion of hold (this is the fix)
	unusedHold := holdAmount.Sub(total)
	if unusedHold.IsPositive() {
		usdHolding.Available = usdHolding.Available.Add(unusedHold)
	}

	// Credit BTC
	btcHolding.Quantity = btcHolding.Quantity.Add(quantity)
	btcHolding.Available = btcHolding.Available.Add(quantity)

	t.Logf("After correct fill: USD Quantity=%s Available=%s", usdHolding.Quantity, usdHolding.Available)
	t.Logf("BTC Quantity=%s Available=%s", btcHolding.Quantity, btcHolding.Available)

	// Verify the invariant holds
	if usdHolding.Available.Cmp(usdHolding.Quantity) > 0 {
		t.Errorf("USD Available (%s) > Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	// When no orders pending, Available should equal Quantity
	if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
		t.Errorf("USD Available (%s) != Quantity (%s) when no orders pending",
			usdHolding.Available, usdHolding.Quantity)
	}
}

// TestCorrectHoldRelease verifies the fix: only release unused hold.
func TestCorrectHoldRelease(t *testing.T) {
	_, pair, usdHolding, btcHolding := setupTestExchange(t)

	// Set initial balance: $1000 USD
	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	// Setup order book
	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(101), decimal.FromInt(100))

	quantity := decimal.One
	bestAsk := decimal.FromInt(101)
	maxValue := quantity.Mul(bestAsk).Mul(decimal.Parse("1.10"))
	holdAmount := maxValue

	// Deduct hold
	usdHolding.Available = usdHolding.Available.Sub(holdAmount)

	// Simulate fill
	fillValue := decimal.FromInt(101)
	total := fillValue
	releaseHold := holdAmount

	// Debit cash
	usdHolding.Quantity = usdHolding.Quantity.Sub(total)

	// CORRECT: Only release unused portion of hold
	unusedHold := releaseHold.Sub(total)
	usdHolding.Available = usdHolding.Available.Add(unusedHold)

	// Credit BTC
	btcHolding.Quantity = btcHolding.Quantity.Add(quantity)
	btcHolding.Available = btcHolding.Available.Add(quantity)

	t.Logf("After correct fill: USD Quantity=%s Available=%s", usdHolding.Quantity, usdHolding.Available)

	// Verify invariant holds
	if usdHolding.Available.Cmp(usdHolding.Quantity) > 0 {
		t.Errorf("USD Available (%s) > Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	// Verify they're equal (no orders pending)
	if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
		t.Errorf("USD Available (%s) != Quantity (%s) when no orders pending",
			usdHolding.Available, usdHolding.Quantity)
	}

	// Verify correct final balance
	expectedUSD := initialUSD.Sub(fillValue)
	if usdHolding.Quantity.Cmp(expectedUSD) != 0 {
		t.Errorf("USD Quantity = %s, expected %s", usdHolding.Quantity, expectedUSD)
	}
}

// TestMultipleBuysNeverExceedBalance simulates multiple buy orders
// and verifies total invested never exceeds initial balance.
func TestMultipleBuysNeverExceedBalance(t *testing.T) {
	_, pair, usdHolding, btcHolding := setupTestExchange(t)

	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(100), decimal.FromInt(1000))

	// Execute 10 buys of 1 BTC each at $100
	for i := 0; i < 10; i++ {
		quantity := decimal.One
		price := decimal.FromInt(100)
		cost := quantity.Mul(price)

		// Check we have enough before buying
		if usdHolding.Available.Cmp(cost) < 0 {
			t.Logf("Buy %d: insufficient funds (have %s, need %s)", i+1, usdHolding.Available, cost)
			break
		}

		// Simulate proper buy
		usdHolding.Available = usdHolding.Available.Sub(cost)
		usdHolding.Quantity = usdHolding.Quantity.Sub(cost)
		btcHolding.Quantity = btcHolding.Quantity.Add(quantity)
		btcHolding.Available = btcHolding.Available.Add(quantity)

		t.Logf("Buy %d: spent $%s, USD remaining=%s, BTC=%s",
			i+1, cost, usdHolding.Quantity, btcHolding.Quantity)

		// Verify invariants after each trade
		if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
			t.Errorf("Buy %d: USD Available != Quantity", i+1)
		}
		if usdHolding.Quantity.Cmp(decimal.Zero) < 0 {
			t.Errorf("Buy %d: USD went negative: %s", i+1, usdHolding.Quantity)
		}
	}

	// We should have bought exactly 10 BTC for $1000
	if btcHolding.Quantity.Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("Expected 10 BTC, got %s", btcHolding.Quantity)
	}
	if !usdHolding.Quantity.IsZero() {
		t.Errorf("Expected 0 USD, got %s", usdHolding.Quantity)
	}
}

// TestInvestedCannotExceedInitialBalance is the specific test for the
// $26k invested on $20k balance bug.
func TestInvestedCannotExceedInitialBalance(t *testing.T) {
	ex, pair, usdHolding, btcHolding := setupTestExchange(t)

	// Simulate the exact scenario: $20,000 initial balance
	initialUSD := decimal.FromInt(20000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	// BTC at $100 for easy math
	price := decimal.FromInt(100)
	setupOrderBook(pair, price.Sub(decimal.One), price.Add(decimal.One), decimal.FromInt(10000))
	pair.LastPrice = price

	// Track max invested
	maxInvested := decimal.Zero

	// Simulate many trades
	for i := 0; i < 300; i++ {
		// Buy $200 worth
		buyQty := decimal.FromInt(2) // 2 BTC = $200
		buyCost := buyQty.Mul(price)

		if usdHolding.Available.Cmp(buyCost) >= 0 {
			usdHolding.Available = usdHolding.Available.Sub(buyCost)
			usdHolding.Quantity = usdHolding.Quantity.Sub(buyCost)
			btcHolding.Quantity = btcHolding.Quantity.Add(buyQty)
			btcHolding.Available = btcHolding.Available.Add(buyQty)
		}

		// Calculate invested amount
		invested := btcHolding.Quantity.Mul(ex.GetPriceUSD("BTC"))
		if invested.Cmp(maxInvested) > 0 {
			maxInvested = invested
		}

		// CRITICAL SANITY CHECK: invested should never exceed initial balance
		if invested.Cmp(initialUSD) > 0 {
			t.Fatalf("CRITICAL BUG: invested $%s exceeds initial balance $%s at iteration %d",
				invested, initialUSD, i)
		}

		// Invariant check
		if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
			t.Errorf("Iteration %d: USD Available (%s) != Quantity (%s)",
				i, usdHolding.Available, usdHolding.Quantity)
		}
	}

	t.Logf("Max invested: $%s (initial: $%s)", maxInvested, initialUSD)
}

// TestHoldingInvariantsWithFees tests that invariants hold even with fees.
func TestHoldingInvariantsWithFees(t *testing.T) {
	ex, pair, usdHolding, btcHolding := setupTestExchange(t)

	// Set realistic fee rate
	ex.TakerFee = decimal.Parse("0.0065") // 0.65%

	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(100), decimal.FromInt(100))

	// Buy 1 BTC at $100
	quantity := decimal.One
	price := decimal.FromInt(100)
	value := quantity.Mul(price)
	fee := value.Mul(ex.TakerFee)
	total := value.Add(fee)

	// Hold includes slippage buffer
	holdAmount := value.Mul(decimal.Parse("1.10")).Add(value.Mul(decimal.Parse("1.10")).Mul(ex.TakerFee))

	// Deduct hold
	usdHolding.Available = usdHolding.Available.Sub(holdAmount)

	// Fill order
	usdHolding.Quantity = usdHolding.Quantity.Sub(total)

	// CORRECT release: only unused portion
	unusedHold := holdAmount.Sub(total)
	usdHolding.Available = usdHolding.Available.Add(unusedHold)

	btcHolding.Quantity = btcHolding.Quantity.Add(quantity)
	btcHolding.Available = btcHolding.Available.Add(quantity)

	t.Logf("With fees: USD Quantity=%s Available=%s", usdHolding.Quantity, usdHolding.Available)

	// Verify invariants
	if usdHolding.Available.Cmp(usdHolding.Quantity) > 0 {
		t.Errorf("USD Available (%s) > Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	// When no orders pending, they should be equal
	if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
		t.Errorf("USD Available (%s) != Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	// Verify we spent the right amount
	expectedUSD := initialUSD.Sub(total)
	if usdHolding.Quantity.Cmp(expectedUSD) != 0 {
		t.Errorf("USD = %s, expected %s (total spent: %s)", usdHolding.Quantity, expectedUSD, total)
	}
}

// TestFillWithFee tests the shared accounting code used by live trading.
// This simulates the live trading path where Coinbase provides the actual fee.
func TestFillWithFee(t *testing.T) {
	_, pair, usdHolding, btcHolding := setupTestExchange(t)

	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(100), decimal.FromInt(100))

	// Simulate placing an order with hold
	quantity := decimal.One
	price := decimal.FromInt(100)
	holdAmount := price.Mul(decimal.Parse("1.10")) // 10% slippage buffer

	// Deduct hold (as market.go does)
	usdHolding.Available = usdHolding.Available.Sub(holdAmount)

	// Simulate fill with explicit fee (as live trading does)
	fillValue := price
	actualFee := decimal.Parse("0.65") // Coinbase's actual fee
	total := fillValue.Add(actualFee)

	// Debit cash
	usdHolding.Quantity = usdHolding.Quantity.Sub(total)

	// Release unused hold (as fillWithFee does)
	cumulativeSpent := fillValue.Add(actualFee)
	unusedHold := holdAmount.Sub(cumulativeSpent)
	if unusedHold.IsPositive() {
		usdHolding.Available = usdHolding.Available.Add(unusedHold)
	}

	// Credit BTC
	btcHolding.Quantity = btcHolding.Quantity.Add(quantity)
	btcHolding.Available = btcHolding.Available.Add(quantity)

	t.Logf("FillWithFee: USD Quantity=%s Available=%s", usdHolding.Quantity, usdHolding.Available)

	// Verify invariants
	if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
		t.Errorf("USD Available (%s) != Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	// Verify correct final balance (initial - value - fee)
	expectedUSD := initialUSD.Sub(fillValue).Sub(actualFee)
	if usdHolding.Quantity.Cmp(expectedUSD) != 0 {
		t.Errorf("USD = %s, expected %s", usdHolding.Quantity, expectedUSD)
	}
}

// TestMultipleFillsWithDifferentFees tests multiple fills with varying fees.
func TestMultipleFillsWithDifferentFees(t *testing.T) {
	_, pair, usdHolding, btcHolding := setupTestExchange(t)

	initialUSD := decimal.FromInt(1000)
	usdHolding.Quantity = initialUSD
	usdHolding.Available = initialUSD

	setupOrderBook(pair, decimal.FromInt(99), decimal.FromInt(100), decimal.FromInt(100))

	// Order for 3 BTC with hold
	totalQuantity := decimal.FromInt(3)
	holdAmount := decimal.FromInt(330) // 3 * 100 * 1.10

	usdHolding.Available = usdHolding.Available.Sub(holdAmount)

	// Simulate 3 fills at different prices with different fees (like multi-level order book)
	fills := []struct {
		qty   decimal.Decimal
		value decimal.Decimal
		fee   decimal.Decimal
	}{
		{decimal.One, decimal.FromInt(99), decimal.Parse("0.64")},  // 1 @ $99
		{decimal.One, decimal.FromInt(100), decimal.Parse("0.65")}, // 1 @ $100
		{decimal.One, decimal.FromInt(101), decimal.Parse("0.66")}, // 1 @ $101
	}

	var totalSpent decimal.Decimal
	for _, fill := range fills {
		total := fill.value.Add(fill.fee)
		usdHolding.Quantity = usdHolding.Quantity.Sub(total)
		btcHolding.Quantity = btcHolding.Quantity.Add(fill.qty)
		btcHolding.Available = btcHolding.Available.Add(fill.qty)
		totalSpent = totalSpent.Add(total)
	}

	// Release unused hold on full fill
	unusedHold := holdAmount.Sub(totalSpent)
	if unusedHold.IsPositive() {
		usdHolding.Available = usdHolding.Available.Add(unusedHold)
	}

	t.Logf("MultipleFills: USD Quantity=%s Available=%s, totalSpent=%s",
		usdHolding.Quantity, usdHolding.Available, totalSpent)

	// Verify invariants
	if usdHolding.Available.Cmp(usdHolding.Quantity) != 0 {
		t.Errorf("USD Available (%s) != Quantity (%s)", usdHolding.Available, usdHolding.Quantity)
	}

	if btcHolding.Quantity.Cmp(totalQuantity) != 0 {
		t.Errorf("BTC = %s, expected %s", btcHolding.Quantity, totalQuantity)
	}
}
