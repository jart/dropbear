package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"fmt"
	"log"
)

func main() {
	instruments, err := databento.GetInstruments(databento.MustLoadDefaultKey(), "OPRA.PILLAR", "SPY.OPT", clocky.Now().Add(-clocky.Day*4))
	if err != nil {
		log.Fatal(err)
	}

	for _, instr := range instruments {
		fmt.Printf("%#v\n", instr)
	}
}
