package options

func CompareStrikes(a, b *Strike) int {
	return a.Price.Cmp(b.Price)
}
