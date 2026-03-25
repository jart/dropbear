package options

type Mode int8

const (
	ModeUnrestricted Mode = iota
	ModeBuyOnly
	ModeSellOnly
)

func (m Mode) String() string {
	switch m {
	case ModeBuyOnly:
		return "buy-only"
	case ModeSellOnly:
		return "sell-only"
	default:
		return "unrestricted"
	}
}

func (m Mode) Flip() Mode {
	switch m {
	case ModeBuyOnly:
		return ModeSellOnly
	case ModeSellOnly:
		return ModeBuyOnly
	default:
		return ModeUnrestricted
	}
}
