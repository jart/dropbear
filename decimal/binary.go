package decimal

import "encoding/binary"

func (d Decimal) Encode(b []byte) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(d))
}

func (d *Decimal) Decode(b []byte) []byte {
	*d = Decimal(int64(binary.LittleEndian.Uint64(b)))
	return b[8:]
}
