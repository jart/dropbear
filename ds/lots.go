package ds

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

// Lot represents a unit of inventory with its cost basis.
type Lot struct {
	Time clocky.Time     // unix microsecond timestamp
	Size decimal.Decimal // base asset amount
	Cost decimal.Decimal // total cost (size * price + commission)
	seq  int             // insertion sequence for stable ordering
}

// Lots manages a collection of lots using a tree set ordered by
// the cost basis method (FIFO, LIFO, or HIFO).
type Lots struct {
	Size decimal.Decimal
	Cost decimal.Decimal
	set  *treeset.Set[*Lot]
	seq  int
}

// NewLots creates a new Lots collection for the given cost basis method.
func NewLots(method CostBasisMethod) *Lots {
	switch method {
	case CostBasisMethodHIFO:
		return &Lots{set: treeset.NewWith(compareLotsHIFO)}
	case CostBasisMethodFIFO:
		return &Lots{set: treeset.NewWith(compareLotsFIFO)}
	case CostBasisMethodLIFO:
		return &Lots{set: treeset.NewWith(compareLotsLIFO)}
	default:
		panic("unknown cost basis method")
	}
}

// Add inserts a lot into the collection.
func (ls *Lots) Add(time clocky.Time, size, cost decimal.Decimal) {
	if !size.IsPositive() {
		loggy.Fatalf("size must be positive, got %s", size)
	}
	if cost.IsNegative() {
		loggy.Fatalf("cost cannot be negative, got %s", cost)
	}
	if ls.Size.IsNegative() {
		debt := ls.Size.Neg()
		if size.Cmp(debt) == 0 {
			ls.Size = decimal.Zero
			return
		} else if size.Cmp(debt) > 0 {
			fraction := debt.DivEven(size)
			cost = cost.Sub(cost.Mul(fraction))
			size = size.Sub(debt)
			ls.Size = decimal.Zero
		} else {
			ls.Size = ls.Size.Add(size)
			return
		}
	}
	ls.set.Add(&Lot{
		Time: time,
		Size: size,
		Cost: cost,
		seq:  ls.seq,
	})
	ls.Size = ls.Size.Add(size)
	ls.Cost = ls.Cost.Add(cost)
	ls.seq++
}

// PeekTopCostPerUnit returns the cost per unit of the top lot (the one that
// would be consumed first), or zero if there are no lots.
func (ls *Lots) PeekTopCostPerUnit() decimal.Decimal {
	it := ls.set.Iterator()
	if it.Next() {
		lot := it.Value()
		return lot.Cost.DivEven(lot.Size)
	}
	return decimal.Zero
}

// GetCostBasis computes the cost basis for selling a quantity without mutating.
func (ls *Lots) GetCostBasis(size, fallbackPrice decimal.Decimal) decimal.Decimal {
	if !size.IsPositive() {
		loggy.Fatalf("size must be positive, got %s", size)
	}
	if fallbackPrice.IsNegative() {
		loggy.Fatalf("fallbackPrice cannot be negative, got %s", fallbackPrice)
	}
	cost := decimal.Zero
	for it := ls.set.Iterator(); size.IsPositive() && it.Next(); {
		lot := it.Value()
		if size.Cmp(lot.Size) >= 0 {
			cost = cost.Add(lot.Cost)
			size = size.Sub(lot.Size)
		} else {
			fraction := size.DivEven(lot.Size)
			cost = cost.Add(lot.Cost.Mul(fraction))
			size = decimal.Zero
		}
	}
	return cost.Add(size.Mul(fallbackPrice))
}

// Consume removes lots to cover the given quantity and returns the cost basis.
func (ls *Lots) Consume(size, fallbackPrice decimal.Decimal) decimal.Decimal {
	if !size.IsPositive() {
		loggy.Fatalf("size must be positive, got %s", size)
	}
	if fallbackPrice.IsNegative() {
		loggy.Fatalf("fallbackPrice cannot be negative, got %s", fallbackPrice)
	}
	cost := decimal.Zero
	for {
		it := ls.set.Iterator()
		if !it.Next() {
			ls.Size = ls.Size.Sub(size)
			return cost.Add(size.Mul(fallbackPrice))
		}
		lot := it.Value()
		if size.Cmp(lot.Size) >= 0 {
			ls.set.Remove(lot)
			size = size.Sub(lot.Size)
			cost = cost.Add(lot.Cost)
			ls.Size = ls.Size.Sub(lot.Size)
			ls.Cost = ls.Cost.Sub(lot.Cost)
		} else {
			fraction := size.DivEven(lot.Size)
			partialCost := lot.Cost.Mul(fraction)
			ls.Size = ls.Size.Sub(size)
			ls.Cost = ls.Cost.Sub(partialCost)
			lot.Size = lot.Size.Sub(size)
			lot.Cost = lot.Cost.Sub(partialCost)
			return cost.Add(partialCost)
		}
	}
}

func compareLotsHIFO(a, b *Lot) int {
	x := a.Cost.DivEven(a.Size)
	y := b.Cost.DivEven(b.Size)
	cmp := y.Cmp(x)
	if cmp != 0 {
		return cmp
	}
	return compareLotsInsertion(a, b)
}

func compareLotsFIFO(a, b *Lot) int {
	if a.Time < b.Time {
		return -1
	} else if a.Time > b.Time {
		return +1
	}
	return compareLotsInsertion(a, b)
}

func compareLotsLIFO(a, b *Lot) int {
	if a.Time > b.Time {
		return -1
	} else if a.Time < b.Time {
		return +1
	}
	return -compareLotsInsertion(a, b)
}

func compareLotsInsertion(a, b *Lot) int {
	if a.seq < b.seq {
		return -1
	} else if a.seq > b.seq {
		return +1
	}
	return 0
}
