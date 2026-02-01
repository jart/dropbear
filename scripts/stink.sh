#!/usr/bin/env bash
ARGS="$*"
(
  for dataset in dogdays churu bone dreary drop chaos; do
    for coin in ETH SOL LTC ZEC BTC; do
    # for coin in ZEC; do
      (
        printf "%-12s %-4s %s\n" $dataset $coin $(go run ./cmd/stink-dynamic -backtest $dataset -symbol $coin $ARGS |& grep cagr | awk2 | head -n1)
      ) &
    done
  done
  wait
) | sort
