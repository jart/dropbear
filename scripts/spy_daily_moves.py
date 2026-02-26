#!/usr/bin/env python3
"""Analyze SPY daily open-to-close price moves for SPX 0DTE option strike selection.

Reads SPY minute bars from ~/.dropbear.sqlite3 (populated by sqlifybars)
and computes the distribution of daily moves from 9:30 open to 4:00 close.

Usage:
    go run ./broker/alpaca/cmd/sqlifybars SPY
    python3 scripts/spy_daily_moves.py
"""

import os
import sqlite3
import math
from datetime import datetime, timezone, timedelta
from zoneinfo import ZoneInfo
from collections import defaultdict

ET = ZoneInfo("America/New_York")

DB_PATH = os.path.expanduser("~/.dropbear.sqlite3")
# Also save results to a throwaway analysis DB for future queries
ANALYSIS_DB = os.path.expanduser("~/.spy_daily_moves.sqlite3")

# Jan 1, 2025 in nanoseconds since epoch
START_NS = int(datetime(2025, 1, 1, tzinfo=timezone.utc).timestamp()) * 1_000_000_000


def load_bars():
    conn = sqlite3.connect(DB_PATH)
    rows = conn.execute("""
        SELECT timestamp, open, high, low, close, volume
        FROM equity_bars
        WHERE symbol = 'SPY' AND timestamp >= ?
        ORDER BY timestamp
    """, (START_NS,)).fetchall()
    conn.close()
    return rows


def group_by_day(bars):
    """Group bars by trading day (Eastern Time date)."""
    days = defaultdict(list)
    for ts_ns, o, h, l, c, v in bars:
        dt = datetime.fromtimestamp(ts_ns / 1e9, tz=ET)
        date_key = dt.date()
        days[date_key].append((dt, o, h, l, c, v))
    return days


def compute_daily_moves(days):
    """For each trading day, compute the move from ~9:30 open to ~4:00 close."""
    moves = []
    for date, bars in sorted(days.items()):
        # bars are sorted by time
        bars.sort(key=lambda x: x[0])

        # Filter to regular trading hours only (9:30 - 16:00 ET)
        rth = [(dt, o, h, l, c, v) for dt, o, h, l, c, v in bars
               if dt.hour * 60 + dt.minute >= 9 * 60 + 30
               and dt.hour * 60 + dt.minute < 16 * 60]
        if len(rth) < 10:
            continue  # skip partial days

        open_price = rth[0][1]   # open of first RTH bar
        close_price = rth[-1][4]  # close of last RTH bar
        day_high = max(b[2] for b in rth)
        day_low = min(b[3] for b in rth)

        if open_price <= 0:
            continue

        pct_move = (close_price - open_price) / open_price * 100
        pct_high = (day_high - open_price) / open_price * 100
        pct_low = (day_low - open_price) / open_price * 100
        dollar_move = close_price - open_price

        moves.append({
            "date": date,
            "open": open_price,
            "close": close_price,
            "high": day_high,
            "low": day_low,
            "pct_move": pct_move,
            "pct_high": pct_high,
            "pct_low": pct_low,
            "dollar_move": dollar_move,
            "range_pct": (day_high - day_low) / open_price * 100,
        })
    return moves


