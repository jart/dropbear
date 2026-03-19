#!/usr/bin/env gawk -f
# Shows price improvement statistics by PFOF processor (route name).
#
# Usage: gawk -f scripts/price_improvement.awk ~/scratch/boxer-2026-03-18.log.*
#
# Parses "leg fill" lines which contain "with N price improvement from EXCHANGE".
# The price improvement value is in cents (e.g. "5" = $0.05).

# leg fill #1 for 1005738135642 at 6.8 with 0 price improvement from WOLF_OPT_F3_J2
/leg fill #[0-9]/ {
    match($0, /with ([0-9.]+) price improvement from ([A-Za-z0-9_]+)/, _m)
    if (_m[2] != "") {
        _xchg = _m[2]
        _imp = _m[1] + 0
        fill_n[_xchg]++
        total_imp[_xchg] += _imp
        if (_imp > 0) improved_n[_xchg]++
        # bucket by improvement amount (in cents)
        if (_imp == 0) bkt[_xchg, "0"]++
        else if (_imp <= 5) bkt[_xchg, "1-5"]++
        else if (_imp <= 10) bkt[_xchg, "6-10"]++
        else bkt[_xchg, ">10"]++
    }
}

END {
    nbkt = 4
    bkt_names[1] = "0"
    bkt_names[2] = "1-5"
    bkt_names[3] = "6-10"
    bkt_names[4] = ">10"
    bkt_labels[1] = "$0.00"
    bkt_labels[2] = "$0.01-0.05"
    bkt_labels[3] = "$0.06-0.10"
    bkt_labels[4] = ">$0.10"

    # find max for scaling
    max_count = 0
    for (_e in fill_n) {
        for (i = 1; i <= nbkt; i++) {
            c = bkt[_e, bkt_names[i]] + 0
            if (c > max_count) max_count = c
        }
    }
    bar_width = 40

    # sort by avg improvement descending — build sortable keys
    for (_e in fill_n) {
        avg = total_imp[_e] / fill_n[_e]
        # gawk asorti sorts lexically, so zero-pad the inverse for descending
        sort_key[_e] = sprintf("%010.4f|%s", 9999 - avg, _e)
    }
    asorti(sort_key, sorted_keys)

    for (si = 1; si <= length(sorted_keys); si++) {
        _e = sorted_keys[si]
        # recover exchange name from sort_key value
        split(sort_key[_e], _parts, "|")
        xchg = _parts[2]
        fills = fill_n[xchg] + 0
        imp_count = improved_n[xchg] + 0
        avg = (fills > 0) ? total_imp[xchg] / fills : 0
        total_dollars = total_imp[xchg] / 100
        pct = (fills > 0) ? 100 * imp_count / fills : 0
        printf "\n%s  fills=%d improved=%d (%.0f%%) avg=$%.2f total=$%.2f\n",
            xchg, fills, imp_count, pct, avg / 100, total_dollars
        printf "  %-12s %5s  %s\n", "improvement", "count", ""
        for (i = 1; i <= nbkt; i++) {
            c = bkt[xchg, bkt_names[i]] + 0
            if (c > 0) {
                bar_len = (max_count > 0) ? int(c * bar_width / max_count) : 0
                bar = ""
                for (j = 0; j < bar_len; j++) bar = bar "#"
                printf "  %-12s %5d  %s\n", bkt_labels[i], c, bar
            }
        }
    }
}
