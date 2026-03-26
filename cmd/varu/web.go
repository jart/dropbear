package main

import (
	"dropbear/auth"
	"dropbear/clocky"
	"dropbear/db"
	"dropbear/decimal"
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
)

//go:embed static
var staticFiles embed.FS

var (
	listenFlag = flag.String("listen", "localhost:8484", "web dashboard bind address")
	rpIDFlag   = flag.String("rpid", "varu.justinestreet.capital", "WebAuthn relying party ID (domain)")
	originFlag = flag.String("origin", "https://varu.justinestreet.capital", "WebAuthn origin URL")
)

// gPaused controls whether onThink runs.
var gPaused bool

// WebRequest is a command sent from the web server to the main goroutine.
type WebRequest struct {
	Type     string          // "flags", "pause", "resume", "strategies"
	Flags    *FlagsData      // for "flags"
	Strats   map[string]bool // for "strategies"
	Response chan struct{}   // closed when processed
}

// gWebRequests is the channel for web requests to the main goroutine.
var gWebRequests = make(chan WebRequest, 8)

// gSSEBroadcast is the channel for state snapshots to SSE clients.
var gSSEBroadcast = make(chan []byte, 4)

// SSE subscriber management
var (
	sseMu          sync.Mutex
	sseSubscribers = map[chan []byte]struct{}{}
)

func sseSubscribe() chan []byte {
	ch := make(chan []byte, 4)
	sseMu.Lock()
	sseSubscribers[ch] = struct{}{}
	sseMu.Unlock()
	return ch
}

func sseUnsubscribe(ch chan []byte) {
	sseMu.Lock()
	delete(sseSubscribers, ch)
	sseMu.Unlock()
}

func sseBroadcaster() {
	for data := range gSSEBroadcast {
		sseMu.Lock()
		for ch := range sseSubscribers {
			select {
			case ch <- data:
			default:
				// drop if subscriber is slow
			}
		}
		sseMu.Unlock()
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
	EOD         string `json:"eod"`
	Sigma       string `json:"sigma"`
	Bias        string `json:"bias"`
	Realized    string `json:"realized"`
	Drift       string `json:"drift"`
}

type FlagsData struct {
	Sigmas   string `json:"sigmas"`
	Budget   string `json:"budget"`
	Floor    string `json:"floor"`
	Spread   string `json:"spread"`
	Cooldown string `json:"cooldown"`
	Patience string `json:"patience"`
	Prune    string `json:"prune"`
	WPayoff  string `json:"wPayoff"`
	WRisk    string `json:"wRisk"`
	WDelta   string `json:"wDelta"`
}

type StrategyInfo struct {
	Enabled bool `json:"enabled"`
	Count   int  `json:"count"`
}

// buildStateSnapshot serializes all state into a snapshot.
// Must be called from the main goroutine.
func buildStateSnapshot() StateSnapshot {
	now := clocky.Now()
	delta, gamma, theta, vega := computeGreeks()
	snap := StateSnapshot{
		Time:   now.String(),
		Symbol: gSymbol.String(),
		Price:  gChain.Price.Format(2),
		Sigma:  gChain.ExpectedMove().Format(2),
		Cash:   gCash.Format(2),
		Paused: gPaused,
		Greeks: GreeksData{
			Delta: delta.Format(3),
			Gamma: gamma.Format(3),
			Theta: theta.Format(3),
			Vega:  vega.Format(3),
		},
		Stats: StatsData{
			Payoff:      computeExpectedPayoff().Format(2),
			Worst:       computeRisk().Format(2),
			Liquidation: computeLiquidationValue().Truncate().Format(2),
			EOD:         computeSettlementAt(gChain.Price).Format(2),
			Sigma:       gChain.ExpectedMove().Format(2),
			Bias:        computeBias().Format(2),
			Realized:    gRealizedPnL.Format(2),
			Drift:       gDrift.String(),
		},
		Flags: FlagsData{
			Sigmas:   (*sigmasFlag).String(),
			Budget:   (*budgetFlag).String(),
			Floor:    (*floorFlag).String(),
			Spread:   (*spreadFlag).String(),
			Cooldown: (*cooldownFlag).String(),
			Patience: (*patienceFlag).String(),
			Prune:    strconv.FormatFloat(*pruneFlag, 'f', -1, 64),
			WPayoff:  (*wPayoffFlag).String(),
			WRisk:    (*wRiskFlag).String(),
			WDelta:   (*wDeltaFlag).String(),
		},
		Strategies: make(map[string]StrategyInfo),
	}

	// find strike range from positions
	price := gChain.Price
	var minStrike, maxStrike decimal.Decimal
	first := true
	iteratePositions(func(sym string, pos decimal.Decimal) {
		o := gOptionsByOSI[sym]
		sp := o.Strike.Price
		if first || sp.Cmp(minStrike) < 0 {
			minStrike = sp
		}
		if first || sp.Cmp(maxStrike) > 0 {
			maxStrike = sp
		}
		first = false
	})

	// build position lookup
	posMap := map[string]decimal.Decimal{}
	iteratePositions(func(sym string, pos decimal.Decimal) {
		posMap[sym] = pos
	})

	// emit all strikes in the range with call/put data
	if !first {
		for it := gChain.Strikes.Iterator(); it.Next(); {
			strike := it.Value()
			if strike.Price.Cmp(minStrike) < 0 || strike.Price.Cmp(maxStrike) > 0 {
				continue
			}
			prob := strike.Probability().MulInt(100).Format(2)
			if strike.Call != nil {
				callQty := posMap[strike.Call.OSI()]
				snap.Positions = append(snap.Positions, PositionRow{
					OSI:    strike.Call.OSI(),
					Strike: strike.Price.String(),
					Class:  "C",
					Qty:    callQty.String(),
					Bid:    strike.Call.Bid.Format(2),
					Ask:    strike.Call.Ask.Format(2),
					Mid:    strike.Call.MarketPrice().Format(2),
					Delta:  strike.Call.Delta.Format(4),
					Prob:   prob,
					ITM:    price.Cmp(strike.Price) > 0,
				})
			}
			if strike.Put != nil {
				putQty := posMap[strike.Put.OSI()]
				snap.Positions = append(snap.Positions, PositionRow{
					OSI:    strike.Put.OSI(),
					Strike: strike.Price.String(),
					Class:  "P",
					Qty:    putQty.String(),
					Bid:    strike.Put.Bid.Format(2),
					Ask:    strike.Put.Ask.Format(2),
					Mid:    strike.Put.MarketPrice().Format(2),
					Delta:  strike.Put.Delta.Format(4),
					Prob:   prob,
					ITM:    price.Cmp(strike.Price) < 0,
				})
			}
		}
	}

	// risk profile across strikes within ±4 sigmas
	em := gChain.ExpectedMove()
	chartLo := gChain.Price.Sub(em.MulInt(4))
	chartHi := gChain.Price.Add(em.MulInt(4))
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		if strike.Price.Cmp(chartLo) < 0 || strike.Price.Cmp(chartHi) > 0 {
			continue
		}
		settlement := computeSettlementAt(strike.Price)
		snap.Risk = append(snap.Risk, RiskPoint{
			Strike:     strike.Price.Format(2),
			Settlement: settlement.Format(2),
		})
	}

	// strategies
	for name, enabled := range gStrategyEnabled {
		count, _ := gStrategiesUsed.Get(name)
		snap.Strategies[name] = StrategyInfo{
			Enabled: enabled,
			Count:   count,
		}
	}

	return snap
}

