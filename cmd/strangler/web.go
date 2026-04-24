package main

import (
	"dropbear/auth"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/options"
	"dropbear/osi"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

//go:embed static
var staticFiles embed.FS

var (
	listenFlag = flag.String("listen", "0.0.0.0:8484", "web dashboard bind address")
	sldFlag    = flag.String("sld", "justinestreet.capital", "second level domain flag")
)

type Web struct {
	Trader         *Trader
	WebRequests    chan WebRequest
	SSEBroadcast   chan []byte
	SSEMu          sync.Mutex
	SSESubscribers map[chan []byte]struct{}
}

func NewWeb() *Web {
	return &Web{
		WebRequests:    make(chan WebRequest, 8),
		SSEBroadcast:   make(chan []byte, 4),
		SSESubscribers: map[chan []byte]struct{}{},
	}
}

// WebRequest is a command sent from the web server to the main goroutine.
type WebRequest struct {
	Type     string        // "flags", "pause", "resume", "broadcast"
	Flags    *FlagsData    // for "flags"
	Response chan struct{} // closed when processed
}

func (w *Web) sseSubscribe() chan []byte {
	ch := make(chan []byte, 4)
	w.SSEMu.Lock()
	w.SSESubscribers[ch] = struct{}{}
	w.SSEMu.Unlock()
	return ch
}

func (w *Web) sseUnsubscribe(ch chan []byte) {
	w.SSEMu.Lock()
	delete(w.SSESubscribers, ch)
	w.SSEMu.Unlock()
}

func (w *Web) sseBroadcaster() {
	for data := range w.SSEBroadcast {
		w.SSEMu.Lock()
		for ch := range w.SSESubscribers {
			select {
			case ch <- data:
			default:
			}
		}
		w.SSEMu.Unlock()
	}
}

// StateSnapshot is the JSON payload sent to the dashboard.
type StateSnapshot struct {
	Time          string            `json:"time"`
	Symbol        string            `json:"symbol"`
	Price         decimal.Decimal   `json:"price"`
	Sigma         decimal.Decimal   `json:"sigma"`
	Cash          decimal.Decimal   `json:"cash"`
	Paused        bool              `json:"paused"`
	Positions     []PositionRow     `json:"positions"`
	Risk          []RiskPoint       `json:"risk"`
	Greeks        GreeksData        `json:"greeks"`
	Stats         StatsData         `json:"stats"`
	Flags         FlagsData         `json:"flags"`
	PendingOrders []PendingOrderRow `json:"pendingOrders"`
	Transactions  []TransactionRow  `json:"transactions"`
}

type PositionRow struct {
	Strike decimal.Decimal `json:"strike"`
	Class  string          `json:"class"`
	Qty    decimal.Decimal `json:"qty"`
	Bid    decimal.Decimal `json:"bid"`
	Ask    decimal.Decimal `json:"ask"`
	Mid    decimal.Decimal `json:"mid"`
	Delta  decimal.Decimal `json:"delta"`
	IV     decimal.Decimal `json:"iv"`
	ITM    bool            `json:"itm"`
}

type RiskPoint struct {
	Strike     decimal.Decimal `json:"strike"`
	Settlement decimal.Decimal `json:"settlement"`
}

type GreeksData struct {
	Delta decimal.Decimal `json:"delta"`
	Gamma decimal.Decimal `json:"gamma"`
	Theta decimal.Decimal `json:"theta"`
	Vega  decimal.Decimal `json:"vega"`
}

type StatsData struct {
	Liquidation decimal.Decimal `json:"liquidation"`
	Realized    decimal.Decimal `json:"realized"`
	Fees        decimal.Decimal `json:"fees"`
	Volume      decimal.Decimal `json:"volume"`
	Error       decimal.Decimal `json:"error"`
	Shares      decimal.Decimal `json:"shares"`
	ShareCost   decimal.Decimal `json:"shareCost"`
	Worst       decimal.Decimal `json:"worst"`
	EOD         decimal.Decimal `json:"eod"`
	Orders      string          `json:"orders"`
}

type PendingOrderRow struct {
	ID       int             `json:"id"`
	Security string          `json:"security"`
	Qty      decimal.Decimal `json:"qty"`
	Price    decimal.Decimal `json:"price"`
	Mid      decimal.Decimal `json:"mid"`
	Status   string          `json:"status"`
	Age      string          `json:"age"`
}

type TransactionRow struct {
	Time        string          `json:"time"`
	Security    string          `json:"security"`
	Qty         decimal.Decimal `json:"qty"`
	Limit       decimal.Decimal `json:"limit"`
	Fill        decimal.Decimal `json:"fill"`
	Improvement decimal.Decimal `json:"improvement"`
	PnL         decimal.Decimal `json:"pnl"`
	Fee         decimal.Decimal `json:"fee"`
}

type FlagsData struct {
	Contracts decimal.Decimal `json:"contracts"`
	Tolerance decimal.Decimal `json:"tolerance"`
	Spread    decimal.Decimal `json:"spread"`
	Risk      decimal.Decimal `json:"risk"`
	Patience  string          `json:"patience"`
}

// buildStateSnapshot serializes all state into a snapshot.
// Must be called from the main goroutine.
func (w *Web) buildStateSnapshot() StateSnapshot {
	t := w.Trader
	now := clocky.Now()
	price := decimal.Zero
	if t.Underlying != nil {
		price = t.Underlying.MidPrice()
	}

	// greeks
	vega := decimal.Zero
	delta := decimal.Zero
	gamma := decimal.Zero
	theta := decimal.Zero
	for security, holding := range t.Holdings.Positions {
		qty := holding.Quantity
		mult := security.Multiplier()
		delta = delta.Add(security.GetDelta().Mul(qty).MulInt(mult))
		if o, ok := security.(*options.Option); ok {
			gamma = gamma.Add(o.Gamma.Mul(qty).MulInt(mult))
			theta = theta.Add(o.Theta.Mul(qty).MulInt(mult))
			vega = vega.Add(o.Vega.Mul(qty).MulInt(mult))
		}
	}

	// equity hedge position
	shares := decimal.Zero
	shareCost := decimal.Zero
	if holding := t.Holdings.Positions[t.Underlying]; holding != nil {
		shares = holding.Quantity
		shareCost = holding.AverageCost
	}

	// end of day and worst case settlement
	eod := decimal.Zero
	if price.IsPositive() {
		eod = t.computeSettlementAt(price).Truncate()
	}
	worst := decimal.Zero
	worstSet := false
	em := decimal.Zero
	if t.Chain.AtTheMoney != nil {
		em = t.Chain.ExpectedMove()
	}

	snap := StateSnapshot{
		Time:   now.String(),
		Symbol: t.Config.Symbol.String(),
		Price:  price,
		Sigma:  em,
		Cash:   t.Holdings.Cash,
		Paused: t.Paused,
		Greeks: GreeksData{
			Delta: delta,
			Gamma: gamma,
			Theta: theta,
			Vega:  vega,
		},
		Stats: StatsData{
			Liquidation: t.Holdings.LiquidationValue().Truncate(),
			EOD:         eod,
			Realized:    t.Holdings.RealizedPnL,
			Fees:        t.Holdings.TotalFees,
			Volume:      t.Holdings.Volume,
			Error:       t.Holdings.TotalError,
			Shares:      shares,
			ShareCost:   shareCost,
			Orders:      fmt.Sprintf("%d", t.orderCount()),
		},
		Flags: FlagsData{
			Contracts: t.Config.Contracts,
			Tolerance: t.Config.Tolerance,
			Spread:    t.Config.Spread,
			Risk:      t.Config.Risk,
			Patience:  t.Config.Patience.String(),
		},
	}

	// find strike range of held option positions
	heldQty := map[decimal.Decimal]map[byte]decimal.Decimal{}
	var minStrike, maxStrike decimal.Decimal
	hasPositions := false
	for security, holding := range t.Holdings.Positions {
		o, ok := security.(*options.Option)
		if !ok {
			continue
		}
		sp := o.Strike.Price
		if !hasPositions || sp.Cmp(minStrike) < 0 {
			minStrike = sp
		}
		if !hasPositions || sp.Cmp(maxStrike) > 0 {
			maxStrike = sp
		}
		hasPositions = true
		if heldQty[sp] == nil {
			heldQty[sp] = map[byte]decimal.Decimal{}
		}
		heldQty[sp][byte(o.Class)] = holding.Quantity
	}

	// emit chain data for all strikes in the held range
	if hasPositions && t.Chain.Strikes.Size() > 0 {
		for it := t.Chain.Strikes.Iterator(); it.Next(); {
			strike := it.Value()
			if strike.Price.Cmp(minStrike) < 0 || strike.Price.Cmp(maxStrike) > 0 {
				continue
			}
			if strike.Call != nil {
				qty := decimal.Zero
				if m := heldQty[strike.Price]; m != nil {
					qty = m['C']
				}
				snap.Positions = append(snap.Positions, PositionRow{
					Strike: strike.Price,
					Class:  "C",
					Qty:    qty,
					Bid:    strike.Call.Bid,
					Ask:    strike.Call.Ask,
					Mid:    strike.Call.MidPrice(),
					Delta:  strike.Call.Delta,
					IV:     strike.Call.IV,
					ITM:    price.Cmp(strike.Price) > 0,
				})
			}
			if strike.Put != nil {
				qty := decimal.Zero
				if m := heldQty[strike.Price]; m != nil {
					qty = m['P']
				}
				snap.Positions = append(snap.Positions, PositionRow{
					Strike: strike.Price,
					Class:  "P",
					Qty:    qty,
					Bid:    strike.Put.Bid,
					Ask:    strike.Put.Ask,
					Mid:    strike.Put.MidPrice(),
					Delta:  strike.Put.Delta,
					IV:     strike.Put.IV,
					ITM:    price.Cmp(strike.Price) < 0,
				})
			}
		}
	}

	// risk profile across strikes within ±4 sigmas
	if price.IsPositive() && t.Chain.Strikes.Size() > 0 {
		span := em.MulInt(4)
		if span.IsZero() {
			span = price.DivInt(20)
		}
		chartLo := price.Sub(span)
		chartHi := price.Add(span)
		for it := t.Chain.Strikes.Iterator(); it.Next(); {
			strike := it.Value()
			if strike.Price.Cmp(chartLo) < 0 || strike.Price.Cmp(chartHi) > 0 {
				continue
			}
			settlement := t.computeSettlementAt(strike.Price)
			snap.Risk = append(snap.Risk, RiskPoint{
				Strike:     strike.Price,
				Settlement: settlement,
			})
			if !worstSet || settlement.Cmp(worst) < 0 {
				worst = settlement
				worstSet = true
			}
		}
	}
	snap.Stats.Worst = worst

	// pending orders
	for order := range t.PendingOrders {
		status := "pending"
		if order.Canceling {
			status = "canceling"
		} else if order.Making {
			status = "making"
		}
		snap.PendingOrders = append(snap.PendingOrders, PendingOrderRow{
			ID:       order.ID,
			Security: osi.Humanize(order.Security.Name()),
			Qty:      order.Quantity,
			Price:    order.Price,
			Mid:      order.Security.MidPrice(),
			Status:   status,
			Age:      now.Sub(order.Created).String(),
		})
	}

	// transactions (most recent first, cap at 50)
	txs := t.Transactions
	start := 0
	if len(txs) > 50 {
		start = len(txs) - 50
	}
	for i := len(txs) - 1; i >= start; i-- {
		tx := txs[i]
		// improvement: positive means we got a better price than our limit
		// buying: limit - fill (paid less than willing)
		// selling: fill - limit (received more than willing)
		improvement := decimal.Zero
		if !tx.Limit.IsZero() {
			if tx.Quantity.IsPositive() {
				improvement = tx.Limit.Sub(tx.Fill)
			} else {
				improvement = tx.Fill.Sub(tx.Limit)
			}
		}
		snap.Transactions = append(snap.Transactions, TransactionRow{
			Time:        tx.Time.String(),
			Security:    osi.Humanize(tx.Security.Name()),
			Qty:         tx.Quantity,
			Limit:       tx.Limit,
			Fill:        tx.Fill,
			Improvement: improvement,
			PnL:         tx.PnL,
			Fee:         tx.Fee,
		})
	}

	return snap
}

// computeSettlementAt returns the total portfolio value if the underlying
// settles at the given price. Options are valued at intrinsic; equity
// positions are valued at the settlement price.
func (t *Trader) computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	value := t.Holdings.Cash
	for security, holding := range t.Holdings.Positions {
		switch s := security.(type) {
		case *options.Option:
			intrinsic := s.IntrinsicValue(underlyingPrice)
			value = value.Add(intrinsic.Mul(holding.Quantity).MulInt(100))
		case *options.Equity:
			value = value.Add(underlyingPrice.Mul(holding.Quantity))
		}
	}
	return value
}

