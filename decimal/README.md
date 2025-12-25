# decimal

A fixed-point decimal library optimized for financial calculations.

## Design

`Decimal` is a 64-bit signed integer representing a number with 9 fixed
decimal places. The value `1.00` is stored internally as
`1_000_000_000`.

```go
type Decimal int64
```

## Why not float64?

Floating-point numbers have rounding errors that accumulate in financial
calculations:

```go
0.1 + 0.2 == 0.30000000000000004  // float64
0.1 + 0.2 == 0.3                  // Decimal
```

## Why not big.Int or big.Rat?

Performance. In benchmarks, this implementation is significantly faster:

| Operation | int64 Decimal | big.Int | Slowdown |
|-----------|---------------|---------|----------|
| Add       | 0.48ns        | 18.5ns  | 39x      |
| Mul       | 0.48ns        | 47ns    | 98x      |
| Parse     | 12ns          | 266ns   | 22x      |
| Div       | 16ns          | 50ns    | 3x       |
| String    | 29ns          | 199ns   | 7x       |

The big.Int version also allocates 1-3 heap objects per operation, while
int64 arithmetic has zero allocations. For a trading bot doing thousands
of calculations per second, this matters.

## Range and Precision

- **Precision**: 9 decimal places (1 billionth)
- **Range**: ±9,223,372,036 (approximately ±9.2 billion)
- **Smallest value**: 0.000000001

This is sufficient for:
- Cryptocurrency prices (BTC at $100k = 100000.000000000)
- Order quantities (0.00000001 BTC = 1 satoshi)
- Basis point calculations (0.0001 = 1 bps)

This is NOT sufficient for:
- US M2 money supply (~$21 trillion)
- National debt figures
- Values exceeding ~$9 billion

## Usage

```go
price := decimal.Parse("123.45")
quantity := decimal.Parse("0.5")
total := price.Mul(quantity)  // 61.725

// Quantize to exchange tick size
tick := decimal.Parse("0.01")
total = total.Quantize(tick)  // 61.72

// Convert to/from basis points
edge := decimal.FromBPS(5)    // 0.0005 (5 basis points)
bps := edge.BPS()             // 5
```

## Overflow

Multiplication and division use 128-bit intermediate results to avoid
overflow during calculation. However, the final result must fit in the
±9.2 billion range. Operations that would overflow will produce
incorrect results without warning.
