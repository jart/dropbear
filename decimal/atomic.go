package decimal

import "sync/atomic"

// Store atomically stores v into d.
func (d *Decimal) Store(v Decimal) {
	atomic.StoreInt64((*int64)(d), int64(v))
}

// Load atomically loads and returns the value of d.
func (d *Decimal) Load() Decimal {
	return Decimal(atomic.LoadInt64((*int64)(d)))
}

// AtomicAdd atomically adds v to d.
func (d *Decimal) AtomicAdd(v Decimal) Decimal {
	return Decimal(atomic.AddInt64((*int64)(d), int64(v)))
}

// Swap atomically replaces v into d.
func (d *Decimal) Swap(v Decimal) Decimal {
	return Decimal(atomic.SwapInt64((*int64)(d), int64(v)))
}
