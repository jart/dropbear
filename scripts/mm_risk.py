#!/usr/bin/env python3
"""Analyze market maker risk from boxer fill logs.

Parses fill lines from the boxer log, attributes each trade to a market
maker (WOLF, DASH, CES, SIG, JANESTREET), and models their risk profile
at SPX settlement.

Usage:
    python3 scripts/mm_risk.py ~/scratch/always-crossing-the-spread.txt [--settle 6820]
"""

import re
import sys
from collections import defaultdict

MULTIPLIER = 100

ROUTE_TO_MM = {
    "WOLF": "WOLF",
    "DASH": "DASH",
    "CES": "CES",
    "SIG": "SIG",
    "JANESTREET": "JANESTREET",
}

MM_NAMES = {
    "CES": "Citadel",
    "DASH": "CBOE/Dash",
    "JANESTREET": "Jane Street",
    "SIG": "Susquehanna",
    "WOLF": "Wolverine",
}

# fill #1 SPXW  260310C06820000 via CES_OPT_F1_J1 @ 13.50 qty now 1
FILL_RE = re.compile(
    r"fill #\d+ "
    r"SPXW\s+(\d{6})([CP])(\d{8})"  # expiry, put/call, strike*1000
    r" via (\S+)"                     # route name
    r" @ (\S+)"                       # fill price
)


def parse_route(route_name):
    """Extract market maker from route name like CES_OPT_F2_J1."""
    prefix = route_name.split("_")[0]
    return ROUTE_TO_MM.get(prefix, prefix)


def parse_osi_strike(strike_str):
    """Parse 8-digit strike like 06820000 to float 6820."""
    return int(strike_str) / 1000


def settlement_value(pc, strike, settle):
    """Intrinsic value of option at settlement."""
    if pc == "C":
        return max(settle - strike, 0)
    else:
        return max(strike - settle, 0)


def compute_pnl(net, settle):
    """Compute P&L for a set of net positions at a given settlement price."""
    pnl = 0.0
    for (pc, strike), pos in net.items():
        sv = settlement_value(pc, strike, settle)
        pnl += (pos["qty"] * sv - pos["cost"]) * MULTIPLIER
    return pnl


def build_net(positions):
    """Build net position dict from a list of (pc, strike, mm_qty, price)."""
    net = defaultdict(lambda: {"qty": 0, "cost": 0.0})
    for pc, strike, mm_qty, price in positions:
        key = (pc, strike)
        net[key]["qty"] += mm_qty
        net[key]["cost"] += mm_qty * price
    return net


def find_best_worst(net, test_prices):
    """Find settlement prices that maximize and minimize P&L."""
    best_pnl = None
    best_settle = None
    worst_pnl = None
    worst_settle = None
    for s in test_prices:
        pnl = compute_pnl(net, s)
        if best_pnl is None or pnl > best_pnl:
            best_pnl = pnl
            best_settle = s
        if worst_pnl is None or pnl < worst_pnl:
            worst_pnl = pnl
            worst_settle = s
    return best_pnl, best_settle, worst_pnl, worst_settle


