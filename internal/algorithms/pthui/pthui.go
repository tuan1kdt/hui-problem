// Package thui implements a parallel version of the THUI (Top-k High Utility
// Itemsets) algorithm.
//
// Four parallelism points are introduced, each labelled [PARALLEL]:
//
//  1. [PARALLEL – MapReduce] firstScan: each worker reads a partition of
//     lines, builds a local TWU/RIU map, then a single reducer merges all
//     local maps into the shared ones.
//
//  2. [PARALLEL – MapReduce] secondScan: each worker reads a partition of
//     transactions and emits (leafIndex, suffixIndex, utility) triples into
//     a per-worker slice; a reducer merges them into mapLeafMAP and appends
//     Elements into each UtilityList under a per-list mutex.
//
//  3. [PARALLEL – Fork/Join] raisingThresholdLeaf: the outer loop over
//     mapLeafMAP entries is distributed across goroutines; each goroutine
//     collects candidate utilities into a local longHeap, which the main
//     goroutine merges at the end.
//
//  4. [PARALLEL – Fork/Join] thui: at the root call only, each item X in
//     the extension loop is forked into a separate goroutine; deeper
//     recursion stays serial to avoid goroutine explosion.
//
// Reference:
//   Srikumar Krishnamoorthy: "Mining top-k high utility itemsets with effective
//   threshold raising strategies." Expert Syst. Appl. 117: 148-165 (2019)
package pthui

import (
	"bufio"
	"bytes"
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
)

// ---------------------------------------------------------------------------
// Data structures
// ---------------------------------------------------------------------------

// Element represents one entry in a UtilityList.
// int32 fields save 4 bytes each vs int64; TIDs and utilities fit comfortably.
type Element struct {
	tid    int32
	iutils int32
	rutils int32
}

// UtilityList holds utility information for one item / itemset extension.

type UtilityList struct {
	mu        sync.Mutex // [PARALLEL] guards concurrent addElement calls in secondScan
	item      int
	sumIutils int64
	sumRutils int64
	elements  []Element
}

func newUtilityList(item, capacity int) *UtilityList {
	return &UtilityList{
		item:     item,
		elements: make([]Element, 0, capacity),
	}
}

func (ul *UtilityList) addElement(e Element) {
	ul.sumIutils += int64(e.iutils)
	ul.sumRutils += int64(e.rutils)
	ul.elements = append(ul.elements, e)
}

func (ul *UtilityList) getUtils() int64 { return ul.sumIutils }

// ItemTHUI stores TWU and combined utility for one item pair in mapFMAP (EUCS).
type ItemTHUI struct {
	twu     int64
	utility int64
}

// PatternTHUI stores the prefix as an int slice; string is built only at write time.
type PatternTHUI struct {
	prefixItems []int
	lastItem    int
	utility     int64
}

func (p *PatternTHUI) prefixString() string {
	var b strings.Builder
	for _, v := range p.prefixItems {
		b.WriteString(strconv.Itoa(v))
		b.WriteByte(' ')
	}
	b.WriteString(strconv.Itoa(p.lastItem))
	return b.String()
}

func (p *PatternTHUI) firstItem() int {
	if len(p.prefixItems) > 0 {
		return p.prefixItems[0]
	}
	return p.lastItem
}

// ---------------------------------------------------------------------------
// Priority queues
// ---------------------------------------------------------------------------

type patternHeap []PatternTHUI

