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
	"strconv"
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
		WebRequests:      make(chan WebRequest, 8),
		SSEBroadcast:     make(chan []byte, 4),
		SSEMu:            sync.Mutex{},
		SSESubscribers:   map[chan []byte]struct{}{},
		LastSnapshotMu:   sync.RWMutex{},
		LastSnapshotJSON: []byte{},
	}
}

// WebRequest is a command sent from the web server to the main goroutine.
type WebRequest struct {
	Type     string          // "flags", "pause", "resume", "strategies"
	Flags    *FlagsData      // for "flags"
	Strats   map[string]bool // for "strategies"
	Response chan struct{}   // closed when processed
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
				// drop if subscriber is slow
			}
		}
		w.SSEMu.Unlock()
	}
}

// StateSnapshot is the JSON payload sent to the dashboard.
type StateSnapshot struct {
	Time       string                  `json:"time"`
	Symbol     string                  `json:"symbol"`
	Price      string                  `json:"price"`
	Sigma      string                  `json:"sigma"`
	Cash       string                  `json:"cash"`
	Paused     bool                    `json:"paused"`
	Panicking  bool                    `json:"panicking"`
	Positions  []PositionRow           `json:"positions"`
	Risk       []RiskPoint             `json:"risk"`
	Greeks     GreeksData              `json:"greeks"`
	Stats      StatsData               `json:"stats"`
	Strategies map[string]StrategyInfo `json:"strategies"`
	Flags      FlagsData               `json:"flags"`
}

type PositionRow struct {
	OSI    string `json:"osi"`
	Strike string `json:"strike"`
	Class  string `json:"class"`
	Qty    string `json:"qty"`
	Bid    string `json:"bid"`
	Ask    string `json:"ask"`
	Mid    string `json:"mid"`
	Delta  string `json:"delta"`
	Prob   string `json:"prob"`
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
	Payoff      string `json:"payoff"`
	Worst       string `json:"worst"`
	Liquidation string `json:"liquidation"`
	Notional    string `json:"notional"`
	EOD         string `json:"eod"`
	Sigma       string `json:"sigma"`
	Bias        string `json:"bias"`
	Realized    string `json:"realized"`
	Error       string `json:"error"`
	Fees        string `json:"fees"`
	Volume      string `json:"volume"`
}

type FlagsData struct {
	Sigmas       string `json:"sigmas"`
	Budget       string `json:"budget"`
	Floor        string `json:"floor"`
	Spread       string `json:"spread"`
	Eval         string `json:"eval"`
	Cooldown     string `json:"cooldown"`
	Patience     string `json:"patience"`
	Prune        string `json:"prune"`
	WPayoff      string `json:"wPayoff"`
	WRisk        string `json:"wRisk"`
	WDelta       string `json:"wDelta"`
	Demand       string `json:"demand"`
	BypassRisk   string `json:"bypassRisk"`
	BypassScore  string `json:"bypassScore"`
	BypassPayoff string `json:"bypassPayoff"`
	Closing      string `json:"closing"`
}

type StrategyInfo struct {
	Enabled bool `json:"enabled"`
	Count   int  `json:"count"`
}

