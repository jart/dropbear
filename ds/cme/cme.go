package cme

import (
	"dropbear/clocky"
	"dropbear/ds/symbol"
	"errors"
)

/*
╔──────────────────────────────────────────────────────────────────────────────╗
│ CME Futures Symbol Format                                                    │
│                                                                              │
│   SS[S]MY                                                                    │
│   ├──┘ ││                                                                    │
│   │    │└─ year code (single digit '0'-'9', decoded to nearest decade)       │
│   │    └── month code (F=Jan G=Feb H=Mar J=Apr K=May M=Jun                   │
│   │                    N=Jul Q=Aug U=Sep V=Oct X=Nov Z=Dec)                  │
│   └──── underlying symbol (2–3 chars, packed little-endian into uint64)      │
│                                                                              │
│   Examples:                                                                  │
│                                                                              │
│     ESH6   E-mini S&P 500  exp 2026-03                                       │
│     CLZ9   Crude Oil WTI   exp 2029-12                                       │
│     NGV7   Natural Gas     exp 2027-10                                       │
│     SR1X6  SOFR 1-Month    exp 2026-11                                       │
│                                                                              │
╚──────────────────────────────────────────────────────────────────────────────╝
*/
func Parse(cme string) (sym symbol.Symbol, year int, month clocky.Month, err error) {
	if len(cme) != 4 && len(cme) != 5 {
		return 0, 0, 0, errors.New("bad cme length")
	}
	yearCode := cme[len(cme)-1]
	if yearCode < '0' || yearCode > '9' {
		return 0, 0, 0, errors.New("bad cme year code")
	}
	yearDigit := int(yearCode - '0')
	thisYear := clocky.Now().Year()
	thisDecade := thisYear / 10 * 10
	nextDecade := thisDecade + 10
	year1 := thisDecade + yearDigit
	year2 := nextDecade + yearDigit
	dist1 := year1 - thisYear
	dist2 := year2 - thisYear
	if dist1 < 0 {
		dist1 = -dist1
	}
	if dist2 < 0 {
		dist2 = -dist2
	}
	if dist1 <= dist2 {
		year = year1
	} else {
		year = year2
	}
	monthCode := cme[len(cme)-2]
	month, ok := Months[monthCode]
	if !ok {
		return 0, 0, 0, errors.New("bad cme month code")
	}
	var s uint64
	var shift uint
	for i := 0; i < len(cme)-2; i++ {
		s |= uint64(cme[i]) << shift
		shift += 8
	}
	sym = symbol.Symbol(s)
	return sym, year, month, nil
}
