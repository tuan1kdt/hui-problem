// Package memory tracks approximate peak heap use via runtime.MemStats.Alloc.
package memory

import (
	"runtime"
	"sync"
)

var (
	mu          sync.Mutex
	maxMemoryMB float64
)

// Reset sets the recorded peak to 0. Call at the start of a timed run so Sample/MaxMB refer to that run only.
func Reset() {
	mu.Lock()
	maxMemoryMB = 0
	mu.Unlock()
}

// Sample reads current heap allocation (MB) and raises the package peak if higher.
func Sample() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	used := float64(ms.Alloc) / (1024 * 1024)
	mu.Lock()
	if used > maxMemoryMB {
		maxMemoryMB = used
	}
	mu.Unlock()
}

// MaxMB returns the peak megabytes recorded since the last Reset.
func MaxMB() float64 {
	mu.Lock()
	v := maxMemoryMB
	mu.Unlock()
	return v
}
