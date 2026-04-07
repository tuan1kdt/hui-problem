// Package ptku implements PTKU (Parallel TKU): a parallel variant of the two-phase Top-K
// High Utility Itemset algorithm (Tseng et al., TKDE 2016). Phase 1 uses a UP-Tree and
// threshold-raising strategies (PE, NU via tree nodes, MD, MC); Phase 2 verifies candidates
// with exact utilities. Parallelism follows MapReduce-style scans and Fork/Join on independent
// UPGrowth branches (see report/main.tex, Section 5.2).
package ptku

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
	"time"
)

// parsedTx is one parsed line of the utility database (SPMF: items : TU : utilities).
// Kept in memory so Phase 1 can be scanned in parallel without re-reading the file.
type parsedTx struct {
	items []int // item ids in transaction order
	utils []int // per-item utility in same order
	tu    int   // transaction utility (field after first ':')
}

// PTKU holds all Phase-1 state and the shared thread-safe utility border (minUtil).
type PTKU struct {
	inputFile string
	kValue    int
	itemCount int // maxItemID + 1; length of TWU/MIU/MAU slices

	// Per-item aggregates from the first logical DB pass (merged from worker partials).
	arrayTWUItems []int // TWU: sum of TU over transactions containing the item
	arrayMIU      []int // minimum item utility per item (for MIU / MD / MC bounds)
	arrayMAU      []int // maximum item utility per item (for MAU upper bounds)

	// Shared minUtil border and top-k multiset logic (safe across ParallelUPGrowth goroutines).
	threshold *SafeHeap
	Workers   int // goroutine count for parallel pre-eval, phase-2 partitions, and branch fan-out

	totalTime    float64
	patternCount int
	maxMemoryMB  float64
}

// NewPTKU creates a new PTKU miner with default worker count = NumCPU.
func NewPTKU() *PTKU {
	return &PTKU{Workers: runtime.NumCPU()}
}

