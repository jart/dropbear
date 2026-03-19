#!/usr/bin/env gawk -f
# Checks whether the RouteName ever changes between route and fill for a given order.
#
# Usage: gawk -f scripts/route_changes.awk ~/scratch/boxer-2026-03-18.log.*
#
# Tracks three events per order ID:
#   1. "order SENT #N ... (id=ORDERID)" — records leg name
#   2. "route #N ... -> ROUTE @ PRICE"  — records route exchange (matched via leg+strike)
#      OR JSON "RouteName" after "ExecutionRequested" — records route exchange by order ID
#   3. "leg fill #N for ORDERID ... from EXCHANGE" — records fill exchange
#
# Reports any order where the fill exchange differs from the route exchange.

# order SENT #1 BUY_TO_OPEN call 6710 @ 6.8 (id=1005738135642)
/order SENT #[0-9]/ {
    match($0, /order SENT (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+)/, m)
    match($0, /\(id=([0-9]+)\)/, id)
    if (m[1] != "" && id[1] != "") {
        leg = m[1]
        instr = m[2]
        class = m[3]
        strike = m[4]
        key = leg "|" instr "|" class "|" strike
        sent_key_to_id[key] = id[1]
        sent_id_to_key[id[1]] = key
        sent_id_to_desc[id[1]] = leg " " instr " " class " " strike
    }
}

# route #1 BUY_TO_OPEN call 6710 -> WOLF_OPT_F3_J2 @ 6.8
/route #[0-9]/ {
    match($0, /route (#[0-9]+) ([A-Z_]+) ([a-z]+) ([0-9]+) -> ([A-Za-z0-9_]+)/, m)
    if (m[1] != "") {
        key = m[1] "|" m[2] "|" m[3] "|" m[4]
        oid = sent_key_to_id[key]
        if (oid != "") {
            route_exchange[oid] = m[5]
        }
    }
}

# JSON RouteName from ExecutionRequested events (keyed by preceding order ID line)
/order [0-9]+: ExecutionRequested/ {
    match($0, /order ([0-9]+):/, m)
    pending_order_id = m[1]
}

# Catch RouteName in JSON after ExecutionRequested
pending_order_id && /"RouteName":/ {
    match($0, /"RouteName": "([^"]+)"/, m)
    if (m[1] != "" && pending_order_id != "") {
        if (!(pending_order_id in route_exchange)) {
            route_exchange[pending_order_id] = m[1]
        }
        pending_order_id = ""
    }
}

# Reset pending on blank line or next order event
/^[0-9]{4}-[0-9]{2}-[0-9]{2}T/ {
    if (pending_order_id && !/ExecutionRequested/ && !/"RouteName"/) {
        # still in JSON block, don't reset
    }
}

# leg fill #1 for 1005738135642 at 6.8 with 0 price improvement from WOLF_OPT_F3_J2
/leg fill #[0-9]/ {
    match($0, /for ([0-9]+) at/, id)
    match($0, /from ([A-Za-z0-9_]+)/, ex)
    if (id[1] != "" && ex[1] != "") {
        oid = id[1]
        fill = ex[1]
        routed = route_exchange[oid]
        desc = sent_id_to_desc[oid]
        if (desc == "") desc = "unknown"
        if (routed != "") {
            if (routed != fill) {
                printf "CHANGED %s (id=%s): routed to %s, filled at %s\n", desc, oid, routed, fill
                changes++
            } else {
                same++
            }
        } else {
            # no route event seen for this order
            no_route++
        }
    }
}

END {
    printf "\nsummary: %d same, %d changed, %d no-route-seen\n", same+0, changes+0, no_route+0
}
