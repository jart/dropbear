package decimal

import (
	"flag"
)

type decimalValue Decimal

func (d *decimalValue) Set(s string) error {
	v := Parse(s)
	*d = decimalValue(v)
	return nil
}

func (d *decimalValue) Get() any {
	return Decimal(*d)
}

func (d *decimalValue) String() string {
	return Decimal(*d).String()
}

func Var(p *Decimal, name string, value Decimal, usage string) {
	*p = value
	flag.Var((*decimalValue)(p), name, usage)
}

// Flag defines a Decimal flag with specified name, default value, and usage string.
// The return value is the address of a Decimal variable that stores the value of the flag.
func Flag(name string, value string, usage string) *Decimal {
	p := new(Decimal)
	Var(p, name, Parse(value), usage)
	return p
}

type bpsValue Decimal

func (d *bpsValue) Set(s string) error {
	v := ParseBPS(s)
	*d = bpsValue(v)
	return nil
}

func (d *bpsValue) Get() any {
	return Decimal(*d)
}

func (d *bpsValue) String() string {
	return Decimal(*d).BPS().String()
}

func VarBPS(p *Decimal, name string, value Decimal, usage string) {
	*p = value
	flag.Var((*bpsValue)(p), name, usage)
}

// FlagBPS defines a Decimal basis points flag.
func FlagBPS(name string, value string, usage string) *Decimal {
	p := new(Decimal)
	VarBPS(p, name, ParseBPS(value), usage)
	return p
}

type percentValue Decimal

func (d *percentValue) Set(s string) error {
	// User provides "10" for 10%, stored as 0.1
	*d = percentValue(Parse(s).DivInt(100))
	return nil
}

func (d *percentValue) Get() any {
	return Decimal(*d)
}

func (d *percentValue) String() string {
	return Decimal(*d).MulInt(100).String()
}

// FlagPercent defines a percentage flag. Default and user values are percentages
// (e.g., "10" for 10%), stored internally as decimal fractions (0.1).
func FlagPercent(name string, defaultPercent string, usage string) *Decimal {
	p := new(Decimal)
	*p = Parse(defaultPercent).DivInt(100)
	flag.Var((*percentValue)(p), name, usage)
	return p
}
