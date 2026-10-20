package scene

import (
	"aego/core/handle"
	"aego/core/rewind"
	"aego/schema"
	"reflect"
	"unsafe"
)

const pendingChunk = 64

type sparseIndex struct {
	dense []int32
}

const (
	slotEmpty     int32 = -1
	tombstoneBias int32 = -3
)

func (s *sparseIndex) get(idx uint32) int32 {
	if int(idx) >= len(s.dense) {
		return slotEmpty
	}
	return s.dense[idx]
}

func (s *sparseIndex) set(idx uint32, v int32) {
	for int(idx) >= len(s.dense) {
		s.dense = append(s.dense, slotEmpty)
	}
	s.dense[idx] = v
}

func (s *sparseIndex) tomb(slot int32) int32 {
	return tombstoneBias - slot
}

func (s *sparseIndex) tombSlot(v int32) int32 {
	return tombstoneBias - v
}

func (s *sparseIndex) isTomb(v int32) bool {
	return v <= tombstoneBias
}

type anyStore interface {
	typeID() schema.TypeID
	descriptor() *schema.TypeDescriptor
	hasNode(h NodeHandle) bool
	pointer(h NodeHandle) (unsafe.Pointer, bool)
	addRaw(h NodeHandle) (unsafe.Pointer, bool)
	removeNode(h NodeHandle)
	dropNode(h NodeHandle)
	flush(s *Scene)
	count() int
	collect(out []NodeHandle, all bool) int
	capture(reuse rewind.Key) rewind.Key
	restore(k rewind.Key)
	hash(h *rewind.Hasher)
}

type pendingEntry[T any] struct {
	node  NodeHandle
	chunk int
	slot  int
}

type ComponentStore[T any] struct {
	scene   *Scene
	desc    *schema.TypeDescriptor
	sparse  sparseIndex
	nodes   []NodeHandle
	data    []T
	pending []pendingEntry[T]
	chunks  [][]T
	dead    []NodeHandle
}

func (c *ComponentStore[T]) typeID() schema.TypeID {
	return c.desc.ID
}

func (c *ComponentStore[T]) descriptor() *schema.TypeDescriptor {
	return c.desc
}

func (c *ComponentStore[T]) Type() schema.TypeID {
	return c.desc.ID
}

func (c *ComponentStore[T]) Len() int {
	return len(c.data)
}

func (c *ComponentStore[T]) count() int {
	return len(c.data) + len(c.pending)
}

func (c *ComponentStore[T]) Nodes() []NodeHandle {
	return c.nodes
}

func (c *ComponentStore[T]) Data() []T {
	return c.data
}

func (c *ComponentStore[T]) pendingIndex(h NodeHandle) int {
	for i := range c.pending {
		if c.pending[i].node == h {
			return i
		}
	}
	return -1
}

func (c *ComponentStore[T]) pendingPtr(i int) *T {
	e := &c.pending[i]
	return &c.chunks[e.chunk][e.slot]
}

func (c *ComponentStore[T]) get(h NodeHandle) (*T, bool) {
	if !c.scene.IsAlive(h) {
		return nil, false
	}
	if slot := c.sparse.get(h.Index); slot >= 0 {
		if c.nodes[slot] == h {
			return &c.data[slot], true
		}
		return nil, false
	} else if c.sparse.isTomb(slot) {
		return nil, false
	}
	if i := c.pendingIndex(h); i >= 0 {
		return c.pendingPtr(i), true
	}
	return nil, false
}

func (c *ComponentStore[T]) add(h NodeHandle) (*T, bool) {
	n, ok := c.scene.node(h)
	if !ok || n.has(flagPendingDestroy) {
		return nil, false
	}
	slot := c.sparse.get(h.Index)
	if slot >= 0 && c.nodes[slot] == h {
		return &c.data[slot], true
	}
	if c.sparse.isTomb(slot) {
		if p := c.reviveTombstone(h, slot); p != nil {
			return p, true
		}
	}
	if i := c.pendingIndex(h); i >= 0 {
		return c.pendingPtr(i), true
	}
	chunk := len(c.chunks) - 1
	if chunk < 0 || len(c.chunks[chunk]) == pendingChunk {
		c.chunks = append(c.chunks, make([]T, 0, pendingChunk))
		chunk = len(c.chunks) - 1
	}
	var zero T
	c.chunks[chunk] = append(c.chunks[chunk], zero)
	slotIdx := len(c.chunks[chunk]) - 1
	c.pending = append(c.pending, pendingEntry[T]{node: h, chunk: chunk, slot: slotIdx})
	c.scene.queueStructural()
	c.scene.emit(Event{Kind: EvComponentAdded, Node: h, ID: c.scene.ID(h), Type: c.desc.ID})
	return &c.chunks[chunk][slotIdx], true
}

func (c *ComponentStore[T]) reviveTombstone(h NodeHandle, marker int32) *T {
	slot := c.sparse.tombSlot(marker)
	if int(slot) >= len(c.nodes) || c.nodes[slot] != h {
		return nil
	}
	for i := range c.dead {
		if c.dead[i] == h {
			c.dead = append(c.dead[:i], c.dead[i+1:]...)
			break
		}
	}
	var zero T
	c.data[slot] = zero
	c.sparse.set(h.Index, slot)
	c.scene.emit(Event{Kind: EvComponentAdded, Node: h, ID: c.scene.ID(h), Type: c.desc.ID})
	return &c.data[slot]
}

func (c *ComponentStore[T]) hasNode(h NodeHandle) bool {
	_, ok := c.get(h)
	return ok
}

