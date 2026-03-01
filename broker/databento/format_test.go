package databento

import (
	"strings"
	"testing"

	"dropbear/clocky"
)

func TestPrettyPrintRecordHeader(t *testing.T) {
	h := RecordHeader{
		Length:       20,
		RType:        RTypeInstrumentDef,
		PublisherID:  30,
		InstrumentID: 12345,
		TSEvent:      clocky.MustParseTime("2026-02-27T14:30:00Z"),
	}
	got := PrettyPrint(&h)
	if !strings.Contains(got, "RecordHeader{") {
		t.Errorf("missing struct name in:\n%s", got)
	}
	if !strings.Contains(got, "RTypeInstrumentDef") {
		t.Errorf("missing RType constant in:\n%s", got)
	}
	if !strings.Contains(got, "clocky.MustParseTime") {
		t.Errorf("missing clocky timestamp in:\n%s", got)
	}
}

func TestPrettyPrintCBBO(t *testing.T) {
	c := CBBO{
		Header: RecordHeader{
			Length:       20,
			RType:        RTypeCMBP1S,
			PublisherID:  40,
			InstrumentID: 99999,
			TSEvent:      clocky.MustParseTime("2026-02-27T15:00:00Z"),
		},
		Price:  5975000000000,
		Size:   100,
		Side:   SideAsk,
		Flags:  0,
		TSRecv: clocky.MustParseTime("2026-02-27T15:00:00.001Z"),
		Levels: [1]ConsolidatedBidAskPair{{
			BidPx: 5974500000000,
			AskPx: 5975500000000,
			BidSz: 50,
			AskSz: 30,
			BidPb: 40,
			AskPb: 40,
		}},
	}
	got := PrettyPrint(&c)
	if !strings.Contains(got, "CBBO{") {
		t.Errorf("missing struct name in:\n%s", got)
	}
	if !strings.Contains(got, "SideAsk") {
		t.Errorf("missing Side constant in:\n%s", got)
	}
	if !strings.Contains(got, "ConsolidatedBidAskPair{") {
		t.Errorf("missing nested struct in:\n%s", got)
	}
}

func TestPrettyPrintCMBP1(t *testing.T) {
	c := CMBP1{
		Header: RecordHeader{
			Length:       20,
			RType:        RTypeCMBP1,
			PublisherID:  40,
			InstrumentID: 88888,
			TSEvent:      clocky.MustParseTime("2026-02-27T15:00:00Z"),
		},
		Price:     5975000000000,
		Size:      100,
		Action:    ActionTrade,
		Side:      SideBid,
		Flags:     0,
		TSRecv:    clocky.MustParseTime("2026-02-27T15:00:00.001Z"),
		TSInDelta: 500,
		Levels: [1]ConsolidatedBidAskPair{{
			BidPx: 5974500000000,
			AskPx: 5975500000000,
			BidSz: 50,
			AskSz: 30,
			BidPb: 40,
			AskPb: 40,
		}},
	}
	got := PrettyPrint(&c)
	if !strings.Contains(got, "CMBP1{") {
		t.Errorf("missing struct name in:\n%s", got)
	}
	if !strings.Contains(got, "ActionTrade") {
		t.Errorf("missing Action constant in:\n%s", got)
	}
	if !strings.Contains(got, "SideBid") {
		t.Errorf("missing Side constant in:\n%s", got)
	}
}

func TestPrettyPrintInstrument(t *testing.T) {
	inst := Instrument{
		Header: RecordHeader{
			Length:       130,
			RType:        RTypeInstrumentDef,
			PublisherID:  30,
			InstrumentID: 12345,
			TSEvent:      clocky.MustParseTime("2026-02-27T14:30:00Z"),
		},
		TSRecv:                clocky.MustParseTime("2026-02-27T14:30:00.001Z"),
		MinPriceIncrement:     10000000,
		StrikePrice:           5000000000000,
		Expiration:            clocky.MustParseTime("2026-02-27T21:15:00Z"),
		Activation:            clocky.MustParseTime("2026-02-27T00:00:00Z"),
		HighLimitPrice:        UndefPrice,
		LowLimitPrice:         UndefPrice,
		InstrumentClass:       InstrumentClassCall,
		MatchAlgorithm:        MatchAlgorithmFIFO,
		SecurityUpdateAction:  SecurityUpdateActionAdd,
		UserDefinedInstrument: UserDefinedInstrumentNo,
		InstAttribValue:       UndefInt32,
		UnderlyingID:          UndefUint32,
	}
	copy(inst.RawSymbol[:], "SPXW  260227C05000")
	copy(inst.Currency[:], "USD")
	copy(inst.Exchange[:], "OPRA")
	copy(inst.Asset[:], "SPXW")
	copy(inst.SecurityType[:], "OPT")

	got := PrettyPrint(&inst)
	if !strings.Contains(got, "Instrument{") {
		t.Errorf("missing struct name in:\n%s", got)
	}
	if !strings.Contains(got, "InstrumentClassCall") {
		t.Errorf("missing InstrumentClass constant in:\n%s", got)
	}
	if !strings.Contains(got, "MatchAlgorithmFIFO") {
		t.Errorf("missing MatchAlgorithm constant in:\n%s", got)
	}
	if !strings.Contains(got, "UndefPrice") {
		t.Errorf("missing UndefPrice sentinel in:\n%s", got)
	}
	if !strings.Contains(got, "UndefInt32") {
		t.Errorf("missing UndefInt32 sentinel in:\n%s", got)
	}
	if !strings.Contains(got, "UndefUint32") {
		t.Errorf("missing UndefUint32 sentinel in:\n%s", got)
	}
	if !strings.Contains(got, `"SPXW  260227C05000"`) {
		t.Errorf("missing RawSymbol string in:\n%s", got)
	}
	if !strings.Contains(got, `"USD"`) {
		t.Errorf("missing Currency string in:\n%s", got)
	}
	if !strings.Contains(got, "SecurityUpdateActionAdd") {
		t.Errorf("missing SecurityUpdateAction constant in:\n%s", got)
	}
	if !strings.Contains(got, "UserDefinedInstrumentNo") {
		t.Errorf("missing UserDefinedInstrument constant in:\n%s", got)
	}
}