func (h patternHeap) Len() int            { return len(h) }
func (h patternHeap) Less(i, j int) bool  { return h[i].utility < h[j].utility }
func (h patternHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *patternHeap) Push(x interface{}) { *h = append(*h, x.(PatternTHUI)) }
func (h *patternHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

type longHeap []int64

func (h longHeap) Len() int            { return len(h) }
func (h longHeap) Less(i, j int) bool  { return h[i] < h[j] }
func (h longHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *longHeap) Push(x interface{}) { *h = append(*h, x.(int64)) }
func (h *longHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// ---------------------------------------------------------------------------
// Pair helper
// ---------------------------------------------------------------------------

type pair struct {
	item    int
	utility int32
}

// =============================================================================
// leafEmit – used by parallel second scan to collect results before merge
// =============================================================================

// leafEmit carries one (endIdx, startIdx, cutil) triple emitted by a worker
// during the parallel leaf-map construction phase.
// [PARALLEL – MapReduce] Workers emit these; the reducer sums them into mapLeafMAP.
type leafEmit struct {
	endIdx   int
	startIdx int
	cutil    int64
}

// =============================================================================
// AlgoPTHUI
// =============================================================================

type AlgoPTHUI struct {
	MaxMemory      float64
	StartTimestamp int64
	EndTimestamp   int64
	HuiCount       int
	CandidateCount int // final value, copied from candidateCount64 after mining

	// candidateCount64 is the atomic counter incremented by parallel goroutines.
	// [PARALLEL] int64 aligned for atomic operations on 32-bit platforms.
	candidateCount64 int64

	// ---- internal state ----
	mapItemToTWU map[int]int64
	minUtility   int64
	topkstatic   int

	writer *bufio.Writer

	kPatterns      patternHeap
	kPatternsMu    sync.Mutex  // [PARALLEL] guards kPatterns / minUtility during parallel mining
	leafPruneUtils *longHeap

	itemsetBuffer []int

	mapFMAP    map[int]map[int]*ItemTHUI
	mapLeafMAP map[int]map[int]int64

	riuRaiseValue  int64
	leafRaiseValue int64
	leafMapSize    int

	eucsPrune bool
	leafPrune bool

	inputFile string

	itemIndex map[int]int

	//flat array for O(1) TWU lookup by UL position
	twuByIdx []int64

	//  reusable transaction buffer
	txBuf []pair

	memCheckCounter int

	// numWorkers controls the degree of parallelism for all four parallel phases.
	// Defaults to runtime.NumCPU().
	numWorkers int
}

// NewAlgoPTHUI returns a ready-to-use instance with parallelism = NumCPU.
func NewAlgoPTHUI() *AlgoPTHUI {
	return &AlgoPTHUI{
		leafPrune: true,
		txBuf:     make([]pair, 0, 256),
		numWorkers: runtime.NumCPU(),
	}
}

// SetWorkers overrides the number of parallel workers (useful for testing).
func (a *AlgoPTHUI) SetWorkers(n int) {
	if n > 0 {
		a.numWorkers = n
	}
}

// RunAlgorithm is the main entry point.
// Parallel phases:
//  1. [PARALLEL – MapReduce] firstScan
//  2. [PARALLEL – MapReduce] secondScan
//  3. [PARALLEL – Fork/Join] raisingThresholdLeaf
//  4. [PARALLEL – Fork/Join] thui (root level only)
func (a *AlgoPTHUI) RunAlgorithm(input, output string, eucsPrune bool, topk int) error {
	a.topkstatic = topk
	a.MaxMemory = 0
	a.itemsetBuffer = make([]int, 200)
	a.eucsPrune = eucsPrune
	a.inputFile = input

	riu := make(map[int]int64)

	if a.eucsPrune {
		a.mapFMAP = make(map[int]map[int]*ItemTHUI)
	}
	if a.leafPrune {
		a.mapLeafMAP = make(map[int]map[int]int64)
		lph := longHeap(nil)
		a.leafPruneUtils = &lph
		heap.Init(a.leafPruneUtils)
	}

	a.StartTimestamp = time.Now().UnixMilli()

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer f.Close()
	a.writer = bufio.NewWriterSize(f, 1<<20) // 1 MB write buffer

	a.mapItemToTWU = make(map[int]int64)

	// ---- Phase 1: parallel first scan [PARALLEL – MapReduce] ----
	// Workers process file partitions concurrently; reducer merges local maps.
	txCount, err := a.firstScanParallel(input, riu)
	if err != nil {
		return err
	}

	a.raisingThresholdRIU(riu, a.topkstatic)
	a.riuRaiseValue = a.minUtility

	// Build utility lists
	mapItemToUL := make(map[int]*UtilityList, len(a.mapItemToTWU))
	var listOfULs []*UtilityList

	for item, twu := range a.mapItemToTWU {
		if twu >= a.minUtility {
			cap := txCount/4 + 8 //  generous capacity hint
			ul := newUtilityList(item, cap)
			mapItemToUL[item] = ul
			listOfULs = append(listOfULs, ul)
		}
	}

	sort.Slice(listOfULs, func(i, j int) bool {
		return a.compareItemsDirect(listOfULs[i].item, listOfULs[j].item) < 0
	})

	// build itemIndex map
	a.itemIndex = make(map[int]int, len(listOfULs))
	for idx, ul := range listOfULs {
		a.itemIndex[ul.item] = idx
	}

	// build flat TWU array
	a.twuByIdx = make([]int64, len(listOfULs))
	for idx, ul := range listOfULs {
		a.twuByIdx[idx] = a.mapItemToTWU[ul.item]
	}

	// ---- Phase 2: parallel second scan [PARALLEL – MapReduce] ----
	// Workers populate UtilityLists and emit leaf/EUCS entries; reducers merge.
	if err = a.secondScanParallel(input, mapItemToUL, listOfULs); err != nil {
		return err
	}

	// After parallel population the element slices are unordered by tid because
	// different workers may have written non-contiguous transactions.
	// Sort each UtilityList's elements by tid so construct() works correctly.
	// [PARALLEL – Fork/Join] Each list is independent → sort in parallel.
	a.sortUtilityListsParallel(listOfULs)

	if a.eucsPrune {
		a.raisingThresholdCUDOptimize(a.topkstatic)
		a.removeEntry()
	}

	if a.leafPrune {
		// ---- Phase 3: parallel LIU-LB threshold raising [PARALLEL – Fork/Join] ----
		a.raisingThresholdLeafParallel(listOfULs)
		a.setLeafMapSize()
		a.removeLeafEntry()
		a.leafPruneUtils = nil
	}
	a.leafRaiseValue = a.minUtility
	mapItemToUL = nil

	a.checkMemoryForce()
	ph := patternHeap(nil)
	a.kPatterns = ph
	heap.Init(&a.kPatterns)

	// ---- Phase 4: parallel recursive mining [PARALLEL – Fork/Join] ----
	if err = a.thuiParallel(a.itemsetBuffer, listOfULs); err != nil {
		return err
	}
	a.checkMemoryForce()
	// Copy the atomic counter into the public CandidateCount field.
	a.CandidateCount = int(atomic.LoadInt64(&a.candidateCount64))

	if err = a.writeResultToFile(); err != nil {
		return err
	}
	a.writer.Flush()

	a.EndTimestamp = time.Now().UnixMilli()
	a.kPatterns = nil
	return nil
}

// parsedLine holds the three colon-separated sections of one transaction line.
type parsedLine struct {
	itemsRaw []byte
	tuRaw    []byte
	utilsRaw []byte
}

// parseLine splits a raw transaction line into its three ':'-delimited sections.
// Returns false if the line is malformed (missing one or both ':' separators).
func parseLine(line []byte) (parsedLine, bool) {
	col1 := bytes.IndexByte(line, ':')
	if col1 < 0 {
		return parsedLine{}, false
	}
	rest := line[col1+1:]
	col2 := bytes.IndexByte(rest, ':')
	if col2 < 0 {
		return parsedLine{}, false
	}
	return parsedLine{
		itemsRaw: line[:col1],
		tuRaw:    bytes.TrimSpace(rest[:col2]),
		utilsRaw: rest[col2+1:],
	}, true
}

// isCommentOrEmpty returns true for blank lines and lines starting with #, %, or @.
func isCommentOrEmpty(line []byte) bool {
	return len(line) == 0 || line[0] == '#' || line[0] == '%' || line[0] == '@'
}

// =============================================================================
// [PARALLEL – MapReduce] Phase 1: first scan
// =============================================================================
//
// Strategy:
//   Map  – read all lines into memory; divide into numWorkers chunks;
//          each goroutine scans its chunk and builds a local twuMap / riuMap.
//   Reduce – the main goroutine merges all local maps into a.mapItemToTWU / riu.
//
// This avoids lock contention on the shared maps during the map phase.

// firstScanParallel reads the entire file into memory, splits it into worker
// chunks, lets each worker accumulate local TWU/RIU maps, then merges them.
func (a *AlgoPTHUI) firstScanParallel(input string, riu map[int]int64) (int, error) {
	// Read all lines up front so we can partition them freely.
	lines, err := readAllLines(input)
	if err != nil {
		return 0, err
	}

	type localResult struct {
		twu map[int]int64
		riu map[int]int64
		cnt int // number of valid transactions in this partition
	}

	nw := a.numWorkers
	chunkSize := (len(lines) + nw - 1) / nw

	results := make([]localResult, nw)

	// [PARALLEL – MapReduce MAP] Each goroutine processes its own line partition.
	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		w := w
		start := w * chunkSize
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			localTWU := make(map[int]int64)
			localRIU := make(map[int]int64)
			cnt := 0
			for _, line := range lines[start:end] {
				if isCommentOrEmpty(line) {
					continue
				}
				pl, ok := parseLine(line)
				if !ok {
			continue // skip malformed lines gracefully
				}
				cnt++
				tu := parseInt(pl.tuRaw)
				ii := newByteFieldIter(pl.itemsRaw)
				ui := newByteFieldIter(pl.utilsRaw)
				for {
					it, ok1 := ii.next()
					ut, ok2 := ui.next()
					if !ok1 || !ok2 {
						break
					}
					item := int(parseInt(it))
					localTWU[item] += tu
					localRIU[item] += parseInt(ut)
				}
			}
			results[w] = localResult{localTWU, localRIU, cnt}
		}()
	}
	wg.Wait()

	// [PARALLEL – MapReduce REDUCE] Merge all local maps into shared maps (serial).
	txCount := 0
	for _, r := range results {
		txCount += r.cnt
		for item, v := range r.twu {
			a.mapItemToTWU[item] += v
		}
		for item, v := range r.riu {
			riu[item] += v
		}
	}
	return txCount, nil
}

