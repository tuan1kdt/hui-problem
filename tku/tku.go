package tku

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// TKU implements the Top-K High Utility Itemsets algorithm (Tseng et al., TKDE 2016).
type TKU struct {
	theInputFile     string
	theCandidateFile string
	kValue           int
	itemCount        int
	globalMinUtil    float64

	arrayTWUItems []int
	arrayMIU      []int
	arrayMAU      []int

	totalTime    float64
	patternCount int
	maxMemoryMB  float64
}

// NewTKU creates a new TKU miner.
func NewTKU() *TKU {
	return &TKU{}
}

// RunAlgorithm executes TKU: Phase 1 candidate generation, sort, Phase 2 verification.
func (a *TKU) RunAlgorithm(inputFile, outputFile string, k int) error {
	a.maxMemoryMB = 0
	a.sampleMemory()

	start := time.Now()
	a.globalMinUtil = 0

	tool := NewDatabaseInfo(inputFile)
	if err := tool.RunCalculate(); err != nil {
		return err
	}

	a.kValue = k
	a.theInputFile = inputFile
	a.theCandidateFile = "topKcandidate.txt"
	a.itemCount = tool.GetMaxID() + 1

	a.arrayTWUItems = make([]int, a.itemCount)
	a.arrayMIU = make([]int, a.itemCount)
	a.arrayMAU = make([]int, a.itemCount)

	cand, err := os.Create(a.theCandidateFile)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(cand)

	gmu, err := a.preEvaluation(a.theInputFile, a.arrayTWUItems, a.itemCount, a.arrayMIU, a.arrayMAU, a.globalMinUtil, a.kValue)
	if err != nil {
		_ = cand.Close()
		return err
	}
	a.globalMinUtil = gmu

	tree, err := a.buildUPTree(a.arrayTWUItems, a.theInputFile)
	if err != nil {
		_ = cand.Close()
		return err
	}

	dsHeap := NewIntRedBlackTree()
	for i := 0; i < len(tree.Root.Children); i++ {
		sumDS := make([]int, a.itemCount)
		dsItem := tree.Root.Children[i].Item
		tree.SumDescendent(tree.Root.Children[i], sumDS)
		for j := 0; j < len(sumDS); j++ {
			if sumDS[j] != 0 && j != dsItem {
				dsVal := (a.arrayMIU[j] + a.arrayMIU[dsItem]) * sumDS[j]
				a.updateNodeCountHeap(dsHeap, dsVal)
			}
		}
	}

	isHeap := NewIntRedBlackTree()
	ulist := a.getUlist(a.arrayTWUItems)

	if err := tree.UPGrowth(tree, ulist, "", bw, isHeap, a.arrayTWUItems); err != nil {
		_ = cand.Close()
		return err
	}
	for i := 0; i < len(a.arrayTWUItems); i++ {
		if float64(a.arrayTWUItems[i]) >= a.globalMinUtil {
			if _, err := bw.WriteString(strconv.Itoa(i) + ":" + strconv.Itoa(a.arrayTWUItems[i]) + "\n"); err != nil {
				_ = cand.Close()
				return err
			}
		}
	}
	if err := bw.Flush(); err != nil {
		_ = cand.Close()
		return err
	}
	if err := cand.Close(); err != nil {
		return err
	}

	a.sampleMemory()

	sortedFile := "sortedTopKcandidate.txt"
	if err := a.runSortHUIAlgorithm(a.theCandidateFile, sortedFile); err != nil {
		return err
	}
	_ = os.Remove(a.theCandidateFile)

	a.sampleMemory()

	minUtil := int(a.globalMinUtil)
	phase2 := NewPhase2()
	if err := phase2.RunAlgorithm(minUtil, tool.GetDBSize(), k, inputFile, sortedFile, outputFile); err != nil {
		return err
	}
	a.patternCount = phase2.NumberOfTopKHUIs()

	a.sampleMemory()
	a.totalTime = time.Since(start).Seconds()
	return nil
}

func (a *TKU) sampleMemory() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	used := float64(ms.Alloc) / (1024 * 1024)
	if used > a.maxMemoryMB {
		a.maxMemoryMB = used
	}
}

