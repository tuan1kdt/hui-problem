package tku

import (
	"bufio"
	"strconv"
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

func (t *FPTree) instrans3(tran []int, bran []string, tranlen, ic int, l1 []int, nodeCountHeap *RedBlackTree[int]) {
	twu := 0
	par := t.Root
	for i := 0; i < tranlen; i++ {
		u, _ := strconv.Atoi(bran[i])
		twu += u
		target := tran[i]
		cs := len(par.Children)
		if cs == 0 {
			nNode := NewTreeNode(target, twu, ic)
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
					t.algo.updateNodeCountHeap(nodeCountHeap, comp.TWU+twu)
					comp.TWU += twu
					comp.Count += ic
					par = comp
					break
				}
				if l1[target] > l1[comp.Item] {
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, twu)
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
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, twu)
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
					if float64(comp.TWU) > t.algo.globalMinUtil {
						t.algo.updateNodeCountHeap(nodeCountHeap, twu)
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

// UPGrowth runs TKU mining on the global tree (Java UPGrowth).
func (t *FPTree) UPGrowth(tree2 *FPTree, flist2 []int, prefix string, w *bufio.Writer, isNodeCountHeap *RedBlackTree[int], lp1 []int) error {
	for i := 0; i < len(flist2); i++ {
		if float64(lp1[flist2[i]]) >= t.algo.globalMinUtil {
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
					ltran := cpb[k]
					sumMinBNF := 0
					tran := make([]int, len(ltran))
					tranlen := 0
					for h := 0; h < len(ltran); h++ {
						it := ltran[h]
						if float64(localF1[it]) >= t.algo.globalMinUtil {
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

func (t *FPTree) upGrowthMinBNF(tree2 *FPTree, flist2 []int, prefix string, w *bufio.Writer, isNodeCountHeap *RedBlackTree[int], lp1 []int) error {
	for i := 0; i < len(flist2); i++ {
		if float64(lp1[flist2[i]]) >= t.algo.globalMinUtil {
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
					ltran := cpb[k]
					sumMinBNF := 0
					tran := make([]int, len(ltran))
					tranlen := 0
					for h := 0; h < len(ltran); h++ {
						it := ltran[h]
						if float64(localF1[it]) >= t.algo.globalMinUtil {
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
				if err := cFptree.upGrowthMinBNF(cFptree, localflist, nprefix, w, isNodeCountHeap, localF1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SumDescendent accumulates descendant counts into dsSumTable.
func (t *FPTree) SumDescendent(cNode *TreeNode, dsSumTable []int) {
	if cNode == nil {
		return
	}
	dsSumTable[cNode.Item] += cNode.Count
	for i := 0; i < len(cNode.Children); i++ {
		t.SumDescendent(cNode.Children[i], dsSumTable)
	}
}
