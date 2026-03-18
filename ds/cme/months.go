package cme

import "dropbear/clocky"

var Months = map[byte]clocky.Month{
	'F': clocky.January,
	'G': clocky.February,
	'H': clocky.March,
	'J': clocky.April,
	'K': clocky.May,
	'M': clocky.June,
	'N': clocky.July,
	'Q': clocky.August,
	'U': clocky.September,
	'V': clocky.October,
	'X': clocky.November,
	'Z': clocky.December,
}

var MonthCodes = map[clocky.Month]byte{
	clocky.January:   'F',
	clocky.February:  'G',
	clocky.March:     'H',
	clocky.April:     'J',
	clocky.May:       'K',
	clocky.June:      'M',
	clocky.July:      'N',
	clocky.August:    'Q',
	clocky.September: 'U',
	clocky.October:   'V',
	clocky.November:  'X',
	clocky.December:  'Z',
}
