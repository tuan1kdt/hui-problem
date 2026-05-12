// Package ptko implements PTKO (Parallel TKO): a parallel variant of the one-phase
// utility-list-based top-k HUI algorithm. Parallelism uses two complementary techniques:
//
//  1. Speculative initial threshold: after building utility lists, the k-th largest
//     single-item SumIutils is used as the starting minUtility, avoiding the expensive
//     near-zero-threshold exploration that sequential TKO suffers at startup.
//
//  2. Top-level Fork/Join with atomic threshold: the root-level item list is partitioned
//     across Workers goroutines. Each goroutine explores its disjoint subtree sequentially
//     (depth-cutoff = 0). The shared minUtility is an atomic int64 raised via CAS; the
//     top-k heap is protected by a mutex. Utility-list construction inside each subtree
//     creates new lists (no mutation of shared lists), so goroutines share only read-only
//     structures plus the two synchronization primitives.
//
// See report/main.tex Section 5.3 for the formal description and correctness argument.
package ptko

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hui-problem/internal/pkg/memory"
)

// PTKO holds all algorithm state.
type PTKO struct {
	k       int
	Workers int

	// atomicMinUtil is the shared pruning threshold, raised monotonically via CAS.
	// Goroutines read it on every pruning check without holding any lock.
	atomicMinUtil atomic.Int64

	// mu protects kHeap. It is acquired only in writeOut, never during recursion.
	mu    sync.Mutex
	kHeap itemsetMinHeap

	mapItemToTWU map[int]int
	totalTime    float64
}

// NewPTKO creates a new miner with Workers defaulting to NumCPU.
func NewPTKO() *PTKO {
	return &PTKO{
		Workers:      runtime.NumCPU(),
		mapItemToTWU: make(map[int]int),
	}
}

// RunAlgorithm executes the full PTKO pipeline and writes top-k results to outputPath.
//
// Step 1: firstPass  — compute TWU[item] with one DB scan.
// Step 2: buildUtilityListShells — create empty ULs sorted by TWU ascending.
// Step 3: secondPass — fill ULs with (tid, iutils, rutils) tuples.
// Step 4: bootstrapMinUtil — raise threshold to k-th single-item SumIutils.
// Step 5: parallelSearch — Fork/Join at top level; sequential DFS per goroutine.
func (a *PTKO) RunAlgorithm(inputPath, outputPath string, k int) error {
	memory.Reset()
	memory.Sample()

	start := time.Now()
	a.k = k
	a.kHeap = nil
	a.mapItemToTWU = make(map[int]int)
	a.atomicMinUtil.Store(1)

	if a.Workers < 1 {
		a.Workers = 1
	}

	if err := a.firstPass(inputPath); err != nil {
		return err
	}

	listItems, mapItemToUL, err := a.buildUtilityListShells()
	if err != nil {
		return err
	}

	if err := a.secondPass(inputPath, mapItemToUL); err != nil {
		return err
	}

	memory.Sample()

	a.bootstrapMinUtil(listItems)

	if err := a.parallelSearch(listItems); err != nil {
		return err
	}

	memory.Sample()
	a.totalTime = time.Since(start).Seconds()

	if err := a.writeResultToFile(outputPath); err != nil {
		return err
	}
	memory.Sample()
	return nil
}

// firstPass computes TWU[item] = sum of transaction utilities over all transactions
// containing the item. Identical to TKO's first pass.
func (a *PTKO) firstPass(inputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' || line[0] == '%' || line[0] == '@' {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		tu, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		for _, s := range strings.Fields(strings.TrimSpace(parts[0])) {
			item, err := strconv.Atoi(s)
			if err != nil {
				continue
			}
			a.mapItemToTWU[item] += tu
		}
	}
	return sc.Err()
}

// buildUtilityListShells creates empty UtilityLists and sorts them by TWU ascending.
// Ascending order ensures low-utility branches are explored first; combined with the
// speculative bootstrap, the threshold rises quickly and prunes deep high-TWU branches.
func (a *PTKO) buildUtilityListShells() ([]*UtilityList, map[int]*UtilityList, error) {
	listItems := make([]*UtilityList, 0, len(a.mapItemToTWU))
	mapItemToUL := make(map[int]*UtilityList, len(a.mapItemToTWU))
	for item := range a.mapItemToTWU {
		ul := NewUtilityList(item)
		listItems = append(listItems, ul)
		mapItemToUL[item] = ul
	}
	sort.Slice(listItems, func(i, j int) bool {
		twi := a.mapItemToTWU[listItems[i].Item]
		twj := a.mapItemToTWU[listItems[j].Item]
		if twi != twj {
			return twi < twj
		}
		return listItems[i].Item < listItems[j].Item
	})
	return listItems, mapItemToUL, nil
}

