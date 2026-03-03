package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"errors"
)

/*
╔──────────────────────────────────────────────────────────────────────────────╗
│ OSI (Options Symbology Initiative) Symbol Format                             │
│                                                                              │
│   SSSSSSYYMMDDCPPPPPPPP                                                      │
│   ├─┬──┘├─┬──┘│├──┬───┘                                                      │
│   │ │   │ │   ││  │                                                          │
│   │ │   │ │   ││  └─ strike price × 1000, zero-padded                        │
│   │ │   │ │   │└──── C = call, P = put                                       │
│   │ │   │ │   └───── expiration day                                          │
│   │ │   │ └───────── expiration month                                        │
│   │ │   └─────────── expiration year (two digit)                             │
│   │ └─────────────── underlying symbol, left-justified, space-padded to 6    │
│   │                                                                          │
│   Examples:                                                                  │
│                                                                              │
│     AAPL  260417C00200000  AAPL $200   call exp 2026-04-17                   │
│     JPST  260619P00050500  JPST $50.50 put  exp 2026-06-19                   │
│                                                                              │
╚──────────────────────────────────────────────────────────────────────────────╝
*/
func parseOSI(osi string) (sym symbol.Symbol, strike decimal.Decimal, class databento.InstrumentClass, year int, month clocky.Month, day int, err error) {
	if len(osi) != 21 {
		return 0, decimal.Zero, 0, 0, 0, 0, errors.New("bad osi length")
	}
	var s uint64
	var shift uint
	for i := 0; i < 6 && osi[i] != ' '; i++ {
		s |= uint64(osi[i]) << shift
		shift += 8
	}
	sym = symbol.Symbol(s)
	var strikeVal int64
	for i := 13; i < 21; i++ {
		c := osi[i]
		if c < '0' || c > '9' {
			return 0, decimal.Zero, 0, 0, 0, 0, errors.New("bad osi strike digit")
		}
		strikeVal = strikeVal*10 + int64(c-'0')
	}
	strike = decimal.FromInt64(strikeVal).DivInt(1000)
	class = databento.InstrumentClass(osi[12])
	year = int(osi[6]-'0')*10 + int(osi[7]-'0') + 2000
	month = clocky.Month(int(osi[8]-'0')*10 + int(osi[9]-'0'))
	day = int(osi[10]-'0')*10 + int(osi[11]-'0')
	return sym, strike, class, year, month, day, nil
}
