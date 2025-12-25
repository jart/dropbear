package main

type Disposition int

const (
	Cheap        = Disposition(-2)
	TooCheap     = Disposition(-1)
	Normal       = Disposition(0)
	Expensive    = Disposition(1)
	TooExpensive = Disposition(2)
)

func (d Disposition) String() string {
	switch d {
	case Normal:
		return "normal"
	case Cheap:
		return "cheap"
	case TooCheap:
		return "too cheap"
	case Expensive:
		return "expensive"
	case TooExpensive:
		return "too expensive"
	default:
		panic("unknown disposition")
	}
}
