package main

import (
	"database/sql"
	"dropbear/db"
	"dropbear/decimal"
	"embed"
	"encoding/json"
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
	sseMu          sync.Mutex
	sseSubscribers = map[chan []byte]struct{}{}
	sseBroadcastCh = make(chan []byte, 4)
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
	for data := range sseBroadcastCh {
		sseMu.Lock()
		for ch := range sseSubscribers {
			select {
			case ch <- data:
			default:
			}
		}
		sseMu.Unlock()
	}
}

func broadcastRunStatus(runID int64, status string) {
	data, _ := json.Marshal(map[string]any{
		"type":   "run_status",
		"run_id": runID,
		"status": status,
	})
	select {
	case sseBroadcastCh <- data:
	default:
	}
}

func broadcastState(database *sql.DB) {
	state := buildDashboardState(database)
	data, _ := json.Marshal(state)
	select {
	case sseBroadcastCh <- data:
	default:
	}
}

type DashboardState struct {
	Type     string       `json:"type"`
	Counts   RunCounts    `json:"counts"`
	Active   []RunSummary `json:"active"`
	Recent   []RunSummary `json:"recent"`
	Flags    []string     `json:"flags"`
}

type RunCounts struct {
	Pending int `json:"pending"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
}

type RunSummary struct {
	ID      int64  `json:"id"`
	Symbol  string `json:"symbol"`
	Date    string `json:"date"`
	Flags   string `json:"flags"`
	Status  string `json:"status"`
	Winning string `json:"winning,omitempty"`
}

func buildDashboardState(database *sql.DB) DashboardState {
	state := DashboardState{Type: "state"}

	database.QueryRow(`SELECT COUNT(*) FROM varulab_runs WHERE status IN ('pending','claimed')`).Scan(&state.Counts.Pending)
	database.QueryRow(`SELECT COUNT(*) FROM varulab_runs WHERE status = 'running'`).Scan(&state.Counts.Running)
	database.QueryRow(`SELECT COUNT(*) FROM varulab_runs WHERE status = 'done'`).Scan(&state.Counts.Done)
	database.QueryRow(`SELECT COUNT(*) FROM varulab_runs WHERE status = 'failed'`).Scan(&state.Counts.Failed)

	// active runs
	rows, _ := database.Query(`SELECT id, symbol, date, flags, status FROM varulab_runs WHERE status = 'running' ORDER BY started_at DESC LIMIT 50`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var r RunSummary
			rows.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status)
			state.Active = append(state.Active, r)
		}
	}

	// recent completed
	rows2, _ := database.Query(`SELECT id, symbol, date, flags, status, winning FROM varulab_runs WHERE status IN ('done','failed') ORDER BY finished_at DESC LIMIT 20`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var r RunSummary
			var winning sql.NullInt64
			rows2.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status, &winning)
			if winning.Valid {
				r.Winning = decimal.Decimal(winning.Int64).Format(2)
			}
			state.Recent = append(state.Recent, r)
		}
	}

	state.Flags, _ = generateFlagCombinations()
	return state
}

// Grid data

type GridCell struct {
	Symbol  string `json:"symbol"`
	Date    string `json:"date"`
	Winning string `json:"winning"`
	Flags   string `json:"flags"`
	Status  string `json:"status"`
	RunID   int64  `json:"run_id"`
}

func queryGrid(database *sql.DB) []GridCell {
	// best winning per (symbol, date) across all flag combos
	rows, err := database.Query(`
		SELECT r.id, r.symbol, r.date, r.flags, r.status, r.winning
		FROM varulab_runs r
		INNER JOIN (
			SELECT symbol, date, MAX(winning) as max_winning
			FROM varulab_runs
			WHERE status = 'done'
			GROUP BY symbol, date
		) best ON r.symbol = best.symbol AND r.date = best.date AND r.winning = best.max_winning
		ORDER BY r.date DESC, r.symbol
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var cells []GridCell
	for rows.Next() {
		var c GridCell
		var winning sql.NullInt64
		rows.Scan(&c.RunID, &c.Symbol, &c.Date, &c.Flags, &c.Status, &winning)
		if winning.Valid {
			c.Winning = decimal.Decimal(winning.Int64).Format(2)
		}
		cells = append(cells, c)
	}
	return cells
}

type FlagSummary struct {
	Flags      string `json:"flags"`
	AvgWinning string `json:"avg_winning"`
	BestWin    string `json:"best_win"`
	WorstLoss  string `json:"worst_loss"`
	Median     string `json:"median"`
	StdDev     string `json:"std_dev"`
	Sharpe     string `json:"sharpe"`
	WinRate    string `json:"win_rate"`
	Count      int    `json:"count"`
}

type SymbolSummary struct {
	Symbol     string `json:"symbol"`
	BestFlags  string `json:"best_flags"`
	AvgWinning string `json:"avg_winning"`
	Count      int    `json:"count"`
}

