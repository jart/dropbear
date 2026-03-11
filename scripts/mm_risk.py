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
    r"fill #(\d+) "
    r"SPXW\s+(\d{6})([CP])(\d{8})"  # leg#, expiry, put/call, strike*1000
    r" via (\S+)"                     # route name
    r" @ (\S+)"                       # fill price
)

# order SENT #1 BUY_TO_OPEN call 6820 @ 13.80 (id=1005654201334)
ORDER_RE = re.compile(
    r"order SENT #(\d+) "
    r"(BUY_TO_OPEN|SELL_TO_OPEN) "
    r"\S+ \S+ "                       # class and strike (ignored, matched by leg#)
    r"@ (\S+)"                        # limit price
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
    fills = []  # (mm, pc, strike, fill_price, we_bought, price_improvement)
    pending_orders = {}  # leg# -> (limit_price, is_buy)

    with open(logfile) as f:
        for line in f:
            # Track order SENT lines to get limit prices
            om = ORDER_RE.search(line)
            if om:
                leg_num, instruction, limit_str = om.groups()
                is_buy = instruction == "BUY_TO_OPEN"
                pending_orders[leg_num] = (float(limit_str), is_buy)
                continue

            m = FILL_RE.search(line)
            if not m:
                continue
            leg_num, expiry, pc, strike_str, route, price_str = m.groups()
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

            # Compute price improvement from limit vs fill
            pi = 0.0
            if leg_num in pending_orders:
                limit_price, is_buy = pending_orders[leg_num]
                if is_buy:
                    pi = limit_price - price  # bought cheaper than limit
                else:
                    pi = price - limit_price  # sold higher than limit
            fills.append((mm, pc, strike, price, we_bought, pi))

    if not fills:
        print("no fills found")
        sys.exit(1)

    # Build market maker positions
    # When we buy, the MM sells (short). When we sell, the MM buys (long).
    mm_positions = defaultdict(list)
    mm_pi = defaultdict(float)  # total price improvement per MM (* 100)
    for mm, pc, strike, price, we_bought, pi in fills:
        mm_qty = -1 if we_bought else +1
        mm_positions[mm].append((pc, strike, mm_qty, price))
        mm_pi[mm] += pi * MULTIPLIER

    # Test prices for best/worst case within +-100 of current range
    all_strikes = sorted(set(strike for _, _, strike, _, _, _ in fills))
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
    total_pi = sum(mm_pi.values())
    rows = []
    for mm in sorted(mm_stats.keys()):
        st = mm_stats[mm]
        name = MM_NAMES.get(mm, mm)
        lo_pnl = compute_pnl(st["net"], lo)
        hi_pnl = compute_pnl(st["net"], hi)
        rows.append((name, st["fills"], lo_pnl, hi_pnl, st.get("settle_pnl"), mm_pi[mm]))
    our_lo_pnl = compute_pnl(our_net, lo)
    our_hi_pnl = compute_pnl(our_net, hi)
    rows.append(("Justine Street", len(fills), our_lo_pnl, our_hi_pnl, our_settle_pnl, total_pi))

    has_settle = settle is not None
    lo_label = f"SPX -100"
    hi_label = f"SPX +100"
    cols = ["Party", "Fills", lo_label, hi_label]
    widths = [14, 7, 14, 14]
    if has_settle:
        cols.append(f"@ {int(settle)}")
        widths.append(14)
    cols.append("Price Impr")
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
    for i, (name, n, lo_pnl, hi_pnl, spnl, pi) in enumerate(rows):
        if i == len(rows) - 1:  # separator before Justine Street
            print(hline(LT, CR, RT))
        vals = [name, str(n), fmt_money(lo_pnl), fmt_money(hi_pnl)]
        if has_settle:
            vals.append(fmt_money(spnl))
        vals.append(fmt_money(pi))
        print(row_str(vals))
    print(hline(BL, BT, BR))


if __name__ == "__main__":
    main()
