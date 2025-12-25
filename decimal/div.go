package decimal

// Div divides two decimals with rounding to nearest.
func (d Decimal) Div(o Decimal) Decimal {
	// we want (d * scale) / o, but d * scale can overflow.
	// use the identity: (d * scale) / o = (d / o) * scale + ((d % o) * scale) / o
	// but (d % o) * scale can also overflow, so we compute it iteratively.
	dv, ov := int64(d), int64(o)

	// for very large divisors where our iterative approach would overflow,
	// fall back to float64. This happens when |ov| > scale^2 / |dv|.
	// simplified: if |ov| > 10^17, use float64.
	const threshold = 100_000_000_000_000_000 // 10^17
	if ov > threshold || ov < -threshold {
		f := float64(dv) / float64(ov) * Scale
		if f >= 0 {
			return Decimal(int64(f + .5))
		}
		return Decimal(int64(f - .5))
	}

	quot := dv / ov * Scale
	rem := dv % ov

	// now we need rem * scale / ov without overflow.
	// do this by multiplying rem by 10 repeatedly and accumulating.
	// stop early if rem * 10 would overflow int64.
	const maxSafe = 9223372036854775807 / 10 // max int64 / 10
	for i := 0; i < Places && rem != 0; i++ {
		if rem > maxSafe || rem < -maxSafe {
			break // would overflow, remaining terms are negligible
		}
		rem *= 10
		quot += rem / ov * pow10[Places-1-i]
		rem = rem % ov
	}

	// round to nearest: if |rem * 2| >= |ov|, round away from zero
	if rem != 0 {
		// Compare |rem * 2| with |ov|
		absRem := rem
		if absRem < 0 {
			absRem = -absRem
		}
		absOv := ov
		if absOv < 0 {
			absOv = -absOv
		}
		// check if 2*|rem| >= |ov| (avoiding overflow by comparing absRem >= absOv/2)
		// for odd divisors, we round up when rem > ov/2, i.e., 2*rem > ov, i.e., 2*rem >= ov+1
		// simplify: round if 2*absRem >= absOv
		if absRem*2 >= absOv {
			// Round away from zero
			if (dv >= 0) == (ov >= 0) {
				quot++ // positive result, round up
			} else {
				quot-- // negative result, round down (more negative)
			}
		}
	}

	return Decimal(quot)
}
