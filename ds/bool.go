package ds

import (
	"fmt"
	"sync/atomic"
)

// Bool is a boolean that can be atomically loaded and stored.
// Unlike Go's paternalistic sync/atomic.Bool this is literally representable.
type Bool int32

const (
	False Bool = 0
	True  Bool = 1
)

func MustParseBool(s string) Bool {
	b, err := ParseBool(s)
	if err != nil {
		panic(err)
	}
	return b
}

func ParseBool(s string) (Bool, error) {
	switch s {
	case "true", "True", "TRUE", "yes", "1":
		return True, nil
	case "false", "False", "FALSE", "no", "0":
		return False, nil
	default:
		return False, fmt.Errorf("invalid boolean string: %q", s)
	}
}

func (b *Bool) String() string {
	if b.Load() {
		return "true"
	}
	return "false"
}

func (b *Bool) GoString() string {
	if b.Load() {
		return "True"
	}
	return "False"
}

// Load atomically loads and returns the value.
func (b *Bool) Load() bool {
	return atomic.LoadInt32((*int32)(b)) != 0
}

// Store atomically stores v.
func (b *Bool) Store(v bool) {
	var i int32
	if v {
		i = 1
	}
	atomic.StoreInt32((*int32)(b), i)
}

// Swap atomically stores v and returns the previous value.
func (b *Bool) Swap(v bool) bool {
	var i int32
	if v {
		i = 1
	}
	return atomic.SwapInt32((*int32)(b), i) != 0
}

// CompareAndSwap atomically sets the value to new if the current value equals old.
// Returns whether the swap occurred.
func (b *Bool) CompareAndSwap(old, new bool) bool {
	var o, n int32
	if old {
		o = 1
	}
	if new {
		n = 1
	}
	return atomic.CompareAndSwapInt32((*int32)(b), o, n)
}
