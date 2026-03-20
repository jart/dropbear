#!/usr/bin/env gawk -f
# Analyzes whether partially-filled boxes rose in value while waiting for remaining legs.
#
# Usage: gawk -f scripts/partial_box_value.awk ~/scratch/boxer-2026-03-19.log.*
#
# For each box that took more than 5 seconds to complete, computes:
# - The profit at creation (from the "best box" line)
# - The profit at each heartbeat (from the "profit=X->Y" in the box string)
# - The final profit at fill (from the "box filled" line)
# - Whether the box's value *increased* while partially filled
#
# The "profit=X->Y" field in box/leg strings shows original->current profit.
# "box filled: sell -25w box @ 25.3 (market=-25.025 profit=0.3->0.55 es=6573)"
# means original estimate was 0.3, current estimate at fill time is 0.55.

# best box: sell -15w box @ 15.3 (market=-15.1 profit=0.3->0.3 es=6572.875)
/best box:/ {
    match($0, /^([0-9T:.-]+) best box:/, _ts)
    match($0, /profit=([0-9.-]+)->([0-9.-]+)/, _p)
    if (_ts[1] != "" && _p[1] != "") {
        last_box_time = _ts[1]
        last_box_profit_orig = _p[1] + 0
        last_box_profit_curr = _p[2] + 0
    }
}

# box filled: sell -25w box @ 25.3 (market=-25.025 profit=0.3->0.55 es=6573)
/box filled:/ {
    match($0, /^([0-9T:.-]+) box filled:/, _ts)
    match($0, /profit=([0-9.-]+)->([0-9.-]+)/, _p)
    match($0, /(buy|sell) (-?[0-9]+)w box/, _box)
    if (_ts[1] != "" && _p[1] != "") {
        fill_time = _ts[1]
        orig = _p[1] + 0
        final = _p[2] + 0
        delta = final - orig
        width = _box[2]
        if (width < 0) width = -width
        direction = _box[1]

        # compute time difference in seconds
        split(last_box_time, _a, "T")
        split(_a[2], _b, ":")
        t1 = _b[1]*3600 + _b[2]*60 + _b[3]
        split(fill_time, _c, "T")
        split(_c[2], _d, ":")
        t2 = _d[1]*3600 + _d[2]*60 + _d[3]
        elapsed = t2 - t1
        if (elapsed < 0) elapsed += 86400

        total_boxes++
        total_orig += orig
        total_final += final

        if (elapsed >= 5) {
            slow_boxes++
            slow_orig += orig
            slow_final += final
            if (final > orig + 0.001) {
                improved++
                improved_gain += delta
            } else if (final < orig - 0.001) {
                degraded++
                degraded_loss += delta
            } else {
                unchanged++
            }

            # bucket by time
            if (elapsed < 10) bkt_time = "5-10s"
            else if (elapsed < 30) bkt_time = "10-30s"
            else if (elapsed < 60) bkt_time = "30-60s"
            else if (elapsed < 300) bkt_time = "1-5m"
            else bkt_time = "> 5m"
            bkt_count[bkt_time]++
            bkt_improved[bkt_time] += (final > orig + 0.001) ? 1 : 0
            bkt_degraded[bkt_time] += (final < orig - 0.001) ? 1 : 0
            bkt_gain[bkt_time] += delta

            # bucket by improvement size
            if (delta >= 0.3) big_improve++
            else if (delta >= 0.1) med_improve++

            # track best/worst
            if (delta > best_delta) {
                best_delta = delta
                best_desc = sprintf("%s %s %sw elapsed=%ds orig=%.2f final=%.2f", fill_time, direction, width, elapsed, orig, final)
            }
            if (delta < worst_delta) {
                worst_delta = delta
                worst_desc = sprintf("%s %s %sw elapsed=%ds orig=%.2f final=%.2f", fill_time, direction, width, elapsed, orig, final)
            }
        }
    }
}

END {
    printf "Partial Box Value Analysis\n"
    printf "==========================\n\n"

    printf "Total boxes filled:    %d\n", total_boxes
    printf "Total original profit: $%.2f ($%.0f notional)\n", total_orig, total_orig * 100
    printf "Total final profit:    $%.2f ($%.0f notional)\n", total_final, total_final * 100
    printf "Total improvement:     $%.2f ($%.0f notional)\n\n", total_final - total_orig, (total_final - total_orig) * 100

    printf "Slow boxes (>= 5s to fill): %d of %d (%.0f%%)\n\n", slow_boxes, total_boxes, 100*slow_boxes/total_boxes

    if (slow_boxes > 0) {
        printf "Of %d slow boxes:\n", slow_boxes
        avg_gain = (improved > 0) ? improved_gain/improved : 0
        avg_loss = (degraded > 0) ? degraded_loss/degraded : 0
        printf "  Improved (profit went up):   %d (%.0f%%)  avg gain: $%.3f\n", improved, 100*improved/slow_boxes, avg_gain
        printf "  Unchanged:                   %d (%.0f%%)\n", unchanged, 100*unchanged/slow_boxes
        printf "  Degraded (profit went down): %d (%.0f%%)  avg loss: $%.3f\n", degraded, 100*degraded/slow_boxes, avg_loss
        printf "\n"
        printf "  Big improvements (>= $0.30): %d\n", big_improve
        printf "  Med improvements (>= $0.10): %d\n\n", med_improve

        printf "By fill time:\n"
        printf "  %-8s %6s %8s %8s %10s\n", "Bucket", "Count", "Improved", "Degraded", "Avg Delta"
        printf "  %-8s %6s %8s %8s %10s\n", "--------", "------", "--------", "--------", "----------"
        nbkt = 5
        bkt_names[1] = "5-10s"
        bkt_names[2] = "10-30s"
        bkt_names[3] = "30-60s"
        bkt_names[4] = "1-5m"
        bkt_names[5] = "> 5m"
        for (i = 1; i <= nbkt; i++) {
            b = bkt_names[i]
            if (bkt_count[b] > 0) {
                printf "  %-8s %6d %8d %8d %+10.3f\n", b, bkt_count[b], bkt_improved[b], bkt_degraded[b], bkt_gain[b]/bkt_count[b]
            }
        }

        printf "\n  Best:  %s\n", best_desc
        printf "  Worst: %s\n", worst_desc
    }
}
