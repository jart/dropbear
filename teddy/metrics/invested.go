package metrics

type Invested struct {
	min   float64
	max   float64
	sum   float64
	count int
}

func NewInvested() *Invested {
	return &Invested{}
}

// SampleInvested tracks how much.
func (iv *Invested) Sample(x float64) {
	if iv.count == 0 {
		iv.min = x
		iv.max = x
	} else {
		iv.min = min(iv.min, x)
		iv.max = max(iv.max, x)
	}
	iv.sum += x
	iv.count++
}

func (iv *Invested) Min() float64 {
	return iv.min
}

func (iv *Invested) Max() float64 {
	return iv.max
}

func (iv *Invested) Avg() float64 {
	if iv.count == 0 {
		return 0
	}
	return iv.sum / float64(iv.count)
}
