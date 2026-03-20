package main

type Got uint8

const (
	GotBid   Got = 1 << iota // have a bid price
	GotAsk                   // have an ask price
	GotDelta                 // have a computed delta
	GotES                    // have a computed ES price
)
