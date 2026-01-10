package databento

import (
	"testing"
	"unsafe"
)

func TestRecordSizes(t *testing.T) {
	tests := []struct {
		name string
		size uintptr
		want uintptr
	}{
		{"RecordHeader", unsafe.Sizeof(RecordHeader{}), 16},
		{"MboMsg", unsafe.Sizeof(MboMsg{}), 56},
		{"BidAskPair", unsafe.Sizeof(BidAskPair{}), 32},
		{"Mbp1Msg", unsafe.Sizeof(Mbp1Msg{}), 80},
		{"Mbp10Msg", unsafe.Sizeof(Mbp10Msg{}), 368},
		{"TradeMsg", unsafe.Sizeof(TradeMsg{}), 48},
		{"OhlcvMsg", unsafe.Sizeof(OhlcvMsg{}), 56},
	}
	for _, tt := range tests {
		if tt.size != tt.want {
			t.Errorf("%s size = %d, want %d", tt.name, tt.size, tt.want)
		}
	}
}

func TestRecordHeaderOffset(t *testing.T) {
	var h RecordHeader
	base := uintptr(unsafe.Pointer(&h))

	tests := []struct {
		name   string
		offset uintptr
		want   uintptr
	}{
		{"Length", uintptr(unsafe.Pointer(&h.Length)) - base, 0},
		{"RType", uintptr(unsafe.Pointer(&h.RType)) - base, 1},
		{"PublisherID", uintptr(unsafe.Pointer(&h.PublisherID)) - base, 2},
		{"InstrumentID", uintptr(unsafe.Pointer(&h.InstrumentID)) - base, 4},
		{"TsEvent", uintptr(unsafe.Pointer(&h.TsEvent)) - base, 8},
	}
	for _, tt := range tests {
		if tt.offset != tt.want {
			t.Errorf("%s offset = %d, want %d", tt.name, tt.offset, tt.want)
		}
	}
}

func TestMboMsgOffset(t *testing.T) {
	var m MboMsg
	base := uintptr(unsafe.Pointer(&m))

	tests := []struct {
		name   string
		offset uintptr
		want   uintptr
	}{
		{"Header", uintptr(unsafe.Pointer(&m.Header)) - base, 0},
		{"OrderID", uintptr(unsafe.Pointer(&m.OrderID)) - base, 16},
		{"Price", uintptr(unsafe.Pointer(&m.Price)) - base, 24},
		{"Size", uintptr(unsafe.Pointer(&m.Size)) - base, 32},
		{"Flags", uintptr(unsafe.Pointer(&m.Flags)) - base, 36},
		{"ChannelID", uintptr(unsafe.Pointer(&m.ChannelID)) - base, 37},
		{"Action", uintptr(unsafe.Pointer(&m.Action)) - base, 38},
		{"Side", uintptr(unsafe.Pointer(&m.Side)) - base, 39},
		{"TsRecv", uintptr(unsafe.Pointer(&m.TsRecv)) - base, 40},
		{"TsInDelta", uintptr(unsafe.Pointer(&m.TsInDelta)) - base, 48},
		{"Sequence", uintptr(unsafe.Pointer(&m.Sequence)) - base, 52},
	}
	for _, tt := range tests {
		if tt.offset != tt.want {
			t.Errorf("%s offset = %d, want %d", tt.name, tt.offset, tt.want)
		}
	}
}
