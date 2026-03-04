package databento

import (
	"testing"
	"unsafe"
)

func TestInstrument_Size(t *testing.T) {
	if size := unsafe.Sizeof(Instrument{}); size != 520 {
		t.Errorf("expected Instrument size to be 520 bytes, got %d", size)
	}
	if align := unsafe.Alignof(Instrument{}); align != 8 {
		t.Errorf("expected Instrument alignment to be 8 bytes, got %d", align)
	}
}

func TestInstrumentV2_Size(t *testing.T) {
	if size := unsafe.Sizeof(instrumentV2{}); size != 400 {
		t.Errorf("expected InstrumentV2 size to be 400 bytes, got %d", size)
	}
	if align := unsafe.Alignof(instrumentV2{}); align != 8 {
		t.Errorf("expected InstrumentV2 alignment to be 8 bytes, got %d", align)
	}
}

func TestInstrumentV1_Size(t *testing.T) {
	if size := unsafe.Sizeof(instrumentV1{}); size != 360 {
		t.Errorf("expected InstrumentV1 size to be 360 bytes, got %d", size)
	}
	if align := unsafe.Alignof(instrumentV1{}); align != 8 {
		t.Errorf("expected InstrumentV1 alignment to be 8 bytes, got %d", align)
	}
}

func TestCBBO_Size(t *testing.T) {
	if size := unsafe.Sizeof(CBBO{}); size != 80 {
		t.Errorf("expected CBBO size to be 80 bytes, got %d", size)
	}
	if align := unsafe.Alignof(CBBO{}); align != 8 {
		t.Errorf("expected CBBO alignment to be 8 bytes, got %d", align)
	}
}

func TestConsolidatedBidAskPair_Size(t *testing.T) {
	if size := unsafe.Sizeof(ConsolidatedBidAskPair{}); size != 32 {
		t.Errorf("expected ConsolidatedBidAskPair size to be 32 bytes, got %d", size)
	}
}

func TestBidAskPair_Size(t *testing.T) {
	if size := unsafe.Sizeof(BidAskPair{}); size != 32 {
		t.Errorf("expected BidAskPair size to be 32 bytes, got %d", size)
	}
}

func TestMBP1_Size(t *testing.T) {
	if size := unsafe.Sizeof(MBP1{}); size != 80 {
		t.Errorf("expected MBP1 size to be 80 bytes, got %d", size)
	}
}

func TestMBP10_Size(t *testing.T) {
	if size := unsafe.Sizeof(MBP10{}); size != 368 {
		t.Errorf("expected MBP10 size to be 368 bytes, got %d", size)
	}
}

func TestCMBP1_Size(t *testing.T) {
	if size := unsafe.Sizeof(CMBP1{}); size != 80 {
		t.Errorf("expected CMBP1 size to be 80 bytes, got %d", size)
	}
	if align := unsafe.Alignof(CMBP1{}); align != 8 {
		t.Errorf("expected CMBP1 alignment to be 8 bytes, got %d", align)
	}
}

func TestSymbolMappingMsg_Size(t *testing.T) {
	if size := unsafe.Sizeof(SymbolMappingMsg{}); size != 176 {
		t.Errorf("expected SymbolMappingMsgV3 size to be 176 bytes, got %d", size)
	}
	if align := unsafe.Alignof(SymbolMappingMsg{}); align != 8 {
		t.Errorf("expected SymbolMappingMsgV3 alignment to be 8 bytes, got %d", align)
	}
	if size := unsafe.Sizeof(symbolMappingMsgV1{}); size != 80 {
		t.Errorf("expected SymbolMappingMsgV1 size to be 80 bytes, got %d", size)
	}
	if align := unsafe.Alignof(symbolMappingMsgV1{}); align != 8 {
		t.Errorf("expected SymbolMappingMsgV1 alignment to be 8 bytes, got %d", align)
	}
}
