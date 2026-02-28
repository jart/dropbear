package databento

type UserDefinedInstrument byte

const (
	UserDefinedInstrumentNo  UserDefinedInstrument = 'N'
	UserDefinedInstrumentYes UserDefinedInstrument = 'Y'
)

func (udi UserDefinedInstrument) String() string {
	switch udi {
	case UserDefinedInstrumentNo:
		return "no"
	case UserDefinedInstrumentYes:
		return "yes"
	default:
		return string(udi)
	}
}

func (udi UserDefinedInstrument) GoString() string {
	switch udi {
	case UserDefinedInstrumentNo:
		return "UserDefinedInstrumentNo"
	case UserDefinedInstrumentYes:
		return "UserDefinedInstrumentYes"
	default:
		return string(udi)
	}
}
