package ds

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"testing"
)

var (
	zero  = decimal.Zero
	one   = decimal.One
	two   = decimal.Two
	three = decimal.FromInt(3)
	four  = decimal.FromInt(4)
	five  = decimal.FromInt(5)
	six   = decimal.FromInt(6)
	seven = decimal.FromInt(7)
	eight = decimal.FromInt(8)
)

func TestLots_Simple(t *testing.T) {
	lots := NewLots(CostBasisMethodLIFO)
	lots.Add(clocky.Time(0), two, two)
	lots.Add(clocky.Time(1), two, four)
	if lots.Size.Cmp(four) != 0 {
		t.Fatalf("expected size 4, got %s", lots.Size)
	}
	if lots.Cost.Cmp(six) != 0 {
		t.Fatalf("expected cost 6, got %s", lots.Cost)
	}
	x := lots.Consume(one, three)
	if x.Cmp(two) != 0 {
		t.Fatalf("expected cost 2, got %s", x)
	}
	if lots.Size.Cmp(three) != 0 {
		t.Fatalf("expected size 3, got %s", lots.Size)
	}
	if lots.Cost.Cmp(four) != 0 {
		t.Fatalf("expected cost 4, got %s", lots.Cost)
	}
	x = lots.Consume(three, three)
	if x.Cmp(four) != 0 {
		t.Fatalf("expected cost 4, got %s", x)
	}
	if lots.Size.Cmp(zero) != 0 {
		t.Fatalf("expected size 0, got %s", lots.Size)
	}
	if lots.Cost.Cmp(zero) != 0 {
		t.Fatalf("expected cost 0, got %s", lots.Cost)
	}
}

func TestLots_FIFO(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	// Add 3 lots at different times with different prices
	lots.Add(clocky.Time(100), one, decimal.FromInt(10))  // $10/unit
	lots.Add(clocky.Time(200), one, decimal.FromInt(20))  // $20/unit
	lots.Add(clocky.Time(300), one, decimal.FromInt(30))  // $30/unit

	// FIFO should consume oldest first
	cost := lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("FIFO should consume oldest first: expected 10, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("FIFO should consume second oldest: expected 20, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(30)) != 0 {
		t.Fatalf("FIFO should consume newest last: expected 30, got %s", cost)
	}
}

func TestLots_LIFO(t *testing.T) {
	lots := NewLots(CostBasisMethodLIFO)
	// Add 3 lots at different times with different prices
	lots.Add(clocky.Time(100), one, decimal.FromInt(10))  // $10/unit
	lots.Add(clocky.Time(200), one, decimal.FromInt(20))  // $20/unit
	lots.Add(clocky.Time(300), one, decimal.FromInt(30))  // $30/unit

	// LIFO should consume newest first
	cost := lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(30)) != 0 {
		t.Fatalf("LIFO should consume newest first: expected 30, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("LIFO should consume second newest: expected 20, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("LIFO should consume oldest last: expected 10, got %s", cost)
	}
}

func TestLots_HIFO(t *testing.T) {
	lots := NewLots(CostBasisMethodHIFO)
	// Add 3 lots with different cost-per-unit
	lots.Add(clocky.Time(100), one, decimal.FromInt(20))  // $20/unit
	lots.Add(clocky.Time(200), one, decimal.FromInt(10))  // $10/unit
	lots.Add(clocky.Time(300), one, decimal.FromInt(30))  // $30/unit

	// HIFO should consume highest cost first
	cost := lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(30)) != 0 {
		t.Fatalf("HIFO should consume highest first: expected 30, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("HIFO should consume second highest: expected 20, got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("HIFO should consume lowest last: expected 10, got %s", cost)
	}
}

func TestLots_GetCostBasis(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	lots.Add(clocky.Time(100), two, decimal.FromInt(20))  // $10/unit
	lots.Add(clocky.Time(200), two, decimal.FromInt(40))  // $20/unit

	// GetCostBasis should not mutate
	cost := lots.GetCostBasis(one, zero)
	if cost.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("expected cost 10, got %s", cost)
	}
	if lots.Size.Cmp(four) != 0 {
		t.Fatalf("GetCostBasis should not mutate size: expected 4, got %s", lots.Size)
	}

	// Partial lot
	cost = lots.GetCostBasis(three, zero)
	// 2 units @ $10 = $20, plus 1 unit @ $20 = $20, total = $40
	if cost.Cmp(decimal.FromInt(40)) != 0 {
		t.Fatalf("expected cost 40, got %s", cost)
	}

	// Exact match
	cost = lots.GetCostBasis(four, zero)
	if cost.Cmp(decimal.FromInt(60)) != 0 {
		t.Fatalf("expected cost 60, got %s", cost)
	}
}

func TestLots_GetCostBasis_Fallback(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	lots.Add(clocky.Time(100), two, decimal.FromInt(20))  // $10/unit

	// Request more than available, fallback price = $50/unit
	fallback := decimal.FromInt(50)
	cost := lots.GetCostBasis(five, fallback)
	// 2 units @ $10 = $20, plus 3 units @ $50 = $150, total = $170
	if cost.Cmp(decimal.FromInt(170)) != 0 {
		t.Fatalf("expected cost 170, got %s", cost)
	}
}