// =============================================================================
// [PARALLEL – MapReduce] Phase 2: second scan
// =============================================================================
//
// Strategy:
//   Map  – each goroutine processes its line partition:
//            • appends Elements to UtilityLists (protected by per-list mutex)
//            • accumulates EUCS updates into a local mapFMAP copy
//            • emits leafEmit triples into a local slice
//   Reduce – main goroutine merges local EUCS maps and folds leafEmit slices
//            into a.mapLeafMAP.

// secondScanParallel populates UtilityLists, mapFMAP, and mapLeafMAP in parallel.
func (a *AlgoPTHUI) secondScanParallel(input string, mapItemToUL map[int]*UtilityList, listOfULs []*UtilityList) error {
	lines, err := readAllLines(input)
	if err != nil {
		return err
	}

	nw := a.numWorkers
	chunkSize := (len(lines) + nw - 1) / nw

	type workerResult struct {
		localFMAP   map[int]map[int]*ItemTHUI // local EUCS accumulator
		leafEmits   []leafEmit                 // raw leaf contributions
		candidateN  int64                      // not used here but tracked for future
	}

	results := make([]workerResult, nw)

	// [PARALLEL – MapReduce MAP] Workers process their line partitions.
	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		w := w
		start := w * chunkSize
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			localFMAP := make(map[int]map[int]*ItemTHUI)
			var localLeafEmits []leafEmit
			tx := make([]pair, 0, 64)

			// Each goroutine needs its own tid counter for its partition.
			// We compute the tid for the first line of this partition by
			// counting valid lines before it.
			tid := countValidLines(lines[:start])

			for _, line := range lines[start:end] {
				if isCommentOrEmpty(line) {
					continue
				}
				pl, ok := parseLine(line)
				if !ok {
					continue // skip malformed lines gracefully
				}

				tx = tx[:0]
				var newTWU int64

				ii := newByteFieldIter(pl.itemsRaw)
				ui := newByteFieldIter(pl.utilsRaw)
				for {
					it, ok1 := ii.next()
					ut, ok2 := ui.next()
					if !ok1 || !ok2 {
						break
					}
					item := int(parseInt(it))
					util := int32(parseInt(ut))
					if a.mapItemToTWU[item] >= a.minUtility {
						tx = append(tx, pair{item, util})
						newTWU += int64(util)
					}
				}
				if len(tx) == 0 {
					tid++
					continue
				}

		// Sort using itemIndex for O(1) comparisons 
				sort.Slice(tx, func(i, j int) bool {
					return a.itemIndex[tx[i].item] < a.itemIndex[tx[j].item]
				})

				remainingUtility := int32(0)
				for i := len(tx) - 1; i >= 0; i-- {
					p := tx[i]

					// Append Element to the shared UtilityList under its per-list lock.
					// [PARALLEL] Each UtilityList has its own mutex so different items
					// never block each other; only concurrent appends to the same item's
					// list serialize here.
					ul := mapItemToUL[p.item]
					ul.mu.Lock()
					ul.addElement(Element{int32(tid), p.utility, remainingUtility})
					ul.mu.Unlock()

					if a.eucsPrune {
						// Accumulate into the worker-local EUCS map (no lock needed).
						workerUpdateEUCS(localFMAP, i, p, tx, newTWU)
					}
					if a.leafPrune {
						// Emit leaf triples into the worker-local slice (no lock needed).
						localLeafEmits = workerEmitLeaf(localLeafEmits, i, p, tx, a.itemIndex)
					}
					remainingUtility += p.utility
				}
				tid++
			}
			results[w] = workerResult{localFMAP, localLeafEmits, 0}
		}()
	}
	wg.Wait()

	// [PARALLEL – MapReduce REDUCE] Merge worker results into shared structures (serial).

	// Merge EUCS maps.
	if a.eucsPrune {
		for _, r := range results {
			for item, inner := range r.localFMAP {
				dst := a.mapFMAP[item]
				if dst == nil {
					dst = make(map[int]*ItemTHUI)
					a.mapFMAP[item] = dst
				}
				for other, v := range inner {
					d := dst[other]
					if d == nil {
						d = &ItemTHUI{}
						dst[other] = d
					}
					d.twu += v.twu
					d.utility += v.utility
				}
			}
		}
	}

	// Merge leaf emits into mapLeafMAP.
	if a.leafPrune {
		for _, r := range results {
			for _, e := range r.leafEmits {
				inner := a.mapLeafMAP[e.endIdx]
				if inner == nil {
					inner = make(map[int]int64)
					a.mapLeafMAP[e.endIdx] = inner
				}
				inner[e.startIdx] += e.cutil
			}
		}
	}

	return nil
}
// compareItemsDirect compares two items by their TWU values (ascending order).
func (a *AlgoPTHUI) compareItemsDirect(item1, item2 int) int {
	d := a.mapItemToTWU[item1] - a.mapItemToTWU[item2]
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return item1 - item2
}


