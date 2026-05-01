package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/symbol"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var typeOrder = []sip.MessageType{
	0,
	sip.MessageTypeTrade,
	sip.MessageTypeQuote,
	sip.MessageTypeBar,
	sip.MessageTypeDailyBar,
	sip.MessageTypeUpdatedBar,
	sip.MessageTypeStatus,
	sip.MessageTypeLULD,
	sip.MessageTypeImbalance,
}

// Column positions computed from index width.
type cols struct {
	idxW                         int
	time, typ, tape, sym         int
	p1, s1, x1, p2, s2, x2, cond int
}

type viewer struct {
	file       *sip.File
	path       string
	oldState   *term.State
	col        cols
	pos        int // file index of selected message
	scroll     int // file index of first visible area
	visBuf     []int
	symFilter  symbol.Symbol
	typeFilter sip.MessageType
	typeCounts [256]int
	statsReady bool
	searching  bool
	searchBuf  []byte
	detail     bool
	stats      bool
	quit       bool
}

func (v *viewer) hasFilter() bool {
	return v.symFilter != 0 || v.typeFilter != 0
}

func (v *viewer) matchesIdx(i int) bool {
	if !v.hasFilter() {
		return true
	}
	msg, _ := v.file.Get(i)
	if v.symFilter != 0 && msg.Symbol != v.symFilter {
		return false
	}
	if v.typeFilter != 0 && msg.Type != v.typeFilter {
		return false
	}
	return true
}

func (v *viewer) nextMatch(start int) int {
	for i := start; i < v.file.Count(); i++ {
		if v.matchesIdx(i) {
			return i
		}
	}
	return -1
}

func (v *viewer) prevMatch(start int) int {
	for i := start; i >= 0; i-- {
		if v.matchesIdx(i) {
			return i
		}
	}
	return -1
}

// collectVisible gathers up to count matching file indices starting from start.
func (v *viewer) collectVisible(start, count int) []int {
	v.visBuf = v.visBuf[:0]
	if !v.hasFilter() {
		end := start + count
		if end > v.file.Count() {
			end = v.file.Count()
		}
		for i := start; i < end; i++ {
			v.visBuf = append(v.visBuf, i)
		}
	} else {
		for i := start; i < v.file.Count() && len(v.visBuf) < count; i++ {
			if v.matchesIdx(i) {
				v.visBuf = append(v.visBuf, i)
			}
		}
	}
	return v.visBuf
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: sipview FILE.sip\n")
		os.Exit(1)
	}
	path := os.Args[1]
	f, err := sip.OpenFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sipview: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "sipview: stdin is not a terminal\n")
		os.Exit(1)
	}
	v := &viewer{file: f, path: path}
	v.initColumns()
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sipview: %v\n", err)
		os.Exit(1)
	}
	v.oldState = oldState
	os.Stdout.WriteString("\033[?1049h\033[?25l")
	defer func() {
		os.Stdout.WriteString("\033[?25h\033[?1049l")
		term.Restore(int(os.Stdin.Fd()), v.oldState)
	}()
	for !v.quit {
		v.render()
		v.input()
	}
}

func (v *viewer) initColumns() {
	iw := indexWidth(v.file.Count())
	v.col.idxW = iw
	b := 1 + iw + 1
	v.col.time = b
	b += 29 + 1
	v.col.typ = b
	b += 4
	v.col.tape = b
	b += 2
	v.col.sym = b
	b += 9
	v.col.p1 = b
	b += 11
	v.col.s1 = b
	b += 8
	v.col.x1 = b
	b += 7
	v.col.p2 = b
	b += 11
	v.col.s2 = b
	b += 8
	v.col.x2 = b
	b += 7
	v.col.cond = b
}

func (v *viewer) moveDown(n int) {
	if !v.hasFilter() {
		v.pos += n
		if v.pos >= v.file.Count() {
			v.pos = v.file.Count() - 1
		}
		return
	}
	for i := v.pos + 1; i < v.file.Count() && n > 0; i++ {
		if v.matchesIdx(i) {
			v.pos = i
			n--
		}
	}
}

func (v *viewer) moveUp(n int) {
	if !v.hasFilter() {
		v.pos -= n
		if v.pos < 0 {
			v.pos = 0
		}
		return
	}
	for i := v.pos - 1; i >= 0 && n > 0; i-- {
		if v.matchesIdx(i) {
			v.pos = i
			n--
		}
	}
}

func (v *viewer) goTop() {
	if v.hasFilter() {
		if n := v.nextMatch(0); n >= 0 {
			v.pos = n
		}
	} else {
		v.pos = 0
	}
	v.scroll = v.pos
}

func (v *viewer) goBottom() {
	last := v.file.Count() - 1
	if v.hasFilter() {
		if n := v.prevMatch(last); n >= 0 {
			v.pos = n
		}
	} else {
		v.pos = last
	}
}

// ensurePosValid adjusts pos to the nearest matching message after a filter change.
func (v *viewer) ensurePosValid() {
	if !v.hasFilter() || v.matchesIdx(v.pos) {
		return
	}
	if n := v.nextMatch(v.pos); n >= 0 {
		v.pos = n
		return
	}
	if n := v.prevMatch(v.pos); n >= 0 {
		v.pos = n
		return
	}
	v.pos = 0
}

