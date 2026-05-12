package ptko

// Element is one row in a utility list (tid, iutils, rutils).
type Element struct {
	TID    int
	Iutils int
	Rutils int
}

// UtilityList holds aggregated utility information for an item or itemset extension.
type UtilityList struct {
	Item      int
	SumIutils int64
	SumRutils int64
	Elements  []Element
}

// NewUtilityList creates an empty utility list for an item.
func NewUtilityList(item int) *UtilityList {
	return &UtilityList{Item: item}
}

// AddElement appends an element and updates SumIutils / SumRutils.
func (u *UtilityList) AddElement(e Element) {
	u.SumIutils += int64(e.Iutils)
	u.SumRutils += int64(e.Rutils)
	u.Elements = append(u.Elements, e)
}
