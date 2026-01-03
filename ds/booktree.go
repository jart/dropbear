package ds

import "dropbear/decimal"

// bookNode is an intrusive red-black tree node for order book levels.
// Price and size are embedded directly - no indirection.
type bookNode struct {
	price  decimal.Decimal
	size   decimal.Decimal
	left   int32 // index into arena, -1 for nil
	right  int32
	parent int32
	red    bool
}

// bookArena is a contiguous arena for bookNode allocation with LIFO reuse.
// Recently freed nodes are reused first, maximizing cache hits.
type bookArena struct {
	nodes []bookNode
	free  int32 // head of free list (-1 if empty), uses 'left' as next pointer
	len   int32 // number of allocated nodes in use
}

func newBookArena(capacity int) *bookArena {
	return &bookArena{
		nodes: make([]bookNode, 0, capacity),
		free:  -1,
	}
}

func (a *bookArena) alloc(price, size decimal.Decimal) int32 {
	var idx int32
	if a.free != -1 {
		// LIFO reuse - hot in cache
		idx = a.free
		a.free = a.nodes[idx].left
	} else {
		// Grow arena
		idx = int32(len(a.nodes))
		a.nodes = append(a.nodes, bookNode{})
	}
	a.len++
	n := &a.nodes[idx]
	n.price = price
	n.size = size
	n.left = -1
	n.right = -1
	n.parent = -1
	n.red = true // new nodes start red
	return idx
}

func (a *bookArena) release(idx int32) {
	// Push onto free list (LIFO)
	a.nodes[idx].left = a.free
	a.free = idx
	a.len--
}

func (a *bookArena) get(idx int32) *bookNode {
	return &a.nodes[idx]
}

// bidTree is a red-black tree for bids, sorted descending (best bid = first).
type bidTree struct {
	arena *bookArena
	root  int32 // -1 for empty
}

func newBidTree(arena *bookArena) *bidTree {
	return &bidTree{arena: arena, root: -1}
}

// askTree is a red-black tree for asks, sorted ascending (best ask = first).
type askTree struct {
	arena *bookArena
	root  int32
}

func newAskTree(arena *bookArena) *askTree {
	return &askTree{arena: arena, root: -1}
}

// Bid tree operations (descending order: highest price first)

func (t *bidTree) get(price decimal.Decimal) int32 {
	idx := t.root
	for idx != -1 {
		n := t.arena.get(idx)
		if price > n.price {
			idx = n.left // higher prices go left (descending)
		} else if price < n.price {
			idx = n.right
		} else {
			return idx
		}
	}
	return -1
}

func (t *bidTree) first() int32 {
	idx := t.root
	if idx == -1 {
		return -1
	}
	for {
		n := t.arena.get(idx)
		if n.left == -1 {
			return idx
		}
		idx = n.left
	}
}

func (t *bidTree) next(idx int32) int32 {
	if idx == -1 {
		return -1
	}
	n := t.arena.get(idx)
	if n.right != -1 {
		idx = n.right
		for {
			n = t.arena.get(idx)
			if n.left == -1 {
				return idx
			}
			idx = n.left
		}
	}
	for n.parent != -1 {
		p := t.arena.get(n.parent)
		if idx != p.right {
			return n.parent
		}
		idx = n.parent
		n = p
	}
	return -1
}

func (t *bidTree) rotateLeft(idx int32) {
	n := t.arena.get(idx)
	r := n.right
	rn := t.arena.get(r)

	n.right = rn.left
	if rn.left != -1 {
		t.arena.get(rn.left).parent = idx
	}

	rn.parent = n.parent
	if n.parent == -1 {
		t.root = r
	} else {
		p := t.arena.get(n.parent)
		if idx == p.left {
			p.left = r
		} else {
			p.right = r
		}
	}

	rn.left = idx
	n.parent = r
}

func (t *bidTree) rotateRight(idx int32) {
	n := t.arena.get(idx)
	l := n.left
	ln := t.arena.get(l)

	n.left = ln.right
	if ln.right != -1 {
		t.arena.get(ln.right).parent = idx
	}

	ln.parent = n.parent
	if n.parent == -1 {
		t.root = l
	} else {
		p := t.arena.get(n.parent)
		if idx == p.right {
			p.right = l
		} else {
			p.left = l
		}
	}

	ln.right = idx
	n.parent = l
}

