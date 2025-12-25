package teddy

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
)

// GenerateOrderID generates a unique order ID.
func GenerateOrderID() string {
	return uuid.New().String()
}

// Stagger sleeps for a short duration.
// This is used to stagger API polling inner loops.
func Stagger() {
	time.Sleep(time.Duration(rand.Intn(150)+150) * time.Millisecond)
}

// Slumber sleeps for a random duration.
// This is used to stagger API polling loops.
func Slumber() {
	time.Sleep(time.Duration(rand.Intn(15)+15) * time.Second)
}

// Hibernate sleeps for a very long time.
// This is used to stagger API polling loops.
func Hibernate() {
	time.Sleep(time.Duration(rand.Intn(15)+15) * time.Minute)
}
