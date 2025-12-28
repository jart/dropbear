package cubby

import (
	"dropbear/decimal"
	"math/rand"
	"time"

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
	time.Sleep(time.Duration(rand.Intn(150)+150) * time.Millisecond)
}

// Slumber sleeps for a random duration.
func Slumber() {
	time.Sleep(time.Duration(rand.Intn(15)+15) * time.Second)
}

// Hibernate sleeps for a very long time.
func Hibernate() {
	time.Sleep(time.Duration(rand.Intn(15)+15) * time.Minute)
}