func (t *bidTree) insertFixup(idx int32) {
	for idx != t.root {
		n := t.arena.get(idx)
		if n.parent == -1 {
			break
		}
		p := t.arena.get(n.parent)
		if !p.red {
			break
		}

		gp := t.arena.get(p.parent)
		if n.parent == gp.left {
			uncle := gp.right
			if uncle != -1 && t.arena.get(uncle).red {
				p.red = false
				t.arena.get(uncle).red = false
				gp.red = true
				idx = p.parent
			} else {
				if idx == p.right {
					idx = n.parent
					t.rotateLeft(idx)
					n = t.arena.get(idx)
					p = t.arena.get(n.parent)
					gp = t.arena.get(p.parent)
				}
				p.red = false
				gp.red = true
				t.rotateRight(p.parent)
			}
		} else {
			uncle := gp.left
			if uncle != -1 && t.arena.get(uncle).red {
				p.red = false
				t.arena.get(uncle).red = false
				gp.red = true
				idx = p.parent
			} else {
				if idx == p.left {
					idx = n.parent
					t.rotateRight(idx)
					n = t.arena.get(idx)
					p = t.arena.get(n.parent)
					gp = t.arena.get(p.parent)
				}
				p.red = false
				gp.red = true
				t.rotateLeft(p.parent)
			}
		}
	}
	t.arena.get(t.root).red = false
}

func (t *bidTree) insert(price, size decimal.Decimal) int32 {
	idx := t.arena.alloc(price, size)

	if t.root == -1 {
		t.root = idx
		t.arena.get(idx).red = false
		return idx
	}

	// Find insertion point (descending: higher prices go left)
	cur := t.root
	for {
		cn := t.arena.get(cur)
		if price > cn.price {
			if cn.left == -1 {
				cn.left = idx
				t.arena.get(idx).parent = cur
				break
			}
			cur = cn.left
		} else {
			if cn.right == -1 {
				cn.right = idx
				t.arena.get(idx).parent = cur
				break
			}
			cur = cn.right
		}
	}

	t.insertFixup(idx)
	return idx
}

func (t *bidTree) transplant(u, v int32) {
	un := t.arena.get(u)
	if un.parent == -1 {
		t.root = v
	} else {
		p := t.arena.get(un.parent)
		if u == p.left {
			p.left = v
		} else {
			p.right = v
		}
	}
	if v != -1 {
		t.arena.get(v).parent = un.parent
	}
}

func (t *bidTree) removeFixup(idx, parent int32) {
	for idx != t.root && (idx == -1 || !t.arena.get(idx).red) {
		if parent == -1 {
			break
		}
		pn := t.arena.get(parent)
		if idx == pn.left {
			sib := pn.right
			sn := t.arena.get(sib)
			if sn.red {
				sn.red = false
				pn.red = true
				t.rotateLeft(parent)
				pn = t.arena.get(parent)
				sib = pn.right
				sn = t.arena.get(sib)
			}
			leftBlack := sn.left == -1 || !t.arena.get(sn.left).red
			rightBlack := sn.right == -1 || !t.arena.get(sn.right).red
			if leftBlack && rightBlack {
				sn.red = true
				idx = parent
				parent = t.arena.get(idx).parent
			} else {
				if rightBlack {
					if sn.left != -1 {
						t.arena.get(sn.left).red = false
					}
					sn.red = true
					t.rotateRight(sib)
					pn = t.arena.get(parent)
					sib = pn.right
					sn = t.arena.get(sib)
				}
				sn.red = pn.red
				pn.red = false
				if sn.right != -1 {
					t.arena.get(sn.right).red = false
				}
				t.rotateLeft(parent)
				idx = t.root
				break
			}
		} else {
			sib := pn.left
			sn := t.arena.get(sib)
			if sn.red {
				sn.red = false
				pn.red = true
				t.rotateRight(parent)
				pn = t.arena.get(parent)
				sib = pn.left
				sn = t.arena.get(sib)
			}
			leftBlack := sn.left == -1 || !t.arena.get(sn.left).red
			rightBlack := sn.right == -1 || !t.arena.get(sn.right).red
			if leftBlack && rightBlack {
				sn.red = true
				idx = parent
				parent = t.arena.get(idx).parent
			} else {
				if leftBlack {
					if sn.right != -1 {
						t.arena.get(sn.right).red = false
					}
					sn.red = true
					t.rotateLeft(sib)
					pn = t.arena.get(parent)
					sib = pn.left
					sn = t.arena.get(sib)
				}
				sn.red = pn.red
				pn.red = false
				if sn.left != -1 {
					t.arena.get(sn.left).red = false
				}
				t.rotateRight(parent)
				idx = t.root
				break
			}
		}
	}
	if idx != -1 {
		t.arena.get(idx).red = false
	}
}

