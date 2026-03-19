#!/usr/bin/env gawk -f
# Shows P&L by PFOF processor and total user P&L from filled legs.
#
# Usage: gawk -f scripts/route_pnl.awk -v spx=6624.70 ~/scratch/boxer-2026-03-18.log.*
#
# Each filled 0DTE SPX option settles at intrinsic value:
#   call = max(0, SPX - strike)
#   put  = max(0, strike - SPX)
#
# User P&L per leg:
#   bought: settlement - fill_price
#   sold:   fill_price - settlement
#
# Market maker P&L = -user P&L (they are the counterparty).
# All dollar amounts use the 100x SPX multiplier.

BEGIN {
    if (spx == 0) {
        print "usage: gawk -f scripts/route_pnl.awk -v spx=CLOSE_PRICE logfiles..." > "/dev/stderr"
        exit 1
    }
}

# order SENT #1 BUY_TO_OPEN call 6710 @ 6.8 (id=1005738135642)
/order SENT #[0-9]/ {
    match($0, /order SENT (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+)/, _m)
    match($0, /\(id=([0-9]+)\)/, _id)
    if (_m[1] != "" && _id[1] != "") {
        _oid = _id[1]
        order_instr[_oid] = _m[2]   # BUY_TO_OPEN or SELL_TO_OPEN
        order_class[_oid] = _m[3]   # call or put
        order_strike[_oid] = _m[4] + 0
        _key = _m[1] "|" _m[2] "|" _m[3] "|" _m[4]
        key_to_id[_key] = _oid
    }
}

# route #1 BUY_TO_OPEN call 6710 -> WOLF_OPT_F3_J2 @ 6.8
/route #[0-9]/ {
    match($0, /route (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+) -> ([A-Za-z0-9_]+)/, _m)
    if (_m[1] != "") {
        _key = _m[1] "|" _m[2] "|" _m[3] "|" _m[4]
        _oid = key_to_id[_key]
        if (_oid != "") {
            order_route[_oid] = _m[5]
        }
    }
}

# leg fill #1 for ID at PRICE ... from ROUTE
# leg filled for order id ID at PRICE ... from ROUTE route:
/leg fill/ {
    match($0, /(for|for order id) ([0-9]+) at ([0-9.]+)/, _m)
    match($0, /from ([A-Za-z0-9_]+)/, _ex)
    if (_m[2] != "") {
        _oid = _m[2]
        _fill = _m[3] + 0.0
        _xchg = _ex[1]

        _strike = order_strike[_oid]
        _class = order_class[_oid]
        _instr = order_instr[_oid]
        _route = order_route[_oid]
        if (_route == "") _route = _xchg

        # settlement value
        if (_class == "call") {
            _settle = (spx > _strike) ? spx - _strike : 0
        } else {
            _settle = (_strike > spx) ? _strike - spx : 0
        }

        # user P&L per contract (in option points)
        if (_instr ~ /BUY/) {
            _user_pnl = _settle - _fill
        } else {
            _user_pnl = _fill - _settle
        }
        # market maker is counterparty
        _mm_pnl = -_user_pnl

        # accumulate (multiply by 100 for SPX dollar value)
        route_mm_pnl[_route] += _mm_pnl * 100
        route_user_pnl[_route] += _user_pnl * 100
        route_fills[_route]++
        total_user += _user_pnl * 100
        total_mm += _mm_pnl * 100
        total_fills++

        # track per-leg detail for debugging
        fill_detail[_oid] = sprintf("%s %s %s %d @ %.2f settle=%.2f user=%.2f mm=%.2f route=%s",
            _instr, _class, _strike, _strike, _fill, _settle, _user_pnl, _mm_pnl, _route)
    }
}

END {
    printf "SPX close: %.2f\n", spx
    printf "Filled legs: %d\n\n", total_fills

    # sort by MM profit descending
    for (_r in route_fills) {
        sort_key[_r] = sprintf("%020.2f|%s", 9999999999 - route_mm_pnl[_r], _r)
    }
    asorti(sort_key, sorted)

    printf "%-25s %6s %10s %10s %10s\n", "ROUTE", "FILLS", "MM P&L", "YOUR P&L", "AVG/FILL"
    printf "%-25s %6s %10s %10s %10s\n", "-------------------------", "------", "----------", "----------", "----------"

    for (si = 1; si <= length(sorted); si++) {
        _r = sorted[si]
        split(sort_key[_r], _parts, "|")
        xchg = _parts[2]
        fills = route_fills[xchg]
        mm = route_mm_pnl[xchg]
        usr = route_user_pnl[xchg]
        avg_mm = mm / fills
        printf "%-25s %6d %+10.2f %+10.2f %+10.2f\n", xchg, fills, mm, usr, avg_mm
    }

    printf "%-25s %6s %10s %10s %10s\n", "-------------------------", "------", "----------", "----------", "----------"
    printf "%-25s %6d %+10.2f %+10.2f\n", "TOTAL", total_fills, total_mm, total_user
}
