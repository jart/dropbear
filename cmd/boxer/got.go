package main

type Got uint8

const (
	GotBid    Got = 1 << iota // have a bid price
	GotAsk                    // have an ask price
	GotGreeks                 // have computed greeks
)
