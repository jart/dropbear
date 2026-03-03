package databento

import (
	"fmt"
	"strconv"
	"strings"

	"dropbear/clocky"
	"dropbear/decimal"
)

func appendName(b *strings.Builder, name string) {
	b.WriteByte('\t')
	b.WriteString(name)
	b.WriteString(": ")
}

func appendTimestamp(b *strings.Builder, t clocky.Time) {
	if uint64(t) == UndefTimestamp {
		b.WriteString("clocky.Time(UndefTimestamp),\n")
	} else {
		b.WriteString(t.GoString())
		b.WriteString(",\n")
	}
}

func appendPrice(b *strings.Builder, p int64) {
	if p == UndefPrice {
		b.WriteString("UndefPrice,\n")
		return
	}
	b.WriteString(strconv.FormatInt(p, 10))
	b.WriteString(", // ")
	b.WriteString(formatPrice(p))
	b.WriteByte('\n')
}

func appendPublisher(b *strings.Builder, p Publisher) {
	b.WriteString(p.GoString())
	b.WriteString(",\n")
}

func formatPrice(p int64) string {
	return decimal.Decimal(p / 1000).String()
}

func appendInt32(b *strings.Builder, v int32) {
	if v == UndefInt32 {
		b.WriteString("UndefInt32,\n")
	} else {
		b.WriteString(strconv.FormatInt(int64(v), 10))
		b.WriteString(",\n")
	}
}

func appendUint32(b *strings.Builder, v uint32) {
	if v == UndefUint32 {
		b.WriteString("UndefUint32,\n")
	} else {
		b.WriteString(strconv.FormatUint(uint64(v), 10))
		b.WriteString(",\n")
	}
}

func appendByteString(b *strings.Builder, data []byte) {
	b.WriteString(strconv.Quote(convertBytesToString(data)))
	b.WriteString(",\n")
}

func appendField(b *strings.Builder, v any) {
	switch x := v.(type) {
	case uint8:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint16:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint32:
		b.WriteString(strconv.FormatUint(uint64(x), 10))
	case uint64:
		b.WriteString(strconv.FormatUint(x, 10))
	case int8:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int16:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(x), 10))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case string:
		b.WriteString(strconv.Quote(x))
	case []string:
		b.WriteString("[]string{")
		for i, s := range x {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(strconv.Quote(s))
		}
		b.WriteByte('}')
	case fmt.GoStringer:
		b.WriteString(x.GoString())
	default:
		fmt.Fprintf(b, "%#v", v)
	}
	b.WriteString(",\n")
}
