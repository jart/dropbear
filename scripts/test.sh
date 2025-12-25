#!/bin/sh
# ./scripts/test.sh spread -spread 4 -samples 7000

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
                JUDGEMENT="loco"
            else
                JUDGEMENT="bad"
            fi
            printf "%s %-10s %-4s vol=%5s profit=%8s sharpe=%7s buys=%3d sells=%3d invested=%5s %s\n" \
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
awk '
BEGIN { great=0; good=0; bad=0; n=0; volume=0; profit=0; profit_bench=0; sharpe=0; sharpe_bench=0; invested=0 }
{
    n++
    volume += $1
    profit += $2
    profit_bench += $3
    sharpe += $4
    sharpe_bench += $5
    invested += $6
    if ($7 == "GREAT") great++
    else if ($7 == "good") good++
    else if ($7 == "bad") bad++
}
END {
    if (n > 0) {
        printf "total n=%d volume=%.2f profit=%+.2f (vs. %+.2f) sharpe=%+.2f (vs. %+.2f) invested=%.0f | GREAT=%d good=%d bad=%d\n",
            n, volume/n, profit/n, profit_bench/n, sharpe/n, sharpe_bench/n, invested/n, great, good, bad
    }
}
' $RESULTS