func querySummary(database *sql.DB) ([]FlagSummary, []SymbolSummary) {
	var flagSummaries []FlagSummary
	rows, _ := database.Query(`
		SELECT flags,
			AVG(winning) as avg_w,
			MAX(winning) as best_w,
			MIN(winning) as worst_w,
			CAST(SUM(CASE WHEN winning > 0 THEN 1 ELSE 0 END) AS REAL) / COUNT(*) as win_rate,
			COUNT(*) as n
		FROM varulab_runs WHERE status = 'done'
		GROUP BY flags ORDER BY avg_w DESC
	`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var fs FlagSummary
			var avgW, bestW, worstW float64
			var winRate float64
			rows.Scan(&fs.Flags, &avgW, &bestW, &worstW, &winRate, &fs.Count)
			fs.AvgWinning = decimal.Decimal(int64(avgW)).Format(2)
			fs.BestWin = decimal.Decimal(int64(bestW)).Format(2)
			fs.WorstLoss = decimal.Decimal(int64(worstW)).Format(2)
			fs.WinRate = strconv.FormatFloat(winRate*100, 'f', 1, 64) + "%"
			flagSummaries = append(flagSummaries, fs)
		}
	}
	// second pass: compute stddev and sharpe per flag combo
	for i := range flagSummaries {
		fs := &flagSummaries[i]
		var stddev float64
		database.QueryRow(`
			SELECT COALESCE(SQRT(AVG((winning - ?) * (winning - ?))), 0)
			FROM varulab_runs WHERE status = 'done' AND flags = ?
		`, fs.AvgWinning, fs.AvgWinning, fs.Flags).Scan(&stddev) // note: this uses string avg, close enough
		// recompute properly with float avg
		var avgF float64
		database.QueryRow(`SELECT AVG(winning) FROM varulab_runs WHERE status = 'done' AND flags = ?`, fs.Flags).Scan(&avgF)
		database.QueryRow(`
			SELECT COALESCE(SQRT(AVG((winning - ?) * (winning - ?))), 0)
			FROM varulab_runs WHERE status = 'done' AND flags = ?
		`, avgF, avgF, fs.Flags).Scan(&stddev)
		fs.StdDev = decimal.Decimal(int64(stddev)).Format(2)
		if stddev > 0 {
			fs.Sharpe = strconv.FormatFloat(avgF/stddev, 'f', 2, 64)
		} else {
			fs.Sharpe = "-"
		}
		// median
		var median float64
		database.QueryRow(`
			SELECT winning FROM varulab_runs WHERE status = 'done' AND flags = ?
			ORDER BY winning LIMIT 1 OFFSET ?
		`, fs.Flags, fs.Count/2).Scan(&median)
		fs.Median = decimal.Decimal(int64(median)).Format(2)
	}

	var symbolSummaries []SymbolSummary
	rows2, _ := database.Query(`
		SELECT symbol, flags, AVG(winning) as avg_w, COUNT(*) as n
		FROM varulab_runs WHERE status = 'done'
		GROUP BY symbol, flags
		ORDER BY symbol, avg_w DESC
	`)
	if rows2 != nil {
		defer rows2.Close()
		seen := map[string]bool{}
		for rows2.Next() {
			var ss SymbolSummary
			var flags string
			var avgW float64
			rows2.Scan(&ss.Symbol, &flags, &avgW, &ss.Count)
			if !seen[ss.Symbol] {
				seen[ss.Symbol] = true
				ss.BestFlags = flags
				ss.AvgWinning = decimal.Decimal(int64(avgW)).Format(2)
				symbolSummaries = append(symbolSummaries, ss)
			}
		}
	}
	return flagSummaries, symbolSummaries
}

// HTTP handlers

func startWeb() {
	go sseBroadcaster()

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/events", handleEvents)
	http.HandleFunc("/api/state", handleState)
	http.HandleFunc("/api/grid", handleGrid)
	http.HandleFunc("/api/summary", handleSummaryAPI)
	http.HandleFunc("/api/runs", handleRuns)
	http.HandleFunc("/api/flagsets", handleFlagSets)
	http.HandleFunc("/api/regenerate", handleRegenerate)

	sock, err2 := net.Listen("tcp", *listenFlag)
	if err2 != nil {
		log.Fatalf("web: listen: %v", err2)
	}
	log.Printf("dashboard at http://%s", sock.Addr())
	go http.Serve(sock, nil)
}

func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	noCache(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := sseSubscribe()
	defer sseUnsubscribe(ch)
	// send initial state
	broadcastState(db.Get())
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

func handleState(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildDashboardState(db.Get()))
}

func handleGrid(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queryGrid(db.Get()))
}

func handleSummaryAPI(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	flagS, symS := querySummary(db.Get())
	json.NewEncoder(w).Encode(map[string]any{
		"flags":   flagS,
		"symbols": symS,
	})
}

func handleRuns(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		var logText sql.NullString
		var run RunSummary
		var winning sql.NullInt64
		db.Get().QueryRow(`SELECT id, symbol, date, flags, status, winning, log FROM varulab_runs WHERE id = ?`, id).
			Scan(&run.ID, &run.Symbol, &run.Date, &run.Flags, &run.Status, &winning, &logText)
		if winning.Valid {
			run.Winning = decimal.Decimal(winning.Int64).Format(2)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"run": run,
			"log": logText.String,
		})
		return
	}
	// list recent
	rows, _ := db.Get().Query(`SELECT id, symbol, date, flags, status, winning FROM varulab_runs ORDER BY id DESC LIMIT 100`)
	var runs []RunSummary
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var r RunSummary
			var winning sql.NullInt64
			rows.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status, &winning)
			if winning.Valid {
				r.Winning = decimal.Decimal(winning.Int64).Format(2)
			}
			runs = append(runs, r)
		}
	}
	json.NewEncoder(w).Encode(runs)
}

func handleFlagSets(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	combos, _ := generateFlagCombinations()
	json.NewEncoder(w).Encode(map[string]any{
		"dimensions":   kFlagDimensions,
		"combinations": len(combos),
	})
}

func handleRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	n := gScheduler.GenerateRuns(gGitRev)
	notifyScheduler()
	broadcastState(db.Get())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"created": n})
}
