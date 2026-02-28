// Command dump.defs fetches instrument definitions from Databento's historical
// API and prints every field. Useful for inspecting data to verify struct layout.
//
// Usage:
//
//	go run ./broker/databento/cmd/dump.defs -date 2026-02-27
//	go run ./broker/databento/cmd/dump.defs -date 2026-02-27 -dataset OPRA.PILLAR -symbol SPXW.OPT
package main

import (
	"flag"
	"fmt"
	"log"
	"time"
	"unsafe"

	"dropbear/broker/databento"
	"dropbear/clocky"
)

func main() {
	dateStr := flag.String("date", "", "query date in YYYY-MM-DD format (required)")
	dataset := flag.String("dataset", "OPRA.PILLAR", "Databento dataset")
	symbol := flag.String("symbol", "SPXW.OPT", "parent symbol to query")
	flag.Parse()

	if *dateStr == "" {
		log.Fatal("flag -date is required (e.g. -date 2026-02-27)")
	}

	date, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		log.Fatalf("bad date %q: %v", *dateStr, err)
	}
	start := date.Format("2006-01-02") + "T00:00:00Z"
	end := date.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00Z"

	// Query date as YYYYMMDD integer for filtering 0DTE
	queryDateInt := date.Year()*10000 + int(date.Month())*100 + date.Day()

	apiKey, err := databento.GetKey()
	if err != nil {
		log.Fatalf("read API key: %v", err)
	}

	client := databento.NewHistoricalClient(apiKey)
	fmt.Printf("fetching definitions: dataset=%s symbol=%s start=%s end=%s\n", *dataset, *symbol, start, end)

	meta, records, err := client.GetRange(*dataset, databento.SchemaDefinition,
		databento.STypeParent, []string{*symbol}, start, end)
	if err != nil {
		log.Fatalf("GetRange: %v", err)
	}

	fmt.Printf("\nmetadata:\n")
	fmt.Printf("  version:        %d\n", meta.Version)
	fmt.Printf("  dataset:        %s\n", meta.Dataset)
	fmt.Printf("  schema:         %s\n", meta.Schema)
	fmt.Printf("  start:          %s\n", meta.Start)
	fmt.Printf("  end:            %s\n", meta.End)
	fmt.Printf("  limit:          %d\n", meta.Limit)
	fmt.Printf("  stype_in:       %s\n", meta.STypeIn)
	fmt.Printf("  stype_out:      %s\n", meta.STypeOut)
	fmt.Printf("  ts_out:         %v\n", meta.TSOut)
	fmt.Printf("  symbol_cstr_len:%d\n", meta.SymbolCstrLen)
	fmt.Printf("  symbols:        %v\n", meta.Symbols)
	fmt.Printf("  partial:        %v\n", meta.Partial)
	fmt.Printf("  not_found:      %v\n", meta.NotFound)
	fmt.Printf("  mappings:       %d\n", len(meta.Mappings))
	fmt.Printf("  total records:  %d\n", len(records))

	// Filter for 0DTE instruments and print all fields
	printed := 0
	for i, rec := range records {
		if len(rec) < int(unsafe.Sizeof(databento.Instrument{})) {
			fmt.Printf("\nrecord %d: too small (%d bytes, need %d)\n",
				i, len(rec), unsafe.Sizeof(databento.Instrument{}))
			continue
		}
		inst := (*databento.Instrument)(unsafe.Pointer(&rec[0]))

		// Filter: only 0DTE (expiration UTC date matches query date)
		expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
		expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
		if expDateInt != queryDateInt {
			continue
		}

		printed++
		printInstrument(inst)
	}
	fmt.Printf("\n0DTE instruments: %d (of %d total)\n", printed, len(records))
}

