
package thui

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

// ItemTHUI stores TWU and utility for the EUCS structure.
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

// ---------------------------------------------------------------------------
// AlgoTHUI
// ---------------------------------------------------------------------------

type AlgoTHUI struct {
	MaxMemory      float64
	StartTimestamp int64
	EndTimestamp   int64
	HuiCount       int
	CandidateCount int

	mapItemToTWU map[int]int64
	minUtility   int64
	topkstatic   int

	writer *bufio.Writer

	kPatterns      patternHeap
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
}

func NewAlgoTHUI() *AlgoTHUI {
	return &AlgoTHUI{
		leafPrune: true,
		txBuf:     make([]pair, 0, 256),
	}
}

// RunAlgorithm executes the THUI algorithm.
func (a *AlgoTHUI) RunAlgorithm(input, output string, eucsPrune bool, topk int) error {
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

	// ---- First scan ----
	txCount, err := a.firstScan(input, riu)
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

	// ---- Second scan ----
	if err = a.secondScan(input, mapItemToUL, listOfULs); err != nil {
		return err
	}

	if a.eucsPrune {
		a.raisingThresholdCUDOptimize(a.topkstatic)
		a.removeEntry()
	}

	if a.leafPrune {
		a.raisingThresholdLeaf(listOfULs)
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

	if err = a.thui(a.itemsetBuffer, 0, nil, listOfULs); err != nil {
		return err
	}
	a.checkMemoryForce()

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

// firstScan computes TWU and RIU, returns transaction count.
func (a *AlgoTHUI) firstScan(input string, riu map[int]int64) (int, error) {
	f, err := os.Open(input)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4<<20), 4<<20) //  4 MB buffer
	txCount := 0

	for scanner.Scan() {
		line := scanner.Bytes()
		if isCommentOrEmpty(line) {
			continue
		}
		pl, ok := parseLine(line)
		if !ok {
			continue // skip malformed lines gracefully
		}
		txCount++

		transactionUtility := parseInt(pl.tuRaw)
		ii := newByteFieldIter(pl.itemsRaw)
		ui := newByteFieldIter(pl.utilsRaw)
		for {
			itemToken, ok1 := ii.next()
			utilToken, ok2 := ui.next()
			if !ok1 || !ok2 {
				break
			}
			item := int(parseInt(itemToken))
			a.mapItemToTWU[item] += transactionUtility
			riu[item] += parseInt(utilToken)
		}
	}
	return txCount, scanner.Err()
}

// secondScan builds utility lists.
func (a *AlgoTHUI) secondScan(input string, mapItemToUL map[int]*UtilityList, listOfULs []*UtilityList) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4<<20), 4<<20)
	tid := 0

	//  keep txBuf in a local var; write back at end to persist grown capacity
	tx := a.txBuf[:0]

	for scanner.Scan() {
		line := scanner.Bytes()
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
			itemToken, ok1 := ii.next()
			utilToken, ok2 := ui.next()
			if !ok1 || !ok2 {
				break
			}
			item := int(parseInt(itemToken))
			util := int32(parseInt(utilToken))
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
			ulOfItem := mapItemToUL[p.item]
			ulOfItem.addElement(Element{int32(tid), p.utility, remainingUtility})

			if a.eucsPrune {
				a.updateEUCSprune(i, p, tx, newTWU)
			}
			if a.leafPrune {
				a.updateLeafprune(i, p, tx)
			}
			remainingUtility += p.utility
		}
		tid++
	}
	a.txBuf = tx // persist grown capacity back to struct
	return scanner.Err()
}

// compareItemsDirect uses the map (before twuByIdx is built).
func (a *AlgoTHUI) compareItemsDirect(item1, item2 int) int {
	d := a.mapItemToTWU[item1] - a.mapItemToTWU[item2]
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return item1 - item2
}

func (a *AlgoTHUI) updateEUCSprune(i int, p pair, tx []pair, newTWU int64) {
	mapFMAPItem := a.mapFMAP[p.item]
	if mapFMAPItem == nil {
		mapFMAPItem = make(map[int]*ItemTHUI, len(tx)-i)
		a.mapFMAP[p.item] = mapFMAPItem
	}
	for j := i + 1; j < len(tx); j++ {
		after := tx[j]
		if p.item == after.item {
			continue
		}
		ti := mapFMAPItem[after.item]
		if ti == nil {
			ti = &ItemTHUI{}
			mapFMAPItem[after.item] = ti
		}
		ti.twu += newTWU
		ti.utility += int64(p.utility) + int64(after.utility)
	}
}

func (a *AlgoTHUI) updateLeafprune(i int, p pair, tx []pair) {
	cutil := int64(p.utility)
	followingItemIdx := a.itemIndex[p.item] 
	mapLeafItem := a.mapLeafMAP[followingItemIdx]
	if mapLeafItem == nil {
		mapLeafItem = make(map[int]int64, i)
		a.mapLeafMAP[followingItemIdx] = mapLeafItem
	}
	for j := i - 1; j >= 0; j-- {
		after := tx[j]
		if p.item == after.item {
			continue
		}
		followingItemIdx--
		if followingItemIdx < 0 || a.itemIndex[after.item] != followingItemIdx {
			break
		}
		cutil += int64(after.utility)
		mapLeafItem[followingItemIdx] += cutil
	}
}

func (a *AlgoTHUI) setLeafMapSize() {
	for _, v := range a.mapLeafMAP {
		a.leafMapSize += len(v)
	}
}