type pair struct {
	item    int
	utility int
}

// secondPass populates utility lists with (tid, iutils, rutils) tuples from a second DB scan.
func (a *PTKO) secondPass(inputPath string, mapItemToUL map[int]*UtilityList) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	tid := 0
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' || line[0] == '%' || line[0] == '@' {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		items := strings.Fields(strings.TrimSpace(parts[0]))
		utilityValues := strings.Fields(strings.TrimSpace(parts[2]))
		if len(items) != len(utilityValues) {
			continue
		}

		nTok := len(items)
		revised := make([]pair, 0, nTok)
		remainingUtility := 0
		for i := range items {
			item, _ := strconv.Atoi(items[i])
			u, _ := strconv.Atoi(utilityValues[i])
			revised = append(revised, pair{item: item, utility: u})
			remainingUtility += u
		}
		sort.Slice(revised, func(i, j int) bool {
			return a.compareItems(revised[i].item, revised[j].item) < 0
		})

		for _, p := range revised {
			remainingUtility -= p.utility
			ul := mapItemToUL[p.item]
			ul.AddElement(Element{tid, p.utility, remainingUtility})
		}
		tid++
	}
	return sc.Err()
}

func (a *PTKO) compareItems(item1, item2 int) int {
	c := a.mapItemToTWU[item1] - a.mapItemToTWU[item2]
	if c != 0 {
		return c
	}
	return item1 - item2
}

// bootstrapMinUtil raises atomicMinUtil to the k-th largest single-item SumIutils.
// This gives all goroutines a meaningful pruning threshold from the very first iteration,
// replacing the minUtil=1 cold-start that causes sequential TKO to over-explore.
func (a *PTKO) bootstrapMinUtil(listItems []*UtilityList) {
	if a.k <= 0 || len(listItems) < a.k {
		return
	}
	utils := make([]int64, len(listItems))
	for i, ul := range listItems {
		utils[i] = ul.SumIutils
	}
	sort.Slice(utils, func(i, j int) bool { return utils[i] > utils[j] })
	a.raiseMinUtil(utils[a.k-1])
}

// parallelSearch partitions listItems across Workers goroutines.
// Each goroutine handles items [from, to) as its top-level loop, building
// and recursing into extensions independently. Only atomicMinUtil (read/CAS)
// and kHeap (mutex) are shared.
func (a *PTKO) parallelSearch(listItems []*UtilityList) error {
	n := len(listItems)
	if n == 0 {
		return nil
	}

	w := a.Workers
	if w > n {
		w = n
	}

	chunk := (n + w - 1) / w
	errCh := make(chan error, w)
	var wg sync.WaitGroup

	for wi := 0; wi < w; wi++ {
		from := wi * chunk
		to := from + chunk
		if to > n {
			to = n
		}
		if from >= to {
			continue
		}
		wg.Add(1)
		go func(from, to int) {
			defer wg.Done()
			for i := from; i < to; i++ {
				X := listItems[i]
				minU := a.atomicMinUtil.Load()

				if X.SumIutils >= minU {
					a.writeOut(nil, X.Item, X.SumIutils)
				}

				// Upper-bound pruning: if even the best possible extension
				// can't beat the threshold, skip this subtree entirely.
				if X.SumRutils+X.SumIutils < a.atomicMinUtil.Load() {
					continue
				}

				// Build extensions: join X with every item j > i (items are
				// sorted ascending by TWU, so j always comes after i in order).
				exULs := make([]*UtilityList, 0, n-i-1)
				for j := i + 1; j < n; j++ {
					exULs = append(exULs, construct(nil, X, listItems[j]))
				}
				newPrefix := []int{X.Item}
				if err := a.search(newPrefix, X, exULs); err != nil {
					errCh <- err
					return
				}
			}
		}(from, to)
	}

	wg.Wait()
	close(errCh)
	return <-errCh
}

