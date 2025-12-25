package ds

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"errors"

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
	set     *treeset.Set[Lot]
	nextSeq int
}

// minutesPerYear is used for time-value calculations.
const minutesPerYear = 365 * 24 * 60

// ErrInsufficientLots is returned when trying to consume more than available.
var ErrInsufficientLots = errors.New("insufficient lots to consume")

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
	lot := Lot{
		Time: time,
		Size: size,
		Cost: cost,
	}
	lot.seq = ls.nextSeq
	ls.nextSeq++
	ls.set.Add(lot)
}

// Empty returns true if there are no lots.
func (ls *Lots) Empty() bool {
	return ls.set.Empty()
}

// Size returns the number of lots.
func (ls *Lots) Size() int {
	return ls.set.Size()
}

// Iterator returns an iterator over all lots in sorted order.
func (ls *Lots) Iterator() treeset.Iterator[Lot] {
	return ls.set.Iterator()
}

// ToSlice returns all lots as a slice in sorted order.
func (ls *Lots) ToSlice() []Lot {
	return ls.set.Values()
}

// PeekTopCostPerUnit returns the cost per unit of the top lot (the one that would
// be sold first), or zero if there are no lots. For LIFO this is the most recent buy.
func (ls *Lots) PeekTopCostPerUnit() decimal.Decimal {
	it := ls.set.Iterator()
	if it.Next() {
		lot := it.Value()
		if lot.Size.IsPositive() {
			return lot.Cost.Div(lot.Size)
		}
	}
	return decimal.Zero
}

// GetCostBasis computes the cost basis for selling a quantity without mutating.
// riskFreeRate is the yearly rate for time-value adjustment.
// Pass decimal.Zero to disable time-value adjustment.
// Returns ErrInsufficientLots if quantity exceeds available inventory.
func (ls *Lots) GetCostBasis(quantity decimal.Decimal, now clocky.Time, riskFreeRate decimal.Decimal) (decimal.Decimal, error) {
	cost := decimal.Zero
	for it := ls.set.Iterator(); quantity.IsPositive() && it.Next(); {
		lot := it.Value()
		adjustedCost := adjustCost(lot, now, riskFreeRate)
		if lot.Size.Cmp(quantity) <= 0 {
			quantity = quantity.Sub(lot.Size)
			cost = cost.Add(adjustedCost)
		} else {
			fraction := quantity.Div(lot.Size)
			cost = cost.Add(adjustedCost.Mul(fraction))
			quantity = decimal.Zero
		}
	}
	if quantity.IsPositive() {
		return cost, ErrInsufficientLots
	}
	return cost, nil
}

// Consume removes lots to cover the given quantity and returns the cost basis.
// riskFreeRate is the yearly rate for time-value adjustment.
// Pass decimal.Zero to disable time-value adjustment.
func (ls *Lots) Consume(quantity decimal.Decimal, now clocky.Time, riskFreeRate decimal.Decimal) (decimal.Decimal, error) {
	cost := decimal.Zero
	var toRemove []Lot
	var remainder *Lot
	for it := ls.set.Iterator(); quantity.IsPositive() && it.Next(); {
		lot := it.Value()
		adjustedCost := adjustCost(lot, now, riskFreeRate)
		if lot.Size.Cmp(quantity) <= 0 {
			quantity = quantity.Sub(lot.Size)
			cost = cost.Add(adjustedCost)
			toRemove = append(toRemove, lot)
		} else {
			fraction := quantity.Div(lot.Size)
			cost = cost.Add(adjustedCost.Mul(fraction))
			toRemove = append(toRemove, lot)
			remainder = &Lot{
				Time: lot.Time,
				Size: lot.Size.Sub(quantity),
				Cost: lot.Cost.Sub(lot.Cost.Mul(fraction)),
				seq:  lot.seq,
			}
			quantity = decimal.Zero
		}
	}
	if quantity.IsPositive() {
		return cost, ErrInsufficientLots
	}
	for _, lot := range toRemove {
		ls.set.Remove(lot)
	}
	if remainder != nil {
		ls.set.Add(*remainder)
	}
	return cost, nil
}

// adjustCost applies time-value adjustment to a lot's cost.
func adjustCost(lot Lot, now clocky.Time, riskFreeRate decimal.Decimal) decimal.Decimal {
	if riskFreeRate.IsZero() {
		return lot.Cost
	}
	minutes := decimal.FromInt(int((now - lot.Time) / (60 * 1_000_000)))
	exponent := riskFreeRate.Mul(minutes).DivInt(minutesPerYear)
	return lot.Cost.Mul(decimal.One.Add(exponent)) // e^x ≈ 1+x
}

func compareLotsHIFO(a, b Lot) int {
	x := a.Cost.Div(a.Size)
	y := b.Cost.Div(b.Size)
	cmp := y.Cmp(x)
	if cmp != 0 {
		return cmp
	}
	return compareLotsInsertion(a, b)
}

func compareLotsFIFO(a, b Lot) int {
	if a.Time < b.Time {
		return -1
	} else if a.Time > b.Time {
		return +1
	}
	return compareLotsInsertion(a, b)
}

func compareLotsLIFO(a, b Lot) int {
	if a.Time > b.Time {
		return -1
	} else if a.Time < b.Time {
		return +1
	}
	return -compareLotsInsertion(a, b)
}

func compareLotsInsertion(a, b Lot) int {
	if a.seq < b.seq {
		return -1
	} else if a.seq > b.seq {
		return +1
	}
	return 0
}
