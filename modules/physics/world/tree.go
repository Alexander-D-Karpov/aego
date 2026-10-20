package world

const nullNode int32 = -1

type treeNode struct {
	box    AABB
	parent int32
	child1 int32
	child2 int32
	height int32
	data   int32
}

func (n *treeNode) leaf() bool {
	return n.child1 == nullNode
}

type tree struct {
	nodes []treeNode
	root  int32
	free  int32
	count int
	stack []int32
}

func newTree(capacity int) tree {
	t := tree{root: nullNode, free: nullNode, stack: make([]int32, 0, 256)}
	t.nodes = make([]treeNode, 0, max(capacity, 16))
	return t
}

func (t *tree) alloc() int32 {
	if t.free != nullNode {
		id := t.free
		t.free = t.nodes[id].parent
		t.nodes[id] = treeNode{parent: nullNode, child1: nullNode, child2: nullNode}
		t.count++
		return id
	}
	t.nodes = append(t.nodes, treeNode{parent: nullNode, child1: nullNode, child2: nullNode})
	t.count++
	return int32(len(t.nodes) - 1)
}

func (t *tree) release(id int32) {
	t.nodes[id] = treeNode{parent: t.free, child1: nullNode, child2: nullNode, height: -1}
	t.free = id
	t.count--
}

func (t *tree) create(box AABB, data int32) int32 {
	id := t.alloc()
	n := &t.nodes[id]
	n.box, n.data = box, data
	t.insertLeaf(id)
	return id
}

func (t *tree) destroy(id int32) {
	t.removeLeaf(id)
	t.release(id)
}

func (t *tree) move(id int32, box AABB) {
	t.removeLeaf(id)
	t.nodes[id].box = box
	t.insertLeaf(id)
}

func (t *tree) fat(id int32) AABB {
	return t.nodes[id].box
}

func (t *tree) data(id int32) int32 {
	return t.nodes[id].data
}

func (t *tree) setData(id, data int32) {
	t.nodes[id].data = data
}

func (t *tree) height() int32 {
	if t.root == nullNode {
		return 0
	}
	return t.nodes[t.root].height
}

func (t *tree) insertLeaf(leaf int32) {
	if t.root == nullNode {
		t.root = leaf
		t.nodes[leaf].parent = nullNode
		return
	}
	lb := t.nodes[leaf].box
	index := t.root
	for !t.nodes[index].leaf() {
		n := &t.nodes[index]
		c1, c2 := n.child1, n.child2
		area := boxPerimeter(n.box)
		combined := boxPerimeter(boxUnion(n.box, lb))
		cost := 2 * combined
		inherit := 2 * (combined - area)
		descend := func(c int32) float32 {
			cn := &t.nodes[c]
			u := boxPerimeter(boxUnion(lb, cn.box))
			if cn.leaf() {
				return u + inherit
			}
			return u - boxPerimeter(cn.box) + inherit
		}
		cost1, cost2 := descend(c1), descend(c2)
		if cost < cost1 && cost < cost2 {
			break
		}
		if cost1 < cost2 {
			index = c1
		} else {
			index = c2
		}
	}
	sibling := index
	oldParent := t.nodes[sibling].parent
	newParent := t.alloc()
	np := &t.nodes[newParent]
	np.parent = oldParent
	np.data = -1
	np.box = boxUnion(lb, t.nodes[sibling].box)
	np.height = t.nodes[sibling].height + 1
	np.child1, np.child2 = sibling, leaf
	if oldParent != nullNode {
		op := &t.nodes[oldParent]
		if op.child1 == sibling {
			op.child1 = newParent
		} else {
			op.child2 = newParent
		}
	} else {
		t.root = newParent
	}
	t.nodes[sibling].parent = newParent
	t.nodes[leaf].parent = newParent
	t.refit(t.nodes[leaf].parent)
}

func (t *tree) refit(index int32) {
	for index != nullNode {
		index = t.balance(index)
		n := &t.nodes[index]
		c1, c2 := &t.nodes[n.child1], &t.nodes[n.child2]
		n.height = 1 + max(c1.height, c2.height)
		n.box = boxUnion(c1.box, c2.box)
		index = n.parent
	}
}

func (t *tree) removeLeaf(leaf int32) {
	if leaf == t.root {
		t.root = nullNode
		return
	}
	parent := t.nodes[leaf].parent
	grand := t.nodes[parent].parent
	sibling := t.nodes[parent].child1
	if sibling == leaf {
		sibling = t.nodes[parent].child2
	}
	if grand == nullNode {
		t.root = sibling
		t.nodes[sibling].parent = nullNode
		t.release(parent)
		return
	}
	g := &t.nodes[grand]
	if g.child1 == parent {
		g.child1 = sibling
	} else {
		g.child2 = sibling
	}
	t.nodes[sibling].parent = grand
	t.release(parent)
	t.refit(grand)
}

