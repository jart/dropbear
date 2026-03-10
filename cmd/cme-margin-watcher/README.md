# cme-margin-watcher

Apple Watch buzzer for CME margin changes. Runs three monitors concurrently:

| Monitor | Source | Latency |
|---------|--------|---------|
| **Advisory RSS** | `feeds.feedburner.com/ClearingAdvisories` | ~2 min |
| **SPAN file** | `cmegroup.com/ftp/span/data/nymex/` | ~15 min |
| **Margins API** | CME internal JSON endpoint | ~5 min |

The RSS feed is the fastest signal — it's what fired for Advisory 26-095.
SPAN catches the underlying parameter change which sometimes precedes the formal notice.
Margins API gives you the human-readable dollar figure.

## Setup

### 1. Pushover

- Install [Pushover](https://pushover.net) on your iPhone (~$5 one-time)
- Enable Watch notifications in Pushover settings
- Get your **User Key** from the Pushover dashboard
- Create an **Application** → get the API Token

### 2. Build

```bash
go build -o cme-margin-watcher .
```

### 3. Run

```bash
export PUSHOVER_TOKEN=your_app_token
export PUSHOVER_USER=your_user_key
./cme-margin-watcher
```

You'll get a startup ping on your Watch to confirm it's running.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PUSHOVER_TOKEN` | *required* | Pushover application token |
| `PUSHOVER_USER` | *required* | Pushover user key |
| `STATE_FILE` | `state.json` | Where to persist seen advisory GUIDs and last-known margins |
| `ADVISORY_INTERVAL` | `2m` | How often to poll the Clearing Advisory RSS |
| `SPAN_INTERVAL` | `15m` | How often to check for a new SPAN file |
| `MARGINS_INTERVAL` | `5m` | How often to poll the CME margins API |

### Run as a systemd service

```bash
cp cme-margin-watcher /usr/local/bin/
cp cme-margin-watcher.service /etc/systemd/system/
# Edit the service file to set your env vars
systemctl daemon-reload
systemctl enable --now cme-margin-watcher
journalctl -fu cme-margin-watcher
```

## Watched products

Default: `CL BRN NG GC SI` (WTI crude, Brent, nat gas, gold, silver).
Edit `cfg.WatchProducts` in `config.go` to change.

## SPAN file parsing notes

CME SPAN files use a fixed-width positional format. The price scan range in
"B"/"P" records is a 7-digit integer. For CL (crude oil):

- 1 contract = 1,000 barrels
- Scan range is in hundredths of a dollar per barrel
- `scanRange / 100` ≈ dollar margin per contract

**First run**: compare the logged SPAN scan range against the CME margins page at
`https://www.cmegroup.com/markets/energy/crude-oil/light-sweet-crude.margins.html`
and adjust `spanScanRangeDivisor` in `span.go` if the numbers look off.

The `cmeMarginAPIURL` in `margins.go` is CME's undocumented internal endpoint.
If it stops returning JSON, the SPAN monitor still works independently.

## Alert priority

All alerts fire at **Pushover priority 1** (High) — bypasses Do Not Disturb,
forces a sound + Watch haptic. If you want Emergency (requires tap-to-ack,
retries every 30s): change `PriorityHigh` → `PriorityEmergency` in the
relevant `push.Sendf` calls.
