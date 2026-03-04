package databento

import "strconv"

type FlagSet uint8

const (
	FlagSetLast              FlagSet = 1 << 7 // indicates last message in event from venue for a given instrument id
	FlagSetTob               FlagSet = 1 << 6 // indicates a top-of-book message, not an individual order
	FlagSetSnapshot          FlagSet = 1 << 5 // indicates the message was sourced from a replay, such as a snapshot server
	FlagSetMBP               FlagSet = 1 << 4 // indicates aggregated price level message, not an individual order
	FlagSetBadTsRecv         FlagSet = 1 << 3 // indicates the `ts_recv` value is inaccurate due to clock issues or packet reordering
	FlagSetMaybeBadBook      FlagSet = 1 << 2 // indicates unrecoverable gap was detected in the channel
	FlagSetPublisherSpecific FlagSet = 1 << 1 // indicates publisher-specific event
)

var flagSetNames = [...]struct {
	bit  FlagSet
	name string
}{
	{FlagSetLast, "FlagSetLast"},
	{FlagSetTob, "FlagSetTob"},
	{FlagSetSnapshot, "FlagSetSnapshot"},
	{FlagSetMBP, "FlagSetMBP"},
	{FlagSetBadTsRecv, "FlagSetBadTsRecv"},
	{FlagSetMaybeBadBook, "FlagSetMaybeBadBook"},
	{FlagSetPublisherSpecific, "FlagSetPublisherSpecific"},
}

func (f FlagSet) String() string {
	if f == 0 {
		return "0"
	}
	var buf []byte
	for _, fn := range flagSetNames {
		if f&fn.bit != 0 {
			if len(buf) > 0 {
				buf = append(buf, '|')
			}
			buf = append(buf, fn.name...)
			f &^= fn.bit
		}
	}
	if f != 0 {
		if len(buf) > 0 {
			buf = append(buf, '|')
		}
		buf = append(buf, '0', 'x')
		buf = strconv.AppendUint(buf, uint64(f), 16)
	}
	return string(buf)
}

func (f FlagSet) GoString() string {
	return f.String()
}
