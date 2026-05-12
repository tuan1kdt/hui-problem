package tko

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"hui-problem/internal/pkg/memory"
)

// AlgoTKOBasic is the SPMF "TKO-Basic" algorithm (utility-list top-k HUI mining).
type AlgoTKOBasic struct {
	k            int
	minUtility   int64
	kHeap        itemsetMinHeap
	mapItemToTWU map[int]int

	totalTime float64
}

// NewAlgoTKOBasic creates a new miner.
func NewAlgoTKOBasic() *AlgoTKOBasic {
	return &AlgoTKOBasic{
		mapItemToTWU: make(map[int]int),
	}
}

// RunAlgorithm scans the database, mines, and writes results to output (Java: runAlgorithm + writeResultTofile).
// TKO uses a one-phase approach: builds utility lists once, then dynamically raises minUtility threshold during search.
func (a *AlgoTKOBasic) RunAlgorithm(inputPath, outputPath string, k int) error {
	memory.Reset()
	memory.Sample()

	start := time.Now()
	a.minUtility = 1 // Start with minUtility = 1; will be raised dynamically as better itemsets are found
	a.k = k
	a.kHeap = nil // Min-heap to store top-k itemsets
	a.mapItemToTWU = make(map[int]int)

	// Step 1: First database pass — compute Transaction Weighted Utility (TWU) for each item.
	// TWU is a global pruning threshold: items with low TWU cannot be in high-utility itemsets.
	if err := a.firstPass(inputPath); err != nil {
		return err
	}

	// Step 2: Build utility list structure and populate with transaction data.
	// Sort items by TWU ascending to enable aggressive pruning (explore high-TWU items last).
	listItems, mapItemToUL, err := a.buildUtilityListShells()
	if err != nil {
		return err
	}

	// Step 2b: Second database pass — populate utility lists with (tid, iutils, rutils) tuples.
	if err := a.secondPass(inputPath, mapItemToUL); err != nil {
		return err
	}

	memory.Sample()

	// Step 3: Recursive depth-first search mining phase.
	// Uses utility-list joins to construct combined itemsets; dynamically raises minUtility as k itemsets are found.
	if err := a.search([]int{}, nil, listItems); err != nil {
		return err
	}

	memory.Sample()
	a.totalTime = time.Since(start).Seconds()

	// Step 4: Write top-k results to output file in SPMF format.
	if err := a.WriteResultToFile(outputPath); err != nil {
		return err
	}
	memory.Sample()
	return nil
}

// firstPass scans the database once to compute Transaction Weighted Utility (TWU) for each item.
// TWU[item] = sum of transaction utilities for all transactions containing that item.
// Used as a global pruning threshold in the search phase.
func (a *AlgoTKOBasic) firstPass(inputPath string) error {
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
		items := strings.Fields(strings.TrimSpace(parts[0]))
		for _, s := range items {
			item, err := strconv.Atoi(s)
			if err != nil {
				continue
			}
			a.mapItemToTWU[item] += tu
		}
	}
	return sc.Err()
}

// buildUtilityListShells creates empty utility lists for each item and sorts them by TWU ascending.
// Sorting by TWU enables pruning: low-TWU items are explored first, so when k itemsets are found,
// we can raise minUtility and prune branches containing high-TWU items more aggressively.
func (a *AlgoTKOBasic) buildUtilityListShells() ([]*UtilityList, map[int]*UtilityList, error) {
	listItems := make([]*UtilityList, 0, len(a.mapItemToTWU))
	mapItemToUL := make(map[int]*UtilityList, len(a.mapItemToTWU))
	for item := range a.mapItemToTWU {
		ul := NewUtilityList(item)
		listItems = append(listItems, ul)
		mapItemToUL[item] = ul
	}
	// Sort by TWU ascending (low first); ties broken by item ID for determinism.
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

// secondPass populates utility lists with transaction data: (tid, iutils, rutils) tuples.
// iutils = item utility in this transaction; rutils = sum of utilities for items after this one (for pruning).
func (a *AlgoTKOBasic) secondPass(inputPath string, mapItemToUL map[int]*UtilityList) error {
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

		revised := make([]pair, 0, len(items))
		remainingUtility := 0
		for i := range items {
			item, _ := strconv.Atoi(items[i])
			u, _ := strconv.Atoi(utilityValues[i])
			revised = append(revised, pair{item: item, utility: u})
			remainingUtility += u
		}
		// Sort items by TWU (same order as listItems for consistency).
		sort.Slice(revised, func(i, j int) bool {
			return a.compareItems(revised[i].item, revised[j].item) < 0
		})

		// Add each (tid, iutils, rutils) tuple to the item's utility list.
		for _, p := range revised {
			remainingUtility -= p.utility
			ul := mapItemToUL[p.item]
			el := Element{tid, p.utility, remainingUtility}
			ul.AddElement(el)
		}
		tid++
	}
	return sc.Err()
}