// PrintStats prints execution statistics (SPMF-style).
func (a *TKU) PrintStats() {
	fmt.Println("=============  TKU (Go)  =============")
	fmt.Println(" Total execution time :", a.totalTime, "s")
	fmt.Println(" Number of top-k high utility patterns :", a.patternCount)
	fmt.Println(" Max memory usage (approx) :", a.maxMemoryMB, "MB")
	fmt.Println("===================================================")
}

func (a *TKU) runSortHUIAlgorithm(candidateFile, sortedFile string) error {
	in, err := os.Open(candidateFile)
	if err != nil {
		return err
	}
	defer in.Close()

	h := NewStringPairRedBlackTree()
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
		h.Add(StringPair{X: parts[0], Y: u})
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

func (a *TKU) preEvaluation(hdb string, twu1 []int, numItem int, minBNF, maxBNF []int, miniUtility float64, pK int) (float64, error) {
	tm := NewTriangularMatrix(numItem)

	f, err := os.Open(hdb)
	if err != nil {
		return miniUtility, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		transaction := strings.TrimSpace(sc.Text())
		if transaction == "" {
			continue
		}
		temp1 := strings.Split(transaction, ":")
		if len(temp1) < 3 {
			continue
		}
		temp2 := strings.Fields(temp1[0])
		twuVal, _ := strconv.Atoi(strings.TrimSpace(temp1[1]))
		temp3 := strings.Fields(temp1[2])
		if len(temp2) == 0 || len(temp3) != len(temp2) {
			continue
		}
		firstItem, _ := strconv.Atoi(temp2[0])
		u0, _ := strconv.Atoi(temp3[0])

		for s := 0; s < len(temp2); s++ {
			itemID, _ := strconv.Atoi(temp2[s])
			util, _ := strconv.Atoi(temp3[s])

			if minBNF[itemID] == 0 {
				if util > 0 {
					minBNF[itemID] = util
				}
			} else if minBNF[itemID] > util {
				minBNF[itemID] = util
			}

			if maxBNF[itemID] < util {
				maxBNF[itemID] = util
			}

			twu1[itemID] += twuVal

			if s > 0 {
				tm.IncrementCount(firstItem, itemID, u0+util)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return miniUtility, err
	}

	initial := a.getInitialUtility(tm, numItem, pK)
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

func (a *TKU) getInitialUtility(tm *TriangularMatrix, nItem, k int) float64 {
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

func (a *TKU) getUlist(p1 []int) []int {
	list := make([]int, 0)
	for i := 0; i < len(p1); i++ {
		if p1[i] > 0 && float64(p1[i]) >= a.globalMinUtil {
			a.insertItem(&list, i, p1)
		}
	}
	return list
}

func (a *TKU) insertItem(list *[]int, target int, order []int) {
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

func (a *TKU) sortTrans(tran []int, pre, tranlen int, p1 []int) {
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

func (a *TKU) sortTrans2(tran []int, bran []string, pre, tranlen int, p1 []int) {
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

func (a *TKU) updateNodeCountHeap(nch *RedBlackTree[int], newValue int) {
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

func (a *TKU) buildUPTree(p1 []int, hdb string) (*FPTree, error) {
	nodeCountHeap := NewIntRedBlackTree()
	tree := NewFPTree(a)

	f, err := os.Open(hdb)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		transaction := strings.TrimSpace(sc.Text())
		if transaction == "" {
			continue
		}
		temp1 := strings.Split(transaction, ":")
		if len(temp1) < 3 {
			continue
		}
		temp2 := strings.Fields(temp1[0])
		bran := strings.Fields(temp1[2])
		if len(temp2) != len(bran) {
			continue
		}
		tran := make([]int, len(temp2))
		bran2 := make([]string, len(bran))
		tranlen := 0
		for m := 0; m < len(temp2); m++ {
			id, _ := strconv.Atoi(temp2[m])
			if float64(p1[id]) >= a.globalMinUtil {
				bran2[tranlen] = bran[m]
				tran[tranlen] = id
				tranlen++
			}
		}
		a.sortTrans2(tran, bran2, 0, tranlen, p1)
		tree.instrans3(tran, bran2, tranlen, 1, p1, nodeCountHeap)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	a.sampleMemory()
	return tree, nil
}