func (v *viewer) visibleRows() int {
	_, h, _ := term.GetSize(int(os.Stdout.Fd()))
	r := h - 2 // column header + status bar
	if r < 1 {
		r = 1
	}
	return r
}

func (v *viewer) clampScroll() {
	if v.scroll > v.pos {
		v.scroll = v.pos
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
}

// --- rendering ---

func (v *viewer) render() {
	w, h, _ := term.GetSize(int(os.Stdout.Fd()))
	if w < 40 || h < 4 {
		return
	}
	v.clampScroll()
	var b bytes.Buffer
	b.Grow(w * h * 2)
	b.WriteString("\033[H")
	if v.stats {
		v.renderStats(&b, w, h)
	} else if v.detail {
		v.renderDetail(&b, w, h)
	} else {
		v.renderList(&b, w, h)
	}
	os.Stdout.Write(b.Bytes())
}

func at(b *bytes.Buffer, col int, s string) {
	fmt.Fprintf(b, "\033[%dG%s", col, s)
}

func (v *viewer) renderList(b *bytes.Buffer, w, h int) {
	c := &v.col
	vis := h - 2

	// Collect visible messages; adjust scroll if pos is off-screen.
	visible := v.collectVisible(v.scroll, vis)
	posRow := -1
	for i, idx := range visible {
		if idx == v.pos {
			posRow = i
			break
		}
	}
	if posRow < 0 {
		// pos is past the visible area — scroll forward so pos is last row.
		v.scroll = v.pos
		count := 0
		for i := v.pos - 1; i >= 0 && count < vis-1; i-- {
			if v.matchesIdx(i) {
				v.scroll = i
				count++
			}
		}
		visible = v.collectVisible(v.scroll, vis)
	}

	// Row 1: column headers.
	b.WriteString("\033[2K\033[90m")
	at(b, 1, fmt.Sprintf("%*s", c.idxW, "#"))
	at(b, c.time, "TIMESTAMP")
	at(b, c.typ, "TYP")
	at(b, c.tape, "T")
	at(b, c.sym, "SYMBOL")
	at(b, c.p1, fmt.Sprintf("%10s", "PRICE"))
	at(b, c.s1, fmt.Sprintf("%7s", "SIZE"))
	at(b, c.x1, "EXCH")
	at(b, c.p2, fmt.Sprintf("%10s", "PRICE"))
	at(b, c.s2, fmt.Sprintf("%7s", "SIZE"))
	at(b, c.x2, "EXCH")
	at(b, c.cond, "COND")
	b.WriteString("\033[0m\r\n")

	// Data rows.
	var prevTime clocky.Time
	for i := 0; i < vis; i++ {
		b.WriteString("\033[2K")
		if i >= len(visible) {
			b.WriteString("\r\n")
			continue
		}
		msgIdx := visible[i]
		msg, _ := v.file.Get(msgIdx)
		sel := msgIdx == v.pos
		outOfOrder := prevTime != 0 && msg.Timestamp < prevTime
		prevTime = msg.Timestamp

		if sel {
			b.WriteString("\033[7m")
		}

		at(b, 1, fmt.Sprintf("%*d", c.idxW, msgIdx))

		if outOfOrder {
			at(b, c.time, "\033[31m"+msg.Timestamp.String()+"\033[39m")
		} else {
			at(b, c.time, msg.Timestamp.String())
		}

		at(b, c.typ, typeColor(msg.Type)+typeName(msg.Type)+"\033[39m")
		at(b, c.tape, string(byte(msg.Tape)))
		at(b, c.sym, fmt.Sprintf("%-8s", msg.Symbol))

		v.renderListColumns(b, msg, msgIdx)

		b.WriteString("\033[0m\r\n")
	}

	v.renderStatusBar(b, w, h)
}

func (v *viewer) renderListColumns(b *bytes.Buffer, msg *sip.Message, fileIdx int) {
	c := &v.col
	switch msg.Type {
	case sip.MessageTypeTrade:
		t := msg.Trade()
		at(b, c.p1, fmt.Sprintf("%10s", t.Price))
		at(b, c.s1, fmt.Sprintf("%7d", t.Size))
		at(b, c.x1, fmt.Sprintf("%-6s", t.Exchange))
		if t.Conditions != 0 {
			at(b, c.cond, fmt.Sprintf("[%s]", t.Conditions))
		}
	case sip.MessageTypeQuote:
		q := msg.Quote()
		at(b, c.p1, fmt.Sprintf("%10s", q.BidPrice))
		at(b, c.s1, fmt.Sprintf("%7d", q.BidSize))
		at(b, c.x1, fmt.Sprintf("%-6s", q.BidExchange))
		at(b, c.p2, fmt.Sprintf("%10s", q.AskPrice))
		at(b, c.s2, fmt.Sprintf("%7d", q.AskSize))
		at(b, c.x2, fmt.Sprintf("%-6s", q.AskExchange))
		if q.Conditions != 0 {
			at(b, c.cond, fmt.Sprintf("[%s]", q.Conditions))
		}
	case sip.MessageTypeBar, sip.MessageTypeDailyBar, sip.MessageTypeUpdatedBar:
		bar := msg.Bar()
		at(b, c.p1, fmt.Sprintf("O:%s H:%s L:%s C:%s V:%d VW:%s N:%d",
			bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.VWAP, bar.NumTrades))
	case sip.MessageTypeStatus:
		st := msg.Status()
		desc := describeStatusCode(st.Code, msg.Tape)
		s := fmt.Sprintf("%c %s", byte(st.Code), desc)
		if st.Halt() {
			s += " [HALT]"
		} else if st.Resume() {
			s += " [RESUME]"
		}
		at(b, c.p1, s)
	case sip.MessageTypeLULD:
		l := msg.LULD()
		at(b, c.p1, fmt.Sprintf("%10s", l.LowerLimit))
		at(b, c.x1, fmt.Sprintf("%-6s", string(byte(l.Indicator))))
		at(b, c.p2, fmt.Sprintf("%10s", l.UpperLimit))
	case sip.MessageTypeImbalance:
		m := msg.Imbalance()
		at(b, c.p1, fmt.Sprintf("%10s", m.Price))
		if delta := v.imbDelta(msg.Symbol, m, fileIdx); delta != "" {
			at(b, c.cond, delta)
		}
	}
}

func (v *viewer) imbDelta(sym symbol.Symbol, imb *sip.Imbalance, fileIdx int) string {
	end := fileIdx - 100000
	if end < 0 {
		end = 0
	}
	for i := fileIdx - 1; i >= end; i-- {
		msg, _ := v.file.Get(i)
		if msg.Type == sip.MessageTypeTrade && msg.Symbol == sym {
			tp := msg.Trade().Price
			if tp.IsPositive() {
				pct := float64(imb.Price.Sub(tp).Int64()) * 100.0 / float64(tp.Int64())
				return fmt.Sprintf("%+.2f%%", pct)
			}
		}
	}
	return ""
}

func (v *viewer) renderStatusBar(b *bytes.Buffer, w, h int) {
	var sb strings.Builder
	sb.WriteString(" ")
	sb.WriteString(filepath.Base(v.path))
	sb.WriteString("  ")
	sb.WriteString(commaInt(v.file.Count()))
	sb.WriteString(" msgs")

	if v.searching {
		sb.Reset()
		sb.WriteString(" /")
		sb.WriteString(string(v.searchBuf))
		sb.WriteString("_")
	} else {
		sb.WriteString("  #")
		sb.WriteString(commaInt(v.pos))
		sb.WriteString("/")
		sb.WriteString(commaInt(v.file.Count()))
		if v.hasFilter() {
			sb.WriteString("  ")
			if v.symFilter != 0 {
				sb.WriteString("sym=")
				sb.WriteString(v.symFilter.String())
				sb.WriteString(" ")
			}
			if v.typeFilter != 0 {
				sb.WriteString("type=")
				sb.WriteString(typeName(v.typeFilter))
			}
		}
		sb.WriteString("  j/k ^D/U g/G /:sym f:type Enter:detail ?:stats Esc:clear q:quit")
	}
	fmt.Fprintf(b, "\033[%d;1H\033[7m%-*s\033[0m", h, w, sb.String())
}

func (v *viewer) renderDetail(b *bytes.Buffer, w, h int) {
	n := v.file.Count()
	maxRow := h - 1
	row := 0
	mid := w/2 + 1
	if mid < 50 {
		mid = 50
	}

	wr := func(left, right string) {
		if row >= maxRow {
			return
		}
		b.WriteString("\033[2K")
		b.WriteString(left)
		fmt.Fprintf(b, "\033[%dG\033[90m│\033[0m", mid)
		if right != "" {
			b.WriteByte(' ')
			b.WriteString(right)
		}
		b.WriteString("\r\n")
		row++
	}

	if v.pos >= 0 && v.pos < n {
		idx := v.pos
		msg, _ := v.file.Get(idx)

		// Build right side: asset info.
		var rightLines []string
		asset := alpaca.GetAsset(msg.Symbol)
		if asset != nil {
			rightLines = append(rightLines,
				fmt.Sprintf("\033[1m%s\033[0m (%s)", asset.Symbol, asset.Name),
				fmt.Sprintf("Exchange:          %s", asset.Exchange),
				fmt.Sprintf("Class:             %s", asset.Class),
				fmt.Sprintf("Status:            %s", asset.Status),
				fmt.Sprintf("Tradable:          %v", asset.Tradable.Load()),
				fmt.Sprintf("Marginable:        %v", asset.Marginable.Load()),
				fmt.Sprintf("Shortable:         %v", asset.Shortable.Load()),
				fmt.Sprintf("Fraction:          %v", asset.Fractionable.Load()),
				fmt.Sprintf("MarginLong:        %s", asset.MarginRequirementLong.Load()),
				fmt.Sprintf("PriceIncr:         %s", asset.PriceIncrement.Load()),
				fmt.Sprintf("IPO:               %v", asset.IPO.Load()),
				fmt.Sprintf("EasyBorrow:        %v", asset.EasyToBorrow.Load()),
				fmt.Sprintf("Options:           %v", asset.HasOptions.Load()),
				fmt.Sprintf("MarginShort:       %s", asset.MarginRequirementShort.Load()),
				fmt.Sprintf("MinTradeIncr:      %s", asset.MinTradeIncrement.Load()),
				fmt.Sprintf("OvernightTradable: %v", asset.OvernightTradable.Load()),
			)
		} else {
			rightLines = append(rightLines, "(asset info not available)")
		}

		rline := func(i int) string {
			if i < len(rightLines) {
				return rightLines[i]
			}
			return ""
		}

		ri := 0 // right-line index

		wr(fmt.Sprintf(" \033[1mMESSAGE #%s\033[0m  (%s msgs)",
			commaInt(idx), commaInt(n)), rline(ri))
		ri++
		wr(fmt.Sprintf("\033[90m %s\033[0m", strings.Repeat("─", min(mid-3, 80))), rline(ri))
		ri++
		wr(fmt.Sprintf(" %-14s %s", "Timestamp:", msg.Timestamp), rline(ri))
		ri++
		wr(fmt.Sprintf(" %-14s %s", "Type:", describeType(msg.Type)), rline(ri))
		ri++
		wr(fmt.Sprintf(" %-14s %c (%s)", "Tape:", byte(msg.Tape), describeTape(msg.Tape)), rline(ri))
		ri++
		wr(fmt.Sprintf(" %-14s %s", "Symbol:", msg.Symbol), rline(ri))
		ri++
		wr("", rline(ri))
		ri++

		switch msg.Type {
		case sip.MessageTypeTrade:
			t := msg.Trade()
			wr(fmt.Sprintf(" %-14s %s (%s)", "Exchange:", t.Exchange, t.Exchange.Code()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Price:", t.Price), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Size:", t.Size), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Trade ID:", t.TradeID), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Conditions:", t.Conditions), rline(ri))
			ri++
			for _, d := range describeTradeConditions(t.Conditions, msg.Tape) {
				wr(d, rline(ri))
				ri++
			}
			wr(fmt.Sprintf(" %-14s %v", "Regular Sale:", t.IsRegularSale()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Odd Lot:", t.IsOddLot()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Ext Hours:", t.IsExtendedHours()), rline(ri))
			ri++

		case sip.MessageTypeQuote:
			q := msg.Quote()
			wr(fmt.Sprintf(" %-14s %s", "Bid Price:", q.BidPrice), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Bid Size:", q.BidSize), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s (%s)", "Bid Exchange:", q.BidExchange, q.BidExchange.Code()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Ask Price:", q.AskPrice), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Ask Size:", q.AskSize), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s (%s)", "Ask Exchange:", q.AskExchange, q.AskExchange.Code()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Spread:", q.Spread()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Midpoint:", q.Midpoint()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Conditions:", q.Conditions), rline(ri))
			ri++
			for _, d := range describeQuoteConditions(q.Conditions, msg.Tape) {
				wr(d, rline(ri))
				ri++
			}
			wr(fmt.Sprintf(" %-14s %v", "Indicative:", q.Indicative()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Danger Bid:", q.DangerousBid()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Danger Ask:", q.DangerousAsk()), rline(ri))
			ri++

		case sip.MessageTypeBar, sip.MessageTypeDailyBar, sip.MessageTypeUpdatedBar:
			bar := msg.Bar()
			wr(fmt.Sprintf(" %-14s %s", "Open:", bar.Open), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "High:", bar.High), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Low:", bar.Low), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Close:", bar.Close), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Volume:", bar.Volume), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "VWAP:", bar.VWAP), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %d", "Num Trades:", bar.NumTrades), rline(ri))
			ri++

		case sip.MessageTypeStatus:
			st := msg.Status()
			wr(fmt.Sprintf(" %-14s %c", "Code:", byte(st.Code)), rline(ri))
			ri++
			wr(fmt.Sprintf("   = %s", describeStatusCode(st.Code, msg.Tape)), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Reason:", st.Reason), rline(ri))
			ri++
			wr(fmt.Sprintf("   = %s", describeReasonCode(st.Reason, msg.Tape)), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Message:", st.Message), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Reason Msg:", st.ReasonMsg), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Halt:", st.Halt()), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %v", "Resume:", st.Resume()), rline(ri))
			ri++

		case sip.MessageTypeLULD:
			l := msg.LULD()
			wr(fmt.Sprintf(" %-14s %c", "Indicator:", byte(l.Indicator)), rline(ri))
			ri++
			wr(fmt.Sprintf("   = %s", describeLULDIndicator(l.Indicator)), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Upper Limit:", l.UpperLimit), rline(ri))
			ri++
			wr(fmt.Sprintf(" %-14s %s", "Lower Limit:", l.LowerLimit), rline(ri))
			ri++

		case sip.MessageTypeImbalance:
			m := msg.Imbalance()
			wr(fmt.Sprintf(" %-14s %s", "Price:", m.Price), rline(ri))
			ri++
			if delta := v.imbDelta(msg.Symbol, m, idx); delta != "" {
				wr(fmt.Sprintf(" %-14s %s", "vs Last Trade:", delta), rline(ri))
				ri++
			}
		}

		// Fill remaining right-side lines.
		for ri < len(rightLines) {
			wr("", rline(ri))
			ri++
		}
	} else {
		wr(" (no message selected)", "")
	}

	for row < maxRow {
		fmt.Fprintf(b, "\033[2K\033[%dG\033[90m│\033[0m\r\n", mid)
		row++
	}
	help := " j/k:prev/next  Enter/Esc:back  q:quit"
	fmt.Fprintf(b, "\033[%d;1H\033[7m%-*s\033[0m", h, w, help)
}