// workerUpdateEUCS is the goroutine-local version of updateEUCSprune.
// It writes into the worker's own localFMAP with no locking required.
func workerUpdateEUCS(localFMAP map[int]map[int]*ItemTHUI, i int, p pair, tx []pair, newTWU int64) {
	inner := localFMAP[p.item]
	if inner == nil {
		inner = make(map[int]*ItemTHUI, len(tx)-i)
		localFMAP[p.item] = inner
	}
	for j := i + 1; j < len(tx); j++ {
		after := tx[j]
		if p.item == after.item {
			continue
		}
		ti := inner[after.item]
		if ti == nil {
			ti = &ItemTHUI{}
			inner[after.item] = ti
		}
		ti.twu += newTWU
		ti.utility += int64(p.utility) + int64(after.utility)
	}
}

// workerEmitLeaf is the goroutine-local version of updateLeafprune.
// Instead of writing to a shared map it appends leafEmit triples to the
// worker's own local slice; the reducer sums them later.
func workerEmitLeaf(out []leafEmit, i int, p pair, tx []pair, itemIndex map[int]int) []leafEmit {
	cutil := int64(p.utility)
	followingItemIdx := itemIndex[p.item]
	for j := i - 1; j >= 0; j-- {
		after := tx[j]
		if p.item == after.item {
			continue
		}
		followingItemIdx--
		if followingItemIdx < 0 || itemIndex[after.item] != followingItemIdx {
			break
		}
		cutil += int64(after.utility)
		out = append(out, leafEmit{
			endIdx:   itemIndex[p.item],
			startIdx: followingItemIdx,
			cutil:    cutil,
		})
	}
	return out
}

