package main

import (
	"sort"
	"time"

	"dropbear/broker/databento"
	"dropbear/decimal"
)

type defBuilder struct {
	instruments map[uint32]optionInfo
	strikeMap   map[decimal.Decimal]*strikeLevel
	minTick     decimal.Decimal
}

func newDefBuilder() *defBuilder {
	return &defBuilder{
		instruments: make(map[uint32]optionInfo),
		strikeMap:   make(map[decimal.Decimal]*strikeLevel),
	}
}

func (db *defBuilder) addInstrument(inst *databento.Instrument, queryDateInt int) bool {
	var class byte
	switch inst.InstrumentClass {
	case databento.InstrumentClassCall:
		class = 'C'
	case databento.InstrumentClassPut:
		class = 'P'
	default:
		return false
	}
	expUTC := time.Unix(int64(inst.Expiration)/1e9, int64(inst.Expiration)%1e9).UTC()
	expDateInt := expUTC.Year()*10000 + int(expUTC.Month())*100 + expUTC.Day()
	if expDateInt != queryDateInt {
		return false
	}
	if inst.StrikePrice == databento.UndefPrice {
		return false
	}
	strike := dbnPrice(inst.StrikePrice)
	db.instruments[inst.Header.InstrumentID] = optionInfo{strike: strike, class: class, symbol: inst.GetRawSymbol()}
	sl, ok := db.strikeMap[strike]
	if !ok {
		sl = &strikeLevel{strike: strike}
		db.strikeMap[strike] = sl
	}
	if class == 'C' {
		sl.callID = inst.Header.InstrumentID
	} else {
		sl.putID = inst.Header.InstrumentID
	}
	if db.minTick.IsZero() && inst.MinPriceIncrement != databento.UndefPrice {
		db.minTick = dbnPrice(inst.MinPriceIncrement)
	}
	return true
}

func (db *defBuilder) addInstrumentFromOSI(id uint32, osi string, queryDateInt int) bool {
	strike, class, expVal, err := parseOSI(osi)
	if err != nil || expVal != queryDateInt {
		return false
	}
	db.instruments[id] = optionInfo{strike: strike, class: class, symbol: osi}
	sl, ok := db.strikeMap[strike]
	if !ok {
		sl = &strikeLevel{strike: strike}
		db.strikeMap[strike] = sl
	}
	if class == 'C' {
		sl.callID = id
	} else {
		sl.putID = id
	}
	return true
}

func (db *defBuilder) buildPairs(widthInt int) []boxPair {
	width := decimal.FromInt(widthInt)
	var strikes []decimal.Decimal
	for s := range db.strikeMap {
		strikes = append(strikes, s)
	}
	sort.Slice(strikes, func(i, j int) bool { return strikes[i].Cmp(strikes[j]) < 0 })
	var pairs []boxPair
	for _, s1 := range strikes {
		s2 := s1.Add(width)
		sl1 := db.strikeMap[s1]
		sl2, ok := db.strikeMap[s2]
		if !ok {
			continue
		}
		if sl1.callID != 0 && sl1.putID != 0 && sl2.callID != 0 && sl2.putID != 0 {
			pairs = append(pairs, boxPair{loStrike: s1, hiStrike: s2, callLoID: sl1.callID, callHiID: sl2.callID, putLoID: sl1.putID, putHiID: sl2.putID})
		}
	}
	return pairs
}
