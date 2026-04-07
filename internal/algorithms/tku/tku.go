package tku

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"hui-problem/internal/pkg/datastructure"
	"hui-problem/internal/pkg/dbinfo"
	"hui-problem/internal/pkg/memory"
)

// TKU implements the Top-K High Utility Itemsets algorithm (Tseng et al., TKDE 2016).
// Pipeline: pre-evaluation → global UP-tree + DS pruning → UP-Growth (Phase 1 candidates) → sort →
// Phase 2 exact verification (database scan).
type TKU struct {
	theInputFile     string
	theCandidateFile string
	kValue           int
	itemCount        int
	globalMinUtil    float64

	arrayTWUItems []int
	arrayMIU      []int
	arrayMAU      []int

	totalTime    time.Duration
	patternCount int
}

// NewTKU creates a new TKU miner.
func NewTKU() *TKU {
	return &TKU{}
}

// RunAlgorithm executes TKU: Phase 1 candidate generation, sort, Phase 2 verification.
func (a *TKU) RunAlgorithm(inputFile, outputFile string, k int) error {
	memory.Reset()
	memory.Sample()
	start := time.Now()

	// Minimum utility threshold is 0 at the start time.
	a.globalMinUtil = 0

	// Open the database and determine the item id range (max id + 1).
	tool, err := dbinfo.New(inputFile)
	if err != nil {
		return err
	}

	a.kValue = k
	a.theInputFile = inputFile
	a.theCandidateFile = "topKcandidate.txt"
	a.itemCount = tool.GetMaxID() + 1

	// Allocate per-item TWU and min/max item utility (MIU, MAU) arrays.
	a.arrayTWUItems = make([]int, a.itemCount)
	a.arrayMIU = make([]int, a.itemCount)
	a.arrayMAU = make([]int, a.itemCount)

	// Phase 1 — Count TWU -> PE -> build UP-Tree -> NU -> MD -> MC -> write candidate itemsets to topKcandidate.txt
	// The target of Phase 1 is increase the globalMinUtil to hightest possible
	if err := a.runPhase1CandidateGeneration(); err != nil {
		return err
	}

	memory.Sample()

	// Step 10: Sort candidates by estimated utility (descending) for Phase-2 processing order.
	sortedFile := "sortedTopKcandidate.txt"
	if err := a.runSortHUIAlgorithm(a.theCandidateFile, sortedFile); err != nil {
		return err
	}
	_ = os.Remove(a.theCandidateFile)

	memory.Sample()

	// Step 11: Phase 2 — scan the real database to compute exact utility for each candidate and output true top-k HUIs.
	minUtil := int(a.globalMinUtil)
	phase2 := NewPhase2()
	if err := phase2.RunAlgorithm(minUtil, tool.GetDBSize(), k, inputFile, sortedFile, outputFile); err != nil {
		return err
	}
	a.patternCount = phase2.NumberOfTopKHUIs()

	memory.Sample()
	a.totalTime = time.Since(start)
	return nil
}

// runPhase1CandidateGeneration writes Phase-1 candidates to theCandidateFile. The file is closed before RunAlgorithm
// continues to sorting (defer here runs when this function returns, not at the end of RunAlgorithm).
func (a *TKU) runPhase1CandidateGeneration() (err error) {
	cand, err := os.Create(a.theCandidateFile)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(cand)
	defer func() {
		if ferr := bw.Flush(); err == nil {
			err = ferr
		}
		if cerr := cand.Close(); err == nil {
			err = cerr
		}
	}()

	// Step 1: Pre-evaluation — first DB scan: TWU, MIU, MAU, co-occurrence matrix; derive initial global minimum utility threshold.
	gmu, err := a.preEvaluation(a.arrayTWUItems, a.itemCount, a.arrayMIU, a.arrayMAU, a.globalMinUtil, a.kValue)
	if err != nil {
		return err
	}
	a.globalMinUtil = gmu

	// Step 2: Build the global UP-tree from transactions (filter by TWU, order items, insert paths).
	upTree, err := a.buildUPTree(a.arrayTWUItems)
	if err != nil {
		return err
	}

	// Step 6: Descendant-sum (DS) pruning — tighten globalMinUtil using MIU × descendant counts from each root child subtree.
	dsHeap := datastructure.NewIntRedBlackTree()
	for i := 0; i < len(upTree.Root.Children); i++ {
		sumDS := make([]int, a.itemCount)
		dsItem := upTree.Root.Children[i].Item
		upTree.SumDescendent(upTree.Root.Children[i], sumDS)
		for j := 0; j < len(sumDS); j++ {
			if sumDS[j] != 0 && j != dsItem {
				dsVal := (a.arrayMIU[j] + a.arrayMIU[dsItem]) * sumDS[j]
				a.updateNodeCountHeap(dsHeap, dsVal)
			}
		}
	}

	// Step 7: Prepare item order for UP-Growth (TWU-descending list of promising items).
	isHeap := datastructure.NewIntRedBlackTree()
	ulist := a.getUlist(a.arrayTWUItems)

	// Step 8: UP-Growth on the global tree — recursively mine conditional pattern bases and emit candidate itemsets with estimated utilities.
	if err = upTree.UPGrowth(upTree, ulist, "", bw, isHeap, a.arrayTWUItems); err != nil {
		return err
	}
	// Step 9: Append single items whose TWU meets the current threshold as length-1 candidates.
	for i := 0; i < len(a.arrayTWUItems); i++ {
		if float64(a.arrayTWUItems[i]) >= a.globalMinUtil {
			if _, werr := bw.WriteString(strconv.Itoa(i) + ":" + strconv.Itoa(a.arrayTWUItems[i]) + "\n"); werr != nil {
				err = werr
				return err
			}
		}
	}
	return nil
}

