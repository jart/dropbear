package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

const tuiMaxLogLines = 1000

const (
	tuiCmdToggle = iota
	tuiCmdAdjust
)

// TUICommand is sent from the TUI goroutine to the main event loop.
type TUICommand struct {
	kind   int
	symbol symbol.Symbol
	field  int
	delta  decimal.Decimal
}

// Editable field indices.
const (
	fieldTarget = iota
	fieldQty
	fieldSpread
	fieldDrift
	fieldGreed
	fieldCount
)

var fieldSteps = [fieldCount]decimal.Decimal{
	decimal.Parse("100"),   // target: ±100 shares
	decimal.Parse("50"),    // qty: ±50 shares
	decimal.Parse("0.005"), // spread
	decimal.Parse("0.005"), // drift
	decimal.Parse("0.005"), // greed
}

var fieldLabels = [fieldCount]string{
	"TARGET", "QTY", "SPREAD", "DRIFT", "GREED",
}

// Column positions for stock table.
const (
	cpSym  = 2
	cpSt   = 9
	cpVnue = 13
	cpPos  = 18
	cpTarg = 25
	cpQty  = 32
	cpSpr  = 38
	cpDft  = 45
	cpGrd  = 52
	cpBuy  = 60
	cpBid  = 70
	cpAsk  = 84
	cpSell = 98
	cpEma  = 108
	cpReal = 118
	cpUnrl = 128
	cpEq   = 138
	cpFees = 148
)

// TUI manages the terminal interface for the maker.
type TUI struct {
	commands chan<- TUICommand
	sigChan  chan<- os.Signal
	oldState *term.State
	row      int // selected stock row
	col      int // selected editable field
	mu       sync.Mutex
	logLines []string
}

func startTUI(commands chan TUICommand, sigChan chan os.Signal) *TUI {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(os.Stderr, "tui: stdin is not a terminal\n")
		os.Exit(1)
	}
	t := &TUI{
		commands: commands,
		sigChan:  sigChan,
	}
	gLogWriter.capture = t.captureLog
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tui: %v\n", err)
		os.Exit(1)
	}
	t.oldState = oldState
	os.Stdout.WriteString("\033[?1049h\033[?25l") // alt screen, hide cursor
	go t.inputLoop()
	go t.renderLoop()
	return t
}

func (t *TUI) stop() {
	gLogWriter.capture = nil
	os.Stdout.WriteString("\033[?25h\033[?1049l") // show cursor, restore screen
	term.Restore(int(os.Stdin.Fd()), t.oldState)
}

func (t *TUI) captureLog(line string) {
	t.mu.Lock()
	t.logLines = append(t.logLines, strings.TrimRight(line, "\n"))
	if len(t.logLines) > tuiMaxLogLines {
		t.logLines = t.logLines[tuiMaxLogLines/2:]
	}
	t.mu.Unlock()
}

func (t *TUI) inputLoop() {
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		for i := 0; i < n; {
			if buf[i] == 0x1b && i+2 < n && buf[i+1] == '[' {
				switch buf[i+2] {
				case 'A':
					t.moveRow(-1)
				case 'B':
					t.moveRow(1)
				case 'C':
					t.moveCol(1)
				case 'D':
					t.moveCol(-1)
				case 'Z': // shift-tab
					t.moveCol(-1)
				}
				i += 3
				continue
			}
			switch buf[i] {
			case 'k':
				t.moveRow(-1)
			case 'j':
				t.moveRow(1)
			case 'h':
				t.moveCol(-1)
			case 'l':
				t.moveCol(1)
			case '\t':
				t.moveCol(1)
			case ' ':
				t.toggle()
			case '+', '=':
				t.adjust(1)
			case '-':
				t.adjust(-1)
			case 'q', 3: // q or ctrl-c
				select {
				case t.sigChan <- syscall.SIGTERM:
				default:
				}
				return
			}
			i++
		}
	}
}

func (t *TUI) moveRow(d int) {
	n := len(gSymbols)
	if n == 0 {
		return
	}
	t.row += d
	if t.row < 0 {
		t.row = 0
	} else if t.row >= n {
		t.row = n - 1
	}
}

func (t *TUI) moveCol(d int) {
	t.col += d
	if t.col < 0 {
		t.col = 0
	} else if t.col >= fieldCount {
		t.col = fieldCount - 1
	}
}

func (t *TUI) toggle() {
	states := sortedSymbols()
	if t.row < len(states) {
		t.commands <- TUICommand{kind: tuiCmdToggle, symbol: states[t.row].symbol}
	}
}

func (t *TUI) adjust(sign int) {
	states := sortedSymbols()
	if t.row >= len(states) {
		return
	}
	d := fieldSteps[t.col]
	if sign < 0 {
		d = d.Neg()
	}
	t.commands <- TUICommand{
		kind:   tuiCmdAdjust,
		symbol: states[t.row].symbol,
		field:  t.col,
		delta:  d,
	}
}

