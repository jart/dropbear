package databento

import "fmt"

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
		return fmt.Sprintf("UserDefinedInstrument(%d)", udi)
	}
}

func (udi UserDefinedInstrument) GoString() string {
	switch udi {
	case UserDefinedInstrumentNo:
		return "UserDefinedInstrumentNo"
	case UserDefinedInstrumentYes:
		return "UserDefinedInstrumentYes"
	default:
		return fmt.Sprintf("UserDefinedInstrument(%d)", udi)
	}
}