func (v *viewer) renderStats(b *bytes.Buffer, w, h int) {
	if !v.statsReady {
		b.WriteString(" \033[1mFILE STATISTICS\033[0m  scanning...\033[K\r\n")
		os.Stdout.Write(b.Bytes())
		b.Reset()
		b.WriteString("\033[H")
		for i := 0; i < v.file.Count(); i++ {
			msg, _ := v.file.Get(i)
			v.typeCounts[msg.Type]++
		}
		v.statsReady = true
	}

	row := 0
	wr := func(format string, args ...any) {
		if row >= h-1 {
			return
		}
		b.WriteString("\033[2K")
		fmt.Fprintf(b, format, args...)
		b.WriteString("\r\n")
		row++
	}

	total := v.file.Count()
	wr(" \033[1mFILE STATISTICS\033[0m")
	wr("\033[90m %s\033[0m", strings.Repeat("─", min(w-2, 60)))
	wr(" %-14s %s", "File:", filepath.Base(v.path))
	wr(" %-14s %s", "Messages:", commaInt(total))
	if total > 0 {
		firstMsg, _ := v.file.Get(0)
		lastMsg, _ := v.file.Get(total - 1)
		wr(" %-14s %s", "First:", firstMsg.Timestamp)
		wr(" %-14s %s", "Last:", lastMsg.Timestamp)
	}
	wr("")
	wr(" \033[1mMessage Types:\033[0m")
	for _, t := range typeOrder[1:] {
		c := v.typeCounts[t]
		if c > 0 {
			pct := float64(c) * 100.0 / float64(total)
			wr("   %-3s  %13s  (%5.1f%%)", typeName(t), commaInt(c), pct)
		}
	}

	for row < h-1 {
		wr("")
	}
	fmt.Fprintf(b, "\033[%d;1H\033[7m%-*s\033[0m", h, w, " Press any key to return")
}