// broadcastState builds a snapshot and pushes it to all SSE subscribers.
// Must be called from the main goroutine.
func (w *Web) broadcastState() {
	data, err := json.Marshal(w.buildStateSnapshot())
	if err != nil {
		return
	}
	select {
	case w.SSEBroadcast <- data:
	default:
	}
}

// processWebRequest handles a web request on the main goroutine.
func (w *Web) processWebRequest(req WebRequest) {
	t := w.Trader
	switch req.Type {
	case "pause":
		t.Paused = true
		log.Printf("web: paused")
	case "resume":
		t.Paused = false
		log.Printf("web: resumed")
	case "flags":
		if req.Flags != nil {
			w.applyFlags(req.Flags)
		}
	case "broadcast":
		// just broadcast state, handled below
	}
	w.broadcastState()
	if req.Response != nil {
		close(req.Response)
	}
}

func (w *Web) applyFlags(f *FlagsData) {
	t := w.Trader
	t.Config.Contracts = f.Contracts
	log.Printf("web: contracts = %s", f.Contracts)
	t.Config.Tolerance = f.Tolerance
	log.Printf("web: tolerance = %s", f.Tolerance)
	t.Config.Spread = f.Spread
	log.Printf("web: spread = %s", f.Spread)
	t.Config.Risk = f.Risk
	log.Printf("web: risk = %s", f.Risk)
	d, err := clocky.ParseDuration(f.Patience)
	if err == nil {
		t.Config.Patience = d
		log.Printf("web: patience = %s", f.Patience)
	}
}

