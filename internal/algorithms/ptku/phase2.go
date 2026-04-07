package ptku

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"

	"hui-problem/internal/pkg/datastructure"
)

// Phase2 runs TKU Phase 2: verify Phase-1 candidates against the original DB for exact EU(X).
//
// SE: candidates must be passed in descending estimated utility (caller sorts). Early skip when
// estimate < minUtility. Top-k heap on (itemset, exact utility) raises minUtility during verification.
//
// Parallelism: for each candidate, parallelExactUtility partitions transaction indices across workers
// and sums partial utilities (MapReduce over rows of hdb/bnf).
type Phase2 struct {
	workers              int
	minUtility           int
	theCurrentK          int
	numberOfTransactions int
	inputFilePath        string
	temporaryFilePath    string
	outputFilePath       string
	numTopKHUI           int
}

// NewPhase2 creates a Phase2 runner with the given worker count for DB partitioning.
func NewPhase2(workers int) *Phase2 {
	if workers < 1 {
		workers = 1
	}
	return &Phase2{temporaryFilePath: "HUI_ptku.txt", workers: workers}
}

// RunAlgorithm loads the database, verifies each candidate, filters by final border, writes output.
//
// Step A: allocate hdb/bnf; Step B: readDatabase; Step C: readCandidateItemsetsParallel (SE order,
// parallel exact sum per candidate, temp HUI file, top-k heap); Step D: format final SPMF lines.
func (p *Phase2) RunAlgorithm(minUtil, transactionCount, currentK int, inputPath string, candidates []datastructure.StringPair, outputFile string) error {
	p.minUtility = minUtil
	p.numberOfTransactions = transactionCount
	p.theCurrentK = currentK
	p.inputFilePath = inputPath
	p.outputFilePath = outputFile

	tmp, err := os.Create(p.temporaryFilePath)
	if err != nil {
		return err
	}
	bfw := bufio.NewWriter(tmp)

	// Step A–B: materialize DB in RAM for repeated candidate checks (same as TKU phase 2).
	hdb := make([][]int, p.numberOfTransactions)
	bnf := make([][]int, p.numberOfTransactions)
	p.initialization(hdb, bnf)

	if err := readDatabase(hdb, bnf, p.inputFilePath); err != nil {
		_ = tmp.Close()
		return err
	}

	// Step C: verify candidates (parallel inner loop per candidate).
	if _, err := p.readCandidateItemsetsParallel(hdb, bnf, candidates, bfw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := bfw.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Step D: read back HUI lines and apply final minUtility filter for output formatting.
	in, err := os.Open(p.temporaryFilePath)
	if err != nil {
		return err
	}
	defer in.Close()
	bfr := bufio.NewScanner(in)

	out, err := os.Create(p.outputFilePath)
	if err != nil {
		return err
	}
	defer out.Close()
	bfwOut := bufio.NewWriter(out)

	p.numTopKHUI = 0
	for bfr.Scan() {
		record := strings.TrimSpace(bfr.Text())
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, ":", 2)
		if len(parts) != 2 {
			continue
		}
		u, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		if u >= p.minUtility {
			line := strings.Replace(record, ":", " #UTIL: ", 1)
			if _, err := bfwOut.WriteString(line + "\n"); err != nil {
				return err
			}
			p.numTopKHUI++
		}
	}
	if err := bfr.Err(); err != nil {
		return err
	}
	if err := bfwOut.Flush(); err != nil {
		return err
	}

	_ = os.Remove(p.temporaryFilePath)
	return nil
}

// NumberOfTopKHUIs returns how many lines were written to the output file.
func (p *Phase2) NumberOfTopKHUIs() int {
	return p.numTopKHUI
}

