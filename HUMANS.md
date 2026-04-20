# dropbear for humans

See [CLAUDE.md](CLAUDE.md) for further information.

---

## Getting Started

### Getting Data
  - Equities minute bars download: <https://drive.google.com/drive/folders/1xs1--iiHBQFqYckefw5xlxvq8E0DUo5I?usp=sharing>
    - Extract to `~/equitydata` folder
    - Useful if getting started for first time without access to Databento key
  - Sample DataBento data download: <https://drive.google.com/drive/folders/1sDr7hmdthRbn1iclui6I7PuooVn8IuQm>
  - Save to `~/databento`, directory structure example: `~/databento/SPXW/2026-04-01.dbn`
  - Save your DataBento API key to `~/.databento.key`
  - Merge OPRA files with the underlying asset's quote data: `go run ./broker/databento/cmd/dbninject -file ~/databento/AVGO/2026-04-15.dbn -dataset EQUS.MINI -sym AVGO -schema mbp-1`
  - View data: `go run ./broker/databento/cmd/dbndump -strike 430 -call -expiry 2026-01-02 ~/databento/TSLA/2026-01-02.dbn |& less` 
  - `varulab` supports automatic data downloading and merging from Databento

### Running Backtests

#### `varu`: Single-run Backtesting
  - `go run ./cmd/varu -date 2026-04-01 -dbn ~/databento/SPXW/2026-04-01.dbn -symbol SPXW -web -unsecure`
  - Default strategies ran: `[kStrategySellCallVertical, kStrategySellPutVertical]`
  - Webserver spawns at `http://localhost:8484`

#### `varulab`: Backtesting w/ Parameter Optimizations
  - Modify `./cmd/varulab/config.go` to configure backtest parameters & start date
  - By default, webserver spawns at `0.0.0.0:8585`
  - Start a new experiment: `go run ./cmd/varulab -new experiment-name`
  - Resume a previous experiment: `go run ./cmd/varulab -open experiment-name -resume`
