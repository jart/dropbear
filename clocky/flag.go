package clocky

import "flag"

// durationValue wraps Duration to implement flag.Value interface.
type durationValue Duration

func (d *durationValue) Set(s string) error {
	v, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = durationValue(v)
	return nil
}

func (d *durationValue) Get() any {
	return Duration(*d)
}

func (d *durationValue) String() string {
	return Duration(*d).String()
}

// DurationVar defines a Duration flag with specified name, default value, and usage string.
// The argument p points to a Duration variable in which to store the value of the flag.
func DurationVar(p *Duration, name string, value Duration, usage string) {
	*p = value
	flag.Var((*durationValue)(p), name, usage)
}

// Duration defines a Duration flag with specified name, default value, and usage string.
// The return value is the address of a Duration variable that stores the value of the flag.
func DurationFlag(name string, value string, usage string) *Duration {
	p := new(Duration)
	DurationVar(p, name, MustParseDuration(value), usage)
	return p
}

// timeValue wraps Duration to implement flag.Value interface.
type timeValue Time

func (d *timeValue) Set(s string) error {
	v, err := ParseTime(s)
	if err != nil {
		return err
	}
	*d = timeValue(v)
	return nil
}

func (d *timeValue) Get() any {
	return Time(*d)
}

func (d *timeValue) String() string {
	return Time(*d).String()
}

// TimeVar defines a Time flag with specified name, default value, and usage string.
// The argument p points to a Time variable in which to store the value of the flag.
func TimeVar(p *Time, name string, value Time, usage string) {
	*p = value
	flag.Var((*timeValue)(p), name, usage)
}

// Time defines a Time flag with specified name, default value, and usage string.
// The return value is the address of a Time variable that stores the value of the flag.
func TimeFlag(name string, value string, usage string) *Time {
	p := new(Time)
	TimeVar(p, name, MustParseTime(value), usage)
	return p
}
