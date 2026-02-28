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
