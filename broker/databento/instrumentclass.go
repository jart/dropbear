package databento

import "fmt"

type InstrumentClass byte

const (
	InstrumentClassPut           InstrumentClass = 'P'
	InstrumentClassCall          InstrumentClass = 'C'
	InstrumentClassBond          InstrumentClass = 'B'
	InstrumentClassStock         InstrumentClass = 'K'
	InstrumentClassFuture        InstrumentClass = 'F'
	InstrumentClassFXSpot        InstrumentClass = 'X'
	InstrumentClassMixedSpread   InstrumentClass = 'M'
	InstrumentClassFutureSpread  InstrumentClass = 'S'
	InstrumentClassOptionSpread  InstrumentClass = 'T'
	InstrumentClassCommoditySpot InstrumentClass = 'Y'
)

func (ic InstrumentClass) String() string {
	switch ic {
	case 0:
		return "0"
	case InstrumentClassPut:
		return "put"
	case InstrumentClassCall:
		return "call"
	case InstrumentClassBond:
		return "bond"
	case InstrumentClassFuture:
		return "future"
	case InstrumentClassStock:
		return "stock"
	case InstrumentClassMixedSpread:
		return "mixed spread"
	case InstrumentClassFutureSpread:
		return "future spread"
	case InstrumentClassOptionSpread:
		return "option spread"
	case InstrumentClassFXSpot:
		return "fx spot"
	case InstrumentClassCommoditySpot:
		return "commodity spot"
	default:
		return fmt.Sprintf("InstrumentClass(%d)", ic)
	}
}

func (ic InstrumentClass) GoString() string {
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
