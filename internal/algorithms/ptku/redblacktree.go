package ptku

// RedBlackTree is a multiset BST with red-black balancing (SPMF-compatible behavior).
type RedBlackTree[T any] struct {
	root     *rbNode[T]
	size     int
	allowDup bool
	cmp      func(a, b T) int
	eq       func(a, b T) bool
}

const (
	rbBlack = true
	rbRed   = false
)

type rbNode[T any] struct {
	key                 T
	color               bool
	left, right, parent *rbNode[T]
}

// NewRedBlackTree creates a tree. If eq is nil, keys match when cmp(a,b)==0.
func NewRedBlackTree[T any](allowDup bool, cmp func(a, b T) int, eq func(a, b T) bool) *RedBlackTree[T] {
	return &RedBlackTree[T]{allowDup: allowDup, cmp: cmp, eq: eq}
}

func (t *RedBlackTree[T]) match(k, nodeKey T) bool {
	if t.eq != nil {
		return t.eq(k, nodeKey)
	}
	return t.cmp(k, nodeKey) == 0
}

// Size returns the number of keys stored.
func (t *RedBlackTree[T]) Size() int {
	return t.size
}

// Add inserts a key.
func (t *RedBlackTree[T]) Add(element T) {
	z := &rbNode[T]{key: element, color: rbRed}
	var y *rbNode[T]
	x := t.root
	for x != nil {
		y = x
		c := t.cmp(z.key, x.key)
		if c < 0 {
			x = x.left
		} else {
			if c == 0 && !t.allowDup {
				return
			}
			x = x.right
		}
	}
	z.parent = y
	if y == nil {
		t.root = z
	} else if t.cmp(z.key, y.key) < 0 {
		y.left = z
	} else {
		y.right = z
	}
	z.left = nil
	z.right = nil
	z.color = rbRed
	t.size++
	t.insertFixup(z)
}

func (t *RedBlackTree[T]) leftRotate(x *rbNode[T]) {
	y := x.right
	x.right = y.left
	if y.left != nil {
		y.left.parent = x
	}
	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.left {
		x.parent.left = y
	} else {
		x.parent.right = y
	}
	y.left = x
	x.parent = y
}

func (t *RedBlackTree[T]) rightRotate(x *rbNode[T]) {
	y := x.left
	x.left = y.right
	if y.right != nil {
		y.right.parent = x
	}
	y.parent = x.parent
	if x.parent == nil {
		t.root = y
	} else if x == x.parent.right {
		x.parent.right = y
	} else {
		x.parent.left = y
	}
	y.right = x
	x.parent = y
}

func (t *RedBlackTree[T]) insertFixup(z *rbNode[T]) {
	for z.parent != nil && z.parent.color == rbRed {
		if z.parent == z.parent.parent.left {
			y := z.parent.parent.right
			if y != nil && y.color == rbRed {
				z.parent.color = rbBlack
				y.color = rbBlack
				z.parent.parent.color = rbRed
				z = z.parent.parent
			} else {
				if z == z.parent.right {
					z = z.parent
					t.leftRotate(z)
				}
				z.parent.color = rbBlack
				z.parent.parent.color = rbRed
				t.rightRotate(z.parent.parent)
			}
		} else {
			y := z.parent.parent.left
			if y != nil && y.color == rbRed {
				z.parent.color = rbBlack
				y.color = rbBlack
				z.parent.parent.color = rbRed
				z = z.parent.parent
			} else {
				if z == z.parent.left {
					z = z.parent
					t.rightRotate(z)
				}
				z.parent.color = rbBlack
				z.parent.parent.color = rbRed
				t.leftRotate(z.parent.parent)
			}
		}
	}
	t.root.color = rbBlack
}

func (t *RedBlackTree[T]) transplant(u, v *rbNode[T]) {
	if u.parent == nil {
		t.root = v
	} else if u == u.parent.left {
		u.parent.left = v
	} else {
		u.parent.right = v
	}
	if v != nil {
		v.parent = u.parent
	}
}

// Remove deletes one occurrence of element (by cmp/eq), if present.
func (t *RedBlackTree[T]) Remove(element T) {
	z := t.search(t.root, element)
	if z == nil {
		return
	}
	t.performDelete(z)
}

func (t *RedBlackTree[T]) search(x *rbNode[T], k T) *rbNode[T] {
	for x != nil && !t.match(k, x.key) {
		if t.cmp(k, x.key) < 0 {
			x = x.left
		} else {
			x = x.right
		}
	}
	return x
}

func (t *RedBlackTree[T]) minimumNode(x *rbNode[T]) *rbNode[T] {
	for x.left != nil {
		x = x.left
	}
	return x
}

func childSideLeft[T any](z *rbNode[T]) bool {
	return z.parent != nil && z == z.parent.left
}