var (
	lastSnapshotMu   sync.RWMutex
	lastSnapshotJSON []byte
)

// broadcastState builds a snapshot and caches it for polling.
// Must be called from the main goroutine.
func broadcastState() {
	data, err := json.Marshal(buildStateSnapshot())
	if err != nil {
		return
	}
	lastSnapshotMu.Lock()
	lastSnapshotJSON = data
	lastSnapshotMu.Unlock()
	select {
	case gSSEBroadcast <- data:
	default:
	}
}

// processWebRequest handles a web request on the main goroutine.
func processWebRequest(req WebRequest) {
	switch req.Type {
	case "pause":
		gPaused = true
		log.Printf("web: paused")
	case "resume":
		gPaused = false
		log.Printf("web: resumed")
	case "flags":
		if req.Flags != nil {
			applyFlags(req.Flags)
		}
	case "strategies":
		for name, enabled := range req.Strats {
			gStrategyEnabled[name] = enabled
			log.Printf("web: strategy %s = %v", name, enabled)
		}
	case "broadcast":
		// just broadcast state, handled below
	}
	broadcastState()
	if req.Response != nil {
		close(req.Response)
	}
}

func applyFlags(f *FlagsData) {
	if f.Sigmas != "" {
		*sigmasFlag = decimal.Parse(f.Sigmas)
		log.Printf("web: sigmas = %s", f.Sigmas)
	}
	if f.Budget != "" {
		*budgetFlag = decimal.Parse(f.Budget)
		gRiskBudget = (*budgetFlag).Float64()
		log.Printf("web: budget = %s", f.Budget)
	}
	if f.Floor != "" {
		*floorFlag = decimal.Parse(f.Floor)
		gRiskFloor = (*floorFlag).Float64()
		log.Printf("web: floor = %s", f.Floor)
	}
	if f.Spread != "" {
		*spreadFlag = decimal.Parse(f.Spread)
		log.Printf("web: spread = %s", f.Spread)
	}
	if f.Cooldown != "" {
		d, err := clocky.ParseDuration(f.Cooldown)
		if err == nil {
			*cooldownFlag = d
			log.Printf("web: cooldown = %s", f.Cooldown)
		}
	}
	if f.Patience != "" {
		d, err := clocky.ParseDuration(f.Patience)
		if err == nil {
			*patienceFlag = d
			log.Printf("web: patience = %s", f.Patience)
		}
	}
	if f.Prune != "" {
		v, err := strconv.ParseFloat(f.Prune, 64)
		if err == nil && v >= 0 && v <= 1 {
			*pruneFlag = v
			log.Printf("web: prune = %s", f.Prune)
		}
	}
	if f.WPayoff != "" {
		*wPayoffFlag = decimal.Parse(f.WPayoff)
		log.Printf("web: w-payoff = %s", f.WPayoff)
	}
	if f.WRisk != "" {
		*wRiskFlag = decimal.Parse(f.WRisk)
		log.Printf("web: w-risk = %s", f.WRisk)
	}
	if f.WDelta != "" {
		*wDeltaFlag = decimal.Parse(f.WDelta)
		log.Printf("web: w-delta = %s", f.WDelta)
	}
}

