package symbol

import "flag"

func Flag(name string, value string, usage string) *Symbol {
	p := new(Symbol)
	if value != "" {
		*p = MustParse(value)
	}
	flag.Var((*symbolValue)(p), name, usage)
	return p
}

type symbolValue Symbol

func (d *symbolValue) Set(s string) error {
	if s == "" {
		*d = 0
		return nil
	}
	v, e := Parse(s)
	if e != nil {
		return e
	}
	*d = symbolValue(v)
	return nil
}

func (d *symbolValue) Get() any {
	return Symbol(*d)
}

func (d *symbolValue) String() string {
	return Symbol(*d).String()
}
