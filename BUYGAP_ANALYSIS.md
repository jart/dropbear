# Buy Gap Protection Analysis

## Summary

The buy gap protection is **extremely aggressive**, blocking 90% of buy signals for BTC and 98% for DOGE. The decay function has minimal practical effect because the median time between sell and next buy signal is effectively 0 seconds.

## Tool Created

`cmd/buygap/main.go` - Analyzer for buy gap protection effectiveness

Usage:
```bash
go run ./cmd/buygap -dataset chaos -symbol BTC
go run ./cmd/buygap -dataset chaos -symbol BTC -buygap 3
go run ./cmd/buygap -dataset chaos -symbol BTC -decay 60s
```

## Key Findings

### 1. Gap Protection is Very Restrictive

**BTC (buygap=5bps, decay=30s):**
- Total buy signals: 10,578
- Executed: 1,077 (10.2%)
- Blocked: 9,501 (89.8%)

**DOGE (buygap=3bps, decay=30s):**
- Total buy signals: 87,187
- Executed: 1,645 (1.9%)
- Blocked: 85,542 (98.1%)

The gap protection is working as designed - preventing accumulation of lots at similar prices. However, it may be **too aggressive**, especially for DOGE.

### 2. Decay Factor Has Minimal Effect

**Time Since Last Sell Distribution:**
- Median (P50): 0.0s
- P95: 0.4s
- Max: 1.0s

The decay mechanism assumes there will be substantial time between sells and subsequent buy signals, but in practice:
- Buy signals cluster very tightly in time
- Most new buy signals occur within milliseconds of the last sell
- The exponential decay `e^(-timeRatio*periodScale)` rarely has time to decay significantly

**Decay Factor Distribution:**
- Mean: 0.43
- Median: 0.42
- This shows the decay is active, but the low time-since-sell means it doesn't help much

### 3. Inventory Scaling Works But Rarely Maxes Out

**Inventory Scale Distribution (BTC):**
- Mean: 1.17x
- Median: 1.02x
- Max: 2.00x (at 100% of target)

The inventory scaling (1 + inventory/target) successfully increases the gap as position grows:
- Early phase (<25% inventory): 90.4% blocked
- Over target (>100%): 100% blocked

However, most of the time the bot operates at low inventory levels, so scaling has limited impact.

### 4. Actual Gap vs Required Gap

**For Blocked Signals (BTC, buygap=5bps):**
- Actual gap median: -2.37bps (price is 2.37bps ABOVE last buy, not below)
- Required gap median: 2.56bps
- This means most blocked signals are trying to buy at nearly the same price or higher

The negative actual gaps show the gap protection is correctly preventing "ratchet buys" where we'd accumulate at progressively higher prices.

## Parameter Sensitivity

### Varying Buy Gap (BTC, decay=30s)

| Buy Gap | Executed | Blocked | Block Rate |
|---------|----------|---------|------------|
| 3bps    | ~1100    | ~9500   | ~89%       |
| 5bps    | 1077     | 9501    | 89.8%      |
| 7bps    | 1031     | 9547    | 90.3%      |

**Finding**: Block rate is relatively insensitive to gap size (89-90%). The gap size matters more for the *quality* of trades than quantity.

### Varying Decay Period (BTC, buygap=5bps)

| Decay   | Executed | Blocked | Block Rate |
|---------|----------|---------|------------|
| 30s     | 1077     | 9501    | 89.8%      |
| 60s     | 781      | 9797    | 92.6%      |

**Finding**: Longer decay period → MORE blocking (because decay factor stays higher longer). But given time-since-sell is so low, this effect is minimal.

## Recommendations

### 1. Investigate Signal Clustering

**Problem**: Median time-since-sell is 0s, which suggests buy signals occur in rapid bursts immediately after sells.

**Possible causes:**
- Market oscillates rapidly around spread threshold
- Spread EMA lags behind actual spread, creating false signals
- Binance-Coinbase spread mean-reverts very quickly

**Action**: Add temporal analysis to understand signal timing patterns.

### 2. Consider Alternative Decay Functions

Current decay is exponential: `e^(-timeRatio*periodScale)`

Given the very short time scales, consider:
- **Linear decay**: `max(0, 1 - timeRatio/period)` - simpler and more predictable
- **Step function**: Full protection for first N seconds, then off
- **No decay**: If signals cluster this tightly, decay may not be useful

### 3. Reduce Gap for Lower Volatility Coins

**DOGE blocks 98% of signals** - may be too restrictive.

Consider:
- DOGE: buygap = 2-3bps (currently uses same 5bps as BTC)
- Or: Scale gap by coin volatility (use ATR or realized vol)

### 4. Gap Effectiveness is Good

The gap **is working correctly** to prevent ratchet buys:
- Median actual gap is negative (trying to buy higher than last buy)
- 90% block rate shows strong protection
- Inventory scaling increases protection as position grows

**Don't remove it** - it's preventing bad behavior. Question is whether it's *too strong*.

### 5. Consider Volume-Based Protection

Alternative to time-based decay: decay based on volume traded since last sell.
- More volume = more mean reversion = safer to buy again
- Less sensitive to microsecond-level timing

## Code References

**Buy gap logic**: `cmd/spread/main.go:381-434`
- Lines 393-397: Inventory scaling
- Lines 399-411: Decay calculation
- Lines 413-431: Gap check and blocking

**Flags**:
- `flagBuyGap`: Line 34 (default 5bps)
- `flagBuyDecay`: Line 35 (default 30s)
- `flagTarget`: Line 28 (default $5000)

## Next Steps

1. Add temporal clustering analysis to understand why time-since-sell is so low
2. Run backtest comparison: buygap={3,5,7,10} across multiple datasets
3. Consider implementing linear decay or removing decay entirely
4. Test volume-based decay as alternative to time-based