// sortUtilityListsParallel sorts each UtilityList's elements by tid.
// After the parallel second scan, elements may be out of tid order because
// different workers appended non-contiguous transactions.
// [PARALLEL – Fork/Join] Each list is independent; sort all lists concurrently.
func (a *AlgoPTHUI) sortUtilityListsParallel(listOfULs []*UtilityList) {
	var wg sync.WaitGroup
	// Use a semaphore channel to cap active goroutines at numWorkers.
	sem := make(chan struct{}, a.numWorkers)
	for _, ul := range listOfULs {
		ul := ul
		if len(ul.elements) < 2 {
			continue // already sorted trivially
		}
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			// [PARALLEL – Fork/Join] Sort this list's elements by tid.
			sort.Slice(ul.elements, func(i, j int) bool {
				return ul.elements[i].tid < ul.elements[j].tid
			})
		}()
	}
	wg.Wait()
}
func (a *AlgoPTHUI) setLeafMapSize() {
	for _, v := range a.mapLeafMAP {
		a.leafMapSize += len(v)
	}
}


// =============================================================================
// [PARALLEL – Fork/Join] Phase 4: recursive mining
// =============================================================================
//
// Strategy (root-level only):
//   At depth 0, each item X in the extension loop is treated as an independent
//   sub-tree and forked into its own goroutine. Deeper recursion stays serial
//   to prevent goroutine explosion and excessive memory growth.
//
//   Because multiple goroutines call save() concurrently, kPatterns and
//   minUtility are protected by kPatternsMu. CandidateCount is updated with
//   atomic add.

