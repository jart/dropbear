package databento

import (
	"os"
	"testing"
)

func TestMboFileMmap(t *testing.T) {
	path := os.ExpandEnv("$HOME/databento/GOOG/2025-01-06.mbo")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("file not found: %s (run download first)", path)
	}

	mbo, err := OpenMboFile(path)
	if err != nil {
		t.Fatalf("OpenMboFile: %v", err)
	}
	defer mbo.Close()

	t.Logf("Loaded %d MBO records", mbo.Len())

	// Print first few records
	for i := 0; i < min(10, mbo.Len()); i++ {
		r := &mbo.Records[i]
		t.Logf("Record %d: ts=%d action=%c side=%c price=%.2f size=%d order=%d",
			i, r.Header.TsEvent, r.Action, r.Side,
			float64(r.Price)/PriceScale, r.Size, r.OrderID)
	}

	// Print some trade records
	trades := 0
	for i := 0; i < mbo.Len() && trades < 10; i++ {
		r := &mbo.Records[i]
		if r.Action == ActionTrade {
			t.Logf("Trade %d: ts=%d price=%.2f size=%d",
				trades, r.Header.TsEvent, float64(r.Price)/PriceScale, r.Size)
			trades++
		}
	}

	// Random access test - jump to middle
	mid := mbo.Len() / 2
	r := &mbo.Records[mid]
	t.Logf("Middle record %d: action=%c price=%.2f", mid, r.Action, float64(r.Price)/PriceScale)

	// Last record
	last := &mbo.Records[mbo.Len()-1]
	t.Logf("Last record: action=%c price=%.2f", last.Action, float64(last.Price)/PriceScale)
}

func BenchmarkMboRandomAccess(b *testing.B) {
	path := os.ExpandEnv("$HOME/databento/GOOG/2025-01-06.mbo")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		b.Skipf("file not found: %s", path)
	}

	mbo, err := OpenMboFile(path)
	if err != nil {
		b.Fatalf("OpenMboFile: %v", err)
	}
	defer mbo.Close()

	n := mbo.Len()
	b.ResetTimer()

	var sum int64
	for i := 0; i < b.N; i++ {
		idx := i % n
		sum += mbo.Records[idx].Price
	}
	b.Log(sum) // prevent optimization
}
