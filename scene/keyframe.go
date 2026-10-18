package scene

import (
	"unsafe"

	"aego/core/handle"
	"aego/core/ids"
	"aego/core/rewind"
	"aego/schema"
)

var _ rewind.Keyframer = (*Scene)(nil)

type idEntry struct {
	id ids.NodeID
	h  NodeHandle
}

type nameEntry struct {
	key nameKey
	h   NodeHandle
}

type sceneKey struct {
	nodes  handle.Arena[nodeData]
	byID   []idEntry
	byName []nameEntry
	three  []node3
	free3  []int32
	pres   []presEntry
	space  Space
	stores []rewind.Key
	size   int
}

func (k *sceneKey) Size() int {
	return k.size
}

func keySlots[T any](s []T, n int) []T {
	if cap(s) < n {
		s = append(s[:cap(s)], make([]T, n-cap(s))...)
	}
	return s[:n]
}

func (s *Scene) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*sceneKey)
	if k == nil {
		k = &sceneKey{}
	}
	s.nodes.CopyTo(&k.nodes)
	k.byID = k.byID[:0]
	for id, h := range s.byID {
		k.byID = append(k.byID, idEntry{id: id, h: h})
	}
	k.byName = k.byName[:0]
	for key, h := range s.byName {
		k.byName = append(k.byName, nameEntry{key: key, h: h})
	}
	k.three = append(k.three[:0], s.three...)
	k.free3 = append(k.free3[:0], s.free3...)
	k.pres = append(k.pres[:0], s.pres...)
	k.space = s.defaultSpace
	k.stores = keySlots(k.stores, len(s.storeList))
	k.size = int(unsafe.Sizeof(*k)) +
		k.nodes.Slots()*int(unsafe.Sizeof(nodeData{})) +
		len(k.byID)*int(unsafe.Sizeof(idEntry{})) +
		len(k.byName)*int(unsafe.Sizeof(nameEntry{})) +
		len(k.three)*int(unsafe.Sizeof(node3{}))
	for i, st := range s.storeList {
		k.stores[i] = st.capture(k.stores[i])
		k.size += k.stores[i].Size()
	}
	return k
}

func (s *Scene) Restore(key rewind.Key) {
	k := key.(*sceneKey)
	s.nodes.RestoreFrom(&k.nodes)
	clear(s.byID)
	for _, e := range k.byID {
		s.byID[e.id] = e.h
	}
	clear(s.byName)
	for _, e := range k.byName {
		s.byName[e.key] = e.h
	}
	s.three = append(s.three[:0], k.three...)
	s.free3 = append(s.free3[:0], k.free3...)
	s.pres = append(s.pres[:0], k.pres...)
	s.render = s.render[:0]
	s.presDirty, s.presActive = true, true
	s.defaultSpace = k.space
	for i, st := range s.storeList {
		var sk rewind.Key
		if i < len(k.stores) {
			sk = k.stores[i]
		}
		st.restore(sk)
	}
	s.ops, s.pendingOps = s.ops[:0], 0
	s.orderDirty = true
	s.revision++
	s.structureRev++
	s.tags, s.tagsChecked = nil, false
	s.emit(Event{Kind: EvHierarchyChanged})
}

func hashHandle(h *rewind.Hasher, n NodeHandle) {
	h.U32(n.Index)
	h.U32(n.Gen)
}

func (s *Scene) Hash(h *rewind.Hasher) {
	h.String("scene")
	s.nodes.HashTo(h)
	h.Int(int(s.defaultSpace))
	for _, nh := range s.PreOrder() {
		n, ok := s.nodes.Get(nh)
		if !ok {
			continue
		}
		hashHandle(h, nh)
		h.U64(n.id.Hi)
		h.U64(n.id.Lo)
		h.String(s.interner.String(n.name))
		hashHandle(h, n.parent)
		hashHandle(h, n.firstChild)
		hashHandle(h, n.nextSib)
		l := &n.local
		h.F32(l.Position.X)
		h.F32(l.Position.Y)
		h.F32(l.Rotation)
		h.F32(l.Scale.X)
		h.F32(l.Scale.Y)
		h.U64(n.layers)
		h.U32(uint32(n.flags & (flagAlive | flagEnabled | flagPendingDestroy)))
		h.Int(int(n.space))
		h.Int(int(n.childCount))
		if n.slot >= 0 {
			t := &s.three[n.slot].local
			h.F32(t.Position.X)
			h.F32(t.Position.Y)
			h.F32(t.Position.Z)
			h.F32(t.Rotation.X)
			h.F32(t.Rotation.Y)
			h.F32(t.Rotation.Z)
			h.F32(t.Rotation.W)
			h.F32(t.Scale.X)
			h.F32(t.Scale.Y)
			h.F32(t.Scale.Z)
		}
	}
	h.Int(len(s.free3))
	for _, f := range s.free3 {
		h.U32(uint32(f))
	}
	for _, st := range s.storeList {
		if st.count() > 0 {
			st.hash(h)
		}
	}
}

type storeKey[T any] struct {
	nodes  []NodeHandle
	data   []T
	sparse []int32
}

func (k *storeKey[T]) Size() int {
	var zero T
	return len(k.nodes)*int(unsafe.Sizeof(NodeHandle{})) + len(k.data)*int(unsafe.Sizeof(zero)) + 4*len(k.sparse)
}

func cloneData[T any](dst, src []T, td *schema.TypeDescriptor) []T {
	if len(dst) > len(src) {
		clear(dst[len(src):])
	}
	if td.Flat || td.CloneInto == nil {
		return append(dst[:0], src...)
	}
	dst = keySlots(dst, len(src))
	for i := range src {
		td.CloneInto(unsafe.Pointer(&dst[i]), unsafe.Pointer(&src[i]))
	}
	return dst
}

func (c *ComponentStore[T]) capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*storeKey[T])
	if k == nil {
		k = &storeKey[T]{}
	}
	k.nodes = append(k.nodes[:0], c.nodes...)
	k.sparse = append(k.sparse[:0], c.sparse.dense...)
	k.data = cloneData(k.data, c.data, c.desc)
	return k
}

func (c *ComponentStore[T]) restore(key rewind.Key) {
	c.pending = c.pending[:0]
	for i := range c.chunks {
		c.chunks[i] = c.chunks[i][:0]
	}
	c.dead = c.dead[:0]
	k, _ := key.(*storeKey[T])
	if k == nil {
		clear(c.data)
		c.data, c.nodes, c.sparse.dense = c.data[:0], c.nodes[:0], c.sparse.dense[:0]
		return
	}
	c.nodes = append(c.nodes[:0], k.nodes...)
	c.sparse.dense = append(c.sparse.dense[:0], k.sparse...)
	c.data = cloneData(c.data, k.data, c.desc)
}

func (c *ComponentStore[T]) hash(h *rewind.Hasher) {
	h.String(c.desc.FullName)
	h.Int(len(c.data))
	for i := range c.data {
		hashHandle(h, c.nodes[i])
		schema.HashFields(h, c.desc, unsafe.Pointer(&c.data[i]))
	}
}
