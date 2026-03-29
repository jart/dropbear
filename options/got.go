package options

type Got uint8

const (
	GotBid    Got = 1 << iota // have a bid price
	GotAsk                    // have an ask price
	GotGreeks                 // have computed greeks
)

func (g Got) String() string {
	var b [3]byte
	if g&GotBid != 0 {
		b[0] = 'B'
	} else {
		b[0] = '-'
	}
	if g&GotAsk != 0 {
		b[1] = 'A'
	} else {
		b[1] = '-'
	}
	if g&GotGreeks != 0 {
		b[2] = 'G'
	} else {
		b[2] = '-'
	}
	return string(b[:])
}