func (t *tree) balance(iA int32) int32 {
	A := &t.nodes[iA]
	if A.leaf() || A.height < 2 {
		return iA
	}
	iB, iC := A.child1, A.child2
	B, C := &t.nodes[iB], &t.nodes[iC]
	bal := C.height - B.height
	if bal > 1 {
		iF, iG := C.child1, C.child2
		F, G := &t.nodes[iF], &t.nodes[iG]
		C.child1 = iA
		C.parent = A.parent
		A.parent = iC
		t.relink(C.parent, iA, iC)
		if F.height > G.height {
			C.child2 = iF
			A.child2 = iG
			G.parent = iA
			A.box = boxUnion(B.box, G.box)
			C.box = boxUnion(A.box, F.box)
			A.height = 1 + max(B.height, G.height)
			C.height = 1 + max(A.height, F.height)
		} else {
			C.child2 = iG
			A.child2 = iF
			F.parent = iA
			A.box = boxUnion(B.box, F.box)
			C.box = boxUnion(A.box, G.box)
			A.height = 1 + max(B.height, F.height)
			C.height = 1 + max(A.height, G.height)
		}
		return iC
	}
	if bal < -1 {
		iD, iE := B.child1, B.child2
		D, E := &t.nodes[iD], &t.nodes[iE]
		B.child1 = iA
		B.parent = A.parent
		A.parent = iB
		t.relink(B.parent, iA, iB)
		if D.height > E.height {
			B.child2 = iD
			A.child1 = iE
			E.parent = iA
			A.box = boxUnion(C.box, E.box)
			B.box = boxUnion(A.box, D.box)
			A.height = 1 + max(C.height, E.height)
			B.height = 1 + max(A.height, D.height)
		} else {
			B.child2 = iE
			A.child1 = iD
			D.parent = iA
			A.box = boxUnion(C.box, D.box)
			B.box = boxUnion(A.box, E.box)
			A.height = 1 + max(C.height, D.height)
			B.height = 1 + max(A.height, E.height)
		}
		return iB
	}
	return iA
}

func (t *tree) relink(parent, from, to int32) {
	if parent == nullNode {
		t.root = to
		return
	}
	p := &t.nodes[parent]
	if p.child1 == from {
		p.child1 = to
	} else {
		p.child2 = to
	}
}

func (t *tree) query(box AABB, fn func(id int32) bool) {
	if t.root == nullNode {
		return
	}
	st := append(t.stack[:0], t.root)
	for len(st) > 0 {
		id := st[len(st)-1]
		st = st[:len(st)-1]
		n := &t.nodes[id]
		if !boxOverlap(n.box, box) {
			continue
		}
		if n.leaf() {
			if !fn(id) {
				break
			}
			continue
		}
		st = append(st, n.child1, n.child2)
	}
	t.stack = st[:0]
}

func (t *tree) raycast(p1, p2 Vec2, maxFraction float32, fn func(id int32, p1, p2 Vec2, maxFraction float32) float32) {
	if t.root == nullNode {
		return
	}
	r, l := normalizeLen(p2.Sub(p1))
	if l == 0 {
		return
	}
	v := crossSV(1, r)
	av := abs2(v)
	seg := func(f float32) AABB {
		e := p1.Add(p2.Sub(p1).Scale(f))
		return AABB{Min: min2(p1, e), Max: max2(p1, e)}
	}
	box := seg(maxFraction)
	st := append(t.stack[:0], t.root)
	for len(st) > 0 {
		id := st[len(st)-1]
		st = st[:len(st)-1]
		n := &t.nodes[id]
		if !boxOverlap(n.box, box) {
			continue
		}
		c := n.box.Center()
		h := n.box.Size().Scale(0.5)
		if abs32(v.Dot(p1.Sub(c)))-av.Dot(h) > 0 {
			continue
		}
		if n.leaf() {
			value := fn(id, p1, p2, maxFraction)
			if value == 0 {
				break
			}
			if value > 0 {
				maxFraction = value
				box = seg(maxFraction)
			}
			continue
		}
		st = append(st, n.child1, n.child2)
	}
	t.stack = st[:0]
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

func (t *tree) cloneInto(dst *tree) {
	dst.nodes = append(dst.nodes[:0], t.nodes...)
	dst.root, dst.free, dst.count = t.root, t.free, t.count
	if dst.stack == nil {
		dst.stack = make([]int32, 0, max(cap(t.stack), 256))
	}
}
