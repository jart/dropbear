# decimal

A fixed-point decimal library optimized for financial calculations.

## Design

`Decimal` let's you encode real numbers with up to eight decimal places.
It's an alias for `int64`. The value `1.00` is stored internally as `100_000_000`.
The max number is `+92'233'720'368.54775807` and the min is `-92'233'720'368.54775808`.
For safety, `Decimal` does overflow checking, rather than paying the cost of growing data.
We use Bankers' Rounding for parsing and operations like division, to avoid statistical bias.

## Why not float64?

Floating-point numbers have rounding errors that accumulate in financial
calculations:

```go
0.1 + 0.2 == 0.30000000000000004  // float64
0.1 + 0.2 == 0.3                  // Decimal
```

That prevents your calculations from scaling, unless you use algorithms
like Kahan summation. But in every day scenarios, you're more likely to
run into logic errors with floating point, due to how inexactness hurts
operators like equality unless special care is taken.

## Why not big.Int or big.Rat?

Performance. In benchmarks, this implementation is significantly faster:

| Operation | int64 Decimal | big.Int | Slowdown |
|-----------|---------------|---------|----------|
| Parse     | 16ns          | 266ns   | 16x      |
| Add       | 2.1ns         | 18.5ns  | 10x      |
| Mul       | 4.7ns         | 47ns    | 10x      |
| Div       | 5.2ns         | 50ns    | 10x      |
| MulInt    | 2ns           | 50ns    | 25x      |
| DivInt    | 2ns           | 50ns    | 25x      |
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