// renderLoop redraws the screen on a real wall-clock timer
// (not clocky, which may be simulated during backtest).
func (t *TUI) renderLoop() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		t.render()
	}
}

func (t *TUI) render() {
	w, h, _ := term.GetSize(int(os.Stdout.Fd()))
	if w < 80 || h < 10 {
		return
	}

	states := sortedSymbols()
	var b bytes.Buffer
	b.Grow(w * h * 2)
	b.WriteString("\033[H") // cursor home

	// compute totals
	totalUnrealized := decimal.Zero
	for _, st := range states {
		if st.quote != nil && !st.position.IsZero() && !st.costBasis.IsZero() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			totalUnrealized = totalUnrealized.Add(mid.Mul(st.position).Sub(st.costBasis))
		}
	}
	net := gTotalPnL.Sub(gTotalFees)
	equity := net.Add(totalUnrealized)

	row := 0

	// header
	tuiWriteLn(&b, " \033[1mMAKER\033[0m  equity %s  realized %s  unrealized %s  fees %s  net %s  fills \033[1m%d\033[0m  shares %s",
		tuiColorPnL(equity), tuiColorPnL(gTotalPnL), tuiColorPnL(totalUnrealized),
		tuiColorFee(gTotalFees), tuiColorPnL(net), gTotalFills, gTotalShares)
	row++

	// separator
	tuiWriteLn(&b, "\033[90m %s\033[0m", strings.Repeat("─", min(w-2, 195)))
	row++

	// column headers
	b.WriteString("\033[90m")
	tuiAt(&b, cpSym, "SYMBOL")
	tuiAt(&b, cpSt, "ST")
	tuiAt(&b, cpVnue, "VNUE")
	tuiAt(&b, cpPos, fmt.Sprintf("%6s", "POS"))
	tuiAt(&b, cpTarg, fmt.Sprintf("%6s", "TARGET"))
	tuiAt(&b, cpQty, fmt.Sprintf("%5s", "QTY"))
	tuiAt(&b, cpSpr, fmt.Sprintf("%6s", "SPREAD"))
	tuiAt(&b, cpDft, fmt.Sprintf("%6s", "DRIFT"))
	tuiAt(&b, cpGrd, fmt.Sprintf("%6s", "GREED"))
	tuiAt(&b, cpBuy, "BUY@")
	tuiAt(&b, cpBid, "BID")
	tuiAt(&b, cpAsk, "ASK")
	tuiAt(&b, cpSell, "SELL@")
	tuiAt(&b, cpEma, fmt.Sprintf("%8s", "EMA"))
	tuiAt(&b, cpReal, fmt.Sprintf("%9s", "REALIZED"))
	tuiAt(&b, cpUnrl, fmt.Sprintf("%9s", "UNREAL"))
	tuiAt(&b, cpEq, fmt.Sprintf("%9s", "EQUITY"))
	tuiAt(&b, cpFees, fmt.Sprintf("%8s", "FEES"))
	b.WriteString("\033[0m\033[K\r\n")
	row++

	// stock rows
	for i, st := range states {
		if row >= h-2 {
			break
		}
		t.writeStock(&b, i, st)
		row++
	}

	// separator
	tuiWriteLn(&b, "\033[90m %s\033[0m", strings.Repeat("─", min(w-2, 195)))
	row++

	// log area
	logRows := h - row - 1
	if logRows < 0 {
		logRows = 0
	}
	t.mu.Lock()
	n := len(t.logLines)
	start := n - logRows
	if start < 0 {
		start = 0
	}
	visible := make([]string, n-start)
	copy(visible, t.logLines[start:])
	t.mu.Unlock()

	for i := 0; i < logRows; i++ {
		if i < len(visible) {
			s := visible[i]
			if len(s) > w-2 {
				s = s[:w-2]
			}
			tuiWriteLn(&b, " %s", s)
		} else {
			tuiWriteLn(&b, "")
		}
	}

	// help bar
	help := fmt.Sprintf(" j/k:row  h/l:field  space:on/off  +/-:adjust(%s)  q:quit", fieldLabels[t.col])
	fmt.Fprintf(&b, "\033[%d;1H\033[7m%-*s\033[0m", h, w, help)

	os.Stdout.Write(b.Bytes())
}

