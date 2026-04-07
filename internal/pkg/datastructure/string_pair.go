package datastructure

// StringPair pairs a string (e.g. itemset) with an integer utility, ordered by Y (Java TKU).
type StringPair struct {
	X string
	Y int
}

// CompareStringPair returns Java StringPair.compareTo order: higher Y is "greater" (this.Y - o.Y).
func CompareStringPair(a, b StringPair) int {
	if a.Y > b.Y {
		return 1
	}
	if a.Y < b.Y {
		return -1
	}
	if a.X > b.X {
		return 1
	}
	if a.X < b.X {
		return -1
	}
	return 0
}
