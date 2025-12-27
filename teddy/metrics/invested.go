package metrics

import "dropbear/decimal"

type Invested struct {
	min decimal.Decimal
	max decimal.Decimal
	avg decimal.Decimal
	cnt int
}

func NewInvested() *Invested {
	return &Invested{}
}

func (iv *Invested) Sample(d decimal.Decimal) {
	if iv.cnt == 0 {
		iv.min = d
		iv.max = d
	} else {
		iv.min = iv.min.Min(d)
		iv.max = iv.max.Max(d)
	}
	iv.cnt++
	iv.avg = iv.avg.Add(d.Sub(iv.avg).Div(decimal.FromInt(iv.cnt)))
}

func (iv *Invested) Min() decimal.Decimal {
	return iv.min
}

func (iv *Invested) Max() decimal.Decimal {
	return iv.max
}

func (iv *Invested) Avg() decimal.Decimal {
	return iv.avg
}