// HTTP handlers

func (web *Web) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	web.noCache(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func (web *Web) noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (web *Web) handleEventsAPI(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := web.sseSubscribe()
	defer web.sseUnsubscribe(ch)
	web.WebRequests <- WebRequest{Type: "broadcast"}
	for {
		select {
		case data := <-ch:
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (web *Web) handleFlagsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var flags FlagsData
	if err := json.NewDecoder(r.Body).Decode(&flags); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "flags", Flags: &flags, Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (web *Web) handlePauseAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "pause", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "paused"})
}

func (web *Web) handleResumeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "resume", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resumed"})
}

func (web *Web) Start(t *Trader) {
	web.Trader = t
	rpID := *sldFlag
	origin := "https://" + strings.ToLower(t.Config.Symbol.String()) + "-strangler." + *sldFlag

	go web.sseBroadcaster()

	database := db.Get()
	if err := auth.Migrate(database); err != nil {
		log.Fatalf("web: failed to migrate auth schema: %v", err)
	}
	a, err := auth.New(database, rpID, origin)
	if err != nil {
		log.Fatalf("web: failed to initialize auth: %v", err)
	}

	mux := http.NewServeMux()
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// public auth routes
	mux.Handle("/auth/static/", http.StripPrefix("/auth/static/", auth.StaticHandler()))
	mux.HandleFunc("/login", a.HandleLoginPage)
	mux.HandleFunc("/register", a.HandleRegisterPage)
	mux.HandleFunc("/logout", a.HandleLogout)
	mux.HandleFunc("/auth/register/begin", a.HandleRegisterBegin)
	mux.HandleFunc("/auth/register/finish", a.HandleRegisterFinish)
	mux.HandleFunc("/auth/login/begin", a.HandleLoginBegin)
	mux.HandleFunc("/auth/login/finish", a.HandleLoginFinish)

	// admin routes
	mux.HandleFunc("/admin/invites", a.RequireAdmin(a.HandleInvitesPage))
	mux.HandleFunc("/auth/invites/create", a.RequireAdmin(a.HandleCreateInvite))
	mux.HandleFunc("/auth/invites/list", a.RequireAdmin(a.HandleListInvites))

	// protected dashboard routes
	mux.HandleFunc("/", a.RequireAuth(web.handleIndex))
	mux.HandleFunc("/api/events", a.RequireAuth(web.handleEventsAPI))
	mux.HandleFunc("/api/flags", a.RequireAuthReadOnly(web.handleFlagsAPI))
	mux.HandleFunc("/api/pause", a.RequireAuthReadOnly(web.handlePauseAPI))
	mux.HandleFunc("/api/resume", a.RequireAuthReadOnly(web.handleResumeAPI))

	listen := *listenFlag
	if t.Config.Listen != "" {
		listen = t.Config.Listen
	}
	sock, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("web server listen error: %v", err)
	}
	log.Printf("dashboard for %s at http://%s", t.Config.Symbol, sock.Addr())
	go http.Serve(sock, mux)
}