// readCandidateItemsetsParallel walks candidates in slice order (must be SE-sorted by caller).
// For each: SE skip if estimate < minUtility; compute exact utility via parallelExactUtility;
// if exact ≥ minUtility, append to temp writer and update the top-k StringPair heap (raises minUtility).
func (p *Phase2) readCandidateItemsetsParallel(hdb, bnf [][]int, candidates []datastructure.StringPair, lbfw *bufio.Writer) (int, error) {
	h := datastructure.NewStringPairRedBlackTree()
	numHU := 0

	for _, sp := range candidates {
		cir := sp.X + ":" + strconv.Itoa(sp.Y)
		ci := strings.SplitN(cir, ":", 2)
		if len(ci) != 2 {
			continue
		}
		estUtil, err := strconv.Atoi(strings.TrimSpace(ci[1]))
		if err != nil {
			continue
		}
		candidateStrs := strings.Fields(ci[0])
		if len(candidateStrs) == 0 {
			continue
		}
		candidate := make([]int, len(candidateStrs))
		for i := range candidateStrs {
			candidate[i], _ = strconv.Atoi(candidateStrs[i])
		}

		if estUtil < p.minUtility {
			continue
		}

		eUtility := p.parallelExactUtility(hdb, bnf, candidate)

		if eUtility >= p.minUtility {
			line := ci[0] + ":" + strconv.Itoa(eUtility)
			if _, err := lbfw.WriteString(line + "\n"); err != nil {
				return numHU, err
			}
			p.updateHeap(h, ci[0], eUtility)
			numHU++
		}
	}
	return numHU, nil
}

// parallelExactUtility sums EU(candidate, T) over all transactions T that contain candidate in order.
// Workers split the transaction id range [0,n); each returns a partial sum; results are summed.
func (p *Phase2) parallelExactUtility(hdb, bnf [][]int, candidate []int) int {
	n := p.numberOfTransactions
	if n == 0 {
		return 0
	}
	w := p.workers
	if w > n {
		w = n
	}
	chunk := (n + w - 1) / w
	partials := make([]int, w)
	var wg sync.WaitGroup

	// Map: each worker scans hdb[start:end] only.
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
			local := 0
			for i := start; i < end; i++ {
				if len(hdb[i]) == 0 {
					continue
				}
				pUtility := 0
				allMatch := true
				for _, cid := range candidate {
					idxItem := indexOfInt(hdb[i], cid)
					if idxItem < 0 {
						pUtility = 0
						allMatch = false
						break
					}
					pUtility += bnf[i][idxItem]
				}
				if allMatch {
					local += pUtility
				}
			}
			partials[idx] = local
		}(start, end, wi)
	}
	wg.Wait()
	// Reduce: total exact utility for this candidate.
	sum := 0
	for _, v := range partials {
		sum += v
	}
	return sum
}

// indexOfInt finds v in slice (transaction items are stored in mining order).
func indexOfInt(slice []int, v int) int {
	for i := range slice {
		if slice[i] == v {
			return i
		}
	}
	return -1
}

// readDatabase fills hdb/bnf row-wise from the SPMF utility file (one row per line, up to cap).
func readDatabase(hdb, bnf [][]int, dbPath string) error {
	in, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer in.Close()
	sc := bufio.NewScanner(in)
	transCount := 0
	for sc.Scan() {
		if transCount >= len(hdb) {
			break
		}
		record := sc.Text()
		data := strings.Split(record, ":")
		if len(data) >= 3 {
			transaction := strings.Fields(strings.TrimSpace(data[0]))
			benefit := strings.Fields(strings.TrimSpace(data[2]))
			for i := 0; i < len(transaction) && i < len(benefit); i++ {
				item, _ := strconv.Atoi(transaction[i])
				ben, _ := strconv.Atoi(benefit[i])
				hdb[transCount] = append(hdb[transCount], item)
				bnf[transCount] = append(bnf[transCount], ben)
			}
		}
		transCount++
	}
	return sc.Err()
}

func (p *Phase2) initialization(hdb, bnf [][]int) {
	for i := 0; i < len(hdb); i++ {
		hdb[i] = make([]int, 0)
		bnf[i] = make([]int, 0)
	}
}

// updateHeap maintains a size-k multiset of (itemset string, exact utility); lifts minUtility
// when k true HUIs are known (same logic as TKU Phase2 updateHeap).
func (p *Phase2) updateHeap(nch *datastructure.RedBlackTree[datastructure.StringPair], hui string, utility int) {
	if nch.Size() < p.theCurrentK {
		nch.Add(datastructure.StringPair{X: hui, Y: utility})
	} else if nch.Size() >= p.theCurrentK {
		if utility > p.minUtility {
			nch.Add(datastructure.StringPair{X: hui, Y: utility})
			nch.PopMinimum()
		}
	}
	if nch.Size() >= p.theCurrentK {
		minP := nch.Minimum()
		if minP.Y > p.minUtility {
			p.minUtility = minP.Y
		}
	}
}