func printInstrument(inst *databento.Instrument) {
	fmt.Printf("\n--- %s ---\n", inst.GetRawSymbol())

	// Header
	fmt.Printf("  Header.Length:          %d (%d bytes)\n", inst.Header.Length, int(inst.Header.Length)*4)
	fmt.Printf("  Header.RType:           %s\n", inst.Header.RType)
	fmt.Printf("  Header.PublisherID:     %d\n", inst.Header.PublisherID)
	fmt.Printf("  Header.InstrumentID:    %d\n", inst.Header.InstrumentID)
	fmt.Printf("  Header.TSEvent:         %s\n", fmtTime(inst.Header.TSEvent))

	// Timestamps
	fmt.Printf("  TSRecv:                 %s\n", fmtTime(inst.TSRecv))
	fmt.Printf("  Expiration:             %s\n", fmtTime(inst.Expiration))
	fmt.Printf("  Activation:             %s\n", fmtTime(inst.Activation))

	// Fixed-point prices (divide by FixedPriceScale to get decimal)
	fmt.Printf("  MinPriceIncrement:      %s\n", formatFixedPrice(inst.MinPriceIncrement))
	fmt.Printf("  DisplayFactor:          %s\n", formatFixedPrice(inst.DisplayFactor))
	fmt.Printf("  HighLimitPrice:         %s\n", formatFixedPrice(inst.HighLimitPrice))
	fmt.Printf("  LowLimitPrice:          %s\n", formatFixedPrice(inst.LowLimitPrice))
	fmt.Printf("  MaxPriceVariation:      %s\n", formatFixedPrice(inst.MaxPriceVariation))
	fmt.Printf("  UnitOfMeasureQty:       %s\n", formatFixedPrice(inst.UnitOfMeasureQty))
	fmt.Printf("  MinPriceIncrementAmount:%s\n", formatFixedPrice(inst.MinPriceIncrementAmount))
	fmt.Printf("  PriceRatio:             %s\n", formatFixedPrice(inst.PriceRatio))
	fmt.Printf("  StrikePrice:            %s\n", formatFixedPrice(inst.StrikePrice))
	fmt.Printf("  LegPrice:               %s\n", formatFixedPrice(inst.LegPrice))
	fmt.Printf("  LegDelta:               %s\n", formatFixedPrice(inst.LegDelta))

	// Integer fields
	fmt.Printf("  RawInstrumentID:        %d\n", inst.RawInstrumentID)
	fmt.Printf("  InstAttribValue:        %s\n", fmtInt32(inst.InstAttribValue))
	fmt.Printf("  UnderlyingID:           %s\n", fmtUint32(inst.UnderlyingID))
	fmt.Printf("  MarketDepthImplied:     %s\n", fmtInt32(inst.MarketDepthImplied))
	fmt.Printf("  MarketDepth:            %s\n", fmtInt32(inst.MarketDepth))
	fmt.Printf("  MarketSegmentID:        %s\n", fmtUint32(inst.MarketSegmentID))
	fmt.Printf("  MaxTradeVol:            %s\n", fmtUint32(inst.MaxTradeVol))
	fmt.Printf("  MinLotSize:             %s\n", fmtInt32(inst.MinLotSize))
	fmt.Printf("  MinLotSizeBlock:        %s\n", fmtInt32(inst.MinLotSizeBlock))
	fmt.Printf("  MinLotSizeRoundLot:     %s\n", fmtInt32(inst.MinLotSizeRoundLot))
	fmt.Printf("  MinTradeVol:            %s\n", fmtUint32(inst.MinTradeVol))
	fmt.Printf("  ContractMultiplier:     %s\n", fmtInt32(inst.ContractMultiplier))
	fmt.Printf("  DecayQuantity:          %s\n", fmtInt32(inst.DecayQuantity))
	fmt.Printf("  OriginalContractSize:   %s\n", fmtInt32(inst.OriginalContractSize))
	fmt.Printf("  LegInstrumentID:        %s\n", fmtUint32(inst.LegInstrumentID))
	fmt.Printf("  LegRatioPriceNumerator: %s\n", fmtInt32(inst.LegRatioPriceNumerator))
	fmt.Printf("  LegRatioPriceDenominator:%s\n", fmtInt32(inst.LegRatioPriceDenominator))
	fmt.Printf("  LegRatioQtyNumerator:   %s\n", fmtInt32(inst.LegRatioQtyNumerator))
	fmt.Printf("  LegRatioQtyDenominator: %s\n", fmtInt32(inst.LegRatioQtyDenominator))
	fmt.Printf("  LegUnderlyingID:        %s\n", fmtUint32(inst.LegUnderlyingID))

	// Small integer fields
	fmt.Printf("  ApplID:                 %d\n", inst.ApplID)
	fmt.Printf("  MaturityYear:           %d\n", inst.MaturityYear)
	fmt.Printf("  DecayStartDate:         %d\n", inst.DecayStartDate)
	fmt.Printf("  ChannelID:              %d\n", inst.ChannelID)
	fmt.Printf("  LegCount:               %d\n", inst.LegCount)
	fmt.Printf("  LegIndex:               %d\n", inst.LegIndex)

	// String fields
	fmt.Printf("  Currency:               %s\n", inst.GetCurrency())
	fmt.Printf("  SettlCurrency:          %s\n", convertBytesToString(inst.SettlCurrency[:]))
	fmt.Printf("  SecSubType:             %s\n", convertBytesToString(inst.SecSubType[:]))
	fmt.Printf("  RawSymbol:              %s\n", inst.GetRawSymbol())
	fmt.Printf("  Group:                  %s\n", inst.GetGroup())
	fmt.Printf("  Exchange:               %s\n", inst.GetExchange())
	fmt.Printf("  Asset:                  %s\n", inst.GetAsset())
	fmt.Printf("  CFI:                    %s\n", convertBytesToString(inst.CFI[:]))
	fmt.Printf("  SecurityType:           %s\n", inst.GetSecurityType())
	fmt.Printf("  UnitOfMeasure:          %s\n", convertBytesToString(inst.UnitOfMeasure[:]))
	fmt.Printf("  Underlying:             %s\n", convertBytesToString(inst.Underlying[:]))
	fmt.Printf("  StrikePriceCurrency:    %s\n", convertBytesToString(inst.StrikePriceCurrency[:]))
	fmt.Printf("  LegRawSymbol:           %s\n", convertBytesToString(inst.LegRawSymbol[:]))

	// Enum fields
	fmt.Printf("  InstrumentClass:        %s\n", inst.InstrumentClass)
	fmt.Printf("  MatchAlgorithm:         %s\n", inst.MatchAlgorithm)
	fmt.Printf("  SecurityUpdateAction:   %s\n", inst.SecurityUpdateAction)
	fmt.Printf("  UserDefinedInstrument:  %s\n", inst.UserDefinedInstrument)
	fmt.Printf("  LegInstrumentClass:     %s\n", inst.LegInstrumentClass)
	fmt.Printf("  LegSide:                %s\n", inst.LegSide)

	// Byte fields
	fmt.Printf("  MainFraction:           %d\n", inst.MainFraction)
	fmt.Printf("  PriceDisplayFormat:     %d\n", inst.PriceDisplayFormat)
	fmt.Printf("  SubFraction:            %d\n", inst.SubFraction)
	fmt.Printf("  UnderlyingProduct:      %d\n", inst.UnderlyingProduct)
	fmt.Printf("  MaturityMonth:          %d\n", inst.MaturityMonth)
	fmt.Printf("  MaturityDay:            %d\n", inst.MaturityDay)
	fmt.Printf("  MaturityWeek:           %d\n", inst.MaturityWeek)
	fmt.Printf("  ContractMultiplierUnit: %d\n", inst.ContractMultiplierUnit)
	fmt.Printf("  FlowScheduleType:       %d\n", inst.FlowScheduleType)
	fmt.Printf("  TickRule:               %d\n", inst.TickRule)
}

func fmtTime(t clocky.Time) string {
	if uint64(t) == databento.UndefTimestamp {
		return "UNDEF"
	}
	return t.String()
}

func fmtInt32(v int32) string {
	if v == databento.UndefInt32 {
		return "UNDEF"
	}
	return fmt.Sprintf("%d", v)
}

func fmtUint32(v uint32) string {
	if v == databento.UndefUint32 {
		return "UNDEF"
	}
	return fmt.Sprintf("%d", v)
}

func formatFixedPrice(p int64) string {
	if p == databento.UndefPrice {
		return "UNDEF"
	}
	whole := p / databento.FixedPriceScale
	frac := p % databento.FixedPriceScale
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%09d", whole, frac)
}

func convertBytesToString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