func (t *bidTree) minimum(idx int32) int32 {
	for {
		n := t.arena.get(idx)
		if n.left == -1 {
			return idx
		}
		idx = n.left
	}
}

func (t *bidTree) remove(idx int32) {
	n := t.arena.get(idx)
	var x, xParent int32
	y := idx
	yRed := n.red

	if n.left == -1 {
		x = n.right
		xParent = n.parent
		t.transplant(idx, n.right)
	} else if n.right == -1 {
		x = n.left
		xParent = n.parent
		t.transplant(idx, n.left)
	} else {
		y = t.minimum(n.right)
		yn := t.arena.get(y)
		yRed = yn.red
		x = yn.right
		if yn.parent == idx {
			xParent = y
		} else {
			xParent = yn.parent
			t.transplant(y, yn.right)
			yn = t.arena.get(y)
			yn.right = n.right
			t.arena.get(yn.right).parent = y
		}
		t.transplant(idx, y)
		yn = t.arena.get(y)
		yn.left = n.left
		t.arena.get(yn.left).parent = y
		yn.red = n.red
	}

	if !yRed {
		t.removeFixup(x, xParent)
	}

	t.arena.release(idx)
}

// Ask tree operations (ascending order: lowest price first)

func (t *askTree) get(price decimal.Decimal) int32 {
	idx := t.root
	for idx != -1 {
		n := t.arena.get(idx)
		if price < n.price {
			idx = n.left // lower prices go left (ascending)
		} else if price > n.price {
			idx = n.right
		} else {
			return idx
		}
	}
	return -1
}

func (t *askTree) first() int32 {
	idx := t.root
	if idx == -1 {
		return -1
	}
	for {
		n := t.arena.get(idx)
		if n.left == -1 {
			return idx
		}
		idx = n.left
	}
}

func (t *askTree) next(idx int32) int32 {
	if idx == -1 {
		return -1
	}
	n := t.arena.get(idx)
	if n.right != -1 {
		idx = n.right
		for {
			n = t.arena.get(idx)
			if n.left == -1 {
				return idx
			}
			idx = n.left
		}
	}
	for n.parent != -1 {
		p := t.arena.get(n.parent)
		if idx != p.right {
			return n.parent
		}
		idx = n.parent
		n = p
	}
	return -1
}

func (t *askTree) rotateLeft(idx int32) {
	n := t.arena.get(idx)
	r := n.right
	rn := t.arena.get(r)

	n.right = rn.left
	if rn.left != -1 {
		t.arena.get(rn.left).parent = idx
	}

	rn.parent = n.parent
	if n.parent == -1 {
		t.root = r
	} else {
		p := t.arena.get(n.parent)
		if idx == p.left {
			p.left = r
		} else {
			p.right = r
		}
	}

	rn.left = idx
	n.parent = r
}

func (t *askTree) rotateRight(idx int32) {
	n := t.arena.get(idx)
	l := n.left
	ln := t.arena.get(l)

	n.left = ln.right
	if ln.right != -1 {
		t.arena.get(ln.right).parent = idx
	}

	ln.parent = n.parent
	if n.parent == -1 {
		t.root = l
	} else {
		p := t.arena.get(n.parent)
		if idx == p.right {
			p.right = l
		} else {
			p.left = l
		}
	}

	ln.right = idx
	n.parent = l
}

func (t *askTree) insertFixup(idx int32) {
	for idx != t.root {
		n := t.arena.get(idx)
		if n.parent == -1 {
			break
		}
		p := t.arena.get(n.parent)
		if !p.red {
			break
		}

		gp := t.arena.get(p.parent)
		if n.parent == gp.left {
			uncle := gp.right
			if uncle != -1 && t.arena.get(uncle).red {
				p.red = false
				t.arena.get(uncle).red = false
				gp.red = true
				idx = p.parent
			} else {
				if idx == p.right {
					idx = n.parent
					t.rotateLeft(idx)
					n = t.arena.get(idx)
					p = t.arena.get(n.parent)
					gp = t.arena.get(p.parent)
				}
				p.red = false
				gp.red = true
				t.rotateRight(p.parent)
			}
		} else {
			uncle := gp.left
			if uncle != -1 && t.arena.get(uncle).red {
				p.red = false
				t.arena.get(uncle).red = false
				gp.red = true
				idx = p.parent
			} else {
				if idx == p.left {
					idx = n.parent
					t.rotateRight(idx)
					n = t.arena.get(idx)
					p = t.arena.get(n.parent)
					gp = t.arena.get(p.parent)
				}
				p.red = false
				gp.red = true
				t.rotateLeft(p.parent)
			}
		}
	}
	t.arena.get(t.root).red = false
}