def save_to_analysis_db(moves):
    """Save daily moves to a separate SQLite DB for future analysis."""
    conn = sqlite3.connect(ANALYSIS_DB)
    conn.execute("DROP TABLE IF EXISTS spy_daily_moves")
    conn.execute("""
        CREATE TABLE spy_daily_moves (
            date TEXT PRIMARY KEY,
            open REAL NOT NULL,
            close REAL NOT NULL,
            high REAL NOT NULL,
            low REAL NOT NULL,
            pct_move REAL NOT NULL,
            pct_high REAL NOT NULL,
            pct_low REAL NOT NULL,
            dollar_move REAL NOT NULL,
            range_pct REAL NOT NULL
        )
    """)
    for m in moves:
        conn.execute("""
            INSERT INTO spy_daily_moves VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (str(m["date"]), m["open"], m["close"], m["high"], m["low"],
              m["pct_move"], m["pct_high"], m["pct_low"], m["dollar_move"],
              m["range_pct"]))
    conn.commit()
    conn.close()
    print(f"Saved {len(moves)} daily records to {ANALYSIS_DB}")


def percentile(data, p):
    """Compute the p-th percentile of a sorted list."""
    n = len(data)
    k = (n - 1) * p / 100
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return data[int(k)]
    return data[f] * (c - k) + data[c] * (k - f)


def print_histogram(values, label, bins=20):
    """Print a simple ASCII histogram."""
    if not values:
        return
    lo, hi = min(values), max(values)
    # Symmetric bins around zero
    bound = max(abs(lo), abs(hi))
    step = 2 * bound / bins
    if step == 0:
        return

    counts = [0] * bins
    for v in values:
        idx = int((v + bound) / step)
        idx = min(idx, bins - 1)
        counts[idx] = counts[idx] + 1

    max_count = max(counts)
    bar_width = 50

    print(f"\n{label}")
    print(f"{'Bin':>12s}  {'Count':>5s}  Bar")
    print("-" * 75)
    for i in range(bins):
        lo_edge = -bound + i * step
        hi_edge = lo_edge + step
        bar_len = int(counts[i] / max_count * bar_width) if max_count > 0 else 0
        bar = "#" * bar_len
        center = (lo_edge + hi_edge) / 2
        print(f"  {center:+7.2f}%    {counts[i]:4d}  {bar}")


def print_threshold_table(moves):
    """Print probability of moves exceeding various thresholds."""
    n = len(moves)
    pct_moves = [m["pct_move"] for m in moves]
    pct_highs = [m["pct_high"] for m in moves]
    pct_lows = [m["pct_low"] for m in moves]

    print("\n" + "=" * 80)
    print("PROBABILITY OF INTRADAY MOVES (open-to-close)")
    print("=" * 80)
    thresholds = [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 1.75, 2.00, 2.50, 3.00, 4.00, 5.00]

    print(f"\n{'Threshold':>10s}  {'Close >= +X%':>14s}  {'Close <= -X%':>14s}  {'Either':>14s}")
    print("-" * 60)
    for t in thresholds:
        up = sum(1 for m in pct_moves if m >= t)
        down = sum(1 for m in pct_moves if m <= -t)
        either = up + down
        print(f"  {t:5.2f}%     {up:4d} ({up/n*100:5.1f}%)   {down:4d} ({down/n*100:5.1f}%)   {either:4d} ({either/n*100:5.1f}%)")

    print("\n" + "=" * 80)
    print("PROBABILITY OF INTRADAY EXTREMES (peak/trough vs open)")
    print("These show how far SPY moved DURING the day, not just open-to-close")
    print("=" * 80)

    print(f"\n{'Threshold':>10s}  {'High >= +X%':>14s}  {'Low <= -X%':>14s}  {'Either':>14s}")
    print("-" * 60)
    for t in thresholds:
        up = sum(1 for h in pct_highs if h >= t)
        down = sum(1 for l in pct_lows if l <= -t)
        either_count = sum(1 for h, l in zip(pct_highs, pct_lows) if h >= t or l <= -t)
        print(f"  {t:5.2f}%     {up:4d} ({up/n*100:5.1f}%)   {down:4d} ({down/n*100:5.1f}%)   {either_count:4d} ({either_count/n*100:5.1f}%)")


def print_spx_analysis(moves):
    """Translate SPY moves to SPX points for 0DTE strike selection."""
    n = len(moves)

    # SPX ≈ SPY * 10 (approximately). Use actual SPY prices to estimate SPX.
    # Current SPY ~ 600, so SPX ~ 6000
    print("\n" + "=" * 80)
    print("SPX 0DTE STRIKE SELECTION GUIDE")
    print("(SPX ≈ SPY × 10, so % moves are equivalent)")
    print("=" * 80)

    # Get recent SPY price for SPX estimate
    recent = moves[-1]
    spy_price = recent["close"]
    spx_est = spy_price * 10

    print(f"\nRecent SPY close: ${spy_price:.2f} → estimated SPX: {spx_est:.0f}")
    print(f"Analysis period: {moves[0]['date']} to {moves[-1]['date']} ({n} trading days)")

    pct_moves = sorted([m["pct_move"] for m in moves])
    abs_moves = sorted([abs(m["pct_move"]) for m in moves])

    # Key percentiles
    print(f"\nOpen-to-close move distribution:")
    print(f"  Mean:   {sum(m['pct_move'] for m in moves)/n:+.3f}%")
    print(f"  Median: {percentile(pct_moves, 50):+.3f}%")
    print(f"  StdDev: {(sum((m['pct_move'] - sum(x['pct_move'] for x in moves)/n)**2 for m in moves)/n)**0.5:.3f}%")
    print(f"  Min:    {min(pct_moves):+.3f}% ({min(pct_moves)*spx_est/100:+.0f} SPX pts)")
    print(f"  Max:    {max(pct_moves):+.3f}% ({max(pct_moves)*spx_est/100:+.0f} SPX pts)")

    print(f"\nPercentiles of open-to-close moves:")
    for p in [1, 5, 10, 25, 50, 75, 90, 95, 99]:
        val = percentile(pct_moves, p)
        spx_pts = val * spx_est / 100
        print(f"  P{p:2d}: {val:+.3f}% ({spx_pts:+.0f} SPX pts)")

    # For lottery tickets: what strikes have >0 but low probability of paying off?
    print(f"\n--- LOTTERY TICKET ANALYSIS (buy at 9:31, hold to 4:00 expiry) ---")
    print(f"\nIf you BUY CALLS (need SPX to close ABOVE strike):")
    print(f"{'OTM Distance':>14s}  {'SPX Strike':>11s}  {'P(ITM)':>8s}  {'Avg Payoff if ITM':>18s}")
    print("-" * 60)
    for otm_pct in [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 1.75, 2.00, 2.50, 3.00]:
        strike = spx_est * (1 + otm_pct / 100)
        itm_count = sum(1 for m in pct_moves if m >= otm_pct)
        prob = itm_count / n * 100
        if itm_count > 0:
            avg_payoff_pct = sum(m - otm_pct for m in pct_moves if m >= otm_pct) / itm_count
            avg_payoff_pts = avg_payoff_pct * spx_est / 100
            print(f"  +{otm_pct:.2f}%        {strike:8.0f}    {prob:5.1f}%    {avg_payoff_pts:+.0f} pts (${avg_payoff_pts*100:.0f}/contract)")
        else:
            print(f"  +{otm_pct:.2f}%        {strike:8.0f}    {prob:5.1f}%    n/a")

    print(f"\nIf you BUY PUTS (need SPX to close BELOW strike):")
    print(f"{'OTM Distance':>14s}  {'SPX Strike':>11s}  {'P(ITM)':>8s}  {'Avg Payoff if ITM':>18s}")
    print("-" * 60)
    for otm_pct in [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 1.75, 2.00, 2.50, 3.00]:
        strike = spx_est * (1 - otm_pct / 100)
        itm_count = sum(1 for m in pct_moves if m <= -otm_pct)
        prob = itm_count / n * 100
        if itm_count > 0:
            avg_payoff_pct = sum(abs(m) - otm_pct for m in pct_moves if m <= -otm_pct) / itm_count
            avg_payoff_pts = avg_payoff_pct * spx_est / 100
            print(f"  -{otm_pct:.2f}%        {strike:8.0f}    {prob:5.1f}%    {avg_payoff_pts:+.0f} pts (${avg_payoff_pts*100:.0f}/contract)")
        else:
            print(f"  -{otm_pct:.2f}%        {strike:8.0f}    {prob:5.1f}%    n/a")

    # Expected value analysis
    print(f"\n--- EXPECTED VALUE PER $1 SPENT (rough estimate) ---")
    print(f"Assumes option price ≈ $0.50-5.00 for various OTM distances")
    print(f"(Actual premiums vary with VIX, time to expiry, skew, etc.)")
    print(f"\nFor CALLS:")
    print(f"{'OTM%':>6s}  {'P(ITM)':>8s}  {'E[payoff|ITM]':>14s}  {'EV/contract':>12s}")
    print("-" * 50)
    for otm_pct in [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]:
        itm_moves = [m - otm_pct for m in pct_moves if m >= otm_pct]
        prob = len(itm_moves) / n
        if itm_moves:
            avg_payoff_pts = sum(itm_moves) / len(itm_moves) * spx_est / 100
            ev = prob * avg_payoff_pts * 100  # $100 per SPX point per contract
            print(f"  +{otm_pct:.2f}  {prob*100:6.1f}%    ${avg_payoff_pts*100:8.0f}       ${ev:8.0f}")
        else:
            print(f"  +{otm_pct:.2f}  {prob*100:6.1f}%         n/a            n/a")

    print(f"\nFor PUTS:")
    print(f"{'OTM%':>6s}  {'P(ITM)':>8s}  {'E[payoff|ITM]':>14s}  {'EV/contract':>12s}")
    print("-" * 50)
    for otm_pct in [0.25, 0.50, 0.75, 1.00, 1.25, 1.50, 2.00, 2.50, 3.00]:
        itm_moves = [abs(m) - otm_pct for m in pct_moves if m <= -otm_pct]
        prob = len(itm_moves) / n
        if itm_moves:
            avg_payoff_pts = sum(itm_moves) / len(itm_moves) * spx_est / 100
            ev = prob * avg_payoff_pts * 100
            print(f"  -{otm_pct:.2f}  {prob*100:6.1f}%    ${avg_payoff_pts*100:8.0f}       ${ev:8.0f}")
        else:
            print(f"  -{otm_pct:.2f}  {prob*100:6.1f}%         n/a            n/a")


def print_day_of_week(moves):
    """Break down moves by day of week."""
    print("\n" + "=" * 80)
    print("MOVES BY DAY OF WEEK")
    print("=" * 80)
    dow_names = ["Mon", "Tue", "Wed", "Thu", "Fri"]
    dow_moves = defaultdict(list)
    for m in moves:
        dow = m["date"].weekday()
        dow_moves[dow].append(m["pct_move"])

    print(f"\n{'Day':>5s}  {'N':>4s}  {'Mean':>8s}  {'StdDev':>8s}  {'Min':>8s}  {'Max':>8s}  {'%Up':>6s}")
    print("-" * 55)
    for d in range(5):
        vals = dow_moves[d]
        if not vals:
            continue
        mean = sum(vals) / len(vals)
        std = (sum((v - mean) ** 2 for v in vals) / len(vals)) ** 0.5
        up = sum(1 for v in vals if v > 0) / len(vals) * 100
        print(f"  {dow_names[d]}  {len(vals):4d}  {mean:+7.3f}%  {std:7.3f}%  {min(vals):+7.2f}%  {max(vals):+7.2f}%  {up:5.1f}%")


def main():
    print("Loading SPY minute bars from", DB_PATH)
    bars = load_bars()
    print(f"Loaded {len(bars):,} minute bars since 2025-01-01")

    days = group_by_day(bars)
    print(f"Found {len(days)} unique trading days")

    moves = compute_daily_moves(days)
    print(f"Computed moves for {len(moves)} complete trading days")

    if not moves:
        print("No data!")
        return

    save_to_analysis_db(moves)

    pct_moves = [m["pct_move"] for m in moves]
    print_histogram(pct_moves, "DISTRIBUTION OF DAILY OPEN-TO-CLOSE MOVES (%)")
    print_threshold_table(moves)
    print_spx_analysis(moves)
    print_day_of_week(moves)

    # Summary of biggest move days
    print("\n" + "=" * 80)
    print("TOP 10 BIGGEST MOVE DAYS")
    print("=" * 80)
    by_abs = sorted(moves, key=lambda m: abs(m["pct_move"]), reverse=True)[:10]
    print(f"\n{'Date':>12s}  {'Move':>8s}  {'Open':>9s}  {'Close':>9s}  {'High':>9s}  {'Low':>9s}  {'Range':>7s}")
    print("-" * 70)
    for m in by_abs:
        print(f"  {m['date']}  {m['pct_move']:+7.2f}%  ${m['open']:7.2f}  ${m['close']:7.2f}  ${m['high']:7.2f}  ${m['low']:7.2f}  {m['range_pct']:5.2f}%")


if __name__ == "__main__":
    main()