// --- input handling ---

func (v *viewer) input() {
	buf := make([]byte, 64)
	n, err := os.Stdin.Read(buf)
	if err != nil {
		v.quit = true
		return
	}
	if v.searching {
		v.searchInput(buf[:n])
	} else if v.stats {
		v.stats = false // any key exits
	} else if v.detail {
		v.detailInput(buf[:n])
	} else {
		v.listInput(buf[:n])
	}
}

func (v *viewer) listInput(buf []byte) {
	vis := v.visibleRows()
	for i := 0; i < len(buf); {
		if buf[i] == 0x1b {
			if i+2 < len(buf) && buf[i+1] == '[' {
				switch buf[i+2] {
				case 'A':
					v.moveUp(1)
				case 'B':
					v.moveDown(1)
				case '5':
					if i+3 < len(buf) && buf[i+3] == '~' {
						v.moveUp(vis)
						i += 4
						continue
					}
				case '6':
					if i+3 < len(buf) && buf[i+3] == '~' {
						v.moveDown(vis)
						i += 4
						continue
					}
				}
				i += 3
				continue
			}
			v.clearFilters()
			i++
			continue
		}
		switch buf[i] {
		case 'j':
			v.moveDown(1)
		case 'k':
			v.moveUp(1)
		case 'g':
			v.goTop()
		case 'G':
			v.goBottom()
		case ' ', 4, 6: // Space, Ctrl-D, Ctrl-F
			v.moveDown(vis)
		case 'b', 21, 2: // b, Ctrl-U, Ctrl-B
			v.moveUp(vis)
		case '/':
			v.searching = true
			v.searchBuf = v.searchBuf[:0]
		case 'f':
			v.cycleType(1)
		case 'F':
			v.cycleType(-1)
		case '?':
			v.stats = true
		case 13: // Enter
			v.detail = true
		case 'q', 3:
			v.quit = true
			return
		}
		i++
	}
}

