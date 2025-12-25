package decimal

// String returns the decimal as a string with trailing zeros trimmed.
func (d Decimal) String() string {
	var b [32]byte
	i := len(b)
	v := int64(d)
	s := v < 0
	if s {
		v = -v
	}

	// write fractional digits right-to-left, trimming trailing zeros
	t := true
	for range Places {
		c := byte(v%10) + '0'
		v /= 10
		if t && c == '0' {
			continue
		}
		t = false
		i--
		b[i] = c
	}

	// write decimal point if we have fractional digits
	if !t {
		i--
		b[i] = '.'
	}

	// write integer digits right-to-left
	if v == 0 {
		i--
		b[i] = '0'
	} else {
		for v > 0 {
			i--
			b[i] = byte(v%10) + '0'
			v /= 10
		}
	}

	// write sign
	if s {
		i--
		b[i] = '-'
	}

	return string(b[i:])
}
