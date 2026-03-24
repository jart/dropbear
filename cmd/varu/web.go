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
	"sync"
)

//go:embed static
var staticFiles embed.FS

var (
	listenFlag = flag.String("listen", "localhost:8484", "web dashboard bind address")
	rpIDFlag   = flag.String("rpid", "dropbear.justine.lol", "WebAuthn relying party ID (domain)")
	originFlag = flag.String("origin", "https://dropbear.justine.lol", "WebAuthn origin URL")
)

// gPaused controls whether onThink runs.
var gPaused bool

// gStrategyEnabled controls which strategies are allowed to simulate.
var gStrategyEnabled = map[string]bool{
	"buy call":           true,
	"buy put":            true,
	"buy combo":          true,
	"sell combo":         true,
	"sell call vertical": true,
	"sell put vertical":  true,
	"buy call vertical":  true,
	"buy put vertical":   true,
	"sell condor":        true,
	"buy condor":         true,
}

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
}

type FlagsData struct {
	Sigmas   string `json:"sigmas"`
	Budget   string `json:"budget"`
	Floor    string `json:"floor"`
	Spread   string `json:"spread"`
	Cooldown string `json:"cooldown"`
	Patience string `json:"patience"`
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
		Price:  gChain.Price.String(),
		Cash:   gCash.Format(2),
		Paused: gPaused,
		Greeks: GreeksData{
			Delta: delta.Format(2),
			Gamma: gamma.Format(2),
			Theta: theta.Format(2),
			Vega:  vega.Format(2),
		},
		Stats: StatsData{
			Payoff:      computeExpectedPayoff().Truncate().String(),
			Worst:       computeRisk().String(),
			Liquidation: computeLiquidationValue().Truncate().String(),
			EOD:         computeSettlementAt(gChain.Price).String(),
		},
		Flags: FlagsData{
			Sigmas:   (*sigmasFlag).String(),
			Budget:   (*budgetFlag).String(),
			Floor:    (*floorFlag).String(),
			Spread:   (*spreadFlag).String(),
			Cooldown: (*cooldownFlag).String(),
			Patience: (*patienceFlag).String(),
		},
		Strategies: make(map[string]StrategyInfo),
	}

	// positions
	iteratePositions(func(sym string, pos decimal.Decimal) {
		o := gOptionsByOSI[sym]
		snap.Positions = append(snap.Positions, PositionRow{
			OSI:    sym,
			Strike: o.Strike.Price.String(),
			Class:  string(o.Class),
			Qty:    pos.String(),
			Bid:    o.Bid.String(),
			Ask:    o.Ask.String(),
			Mid:    o.MarketPrice().String(),
			Delta:  o.Delta.Format(4),
		})
	})

	// risk profile across all strikes
	for it := gChain.Strikes.Iterator(); it.Next(); {
		strike := it.Value()
		settlement := computeSettlementAt(strike.Price)
		snap.Risk = append(snap.Risk, RiskPoint{
			Strike:     strike.Price.String(),
			Settlement: settlement.String(),
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

// broadcastState builds a snapshot and sends it to SSE clients.
// Must be called from the main goroutine.
func broadcastState() {
	data, err := json.Marshal(buildStateSnapshot())
	if err != nil {
		return
	}
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
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
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
