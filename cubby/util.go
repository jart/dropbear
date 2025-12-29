package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"math/rand"

	"github.com/google/uuid"
)

// add atomically adds to a decimal.
func add(d *decimal.Decimal, v decimal.Decimal) {
	d.Store(d.Load().Add(v))
}

// sub atomically subtracts from a decimal.
func sub(d *decimal.Decimal, v decimal.Decimal) {
	d.Store(d.Load().Sub(v))
}

// GenerateOrderID generates a unique order ID.
func GenerateOrderID() string {
	return uuid.New().String()
}

// Stagger sleeps for a short duration.
func Stagger() {
	clocky.Sleep(clocky.Duration(rand.Intn(150)+150) * clocky.Millisecond)
}

// Slumber sleeps for a random duration.
func Slumber() {
	clocky.Sleep(clocky.Duration(rand.Intn(15)+15) * clocky.Second)
}

// Hibernate sleeps for a very long time.
func Hibernate() {
	clocky.Sleep(clocky.Duration(rand.Intn(15)+15) * clocky.Minute)
}
