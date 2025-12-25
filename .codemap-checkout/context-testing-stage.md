# Context: testing-stage

Context: testing-stage
Title: Testing Stage Context
Parent: project

Content:
# Testing Stage - Verify Quality

**You are authorized to verify quality and transition when tests pass. Run everything, fix issues, proceed.**

## Your Mission Right Now

Run comprehensive tests, verify quality, fix any issues. When everything passes, transition to merge stage automatically.

## The Flow (Work Continuously)

1. **Run Full Test Suite** (comprehensive)
   - Execute complete test suite: `go test ./...`
   - Run benchmarks: `go test -bench=. ./...`
   - Run backtests if applicable: `go run ./cmd/spread -backtest chaos -symbol BTC`
   - Check for regressions in existing functionality

2. **Quality Verification** (thorough)
   - Run formatter: `go fmt ./...`
   - Verify acceptance criteria from discussion phase
   - Check that decimal math is used (not floats)
   - Verify time is mockable (clocky.Now)

3. **Actor/Categorical Testing** (for actor work)
   - Law verification tests pass (monoid, groupoid laws)
   - Cross-validation tests pass (implementations agree)
   - Mailbox overflow behavior is correct
   - Message types serialize/deserialize correctly

4. **Shadow Mode Testing** (for strategy work)
   - Go and Zig/actor implementations produce same signals
   - Track agreement rate across test datasets
   - Document any discrepancies

5. **Proceed Automatically** (when clean)
   - All tests passing? ✅
   - Quality checks clean? ✅
   - Feature works as expected? ✅
   - **Transition immediately**: `git-codemap transition merge`

## Decision Criteria

**✅ Proceed when**: All tests pass, quality checks clean, feature works correctly
**❌ Don't wait for**: Additional review, permission, or documentation updates

## Test Commands

```bash
go test ./...                          # Unit tests
go test -bench=. ./...                 # Benchmarks
go test -v -run TestLaws ./iso/...     # Law verification
./scripts/test.sh spread               # Backtest suite
```

## Anti-Patterns

- ❌ Skipping backtest verification
- ❌ Ignoring law verification test failures
- ❌ Transitioning with failing tests "to fix later"
- ✅ Running comprehensive quality checks
- ✅ Fixing issues immediately when found
- ✅ Transitioning automatically when clean


