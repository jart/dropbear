package main

import (
	"log"
)

func onHeartbeat() {
	logUnfilledBoxes()
}

func logUnfilledBoxes() {
	for it := gPendingBoxes.Iterator(); it.Next(); {
		box := it.Value()
		log.Printf("incomplete %s", box)
	}
}
