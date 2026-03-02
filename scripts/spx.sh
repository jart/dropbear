#!/usr/bin/env bash
sym=${1:-SPX}
date=${2:-2026-02-27}
go run ./cmd/boxer \
    -date ${date} \
    -defs ~/databento/${sym}/${date}.definition.dbn \
    -data ~/databento/${sym}/${date}.0dte.cmbp1.dbn \
    -width 50 \
    -edge 0.10 \
    -maxedge 2.00 \
    -maxspread 0.50 \
    -minbid 0.05 \
    -maxopen 2 \
    -latency 70ms \
    -greed 0.30 \
    -patience 15s \
    -ratelimit 120 \
    -start 06:30:05 \
    -cutoff 07:30:00 \
    -v
