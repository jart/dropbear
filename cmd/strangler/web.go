package main

import (
	"dropbear/auth"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"dropbear/options"
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
	Trader           *Trader
	WebRequests      chan WebRequest
	SSEBroadcast     chan []byte
	SSEMu            sync.Mutex
	SSESubscribers   map[chan []byte]struct{}
	LastSnapshotMu   sync.RWMutex
	LastSnapshotJSON []byte
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
	Time      string        `json:"time"`
	Symbol    string        `json:"symbol"`
	Price     string        `json:"price"`
	Sigma     string        `json:"sigma"`
	Cash      string        `json:"cash"`
	Paused    bool          `json:"paused"`
	State     string        `json:"state"`
	Positions []PositionRow `json:"positions"`
	Risk      []RiskPoint   `json:"risk"`
	Greeks    GreeksData    `json:"greeks"`
	Stats     StatsData     `json:"stats"`
	Flags     FlagsData     `json:"flags"`
}

type PositionRow struct {
	Strike string `json:"strike"`
	Class  string `json:"class"`
	Qty    string `json:"qty"`
	Bid    string `json:"bid"`
	Ask    string `json:"ask"`
	Mid    string `json:"mid"`
	Delta  string `json:"delta"`
	ITM    bool   `json:"itm"`
}

type RiskPoint struct {
	Strike     string `json:"strike"`
	Settlement string `json:"settlement"`
}

type GreeksData struct {
	Delta string `json:"delta"`
	Gamma string `json:"gamma"`
	Theta string `json:"theta"`
	Vega  string `json:"vega"`
}

type StatsData struct {
	Liquidation string `json:"liquidation"`
	Realized    string `json:"realized"`
	Fees        string `json:"fees"`
	Volume      string `json:"volume"`
	Error       string `json:"error"`
	Shares      string `json:"shares"`
	ShareCost   string `json:"shareCost"`
	Orders      string `json:"orders"`
	Worst       string `json:"worst"`
}

type FlagsData struct {
	Tolerance string `json:"tolerance"`
	Quantum   string `json:"quantum"`
	Spread    string `json:"spread"`
	Patience  string `json:"patience"`
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
	delta := decimal.Zero
	gamma := decimal.Zero
	theta := decimal.Zero
	vega := decimal.Zero
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

	// worst case settlement
	worst := decimal.Zero
	worstSet := false
	em := decimal.Zero
	if t.Chain.AtTheMoney != nil {
		em = t.Chain.ExpectedMove()
	}

	snap := StateSnapshot{
		Time:   now.String(),
		Symbol: t.Config.Symbol.String(),
		Price:  price.Format(2),
		Sigma:  em.Format(2),
		Cash:   t.Holdings.Cash.FormatThousand(2),
		Paused: t.Paused,
		State:  t.State.String(),
		Greeks: GreeksData{
			Delta: delta.Format(3),
			Gamma: gamma.Format(3),
			Theta: theta.Format(3),
			Vega:  vega.Format(3),
		},
		Stats: StatsData{
			Liquidation: t.Holdings.LiquidationValue().Truncate().FormatThousand(2),
			Realized:    t.Holdings.RealizedPnL.FormatThousand(2),
			Fees:        t.Holdings.TotalFees.FormatThousand(2),
			Volume:      t.Holdings.Volume.String(),
			Error:       t.Holdings.TotalError.String(),
			Shares:      shares.String(),
			ShareCost:   shareCost.Format(2),
			Orders:      fmt.Sprintf("%d", t.orderCount()),
		},
		Flags: FlagsData{
			Tolerance: t.Config.Tolerance.String(),
			Quantum:   t.Config.Quantum.String(),
			Spread:    t.Config.Spread.String(),
			Patience:  t.Config.Patience.String(),
		},
	}

	// positions: iterate over option holdings
	for security, holding := range t.Holdings.Positions {
		o, ok := security.(*options.Option)
		if !ok {
			continue
		}
		itm := false
		if o.Class == 'C' {
			itm = price.Cmp(o.Strike.Price) > 0
		} else {
			itm = price.Cmp(o.Strike.Price) < 0
		}
		snap.Positions = append(snap.Positions, PositionRow{
			Strike: o.Strike.Price.String(),
			Class:  string(o.Class),
			Qty:    holding.Quantity.String(),
			Bid:    o.Bid.Format(2),
			Ask:    o.Ask.Format(2),
			Mid:    o.MidPrice().Format(2),
			Delta:  o.Delta.Format(4),
			ITM:    itm,
		})
	}

	// risk profile across strikes within ±4 sigmas
	if price.IsPositive() && t.Chain.Strikes.Size() > 0 {
		chartLo := price.Sub(em.MulInt(4).Max(price.DivInt(10)))
		chartHi := price.Add(em.MulInt(4).Max(price.DivInt(10)))
		for it := t.Chain.Strikes.Iterator(); it.Next(); {
			strike := it.Value()
			if strike.Price.Cmp(chartLo) < 0 || strike.Price.Cmp(chartHi) > 0 {
				continue
			}
			settlement := t.computeSettlementAt(strike.Price)
			snap.Risk = append(snap.Risk, RiskPoint{
				Strike:     strike.Price.Format(2),
				Settlement: settlement.Format(2),
			})
			if !worstSet || settlement.Cmp(worst) < 0 {
				worst = settlement
				worstSet = true
			}
		}
	}
	snap.Stats.Worst = worst.FormatThousand(2)

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

// broadcastState builds a snapshot and caches it for polling.
// Must be called from the main goroutine.
func (w *Web) broadcastState() {
	data, err := json.Marshal(w.buildStateSnapshot())
	if err != nil {
		return
	}
	w.LastSnapshotMu.Lock()
	w.LastSnapshotJSON = data
	w.LastSnapshotMu.Unlock()
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
	if f.Tolerance != "" {
		t.Config.Tolerance = decimal.Parse(f.Tolerance)
		log.Printf("web: tolerance = %s", f.Tolerance)
	}
	if f.Quantum != "" {
		t.Config.Quantum = decimal.Parse(f.Quantum)
		log.Printf("web: quantum = %s", f.Quantum)
	}
	if f.Spread != "" {
		t.Config.Spread = decimal.Parse(f.Spread)
		log.Printf("web: spread = %s", f.Spread)
	}
	if f.Patience != "" {
		d, err := clocky.ParseDuration(f.Patience)
		if err == nil {
			t.Config.Patience = d
			log.Printf("web: patience = %s", f.Patience)
		}
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

func (web *Web) handleStateAPI(w http.ResponseWriter, r *http.Request) {
	web.LastSnapshotMu.RLock()
	data := web.LastSnapshotJSON
	web.LastSnapshotMu.RUnlock()
	web.noCache(w)
	w.Header().Set("Content-Type", "application/json")
	if data != nil {
		w.Write(data)
	} else {
		w.Write([]byte("{}"))
	}
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
	mux.HandleFunc("/api/state", a.RequireAuth(web.handleStateAPI))
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
