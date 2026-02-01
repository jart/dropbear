#!/usr/bin/env bash
ARGS="$*"
(
  for dataset in dogdays churu bone dreary weekend; do
    for coin in ETH SOL; do
      (
        printf "%-12s %-4s %s\n" $dataset $coin $(go run ./cmd/stink-dynamic -backtest $dataset -symbol $coin $ARGS |& grep sharpe | awk2 | head -n1)
      ) &
    done
  done
  wait
) | sort
