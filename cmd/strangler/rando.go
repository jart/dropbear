package main

import (
	"crypto/rand"
	"encoding/binary"
	mathrand "math/rand/v2"
	"sync"
)

var (
	rng     *mathrand.Rand
	rngOnce sync.Once
)

// rando returns a random float64 in [0, 1).
// Deterministic in backtest mode; crypto-seeded in live mode.
func rando() float64 {
	rngOnce.Do(func() {
		if *liveFlag {
			var seed [8]byte
			if _, err := rand.Read(seed[:]); err != nil {
				panic(err)
			}
			rng = mathrand.New(mathrand.NewPCG(binary.LittleEndian.Uint64(seed[:]), 0))
		} else {
			rng = mathrand.New(mathrand.NewPCG(0, 0))
		}
	})
	return rng.Float64()
}