// HTTP handlers

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	noCache(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"pollInterval": int((*slowdownFlag).Milliseconds()),
	})
}

func handleStateAPI(w http.ResponseWriter, r *http.Request) {
	lastSnapshotMu.RLock()
	data := lastSnapshotJSON
	lastSnapshotMu.RUnlock()
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	if data != nil {
		w.Write(data)
	} else {
		w.Write([]byte("{}"))
	}
}

func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func handleEventsAPI(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := sseSubscribe()
	defer sseUnsubscribe(ch)
	// trigger an initial broadcast
	gWebRequests <- WebRequest{Type: "broadcast"}
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

func handleFlagsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var flags FlagsData
	if err := json.NewDecoder(r.Body).Decode(&flags); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp := make(chan struct{})
	gWebRequests <- WebRequest{Type: "flags", Flags: &flags, Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handlePauseAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	gWebRequests <- WebRequest{Type: "pause", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "paused"})
}

func handleResumeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := make(chan struct{})
	gWebRequests <- WebRequest{Type: "resume", Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resumed"})
}

func handleStrategiesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var strats map[string]bool
	if err := json.NewDecoder(r.Body).Decode(&strats); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp := make(chan struct{})
	gWebRequests <- WebRequest{Type: "strategies", Strats: strats, Response: resp}
	<-resp
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func startWeb() {
	go sseBroadcaster()

	// set up database and auth
	database := db.Get()
	if err := auth.Migrate(database); err != nil {
		log.Fatalf("web: failed to migrate auth schema: %v", err)
	}
	a, err := auth.New(database, *rpIDFlag, *originFlag)
	if err != nil {
		log.Fatalf("web: failed to initialize auth: %v", err)
	}

	// static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// public auth routes
	http.Handle("/auth/static/", http.StripPrefix("/auth/static/", auth.StaticHandler()))
	http.HandleFunc("/login", a.HandleLoginPage)
	http.HandleFunc("/register", a.HandleRegisterPage)
	http.HandleFunc("/logout", a.HandleLogout)
	http.HandleFunc("/auth/register/begin", a.HandleRegisterBegin)
	http.HandleFunc("/auth/register/finish", a.HandleRegisterFinish)
	http.HandleFunc("/auth/login/begin", a.HandleLoginBegin)
	http.HandleFunc("/auth/login/finish", a.HandleLoginFinish)

	// admin routes
	http.HandleFunc("/admin/invites", a.RequireAdmin(a.HandleInvitesPage))
	http.HandleFunc("/auth/invites/create", a.RequireAdmin(a.HandleCreateInvite))
	http.HandleFunc("/auth/invites/list", a.RequireAdmin(a.HandleListInvites))

	// protected dashboard routes
	http.HandleFunc("/", a.RequireAuth(handleIndex))
	http.HandleFunc("/api/config", a.RequireAuth(handleConfigAPI))
	http.HandleFunc("/api/state", a.RequireAuth(handleStateAPI))
	http.HandleFunc("/api/events", a.RequireAuth(handleEventsAPI))
	http.HandleFunc("/api/flags", a.RequireAuth(handleFlagsAPI))
	http.HandleFunc("/api/pause", a.RequireAuth(handlePauseAPI))
	http.HandleFunc("/api/resume", a.RequireAuth(handleResumeAPI))
	http.HandleFunc("/api/strategies", a.RequireAuth(handleStrategiesAPI))

	sock, err := net.Listen("tcp", *listenFlag)
	if err != nil {
		log.Fatalf("web server listen error: %v", err)
	}
	log.Printf("dashboard at http://%s", sock.Addr())
	go http.Serve(sock, nil)
}