def main():
    if len(sys.argv) < 2:
        print(f"usage: {sys.argv[0]} <logfile> [--settle <price>]")
        sys.exit(1)

    logfile = sys.argv[1]
    settle = None
    for i, arg in enumerate(sys.argv):
        if arg == "--settle" and i + 1 < len(sys.argv):
            settle = float(sys.argv[i + 1])

    # Track running qty per OSI to infer buy/sell direction from qty changes
    qty = defaultdict(int)
    fills = []  # (mm, pc, strike, fill_price, we_bought)

    with open(logfile) as f:
        for line in f:
            m = FILL_RE.search(line)
            if not m:
                continue
            expiry, pc, strike_str, route, price_str = m.groups()
            strike = parse_osi_strike(strike_str)
            price = float(price_str)
            mm = parse_route(route)
            osi = f"{expiry}{pc}{strike_str}"

            qty_match = re.search(r"qty now (-?\d+)", line)
            if not qty_match:
                continue
            new_qty = int(qty_match.group(1))
            old_qty = qty[osi]
            we_bought = new_qty > old_qty
            qty[osi] = new_qty

            fills.append((mm, pc, strike, price, we_bought))

    if not fills:
        print("no fills found")
        sys.exit(1)

    # Build market maker positions
    # When we buy, the MM sells (short). When we sell, the MM buys (long).
    mm_positions = defaultdict(list)
    for mm, pc, strike, price, we_bought in fills:
        mm_qty = -1 if we_bought else +1
        mm_positions[mm].append((pc, strike, mm_qty, price))

    # Test prices for best/worst case within +-100 of current range
    all_strikes = sorted(set(strike for _, _, strike, _, _ in fills))
    mid = (min(all_strikes) + max(all_strikes)) / 2
    if settle is not None:
        mid = settle
    lo = mid - 100
    hi = mid + 100
    test_prices = sorted(set(
        [lo, hi] +
        [s for s in all_strikes if lo <= s <= hi] +
        [s + 0.01 for s in all_strikes if lo <= s + 0.01 <= hi] +
        [s - 0.01 for s in all_strikes if lo <= s - 0.01 <= hi]
    ))

    # Compute per-MM stats
    mm_stats = {}
    for mm in sorted(mm_positions.keys()):
        positions = mm_positions[mm]
        net = build_net(positions)
        best_pnl, best_settle, worst_pnl, worst_settle = find_best_worst(net, test_prices)
        settle_pnl = compute_pnl(net, settle) if settle is not None else None
        mm_stats[mm] = {
            "fills": len(positions),
            "net": net,
            "best_pnl": best_pnl,
            "best_settle": best_settle,
            "worst_pnl": worst_pnl,
            "worst_settle": worst_settle,
            "settle_pnl": settle_pnl,
        }

    # Compute our stats (opposite of all MMs combined)
    our_positions = []
    for mm in mm_positions:
        for pc, strike, mm_qty, price in mm_positions[mm]:
            our_positions.append((pc, strike, -mm_qty, price))
    our_net = build_net(our_positions)
    our_best, our_best_s, our_worst, our_worst_s = find_best_worst(our_net, test_prices)
    our_settle_pnl = compute_pnl(our_net, settle) if settle is not None else None

    # Print detailed positions per MM
    print(f"parsed {len(fills)} fills across {len(mm_positions)} market makers")
    print()

    for mm in sorted(mm_stats.keys()):
        st = mm_stats[mm]
        name = MM_NAMES.get(mm, mm)
        print(f"=== {mm} / {name} ({st['fills']} fills) ===")
        for (pc, strike), pos in sorted(st["net"].items()):
            if pos["qty"] != 0:
                direction = "long" if pos["qty"] > 0 else "short"
                avg = abs(pos["cost"] / pos["qty"])
                print(f"  {direction} {abs(pos['qty'])} {'call' if pc == 'C' else 'put'} {strike:.0f} @ {avg:.2f}")
        print(f"  best case:  ${st['best_pnl']:+,.2f} (SPX @ {st['best_settle']:.0f})")
        print(f"  worst case: ${st['worst_pnl']:+,.2f} (SPX @ {st['worst_settle']:.0f})")
        if settle is not None:
            print(f"  @ {settle:.0f}:    ${st['settle_pnl']:+,.2f}")
        print()

    # Summary table in CP437 box-drawing style
    rows = []
    for mm in sorted(mm_stats.keys()):
        st = mm_stats[mm]
        name = MM_NAMES.get(mm, mm)
        rows.append((name, st["fills"], st["best_pnl"], st["worst_pnl"], st.get("settle_pnl")))
    rows.append(("Justine Street", len(fills), our_best, our_worst, our_settle_pnl))

    has_settle = settle is not None
    cols = ["Party", "Fills", "Best Case", "Worst Case"]
    widths = [14, 7, 14, 14]
    if has_settle:
        cols.append(f"@ {int(settle)}")
        widths.append(14)

    # CP437 box chars
    TL, TR, BL, BR = "\u250c", "\u2510", "\u2514", "\u2518"
    H, V = "\u2500", "\u2502"
    LT, RT, TT, BT, CR = "\u251c", "\u2524", "\u252c", "\u2534", "\u253c"

    def hline(left, mid, right):
        return left + mid.join(H * (w + 2) for w in widths) + right

    def row_str(vals):
        cells = []
        for i, val in enumerate(vals):
            if i == 0:
                cells.append(f" {val:<{widths[i]}} ")
            else:
                cells.append(f" {val:>{widths[i]}} ")
        return V + V.join(cells) + V

    def fmt_money(v):
        if v is None:
            return ""
        return f"${v:>+,.2f}"

    print()
    print(hline(TL, TT, TR))
    print(row_str(cols))
    print(hline(LT, CR, RT))
    for i, (name, n, best, worst, spnl) in enumerate(rows):
        if i == len(rows) - 1:  # separator before YOU
            print(hline(LT, CR, RT))
        vals = [name, str(n), fmt_money(best), fmt_money(worst)]
        if has_settle:
            vals.append(fmt_money(spnl))
        print(row_str(vals))
    print(hline(BL, BT, BR))


if __name__ == "__main__":
    main()
