package tko

// ItemsetTKO stores one candidate itemset and its utility (SPMF TKO-Basic).
type ItemsetTKO struct {
	Prefix  []int
	Item    int
	Utility int64
}

// itemsetMinHeap is a min-heap by Utility (smallest at index 0), matching Java PriorityQueue order for eviction.
type itemsetMinHeap []*ItemsetTKO

func (h itemsetMinHeap) Len() int { return len(h) }

func (h itemsetMinHeap) Less(i, j int) bool {
	if h[i].Utility != h[j].Utility {
		return h[i].Utility < h[j].Utility
	}
	return compareItemsetLex(h[i], h[j]) < 0
}

func (h itemsetMinHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *itemsetMinHeap) Push(x any) {
	*h = append(*h, x.(*ItemsetTKO))
}

func (h *itemsetMinHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func compareItemsetLex(a, b *ItemsetTKO) int {
	for i := 0; i < len(a.Prefix) && i < len(b.Prefix); i++ {
		if a.Prefix[i] != b.Prefix[i] {
			return a.Prefix[i] - b.Prefix[i]
		}
	}
	if len(a.Prefix) != len(b.Prefix) {
		return len(a.Prefix) - len(b.Prefix)
	}
	return a.Item - b.Item
}

func newItemsetTKO(prefix []int, item int, utility int64) *ItemsetTKO {
	p := make([]int, len(prefix))
	copy(p, prefix)
	return &ItemsetTKO{Prefix: p, Item: item, Utility: utility}
}