// thuiParallel forks the top-level extension loop across goroutines.
func (a *AlgoPTHUI) thuiParallel(prefix []int, ULs []*UtilityList) error {
	// Step 1 – save root-level single-item HUIs (serial; fast).
	for i := len(ULs) - 1; i >= 0; i-- {
		if ULs[i].getUtils() >= a.minUtility {
			a.saveSafe(nil, 0, ULs[i]) // [PARALLEL] uses kPatternsMu
		}
	}

	// Collect all items that pass the upper-bound check so we can fork them.
	type task struct {
		X   *UtilityList
		idx int // position of X in ULs
	}
	var tasks []task
	for i := len(ULs) - 2; i >= 0; i-- {
		X := ULs[i]
		if X.sumIutils+X.sumRutils >= a.minUtility && X.sumIutils > 0 {
			if a.eucsPrune {
				if _, ok := a.mapFMAP[X.item]; !ok {
					continue
				}
			}
			tasks = append(tasks, task{X, i})
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(tasks))
	// Semaphore limits active goroutines so we don't spawn thousands of threads.
	sem := make(chan struct{}, a.numWorkers)

	// [PARALLEL – Fork/Join FORK] Each top-level extension branch runs in its own goroutine.
	for _, t := range tasks {
		t := t
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			// Each goroutine builds the extension UL list for its item X.
			// We read ULs and a.minUtility without a lock here; a.minUtility
			// may be raised by sibling goroutines concurrently, which only makes
			// pruning more aggressive (safe monotone read).
			localPrefix := make([]int, len(prefix)+1)
			copy(localPrefix, prefix)
			localPrefix[0] = t.X.item

			exULs := make([]*UtilityList, 0, len(ULs)-t.idx-1)
			for j := t.idx + 1; j < len(ULs); j++ {
				Y := ULs[j]
				// [PARALLEL] Atomic increment so sibling goroutines do not lose counts.
				atomic.AddInt64(&a.candidateCount64, 1)
				exul := a.construct(nil, t.X, Y)
				if exul != nil {
					exULs = append(exULs, exul)
				}
			}

			// Recurse serially from depth 1 downward.
			if err := a.thuiSerial(localPrefix, 1, t.X, exULs); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// thuiSerial is the serial recursive mining procedure used at depth >= 1.
// It is identical to the original thui() but calls saveSafe() for thread safety.
func (a *AlgoPTHUI) thuiSerial(prefix []int, prefixLength int, pUL *UtilityList, ULs []*UtilityList) error {
	// Read minUtility once; it can only increase so stale reads are safe.
	threshold := a.minUtility

	for i := len(ULs) - 1; i >= 0; i-- {
		if ULs[i].getUtils() >= threshold {
			a.saveSafe(prefix, prefixLength, ULs[i]) // [PARALLEL] guarded by kPatternsMu
		}
	}

	for i := len(ULs) - 2; i >= 0; i-- {
		a.checkMemory()
		X := ULs[i]
		if X.sumIutils+X.sumRutils >= threshold && X.sumIutils > 0 {
			if a.eucsPrune {
				if _, ok := a.mapFMAP[X.item]; !ok {
					continue
				}
			}
			exULs := make([]*UtilityList, 0, len(ULs)-i-1)
			for j := i + 1; j < len(ULs); j++ {
				Y := ULs[j]
				atomic.AddInt64(&a.candidateCount64, 1)
				exul := a.construct(pUL, X, Y)
				if exul != nil {
					exULs = append(exULs, exul)
				}
			}
			prefix[prefixLength] = X.item
			// Refresh threshold after save() may have raised it.
			threshold = a.minUtility
			if err := a.thuiSerial(prefix, prefixLength+1, X, exULs); err != nil {
				return err
			}
		}
	}
	return nil
}


func (a *AlgoPTHUI) construct(P, px, py *UtilityList) *UtilityList {
	//capacity hint = min(|px|, |py|)
	cap := len(px.elements)
	if len(py.elements) < cap {
		cap = len(py.elements)
	}
	pxyUL := &UtilityList{
		item:     py.item,
		elements: make([]Element, 0, cap),
	}

	totUtil := px.sumIutils + px.sumRutils
	ei, ej, Pi := 0, 0, -1

	pxElems := px.elements
	pyElems := py.elements

	for ei < len(pxElems) && ej < len(pyElems) {
		ex := pxElems[ei]
		ey := pyElems[ej]

		if ex.tid > ey.tid {
			ej++
			continue
		}
		if ex.tid < ey.tid {
			totUtil -= int64(ex.iutils) + int64(ex.rutils)
			if totUtil < a.minUtility {
				return nil
			}
			ei++
			Pi++
			continue
		}
		// ex.tid == ey.tid
		if P == nil {
			iutil := ex.iutils + ey.iutils
			pxyUL.sumIutils += int64(iutil)
			pxyUL.sumRutils += int64(ey.rutils)
			pxyUL.elements = append(pxyUL.elements, Element{ex.tid, iutil, ey.rutils})
		} else {
			Pi++
			pElems := P.elements
			for Pi < len(pElems) && pElems[Pi].tid < ex.tid {
				Pi++
			}
			if Pi >= len(pElems) {
				return nil
			}
			e := pElems[Pi]
			iutil := ex.iutils + ey.iutils - e.iutils
			pxyUL.sumIutils += int64(iutil)
			pxyUL.sumRutils += int64(ey.rutils)
			pxyUL.elements = append(pxyUL.elements, Element{ex.tid, iutil, ey.rutils})
		}
		ei++
		ej++
	}

	for ei < len(pxElems) {
		ex := pxElems[ei]
		totUtil -= int64(ex.iutils) + int64(ex.rutils)
		if totUtil < a.minUtility {
			return nil
		}
		ei++
	}
	return pxyUL
}

// saveSafe is the thread-safe version of save().
// It acquires kPatternsMu before touching kPatterns or minUtility.
// [PARALLEL] Called by thuiParallel goroutines concurrently.
func (a *AlgoPTHUI) saveSafe(prefix []int, length int, X *UtilityList) {
	//  store int slice, defer string build to write time
	prefixCopy := make([]int, length)
	if length > 0 {
		copy(prefixCopy, prefix[:length])
	}
	p := PatternTHUI{
		prefixItems: prefixCopy,
		lastItem:    X.item,
		utility:     X.getUtils(),
	}
	a.kPatternsMu.Lock() // [PARALLEL] exclusive access to the shared heap
	heap.Push(&a.kPatterns, p)

	if a.kPatterns.Len() > a.topkstatic {
		if X.getUtils() >= a.minUtility {
			for a.kPatterns.Len() > a.topkstatic {
				heap.Pop(&a.kPatterns)
			}
		}
		a.minUtility = a.kPatterns[0].utility
	}
	a.kPatternsMu.Unlock()
}

func (a *AlgoPTHUI) writeResultToFile() error {
	if a.kPatterns.Len() == 0 {
		return nil
	}

	patterns := make([]PatternTHUI, 0, a.kPatterns.Len())
	for a.kPatterns.Len() > 0 {
		a.HuiCount++
		patterns = append(patterns, heap.Pop(&a.kPatterns).(PatternTHUI))
	}

	sort.Slice(patterns, func(i, j int) bool {
		return a.mapItemToTWU[patterns[i].firstItem()] < a.mapItemToTWU[patterns[j].firstItem()]
	})

	for _, p := range patterns {
		line := p.prefixString() + " #UTIL: " + strconv.FormatInt(p.utility, 10)
		if _, err := fmt.Fprintln(a.writer, line); err != nil {
			return err
		}
	}
	return nil
}

func (a *AlgoPTHUI) raisingThresholdRIU(m map[int]int64, k int) {
	vals := make([]int64, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] > vals[j] })
	if len(vals) >= k && k > 0 {
		a.minUtility = vals[k-1]
	}
}
// The utility stored for each pair (a, b) is the sum of (util_a + util_b) over
// all transactions where both appear. This is an exact utility for the 2-itemset
// {a, b}, so the k-th largest such value is a valid lower bound.
func (a *AlgoPTHUI) raisingThresholdCUDOptimize(k int) {
	ktopls := &longHeap{}
	heap.Init(ktopls)
	for _, inner := range a.mapFMAP {
		for _, item := range inner {
			value := item.utility
			if value >= a.minUtility {
				if ktopls.Len() < k {
					heap.Push(ktopls, value)
				} else if value > (*ktopls)[0] {
					heap.Push(ktopls, value)
					for ktopls.Len() > k {
						heap.Pop(ktopls)
					}
				}
			}
		}
	}
	if ktopls.Len() > k-1 && (*ktopls)[0] > a.minUtility {
		a.minUtility = (*ktopls)[0]
	}
}
// raisingThresholdLeaf raises minUtility using two complementary strategies:
//
//  LIU-Exact: for every entry in mapLeafMAP, the stored value is the exact
//  combined utility of a consecutive item group in some transaction. These
//  are valid itemset utilities and are fed directly into addToLeafPruneUtils.
//
//  LIU-LB (Lower Bound): from each consecutive-group utility, estimated
//  utilities of sub-groups are derived by subtracting individual item
//  utilities. The nested loops enumerate sub-groups of size 2, 3, and 4.
//
//  Finally, all single-item utilities are also considered.
//
//  The k-th largest value across all of these estimates becomes the new minUtility.
func (a *AlgoPTHUI) addToLeafPruneUtils(value int64) {
	if a.leafPruneUtils.Len() < a.topkstatic {
		heap.Push(a.leafPruneUtils, value)
	} else if value > (*a.leafPruneUtils)[0] {
		heap.Push(a.leafPruneUtils, value)
		for a.leafPruneUtils.Len() > a.topkstatic {
			heap.Pop(a.leafPruneUtils)
		}
	}
}

// =============================================================================
// [PARALLEL – Fork/Join] Phase 3: LIU-LB threshold raising
// =============================================================================
//
// Strategy:
//   The outer loop over mapLeafMAP entries is distributed across numWorkers
//   goroutines. Each goroutine accumulates candidates into its own local
//   longHeap. The main goroutine collects all local heaps and merges them
//   into the global leafPruneUtils heap.

// raisingThresholdLeafParallel is the parallel replacement for raisingThresholdLeaf.
func (a *AlgoPTHUI) raisingThresholdLeafParallel(ULs []*UtilityList) {
	// --- LIU-Exact (serial): feed direct values first to set a baseline ---
	// These are exact utilities so they are cheap to process serially.
	for _, inner := range a.mapLeafMAP {
		for _, value := range inner {
			if value >= a.minUtility {
				a.addToLeafPruneUtils(value)
			}
		}
	}

	// Collect mapLeafMAP keys into a slice so we can partition them.
	type entry struct {
		endKey int
		inner  map[int]int64
	}
	entries := make([]entry, 0, len(a.mapLeafMAP))
	for k, v := range a.mapLeafMAP {
		entries = append(entries, entry{k, v})
	}

	nw := a.numWorkers
	chunkSize := (len(entries) + nw - 1) / nw

	// Each worker builds its own longHeap of candidate values.
	localHeaps := make([]longHeap, nw)
	for i := range localHeaps {
		localHeaps[i] = longHeap(nil)
		heap.Init(&localHeaps[i])
	}

	// [PARALLEL – Fork/Join] Distribute outer-loop iterations across goroutines.
	var wg sync.WaitGroup
	for w := 0; w < nw; w++ {
		w := w
		start := w * chunkSize
		end := start + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		if start >= end {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			lh := &localHeaps[w]
			snapshot := a.minUtility // read-only snapshot; fine without a lock

			for _, e := range entries[start:end] {
				end_ := e.endKey + 1
				for stKey, value := range e.inner {
					if value < snapshot {
						continue
					}
					st := stKey
					// LIU-LB: enumerate sub-group size 1, 2, 3 by removing items.
					// [PARALLEL] Each goroutine works on its own subset of (end,start) pairs.
					for i := st + 1; i < end_-1; i++ {
						v2 := value - ULs[i].getUtils()
						if v2 >= snapshot {
							addToHeapK(lh, v2, a.topkstatic)
						}
						for j := i + 1; j < end_-1; j++ {
							v2 = value - ULs[i].getUtils() - ULs[j].getUtils()
							if v2 >= snapshot {
								addToHeapK(lh, v2, a.topkstatic)
							}
							for k := j + 1; k+1 < end_-1; k++ {
								v2 = value - ULs[i].getUtils() - ULs[j].getUtils() - ULs[k].getUtils()
								if v2 >= snapshot {
									addToHeapK(lh, v2, a.topkstatic)
								}
							}
						}
					}
				}
			}
		}()
	}
	wg.Wait()

	// [PARALLEL – Fork/Join JOIN] Merge all local heaps into the global one (serial).
	for w := 0; w < nw; w++ {
		for localHeaps[w].Len() > 0 {
			v := heap.Pop(&localHeaps[w]).(int64)
			a.addToLeafPruneUtils(v)
		}
	}

	// Add all single-item utilities (serial; small compared to LIU-LB).
	for _, u := range ULs {
		value := u.getUtils()
		if value >= a.minUtility {
			a.addToLeafPruneUtils(value)
		}
	}

	if a.leafPruneUtils.Len() > a.topkstatic-1 && (*a.leafPruneUtils)[0] > a.minUtility {
		a.minUtility = (*a.leafPruneUtils)[0]
	}
}

// addToHeapK inserts v into h while keeping h.Len() <= k.
func addToHeapK(h *longHeap, v int64, k int) {
	if h.Len() < k {
		heap.Push(h, v)
	} else if v > (*h)[0] {
		heap.Push(h, v)
		for h.Len() > k {
			heap.Pop(h)
		}
	}
}

func (a *AlgoPTHUI) removeEntry() {
	for _, inner := range a.mapFMAP {
		for key, item := range inner {
			if item.twu < a.minUtility {
				delete(inner, key)
			}
		}
	}
}

func (a *AlgoPTHUI) removeLeafEntry() {
	for outerKey, inner := range a.mapLeafMAP {
		for key := range inner {
			delete(inner, key)
		}
		delete(a.mapLeafMAP, outerKey)
	}
}

// checkMemory is throttled to every 4096 calls. 
func (a *AlgoPTHUI) checkMemory() {
	a.memCheckCounter++
	if a.memCheckCounter&0xFFF == 0 {
		a.checkMemoryForce()
	}
}

func (a *AlgoPTHUI) checkMemoryForce() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	current := float64(ms.Alloc) / 1024.0 / 1024.0
	if current > a.MaxMemory {
		a.MaxMemory = current
	}
}