func TestLots_Consume_Partial(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	lots.Add(clocky.Time(100), four, decimal.FromInt(40))  // $10/unit

	// Consume half
	cost := lots.Consume(two, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("expected cost 20, got %s", cost)
	}
	if lots.Size.Cmp(two) != 0 {
		t.Fatalf("expected size 2, got %s", lots.Size)
	}
	if lots.Cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("expected remaining cost 20, got %s", lots.Cost)
	}

	// Consume the rest
	cost = lots.Consume(two, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("expected cost 20, got %s", cost)
	}
	if !lots.Size.IsZero() {
		t.Fatalf("expected size 0, got %s", lots.Size)
	}
}

func TestLots_Consume_GoesNegative(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	lots.Add(clocky.Time(100), two, decimal.FromInt(20))  // $10/unit

	// Consume more than available with fallback price $50/unit
	fallback := decimal.FromInt(50)
	cost := lots.Consume(five, fallback)
	// 2 units @ $10 = $20, plus 3 units @ $50 = $150, total = $170
	if cost.Cmp(decimal.FromInt(170)) != 0 {
		t.Fatalf("expected cost 170, got %s", cost)
	}

	// Size should be negative (debt)
	expectedSize := decimal.FromInt(-3)
	if lots.Size.Cmp(expectedSize) != 0 {
		t.Fatalf("expected size -3, got %s", lots.Size)
	}
}

func TestLots_DebtRecovery_Partial(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)

	// Create debt by consuming from empty
	lots.Consume(three, decimal.FromInt(100))  // now at -3

	if lots.Size.Cmp(decimal.FromInt(-3)) != 0 {
		t.Fatalf("expected size -3, got %s", lots.Size)
	}

	// Add 2 units - should partially repay debt
	lots.Add(clocky.Time(100), two, decimal.FromInt(20))

	// Should still have -1 debt
	if lots.Size.Cmp(decimal.FromInt(-1)) != 0 {
		t.Fatalf("expected size -1, got %s", lots.Size)
	}
	// No lots should exist
	if !lots.Cost.IsZero() {
		t.Fatalf("expected cost 0 (no lots), got %s", lots.Cost)
	}
}

func TestLots_DebtRecovery_Exact(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)

	// Create debt
	lots.Consume(three, decimal.FromInt(100))  // now at -3

	// Add exactly 3 units - should clear debt with nothing left over
	lots.Add(clocky.Time(100), three, decimal.FromInt(30))

	if !lots.Size.IsZero() {
		t.Fatalf("expected size 0, got %s", lots.Size)
	}
	if !lots.Cost.IsZero() {
		t.Fatalf("expected cost 0, got %s", lots.Cost)
	}
}

func TestLots_DebtRecovery_Overpay(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)

	// Create debt of 2
	lots.Consume(two, decimal.FromInt(100))

	// Add 5 units @ $10/unit = $50 total
	lots.Add(clocky.Time(100), five, decimal.FromInt(50))

	// Should have 3 units remaining (5 - 2 debt)
	if lots.Size.Cmp(three) != 0 {
		t.Fatalf("expected size 3, got %s", lots.Size)
	}

	// Cost should be proportional: 3/5 * $50 = $30
	if lots.Cost.Cmp(decimal.FromInt(30)) != 0 {
		t.Fatalf("expected cost 30, got %s", lots.Cost)
	}
}

func TestLots_HIFO_PartialConsume(t *testing.T) {
	lots := NewLots(CostBasisMethodHIFO)
	// Add lot with 4 units @ $25/unit = $100
	lots.Add(clocky.Time(100), four, decimal.FromInt(100))
	// Add lot with 2 units @ $10/unit = $20
	lots.Add(clocky.Time(200), two, decimal.FromInt(20))

	// HIFO should consume from $25/unit lot first
	// Consume 2 units: 2 @ $25 = $50
	cost := lots.Consume(two, zero)
	if cost.Cmp(decimal.FromInt(50)) != 0 {
		t.Fatalf("expected cost 50, got %s", cost)
	}

	// Remaining: 2 @ $25 = $50, 2 @ $10 = $20, total = $70
	if lots.Cost.Cmp(decimal.FromInt(70)) != 0 {
		t.Fatalf("expected remaining cost 70, got %s", lots.Cost)
	}
	if lots.Size.Cmp(four) != 0 {
		t.Fatalf("expected remaining size 4, got %s", lots.Size)
	}
}

func TestLots_SameTime_FIFO(t *testing.T) {
	lots := NewLots(CostBasisMethodFIFO)
	// Add lots at same time - should use insertion order
	lots.Add(clocky.Time(100), one, decimal.FromInt(10))
	lots.Add(clocky.Time(100), one, decimal.FromInt(20))
	lots.Add(clocky.Time(100), one, decimal.FromInt(30))

	// Should consume in insertion order
	cost := lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(10)) != 0 {
		t.Fatalf("expected 10 (first inserted), got %s", cost)
	}

	cost = lots.Consume(one, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("expected 20 (second inserted), got %s", cost)
	}
}

func TestLots_SamePrice_HIFO(t *testing.T) {
	lots := NewLots(CostBasisMethodHIFO)
	// Add lots with same cost-per-unit - should use insertion order
	lots.Add(clocky.Time(100), two, decimal.FromInt(20))  // $10/unit, first
	lots.Add(clocky.Time(200), two, decimal.FromInt(20))  // $10/unit, second

	// Should consume first inserted when prices equal
	cost := lots.Consume(two, zero)
	if cost.Cmp(decimal.FromInt(20)) != 0 {
		t.Fatalf("expected 20, got %s", cost)
	}

	// Verify we consumed from the first lot by checking remaining size
	if lots.Size.Cmp(two) != 0 {
		t.Fatalf("expected size 2, got %s", lots.Size)
	}
}
