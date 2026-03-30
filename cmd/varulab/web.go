package main

import (
	"database/sql"
	"dropbear/db"
	"dropbear/decimal"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"sort"
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
	Type    string       `json:"type"`
	Counts  RunCounts    `json:"counts"`
	Active  []RunSummary `json:"active"`
	Recent  []RunSummary `json:"recent"`
	Pending []RunSummary `json:"pending"`
	Failed  []RunSummary `json:"failed"`
	Flags   []string     `json:"flags"`
}

type RunCounts struct {
	Pending int `json:"pending"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
}

type RunSummary struct {
	ID         int64  `json:"id"`
	Symbol     string `json:"symbol"`
	Date       string `json:"date"`
	Flags      string `json:"flags"`
	Status     string `json:"status"`
	Winning    string `json:"winning,omitempty"`
	StartedAt  int64  `json:"started_at,omitempty"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

func buildDashboardState(database *sql.DB) DashboardState {
	state := DashboardState{Type: "state"}

	database.QueryRow(`SELECT
		SUM(CASE WHEN status IN ('pending','claimed') THEN 1 ELSE 0 END),
		SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END)
		FROM varulab_runs`).Scan(&state.Counts.Pending, &state.Counts.Running, &state.Counts.Done, &state.Counts.Failed)

	// active runs
	rows, _ := database.Query(`SELECT id, symbol, date, flags, status, started_at FROM varulab_runs WHERE status = 'running' ORDER BY started_at DESC LIMIT 50`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var r RunSummary
			var startedAt sql.NullInt64
			rows.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status, &startedAt)
			if startedAt.Valid {
				r.StartedAt = startedAt.Int64
			}
			state.Active = append(state.Active, r)
		}
	}

	// recent completed
	rows2, _ := database.Query(`SELECT id, symbol, date, flags, status, winning, finished_at FROM varulab_runs WHERE status IN ('done','failed') ORDER BY finished_at DESC LIMIT 20`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var r RunSummary
			var winning, finishedAt sql.NullInt64
			rows2.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status, &winning, &finishedAt)
			if winning.Valid {
				r.Winning = decimal.Decimal(winning.Int64).Format(2)
			}
			if finishedAt.Valid {
				r.FinishedAt = finishedAt.Int64
			}
			state.Recent = append(state.Recent, r)
		}
	}

	// pending runs
	rows3, _ := database.Query(`SELECT id, symbol, date, flags, status FROM varulab_runs WHERE status IN ('pending','claimed') ORDER BY id LIMIT 20`)
	if rows3 != nil {
		defer rows3.Close()
		for rows3.Next() {
			var r RunSummary
			rows3.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status)
			state.Pending = append(state.Pending, r)
		}
	}

	// recent failed runs
	rows4, _ := database.Query(`SELECT id, symbol, date, flags, status, finished_at FROM varulab_runs WHERE status = 'failed' ORDER BY finished_at DESC LIMIT 20`)
	if rows4 != nil {
		defer rows4.Close()
		for rows4.Next() {
			var r RunSummary
			var finishedAt sql.NullInt64
			rows4.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.Status, &finishedAt)
			if finishedAt.Valid {
				r.FinishedAt = finishedAt.Int64
			}
			state.Failed = append(state.Failed, r)
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

func queryGrid(database *sql.DB, dateFrom, dateTo, sym string) []GridCell {
	// best winning per (symbol, date) across all flag combos
	rows, err := database.Query(`
		SELECT r.id, r.symbol, r.date, r.flags, r.status, r.winning
		FROM varulab_runs r
		INNER JOIN (
			SELECT symbol, date, MAX(winning) as max_winning
			FROM varulab_runs
			WHERE status = 'done' AND date >= ? AND date <= ? AND symbol LIKE ?
			GROUP BY symbol, date
		) best ON r.symbol = best.symbol AND r.date = best.date AND r.winning = best.max_winning
		ORDER BY r.date DESC, r.symbol
	`, dateFrom, dateTo, sym)
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
	Flags      string    `json:"flags"`
	AvgWinning string    `json:"avg_winning"`
	AvgFees    string    `json:"avg_fees"`
	BestWin    string    `json:"best_win"`
	WorstLoss  string    `json:"worst_loss"`
	Median     string    `json:"median"`
	StdDev     string    `json:"std_dev"`
	Sharpe     string    `json:"sharpe"`
	WinRate    string    `json:"win_rate"`
	Count      int       `json:"count"`
	Histogram  []float64 `json:"histogram"`
}

type SymbolSummary struct {
	Symbol     string `json:"symbol"`
	BestFlags  string `json:"best_flags"`
	AvgWinning string `json:"avg_winning"`
	Count      int    `json:"count"`
}

func queryFilter(r *http.Request) (string, string, string) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	sym := r.URL.Query().Get("symbol")
	if from == "" {
		from = "2000-01-01"
	}
	if to == "" {
		to = "2099-12-31"
	}
	if sym == "" {
		sym = "%"
	}
	return from, to, sym
}

