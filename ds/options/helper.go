package options

func CompareOptionByStrike(a, b *Option) int {
	return a.Strike.Cmp(b.Strike)
}

func CompareStrikes(a, b *Strike) int {
	return a.Strike().Cmp(b.Strike())
}
