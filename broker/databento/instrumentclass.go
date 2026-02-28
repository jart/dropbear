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
	case InstrumentClassBond:
		return "bond"
	case InstrumentClassCall:
		return "call"
	case InstrumentClassFuture:
		return "future"
	case InstrumentClassStock:
		return "stock"
	case InstrumentClassMixedSpread:
		return "mixed spread"
	case InstrumentClassPut:
		return "put"
	case InstrumentClassFutureSpread:
		return "future spread"
	case InstrumentClassOptionSpread:
		return "option spread"
	case InstrumentClassFXSpot:
		return "fx spot"
	case InstrumentClassCommoditySpot:
		return "commodity spot"
	default:
		return string(ic)
	}
}

func (ic InstrumentClass) GoString() string {
	switch ic {
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
		return fmt.Sprintf("InstrumentClass(0x%02x)", byte(ic))
	}
}