func (t *askTree) insert(price, size decimal.Decimal) int32 {
	idx := t.arena.alloc(price, size)

	if t.root == -1 {
		t.root = idx
		t.arena.get(idx).red = false
		return idx
	}

	// Find insertion point (ascending: lower prices go left)
	cur := t.root
	for {
		cn := t.arena.get(cur)
		if price < cn.price {
			if cn.left == -1 {
				cn.left = idx
				t.arena.get(idx).parent = cur
				break
			}
			cur = cn.left
		} else {
			if cn.right == -1 {
				cn.right = idx
				t.arena.get(idx).parent = cur
				break
			}
			cur = cn.right
		}
	}

	t.insertFixup(idx)
	return idx
}

func (t *askTree) transplant(u, v int32) {
	un := t.arena.get(u)
	if un.parent == -1 {
		t.root = v
	} else {
		p := t.arena.get(un.parent)
		if u == p.left {
			p.left = v
		} else {
			p.right = v
		}
	}
	if v != -1 {
		t.arena.get(v).parent = un.parent
	}
}

func (t *askTree) removeFixup(idx, parent int32) {
	for idx != t.root && (idx == -1 || !t.arena.get(idx).red) {
		if parent == -1 {
			break
		}
		pn := t.arena.get(parent)
		if idx == pn.left {
			sib := pn.right
			sn := t.arena.get(sib)
			if sn.red {
				sn.red = false
				pn.red = true
				t.rotateLeft(parent)
				pn = t.arena.get(parent)
				sib = pn.right
				sn = t.arena.get(sib)
			}
			leftBlack := sn.left == -1 || !t.arena.get(sn.left).red
			rightBlack := sn.right == -1 || !t.arena.get(sn.right).red
			if leftBlack && rightBlack {
				sn.red = true
				idx = parent
				parent = t.arena.get(idx).parent
			} else {
				if rightBlack {
					if sn.left != -1 {
						t.arena.get(sn.left).red = false
					}
					sn.red = true
					t.rotateRight(sib)
					pn = t.arena.get(parent)
					sib = pn.right
					sn = t.arena.get(sib)
				}
				sn.red = pn.red
				pn.red = false
				if sn.right != -1 {
					t.arena.get(sn.right).red = false
				}
				t.rotateLeft(parent)
				idx = t.root
				break
			}
		} else {
			sib := pn.left
			sn := t.arena.get(sib)
			if sn.red {
				sn.red = false
				pn.red = true
				t.rotateRight(parent)
				pn = t.arena.get(parent)
				sib = pn.left
				sn = t.arena.get(sib)
			}
			leftBlack := sn.left == -1 || !t.arena.get(sn.left).red
			rightBlack := sn.right == -1 || !t.arena.get(sn.right).red
			if leftBlack && rightBlack {
				sn.red = true
				idx = parent
				parent = t.arena.get(idx).parent
			} else {
				if leftBlack {
					if sn.right != -1 {
						t.arena.get(sn.right).red = false
					}
					sn.red = true
					t.rotateLeft(sib)
					pn = t.arena.get(parent)
					sib = pn.left
					sn = t.arena.get(sib)
				}
				sn.red = pn.red
				pn.red = false
				if sn.left != -1 {
					t.arena.get(sn.left).red = false
				}
				t.rotateRight(parent)
				idx = t.root
				break
			}
		}
	}
	if idx != -1 {
		t.arena.get(idx).red = false
	}
}

func (t *askTree) minimum(idx int32) int32 {
	for {
		n := t.arena.get(idx)
		if n.left == -1 {
			return idx
		}
		idx = n.left
	}
}

func (t *askTree) remove(idx int32) {
	n := t.arena.get(idx)
	var x, xParent int32
	y := idx
	yRed := n.red

	if n.left == -1 {
		x = n.right
		xParent = n.parent
		t.transplant(idx, n.right)
	} else if n.right == -1 {
		x = n.left
		xParent = n.parent
		t.transplant(idx, n.left)
	} else {
		y = t.minimum(n.right)
		yn := t.arena.get(y)
		yRed = yn.red
		x = yn.right
		if yn.parent == idx {
			xParent = y
		} else {
			xParent = yn.parent
			t.transplant(y, yn.right)
			yn = t.arena.get(y)
			yn.right = n.right
			t.arena.get(yn.right).parent = y
		}
		t.transplant(idx, y)
		yn = t.arena.get(y)
		yn.left = n.left
		t.arena.get(yn.left).parent = y
		yn.red = n.red
	}

	if !yRed {
		t.removeFixup(x, xParent)
	}

	t.arena.release(idx)
}