func (t *RedBlackTree[T]) performDelete(z *rbNode[T]) {
	var x *rbNode[T]
	y := z
	yOrigColor := y.color

	if z.left == nil {
		x = z.right
		parent := z.parent
		wasLeft := childSideLeft(z)
		t.transplant(z, z.right)
		if yOrigColor == rbBlack {
			t.fixAfterDelete(x, parent, wasLeft)
		}
	} else if z.right == nil {
		x = z.left
		parent := z.parent
		wasLeft := childSideLeft(z)
		t.transplant(z, z.left)
		if yOrigColor == rbBlack {
			t.fixAfterDelete(x, parent, wasLeft)
		}
	} else {
		y = t.minimumNode(z.right)
		yOrigColor = y.color
		x = y.right
		if y.parent == z {
			if x != nil {
				x.parent = y
			}
		} else {
			t.transplant(y, y.right)
			y.right = z.right
			if y.right != nil {
				y.right.parent = y
			}
		}
		t.transplant(z, y)
		y.left = z.left
		if y.left != nil {
			y.left.parent = y
		}
		y.color = z.color
		if yOrigColor == rbBlack {
			if x != nil {
				t.fixAfterDelete(x, nil, false)
			} else {
				t.fixAfterDelete(nil, y, false)
			}
		}
	}
	t.size--
}

// fixAfterDelete restores RB invariants. x is the child that replaced the deleted node (may be nil).
// If x is nil, the double-black sits at parent's wasLeft child slot (parent may be nil if deleted was root).
func (t *RedBlackTree[T]) fixAfterDelete(x *rbNode[T], parent *rbNode[T], wasLeft bool) {
	if x != nil && x.color == rbRed {
		x.color = rbBlack
		return
	}

	var cur *rbNode[T]
	var p *rbNode[T]
	var leftChild bool
	if x != nil {
		cur = x
		p = cur.parent
		if p != nil {
			leftChild = cur == p.left
		}
	} else {
		cur = nil
		p = parent
		leftChild = wasLeft
	}

	for cur != t.root && (cur == nil || cur.color == rbBlack) {
		if p == nil {
			break
		}
		if leftChild {
			w := p.right
			if w != nil && w.color == rbRed {
				w.color = rbBlack
				p.color = rbRed
				t.leftRotate(p)
				w = p.right
			}
			if w == nil {
				cur = p
				p = p.parent
				if p != nil {
					leftChild = cur == p.left
				}
				break
			}
			wlBlack := w.left == nil || w.left.color == rbBlack
			wrBlack := w.right == nil || w.right.color == rbBlack
			if wlBlack && wrBlack {
				w.color = rbRed
				cur = p
				p = p.parent
				if p != nil {
					leftChild = cur == p.left
				}
			} else {
				if wrBlack {
					if w.left != nil {
						w.left.color = rbBlack
					}
					w.color = rbRed
					t.rightRotate(w)
					w = p.right
				}
				w.color = p.color
				p.color = rbBlack
				if w.right != nil {
					w.right.color = rbBlack
				}
				t.leftRotate(p)
				cur = t.root
			}
		} else {
			w := p.left
			if w != nil && w.color == rbRed {
				w.color = rbBlack
				p.color = rbRed
				t.rightRotate(p)
				w = p.left
			}
			if w == nil {
				break
			}
			wlBlack := w.left == nil || w.left.color == rbBlack
			wrBlack := w.right == nil || w.right.color == rbBlack
			if wlBlack && wrBlack {
				w.color = rbRed
				cur = p
				p = p.parent
				if p != nil {
					leftChild = cur == p.left
				}
			} else {
				if wlBlack {
					if w.right != nil {
						w.right.color = rbBlack
					}
					w.color = rbRed
					t.leftRotate(w)
					w = p.left
				}
				w.color = p.color
				p.color = rbBlack
				if w.left != nil {
					w.left.color = rbBlack
				}
				t.rightRotate(p)
				cur = t.root
			}
		}
	}
	if cur != nil {
		cur.color = rbBlack
	}
	if t.root != nil {
		t.root.color = rbBlack
	}
}

// Minimum returns the smallest key or zero value if empty.
func (t *RedBlackTree[T]) Minimum() T {
	var zero T
	if t.root == nil {
		return zero
	}
	n := t.minimumNode(t.root)
	return n.key
}

func (t *RedBlackTree[T]) maximumNode(x *rbNode[T]) *rbNode[T] {
	for x.right != nil {
		x = x.right
	}
	return x
}

// Maximum returns the largest key or zero value if empty.
func (t *RedBlackTree[T]) Maximum() T {
	var zero T
	if t.root == nil {
		return zero
	}
	n := t.maximumNode(t.root)
	return n.key
}

// PopMinimum removes and returns the smallest key.
func (t *RedBlackTree[T]) PopMinimum() T {
	var zero T
	if t.root == nil {
		return zero
	}
	x := t.minimumNode(t.root)
	k := x.key
	t.performDelete(x)
	return k
}

// PopMaximum removes and returns the largest key.
func (t *RedBlackTree[T]) PopMaximum() T {
	var zero T
	if t.root == nil {
		return zero
	}
	x := t.maximumNode(t.root)
	k := x.key
	t.performDelete(x)
	return k
}

// NewIntRedBlackTree is a multiset ordered by int value.
func NewIntRedBlackTree() *RedBlackTree[int] {
	return NewRedBlackTree(true, func(a, b int) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}, func(a, b int) bool { return a == b })
}

// NewStringPairRedBlackTree matches SPMF StringPair ordering (Y primary per compareTo; X tie-break).
func NewStringPairRedBlackTree() *RedBlackTree[StringPair] {
	return NewRedBlackTree(true, CompareStringPair, func(a, b StringPair) bool {
		return a.X == b.X && a.Y == b.Y
	})
}
