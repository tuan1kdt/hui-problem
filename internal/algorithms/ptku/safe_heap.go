package ptku

import (
	"sync"
)

// SafeHeap is the shared mutable top-k border (minUtil) used by NU, MD, MC, etc.
// It mirrors TKU's updateNodeCountHeap behavior: each logical "strategy" keeps its own
// RedBlackTree[int] multiset of up to k utility bounds; when full, the tree minimum
// can lift the global minUtil. Concurrent miners call TryUpdateWithHeap with distinct heaps.
//
// Synchronization follows the paper's double-check pattern: RLock + compare, then Lock + recheck
// to avoid serializing obviously useless updates.
type SafeHeap struct {
	mu      sync.RWMutex
	k       int
	minUtil float64
}

// NewSafeHeap creates the border; minUtil starts at initialMinUtil (often 0 before PE raises it).
func NewSafeHeap(k int, initialMinUtil float64) *SafeHeap {
	return &SafeHeap{k: k, minUtil: initialMinUtil}
}

// MinUtil returns the current minimum utility threshold (read lock). Pruning compares against this.
func (s *SafeHeap) MinUtil() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.minUtil
}

// SetMinUtil raises the border if v is higher (e.g. after merging PE k-th largest into the border).
func (s *SafeHeap) SetMinUtil(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v > s.minUtil {
		s.minUtil = v
	}
}

// TryUpdateWithHeap applies one TKU-style heap update for this caller's multiset heap:
//
//  1. Fast path: if heap already has k elements and newValue ≤ minUtil, skip (double-check read).
//  2. Lock and re-check under write lock (another thread may have raised minUtil).
//  3. If heap size < k, push newValue; else if newValue > minUtil, push and pop smallest.
//  4. If heap size ≥ k, promote global minUtil to the multiset minimum when that lifts the border.
//
// heap must belong to a single goroutine (or be externally locked); only minUtil is shared.
func (s *SafeHeap) TryUpdateWithHeap(heap *RedBlackTree[int], newValue int) {
	s.mu.RLock()
	minU := s.minUtil
	k := s.k
	s.mu.RUnlock()

	if heap.Size() >= k && float64(newValue) <= minU {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if heap.Size() >= s.k && float64(newValue) <= s.minUtil {
		return
	}
	if heap.Size() < s.k {
		heap.Add(newValue)
	} else if heap.Size() >= s.k {
		if float64(newValue) > s.minUtil {
			heap.Add(newValue)
			heap.PopMinimum()
		}
	}
	if heap.Size() >= s.k {
		minV := heap.Minimum()
		if float64(minV) > s.minUtil {
			s.minUtil = float64(minV)
		}
	}
}
