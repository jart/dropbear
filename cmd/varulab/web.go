package main

import (
	"database/sql"
	"dropbear/db"
	"dropbear/decimal"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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
			b.values = append(b.values, winning)
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

// experimentDB opens (or returns cached) DB for an experiment name.
// Returns the active experiment's DB if it matches.
var (
	expDBCache   = map[string]*sql.DB{}
	expDBCacheMu sync.Mutex
)

func experimentDB(name string) (*sql.DB, error) {
	if gExperiment != nil && name == gExperiment.Name {
		return gExperiment.DB, nil
	}
	expDBCacheMu.Lock()
	defer expDBCacheMu.Unlock()
	if d, ok := expDBCache[name]; ok {
		return d, nil
	}
	path := varulabDir() + "/" + name + ".sqlite3"
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	d, err := db.OpenReadOnly(path)
	if err != nil {
		return nil, err
	}
	expDBCache[name] = d
	return d, nil
}

// HTTP handlers

func startWeb() {
	go sseBroadcaster()

	staticFS, _ := fs.Sub(staticFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/rename", handleRename)
	http.HandleFunc("/delete", handleDelete)
	http.HandleFunc("/e/", handleExperiment)

	sock, err := net.Listen("tcp", *listenFlag)
	if err != nil {
		log.Fatalf("web: listen: %v", err)
	}
	log.Printf("dashboard at http://%s", sock.Addr())
	go http.Serve(sock, nil)
}

func noCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

const pageStyle = `body{font-family:monospace;background:#1a1a2e;color:#ccc;padding:20px}` +
	`a{color:#6cf}table{border-collapse:collapse}td,th{padding:4px 12px;text-align:left}` +
	`tr:hover{background:#2a2a4e}input{background:#2a2a3e;color:#ccc;border:1px solid #444;padding:4px 8px}` +
	`button{background:#2a2a4e;color:#ccc;border:1px solid #555;padding:4px 12px;cursor:pointer}` +
	`button:hover{background:#3a3a5e}.danger{color:#f66}`

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	noCache(w)
	experiments := listExperiments()
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>varulab</title><style>%s</style></head><body>`, pageStyle)
	fmt.Fprintf(w, `<h1>varulab</h1>`)
	if gExperiment != nil {
		fmt.Fprintf(w, `<p>active experiment: <a href="/e/%s/">%s</a></p>`, gExperiment.Name, gExperiment.Name)
	}
	fmt.Fprintf(w, `<table><tr><th>experiment</th><th>modified</th><th></th><th></th></tr>`)
	for _, e := range experiments {
		active := ""
		if gExperiment != nil && e.Name == gExperiment.Name {
			active = " (active)"
		}
		fmt.Fprintf(w, `<tr>`)
		fmt.Fprintf(w, `<td><a href="/e/%s/">%s%s</a></td>`, e.Name, e.Name, active)
		fmt.Fprintf(w, `<td>%s</td>`, e.ModTime.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, `<td><form method="POST" action="/rename" style="display:inline">`)
		fmt.Fprintf(w, `<input type="hidden" name="old" value="%s">`, e.Name)
		fmt.Fprintf(w, `<input type="text" name="new" size="20" placeholder="new name">`)
		fmt.Fprintf(w, `<button type="submit">rename</button></form></td>`)
		fmt.Fprintf(w, `<td><form method="POST" action="/delete" style="display:inline"`)
		fmt.Fprintf(w, ` onsubmit="return confirm('delete %s?')">`, e.Name)
		fmt.Fprintf(w, `<input type="hidden" name="name" value="%s">`, e.Name)
		fmt.Fprintf(w, `<button type="submit" class="danger">delete</button></form></td>`)
		fmt.Fprintf(w, `</tr>`)
	}
	fmt.Fprintf(w, `</table></body></html>`)
}

func handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.ParseForm()
	oldName := r.FormValue("old")
	newName := r.FormValue("new")
	if oldName == "" || newName == "" {
		http.Error(w, "missing name", 400)
		return
	}
	if gExperiment != nil && oldName == gExperiment.Name {
		// update active experiment reference
		if err := renameExperiment(oldName, newName); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		gExperiment.Name = newName
		gExperiment.Path = varulabDir() + "/" + newName + ".sqlite3"
	} else {
		if err := renameExperiment(oldName, newName); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.ParseForm()
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "missing name", 400)
		return
	}
	if gExperiment != nil && name == gExperiment.Name {
		http.Error(w, "cannot delete the active experiment", 400)
		return
	}
	// close cached DB if open
	expDBCacheMu.Lock()
	if d, ok := expDBCache[name]; ok {
		d.Close()
		delete(expDBCache, name)
	}
	expDBCacheMu.Unlock()
	if err := deleteExperiment(name); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleExperiment routes all /e/{name}/... requests.
func handleExperiment(w http.ResponseWriter, r *http.Request) {
	// parse /e/{name}/rest...
	path := strings.TrimPrefix(r.URL.Path, "/e/")
	slash := strings.IndexByte(path, '/')
	if slash < 0 {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}
	name := path[:slash]
	rest := path[slash:] // includes leading /

	database, err := experimentDB(name)
	if err != nil {
		http.Error(w, "experiment not found: "+name, 404)
		return
	}

	base := "/e/" + name

	switch {
	case rest == "/":
		handleExperimentIndex(w, r, name, base)
	case rest == "/api/events":
		handleEvents(w, r, database)
	case rest == "/api/state":
		handleState(w, r, database)
	case rest == "/api/grid":
		handleGrid(w, r, database)
	case rest == "/api/summary":
		handleSummaryAPI(w, r, database)
	case rest == "/api/dates":
		handleDatesAPI(w, r, database)
	case rest == "/api/runs":
		handleRuns(w, r, database)
	case rest == "/api/flagsets":
		handleFlagSets(w, r)
	case rest == "/api/regenerate":
		handleRegenerate(w, r, name)
	case rest == "/failed":
		handleFailedPage(w, r, database, base)
	case strings.HasPrefix(rest, "/log/"):
		handleLogPage(w, r, database, base, strings.TrimPrefix(rest, "/log/"))
	default:
		http.NotFound(w, r)
	}
}

func handleExperimentIndex(w http.ResponseWriter, r *http.Request, name, base string) {
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	noCache(w)
	w.Header().Set("Content-Type", "text/html")
	// inject BASE path for API calls
	inject := fmt.Sprintf(`<script>window.BASE="%s";</script>`, base)
	htmlStr := strings.Replace(string(data), "</head>", inject+"</head>", 1)
	w.Write([]byte(htmlStr))
}

func handleFailedPage(w http.ResponseWriter, r *http.Request, database *sql.DB, base string) {
	rows, err := database.Query(`SELECT id, symbol, date, flags, finished_at FROM varulab_runs WHERE status = 'failed' ORDER BY finished_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>failed runs</title><style>%s</style></head><body>`, pageStyle)
	fmt.Fprintf(w, `<p><a href="/">&larr; varulab</a> / <a href="%s/">experiment</a></p>`, base)
	fmt.Fprintf(w, "<h1>failed runs</h1>\n<table><tr><th>id</th><th>symbol</th><th>date</th><th>flags</th><th>finished</th></tr>\n")
	for rows.Next() {
		var id int64
		var sym, date, flags string
		var finishedAt sql.NullInt64
		rows.Scan(&id, &sym, &date, &flags, &finishedAt)
		finished := ""
		if finishedAt.Valid {
			finished = time.Unix(0, finishedAt.Int64).Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "<tr><td><a href=\"%s/log/%d\">%d</a></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			base, id, id, sym, date, flags, finished)
	}
	fmt.Fprintf(w, "</table></body></html>\n")
}

func handleLogPage(w http.ResponseWriter, r *http.Request, database *sql.DB, base, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "bad id", 400)
		return
	}
	var sym, date, flags, status, logText string
	err = database.QueryRow(`SELECT symbol, date, flags, status, COALESCE(log,'') FROM varulab_runs WHERE id = ?`, id).
		Scan(&sym, &date, &flags, &status, &logText)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>run %d</title><style>%s</style></head><body>`, id, pageStyle)
	fmt.Fprintf(w, `<p><a href="/">&larr; varulab</a> / <a href="%s/">experiment</a> / <a href="%s/failed">failed</a></p>`, base, base)
	fmt.Fprintf(w, "<h1>run %d: %s %s [%s] %s</h1>\n", id, sym, date, status, flags)
	fmt.Fprintf(w, "<pre>%s</pre>\n", html.EscapeString(logText))
	fmt.Fprintf(w, "</body></html>\n")
}

