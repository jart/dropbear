package ds

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
)

var (
	physicalCoresOnce sync.Once
	physicalCoresSave int
)

// PhysicalCores returns the number of physical CPU cores available on this machine.
func PhysicalCores() int {
	physicalCoresOnce.Do(func() {
		// Linux: count unique thread sibling groups
		siblings := map[string]bool{}
		for cpu := 0; ; cpu++ {
			path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology/thread_siblings", cpu)
			data, err := os.ReadFile(path)
			if err != nil {
				break
			}
			siblings[strings.TrimSpace(string(data))] = true
		}
		count := len(siblings)
		// fallback
		if count < 1 {
			count = runtime.NumCPU() / 2
		}
		physicalCoresSave = max(count, 1)
	})
	return physicalCoresSave
}