// PrintStats prints execution statistics (SPMF-style).
func (a *TKU) PrintStats() {
	fmt.Println("=============  TKU (Go)  =============")
	fmt.Printf(" Total execution time : %.2f seconds\n", a.totalTime.Seconds())
	fmt.Println(" Number of top-k high utility patterns :", a.patternCount)
	fmt.Printf(" Max memory usage (approx): %.2f MB\n", memory.MaxMB())
	fmt.Println("===================================================")
}

// runSortHUIAlgorithm reads candidate lines "pattern:estUtil" and rewrites them in descending order of estimated utility.
func (a *TKU) runSortHUIAlgorithm(candidateFile, sortedFile string) error {
	in, err := os.Open(candidateFile)
	if err != nil {
		return err
	}
	defer in.Close()

	h := datastructure.NewStringPairRedBlackTree()
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		u, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		h.Add(datastructure.StringPair{X: parts[0], Y: u})
	}
	if err := sc.Err(); err != nil {
		return err
	}

	out, err := os.Create(sortedFile)
	if err != nil {
		return err
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	n := h.Size()
	for i := 0; i < n; i++ {
		max := h.Maximum()
		if _, err := w.WriteString(max.X + ":" + strconv.Itoa(max.Y) + "\n"); err != nil {
			return err
		}
		h.PopMaximum()
	}
	return w.Flush()
}

