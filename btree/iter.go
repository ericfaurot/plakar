package btree

type Iterator[K, V any] interface {
	Next() bool
	Current() (K, V)
	Err() error
}

type forwardIter[K, P any, V any] struct {
	b       *BTree[K, P, V]
	ptr     P
	current *Node[K, P, V]
	err     error
	idx     int
}

func (fit *forwardIter[K, P, V]) Next() bool {
	if fit.err != nil {
		return false
	}

	fit.idx++
	if fit.idx < len(fit.current.Values) {
		return true
	}

	if fit.current.Next == nil {
		return false
	}
	nextptr := *fit.current.Next

	next, err := fit.b.store.Get(nextptr)
	if err != nil {
		fit.err = err
		return false
	}

	fit.ptr = nextptr
	fit.current = next
	fit.idx = 0
	return true
}

func (fit *forwardIter[K, P, V]) Current() (K, V) {
	return fit.current.Keys[fit.idx], fit.current.Values[fit.idx]
}

func (fit *forwardIter[K, P, V]) Err() error {
	return fit.err
}

// ScanAll returns an iterator that visits all the values from the
// smaller one onwards.
func (b *BTree[K, P, V]) ScanAll() (Iterator[K, V], error) {
	ptr := b.Root

	var n *Node[K, P, V]
	for {
		node, err := b.store.Get(ptr)
		if err != nil {
			return nil, err
		}

		if node.isleaf() {
			n = node
			break
		}
		ptr = node.Pointers[0]
	}

	return &forwardIter[K, P, V]{
		b:       b,
		ptr:     ptr,
		current: n,
		idx:     -1,
	}, nil
}

// ScanFrom returns an iterator that visits all the values starting
// from the given key, or the first key larger than the given one,
// onwards.
func (b *BTree[K, P, V]) ScanFrom(key K) (Iterator[K, V], error) {
	node, path, err := b.findleaf(key)
	if err != nil {
		return nil, err
	}

	ptr := path[len(path)-1]

	var (
		idx   int
		found bool
	)
	for idx = range node.Keys {
		if b.compare(key, node.Keys[idx]) <= 0 {
			found = true
			break
		}
	}
	if !found {
		if node.Next == nil {
			idx++ // key not found, make an empty iterator
		} else {
			ptr = *node.Next
			node, err = b.store.Get(ptr)
			if err != nil {
				return nil, err
			}
			idx = 0
		}
	}

	idx-- // forwardIter.Next() will bump this
	return &forwardIter[K, P, V]{
		b:       b,
		ptr:     ptr,
		current: node,
		idx:     idx,
	}, nil
}

type step[K, P, V any] struct {
	ptr  P
	idx  int
}

type backwardIter[K, P, V any] struct {
	b     *BTree[K, P, V]
	cur   *Node[K, P, V]
	steps []step[K, P, V]
	err   error
}

func (bit *backwardIter[K, P, V]) dive(ptr P) error {
	for {
		node, err := bit.b.store.Get(ptr)
		if err != nil {
			return err
		}

		bit.steps = append(bit.steps, step[K, P, V]{
			ptr:  ptr,
			idx:  len(node.Keys),
		})

		if node.isleaf() {
			bit.cur = node
			return nil
		}
		ptr = node.Pointers[len(node.Keys)]
	}
}

func (bit *backwardIter[K, P, V]) Next() bool {
	if bit.err != nil {
		return false
	}

	if len(bit.steps) == 0 {
		return false
	}

	if bit.steps[len(bit.steps)-1].idx > 0 {
		bit.steps[len(bit.steps)-1].idx--
		return true
	}

	// rewinding upwards to then dive down
	for len(bit.steps) > 1 {
		// discard last step
		bit.steps = bit.steps[:len(bit.steps)-1]
		last := bit.steps[len(bit.steps)-1]
		if last.idx == 0 {
			// bubble up once more
			continue
		}

		// fetch parent node
		node, err := bit.b.store.Get(last.ptr)
		if err != nil {
			bit.err = err
			return false
		}

		bit.steps[len(bit.steps)-1].idx--
		bit.err = bit.dive(node.Pointers[bit.steps[len(bit.steps)-1].idx])
		bit.steps[len(bit.steps)-1].idx--
		return bit.err == nil
	}

	return false
}

func (bit *backwardIter[K, P, V]) Current() (K, V) {
	last := bit.steps[len(bit.steps)-1]
	return bit.cur.Keys[last.idx], bit.cur.Values[last.idx]
}

func (bit *backwardIter[K, P, V]) Err() error {
	return bit.err
}

func (b *BTree[K, P, V]) ScanAllReverse() (Iterator[K, V], error) {
	bit := &backwardIter[K, P, V]{
		b: b,
	}

	if err := bit.dive(b.Root); err != nil {
		return nil, err
	}

	return bit, nil
}

func (b *BTree[K, P, V]) VisitDFS(cb func(P, *Node[K, P, V]) error) error {
	stack := []step[K, P, V]{{b.Root, -1}}
	for len(stack) > 0 {
		l := &stack[len(stack)-1]

		node, err := b.store.Get(l.ptr)
		if err != nil {
			return err
		}
		if l.idx == -1 {
			if err := cb(l.ptr, node); err != nil {
				return err
			}
		}
		l.idx++

		if l.idx == len(node.Pointers) {
			stack = stack[:len(stack)-1]
			continue
		}
		stack = append(stack, step[K, P, V]{node.Pointers[l.idx], -1})
	}
	return nil
}