// buildStateSnapshot serializes all state into a snapshot.
// Must be called from the main goroutine.
func (w *Web) buildStateSnapshot() StateSnapshot {
	t := w.Trader
	now := clocky.Now()
	t.precomputeSettlements()
	delta, gamma, theta, vega := t.computeGreeks()
	payoff := t.computeExpectedPayoff()
	payoffStr := "N/A"
	if payoff.Cmp(decimal.Min) != 0 {
		payoffStr = payoff.FormatThousand(2)
	}
	snap := StateSnapshot{
		Time:      now.String(),
		Symbol:    t.Symbol.String(),
		Price:     t.Chain.Price.Format(2),
		Sigma:     t.Chain.ExpectedMove().Format(2),
		Cash:      t.Holdings.Cash.FormatThousand(2),
		Paused:    t.Paused,
		Panicking: t.Panicking,
		Greeks: GreeksData{
			Delta: delta.Format(3),
			Gamma: gamma.Format(3),
			Theta: theta.Format(3),
			Vega:  vega.Format(3),
		},
		Stats: StatsData{
			Payoff:      payoffStr,
			Liquidation: t.computeLiquidationValue().Truncate().FormatThousand(2),
			Realized:    t.Holdings.RealizedPnL.FormatThousand(2),
			Worst:       t.computeRisk().FormatThousand(2),
			Notional:    t.computeNotional().FormatThousand(2),
			Fees:        t.Holdings.TotalFees.FormatThousand(2),
			EOD:         t.computeSettlementAt(t.Chain.Price).FormatThousand(2),
			Sigma:       t.Chain.ExpectedMove().Format(2),
			Bias:        t.computeBias().Format(2),
			Error:       t.Holdings.TotalError.String(),
			Volume:      t.Holdings.Volume.String(),
		},
		Flags: FlagsData{
			Sigmas:       t.Config.Sigmas.String(),
			Budget:       fmt.Sprintf("%g", t.Config.Budget),
			Floor:        fmt.Sprintf("%g", t.Config.Floor),
			Spread:       t.Config.Spread.String(),
			Eval:         t.Config.Eval.String(),
			Cooldown:     t.Config.Cooldown.String(),
			Patience:     t.Config.Patience.String(),
			Prune:        strconv.FormatFloat(t.Config.Prune, 'f', -1, 64),
			WPayoff:      t.Config.WeightPayoff.String(),
			WRisk:        t.Config.WeightRisk.String(),
			WDelta:       t.Config.WeightDelta.String(),
			Demand:       t.Config.Demand.String(),
			BypassRisk:   boolToString(t.Config.BypassRisk),
			BypassScore:  boolToString(t.Config.BypassScore),
			BypassPayoff: boolToString(t.Config.BypassPayoff),
			Closing:      boolToString(t.Config.AllowClosing),
		},
		Strategies: make(map[string]StrategyInfo),
	}

	// find strike range from positions
	price := t.Chain.Price
	var minStrike, maxStrike decimal.Decimal
	first := true
	t.iteratePositions(func(option *options.Option, pos decimal.Decimal) {
		sp := option.Strike.Price
		if first || sp.Cmp(minStrike) < 0 {
			minStrike = sp
		}
		if first || sp.Cmp(maxStrike) > 0 {
			maxStrike = sp
		}
		first = false
	})

	// emit all strikes in the range with call/put data
	if !first {
		for it := t.Chain.Strikes.Iterator(); it.Next(); {
			strike := it.Value()
			if strike.Price.Cmp(minStrike) < 0 || strike.Price.Cmp(maxStrike) > 0 {
				continue
			}
			prob := strike.Probability().MulInt(100).Format(2)
			if strike.Call != nil {
				snap.Positions = append(snap.Positions, PositionRow{
					OSI:    strike.Call.Name(),
					Strike: strike.Price.String(),
					Class:  "C",
					Qty:    t.Holdings.GetQuantity(strike.Call).String(),
					Bid:    strike.Call.Bid.Format(2),
					Ask:    strike.Call.Ask.Format(2),
					Mid:    strike.Call.MidPrice().Format(2),
					Delta:  strike.Call.Delta.Format(4),
					Prob:   prob,
					ITM:    price.Cmp(strike.Price) > 0,
				})
			}
			if strike.Put != nil {
				snap.Positions = append(snap.Positions, PositionRow{
					OSI:    strike.Put.Name(),
					Strike: strike.Price.String(),
					Class:  "P",
					Qty:    t.Holdings.GetQuantity(strike.Put).String(),
					Bid:    strike.Put.Bid.Format(2),
					Ask:    strike.Put.Ask.Format(2),
					Mid:    strike.Put.MidPrice().Format(2),
					Delta:  strike.Put.Delta.Format(4),
					Prob:   prob,
					ITM:    price.Cmp(strike.Price) < 0,
				})
			}
		}
	}

	// risk profile across strikes within ±4 sigmas
	em := t.Chain.ExpectedMove()
	chartLo := t.Chain.Price.Sub(em.MulInt(4))
	chartHi := t.Chain.Price.Add(em.MulInt(4))
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
	}

	// strategies
	for _, name := range kStrategies {
		snap.Strategies[name] = StrategyInfo{
			Enabled: t.Config.Strategies[name],
			Count:   t.StrategiesUsed[name],
		}
	}

	return snap
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
	case "panic":
		t.PanicTime = clocky.Now()
		log.Printf("web: panicking")
	case "destroy":
		t.Config.NoHurry = true
		t.PanicTime = clocky.Now()
		log.Printf("web: destroying")
	case "rehabilitate":
		t.Panicking = false
		t.Config.NoHurry = false
		t.Config.BypassRisk = false
		t.Config.BypassScore = false
		t.Config.AllowClosing = false
		t.Config.BypassPayoff = false
		t.PanicTime = t.MarketClose.Add(-t.Config.Panic)
		t.Config.Patience = 30 * clocky.Second
		t.Config.Cooldown = 30 * clocky.Second
		for _, k := range kStrategies {
			t.Config.Strategies[k] = false
		}
		log.Printf("web: rehabilitated")
	case "flags":
		if req.Flags != nil {
			w.applyFlags(req.Flags)
		}
	case "strategies":
		for name, enabled := range req.Strats {
			t.Config.Strategies[name] = enabled
			log.Printf("web: strategy %s = %v", name, enabled)
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
	if f.Sigmas != "" {
		t.Config.Sigmas = decimal.Parse(f.Sigmas)
		log.Printf("web: sigmas = %s", f.Sigmas)
	}
	if f.Budget != "" {
		v, err := strconv.ParseFloat(f.Budget, 64)
		if err == nil {
			t.Config.Budget = v
			log.Printf("web: budget = %s", f.Budget)
		}
	}
	if f.Floor != "" {
		v, err := strconv.ParseFloat(f.Floor, 64)
		if err == nil {
			t.Config.Floor = v
			log.Printf("web: floor = %s", f.Floor)
		}
	}
	if f.Spread != "" {
		t.Config.Spread = decimal.Parse(f.Spread)
		log.Printf("web: spread = %s", f.Spread)
	}
	if f.Eval != "" {
		t.Config.Eval = decimal.Parse(f.Eval)
		log.Printf("web: eval = %s", f.Eval)
	}
	if f.Cooldown != "" {
		d, err := clocky.ParseDuration(f.Cooldown)
		if err == nil {
			t.Config.Cooldown = d
			log.Printf("web: cooldown = %s", f.Cooldown)
		}
	}
	if f.Patience != "" {
		d, err := clocky.ParseDuration(f.Patience)
		if err == nil {
			t.Config.Patience = d
			log.Printf("web: patience = %s", f.Patience)
		}
	}
	if f.Prune != "" {
		v, err := strconv.ParseFloat(f.Prune, 64)
		if err == nil && v >= 0 && v <= 1 {
			t.Config.Prune = v
			log.Printf("web: prune = %s", f.Prune)
		}
	}
	if f.WPayoff != "" {
		t.Config.WeightPayoff = decimal.Parse(f.WPayoff)
		log.Printf("web: w-payoff = %s", f.WPayoff)
	}
	if f.WRisk != "" {
		t.Config.WeightRisk = decimal.Parse(f.WRisk)
		log.Printf("web: w-risk = %s", f.WRisk)
	}
	if f.WDelta != "" {
		t.Config.WeightDelta = decimal.Parse(f.WDelta)
		log.Printf("web: w-delta = %s", f.WDelta)
	}
	if f.Demand != "" {
		t.Config.Demand = decimal.Parse(f.Demand)
		log.Printf("web: demand = %s", f.Demand)
	}
	if f.BypassRisk != "" {
		t.Config.BypassRisk = f.BypassRisk == "true"
		log.Printf("web: bypassRisk = %s", f.BypassRisk)
	}
	if f.BypassScore != "" {
		t.Config.BypassScore = f.BypassScore == "true"
		log.Printf("web: bypassScore = %s", f.BypassScore)
	}
	if f.BypassPayoff != "" {
		t.Config.BypassPayoff = f.BypassPayoff == "true"
		log.Printf("web: bypassPayoff = %s", f.BypassPayoff)
	}
	if f.Closing != "" {
		t.Config.AllowClosing = f.Closing == "true"
		log.Printf("web: closing = %s", f.Closing)
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

func (web *Web) handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	web.noCache(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"pollInterval": int((*slowdownFlag).Milliseconds()),
	})
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
	// trigger an initial broadcast
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

func (web *Web) handlePanicAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "panic", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (web *Web) handleDestroyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "destroy", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

func (web *Web) handleRehabilitateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "rehabilitate", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (web *Web) handleStrategiesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var strats map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&strats); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp := make(chan struct{})
	web.WebRequests <- WebRequest{Type: "strategies", Strats: strats, Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (web *Web) handleChainAPI(w http.ResponseWriter, r *http.Request) {
	t := web.Trader
	web.noCache(w)
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "price: %s  em: %s  atm: %v  ids: %d  osi: %d\n\n",
		t.Chain.Price, t.Chain.ExpectedMove(), t.Chain.AtTheMoney != nil,
		len(t.OptionsByID), len(t.OptionsByOSI))
	fmt.Fprintf(w, "%-8s  %5s %8s %8s %8s %5s  %5s %8s %8s %8s %5s\n",
		"strike", "C id", "C bid", "C ask", "C delta", "C mod",
		"P id", "P bid", "P ask", "P delta", "P mod")
	for it := t.Chain.Strikes.Iterator(); it.Next(); {
		s := it.Value()
		fmt.Fprintf(w, "%-8s  %5d %8s %8s %8s %5s  %5d %8s %8s %8s %5s\n",
			s.Price,
			s.Call.ID, s.Call.Bid, s.Call.Ask, s.Call.Delta, s.Call.Mode,
			s.Put.ID, s.Put.Bid, s.Put.Ask, s.Put.Delta, s.Put.Mode)
	}
}

