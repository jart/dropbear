BEGIN {
    great = 0
    good = 0
    brave = 0
    bad = 0
    n = 0
}

{
    n++
    w = $6  # invested amount as weight
    total_weight += w
    volume += $1 * w
    profit += $2 * w
    profit_bench += $3 * w
    sharpe += $4 * w
    sharpe_bench += $5 * w
    invested += w
    if ($7 == "GREAT") great++
    else if ($7 == "good") good++
    else if ($7 == "brave") brave++
    else if ($7 == "bad") bad++
}

END {
    if (total_weight > 0) {
        printf "total n=%d volume=%.2f profit=%+.2f (vs. %+.2f) sharpe=%+.2f (vs. %+.2f) invested=%.0f | GREAT=%d good=%d brave=%d bad=%d\n",
            n, volume/total_weight, profit/total_weight, profit_bench/total_weight,
            sharpe/total_weight, sharpe_bench/total_weight, invested/n, great, good, brave, bad
    }
}