func (v *viewer) detailInput(buf []byte) {
	for i := 0; i < len(buf); {
		if buf[i] == 0x1b {
			if i+2 < len(buf) && buf[i+1] == '[' {
				switch buf[i+2] {
				case 'A':
					v.moveUp(1)
				case 'B':
					v.moveDown(1)
				}
				i += 3
				continue
			}
			v.detail = false
			i++
			continue
		}
		switch buf[i] {
		case 'j':
			v.moveDown(1)
		case 'k':
			v.moveUp(1)
		case 13:
			v.detail = false
		case 'q', 3:
			v.quit = true
			return
		}
		i++
	}
}

func (v *viewer) searchInput(buf []byte) {
	for _, b := range buf {
		switch b {
		case 13:
			v.searching = false
			s := string(v.searchBuf)
			v.searchBuf = v.searchBuf[:0]
			var newFilter symbol.Symbol
			if s != "" {
				sym, err := symbol.Parse(s)
				if err != nil {
					return
				}
				newFilter = sym
			}
			v.symFilter = newFilter
			v.ensurePosValid()
			return
		case 27:
			v.searching = false
			v.searchBuf = v.searchBuf[:0]
			return
		case 127, 8:
			if len(v.searchBuf) > 0 {
				v.searchBuf = v.searchBuf[:len(v.searchBuf)-1]
			}
		default:
			if b >= 32 && b < 127 {
				if b >= 'a' && b <= 'z' {
					b -= 32
				}
				v.searchBuf = append(v.searchBuf, b)
			}
		}
	}
}