func (web *Web) handlePositionzAPI(w http.ResponseWriter, r *http.Request) {
	t := web.Trader
	web.noCache(w)
	w.Header().Set("Content-Type", "text/plain")
	sorted := t.Holdings.Sorted()
	if len(sorted) == 0 {
		fmt.Fprintf(w, "no positions\n")
		return
	}
	fmt.Fprintf(w, "%-4s  %-30s  %8s  %8s  %8s  %8s  %8s\n",
		"qty", "option", "avg cost", "bid", "ask", "mid", "pnl")
	for _, h := range sorted {
		o := h.Option
		mid := o.MidPrice()
		pnl := mid.Sub(h.AverageCost).Mul(h.Quantity).MulInt(kMultiplier)
		fmt.Fprintf(w, "%-4s  %-30s  %8s  %8s  %8s  %8s  %8s\n",
			h.Quantity, o.StringAligned(), h.AverageCost, o.Bid, o.Ask, mid, pnl)
	}
	fmt.Fprintf(w, "\ncash: %s  realized: %s  fees: %s\n",
		t.Holdings.Cash, t.Holdings.RealizedPnL, t.Holdings.TotalFees)
}

func (web *Web) Start(t *Trader) {
	web.Trader = t
	rpID := *sldFlag
	origin := "https://" + strings.ToLower(t.Symbol.String()) + "." + *sldFlag

	go web.sseBroadcaster()

	// set up database and auth
	database := db.Get()
	if err := auth.Migrate(database); err != nil {
		log.Fatalf("web: failed to migrate auth schema: %v", err)
	}
	a, err := auth.New(database, rpID, origin)
	if err != nil {
		log.Fatalf("web: failed to initialize auth: %v", err)
	}

	// static files
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
	mux.HandleFunc("/api/config", a.RequireAuth(web.handleConfigAPI))
	mux.HandleFunc("/api/state", a.RequireAuth(web.handleStateAPI))
	mux.HandleFunc("/api/events", a.RequireAuth(web.handleEventsAPI))
	mux.HandleFunc("/api/flags", a.RequireAuthReadOnly(web.handleFlagsAPI))
	mux.HandleFunc("/api/pause", a.RequireAuthReadOnly(web.handlePauseAPI))
	mux.HandleFunc("/api/panic", a.RequireAuthReadOnly(web.handlePanicAPI))
	mux.HandleFunc("/api/destroy", a.RequireAuthReadOnly(web.handleDestroyAPI))
	mux.HandleFunc("/api/resume", a.RequireAuthReadOnly(web.handleResumeAPI))
	mux.HandleFunc("/api/strategies", a.RequireAuthReadOnly(web.handleStrategiesAPI))
	mux.HandleFunc("/api/rehabilitate", a.RequireAuthReadOnly(web.handleRehabilitateAPI))

	// monitoring tools
	mux.HandleFunc("/chainz", a.RequireAuth(web.handleChainAPI))
	mux.HandleFunc("/positionz", a.RequireAuth(web.handlePositionzAPI))

	listen := *listenFlag
	if t.Config.Listen != "" {
		listen = t.Config.Listen
	}
	sock, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("web server listen error: %v", err)
	}
	log.Printf("dashboard for %s at http://%s", t.Symbol, sock.Addr())
	go http.Serve(sock, mux)
}
