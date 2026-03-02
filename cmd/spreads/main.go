// OPRA spread viewer: imports DBN CMBP1 data into SQLite, then serves a web
// GUI for viewing bid/ask spread step charts per instrument.
//
// Import mode (when -data is provided):
//
//	go run ./cmd/spreads \
//	  -data /path/to/cmbp1.dbn \
//	  -defs /path/to/definition.dbn \
//	  -start 2026-02-27T06:30:00 -end 2026-02-27T06:35:00
//
// View mode (no -data):
//
//	go run ./cmd/spreads -db spreads.db
package main

import (
	"bufio"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/db"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed static
var staticFiles embed.FS

var (
	flagData  = flag.String("data", "", "Path to CMBP1 .dbn file (import mode)")
	flagDefs  = flag.String("defs", "", "Path to definition .dbn file")
	flagAddr  = flag.String("addr", ":8080", "HTTP listen address")
	flagStart clocky.Time
	flagEnd   clocky.Time
)

func init() {
	clocky.TimeVar(&flagStart, "start", 0, "Start time filter (e.g. 2026-02-27T06:30:00)")
	clocky.TimeVar(&flagEnd, "end", 0, "End time filter (e.g. 2026-02-27T06:35:00)")
}

const schema = `
CREATE TABLE IF NOT EXISTS instruments (
    instrument_id INTEGER PRIMARY KEY,
    symbol TEXT NOT NULL,
    class TEXT NOT NULL,
    strike INTEGER NOT NULL,
    expiration INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS nbbo (
    ts_event INTEGER NOT NULL,
    instrument_id INTEGER NOT NULL,
    bid_px INTEGER NOT NULL,
    ask_px INTEGER NOT NULL,
    bid_sz INTEGER NOT NULL,
    ask_sz INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_nbbo_instrument ON nbbo(instrument_id, ts_event);

CREATE TABLE IF NOT EXISTS trades (
    ts_event INTEGER NOT NULL,
    instrument_id INTEGER NOT NULL,
    price INTEGER NOT NULL,
    size INTEGER NOT NULL,
    side TEXT NOT NULL,
    bid_px INTEGER NOT NULL,
    ask_px INTEGER NOT NULL,
    bid_sz INTEGER NOT NULL,
    ask_sz INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trades_instrument ON trades(instrument_id, ts_event);
`

func main() {
	flag.Parse()

	conn := db.Get()
	defer conn.Close()

	if _, err := conn.Exec(schema); err != nil {
		log.Fatalf("create schema: %v", err)
	}

	if *flagData != "" {
		if *flagDefs == "" {
			log.Fatal("usage: -defs is required when -data is provided")
		}
		importData(conn)
	}

	serve(conn)
}

type instrumentInfo struct {
	Symbol     string
	Class      string
	Strike     int64
	Expiration clocky.Time
}

func importData(conn *sql.DB) {
	// Load instrument definitions
	instruments := loadDefinitions(*flagDefs)
	log.Printf("loaded %d instrument definitions", len(instruments))

	// Insert instruments into SQLite
	tx, err := conn.Begin()
	if err != nil {
		log.Fatalf("begin transaction: %v", err)
	}
	instStmt, err := tx.Prepare("INSERT OR REPLACE INTO instruments (instrument_id, symbol, class, strike, expiration) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("prepare instrument insert: %v", err)
	}
	for id, inst := range instruments {
		if _, err := instStmt.Exec(id, inst.Symbol, inst.Class, inst.Strike, int64(inst.Expiration)); err != nil {
			log.Fatalf("insert instrument %d: %v", id, err)
		}
	}
	instStmt.Close()
	if err := tx.Commit(); err != nil {
		log.Fatalf("commit instruments: %v", err)
	}
	log.Printf("inserted %d instruments into database", len(instruments))

	// Stream CMBP1 records
	f, err := os.Open(*flagData)
	if err != nil {
		log.Fatalf("open data file: %v", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 256*1024)
	meta, err := databento.DecodeMetadata(r)
	if err != nil {
		log.Fatalf("decode metadata: %v", err)
	}

	tx, err = conn.Begin()
	if err != nil {
		log.Fatalf("begin transaction: %v", err)
	}
	nbboStmt, err := tx.Prepare("INSERT INTO nbbo (ts_event, instrument_id, bid_px, ask_px, bid_sz, ask_sz) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("prepare nbbo insert: %v", err)
	}
	tradeStmt, err := tx.Prepare("INSERT INTO trades (ts_event, instrument_id, price, size, side, bid_px, ask_px, bid_sz, ask_sz) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		log.Fatalf("prepare trade insert: %v", err)
	}

	var scanned, inserted, trades int64
	const batchSize = 50000

	for {
		rec, err := databento.DecodeRecord(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("decode record: %v", err)
		}

		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			log.Fatalf("upgrade record: %v", err)
		}

		scanned++
		if scanned%1000000 == 0 {
			log.Printf("scanned %dM records, inserted %d", scanned/1000000, inserted)
		}

		typed := databento.CastRecord(rec)
		cmbp1, ok := typed.(*databento.CMBP1)
		if !ok {
			continue
		}

		ts := cmbp1.Header.TSEvent
		if flagStart != 0 && ts < flagStart {
			continue
		}
		if flagEnd != 0 && ts >= flagEnd {
			break
		}

		bid := cmbp1.Levels[0].BidPx
		ask := cmbp1.Levels[0].AskPx
		if bid == databento.UndefPrice || ask == databento.UndefPrice {
			continue
		}

		if _, err := nbboStmt.Exec(
			int64(ts),
			cmbp1.Header.InstrumentID,
			bid, ask,
			cmbp1.Levels[0].BidSz,
			cmbp1.Levels[0].AskSz,
		); err != nil {
			log.Fatalf("insert nbbo: %v", err)
		}
		inserted++

		if cmbp1.Action == databento.ActionTrade || cmbp1.Action == databento.ActionFill {
			side := string(rune(cmbp1.Side))
			if _, err := tradeStmt.Exec(
				int64(ts),
				cmbp1.Header.InstrumentID,
				cmbp1.Price,
				cmbp1.Size,
				side,
				bid, ask,
				cmbp1.Levels[0].BidSz,
				cmbp1.Levels[0].AskSz,
			); err != nil {
				log.Fatalf("insert trade: %v", err)
			}
			trades++
		}

		if inserted%batchSize == 0 {
			nbboStmt.Close()
			tradeStmt.Close()
			if err := tx.Commit(); err != nil {
				log.Fatalf("commit batch: %v", err)
			}
			tx, err = conn.Begin()
			if err != nil {
				log.Fatalf("begin transaction: %v", err)
			}
			nbboStmt, err = tx.Prepare("INSERT INTO nbbo (ts_event, instrument_id, bid_px, ask_px, bid_sz, ask_sz) VALUES (?, ?, ?, ?, ?, ?)")
			if err != nil {
				log.Fatalf("prepare nbbo insert: %v", err)
			}
			tradeStmt, err = tx.Prepare("INSERT INTO trades (ts_event, instrument_id, price, size, side, bid_px, ask_px, bid_sz, ask_sz) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")
			if err != nil {
				log.Fatalf("prepare trade insert: %v", err)
			}
		}
	}

	nbboStmt.Close()
	tradeStmt.Close()
	if err := tx.Commit(); err != nil {
		log.Fatalf("commit final batch: %v", err)
	}

	log.Printf("import complete: scanned %d records, inserted %d NBBO ticks, %d trades", scanned, inserted, trades)
}

func loadDefinitions(path string) map[uint32]instrumentInfo {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open definitions file: %v", err)
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 256*1024)
	meta, err := databento.DecodeMetadata(r)
	if err != nil {
		log.Fatalf("decode definitions metadata: %v", err)
	}

	instruments := make(map[uint32]instrumentInfo)
	for {
		rec, err := databento.DecodeRecord(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("decode definition record: %v", err)
		}

		rec, err = databento.UpgradeRecord(meta.Version, rec)
		if err != nil {
			log.Fatalf("upgrade definition record: %v", err)
		}

		typed := databento.CastRecord(rec)
		inst, ok := typed.(*databento.Instrument)
		if !ok {
			continue
		}

		class := ""
		switch inst.InstrumentClass {
		case databento.InstrumentClassCall:
			class = "C"
		case databento.InstrumentClassPut:
			class = "P"
		default:
			continue
		}

		instruments[inst.Header.InstrumentID] = instrumentInfo{
			Symbol:     inst.GetRawSymbol(),
			Class:      class,
			Strike:     inst.StrikePrice,
			Expiration: inst.Expiration,
		}
	}

	return instruments
}