// RunAlgorithm executes the full PTKU pipeline (Phase 1, then sort, then Phase 2).
//
// Step 0: DatabaseInfo — max item id and transaction count.
// Step 1: loadAllTransactions — parse file into []parsedTx for parallel Phase 1.
// Step 2: parallelPreEvaluation — MapReduce TWU/MIU/MAU and PE matrix; merge; PE initial minUtil.
// Step 3: NewSafeHeap + SetMinUtil — shared border for all strategies.
// Step 4: buildUPTree — filter/sort txs, insert global UP-Tree; NU raises via nodeCountHeap.
// Step 5: MD — descendant MIU bounds from each root child.
// Step 6: getUlist — ordered frequent items for mining.
// Step 7: ParallelUPGrowth — Fork/Join per root header item; MC; candidates → sink.
// Step 8: append singleton itemsets (TWU as estimate).
// Step 9: sort candidates by descending estimated utility (SE order for Phase 2).
// Step 10: Phase2.RunAlgorithm — load hdb/bnf; parallel exact scan per candidate; write output.
func (a *PTKU) RunAlgorithm(inputFile, outputFile string, k int) error {
	if a.Workers < 1 {
		a.Workers = 1
	}

	a.maxMemoryMB = 0
	a.sampleMemory()
	start := time.Now()

	// Step 0: one pass for |D|, max item id (line count includes blanks in DBSize like TKU).
	tool := NewDatabaseInfo(inputFile)
	if err := tool.RunCalculate(); err != nil {
		return err
	}

	a.kValue = k
	a.inputFile = inputFile
	a.itemCount = tool.GetMaxID() + 1

	a.arrayTWUItems = make([]int, a.itemCount)
	a.arrayMIU = make([]int, a.itemCount)
	a.arrayMAU = make([]int, a.itemCount)

	// Step 1: materialize transactions for parallel map phase (memory vs TKU streaming tradeoff).
	txs, err := loadAllTransactions(inputFile)
	if err != nil {
		return err
	}

	// Step 2: parallel first-scan statistics + PE triangular matrix; returns PE initial border.
	gmu, err := a.parallelPreEvaluation(txs, a.arrayTWUItems, a.arrayMIU, a.arrayMAU, a.itemCount, a.kValue)
	if err != nil {
		return err
	}

	// Step 3: install shared threshold (starts at 0, then lifted by PE and all later strategies).
	a.threshold = NewSafeHeap(a.kValue, 0)
	a.threshold.SetMinUtil(gmu)

	// Step 4: compress revised transactions into the global UP-Tree (sequential insertion).
	tree, err := a.buildUPTree(a.arrayTWUItems, txs)
	if err != nil {
		return err
	}

	// Step 5: MD — MIU of descendants under each root child branch.
	dsHeap := NewIntRedBlackTree()
	for i := 0; i < len(tree.Root.Children); i++ {
		sumDS := make([]int, a.itemCount)
		dsItem := tree.Root.Children[i].Item
		tree.SumDescendent(tree.Root.Children[i], sumDS)
		for j := 0; j < len(sumDS); j++ {
			if sumDS[j] != 0 && j != dsItem {
				dsVal := (a.arrayMIU[j] + a.arrayMIU[dsItem]) * sumDS[j]
				a.threshold.TryUpdateWithHeap(dsHeap, dsVal)
			}
		}
	}

	// Step 6–7: mine extensions in parallel at the root; each branch has its own MC heap.
	ulist := a.getUlist(a.arrayTWUItems)
	sink := &candidateSink{}
	if err := tree.ParallelUPGrowth(tree, ulist, "", a.arrayTWUItems, sink); err != nil {
		return err
	}

	// Step 8: 1-itemsets as candidates (TWU as estimated utility, same as TKU).
	for i := 0; i < len(a.arrayTWUItems); i++ {
		if float64(a.arrayTWUItems[i]) >= a.threshold.MinUtil() {
			sink.add(StringPair{X: strconv.Itoa(i), Y: a.arrayTWUItems[i]})
		}
	}

	// Step 9: SE — verify stronger candidates first in Phase 2.
	candidates := sink.items
	sort.Slice(candidates, func(i, j int) bool {
		return CompareStringPair(candidates[i], candidates[j]) > 0
	})

	a.sampleMemory()

	// Step 10: exact utilities; parallel per-candidate transaction partitioning inside Phase2.
	minUtil := int(a.threshold.MinUtil())
	phase2 := NewPhase2(a.Workers)
	if err := phase2.RunAlgorithm(minUtil, tool.GetDBSize(), k, inputFile, candidates, outputFile); err != nil {
		return err
	}
	a.patternCount = phase2.NumberOfTopKHUIs()

	a.sampleMemory()
	a.totalTime = time.Since(start).Seconds()
	return nil
}

func (a *PTKU) sampleMemory() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	used := float64(ms.Alloc) / (1024 * 1024)
	if used > a.maxMemoryMB {
		a.maxMemoryMB = used
	}
}

// PrintStats prints execution statistics (SPMF-style).
func (a *PTKU) PrintStats() {
	fmt.Println("=============  PTKU (Go)  =============")
	fmt.Println(" Total execution time :", a.totalTime, "s")
	fmt.Println(" Number of top-k high utility patterns :", a.patternCount)
	fmt.Println(" Max memory usage (approx) :", a.maxMemoryMB, "MB")
	fmt.Println(" Workers :", a.Workers)
	fmt.Println("===================================================")
}

// loadAllTransactions reads the full utility database and parses each valid line into parsedTx.
// Used once so parallel workers can scan slices without contending on a file handle.
func loadAllTransactions(path string) ([]parsedTx, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []parsedTx
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		itemStrs := strings.Fields(parts[0])
		tu, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		utilStrs := strings.Fields(parts[2])
		if len(itemStrs) == 0 || len(utilStrs) != len(itemStrs) {
			continue
		}
		tx := parsedTx{tu: tu, items: make([]int, len(itemStrs)), utils: make([]int, len(itemStrs))}
		for i := range itemStrs {
			tx.items[i], _ = strconv.Atoi(itemStrs[i])
			tx.utils[i], _ = strconv.Atoi(utilStrs[i])
		}
		out = append(out, tx)
	}
	return out, sc.Err()
}

