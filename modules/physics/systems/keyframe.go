package systems

import (
	"slices"
	"unsafe"

	"aego/core/mathx"
	"aego/core/rewind"
	"aego/modules/physics/world"
	"aego/scene"
)

var _ rewind.Keyframer = (*Sim)(nil)

type simKey struct {
	snap   world.Snapshot
	bodies []bodyRec
	shapes []shapeRec
	pts    []mathx.Vec2
	npts   []int32
	joints []jointRec
	touch  []Touch
	diags  []Diag
	dirty  bool
	mark   uint32
}

func (k *simKey) Size() int {
	return k.snap.Size() +
		len(k.bodies)*int(unsafe.Sizeof(bodyRec{})) +
		len(k.shapes)*int(unsafe.Sizeof(shapeRec{})) +
		len(k.pts)*int(unsafe.Sizeof(mathx.Vec2{})) + 4*len(k.npts) +
		len(k.joints)*int(unsafe.Sizeof(jointRec{})) +
		len(k.touch)*int(unsafe.Sizeof(Touch{}))
}

func recAt[T any](old []*T, i int) *T {
	if i < len(old) {
		return old[i]
	}
	return new(T)
}

func (m *Sim) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*simKey)
	if k == nil {
		k = &simKey{}
	}
	m.w.SnapshotInto(&k.snap)
	k.bodies = k.bodies[:0]
	for _, b := range m.bodies {
		k.bodies = append(k.bodies, *b)
	}
	k.shapes, k.pts, k.npts = k.shapes[:0], k.pts[:0], k.npts[:0]
	for _, sh := range m.shapes {
		k.npts = append(k.npts, int32(len(sh.sig.pts)))
		k.pts = append(k.pts, sh.sig.pts...)
		k.shapes = append(k.shapes, *sh)
		k.shapes[len(k.shapes)-1].sig.pts = nil
	}
	k.joints = k.joints[:0]
	for _, j := range m.joints {
		k.joints = append(k.joints, *j)
	}
	k.touch = append(k.touch[:0], m.touch...)
	k.diags = append(k.diags[:0], m.diags...)
	k.dirty, k.mark = m.dirty, m.mark
	return k
}

func (m *Sim) Restore(key rewind.Key) {
	k := key.(*simKey)
	m.w.Restore(&k.snap)
	oldB := slices.Clone(m.bodies)
	clear(m.byBody)
	m.bodies = m.bodies[:0]
	for i := range k.bodies {
		b := recAt(oldB, i)
		*b = k.bodies[i]
		m.bodies = append(m.bodies, b)
		m.byBody[b.node] = b
	}
	oldS := slices.Clone(m.shapes)
	clear(m.byShape)
	m.shapes = m.shapes[:0]
	off := 0
	for i := range k.shapes {
		sh := recAt(oldS, i)
		prev := sh.sig.pts
		*sh = k.shapes[i]
		n := int(k.npts[i])
		sh.sig.pts = append(prev[:0], k.pts[off:off+n]...)
		off += n
		m.shapes = append(m.shapes, sh)
		m.byShape[sh.key] = sh
	}
	oldJ := slices.Clone(m.joints)
	clear(m.byJoint)
	m.joints = m.joints[:0]
	for i := range k.joints {
		j := recAt(oldJ, i)
		*j = k.joints[i]
		m.joints = append(m.joints, j)
		m.byJoint[j.key] = j
	}
	m.touch = append(m.touch[:0], k.touch...)
	m.diags = append(m.diags[:0], k.diags...)
	m.diagRev++
	m.dirty, m.mark = k.dirty, k.mark
}

func hashNode(h *rewind.Hasher, n scene.NodeHandle) {
	h.U32(n.Index)
	h.U32(n.Gen)
}

func hashPose(h *rewind.Hasher, p pose) {
	h.F32(p.p.X)
	h.F32(p.p.Y)
	h.F32(p.a)
}

func (m *Sim) Hash(h *rewind.Hasher) {
	h.String("physics")
	var snap world.Snapshot
	m.w.SnapshotInto(&snap)
	h.Bytes(snap.Bytes())
	h.Int(len(m.bodies))
	for _, b := range m.bodies {
		hashNode(h, b.node)
		hashNode(h, b.parent)
		h.U32(b.h.Index)
		h.U32(b.h.Gen)
		h.Bool(b.implicit)
		h.Bool(b.excluded)
		h.Bool(b.warned)
		h.U32(b.mark)
		hashPose(h, b.prev)
		hashPose(h, b.cur)
		h.F32(b.local.Position.X)
		h.F32(b.local.Position.Y)
		h.F32(b.local.Rotation)
		h.F32(b.local.Scale.X)
		h.F32(b.local.Scale.Y)
		for _, v := range b.world {
			h.F32(v)
		}
	}
	h.Int(len(m.shapes))
	for _, sh := range m.shapes {
		hashNode(h, sh.key.node)
		h.U64(uint64(sh.key.kind))
		hashNode(h, sh.owner)
		h.U32(sh.body.Index)
		h.U32(sh.body.Gen)
		h.U32(sh.h.Index)
		h.U32(sh.h.Gen)
		h.Bool(sh.built)
		h.Bool(sh.bad)
		h.Bool(sh.enabled)
		h.U32(sh.mark)
		for _, v := range sh.rel {
			h.F32(v)
		}
	}
	h.Int(len(m.joints))
	for _, j := range m.joints {
		hashNode(h, j.key.node)
		h.U64(uint64(j.key.kind))
		h.U32(j.h.Index)
		h.U32(j.h.Gen)
		h.U32(j.a.Index)
		h.U32(j.b.Index)
		h.Bool(j.built)
		h.Bool(j.failed)
		h.U32(j.mark)
	}
	h.Int(len(m.touch))
	for _, t := range m.touch {
		h.U32(t.ShapeA.Index)
		h.U32(t.ShapeA.Gen)
		h.U32(t.ShapeB.Index)
		h.U32(t.ShapeB.Gen)
		h.Bool(t.Fresh)
	}
	h.Bool(m.dirty)
	h.U32(m.mark)
}
