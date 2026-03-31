package tko

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AlgoTKOBasic is the SPMF "TKO-Basic" algorithm (utility-list top-k HUI mining).
type AlgoTKOBasic struct {
	k            int
	minUtility   int64
	kHeap        itemsetMinHeap
	mapItemToTWU map[int]int

	totalTime   float64
	maxMemoryMB float64
}

// NewAlgoTKOBasic creates a new miner.
func NewAlgoTKOBasic() *AlgoTKOBasic {
	return &AlgoTKOBasic{
		mapItemToTWU: make(map[int]int),
	}
}

// RunAlgorithm scans the database, mines, and writes results to output (Java: runAlgorithm + writeResultTofile).
func (a *AlgoTKOBasic) RunAlgorithm(inputPath, outputPath string, k int) error {
	a.maxMemoryMB = 0
	a.sampleMemory()

	start := time.Now()
	a.minUtility = 1
	a.k = k
	a.kHeap = nil
	a.mapItemToTWU = make(map[int]int)

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

	a.sampleMemory()

	if err := a.search([]int{}, nil, listItems); err != nil {
		return err
	}

	a.sampleMemory()
	a.totalTime = time.Since(start).Seconds()

	if err := a.WriteResultToFile(outputPath); err != nil {
		return err
	}
	a.sampleMemory()
	return nil
}

func (a *AlgoTKOBasic) sampleMemory() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	used := float64(ms.Alloc) / (1024 * 1024)
	if used > a.maxMemoryMB {
		a.maxMemoryMB = used
	}
}

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

func (a *AlgoTKOBasic) buildUtilityListShells() ([]*UtilityList, map[int]*UtilityList, error) {
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
		sort.Slice(revised, func(i, j int) bool {
			return a.compareItems(revised[i].item, revised[j].item) < 0
		})

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

func (a *AlgoTKOBasic) search(prefix []int, pUL *UtilityList, uls []*UtilityList) error {
	a.sampleMemory()
	for i := 0; i < len(uls); i++ {
		X := uls[i]
		if X.SumIutils >= a.minUtility {
			a.writeOut(prefix, X.Item, X.SumIutils)
		}
		if X.SumRutils+X.SumIutils >= a.minUtility {
			exULs := make([]*UtilityList, 0, len(uls)-i-1)
			for j := i + 1; j < len(uls); j++ {
				Y := uls[j]
				exULs = append(exULs, a.construct(pUL, X, Y))
			}
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

func (a *AlgoTKOBasic) writeOut(prefix []int, item int, utility int64) {
	is := newItemsetTKO(prefix, item, utility)
	heap.Push(&a.kHeap, is)
	if a.kHeap.Len() > a.k {
		if utility > a.minUtility {
			for a.kHeap.Len() > a.k {
				heap.Pop(&a.kHeap)
			}
			if a.kHeap.Len() > 0 {
				a.minUtility = a.kHeap[0].Utility
			}
		}
	}
}

func (a *AlgoTKOBasic) construct(P, px, py *UtilityList) *UtilityList {
	pxyUL := NewUtilityList(py.Item)
	for _, ex := range px.Elements {
		ey := a.findElementWithTID(py, ex.TID)
		if ey == nil {
			continue
		}
		if P == nil {
			eXY := Element{ex.TID, ex.Iutils + ey.Iutils, ey.Rutils}
			pxyUL.AddElement(eXY)
		} else {
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
	fmt.Println(" Memory ~", a.maxMemoryMB, "MB (approx)")
	fmt.Println("===================================================")
}

// ResultSize returns the number of itemsets in the result heap.
func (a *AlgoTKOBasic) ResultSize() int { return a.kHeap.Len() }
