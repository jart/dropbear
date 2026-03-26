package options

type Mode uint8

const (
	ModeNone Mode = iota
	ModeLong
	ModeShort
)

func (m Mode) String() string {
	switch m {
	case ModeNone:
		return "none"
	case ModeLong:
		return "long"
	case ModeShort:
		return "short"
	default:
		panic("invalid mode")
	}
}

func (m Mode) Invert() Mode {
	switch m {
	case ModeNone:
		return ModeNone
	case ModeLong:
		return ModeShort
	case ModeShort:
		return ModeLong
	default:
		panic("invalid mode")
	}
}
