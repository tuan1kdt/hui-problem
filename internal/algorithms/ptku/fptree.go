// UP-Tree construction and UPGrowth mining for PTKU Phase 1. The global tree is built sequentially;
// ParallelUPGrowth only parallelizes independent top-level branches (Fork/Join). Shared threshold
// updates go through SafeHeap.TryUpdateWithHeap with per-goroutine int heaps (MC / NU style).
package ptku

import (
	"strconv"
	"sync"
)

// TreeNode is one node in the utility pattern tree (item, aggregated TWU on paths, count, links).
type TreeNode struct {
	Item     int
	Count    int
	TWU      int
	HLink    *TreeNode
	Parent   *TreeNode
	Children []*TreeNode
}

// NewTreeNode creates a tree node.
func NewTreeNode(item, twu, count int) *TreeNode {
	return &TreeNode{Item: item, TWU: twu, Count: count, Children: nil}
}

// FPTree is the utility pattern tree used in PTKU Phase 1.
type FPTree struct {
	algo   *PTKU
	Root   *TreeNode
	Header []*TreeNode
}

// NewFPTree builds an empty tree for the given PTKU context.
func NewFPTree(algo *PTKU) *FPTree {
	return &FPTree{
		algo:   algo,
		Root:   NewTreeNode(-1, 0, 0),
		Header: make([]*TreeNode, algo.itemCount),
	}
}