// preEvaluation performs the first database scan: accumulate TWU per item, min/max item utility (MIU, MAU),
// fill the triangular co-occurrence matrix for pair utilities, then derive the initial k-based utility floor via getInitialUtility.
func (a *TKU) preEvaluation(itemTWUs []int, numItem int, minBNF, maxBNF []int, miniUtility float64, topK int) (float64, error) {
	tm := datastructure.NewTriangularMatrix(numItem)

	sc, cancel := dbinfo.Scanner(a.theInputFile)
	defer cancel()

	for sc.Scan() {
		lineStr := strings.TrimSpace(sc.Text())
		if lineStr == "" {
			continue
		}
		transaction := strings.Split(lineStr, ":")
		if len(transaction) < 3 {
			continue
		}
		items := strings.Fields(transaction[0])
		twu, _ := strconv.Atoi(strings.TrimSpace(transaction[1]))
		itemUtilities := strings.Fields(transaction[2])
		if len(items) == 0 || len(itemUtilities) != len(items) {
			continue
		}
		firstItemID, _ := strconv.Atoi(items[0])
		firstItemUtil, _ := strconv.Atoi(itemUtilities[0])

		for s := 0; s < len(items); s++ {
			itemID, _ := strconv.Atoi(items[s])
			itemUtil, _ := strconv.Atoi(itemUtilities[s])

			if minBNF[itemID] == 0 {
				if itemUtil > 0 {
					minBNF[itemID] = itemUtil
				}
			} else if minBNF[itemID] > itemUtil {
				minBNF[itemID] = itemUtil
			}

			if maxBNF[itemID] < itemUtil {
				maxBNF[itemID] = itemUtil
			}

			itemTWUs[itemID] += twu

			if s > 0 {
				tm.IncrementCount(firstItemID, itemID, firstItemUtil+itemUtil)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return miniUtility, err
	}

	initial := a.getInitialUtility(tm, numItem, topK)
	return initial, nil
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

// getInitialUtility takes the k largest non-zero values from the triangular matrix (pair-utility aggregates) and returns the k-th as the starting threshold.
func (a *TKU) getInitialUtility(tm *datastructure.TriangularMatrix, nItem, k int) float64 {
	minHeap := &intMinHeap{}
	heap.Init(minHeap)

	for i := 0; i < nItem; i++ {
		row := tm.Matrix[i]
		for j := 0; j < len(row); j++ {
			v := row[j]
			if v == 0 {
				continue
			}
			if minHeap.Len() < k {
				heap.Push(minHeap, v)
			} else if minHeap.Len() >= k && v > (*minHeap)[0] {
				heap.Push(minHeap, v)
				heap.Pop(minHeap)
			}
		}
	}
	if minHeap.Len() == 0 {
		return 0
	}
	return float64((*minHeap)[0])
}

// getUlist builds the list of item ids with positive TWU at or above globalMinUtil, ordered by descending TWU (TKU item order).
func (a *TKU) getUlist(twuByItem []int) []int {
	list := make([]int, 0)
	for i := 0; i < len(twuByItem); i++ {
		if twuByItem[i] > 0 && float64(twuByItem[i]) >= a.globalMinUtil {
			a.insertItem(&list, i, twuByItem)
		}
	}
	return list
}

// insertItem inserts target into list keeping descending order by twuByItem (TWU), then by item id for ties.
func (a *TKU) insertItem(list *[]int, target int, twuByItem []int) {
	if len(*list) == 0 {
		*list = append(*list, target)
		return
	}
	for i := 0; i < len(*list); i++ {
		if twuByItem[target] > twuByItem[(*list)[i]] {
			*list = append((*list)[:i], append([]int{target}, (*list)[i:]...)...)
			return
		}
		if twuByItem[target] == twuByItem[(*list)[i]] && target < (*list)[i] {
			*list = append((*list)[:i], append([]int{target}, (*list)[i:]...)...)
			return
		}
		if i == len(*list)-1 {
			*list = append(*list, target)
			return
		}
	}
}

// sortItemsByDescendingTWU bubble-sorts itemIDs[start:length] by descending TWU (twuByItem), tie-break smaller id first (UP-tree sibling order).
func (a *TKU) sortItemsByDescendingTWU(itemIDs []int, start, length int, twuByItem []int) {
	for i := start; i < length-1; i++ {
		for j := start; j < length-1; j++ {
			if twuByItem[itemIDs[j]] < twuByItem[itemIDs[j+1]] {
				itemIDs[j], itemIDs[j+1] = itemIDs[j+1], itemIDs[j]
			} else if twuByItem[itemIDs[j]] == twuByItem[itemIDs[j+1]] && itemIDs[j] > itemIDs[j+1] {
				itemIDs[j], itemIDs[j+1] = itemIDs[j+1], itemIDs[j]
			}
		}
	}
}

// sortItemsAndUtilitiesByDescendingTWU is sortItemsByDescendingTWU but also permutes utilityStrs in parallel with itemIDs.
func (a *TKU) sortItemsAndUtilitiesByDescendingTWU(itemIDs []int, utilityStrs []string, twuByItem []int) {
	if len(itemIDs) != len(utilityStrs) {
		panic("itemIDs and utilityStrs must have the same length")
	}
	for i := 0; i < len(itemIDs)-1; i++ {
		for j := 0; j < len(itemIDs)-1; j++ {
			if twuByItem[itemIDs[j]] < twuByItem[itemIDs[j+1]] {
				itemIDs[j], itemIDs[j+1] = itemIDs[j+1], itemIDs[j]
				utilityStrs[j], utilityStrs[j+1] = utilityStrs[j+1], utilityStrs[j]
			} else if twuByItem[itemIDs[j]] == twuByItem[itemIDs[j+1]] && itemIDs[j] > itemIDs[j+1] {
				itemIDs[j], itemIDs[j+1] = itemIDs[j+1], itemIDs[j]
				utilityStrs[j], utilityStrs[j+1] = utilityStrs[j+1], utilityStrs[j]
			}
		}
	}
}

// updateNodeCountHeap maintains a size-k structure of utility lower bounds and raises globalMinUtil when the k-th best bound increases.
func (a *TKU) updateNodeCountHeap(nch *datastructure.RedBlackTree[int], newValue int) {
	if nch.Size() < a.kValue {
		nch.Add(newValue)
	} else if nch.Size() >= a.kValue {
		if float64(newValue) > a.globalMinUtil {
			nch.Add(newValue)
			nch.PopMinimum()
		}
	}
	if nch.Size() >= a.kValue {
		minV := nch.Minimum()
		if float64(minV) > a.globalMinUtil {
			a.globalMinUtil = float64(minV)
		}
	}
}

// buildUPTree second-scan: for each transaction, keep items with TWU ≥ globalMinUtil, sort by TWU order, insert into the UP-tree.
func (a *TKU) buildUPTree(itemTWU []int) (*FPTree, error) {
	nodeCountHeap := datastructure.NewIntRedBlackTree()
	tree := NewFPTree(a)

	sc, cancel := dbinfo.Scanner(a.theInputFile)
	defer cancel()

	for sc.Scan() {
		transLineStr := strings.TrimSpace(sc.Text())
		if transLineStr == "" {
			continue
		}
		transaction := strings.Split(transLineStr, ":")
		if len(transaction) < 3 {
			continue
		}
		idTokens := strings.Fields(transaction[0])
		itemUtils := strings.Fields(transaction[2])
		if len(idTokens) != len(itemUtils) {
			continue
		}
		nTok := len(idTokens)
		filteredItems := make([]int, 0, nTok)
		utilityStrs := make([]string, 0, nTok)
		for m := 0; m < nTok; m++ {
			id, _ := strconv.Atoi(idTokens[m])
			if float64(itemTWU[id]) >= a.globalMinUtil {
				filteredItems = append(filteredItems, id)
				utilityStrs = append(utilityStrs, itemUtils[m])
			}
		}
		a.sortItemsAndUtilitiesByDescendingTWU(filteredItems, utilityStrs, itemTWU)
		tree.insertGlobalTransactionIntoUPTree(filteredItems, utilityStrs, len(filteredItems), 1, itemTWU, nodeCountHeap)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	memory.Sample()
	return tree, nil
}