// PrintStats prints algorithm statistics.
func (a *AlgoPTHUI) PrintStats() {
	elapsed := a.EndTimestamp - a.StartTimestamp
	fmt.Println("============= PARALLEL THUI ALGORITHM - STATS =============")
	fmt.Printf(" Total time ~ %d ms\n", elapsed)
	fmt.Printf(" Memory ~ %.2f MB\n", a.MaxMemory)
	fmt.Printf(" High-utility itemsets count : %d  Candidates : %d\n", a.HuiCount, a.CandidateCount)
	fmt.Printf(" Final minimum utility : %d\n", a.minUtility)
	fmt.Printf(" Workers : %d\n", a.numWorkers)
	base := a.inputFile
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx >= 0 {
		base = base[:idx]
	}
	fmt.Printf(" Dataset : %s\n", base)
	fmt.Printf(" End time %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("===================================================")
}

// =============================================================================
// File reading utility
// =============================================================================

// readAllLines reads an entire file into a slice of byte slices.
// Each slice is a copy of one line (no scanner buffer lifetime dependency).
// Used by the parallel scans so workers can access lines by index.
func readAllLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4<<20), 4<<20) // 4 MB buffer for wide transactions
	var lines [][]byte
	for scanner.Scan() {
		b := scanner.Bytes()
		cp := make([]byte, len(b)) // copy so the slice outlives the scanner buffer
		copy(cp, b)
		lines = append(lines, cp)
	}
	return lines, scanner.Err()
}

