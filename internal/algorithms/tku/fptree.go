package tku

import (
	"bufio"
	"strconv"

	"hui-problem/internal/pkg/datastructure"
)

// TreeNode is a node in the UP-Tree / FP-Tree used by TKU.
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

// FPTree is the utility pattern tree used in TKU Phase 1.
type FPTree struct {
	algo   *TKU
	Root   *TreeNode
	Header []*TreeNode // one header head per item id
}

// NewFPTree builds an empty tree for the given TKU context.
func NewFPTree(algo *TKU) *FPTree {
	return &FPTree{
		algo:   algo,
		Root:   NewTreeNode(-1, 0, 0),
		Header: make([]*TreeNode, algo.itemCount),
	}
}

// insertConditionalPatternIntoTree inserts a conditional transaction into a projected UP-tree: node TWU is derived from path utility minus min-BNF terms (MIU × pathCount).
func (t *FPTree) insertConditionalPatternIntoTree(itemIDs []int, numItems int, twuByItem []int, pathUtility, pathCount, sumMinBNF int) {
	par := t.Root
	for i := 0; i < numItems; i++ {
		target := itemIDs[i]
		cs := len(par.Children)
		if cs == 0 {
			m := pathUtility - (sumMinBNF - t.algo.arrayMIU[target]*pathCount)
			sumMinBNF = sumMinBNF - (t.algo.arrayMIU[target] * pathCount)
			nNode := NewTreeNode(target, m, pathCount)
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
					m := pathUtility - (sumMinBNF - t.algo.arrayMIU[target]*pathCount)
					sumMinBNF = sumMinBNF - t.algo.arrayMIU[target]*pathCount
					comp.TWU += m
					comp.Count += pathCount
					par = comp
					done = true
					break
				}
				if twuByItem[target] > twuByItem[comp.Item] {
					m := pathUtility - (sumMinBNF - t.algo.arrayMIU[target]*pathCount)
					sumMinBNF = sumMinBNF - t.algo.arrayMIU[target]*pathCount
					nNode := NewTreeNode(target, m, pathCount)
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
				if twuByItem[target] == twuByItem[comp.Item] && target < comp.Item {
					m := pathUtility - (sumMinBNF - t.algo.arrayMIU[target]*pathCount)
					sumMinBNF = sumMinBNF - t.algo.arrayMIU[target]*pathCount
					nNode := NewTreeNode(target, m, pathCount)
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
					m := pathUtility - (sumMinBNF - t.algo.arrayMIU[target]*pathCount)
					sumMinBNF = sumMinBNF - t.algo.arrayMIU[target]*pathCount
					nNode := NewTreeNode(target, m, pathCount)
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
				// unreachable if logic matches Java
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

// insertGlobalTransactionIntoUPTree inserts one filtered, TWU-sorted transaction into the global UP-tree: cumulative prefix utility labels nodes; NU heap raises minUtil.
func (t *FPTree) insertGlobalTransactionIntoUPTree(itemIDs []int, utilityStrs []string, numItems, txnCount int, twuByItem []int, nodeCountHeap *datastructure.RedBlackTree[int]) {
	cumUtility := 0
	par := t.Root
	for i := 0; i < numItems; i++ {
		u, _ := strconv.Atoi(utilityStrs[i])
		cumUtility += u
		target := itemIDs[i]
		cs := len(par.Children)
		if cs == 0 {
			nNode := NewTreeNode(target, cumUtility, txnCount)
			par.Children = append(par.Children, nNode)
			if float64(nNode.TWU) > t.algo.globalMinUtil {
				t.algo.updateNodeCountHeap(nodeCountHeap, nNode.TWU)
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
					t.algo.updateNodeCountHeap(nodeCountHeap, comp.TWU+cumUtility)
					comp.TWU += cumUtility
					comp.Count += txnCount
					par = comp
					break
				}
				if twuByItem[target] > twuByItem[comp.Item] {
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, cumUtility)
					}
					nNode := NewTreeNode(target, cumUtility, txnCount)
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
				if twuByItem[target] == twuByItem[comp.Item] && target < comp.Item {
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, cumUtility)
					}
					nNode := NewTreeNode(target, cumUtility, txnCount)
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
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, cumUtility)
					}
					nNode := NewTreeNode(target, cumUtility, txnCount)
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

// UPGrowth runs TKU mining on the global tree (Java UPGrowth).
// For each header item: extract conditional pattern base (CPB), emit extensions with MAU/MIU estimates, build local tree, recurse with upGrowthMinBNF.
func (t *FPTree) UPGrowth(tree2 *FPTree, flist2 []int, prefix string, w *bufio.Writer, isNodeCountHeap *datastructure.RedBlackTree[int], itemTWU []int) error {
	for i := 0; i < len(flist2); i++ {
		if float64(itemTWU[flist2[i]]) >= t.algo.globalMinUtil {
			// Extend prefix with current header item; citem is the item being mined at this level.
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

			// Walk the header chain for citem: collect prefix paths and aggregate local TWU/counts into conditional transactions (CPB).
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

			// Prune local items below the threshold; for each surviving extension j, emit a candidate line if MAU bound passes; tighten heap with MIU bound.
			localflist := make([]int, 0)
			for j := 0; j < len(localF1); j++ {
				if float64(localF1[j]) < t.algo.globalMinUtil {
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
						if float64(mau) >= t.algo.globalMinUtil {
							miu := sumMiu * localCount[j]
							line := nprefix + " " + strconv.Itoa(j) + ":" + strconv.Itoa(localF1[j])
							if _, err := w.WriteString(line + "\n"); err != nil {
								return err
							}
							if float64(miu) > t.algo.globalMinUtil {
								t.algo.updateNodeCountHeap(isNodeCountHeap, miu)
							}
						}
					}
				}
			}

			// Project CPB rows into a conditional tree and recurse with the same UP-Growth logic on the local header list.
			if len(cpb) > 0 {
				cFptree := NewFPTree(t.algo)
				for k := 0; k < len(cpb); k++ {
					cpbPath := cpb[k]
					sumMinBNF := 0
					projItems := make([]int, len(cpbPath))
					numProj := 0
					for h := 0; h < len(cpbPath); h++ {
						it := cpbPath[h]
						if float64(localF1[it]) >= t.algo.globalMinUtil {
							sumMinBNF += cpbc[k] * t.algo.arrayMIU[it]
							projItems[numProj] = it
							numProj++
						} else {
							cpbw[k] -= cpbc[k] * t.algo.arrayMIU[it]
						}
					}
					t.algo.sortItemsByDescendingTWU(projItems, 0, numProj, localF1)
					cFptree.insertConditionalPatternIntoTree(projItems, numProj, localF1, cpbw[k], cpbc[k], sumMinBNF)
				}
				if err := cFptree.upGrowthMinBNF(cFptree, localflist, nprefix, w, isNodeCountHeap, localF1); err != nil {
					return err
				}
			}
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

// upGrowthMinBNF recursively mines conditional UP-trees produced by insertConditionalPatternIntoTree (same control flow as UPGrowth, different path TWU semantics).
func (t *FPTree) upGrowthMinBNF(tree2 *FPTree, flist2 []int, prefix string, w *bufio.Writer, isNodeCountHeap *datastructure.RedBlackTree[int], itemTWU []int) error {
	for i := 0; i < len(flist2); i++ {
		if float64(itemTWU[flist2[i]]) >= t.algo.globalMinUtil {
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

			localflist := make([]int, 0)
			for j := 0; j < len(localF1); j++ {
				if float64(localF1[j]) < t.algo.globalMinUtil {
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
						if float64(mau) >= t.algo.globalMinUtil {
							miu := sumMiu * localCount[j]
							line := nprefix + " " + strconv.Itoa(j) + ":" + strconv.Itoa(localF1[j])
							if _, err := w.WriteString(line + "\n"); err != nil {
								return err
							}
							if float64(miu) > t.algo.globalMinUtil {
								t.algo.updateNodeCountHeap(isNodeCountHeap, miu)
							}
						}
					}
				}
			}

			if len(cpb) > 0 {
				cFptree := NewFPTree(t.algo)
				for k := 0; k < len(cpb); k++ {
					cpbPath := cpb[k]
					sumMinBNF := 0
					projItems := make([]int, len(cpbPath))
					numProj := 0
					for h := 0; h < len(cpbPath); h++ {
						it := cpbPath[h]
						if float64(localF1[it]) >= t.algo.globalMinUtil {
							sumMinBNF += cpbc[k] * t.algo.arrayMIU[it]
							projItems[numProj] = it
							numProj++
						} else {
							cpbw[k] -= cpbc[k] * t.algo.arrayMIU[it]
						}
					}
					t.algo.sortItemsByDescendingTWU(projItems, 0, numProj, localF1)
					cFptree.insertConditionalPatternIntoTree(projItems, numProj, localF1, cpbw[k], cpbc[k], sumMinBNF)
				}
				if err := cFptree.upGrowthMinBNF(cFptree, localflist, nprefix, w, isNodeCountHeap, localF1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SumDescendent (DS pruning) adds each node's Count to dsSumTable[item] for the whole subtree rooted at cNode.
func (t *FPTree) SumDescendent(cNode *TreeNode, dsSumTable []int) {
	if cNode == nil {
		return
	}
	dsSumTable[cNode.Item] += cNode.Count
	for i := 0; i < len(cNode.Children); i++ {
		t.SumDescendent(cNode.Children[i], dsSumTable)
	}
}
