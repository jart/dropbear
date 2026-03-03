package main

import (
	"time"

	"dropbear/clocky"
	"dropbear/decimal"
)

type optionInfo struct {
	strike decimal.Decimal
	class  byte // 'C' or 'P'
	symbol string
}

type quote struct {
	bid decimal.Decimal
	ask decimal.Decimal
}

type strikeLevel struct {
	strike decimal.Decimal
	callID uint32
	putID  uint32
}

type boxPair struct {
	loStrike, hiStrike decimal.Decimal
	callLoID, callHiID uint32 // C_K1, C_K2
	putLoID, putHiID   uint32 // P_K1, P_K2
}

type pendingOrder struct {
	instID    uint32
	side      byte            // 'B' buy or 'S' sell
	limitPx   decimal.Decimal // live limit price
	postTime  clocky.Time     // when limitPx became effective
	pendingPx decimal.Decimal // new price from modifyOrder
	modifyTS  clocky.Time     // when modifyOrder was called
}

type legSpec struct {
	instID uint32
	side   byte
}

type boxLeg struct {
	instID  uint32
	class   byte // 'C' or 'P'
	strike  decimal.Decimal
	side    byte            // 'B' bought or 'S' sold
	fillPx  decimal.Decimal // per share
	fillTS  clocky.Time
	orderTS clocky.Time
}

type activeBox struct {
	pair    *boxPair
	legs    [4]*boxLeg       // nil until filled
	orders  [4]*pendingOrder // nil when leg is filled
	filled  int
	long    bool
	startTS clocky.Time
}

func (ab *activeBox) pairLeg(i int) uint32 {
	specs := legSpecs(ab.pair, ab.long)
	return specs[i].instID
}

func (ab *activeBox) pairSide(i int) byte {
	specs := legSpecs(ab.pair, ab.long)
	return specs[i].side
}

type fill struct {
	ts     clocky.Time
	instID uint32
	side   byte
	qty    int
	px     decimal.Decimal
	cash   decimal.Decimal
}

type ledger struct {
	fills    []fill
	cash     decimal.Decimal
	position map[uint32]int
}

func (l *ledger) recordFill(ts clocky.Time, id uint32, side byte, px decimal.Decimal) {
	cashFlow := px.MulInt(*multFlag)
	if side == 'B' {
		cashFlow = cashFlow.Neg()
		l.position[id]++
	} else {
		l.position[id]--
	}
	l.cash = l.cash.Add(cashFlow)
	l.fills = append(l.fills, fill{ts, id, side, 1, px, cashFlow})
}

type boxResult struct {
	pair    *boxPair
	long    bool
	filled  int
	debit   decimal.Decimal
	pnl     decimal.Decimal
	startTS clocky.Time
	legs    [4]boxLeg
}

type broker struct {
	lastCall clocky.Time
	rate     int // per second
	limit    int // tokens
	tokens   int
}

func (b *broker) canCall(ts clocky.Time, count int) bool {
	if b.lastCall.IsZero() {
		b.tokens = b.limit
	} else {
		elapsed := time.Duration(ts - b.lastCall)
		b.tokens = min(b.limit, b.tokens+int(elapsed.Seconds()*float64(b.rate)))
	}
	b.lastCall = ts
	if b.tokens >= count {
		b.tokens -= count
		return true
	}
	return false
}

func (b *broker) limitOrder(id uint32, side byte, px decimal.Decimal, ts clocky.Time) *pendingOrder {
	return &pendingOrder{
		instID:   id,
		side:     side,
		limitPx:  px,
		postTime: ts,
	}
}

func (b *broker) modifyOrder(ord *pendingOrder, newPx decimal.Decimal, ts clocky.Time) {
	ord.pendingPx = newPx
	ord.modifyTS = ts
}
