package databento

import "fmt"

type InstrumentClass byte

const (
	InstrumentClassBond          InstrumentClass = 'B'
	InstrumentClassCall          InstrumentClass = 'C'
	InstrumentClassFuture        InstrumentClass = 'F'
	InstrumentClassStock         InstrumentClass = 'K'
	InstrumentClassMixedSpread   InstrumentClass = 'M'
	InstrumentClassPut           InstrumentClass = 'P'
	InstrumentClassFutureSpread  InstrumentClass = 'S'
	InstrumentClassOptionSpread  InstrumentClass = 'T'
	InstrumentClassFXSpot        InstrumentClass = 'X'
	InstrumentClassCommoditySpot InstrumentClass = 'Y'
)

func (ic InstrumentClass) String() string {
	switch ic {
	case 0:
		return "0"
	case InstrumentClassBond:
		return "InstrumentClassBond"
	case InstrumentClassCall:
		return "InstrumentClassCall"
	case InstrumentClassFuture:
		return "InstrumentClassFuture"
	case InstrumentClassStock:
		return "InstrumentClassStock"
	case InstrumentClassMixedSpread:
		return "InstrumentClassMixedSpread"
	case InstrumentClassPut:
		return "InstrumentClassPut"
	case InstrumentClassFutureSpread:
		return "InstrumentClassFutureSpread"
	case InstrumentClassOptionSpread:
		return "InstrumentClassOptionSpread"
	case InstrumentClassFXSpot:
		return "InstrumentClassFXSpot"
	case InstrumentClassCommoditySpot:
		return "InstrumentClassCommoditySpot"
	default:
		return fmt.Sprintf("InstrumentClass(%d)", ic)
	}
}

func (ic InstrumentClass) GoString() string {
	return ic.String()
}