func (a *AlgoTKOBasic) compareItems(item1, item2 int) int {
	c := a.mapItemToTWU[item1] - a.mapItemToTWU[item2]
	if c != 0 {
		return c
	}
	return item1 - item2
}

// search recursively mines itemsets using depth-first search with utility-list pruning.
// prefix: current itemset; pUL: parent (prefix) utility list; uls: candidate items to extend.
// Dynamically raises minUtility as k itemsets are found, enabling aggressive pruning.
func (a *AlgoTKOBasic) search(prefix []int, pUL *UtilityList, uls []*UtilityList) error {
	memory.Sample()
	for i := 0; i < len(uls); i++ {
		X := uls[i]
		// Check if single item X meets minUtility threshold; if so, add itemset {prefix + X} to heap.
		if X.SumIutils >= a.minUtility {
			a.writeOut(prefix, X.Item, X.SumIutils)
		}
		// Pruning: check if X can possibly lead to a high-utility itemset.
		// SumIutils + SumRutils is an upper bound on utility for itemsets containing X.
		if X.SumRutils+X.SumIutils >= a.minUtility {
			// Construct utility lists for all pairs (X, Y) where Y comes after X in the sorted order.
			exULs := make([]*UtilityList, 0, len(uls)-i-1)
			for j := i + 1; j < len(uls); j++ {
				Y := uls[j]
				exULs = append(exULs, a.construct(pUL, X, Y))
			}
			// Recurse with extended prefix.
			newPrefix := make([]int, len(prefix)+1)
			copy(newPrefix, prefix)
			newPrefix[len(prefix)] = X.Item
			if err := a.search(newPrefix, X, exULs); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeOut adds an itemset to the min-heap and maintains only the top-k itemsets.
// When heap size exceeds k, the weakest itemset is removed and minUtility is raised.
// This dynamic threshold enables aggressive pruning in later search branches.
func (a *AlgoTKOBasic) writeOut(prefix []int, item int, utility int64) {
	is := newItemsetTKO(prefix, item, utility)
	heap.Push(&a.kHeap, is)
	if a.kHeap.Len() > a.k {
		if utility > a.minUtility {
			// Remove excess itemsets, keeping only the k strongest.
			for a.kHeap.Len() > a.k {
				heap.Pop(&a.kHeap)
			}
			// Update minUtility to the weakest itemset in the heap for pruning.
			if a.kHeap.Len() > 0 {
				a.minUtility = a.kHeap[0].Utility
			}
		}
	}
}

// construct joins utility lists px and py to create a combined utility list for itemset {P, X, Y}.
// For each transaction containing both X and Y:
//   - iutils = iutils[X] + iutils[Y] (- iutils[P] if P is non-null, to avoid double-counting).
//   - rutils = remaining utility after Y (from py).
func (a *AlgoTKOBasic) construct(P, px, py *UtilityList) *UtilityList {
	pxyUL := NewUtilityList(py.Item)
	for _, ex := range px.Elements {
		ey := a.findElementWithTID(py, ex.TID)
		if ey == nil {
			continue
		}
		if P == nil {
			// First itemset: utility = X_iutils + Y_iutils.
			eXY := Element{ex.TID, ex.Iutils + ey.Iutils, ey.Rutils}
			pxyUL.AddElement(eXY)
		} else {
			// Extended itemset: utility = X_iutils + Y_iutils - P_iutils (subtract prefix to avoid overlap).
			e := a.findElementWithTID(P, ex.TID)
			if e != nil {
				eXY := Element{ex.TID, ex.Iutils + ey.Iutils - e.Iutils, ey.Rutils}
				pxyUL.AddElement(eXY)
			}
		}
	}
	return pxyUL
}

func (a *AlgoTKOBasic) findElementWithTID(ulist *UtilityList, tid int) *Element {
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

// WriteResultToFile writes the current k-heap to a file (SPMF format).
func (a *AlgoTKOBasic) WriteResultToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	// Copy heap to slice for iteration (Java iterator order is undefined; we dump heap order).
	n := a.kHeap.Len()
	items := make([]*ItemsetTKO, n)
	for i := 0; i < n; i++ {
		items[i] = a.kHeap[i]
	}
	for i, it := range items {
		var b strings.Builder
		for j := 0; j < len(it.Prefix); j++ {
			if j > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(it.Prefix[j]))
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

// PrintStats prints SPMF-style statistics.
func (a *AlgoTKOBasic) PrintStats() {
	fmt.Println("=============  TKO-Basic (Go)  =============")
	fmt.Println(" High-utility itemsets count :", a.kHeap.Len())
	fmt.Println(" Total time ~", a.totalTime, "s")
	fmt.Println(" Memory ~", memory.MaxMB(), "MB (approx)")
	fmt.Println("===================================================")
}

// ResultSize returns the number of itemsets in the result heap.
func (a *AlgoTKOBasic) ResultSize() int { return a.kHeap.Len() }
