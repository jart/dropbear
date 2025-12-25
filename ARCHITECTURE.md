# Buy Gap Protection Architecture

## Problem Statement

Current buy gap protection has two issues:
1. **Decay function is ineffective** - Median time-since-sell is 0s, so exponential decay rarely helps
2. **No visibility into gap behavior** - Can't measure effectiveness in real trading

## Proposed Changes

### 1. Add Linear Decay Option (IMPLEMENT)

**Rationale**: Exponential decay `e^(-t/period)` decays slowly at first, then rapidly. For very short time scales (<1s typical), linear decay `max(0, 1-t/period)` is more predictable and easier to reason about.

**Implementation**:
- Add new flag: `-decay-mode={exponential,linear,none}`
- Default to `exponential` (backward compatible)
- Linear formula: `decayFactor = max(0, 1 - timeSinceSell/decayPeriod)`
- None: `decayFactor = 1` (full gap protection always)

**Code location**: `cmd/spread/main.go:399-411`

### 2. Add Gap Metrics Tracking (IMPLEMENT)

**Rationale**: Need runtime visibility into how gap protection performs.

**Implementation**:
- Track counters in global state:
  - `gapSignalsTotal` - total buy signals evaluated
  - `gapSignalsBlocked` - signals blocked by gap
  - `gapSignalsExecuted` - signals that passed gap check
- Log summary periodically (every 1000 signals or 5 minutes)
- Include in existing verbose logging

**Code location**:
- Counters: Add near `gHolding` declarations
- Increment: Lines 417-430
- Logging: Add periodic summary

### 3. Enhance Analyzer with Decay Comparison (IMPLEMENT)

**Rationale**: Need to empirically test which decay mode works best.

**Implementation**:
- Extend `cmd/buygap` to simulate all three decay modes simultaneously
- Compare block rates and gap distributions
- Report which mode allows more/fewer buys

### 4. Add Per-Symbol Gap Configuration (DEFER)

**Rationale**: DOGE needs different gap than BTC (lower volatility).

**Implementation**: NOT implementing in this phase - needs broader config system refactor. Document as future work.

## Architecture Decisions

### Decision 1: Backward Compatibility
**Choice**: Default to `exponential` decay mode, same as current behavior.
**Rationale**: Don't break existing trading behavior. Allow opt-in testing of new modes.

### Decision 2: Decay Mode Options
**Choices**:
- `exponential`: Current behavior `e^(-timeRatio*periodScale)`
- `linear`: New `max(0, 1-timeRatio)`
- `none`: No decay (constant gap protection)

**Rationale**:
- Exponential: Maintain backward compat
- Linear: Better for short time scales
- None: Simplest, may be best given signals cluster so tightly

### Decision 3: No Breaking Changes to Flags
**Choice**: Add new `-decay-mode` flag, keep existing `-decay` flag unchanged.
**Rationale**: Existing scripts and backtests continue to work.

## Implementation Plan

### Phase 1: Core Changes (THIS ITERATION)
1. Add `DecayMode` flag to `cmd/spread/main.go`
2. Refactor decay calculation into separate function
3. Implement linear and none modes
4. Add gap metrics tracking
5. Add periodic metrics logging

### Phase 2: Enhanced Analyzer (THIS ITERATION)
1. Extend `cmd/buygap` to test all decay modes
2. Add comparative analysis output
3. Run across multiple datasets

### Phase 3: Testing & Validation (THIS ITERATION)
1. Run `go test ./...`
2. Run analyzer on chaos/BTC with all three modes
3. Compare backtest results: `./scripts/test.sh spread -buygap 5 -decay-mode linear`
4. Document findings

### Phase 4: Future Work (NOT THIS ITERATION)
- Per-symbol gap configuration
- Volume-based decay
- Adaptive gap based on realized volatility

## Testing Strategy

### Unit Tests
- Test decay function with known inputs
- Verify linear decay reaches 0 at period
- Verify exponential matches current behavior
- Verify none mode always returns 1

### Integration Tests
- Run analyzer on existing datasets
- Verify backward compatibility (default exponential matches current)
- Compare metrics across decay modes

### Backtest Validation
- Run spread strategy with each decay mode
- Compare sharpe, profit, buy counts
- Ensure no regressions

## Success Criteria

1. ✅ All tests pass (`go test ./...`)
2. ✅ Backward compatible (default behavior unchanged)
3. ✅ New modes available via flag
4. ✅ Metrics tracking works
5. ✅ Analyzer enhanced with comparison
6. ✅ Documentation complete

## Files to Modify

1. `cmd/spread/main.go` - Add decay mode flag and refactor calculation
2. `cmd/buygap/main.go` - Add multi-mode comparison
3. `BUYGAP_ANALYSIS.md` - Update with implementation notes
4. New: `cmd/spread/buygap.go` - Extract gap logic for testability

## Risks & Mitigations

**Risk**: New decay modes perform worse than exponential
**Mitigation**: Default to exponential, only opt-in to new modes

**Risk**: Metrics tracking adds latency
**Mitigation**: Simple counter increments, minimal overhead

**Risk**: Breaking backward compatibility
**Mitigation**: Extensive testing, default to current behavior
