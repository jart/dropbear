#!/usr/bin/env python3
"""Analyze varu order log lines for risk/reward distribution.

Usage:
    python scripts/order_analysis.py ~/scratch/varu-2026-03-31.log.1
"""

import re
import sys
import statistics

ORDER_RE = re.compile(
    r'#\d+ (?P<strategy>vertical \w+ \w+): '
    r'price=(?P<price>[-\d.]+) '
    r'natural=(?P<natural>[-\d.]+) '
    r'maker=(?P<maker>[-\d.]+) '
    r'score=(?P<score>[-\d.]+) '
    r'payoff=(?P<payoff_from>[-\d.]+?)->(?P<payoff_to>[-\d.]+) '
    r'risk=(?P<risk_from>[-\d.,]+?)->(?P<risk_to>[-\d.,]+)'
)


def parse_orders(path):
    orders = []
    with open(path) as f:
        for line in f:
            m = ORDER_RE.search(line)
            if not m:
                continue
            d = m.groupdict()
            o = {
                'strategy': d['strategy'],
                'price': float(d['price']),
                'score': float(d['score']),
                'payoff_delta': float(d['payoff_to']) - float(d['payoff_from']),
                'risk_delta': float(d['risk_from']) - float(d['risk_to']),
            }
            orders.append(o)
    return orders


def summarize(name, values):
    if len(values) > 1:
        print(f'  {name:16s}  avg={statistics.mean(values):9.2f}  med={statistics.median(values):9.2f}  '
              f'min={min(values):9.2f}  max={max(values):9.2f}  std={statistics.stdev(values):9.2f}')
    elif values:
        print(f'  {name:16s}  avg={values[0]:9.2f}')


def bucket(values, edges, labels):
    counts = [0] * len(labels)
    for v in values:
        placed = False
        for i, edge in enumerate(edges):
            if v < edge:
                counts[i] += 1
                placed = True
                break
        if not placed:
            counts[-1] += 1
    return list(zip(labels, counts))


def analyze(orders):
    strategies = sorted(set(o['strategy'] for o in orders))
    for strat in strategies:
        group = [o for o in orders if o['strategy'] == strat]
        ratios = [o['payoff_delta'] / abs(o['risk_delta']) for o in group if abs(o['risk_delta']) > 0.01]
        print(f'\n{strat}  (n={len(group)})')
        summarize('payoff Δ', [o['payoff_delta'] for o in group])
        summarize('risk Δ', [o['risk_delta'] for o in group])
        if ratios:
            summarize('payoff/|risk|', ratios)
            print()
            for label, count in bucket(ratios, [0.00025, 0.0005, 0.0010, 0.0025, 0.005, 0.01, 0.02, 0.05, 0.10, .2, .3, .4, .5, .6, .7, .8, .9, 1.0],
                                       ['<.00025', '.00025-.0005', '.0005-.0010', '.0010-.0025', '.0025-.005', '.005-.01', '.01-.02', '.02-.05', '.05-.10', '.10-.20', '.20-.30', '.30-.40', '.40-.50', '.50-.60', '.60-.70', '.70-.80', '.80-.90', '.90-1.0']):
                pct = count * 100 / len(ratios)
                bar = '█' * int(pct / 2)
                print(f'    {label:12s} {count:4d} ({pct:5.1f}%) {bar}')


def main():
    if len(sys.argv) < 2:
        print(f'usage: {sys.argv[0]} <logfile>', file=sys.stderr)
        sys.exit(1)
    orders = parse_orders(sys.argv[1])
    if not orders:
        print('no order lines found', file=sys.stderr)
        sys.exit(1)
    analyze(orders)


if __name__ == '__main__':
    main()