// countValidLines counts non-comment, non-empty, well-formed lines in a slice.
// Used to compute the correct starting tid for each worker's partition.
func countValidLines(lines [][]byte) int {
	n := 0
	for _, line := range lines {
		if isCommentOrEmpty(line) {
			continue
		}
		_, ok := parseLine(line)
		if ok {
			n++
		}
	}
	return n
}

// =============================================================================
// Zero-alloc byte-level parsing helpers
// ---------------------------------------------------------------------------

// parseInt parses a decimal integer from a byte slice with no heap allocations.
func parseInt(b []byte) int64 {
	neg := false
	if len(b) > 0 && b[0] == '-' {
		neg = true
		b = b[1:]
	}
	var n int64
	for _, c := range b {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		return -n
	}
	return n
}

// byteFieldIter iterates over space-separated fields in a byte slice without allocation.
type byteFieldIter struct {
	data []byte
	pos  int
}

func newByteFieldIter(data []byte) byteFieldIter {
	return byteFieldIter{data: data}
}

func (it *byteFieldIter) next() ([]byte, bool) {
	d := it.data
	n := len(d)
	for it.pos < n && d[it.pos] == ' ' {
		it.pos++
	}
	if it.pos >= n {
		return nil, false
	}
	start := it.pos
	for it.pos < n && d[it.pos] != ' ' {
		it.pos++
	}
	return d[start:it.pos], true
}
