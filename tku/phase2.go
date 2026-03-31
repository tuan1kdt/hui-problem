package tku

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Phase2 runs TKU phase 2: verify candidates against the database.
type Phase2 struct {
	minUtility           int
	theCurrentK          int
	numberOfTransactions int
	inputFilePath        string
	sortedCandidatePath  string
	temporaryFilePath    string
	outputFilePath       string
	numTopKHUI           int
}

// NewPhase2 creates a Phase2 runner.
func NewPhase2() *Phase2 {
	return &Phase2{temporaryFilePath: "HUI.txt"}
}

// RunAlgorithm executes phase 2 (Java AlgoPhase2OfTKU.runAlgorithm).
func (p *Phase2) RunAlgorithm(minUtil, transactionCount, currentK int, inputPath, sortedCandidateFile, outputFile string) error {
	p.minUtility = minUtil
	p.numberOfTransactions = transactionCount
	p.theCurrentK = currentK
	p.inputFilePath = inputPath
	p.sortedCandidatePath = sortedCandidateFile
	p.outputFilePath = outputFile

	tmp, err := os.Create(p.temporaryFilePath)
	if err != nil {
		return err
	}
	bfw := bufio.NewWriter(tmp)

	hdb := make([][]int, p.numberOfTransactions)
	bnf := make([][]int, p.numberOfTransactions)
	p.initialization(hdb, bnf)

	if err := readDatabase(hdb, bnf, p.inputFilePath); err != nil {
		_ = tmp.Close()
		return err
	}

	if _, err := p.readCandidateItemsets(hdb, bnf, p.sortedCandidatePath, bfw); err != nil {
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
	_ = os.Remove(p.sortedCandidatePath)
	return nil
}

// NumberOfTopKHUIs returns how many lines were written to the output file.
func (p *Phase2) NumberOfTopKHUIs() int {
	return p.numTopKHUI
}

func (p *Phase2) readCandidateItemsets(hdb, bnf [][]int, ciPath string, lbfw *bufio.Writer) (int, error) {
	h := NewStringPairRedBlackTree()

	in, err := os.Open(ciPath)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	bfr := bufio.NewScanner(in)

	numHU := 0
	for bfr.Scan() {
		cir := strings.TrimSpace(bfr.Text())
		if cir == "" {
			continue
		}
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

		eUtility := 0
		for i := 0; i < p.numberOfTransactions; i++ {
			if len(hdb[i]) == 0 {
				continue
			}
			pUtility := 0
			allMatch := true
			for _, cid := range candidate {
				idx := indexOfInt(hdb[i], cid)
				if idx < 0 {
					pUtility = 0
					allMatch = false
					break
				}
				pUtility += bnf[i][idx]
			}
			if allMatch {
				eUtility += pUtility
			}
		}

		if eUtility >= p.minUtility {
			line := ci[0] + ":" + strconv.Itoa(eUtility)
			if _, err := lbfw.WriteString(line + "\n"); err != nil {
				return numHU, err
			}
			p.updateHeap(h, ci[0], eUtility)
			numHU++
		}
	}
	return numHU, bfr.Err()
}

func indexOfInt(slice []int, v int) int {
	for i := range slice {
		if slice[i] == v {
			return i
		}
	}
	return -1
}

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

func (p *Phase2) updateHeap(nch *RedBlackTree[StringPair], hui string, utility int) {
	if nch.Size() < p.theCurrentK {
		nch.Add(StringPair{X: hui, Y: utility})
	} else if nch.Size() >= p.theCurrentK {
		if utility > p.minUtility {
			nch.Add(StringPair{X: hui, Y: utility})
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
