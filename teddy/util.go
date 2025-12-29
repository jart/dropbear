package teddy

import (
	"dropbear/clocky"
	"math/rand"

	"github.com/google/uuid"
)

// GenerateOrderID generates a unique order ID.
func GenerateOrderID() string {
	return uuid.New().String()
}

// Stagger sleeps for a short duration.
// This is used to stagger API polling inner loops.
func Stagger() {
	clocky.Sleep(clocky.Duration(rand.Intn(150)+150) * clocky.Millisecond)
}

// Slumber sleeps for a random duration.
// This is used to stagger API polling loops.
func Slumber() {
	clocky.Sleep(clocky.Duration(rand.Intn(15)+15) * clocky.Second)
}

// Hibernate sleeps for a very long time.
// This is used to stagger API polling loops.
func Hibernate() {
	clocky.Sleep(clocky.Duration(rand.Intn(15)+15) * clocky.Minute)
}
