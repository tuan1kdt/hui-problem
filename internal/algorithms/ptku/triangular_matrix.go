package ptku

// TriangularMatrix stores PE (Pre-Evaluation) aggregates: for each transaction, pairs (first item, other item)
// accumulate u(first)+u(other). The k-th largest cell is a safe lower bound to initialize minUtil.
type TriangularMatrix struct {
	Matrix       [][]int
	ElementCount int
}

// NewTriangularMatrix allocates rows for elementCount items.
func NewTriangularMatrix(elementCount int) *TriangularMatrix {
	m := make([][]int, elementCount)
	for i := 0; i < elementCount; i++ {
		m[i] = make([]int, elementCount-i)
	}
	return &TriangularMatrix{Matrix: m, ElementCount: elementCount}
}

// IncrementCount adds sum to the cell for pair (id1, id2).
func (t *TriangularMatrix) IncrementCount(id1, id2, sum int) {
	if id2 < id1 {
		t.Matrix[id2][t.ElementCount-id1-1] += sum
	} else {
		t.Matrix[id1][t.ElementCount-id2-1] += sum
	}
}

// GetSupportForItems returns the stored value for pair (id1, id2).
func (t *TriangularMatrix) GetSupportForItems(id1, id2 int) int {
	if id2 < id1 {
		return t.Matrix[id2][t.ElementCount-id1-1]
	}
	return t.Matrix[id1][t.ElementCount-id2-1]
}