func TestPrettyPrintUndefTimestamp(t *testing.T) {
	h := RecordHeader{
		Length:  4,
		RType:   RTypeMBP1,
		TSEvent: clocky.Time(-1), // UndefTimestamp as int64
	}
	got := PrettyPrint(&h)
	if !strings.Contains(got, "UndefTimestamp") {
		t.Errorf("missing UndefTimestamp sentinel in:\n%s", got)
	}
}

func TestPrettyPrintMetadata(t *testing.T) {
	m := Metadata{
		Version:       3,
		Dataset:       "OPRA.PILLAR",
		Schema:        SchemaDefinition,
		Start:         clocky.MustParseTime("2026-02-27T00:00:00Z"),
		End:           clocky.MustParseTime("2026-02-28T00:00:00Z"),
		Limit:         0,
		STypeIn:       STypeParent,
		STypeOut:      STypeInstrumentID,
		TSOut:         false,
		SymbolCstrLen: 71,
		Symbols:       []string{"SPXW.OPT"},
		Partial:       []string{},
		NotFound:      []string{},
		Mappings:      make([]SymbolMapping, 17674),
	}
	got := PrettyPrint(&m)
	if !strings.Contains(got, "Metadata{") {
		t.Errorf("missing struct name in:\n%s", got)
	}
	if !strings.Contains(got, "SchemaDefinition") {
		t.Errorf("missing Schema constant in:\n%s", got)
	}
	if !strings.Contains(got, "STypeParent") {
		t.Errorf("missing STypeIn constant in:\n%s", got)
	}
	if !strings.Contains(got, "STypeInstrumentID") {
		t.Errorf("missing STypeOut constant in:\n%s", got)
	}
	if !strings.Contains(got, `"OPRA.PILLAR"`) {
		t.Errorf("missing Dataset string in:\n%s", got)
	}
	if !strings.Contains(got, "17674 Mappings") {
		t.Errorf("missing mappings count in:\n%s", got)
	}
}

func TestPrettyPrintDecimalOutput(t *testing.T) {
	h := RecordHeader{
		Length:       130,
		RType:        RTypeInstrumentDef,
		PublisherID:  30,
		InstrumentID: 12345,
		TSEvent:      clocky.MustParseTime("2026-02-27T14:30:00Z"),
	}
	got := PrettyPrint(&h)
	// Verify decimal output (not hex)
	if strings.Contains(got, "0x") {
		t.Errorf("unexpected hex in output:\n%s", got)
	}
	if !strings.Contains(got, "Length:       130") {
		t.Errorf("missing decimal Length in:\n%s", got)
	}
	if !strings.Contains(got, "PublisherID:  30") {
		t.Errorf("missing decimal PublisherID in:\n%s", got)
	}
	if !strings.Contains(got, "InstrumentID: 12345") {
		t.Errorf("missing decimal InstrumentID in:\n%s", got)
	}
}

func BenchmarkPrettyPrintCBBO(b *testing.B) {
	c := CBBO{
		Header: RecordHeader{
			Length:       20,
			RType:        RTypeCMBP1S,
			PublisherID:  40,
			InstrumentID: 99999,
			TSEvent:      clocky.MustParseTime("2026-02-27T15:00:00Z"),
		},
		Price:  5975000000000,
		Size:   100,
		Side:   SideAsk,
		Flags:  0,
		TSRecv: clocky.MustParseTime("2026-02-27T15:00:00.001Z"),
		Levels: [1]ConsolidatedBidAskPair{{
			BidPx: 5974500000000,
			AskPx: 5975500000000,
			BidSz: 50,
			AskSz: 30,
			BidPb: 40,
			AskPb: 40,
		}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrettyPrint(&c)
	}
}

func BenchmarkPrettyPrintInstrument(b *testing.B) {
	inst := Instrument{
		Header: RecordHeader{
			Length:       130,
			RType:        RTypeInstrumentDef,
			PublisherID:  30,
			InstrumentID: 12345,
			TSEvent:      clocky.MustParseTime("2026-02-27T14:30:00Z"),
		},
		StrikePrice:     5000000000000,
		InstrumentClass: InstrumentClassCall,
		HighLimitPrice:  UndefPrice,
		LowLimitPrice:   UndefPrice,
	}
	copy(inst.RawSymbol[:], "SPXW  260227C05000")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PrettyPrint(&inst)
	}
}
