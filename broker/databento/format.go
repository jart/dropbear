package databento

import (
	"fmt"
	"go/format"
	"strconv"
	"strings"

	"dropbear/clocky"
)

// PrettyPrint formats a GoStringer's output using go/format for column alignment.
func PrettyPrint(v fmt.GoStringer) string {
	s := v.GoString()
	src := "package p\nvar _ = " + s + "\n"
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return s
	}
	result := string(formatted)
	const prefix = "package p\n\nvar _ = "
	result = strings.TrimPrefix(result, prefix)
	result = strings.TrimSuffix(result, "\n")
	return result
}

func appendName(b *strings.Builder, name string) {
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
	whole := p / FixedPriceScale
	frac := p % FixedPriceScale
	if frac < 0 {
		frac = -frac
	}
	var buf [9]byte
	for i := 8; i >= 0; i-- {
		buf[i] = byte(frac%10) + '0'
		frac /= 10
	}
	end := 9
	for end > 1 && buf[end-1] == '0' {
		end--
	}
	return strconv.FormatInt(whole, 10) + "." + string(buf[:end])
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