func querySummary(database *sql.DB, dateFrom, dateTo, sym string) ([]FlagSummary, []SymbolSummary) {
	// single query: fetch all (flags, winning, fees) rows, group in Go
	rows, _ := database.Query(`
		SELECT flags, winning, fees
		FROM varulab_runs WHERE status = 'done' AND date >= ? AND date <= ? AND symbol LIKE ?
		ORDER BY flags, winning
	`, dateFrom, dateTo, sym)
	type bucket struct {
		values []float64
		feeSum float64
		wins   int
	}
	buckets := map[string]*bucket{}
	var bucketOrder []string
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var flags string
			var winning, fees float64
			rows.Scan(&flags, &winning, &fees)
			b := buckets[flags]
			if b == nil {
				b = &bucket{}
				buckets[flags] = b
				bucketOrder = append(bucketOrder, flags)
			}
			b.values = append(b.values, winning) // already sorted by ORDER BY
			b.feeSum += fees
			if winning > 0 {
				b.wins++
			}
		}
	}
	var flagSummaries []FlagSummary
	for _, flags := range bucketOrder {
		b := buckets[flags]
		values := b.values
		n := len(values)
		if n == 0 {
			continue
		}
		var sum float64
		for _, v := range values {
			sum += v
		}
		mean := sum / float64(n)
		var variance float64
		for _, v := range values {
			d := v - mean
			variance += d * d
		}
		stddev := math.Sqrt(variance / float64(n))
		fs := FlagSummary{
			Flags:      flags,
			AvgWinning: decimal.Decimal(int64(mean)).Format(2),
			AvgFees:    decimal.Decimal(int64(b.feeSum / float64(n))).Format(2),
			BestWin:    decimal.Decimal(int64(values[n-1])).Format(2),
			WorstLoss:  decimal.Decimal(int64(values[0])).Format(2),
			Median:     decimal.Decimal(int64(values[n/2])).Format(2),
			StdDev:     strconv.FormatFloat(stddev/1e6, 'f', 2, 64),
			WinRate:    strconv.FormatFloat(float64(b.wins)/float64(n)*100, 'f', 1, 64) + "%",
			Count:      n,
		}
		if stddev > 0 {
			fs.Sharpe = strconv.FormatFloat(mean/stddev*math.Sqrt(252), 'f', 2, 64)
		} else {
			fs.Sharpe = "-"
		}
		hist := make([]float64, n)
		for j, v := range values {
			hist[j] = v / 1e6
		}
		fs.Histogram = hist
		flagSummaries = append(flagSummaries, fs)
	}
	sort.Slice(flagSummaries, func(i, j int) bool {
		si, _ := strconv.ParseFloat(flagSummaries[i].AvgWinning, 64)
		sj, _ := strconv.ParseFloat(flagSummaries[j].AvgWinning, 64)
		return si > sj
	})

	var symbolSummaries []SymbolSummary
	rows2, _ := database.Query(`
		SELECT symbol, flags, AVG(winning) as avg_w, COUNT(*) as n
		FROM varulab_runs WHERE status = 'done' AND date >= ? AND date <= ? AND symbol LIKE ?
		GROUP BY symbol, flags
		ORDER BY symbol, avg_w DESC
	`, dateFrom, dateTo, sym)
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
	http.HandleFunc("/api/dates", handleDatesAPI)
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
	from, to, sym := queryFilter(r)
	json.NewEncoder(w).Encode(queryGrid(db.Get(), from, to, sym))
}

func handleSummaryAPI(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	from, to, sym := queryFilter(r)
	flagS, symS := querySummary(db.Get(), from, to, sym)
	json.NewEncoder(w).Encode(map[string]any{
		"flags":   flagS,
		"symbols": symS,
	})
}

func handleDatesAPI(w http.ResponseWriter, r *http.Request) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	var minDate, maxDate string
	db.Get().QueryRow(`SELECT MIN(date), MAX(date) FROM varulab_runs WHERE status = 'done'`).Scan(&minDate, &maxDate)
	var dates []string
	rows, _ := db.Get().Query(`SELECT DISTINCT date FROM varulab_runs WHERE status = 'done' ORDER BY date`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			rows.Scan(&d)
			dates = append(dates, d)
		}
	}
	var symbols []string
	srows, _ := db.Get().Query(`SELECT DISTINCT symbol FROM varulab_runs WHERE status = 'done' ORDER BY symbol`)
	if srows != nil {
		defer srows.Close()
		for srows.Next() {
			var s string
			srows.Scan(&s)
			symbols = append(symbols, s)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"min":     minDate,
		"max":     maxDate,
		"dates":   dates,
		"symbols": symbols,
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