// parallelPreEvaluation is the MapReduce-style first pass (paper: parallel TWU / PE).
//
// Map: each worker scans a disjoint chunk of txs and builds local twu[], miu[], mau[], and a local
// TriangularMatrix for PE (first item bonded with each later item: u0+u_s).
//
// Reduce: sum TWU; element-wise min MIU and max MAU; add matrix cells; then getInitialUtility
// returns the k-th largest PE aggregate as the initial minUtil (strategy PE).
func (a *PTKU) parallelPreEvaluation(
	txs []parsedTx,
	twuOut []int,
	minBNF []int,
	maxBNF []int,
	numItem, pK int,
) (float64, error) {
	n := len(txs)
	if n == 0 {
		return 0, nil
	}

	w := a.Workers
	if w > n {
		w = n
	}

	chunk := (n + w - 1) / w
	type partial struct {
		twu []int
		miu []int
		mau []int
		tm  *TriangularMatrix
		err error
	}
	partials := make([]partial, w)

	// Map phase: one goroutine per chunk.
	var wg sync.WaitGroup
	for wi := 0; wi < w; wi++ {
		start := wi * chunk
		end := start + chunk
		if end > n {
			end = n
		}
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(start, end, idx int) {
			defer wg.Done()
			twu := make([]int, numItem)
			miu := make([]int, numItem)
			mau := make([]int, numItem)
			tm := NewTriangularMatrix(numItem)
			for _, tx := range txs[start:end] {
				if len(tx.items) == 0 {
					continue
				}
				firstItem := tx.items[0]
				u0 := tx.utils[0]
				for s := 0; s < len(tx.items); s++ {
					itemID := tx.items[s]
					util := tx.utils[s]

					if miu[itemID] == 0 {
						if util > 0 {
							miu[itemID] = util
						}
					} else if miu[itemID] > util {
						miu[itemID] = util
					}

					if mau[itemID] < util {
						mau[itemID] = util
					}

					twu[itemID] += tx.tu

					if s > 0 {
						tm.IncrementCount(firstItem, itemID, u0+util)
					}
				}
			}
			partials[idx].twu = twu
			partials[idx].miu = miu
			partials[idx].mau = mau
			partials[idx].tm = tm
		}(start, end, wi)
	}
	wg.Wait()

	// Reduce: fold partial TWU/MIU/MAU into the output slices (global aggregates).
	for pi := range partials {
		if partials[pi].twu == nil {
			continue
		}
		if partials[pi].err != nil {
			return 0, partials[pi].err
		}
		for i := 0; i < numItem; i++ {
			twuOut[i] += partials[pi].twu[i]
			mergeMIU(minBNF, partials[pi].miu, i)
			mergeMAU(maxBNF, partials[pi].mau, i)
		}
	}

	// Reduce: sum PE matrix cells from all workers.
	tmMerged := NewTriangularMatrix(numItem)
	for pi := range partials {
		if partials[pi].tm == nil {
			continue
		}
		mergeTriangular(tmMerged, partials[pi].tm, numItem)
	}

	// PE: k-th largest value in the matrix becomes a safe lower bound to start minUtil.
	return a.getInitialUtility(tmMerged, numItem, pK), nil
}

// mergeMIU combines per-partition minima; 0 means "unset" in a partition for that item.
func mergeMIU(dst, src []int, i int) {
	sv := src[i]
	if sv == 0 {
		return
	}
	if dst[i] == 0 {
		dst[i] = sv
		return
	}
	if sv < dst[i] {
		dst[i] = sv
	}
}

// mergeMAU takes the element-wise maximum MAU across partitions.
func mergeMAU(dst, src []int, i int) {
	if src[i] > dst[i] {
		dst[i] = src[i]
	}
}

// mergeTriangular adds src's PE counts into dst (same dimensions).
func mergeTriangular(dst, src *TriangularMatrix, nItem int) {
	for i := 0; i < nItem; i++ {
		for j := 0; j < len(dst.Matrix[i]); j++ {
			dst.Matrix[i][j] += src.Matrix[i][j]
		}
	}
}

type intMinHeap []int

func (h intMinHeap) Len() int           { return len(h) }
func (h intMinHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *intMinHeap) Push(x any) { *h = append(*h, x.(int)) }

func (h *intMinHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[0 : n-1]
	return v
}