// serve starts the HTTP server.
func serve(conn *sql.DB) {
	sub, _ := fs.Sub(staticFiles, "static")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.FileServer(http.FS(sub)).ServeHTTP(w, r)
			return
		}
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	http.HandleFunc("/api/instruments", func(w http.ResponseWriter, r *http.Request) {
		handleInstruments(w, r, conn)
	})

	http.HandleFunc("/api/nbbo", func(w http.ResponseWriter, r *http.Request) {
		handleNBBO(w, r, conn)
	})

	http.HandleFunc("/api/trades", func(w http.ResponseWriter, r *http.Request) {
		handleTrades(w, r, conn)
	})

	log.Printf("serving on %s", *flagAddr)
	if err := http.ListenAndServe(*flagAddr, nil); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

type instrumentJSON struct {
	ID         uint32 `json:"id"`
	Symbol     string `json:"symbol"`
	Class      string `json:"class"`
	Strike     string `json:"strike"`
	Expiration string `json:"expiration"`
	Count      int    `json:"count"`
}

func handleInstruments(w http.ResponseWriter, r *http.Request, conn *sql.DB) {
	rows, err := conn.Query(`
		SELECT i.instrument_id, i.symbol, i.class, i.strike, i.expiration, COUNT(n.rowid)
		FROM instruments i
		JOIN nbbo n ON n.instrument_id = i.instrument_id
		GROUP BY i.instrument_id
		ORDER BY COUNT(n.rowid) DESC, i.strike, i.class
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []instrumentJSON
	for rows.Next() {
		var id uint32
		var symbol, class string
		var strike, expiration int64
		var count int
		if err := rows.Scan(&id, &symbol, &class, &strike, &expiration, &count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Format strike: raw int64 at 1e-9 scale → dollar string
		strikeDollars := fmt.Sprintf("%.2f", float64(strike)/float64(databento.FixedPriceScale))
		// Format expiration: unix nanos → date string
		expTime := clocky.Time(expiration)
		expStr := ""
		if expTime != 0 {
			y, m, d := expTime.Date()
			expStr = fmt.Sprintf("%04d-%02d-%02d", y, m, d)
		}
		result = append(result, instrumentJSON{
			ID:         id,
			Symbol:     symbol,
			Class:      class,
			Strike:     strikeDollars,
			Expiration: expStr,
			Count:      count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type nbboJSON struct {
	TS    int64  `json:"ts"`
	BidPx string `json:"bid_px"`
	AskPx string `json:"ask_px"`
	BidSz uint32 `json:"bid_sz"`
	AskSz uint32 `json:"ask_sz"`
}

func handleNBBO(w http.ResponseWriter, r *http.Request, conn *sql.DB) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id parameter", http.StatusBadRequest)
		return
	}

	rows, err := conn.Query(
		"SELECT ts_event, bid_px, ask_px, bid_sz, ask_sz FROM nbbo WHERE instrument_id = ? ORDER BY ts_event",
		id,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []nbboJSON
	for rows.Next() {
		var ts, bidPx, askPx int64
		var bidSz, askSz uint32
		if err := rows.Scan(&ts, &bidPx, &askPx, &bidSz, &askSz); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result = append(result, nbboJSON{
			TS:    ts,
			BidPx: fmt.Sprintf("%.4f", float64(bidPx)/float64(databento.FixedPriceScale)),
			AskPx: fmt.Sprintf("%.4f", float64(askPx)/float64(databento.FixedPriceScale)),
			BidSz: bidSz,
			AskSz: askSz,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type tradeJSON struct {
	TS    int64  `json:"ts"`
	Price string `json:"price"`
	Size  uint32 `json:"size"`
	Side  string `json:"side"`
	BidPx string `json:"bid_px"`
	AskPx string `json:"ask_px"`
	BidSz uint32 `json:"bid_sz"`
	AskSz uint32 `json:"ask_sz"`
}

func handleTrades(w http.ResponseWriter, r *http.Request, conn *sql.DB) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id parameter", http.StatusBadRequest)
		return
	}

	rows, err := conn.Query(
		"SELECT ts_event, price, size, side, bid_px, ask_px, bid_sz, ask_sz FROM trades WHERE instrument_id = ? ORDER BY ts_event",
		id,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	scale := float64(databento.FixedPriceScale)
	var result []tradeJSON
	for rows.Next() {
		var ts, price, bidPx, askPx int64
		var size, bidSz, askSz uint32
		var side string
		if err := rows.Scan(&ts, &price, &size, &side, &bidPx, &askPx, &bidSz, &askSz); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result = append(result, tradeJSON{
			TS:    ts,
			Price: fmt.Sprintf("%.4f", float64(price)/scale),
			Size:  size,
			Side:  side,
			BidPx: fmt.Sprintf("%.4f", float64(bidPx)/scale),
			AskPx: fmt.Sprintf("%.4f", float64(askPx)/scale),
			BidSz: bidSz,
			AskSz: askSz,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
