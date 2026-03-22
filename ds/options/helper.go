package options

func CompareStrikes(a, b *Strike) int {
	return a.Price.Cmp(b.Price)
}

func CompareOptionByStrike(a, b *Option) int {
	return a.Strike.Price.Cmp(b.Strike.Price)
}