func (v *viewer) cycleType(dir int) {
	cur := 0
	for i, t := range typeOrder {
		if t == v.typeFilter {
			cur = i
			break
		}
	}
	cur += dir
	if cur >= len(typeOrder) {
		cur = 0
	}
	if cur < 0 {
		cur = len(typeOrder) - 1
	}
	v.typeFilter = typeOrder[cur]
	v.ensurePosValid()
}

func (v *viewer) clearFilters() {
	if !v.hasFilter() {
		return
	}
	v.symFilter = 0
	v.typeFilter = 0
}

// --- descriptions ---

func describeType(t sip.MessageType) string {
	switch t {
	case sip.MessageTypeTrade:
		return "Trade"
	case sip.MessageTypeQuote:
		return "Quote"
	case sip.MessageTypeBar:
		return "Minute Bar"
	case sip.MessageTypeDailyBar:
		return "Daily Bar"
	case sip.MessageTypeUpdatedBar:
		return "Updated Bar"
	case sip.MessageTypeStatus:
		return "Status"
	case sip.MessageTypeLULD:
		return "LULD (Limit Up-Limit Down)"
	case sip.MessageTypeImbalance:
		return "Imbalance"
	case sip.MessageTypeCorrection:
		return "Correction"
	case sip.MessageTypeCancelError:
		return "Cancel/Error"
	default:
		return fmt.Sprintf("Unknown (%c)", byte(t))
	}
}

func describeTape(t sip.Tape) string {
	switch t {
	case sip.TapeA:
		return "NYSE listed securities"
	case sip.TapeB:
		return "NYSE Arca, BATS, regional"
	case sip.TapeC:
		return "NASDAQ listed securities"
	case sip.TapeN:
		return "Tape N"
	case sip.TapeO:
		return "Tape O"
	default:
		return "Unknown"
	}
}

func describeStatusCode(code sip.StatusCode, tape sip.Tape) string {
	switch tape {
	case sip.TapeA, sip.TapeB:
		switch code {
		case sip.StatusCodeTradingHaltCTA:
			return "Trading Halt"
		case sip.StatusCodeResumeCTA:
			return "Resume"
		case sip.StatusCodePriceIndication:
			return "Price Indication"
		case sip.StatusCodeTradingRangeIndication:
			return "Trading Range Indication"
		case sip.StatusCodeMarketImbalanceBuy:
			return "Market Imbalance Buy (50K+ shares)"
		case sip.StatusCodeMarketImbalanceSell:
			return "Market Imbalance Sell (50K+ shares)"
		case sip.StatusCodeMOCImbalanceBuy:
			return "Market On Close Imbalance Buy"
		case sip.StatusCodeMOCImbalanceSell:
			return "Market On Close Imbalance Sell"
		case sip.StatusCodeNoMarketImbalance:
			return "No Market Imbalance"
		case sip.StatusCodeNoMOCImbalance:
			return "No Market On Close Imbalance"
		case sip.StatusCodeShortSaleRestriction:
			return "Short Sale Restriction"
		case sip.StatusCodeLimitUpLimitDown:
			return "Limit Up-Limit Down"
		}
	case sip.TapeC, sip.TapeO:
		switch code {
		case sip.StatusCodeTradingHaltUTP:
			return "Trading Halt"
		case sip.StatusCodeVolatilityPause:
			return "Volatility Trading Pause"
		case sip.StatusCodeQuotationResumption:
			return "Quotation Resumption"
		case sip.StatusCodeTradingResumptionUTP:
			return "Trading Resumption"
		}
	}
	return fmt.Sprintf("Unknown (tape=%c code='%c')", byte(tape), byte(code))
}

