# Context: project

Context: project
Title: Project Context
Parent: root

Content:
# Dropbear Development Standards

## Commands
```bash
go fmt ./...           # Format code
go test ./...          # Run tests
go test -bench=. ./... # Run benchmarks
# Never use `go build` - write tests instead
```

## Package Structure
| Package | Purpose |
|---------|---------|
| `teddy/` | QuantConnect-like trading framework |
| `cmd/spread/` | Main spread strategy (binance-coinbase arbitrage) |
| `indicators/` | Technical indicators (EMA, MinMax, Intensity) |
| `decimal/` | Fixed-point math (9 decimal places) |
| `ds/` | Core data structures (Tick, Book, Lots) |
| `orderbook/` | L2 order book with tree set |
| `exchange/{coinbase,binance}/` | Exchange client libraries |

## New Packages (Actor Architecture)
| Package | Purpose |
|---------|---------|
| `actor/` | Actor framework (mailbox, routing) |
| `msg/` | Message types (BinanceTick, SpreadSignal, etc.) |
| `actors/` | Strategy actors (ingest, indicator, spread, risk) |
| `iso/` | Categorical infrastructure (projections, laws) |

## Code Style
- Optimal time complexity for algorithms
- Avoid dependencies (ask before introducing)
- Never use IEEE floats for financial math - use `decimal/`
- Use `clocky.Now()` for mockable time
- Use `loggy.Fatalf()` for panic (raises SIGINT for cleanup)

## Testing Strategy
- Table-driven tests (see `decimal/decimal_test.go`)
- Deterministic random seeds for reproducibility
- Benchmarks for performance-critical code
- Law verification tests for categorical properties

## Havequick Reference
Patterns to port from `~/src/havequick/`:
- `platform/runtime/actor.zig` - Actor with compile-time handler discovery
- `platform/mailbox/mailbox.zig` - Ring buffer with overflow tracking
- `lib/uberparser/iso/groupoid.zig` - Projections and morphisms
- `lib/uberparser/iso/category.zig` - Law verification


