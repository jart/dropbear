#!/bin/sh
# ./scripts/test.sh spread -spread 4 -samples 7000

SCRIPTS=${0%/*}
CMD=${1:-spread}
shift 1
ARGS="$*"

REPORT=$(mktemp /tmp/dropbear.XXXXXX) || exit 1
RESULTS=$(mktemp /tmp/dropbear.XXXXXX) || exit 1
trap onexit EXIT

onexit() {
    rm -f "$RESULTS" "$REPORT"
}

run() {
    COIN=$1
    (
        OUTPATH=$(mktemp /tmp/dropbear.XXXXXX) || exit 1
        EXECUTE="go run ./cmd/$CMD -backtest $DATASET -symbol $COIN $ARGS"
        # echo $EXECUTE >&2
        if $EXECUTE >$OUTPATH 2>/dev/null; then
            VOLUME=$(grep end.vol30day $OUTPATH | awk '{print $2}')
            PROFIT=$(grep end.profit $OUTPATH | awk '{print $2}')
            PROFIT_BENCH=$(grep bench.profit $OUTPATH | awk '{print $2}')
            SHARPE=$(grep end.sharpe $OUTPATH | awk '{print $2}')
            SHARPE_BENCH=$(grep bench.sharpe $OUTPATH | awk '{print $2}')
            BUYS=$(grep end.buys $OUTPATH | awk '{print $2}')
            SELLS=$(grep end.sells $OUTPATH | awk '{print $2}')
            INVESTED=$(grep invested.avg $OUTPATH | awk '{print $2}')
            if [ "$(echo "$SHARPE > $SHARPE_BENCH" | bc)" -eq 1 ]; then
                if [ "$(echo "$PROFIT > $PROFIT_BENCH" | bc)" -eq 1 ]; then
                    JUDGEMENT="GREAT"
                else
                    JUDGEMENT="good"
                fi
            elif [ "$(echo "$PROFIT > $PROFIT_BENCH" | bc)" -eq 1 ]; then
                JUDGEMENT="brave"
            else
                JUDGEMENT="bad"
            fi
            printf "%s %-10s %-4s vol=%4.1f profit=%8.2f sharpe=%7s buys=%4d sells=%4d invested=%5s %s\n" \
                "$CMD" "$DATASET" "$COIN" "$VOLUME" "$PROFIT" "$SHARPE" "$BUYS" "$SELLS" "$INVESTED" "$JUDGEMENT" >>"$REPORT"
            echo "$VOLUME $PROFIT $PROFIT_BENCH $SHARPE $SHARPE_BENCH $INVESTED $JUDGEMENT" >>"$RESULTS"
            rm -f $OUTPATH
        else
            printf "%s %-10s %-4s failed\n" "$CMD" "$DATASET" "$COIN"
        fi
    ) &
}

for DATASET in $(cd ~/marketdata/; ls -1); do
    for COIN in $(cd ~/marketdata/$DATASET/coinbase/; ls -1 *-USD | awk -F- '{print $1}'); do
        if [ $COIN = USDT ]; then
            continue
        fi
        run $COIN
    done
done
wait

# print summary
sort <"$REPORT"
awk -f "$SCRIPTS/test.awk" "$RESULTS"
