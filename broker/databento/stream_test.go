package databento

import (
	"testing"
)

func TestStream_OPRA(t *testing.T) {
	if !*flagLive {
		t.Skip("skipping live test (use -live to enable)")
	}

	client, err := Dial("OPRA.PILLAR", MustLoadDefaultKey())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	err = client.Subscribe(Subscription{
		Schema:  SchemaCMBP1,
		SType:   STypeParent,
		Symbols: []string{"SPXW.OPT"},
	})
	if err != nil {
		t.Fatal(err)
	}

	meta, err := client.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("metadata: version=%d dataset=%s schema=%s symbols=%v",
		meta.Version, meta.Dataset, meta.Schema, meta.Symbols)

	const wantRecords = 10
	got := 0
	for got < wantRecords {
		rec, err := client.Read()
		if err != nil {
			t.Fatalf("NextRecord after %d records: %v", got, err)
		}
		switch r := rec.(type) {
		case *CMBP1:
			t.Logf("CMBP1 instrument=%d bid=%d ask=%d bid_sz=%d ask_sz=%d",
				r.Header.InstrumentID,
				r.Levels[0].BidPx, r.Levels[0].AskPx,
				r.Levels[0].BidSz, r.Levels[0].AskSz)
			got++
		case *Instrument:
			t.Logf("Instrument id=%d symbol=%s class=%v strike=%d expiry=%s",
				r.Header.InstrumentID,
				r.GetRawSymbol(),
				r.InstrumentClass,
				r.StrikePrice,
				r.ExpirationWeird.String())
		case *CBBO:
			t.Logf("CBBO instrument=%d bid=%d ask=%d",
				r.Header.InstrumentID,
				r.Levels[0].BidPx, r.Levels[0].AskPx)
			got++
		case ErrorMsg:
			t.Fatalf("error from gateway: %s", r.Err)
		case SystemMsg:
			t.Logf("system: %s", r.Msg)
		default:
			t.Logf("skipping record")
		}
	}
	t.Logf("successfully read %d CMBP1/CBBO records", got)
}