func (t *TUI) writeStock(b *bytes.Buffer, idx int, st *State) {
	sel := idx == t.row
	cfg := st.config

	// prefix
	if sel {
		b.WriteByte('>')
	} else {
		b.WriteByte(' ')
	}

	// symbol
	tuiAt(b, cpSym, fmt.Sprintf("%-6s", st.symbol))

	// status
	switch {
	case st.disabled:
		tuiAt(b, cpSt, "\033[31mOFF\033[39m")
	case st.halt:
		tuiAt(b, cpSt, "\033[33mHLT\033[39m")
	case st.cooldownUntil != 0 && clocky.Now().Before(st.cooldownUntil):
		tuiAt(b, cpSt, "\033[33mCOL\033[39m")
	default:
		tuiAt(b, cpSt, "\033[32m ON\033[39m")
	}

	// venue
	var venue string
	switch cfg.venue {
	case alpaca.OrderDestinationNASDAQ:
		venue = "NSDQ"
	case alpaca.OrderDestinationNYSE:
		venue = "NYSE"
	case alpaca.OrderDestinationARCA:
		venue = "ARCA"
	case alpaca.OrderDestinationAuto:
		venue = "AUTO"
	default:
		venue = fmt.Sprintf("%-4s", cfg.venue)
	}
	tuiAt(b, cpVnue, venue)

	// position
	tuiAt(b, cpPos, fmt.Sprintf("%6s", st.position))

	// editable fields
	editVals := [fieldCount]string{
		fmt.Sprintf("%6s", cfg.target),
		fmt.Sprintf("%5s", cfg.qty),
		fmt.Sprintf("%6s", cfg.spread),
		fmt.Sprintf("%6s", cfg.drift),
		fmt.Sprintf("%6s", cfg.greed),
	}
	editPos := [fieldCount]int{cpTarg, cpQty, cpSpr, cpDft, cpGrd}
	for i := range fieldCount {
		s := editVals[i]
		if sel && i == t.col {
			s = "\033[1;4m" + s + "\033[22;24m"
		}
		tuiAt(b, editPos[i], s)
	}

	// buy/sell prices
	if st.buyPrice.IsPositive() {
		tuiAt(b, cpBuy, fmt.Sprintf("%-8s", st.buyPrice))
	} else {
		tuiAt(b, cpBuy, "   -    ")
	}

	// bid
	if st.quote != nil && st.quote.BidPrice.IsPositive() {
		tuiAt(b, cpBid, fmt.Sprintf("%-13s", fmt.Sprintf("%sx%d", st.quote.BidPrice, st.quote.BidSize)))
	} else {
		tuiAt(b, cpBid, "     -       ")
	}

	// ask
	if st.quote != nil && st.quote.AskPrice.IsPositive() {
		tuiAt(b, cpAsk, fmt.Sprintf("%-13s", fmt.Sprintf("%sx%d", st.quote.AskPrice, st.quote.AskSize)))
	} else {
		tuiAt(b, cpAsk, "     -       ")
	}

	// sell
	if st.sellPrice.IsPositive() {
		tuiAt(b, cpSell, fmt.Sprintf("%-8s", st.sellPrice))
	} else {
		tuiAt(b, cpSell, "   -    ")
	}

	// EMA
	if st.ema.IsReady() {
		tuiAt(b, cpEma, fmt.Sprintf("%8s", st.ema.Value.Format(3)))
	} else {
		tuiAt(b, cpEma, "    -   ")
	}

	// P&L
	unrealized := decimal.Zero
	if st.quote != nil && !st.position.IsZero() && !st.costBasis.IsZero() {
		mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
		unrealized = mid.Mul(st.position).Sub(st.costBasis)
	}
	eq := st.realizedPnL.Sub(st.totalFees).Add(unrealized)
	tuiAt(b, cpReal, tuiColorW(st.realizedPnL, 9))
	tuiAt(b, cpUnrl, tuiColorW(unrealized, 9))
	tuiAt(b, cpEq, tuiColorW(eq, 9))
	tuiAt(b, cpFees, tuiColorFeeW(st.totalFees, 8))

	b.WriteString("\033[K\r\n")
}

// tuiAt positions cursor at column col and writes s.
func tuiAt(b *bytes.Buffer, col int, s string) {
	fmt.Fprintf(b, "\033[%dG%s", col, s)
}

// tuiWriteLn writes a formatted line and clears to end.
func tuiWriteLn(b *bytes.Buffer, format string, args ...any) {
	fmt.Fprintf(b, format, args...)
	b.WriteString("\033[K\r\n")
}

// tuiColorPnL formats a P&L value with green/red color.
func tuiColorPnL(d decimal.Decimal) string {
	s := d.Format(2)
	if d.IsPositive() {
		return "\033[32m" + s + "\033[0m"
	}
	if d.IsNegative() {
		return "\033[31m" + s + "\033[0m"
	}
	return s
}

// tuiColorW formats a P&L value with color and fixed width.
func tuiColorW(d decimal.Decimal, w int) string {
	s := fmt.Sprintf("%*s", w, d.Format(2))
	if d.IsPositive() {
		return "\033[32m" + s + "\033[39m"
	}
	if d.IsNegative() {
		return "\033[31m" + s + "\033[39m"
	}
	return s
}

// tuiColorFee formats fees with red color (fees are a cost).
func tuiColorFee(d decimal.Decimal) string {
	s := d.Format(2)
	if d.IsPositive() {
		return "\033[31m" + s + "\033[0m"
	}
	return s
}

// tuiColorFeeW formats fees with color and fixed width.
func tuiColorFeeW(d decimal.Decimal, w int) string {
	s := fmt.Sprintf("%*s", w, d.Format(2))
	if d.IsPositive() {
		return "\033[31m" + s + "\033[39m"
	}
	return s
}