func handleEvents(w http.ResponseWriter, r *http.Request, database *sql.DB) {
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
	broadcastState(database)
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

func handleState(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildDashboardState(database))
}

func handleGrid(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	from, to, sym := queryFilter(r)
	json.NewEncoder(w).Encode(queryGrid(database, from, to, sym))
}

func handleSummaryAPI(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	from, to, sym := queryFilter(r)
	flagS, symS := querySummary(database, from, to, sym)
	json.NewEncoder(w).Encode(map[string]any{
		"flags":   flagS,
		"symbols": symS,
	})
}

func handleDatesAPI(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	var minDate, maxDate string
	database.QueryRow(`SELECT MIN(date), MAX(date) FROM varulab_runs WHERE status = 'done'`).Scan(&minDate, &maxDate)
	var dates []string
	rows, _ := database.Query(`SELECT DISTINCT date FROM varulab_runs WHERE status = 'done' ORDER BY date`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			rows.Scan(&d)
			dates = append(dates, d)
		}
	}
	var symbols []string
	srows, _ := database.Query(`SELECT DISTINCT symbol FROM varulab_runs WHERE status = 'done' ORDER BY symbol`)
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

func handleRuns(w http.ResponseWriter, r *http.Request, database *sql.DB) {
	noCache(w)
	w.Header().Set("Content-Type", "application/json")
	idStr := r.URL.Query().Get("id")
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		var logText sql.NullString
		var run RunSummary
		var winning sql.NullInt64
		database.QueryRow(`SELECT id, symbol, date, flags, status, winning, log FROM varulab_runs WHERE id = ?`, id).
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
	rows, _ := database.Query(`SELECT id, symbol, date, flags, status, winning FROM varulab_runs ORDER BY id DESC LIMIT 100`)
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

func handleRegenerate(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if gExperiment == nil || name != gExperiment.Name {
		http.Error(w, "can only regenerate the active experiment", 403)
		return
	}
	n := gScheduler.GenerateRuns(gGitRev)
	notifyScheduler()
	broadcastState(gExperiment.DB)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"created": n})
}