// insPatternBase inserts a conditional pattern base path into a local FP-tree (UP-Growth subtree)
// using MIU-based path utility (same recurrence as TKU Java insPatternBase).
func (t *FPTree) insPatternBase(tran []int, tranlen int, l1 []int, twu, ic, sumBNF int) {
	par := t.Root
	for i := 0; i < tranlen; i++ {
		target := tran[i]
		cs := len(par.Children)
		if cs == 0 {
			m := twu - (sumBNF - t.algo.arrayMIU[target]*ic)
			sumBNF = sumBNF - (t.algo.arrayMIU[target] * ic)
			nNode := NewTreeNode(target, m, ic)
			par.Children = append(par.Children, nNode)
			nNode.Parent = par
			if t.Header[target] == nil {
				t.Header[target] = nNode
			} else {
				nNode.HLink = t.Header[target]
				t.Header[target] = nNode
			}
			par = nNode
		} else {
			done := false
			for j := 0; j < cs; j++ {
				comp := par.Children[j]
				if target == comp.Item {
					m := twu - (sumBNF - t.algo.arrayMIU[target]*ic)
					sumBNF = sumBNF - t.algo.arrayMIU[target]*ic
					comp.TWU += m
					comp.Count += ic
					par = comp
					done = true
					break
				}
				if l1[target] > l1[comp.Item] {
					m := twu - (sumBNF - t.algo.arrayMIU[target]*ic)
					sumBNF = sumBNF - t.algo.arrayMIU[target]*ic
					nNode := NewTreeNode(target, m, ic)
					par.Children = insertChildAt(par.Children, j, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					done = true
					break
				}
				if l1[target] == l1[comp.Item] && target < comp.Item {
					m := twu - (sumBNF - t.algo.arrayMIU[target]*ic)
					sumBNF = sumBNF - t.algo.arrayMIU[target]*ic
					nNode := NewTreeNode(target, m, ic)
					par.Children = insertChildAt(par.Children, j, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					done = true
					break
				}
				if j == cs-1 {
					m := twu - (sumBNF - t.algo.arrayMIU[target]*ic)
					sumBNF = sumBNF - t.algo.arrayMIU[target]*ic
					nNode := NewTreeNode(target, m, ic)
					par.Children = append(par.Children, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					done = true
					break
				}
			}
			if !done {
			}
		}
	}
}

func insertChildAt(children []*TreeNode, idx int, n *TreeNode) []*TreeNode {
	children = append(children, nil)
	copy(children[idx+1:], children[idx:])
	children[idx] = n
	return children
}

// instrans3 inserts one filtered, sorted transaction into the global UP-Tree: prefix utilities on nodes,
// header links, and NU threshold raises via nodeCountHeap + TryUpdateWithHeap when path TWU exceeds border.
func (t *FPTree) instrans3(tran []int, bran []string, tranlen, ic int, l1 []int, nodeCountHeap *RedBlackTree[int]) {
	twu := 0
	par := t.Root
	gmu := t.algo.threshold.MinUtil()
	for i := 0; i < tranlen; i++ {
		u, _ := strconv.Atoi(bran[i])
		twu += u
		target := tran[i]
		cs := len(par.Children)
		if cs == 0 {
			nNode := NewTreeNode(target, twu, ic)
			par.Children = append(par.Children, nNode)
			if float64(nNode.TWU) > gmu {
				t.algo.threshold.TryUpdateWithHeap(nodeCountHeap, nNode.TWU)
				gmu = t.algo.threshold.MinUtil()
			}
			nNode.Parent = par
			if t.Header[target] == nil {
				t.Header[target] = nNode
			} else {
				nNode.HLink = t.Header[target]
				t.Header[target] = nNode
			}
			par = nNode
		} else {
			for j := 0; j < cs; j++ {
				comp := par.Children[j]
				if target == comp.Item {
					nodeCountHeap.Remove(comp.TWU)
					t.algo.threshold.TryUpdateWithHeap(nodeCountHeap, comp.TWU+twu)
					gmu = t.algo.threshold.MinUtil()
					comp.TWU += twu
					comp.Count += ic
					par = comp
					break
				}
				if l1[target] > l1[comp.Item] {
					if float64(comp.TWU) > gmu {
						t.algo.threshold.TryUpdateWithHeap(nodeCountHeap, twu)
						gmu = t.algo.threshold.MinUtil()
					}
					nNode := NewTreeNode(target, twu, ic)
					par.Children = insertChildAt(par.Children, j, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					break
				}
				if l1[target] == l1[comp.Item] && target < comp.Item {
					if float64(comp.TWU) > gmu {
						t.algo.threshold.TryUpdateWithHeap(nodeCountHeap, twu)
						gmu = t.algo.threshold.MinUtil()
					}
					nNode := NewTreeNode(target, twu, ic)
					par.Children = insertChildAt(par.Children, j, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					break
				}
				if j == cs-1 {
					if float64(comp.TWU) > gmu {
						t.algo.threshold.TryUpdateWithHeap(nodeCountHeap, twu)
						gmu = t.algo.threshold.MinUtil()
					}
					nNode := NewTreeNode(target, twu, ic)
					par.Children = append(par.Children, nNode)
					nNode.Parent = par
					if t.Header[target] == nil {
						t.Header[target] = nNode
					} else {
						nNode.HLink = t.Header[target]
						t.Header[target] = nNode
					}
					par = nNode
					break
				}
			}
		}
	}
}

// candidateSink buffers (itemset string, estimated utility) from concurrent UPGrowth workers.
type candidateSink struct {
	mu    sync.Mutex
	items []StringPair
}

func (s *candidateSink) add(p StringPair) {
	s.mu.Lock()
	s.items = append(s.items, p)
	s.mu.Unlock()
}

// ParallelUPGrowth implements Fork/Join at the root UPGrowth loop: for each eligible item in flist2,
// spawn a worker that runs upGrowthOneTopItem with its own MC heap. The read-only global tree2 and
// header links are shared; writes go to sink and SafeHeap only.
func (t *FPTree) ParallelUPGrowth(tree2 *FPTree, flist2 []int, prefix string, lp1 []int, sink *candidateSink) error {
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i := 0; i < len(flist2); i++ {
		if float64(lp1[flist2[i]]) < t.algo.threshold.MinUtil() {
			continue
		}
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			isHeap := NewIntRedBlackTree()
			if err := t.upGrowthOneTopItem(tree2, flist2, prefix, idx, isHeap, lp1, sink); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

// upGrowthOneTopItem runs the body of UPGrowth for a single index i in flist2: build conditional
// pattern base from header links, emit MAU/MIU-qualified extensions to sink, build local tree and recurse
// upGrowthMinBNF. Matches one iteration of TKU UPGrowth outer loop.
func (t *FPTree) upGrowthOneTopItem(tree2 *FPTree, flist2 []int, prefix string, i int, isNodeCountHeap *RedBlackTree[int], lp1 []int, sink *candidateSink) error {
	if float64(lp1[flist2[i]]) < t.algo.threshold.MinUtil() {
		return nil
	}
	var nprefix string
	if prefix == "" {
		nprefix = prefix + strconv.Itoa(flist2[i])
	} else {
		nprefix = prefix + " " + strconv.Itoa(flist2[i])
	}
	citem := flist2[i]
	chlink := tree2.Header[citem]

	var cpb [][]int
	var cpbw []int
	var cpbc []int
	localF1 := make([]int, t.algo.itemCount)
	localCount := make([]int, t.algo.itemCount)

	for chlink != nil {
		var path []int
		cptr := chlink
		for cptr.Parent != nil {
			path = append(path, cptr.Item)
			localF1[cptr.Item] += chlink.TWU
			localCount[cptr.Item] += chlink.Count
			cptr = cptr.Parent
		}
		if len(path) > 0 {
			path = path[1:]
		}
		cpb = append(cpb, path)
		cpbw = append(cpbw, chlink.TWU)
		cpbc = append(cpbc, chlink.Count)
		chlink = chlink.HLink
	}

	gmu := t.algo.threshold.MinUtil()
	localflist := make([]int, 0)
	for j := 0; j < len(localF1); j++ {
		if float64(localF1[j]) < gmu {
			localF1[j] = -1
		} else {
			if j != citem {
				t.algo.insertItem(&localflist, j, localF1)
				uti := nprefix + " " + strconv.Itoa(j)
				tempItems := splitIntsFromString(uti)
				sumMau := 0
				sumMiu := 0
				for _, ti := range tempItems {
					sumMau += t.algo.arrayMAU[ti]
					sumMiu += t.algo.arrayMIU[ti]
				}
				mau := sumMau * localCount[j]
				if float64(mau) >= gmu {
					miu := sumMiu * localCount[j]
					sink.add(StringPair{X: nprefix + " " + strconv.Itoa(j), Y: localF1[j]})
					if float64(miu) > gmu {
						t.algo.threshold.TryUpdateWithHeap(isNodeCountHeap, miu)
						gmu = t.algo.threshold.MinUtil()
					}
				}
			}
		}
	}

	if len(cpb) > 0 {
		cFptree := NewFPTree(t.algo)
		for k := 0; k < len(cpb); k++ {
			ltran := cpb[k]
			sumMinBNF := 0
			tran := make([]int, len(ltran))
			tranlen := 0
			gmu = t.algo.threshold.MinUtil()
			for h := 0; h < len(ltran); h++ {
				it := ltran[h]
				if float64(localF1[it]) >= gmu {
					sumMinBNF += cpbc[k] * t.algo.arrayMIU[it]
					tran[tranlen] = it
					tranlen++
				} else {
					cpbw[k] -= cpbc[k] * t.algo.arrayMIU[it]
				}
			}
			t.algo.sortTrans(tran, 0, tranlen, localF1)
			cFptree.insPatternBase(tran, tranlen, localF1, cpbw[k], cpbc[k], sumMinBNF)
		}
		if err := cFptree.upGrowthMinBNF(cFptree, localflist, nprefix, isNodeCountHeap, localF1, sink); err != nil {
			return err
		}
	}
	return nil
}

func splitIntsFromString(s string) []int {
	fields := splitFields(s)
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		v, err := strconv.Atoi(f)
		if err == nil {
			out = append(out, v)
		}
	}
	return out
}

func splitFields(s string) []string {
	var fields []string
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if start >= 0 {
				fields = append(fields, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	return fields
}

// upGrowthMinBNF is the recursive UPGrowth kernel (sequential inner loops), same structure as TKU:
// for each extension item, scan linked nodes, emit candidates, build conditional trees and recurse.
func (t *FPTree) upGrowthMinBNF(tree2 *FPTree, flist2 []int, prefix string, isNodeCountHeap *RedBlackTree[int], lp1 []int, sink *candidateSink) error {
	for i := 0; i < len(flist2); i++ {
		gmu := t.algo.threshold.MinUtil()
		if float64(lp1[flist2[i]]) < gmu {
			continue
		}
		var nprefix string
		if prefix == "" {
			nprefix = prefix + strconv.Itoa(flist2[i])
		} else {
			nprefix = prefix + " " + strconv.Itoa(flist2[i])
		}
		citem := flist2[i]
		chlink := tree2.Header[citem]

		var cpb [][]int
		var cpbw []int
		var cpbc []int
		localF1 := make([]int, t.algo.itemCount)
		localCount := make([]int, t.algo.itemCount)

		for chlink != nil {
			var path []int
			cptr := chlink
			for cptr.Parent != nil {
				path = append(path, cptr.Item)
				localF1[cptr.Item] += chlink.TWU
				localCount[cptr.Item] += chlink.Count
				cptr = cptr.Parent
			}
			if len(path) > 0 {
				path = path[1:]
			}
			cpb = append(cpb, path)
			cpbw = append(cpbw, chlink.TWU)
			cpbc = append(cpbc, chlink.Count)
			chlink = chlink.HLink
		}

		gmu = t.algo.threshold.MinUtil()
		localflist := make([]int, 0)
		for j := 0; j < len(localF1); j++ {
			if float64(localF1[j]) < gmu {
				localF1[j] = -1
			} else {
				if j != citem {
					t.algo.insertItem(&localflist, j, localF1)
					uti := nprefix + " " + strconv.Itoa(j)
					tempItems := splitIntsFromString(uti)
					sumMau := 0
					sumMiu := 0
					for _, ti := range tempItems {
						sumMau += t.algo.arrayMAU[ti]
						sumMiu += t.algo.arrayMIU[ti]
					}
					mau := sumMau * localCount[j]
					if float64(mau) >= gmu {
						miu := sumMiu * localCount[j]
						sink.add(StringPair{X: nprefix + " " + strconv.Itoa(j), Y: localF1[j]})
						if float64(miu) > gmu {
							t.algo.threshold.TryUpdateWithHeap(isNodeCountHeap, miu)
							gmu = t.algo.threshold.MinUtil()
						}
					}
				}
			}
		}

		if len(cpb) > 0 {
			cFptree := NewFPTree(t.algo)
			for k := 0; k < len(cpb); k++ {
				ltran := cpb[k]
				sumMinBNF := 0
				tran := make([]int, len(ltran))
				tranlen := 0
				gmu = t.algo.threshold.MinUtil()
				for h := 0; h < len(ltran); h++ {
					it := ltran[h]
					if float64(localF1[it]) >= gmu {
						sumMinBNF += cpbc[k] * t.algo.arrayMIU[it]
						tran[tranlen] = it
						tranlen++
					} else {
						cpbw[k] -= cpbc[k] * t.algo.arrayMIU[it]
					}
				}
				t.algo.sortTrans(tran, 0, tranlen, localF1)
				cFptree.insPatternBase(tran, tranlen, localF1, cpbw[k], cpbc[k], sumMinBNF)
			}
			if err := cFptree.upGrowthMinBNF(cFptree, localflist, nprefix, isNodeCountHeap, localF1, sink); err != nil {
				return err
			}
		}
	}
	return nil
}

// SumDescendent DFS from cNode to fill dsSumTable[item] += Count (used by MD strategy on root children).
func (t *FPTree) SumDescendent(cNode *TreeNode, dsSumTable []int) {
	if cNode == nil {
		return
	}
	dsSumTable[cNode.Item] += cNode.Count
	for i := 0; i < len(cNode.Children); i++ {
		t.SumDescendent(cNode.Children[i], dsSumTable)
	}
}