func (a *AlgoTHUI) thui(prefix []int, prefixLength int, pUL *UtilityList, ULs []*UtilityList) error {
	for i := len(ULs) - 1; i >= 0; i-- {
		if ULs[i].getUtils() >= a.minUtility {
			a.save(prefix, prefixLength, ULs[i])
		}
	}

	for i := len(ULs) - 2; i >= 0; i-- {
		a.checkMemory() 
		X := ULs[i]
		if X.sumIutils+X.sumRutils >= a.minUtility && X.sumIutils > 0 {
			if a.eucsPrune {
				if _, ok := a.mapFMAP[X.item]; !ok {
					continue
				}
			}

			//  pre-allocate exULs
			exULs := make([]*UtilityList, 0, len(ULs)-i-1)
			for j := i + 1; j < len(ULs); j++ {
				Y := ULs[j]
				a.CandidateCount++
				exul := a.construct(pUL, X, Y)
				if exul != nil {
					exULs = append(exULs, exul)
				}
			}
			prefix[prefixLength] = X.item
			if err := a.thui(prefix, prefixLength+1, X, exULs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *AlgoTHUI) construct(P, px, py *UtilityList) *UtilityList {
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

func (a *AlgoTHUI) save(prefix []int, length int, X *UtilityList) {
	//  store int slice, defer string build to write time
	prefixCopy := make([]int, length)
	copy(prefixCopy, prefix[:length])

	p := PatternTHUI{
		prefixItems: prefixCopy,
		lastItem:    X.item,
		utility:     X.getUtils(),
	}
	heap.Push(&a.kPatterns, p)

	if a.kPatterns.Len() > a.topkstatic {
		if X.getUtils() >= a.minUtility {
			for a.kPatterns.Len() > a.topkstatic {
				heap.Pop(&a.kPatterns)
			}
		}
		a.minUtility = a.kPatterns[0].utility
	}
}

func (a *AlgoTHUI) writeResultToFile() error {
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

func (a *AlgoTHUI) raisingThresholdRIU(m map[int]int64, k int) {
	vals := make([]int64, 0, len(m))
	for _, v := range m {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i] > vals[j] })
	if len(vals) >= k && k > 0 {
		a.minUtility = vals[k-1]
	}
}

func (a *AlgoTHUI) raisingThresholdCUDOptimize(k int) {
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

func (a *AlgoTHUI) addToLeafPruneUtils(value int64) {
	if a.leafPruneUtils.Len() < a.topkstatic {
		heap.Push(a.leafPruneUtils, value)
	} else if value > (*a.leafPruneUtils)[0] {
		heap.Push(a.leafPruneUtils, value)
		for a.leafPruneUtils.Len() > a.topkstatic {
			heap.Pop(a.leafPruneUtils)
		}
	}
}

func (a *AlgoTHUI) raisingThresholdLeaf(ULs []*UtilityList) {
	for _, inner := range a.mapLeafMAP {
		for _, value := range inner {
			if value >= a.minUtility {
				a.addToLeafPruneUtils(value)
			}
		}
	}

	for endKey, inner := range a.mapLeafMAP {
		end := endKey + 1
		for stKey, value := range inner {
			if value < a.minUtility {
				continue
			}
			st := stKey
			for i := st + 1; i < end-1; i++ {
				value2 := value - ULs[i].getUtils()
				if value2 >= a.minUtility {
					a.addToLeafPruneUtils(value2)
				}
				for j := i + 1; j < end-1; j++ {
					value2 = value - ULs[i].getUtils() - ULs[j].getUtils()
					if value2 >= a.minUtility {
						a.addToLeafPruneUtils(value2)
					}
					for k := j + 1; k+1 < end-1; k++ {
						value2 = value - ULs[i].getUtils() - ULs[j].getUtils() - ULs[k].getUtils()
						if value2 >= a.minUtility {
							a.addToLeafPruneUtils(value2)
						}
					}
				}
			}
		}
	}

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

func (a *AlgoTHUI) removeEntry() {
	for _, inner := range a.mapFMAP {
		for key, item := range inner {
			if item.twu < a.minUtility {
				delete(inner, key)
			}
		}
	}
}

func (a *AlgoTHUI) removeLeafEntry() {
	for outerKey, inner := range a.mapLeafMAP {
		for key := range inner {
			delete(inner, key)
		}
		delete(a.mapLeafMAP, outerKey)
	}
}

// checkMemory is throttled to every 4096 calls. 
func (a *AlgoTHUI) checkMemory() {
	a.memCheckCounter++
	if a.memCheckCounter&0xFFF == 0 {
		a.checkMemoryForce()
	}
}

func (a *AlgoTHUI) checkMemoryForce() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	current := float64(ms.Alloc) / 1024.0 / 1024.0
	if current > a.MaxMemory {
		a.MaxMemory = current
	}
}

// PrintStats prints algorithm statistics.
func (a *AlgoTHUI) PrintStats() {
	elapsed := a.EndTimestamp - a.StartTimestamp
	fmt.Println("=============  THUI ALGORITHM - STATS =============")
	fmt.Printf(" Total time ~ %d ms\n", elapsed)
	fmt.Printf(" Memory ~ %.2f MB\n", a.MaxMemory)
	fmt.Printf(" High-utility itemsets count : %d  Candidates : %d\n", a.HuiCount, a.CandidateCount)
	fmt.Printf(" Final minimum utility : %d\n", a.minUtility)
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

// ---------------------------------------------------------------------------
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

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

// func main() {
// 	algo := NewAlgoTHUI()

// 	input := "retail.txt" // path to your input file
// 	output := "output2.txt"      // path to write results
// 	eucsPrune := true           // enable EUCS pruning
// 	topk := 1000                  // find top-10 HUIs

// 	if err := algo.RunAlgorithm(input, output, eucsPrune, topk); err != nil {
// 		log.Fatalf("Algorithm error: %v", err)
// 	}

// 	algo.PrintStats()
// 	fmt.Printf("Results written to %s\n", output)
// }