func (c *ComponentStore[T]) pointer(h NodeHandle) (unsafe.Pointer, bool) {
	p, ok := c.get(h)
	if !ok {
		return nil, false
	}
	return unsafe.Pointer(p), true
}

func (c *ComponentStore[T]) addRaw(h NodeHandle) (unsafe.Pointer, bool) {
	p, ok := c.add(h)
	if !ok {
		return nil, false
	}
	return unsafe.Pointer(p), true
}

func (c *ComponentStore[T]) removeNode(h NodeHandle) {
	if i := c.pendingIndex(h); i >= 0 {
		c.pending = append(c.pending[:i], c.pending[i+1:]...)
		c.scene.emit(Event{Kind: EvComponentRemoved, Node: h, ID: c.scene.ID(h), Type: c.desc.ID})
		return
	}
	slot := c.sparse.get(h.Index)
	if slot < 0 || c.nodes[slot] != h {
		return
	}
	c.sparse.set(h.Index, c.sparse.tomb(slot))
	c.dead = append(c.dead, h)
	c.scene.queueStructural()
	c.scene.emit(Event{Kind: EvComponentRemoved, Node: h, ID: c.scene.ID(h), Type: c.desc.ID})
}

func (c *ComponentStore[T]) dropNode(h NodeHandle) {
	if i := c.pendingIndex(h); i >= 0 {
		c.pending = append(c.pending[:i], c.pending[i+1:]...)
	}
	slot := c.sparse.get(h.Index)
	if slot == slotEmpty {
		return
	}
	if c.sparse.isTomb(slot) {
		c.sparse.set(h.Index, slotEmpty)
		for i := range c.dead {
			if c.dead[i] == h {
				c.dead = append(c.dead[:i], c.dead[i+1:]...)
				break
			}
		}
	}
	for i := range c.nodes {
		if c.nodes[i] == h {
			c.swapRemove(i)
			break
		}
	}
	c.sparse.set(h.Index, slotEmpty)
}

func (c *ComponentStore[T]) swapRemove(slot int) {
	last := len(c.data) - 1
	if slot != last {
		c.data[slot] = c.data[last]
		c.nodes[slot] = c.nodes[last]
		c.sparse.set(c.nodes[slot].Index, int32(slot))
	}
	var zero T
	c.data[last] = zero
	c.data = c.data[:last]
	c.nodes = c.nodes[:last]
}

func (c *ComponentStore[T]) flush(s *Scene) {
	for _, h := range c.dead {
		slot := c.sparse.get(h.Index)
		if !c.sparse.isTomb(slot) {
			continue
		}
		c.sparse.set(h.Index, slotEmpty)
		for i := range c.nodes {
			if c.nodes[i] == h {
				c.swapRemove(i)
				break
			}
		}
	}
	c.dead = c.dead[:0]

	for i := range c.pending {
		e := &c.pending[i]
		if !s.IsAlive(e.node) {
			continue
		}
		if c.sparse.get(e.node.Index) >= 0 {
			continue
		}
		c.nodes = append(c.nodes, e.node)
		c.data = append(c.data, c.chunks[e.chunk][e.slot])
		c.sparse.set(e.node.Index, int32(len(c.data)-1))
	}
	c.pending = c.pending[:0]
	for i := range c.chunks {
		c.chunks[i] = c.chunks[i][:0]
	}
	if len(c.chunks) > 1 {
		c.chunks = c.chunks[:1]
	}
}

func (c *ComponentStore[T]) Each(fn func(NodeHandle, *T)) {
	s := c.scene
	for i := range c.data {
		h := c.nodes[i]
		if int(c.sparse.get(h.Index)) != i {
			continue
		}
		n, ok := s.node(h)
		if !ok || n.has(flagPendingDestroy) || !n.has(flagEffective) {
			continue
		}
		fn(h, &c.data[i])
	}
}

func (c *ComponentStore[T]) EachAll(fn func(NodeHandle, *T)) {
	for i := range c.data {
		if int(c.sparse.get(c.nodes[i].Index)) != i {
			continue
		}
		fn(c.nodes[i], &c.data[i])
	}
}

func Store[T any](s *Scene) *ComponentStore[T] {
	return storeOf[T](s)
}

func newStore[T any](s *Scene, desc *schema.TypeDescriptor) *ComponentStore[T] {
	c := &ComponentStore[T]{
		scene:  s,
		desc:   desc,
		nodes:  make([]NodeHandle, 0, 64),
		data:   make([]T, 0, 64),
		chunks: [][]T{make([]T, 0, pendingChunk)},
	}
	s.storesByGo[reflect.TypeFor[T]()] = c
	s.storesByID[desc.ID] = c
	s.storeList = append(s.storeList, c)
	return c
}

func storeOf[T any](s *Scene) *ComponentStore[T] {
	rt := reflect.TypeFor[T]()
	if existing, ok := s.storesByGo[rt]; ok {
		return existing.(*ComponentStore[T])
	}
	desc, ok := s.registry.ByGoType(rt)
	if !ok {
		panic("scene: component type " + rt.String() + " is not registered in the schema")
	}
	return newStore[T](s, desc)
}

var _ = handle.Handle[nodeData]{}

func (c *ComponentStore[T]) collect(out []NodeHandle, all bool) int {
	n := 0
	for i := range c.data {
		if n == len(out) {
			break
		}
		h := c.nodes[i]
		if int(c.sparse.get(h.Index)) != i {
			continue
		}
		nd, ok := c.scene.node(h)
		if !ok || nd.has(flagPendingDestroy) || (!all && !nd.has(flagEffective)) {
			continue
		}
		out[n] = h
		n++
	}
	return n
}
