#!/usr/bin/env gawk -f
# Shows fill latency histogram by PFOF processor (route name).
#
# Usage: gawk -f scripts/fill_latency.awk ~/scratch/boxer-2026-03-18.log.*
#
# For each route, tracks time from "order SENT" to "leg fill" and buckets
# into latency bins. Also shows unfilled orders per route.

function parse_ts(s,    parts, hms, h, mn, sec) {
    split(s, parts, "T")
    split(parts[2], hms, ":")
    h = hms[1] + 0
    mn = hms[2] + 0
    sec = hms[3] + 0.0
    return h * 3600 + mn * 60 + sec
}

# order SENT #1 BUY_TO_OPEN call 6710 @ 6.8 (id=1005738135642)
/order SENT #[0-9]/ {
    match($0, /\(id=([0-9]+)\)/, _id)
    if (_id[1] != "") {
        sent_time[_id[1]] = parse_ts($1)
    }
    match($0, /order SENT (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+)/, _m)
    if (_m[1] != "" && _id[1] != "") {
        _key = _m[1] "|" _m[2] "|" _m[3] "|" _m[4]
        key_to_id[_key] = _id[1]
    }
}

# route #1 BUY_TO_OPEN call 6710 -> WOLF_OPT_F3_J2 @ 6.8
/route #[0-9]/ {
    match($0, /route (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+) -> ([A-Za-z0-9_]+)/, _m)
    if (_m[1] != "") {
        _key = _m[1] "|" _m[2] "|" _m[3] "|" _m[4]
        route_key_exchange[_key] = _m[5]
        _oid = key_to_id[_key]
        if (_oid != "") {
            route_exchange[_oid] = _m[5]
        }
    }
}

# leg fill #1 for 1005738135642 at 6.8 with 0 price improvement from WOLF_OPT_F3_J2
/leg fill #[0-9]/ {
    match($0, /for ([0-9]+) at/, _id)
    match($0, /from ([A-Za-z0-9_]+)/, _ex)
    if (_id[1] != "" && _ex[1] != "") {
        _oid = _id[1]
        _exchange = _ex[1]
        _fill_time = parse_ts($1)
        if (_oid in sent_time) {
            _latency = _fill_time - sent_time[_oid]
            if (_latency < 0) _latency += 86400
            fill_n[_exchange]++
            fill_total[_exchange] += _latency
            if (_latency < 1) bkt[_exchange, "< 1s"]++
            else if (_latency < 2) bkt[_exchange, "1-2s"]++
            else if (_latency < 5) bkt[_exchange, "2-5s"]++
            else if (_latency < 10) bkt[_exchange, "5-10s"]++
            else if (_latency < 30) bkt[_exchange, "10-30s"]++
            else if (_latency < 60) bkt[_exchange, "30-60s"]++
            else if (_latency < 300) bkt[_exchange, "1-5m"]++
            else bkt[_exchange, "> 5m"]++
            filled[_oid] = 1
        }
    }
}

END {
    # count unfilled per route
    for (_key in route_key_exchange) {
        _xchg = route_key_exchange[_key]
        _oid = key_to_id[_key]
        if (_oid != "" && !(_oid in filled)) {
            unfilled[_xchg]++
        }
    }

    # collect all exchanges
    for (_e in fill_n) all_exchanges[_e] = 1
    for (_e in unfilled) all_exchanges[_e] = 1

    nbkt = 8
    bkt_names[1] = "< 1s"
    bkt_names[2] = "1-2s"
    bkt_names[3] = "2-5s"
    bkt_names[4] = "5-10s"
    bkt_names[5] = "10-30s"
    bkt_names[6] = "30-60s"
    bkt_names[7] = "1-5m"
    bkt_names[8] = "> 5m"

    # find max count for scaling
    max_count = 0
    for (_e in all_exchanges) {
        for (i = 1; i <= nbkt; i++) {
            c = bkt[_e, bkt_names[i]] + 0
            if (c > max_count) max_count = c
        }
    }
    bar_width = 40

    # sort exchanges by avg latency
    asorti(all_exchanges, sorted)

    for (ei = 1; ei <= length(sorted); ei++) {
        _e = sorted[ei]
        fills = fill_n[_e] + 0
        uf = unfilled[_e] + 0
        avg = (fills > 0) ? fill_total[_e] / fills : 0
        printf "\n%s  fills=%d unfilled=%d avg=%.1fs\n", _e, fills, uf, avg
        printf "  %-7s %5s  %s\n", "bucket", "count", ""
        for (i = 1; i <= nbkt; i++) {
            c = bkt[_e, bkt_names[i]] + 0
            if (c > 0 || i <= 4) {
                bar_len = (max_count > 0) ? int(c * bar_width / max_count) : 0
                bar = ""
                for (j = 0; j < bar_len; j++) bar = bar "#"
                printf "  %-7s %5d  %s\n", bkt_names[i], c, bar
            }
        }
    }
}