// search is the sequential DFS used within each goroutine's subtree.
// It is identical in structure to TKO's search but reads atomicMinUtil atomically
// and delegates all heap mutations to writeOut (which holds the mutex).
func (a *PTKO) search(prefix []int, pUL *UtilityList, uls []*UtilityList) error {
	memory.Sample()
	for i := 0; i < len(uls); i++ {
		X := uls[i]
		minU := a.atomicMinUtil.Load()

		if X.SumIutils >= minU {
			a.writeOut(prefix, X.Item, X.SumIutils)
		}
		if X.SumRutils+X.SumIutils < a.atomicMinUtil.Load() {
			continue
		}

		exULs := make([]*UtilityList, 0, len(uls)-i-1)
		for j := i + 1; j < len(uls); j++ {
			exULs = append(exULs, construct(pUL, X, uls[j]))
		}
		newPrefix := make([]int, len(prefix)+1)
		copy(newPrefix, prefix)
		newPrefix[len(prefix)] = X.Item
		if err := a.search(newPrefix, X, exULs); err != nil {
			return err
		}
	}
	return nil
}

// writeOut adds an itemset to the min-heap under mutex, trims to k, and raises
// atomicMinUtil via CAS when the heap fills. The CAS raise propagates the new threshold
// to all goroutines on their next pruning check without requiring them to hold any lock.
func (a *PTKO) writeOut(prefix []int, item int, utility int64) {
	is := newItemset(prefix, item, utility)

	a.mu.Lock()
	heap.Push(&a.kHeap, is)
	trimmed := false
	var newMin int64
	if a.kHeap.Len() > a.k {
		for a.kHeap.Len() > a.k {
			heap.Pop(&a.kHeap)
		}
		newMin = a.kHeap[0].Utility
		trimmed = true
	}
	a.mu.Unlock()

	if trimmed {
		a.raiseMinUtil(newMin)
	}
}

// raiseMinUtil raises atomicMinUtil to newVal using a CAS retry loop.
// Only raises (never lowers), so concurrent calls are safe and converge.
func (a *PTKO) raiseMinUtil(newVal int64) {
	for {
		old := a.atomicMinUtil.Load()
		if newVal <= old {
			return
		}
		if a.atomicMinUtil.CompareAndSwap(old, newVal) {
			return
		}
	}
}

// construct joins utility lists px and py to produce a new list for itemset {P, X, Y}.
// This function is pure: it reads px, py (and optionally P) but creates a fresh list.
// Multiple goroutines can call construct on the same shared px/py concurrently safely.
func construct(P, px, py *UtilityList) *UtilityList {
	pxyUL := NewUtilityList(py.Item)
	for _, ex := range px.Elements {
		ey := findElementWithTID(py, ex.TID)
		if ey == nil {
			continue
		}
		if P == nil {
			pxyUL.AddElement(Element{ex.TID, ex.Iutils + ey.Iutils, ey.Rutils})
		} else {
			e := findElementWithTID(P, ex.TID)
			if e != nil {
				pxyUL.AddElement(Element{ex.TID, ex.Iutils + ey.Iutils - e.Iutils, ey.Rutils})
			}
		}
	}
	return pxyUL
}

// findElementWithTID performs binary search on the sorted Elements slice.
// The returned pointer is valid as long as the slice is not grown (it never is after secondPass).
func findElementWithTID(ulist *UtilityList, tid int) *Element {
	list := ulist.Elements
	first, last := 0, len(list)-1
	for first <= last {
		middle := (first + last) >> 1
		m := list[middle].TID
		if m < tid {
			first = middle + 1
		} else if m > tid {
			last = middle - 1
		} else {
			return &list[middle]
		}
	}
	return nil
}

// writeResultToFile writes the top-k heap to a file in SPMF format.
func (a *PTKO) writeResultToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	n := a.kHeap.Len()
	items := make([]*Itemset, n)
	copy(items, a.kHeap)
	for i, it := range items {
		var b strings.Builder
		for j, p := range it.Prefix {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(p))
		}
		if len(it.Prefix) > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Itoa(it.Item))
		b.WriteString(" #UTIL: ")
		b.WriteString(strconv.FormatInt(it.Utility, 10))
		if _, err := w.WriteString(b.String()); err != nil {
			return err
		}
		if i < n-1 {
			if err := w.WriteByte('\n'); err != nil {
				return err
			}
		}
	}
	return nil
}

// PrintStats prints execution statistics in SPMF-compatible format.
func (a *PTKO) PrintStats() {
	fmt.Println("=============  PTKO (Go)  =============")
	fmt.Println(" High-utility itemsets count :", a.kHeap.Len())
	fmt.Printf(" Total time ~ %.3f s\n", a.totalTime)
	fmt.Printf(" Memory ~ %.2f MB (approx)\n", memory.MaxMB())
	fmt.Println(" Workers :", a.Workers)
	fmt.Println("===================================================")
}

// ResultSize returns the number of itemsets in the result heap.
func (a *PTKO) ResultSize() int { return a.kHeap.Len() }
