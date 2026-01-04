package clocky

import "fmt"

type YearMonth int

var ErrInvalidYearMonth = fmt.Errorf("invalid year-month format, expected YYYY-MM")

func MustParseYearMonth(ym string) YearMonth {
	yearMonth, err := ParseYearMonth(ym)
	if err != nil {
		panic(err)
	}
	return yearMonth
}

func ParseYearMonth(ym string) (YearMonth, error) {
	if len(ym) != 7 || ym[4] != '-' || !isDigit(ym[0]) || !isDigit(ym[1]) || !isDigit(ym[2]) || !isDigit(ym[3]) || !isDigit(ym[5]) || !isDigit(ym[6]) {
		return 0, ErrInvalidYearMonth
	}
	year := (int(ym[0]-'0') * 1000) + (int(ym[1]-'0') * 100) + (int(ym[2]-'0') * 10) + int(ym[3]-'0')
	month := (int(ym[5]-'0') * 10) + int(ym[6]-'0')
	if month < 1 || month > 12 || year < 1500 {
		return 0, ErrInvalidYearMonth
	}
	return YearMonth(year*100 + month), nil
}

func (ym YearMonth) String() string {
	var buf [7]byte
	buf[0] = byte('0' + (ym/100000)%10)
	buf[1] = byte('0' + (ym/10000)%10)
	buf[2] = byte('0' + (ym/1000)%10)
	buf[3] = byte('0' + (ym/100)%10)
	buf[4] = '-'
	buf[5] = byte('0' + (ym/10)%10)
	buf[6] = byte('0' + (ym)%10)
	return string(buf[:])
}

func (ym YearMonth) Increment() YearMonth {
	year := int(ym) / 100
	month := int(ym) % 100
	month++
	if month > 12 {
		month = 1
		year++
	}
	return YearMonth(year*100 + month)
}

func (ym YearMonth) Year() int {
	return int(ym) / 100
}

func (ym YearMonth) Month() int {
	return int(ym) % 100
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