func describeReasonCode(reason sip.ReasonCode, _ sip.Tape) string {
	switch reason {
	case 0:
		return "(none)"
	case sip.ReasonCodeNewsReleased:
		return "News Released"
	case sip.ReasonCodeOrderImb:
		return "Order Imbalance"
	case sip.ReasonCodeLULD:
		return "LULD Trading Pause"
	case sip.ReasonCodeNewsPending:
		return "News Pending"
	case sip.ReasonCodeOperational:
		return "Operational"
	case sip.ReasonCodeSubPenny:
		return "Sub-Penny Trading"
	case sip.ReasonCodeCircuitLvl1:
		return "Circuit Breaker Level 1 (S&P 500 -7%)"
	case sip.ReasonCodeCircuitLvl2:
		return "Circuit Breaker Level 2 (S&P 500 -13%)"
	case sip.ReasonCodeCircuitLvl3:
		return "Circuit Breaker Level 3 (S&P 500 -20%)"
	case sip.ReasonCodeHaltNewsPending:
		return "Halt News Pending"
	case sip.ReasonCodeHaltNewsDissem:
		return "Halt News Dissemination"
	case sip.ReasonCodeHaltSECOrderSIP:
		return "Single Stock Trading Pause"
	case sip.ReasonCodeLULDPause:
		return "LULD Pause"
	case sip.ReasonCodeIPONotYetTrading:
		return "IPO Not Yet Trading"
	case sip.ReasonCodeCorporateAction:
		return "Corporate Action"
	case sip.ReasonCodeMWCQ:
		return "Circuit Breaker Resumption"
	case sip.ReasonCodeMWC1:
		return "Circuit Breaker Level 1 (S&P 500 -7%)"
	case sip.ReasonCodeMWC2:
		return "Circuit Breaker Level 2 (S&P 500 -13%)"
	case sip.ReasonCodeMWC3:
		return "Circuit Breaker Level 3 (S&P 500 -20%)"
	}
	return reason.String()
}

type tcDesc struct {
	c    sip.TradeCond
	code string
	cta  string
	utp  string
}

var tradeCondTable = []tcDesc{
	{sip.TradeCondRegularSaleCTA, " ", "Regular Sale", "Regular Sale"},
	{sip.TradeCondRegularSale, "@", "Regular Sale", "Regular Sale"},
	{sip.TradeCondStoppedStock, "1", "Stopped Stock", "Stopped Stock"},
	{sip.TradeCondDerivativelyPriced, "4", "Derivatively Priced", "Derivatively Priced"},
	{sip.TradeCondReopening, "5", "Reopening Prints", "Reopening Prints"},
	{sip.TradeCondClosing, "6", "Closing Prints", "Closing Prints"},
	{sip.TradeCondQCT, "7", "Qualified Contingent Trade", "Qualified Contingent Trade"},
	{sip.TradeCondReserved, "8", "Reserved", "611 Exempt"},
	{sip.TradeCondCorrectedClose, "9", "Corrected Close", "Corrected Close"},
	{sip.TradeCondAcquisition, "A", "Acquisition", "Acquisition"},
	{sip.TradeCondB, "B", "Average Price Trade", "Bunched Trade"},
	{sip.TradeCondCash, "C", "Cash Trade", "Cash Trade"},
	{sip.TradeCondDistribution, "D", "Distribution", "Distribution"},
	{sip.TradeCondE, "E", "Auto Execution", "Placeholder"},
	{sip.TradeCondISO, "F", "Intermarket Sweep", "Intermarket Sweep"},
	{sip.TradeCondBunchedSold, "G", "Bunched Sold", "Bunched Sold"},
	{sip.TradeCondPriceVariation, "H", "Price Variation Trade", "Price Variation Trade"},
	{sip.TradeCondOddLot, "I", "Odd Lot Trade", "Odd Lot Trade"},
	{sip.TradeCondRule127, "K", "Rule 127/155", "Rule 127/155"},
	{sip.TradeCondSoldLast, "L", "Sold Last", "Sold Last"},
	{sip.TradeCondOfficialClose, "M", "Market Center Official Close", "Market Center Official Close"},
	{sip.TradeCondNextDay, "N", "Next Day Trade", "Next Day Trade"},
	{sip.TradeCondOpening, "O", "Opening Prints", "Opening Prints"},
	{sip.TradeCondPriorRefPrice, "P", "Prior Reference Price", "Prior Reference Price"},
	{sip.TradeCondOfficialOpen, "Q", "Market Center Official Open", "Market Center Official Open"},
	{sip.TradeCondSeller, "R", "Seller", "Seller"},
	{sip.TradeCondSplit, "S", "Split Trade", "Split Trade"},
	{sip.TradeCondExtendedHours, "T", "Extended Hours / Form T", "Extended Hours / Form T"},
	{sip.TradeCondExtendedHoursOOS, "U", "Ext Hours Sold (Out of Seq)", "Ext Hours Sold (Out of Seq)"},
	{sip.TradeCondContingent, "V", "Contingent Trade", "Contingent Trade"},
	{sip.TradeCondAveragePriceUTP, "W", "Average Price Trade", "Average Price Trade"},
	{sip.TradeCondCross, "X", "Cross Trade", "Cross Trade"},
	{sip.TradeCondYellowFlag, "Y", "Yellow Flag", "Yellow Flag"},
	{sip.TradeCondSoldOOS, "Z", "Sold (Out of Sequence)", "Sold (Out of Sequence)"},
}

func describeTradeConditions(cond sip.TradeCond, tape sip.Tape) []string {
	if cond == 0 {
		return nil
	}
	cta := tape == sip.TapeA || tape == sip.TapeB
	var lines []string
	for _, d := range tradeCondTable {
		if cond.Has(d.c) {
			desc := d.utp
			if cta {
				desc = d.cta
			}
			lines = append(lines, fmt.Sprintf("                %s = %s", d.code, desc))
		}
	}
	return lines
}

type qcDesc struct {
	c    sip.QuoteCond
	code string
	cta  string
	utp  string
}