// getInitialUtility finds the k-th largest non-zero entry in the PE matrix (min-heap of size k).
func (a *PTKU) getInitialUtility(tm *TriangularMatrix, nItem, k int) float64 {
	h := &intMinHeap{}
	heap.Init(h)

	for i := 0; i < nItem; i++ {
		row := tm.Matrix[i]
		for j := 0; j < len(row); j++ {
			v := row[j]
			if v == 0 {
				continue
			}
			if h.Len() < k {
				heap.Push(h, v)
			} else if h.Len() >= k && v > (*h)[0] {
				heap.Push(h, v)
				heap.Pop(h)
			}
		}
	}
	if h.Len() == 0 {
		return 0
	}
	return float64((*h)[0])
}

// getUlist returns items eligible for UPGrowth: positive TWU and TWU ≥ current minUtil border,
// sorted by descending TWU (tie-break by item id) via insertItem.
func (a *PTKU) getUlist(p1 []int) []int {
	list := make([]int, 0)
	gmu := a.threshold.MinUtil()
	for i := 0; i < len(p1); i++ {
		if p1[i] > 0 && float64(p1[i]) >= gmu {
			a.insertItem(&list, i, p1)
		}
	}
	return list
}

// insertItem maintains list in descending order by order[] (TWU), then ascending id tie-break.
func (a *PTKU) insertItem(list *[]int, target int, order []int) {
	if len(*list) == 0 {
		*list = append(*list, target)
		return
	}
	for i := 0; i < len(*list); i++ {
		if order[target] > order[(*list)[i]] {
			*list = append((*list)[:i], append([]int{target}, (*list)[i:]...)...)
			return
		}
		if order[target] == order[(*list)[i]] && target < (*list)[i] {
			*list = append((*list)[:i], append([]int{target}, (*list)[i:]...)...)
			return
		}
		if i == len(*list)-1 {
			*list = append(*list, target)
			return
		}
	}
}

// sortTrans bubble-sorts item ids in tran[pre:tranlen] by descending TWU p1[] (SPMF TKU order).
func (a *PTKU) sortTrans(tran []int, pre, tranlen int, p1 []int) {
	for i := pre; i < tranlen-1; i++ {
		for j := pre; j < tranlen-1; j++ {
			if p1[tran[j]] < p1[tran[j+1]] {
				tran[j], tran[j+1] = tran[j+1], tran[j]
			} else if p1[tran[j]] == p1[tran[j+1]] && tran[j] > tran[j+1] {
				tran[j], tran[j+1] = tran[j+1], tran[j]
			}
		}
	}
}

// sortTrans2 same as sortTrans but permutes parallel utility strings bran alongside tran.
func (a *PTKU) sortTrans2(tran []int, bran []string, pre, tranlen int, p1 []int) {
	for i := pre; i < tranlen-1; i++ {
		for j := pre; j < tranlen-1; j++ {
			if p1[tran[j]] < p1[tran[j+1]] {
				tran[j], tran[j+1] = tran[j+1], tran[j]
				bran[j], bran[j+1] = bran[j+1], bran[j]
			} else if p1[tran[j]] == p1[tran[j+1]] && tran[j] > tran[j+1] {
				tran[j], tran[j+1] = tran[j+1], tran[j]
				bran[j], bran[j+1] = bran[j+1], bran[j]
			}
		}
	}
}

// buildUPTree performs TKU's second database pass: drop items with TWU below border, sort each
// transaction by global TWU order, insert into the UP-Tree (instrans3), and raise border via NU
// using nodeCountHeap + TryUpdateWithHeap. Runs sequentially on the shared tree structure.
func (a *PTKU) buildUPTree(p1 []int, txs []parsedTx) (*FPTree, error) {
	nodeCountHeap := NewIntRedBlackTree()
	tree := NewFPTree(a)

	for _, tx := range txs {
		tran := make([]int, len(tx.items))
		bran2 := make([]string, len(tx.utils))
		tranlen := 0
		gmu := a.threshold.MinUtil()
		for m := 0; m < len(tx.items); m++ {
			id := tx.items[m]
			if float64(p1[id]) >= gmu {
				bran2[tranlen] = strconv.Itoa(tx.utils[m])
				tran[tranlen] = id
				tranlen++
			}
		}
		a.sortTrans2(tran, bran2, 0, tranlen, p1)
		tree.instrans3(tran, bran2, tranlen, 1, p1, nodeCountHeap)
	}

	a.sampleMemory()
	return tree, nil
}