var quoteCondTable = []qcDesc{
	{sip.QuoteCond4, "4", "On Demand Intra Day Auction", "On Demand Intra Day Auction"},
	{sip.QuoteCondA, "A", "Slow Offer", "Manual Ask, Auto Bid"},
	{sip.QuoteCondB, "B", "Slow Bid", "Manual Bid, Auto Ask"},
	{sip.QuoteCondC, "C", "Closing Quote", "Closing Quote"},
	{sip.QuoteCondE, "E", "Slow LRP Bid", "Slow LRP Bid"},
	{sip.QuoteCondF, "F", "Slow LRP Offer", "Fast Trading"},
	{sip.QuoteCondH, "H", "Slow Bid and Offer", "Manual Bid and Ask"},
	{sip.QuoteCondI, "I", "Order Imbalance", "Order Imbalance"},
	{sip.QuoteCondL, "L", "Market Maker Closed", "Closed Quote"},
	{sip.QuoteCondN, "N", "Non-Firm Quote", "Non-Firm Quote"},
	{sip.QuoteCondO, "O", "Opening Quote", "Opening Quote"},
	{sip.QuoteCondR, "R", "Market Maker Open", "Two-Sided Open"},
	{sip.QuoteCondU, "U", "Slow LRP Bid and Offer", "Manual Non-Firm"},
	{sip.QuoteCondW, "W", "Slow Set/Slow List", "Slow Set/Slow List"},
	{sip.QuoteCondX, "X", "Order Influx", "Order Influx"},
	{sip.QuoteCondY, "Y", "One-Sided Open", "One-Sided Open"},
	{sip.QuoteCondZ, "Z", "No Open/No Resume", "No Open/No Resume"},
}

func describeQuoteConditions(cond sip.QuoteCond, tape sip.Tape) []string {
	if cond == 0 {
		return nil
	}
	cta := tape == sip.TapeA || tape == sip.TapeB
	var lines []string
	for _, d := range quoteCondTable {
		if cond.Has(d.c) {
			desc := d.utp
			if cta {
				desc = d.cta
			}
			lines = append(lines, fmt.Sprintf("                %s = %s", d.code, desc))
		}
	}
	return lines
}

func describeLULDIndicator(ind sip.LULDIndicator) string {
	switch ind {
	case sip.LULDIndicatorNA:
		return "Not Applicable"
	case sip.LULDIndicatorPriceBand:
		return "Price Band (initial)"
	case sip.LULDIndicatorRepublished:
		return "Price Band (republished)"
	case sip.LULDIndicatorNBBEntered:
		return "NBB Limit State Entered"
	case sip.LULDIndicatorNBBExited:
		return "NBB Limit State Exited"
	case sip.LULDIndicatorNBOEntered:
		return "NBO Limit State Entered"
	case sip.LULDIndicatorNBOExited:
		return "NBO Limit State Exited"
	case sip.LULDIndicatorNBBNBOEntered:
		return "NBB+NBO Limit State Entered"
	case sip.LULDIndicatorNBBNBOExited:
		return "NBB+NBO Limit State Exited"
	case sip.LULDIndicatorNBBEntNBOExit:
		return "NBB Entered, NBO Exited"
	case sip.LULDIndicatorNBBExitNBOEnt:
		return "NBB Exited, NBO Entered"
	default:
		return fmt.Sprintf("Unknown (%c)", byte(ind))
	}
}

// --- formatting ---

func typeName(t sip.MessageType) string {
	switch t {
	case sip.MessageTypeTrade:
		return "TRD"
	case sip.MessageTypeQuote:
		return "QTE"
	case sip.MessageTypeBar:
		return "BAR"
	case sip.MessageTypeDailyBar:
		return "DLY"
	case sip.MessageTypeUpdatedBar:
		return "UPD"
	case sip.MessageTypeStatus:
		return "STS"
	case sip.MessageTypeLULD:
		return "LLD"
	case sip.MessageTypeImbalance:
		return "IMB"
	case sip.MessageTypeCorrection:
		return "COR"
	case sip.MessageTypeCancelError:
		return "CXE"
	default:
		return fmt.Sprintf("?%c", byte(t))
	}
}

func typeColor(t sip.MessageType) string {
	switch t {
	case sip.MessageTypeTrade:
		return "\033[32m"
	case sip.MessageTypeQuote:
		return "\033[36m"
	case sip.MessageTypeBar, sip.MessageTypeDailyBar, sip.MessageTypeUpdatedBar:
		return "\033[33m"
	case sip.MessageTypeStatus:
		return "\033[31m"
	case sip.MessageTypeLULD:
		return "\033[35m"
	case sip.MessageTypeImbalance:
		return "\033[34m"
	default:
		return ""
	}
}

// --- utilities ---

func commaInt(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return s
	}
	if len(s) <= 3 {
		return s
	}
	var buf bytes.Buffer
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	buf.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		buf.WriteByte(',')
		buf.WriteString(s[i : i+3])
	}
	return buf.String()
}

func indexWidth(total int) int {
	w := 1
	for n := total; n >= 10; n /= 10 {
		w++
	}
	return w
}
