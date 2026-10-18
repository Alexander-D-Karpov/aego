package world

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"unsafe"
)

type Snapshot struct {
	cfg      Config
	bodies   pool[body]
	shapes   pool[shape]
	joints   pool[joint]
	tree     tree
	prox     []proxyInfo
	moveBuf  []int32
	pairs    pairMap
	touch    pairMap
	contacts []contact
	freeC    []int32
	layers   [32]uint32
	pending  []ContactEvent
	dyingB   []BodyHandle
	deadB    []BodyHandle
	dyingS   []ShapeHandle
	deadS    []ShapeHandle
	mark     uint32
	steps    uint64
	awake    []int32
	raw      []byte
}

func cloneBody(dst, src *body) {
	sh, jo, co := dst.shapes, dst.joints, dst.contacts
	*dst = *src
	dst.shapes = append(sh[:0], src.shapes...)
	dst.joints = append(jo[:0], src.joints...)
	dst.contacts = append(co[:0], src.contacts...)
}

func cloneShape(dst, src *shape) {
	px, ch := dst.proxies, dst.g.chain
	*dst = *src
	dst.proxies = append(px[:0], src.proxies...)
	if src.g.chain == nil {
		return
	}
	if ch == nil || ch == src.g.chain {
		ch = &chainData{}
	}
	pts := ch.pts
	*ch = *src.g.chain
	ch.pts = append(pts[:0], src.g.chain.pts...)
	dst.g.chain = ch
}

func (w *World) Snapshot() *Snapshot {
	s := &Snapshot{}
	w.SnapshotInto(s)
	return s
}

func (w *World) SnapshotInto(s *Snapshot) {
	s.cfg = w.cfg
	w.bodies.cloneInto(&s.bodies, cloneBody)
	w.shapes.cloneInto(&s.shapes, cloneShape)
	w.joints.cloneInto(&s.joints, nil)
	w.tree.cloneInto(&s.tree)
	s.prox = append(s.prox[:0], w.prox...)
	s.moveBuf = append(s.moveBuf[:0], w.moveBuf...)
	w.pairs.cloneInto(&s.pairs)
	w.touch.cloneInto(&s.touch)
	s.contacts = append(s.contacts[:0], w.contacts...)
	s.freeC = append(s.freeC[:0], w.freeC...)
	s.layers = w.layers
	s.pending = append(s.pending[:0], w.pending...)
	s.dyingB = append(s.dyingB[:0], w.dyingB...)
	s.deadB = append(s.deadB[:0], w.deadB...)
	s.dyingS = append(s.dyingS[:0], w.dyingS...)
	s.deadS = append(s.deadS[:0], w.deadS...)
	s.awake = append(s.awake[:0], w.awake...)
	s.mark = w.mark
	s.steps = w.steps
	s.raw = nil
}

func (w *World) Restore(s *Snapshot) {
	if s == nil {
		return
	}
	w.cfg = s.cfg
	s.bodies.cloneInto(&w.bodies, cloneBody)
	s.shapes.cloneInto(&w.shapes, cloneShape)
	s.joints.cloneInto(&w.joints, nil)
	s.tree.cloneInto(&w.tree)
	w.prox = append(w.prox[:0], s.prox...)
	w.moveBuf = append(w.moveBuf[:0], s.moveBuf...)
	s.pairs.cloneInto(&w.pairs)
	s.touch.cloneInto(&w.touch)
	w.contacts = append(w.contacts[:0], s.contacts...)
	w.freeC = append(w.freeC[:0], s.freeC...)
	w.layers = s.layers
	w.pending = append(w.pending[:0], s.pending...)
	w.dyingB = append(w.dyingB[:0], s.dyingB...)
	w.deadB = append(w.deadB[:0], s.deadB...)
	w.dyingS = append(w.dyingS[:0], s.dyingS...)
	w.deadS = append(w.deadS[:0], s.deadS...)
	w.mark = s.mark
	w.steps = s.steps
	w.events = w.events[:0]
	w.began = w.began[:0]
	w.newPairs = w.newPairs[:0]
	w.stats = Stats{Steps: s.steps}
	w.rebuildDerived()
	items := w.bodies.items
	for _, i := range w.awake {
		items[i].listed = false
	}
	w.awake = append(w.awake[:0], s.awake...)
	for _, i := range w.awake {
		items[i].listed = true
	}
	w.countStats()
}

func (s *Snapshot) Size() int {
	return len(s.bodies.items)*int(unsafe.Sizeof(body{})) +
		len(s.shapes.items)*int(unsafe.Sizeof(shape{})) +
		len(s.joints.items)*int(unsafe.Sizeof(joint{})) +
		len(s.tree.nodes)*int(unsafe.Sizeof(treeNode{})) +
		len(s.prox)*int(unsafe.Sizeof(proxyInfo{})) +
		len(s.contacts)*int(unsafe.Sizeof(contact{})) +
		12*(len(s.pairs.keys)+len(s.touch.keys)) +
		4*(len(s.awake)+len(s.moveBuf)+len(s.freeC))
}

type enc struct {
	b []byte
}

func (e *enc) u32(v uint32) {
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
}

func (e *enc) u64(v uint64) {
	e.b = binary.LittleEndian.AppendUint64(e.b, v)
}

func (e *enc) i32(v int32) {
	e.u32(uint32(v))
}

func (e *enc) f32(v float32) {
	e.u32(math.Float32bits(v))
}

func (e *enc) v2(v Vec2) {
	e.f32(v.X)
	e.f32(v.Y)
}

func (e *enc) flag(v bool) {
	if v {
		e.b = append(e.b, 1)
		return
	}
	e.b = append(e.b, 0)
}

func (e *enc) box(a AABB) {
	e.v2(a.Min)
	e.v2(a.Max)
}

func (e *enc) handle(index, gen uint32) {
	e.u32(index)
	e.u32(gen)
}

func (e *enc) manifold(m *Manifold) {
	e.v2(m.Normal)
	e.u32(uint32(m.Count))
	for i := 0; i < m.Count; i++ {
		p := &m.Points[i]
		e.v2(p.Point)
		e.v2(p.AnchorA)
		e.v2(p.AnchorB)
		e.f32(p.Separation)
		e.f32(p.NormalImpulse)
		e.f32(p.TangentImpulse)
		e.f32(p.TotalNormalImpulse)
		e.u32(uint32(p.ID))
		e.flag(p.Persisted)
	}
}

func poolHeader[T any](e *enc, p *pool[T]) {
	e.u32(uint32(len(p.items)))
	for i := range p.items {
		e.u32(p.gens[i])
		e.flag(p.alive[i])
	}
	e.u32(uint32(len(p.free)))
	for _, f := range p.free {
		e.u32(f)
	}
}

func (s *Snapshot) Bytes() []byte {
	if s.raw != nil {
		return s.raw
	}
	e := &enc{b: make([]byte, 0, 4096)}
	e.u64(s.steps)
	e.u32(s.mark)
	poolHeader(e, &s.bodies)
	for i := range s.bodies.items {
		if !s.bodies.alive[i] {
			continue
		}
		b := &s.bodies.items[i]
		e.u32(uint32(b.typ))
		e.v2(b.xf.p)
		e.f32(b.xf.q.C)
		e.f32(b.xf.q.S)
		e.f32(b.angle)
		e.v2(b.localCenter)
		e.v2(b.center)
		e.v2(b.v)
		e.f32(b.w)
		e.v2(b.force)
		e.f32(b.torque)
		e.f32(b.mass)
		e.f32(b.inertia)
		e.f32(b.sleepTime)
		e.flag(b.awake)
		e.flag(b.dead)
		e.u32(uint32(b.layer))
		e.u32(b.mask)
		e.u32(b.mark)
		e.u32(uint32(len(b.shapes)))
		for _, h := range b.shapes {
			e.handle(h.Index, h.Gen)
		}
		e.u32(uint32(len(b.joints)))
		for _, h := range b.joints {
			e.handle(h.Index, h.Gen)
		}
		e.u32(uint32(len(b.contacts)))
		for _, c := range b.contacts {
			e.i32(c)
		}
	}
	poolHeader(e, &s.shapes)
	for i := range s.shapes.items {
		if !s.shapes.alive[i] {
			continue
		}
		sh := &s.shapes.items[i]
		e.handle(sh.body.Index, sh.body.Gen)
		e.u32(uint32(sh.g.kind))
		e.flag(sh.trigger)
		e.flag(sh.dead)
		e.flag(sh.disabled)
		e.f32(sh.friction)
		e.f32(sh.restitution)
		e.f32(sh.density)
		for _, id := range sh.proxies {
			e.i32(id)
		}
	}
	poolHeader(e, &s.joints)
	for i := range s.joints.items {
		if !s.joints.alive[i] {
			continue
		}
		j := &s.joints.items[i]
		e.u32(uint32(j.kind))
		e.handle(j.a.Index, j.a.Gen)
		e.handle(j.b.Index, j.b.Gen)
		e.f32(j.impulse)
		e.f32(j.lowerImp)
		e.f32(j.upperImp)
		e.v2(j.pImpulse)
		e.f32(j.motorImp)
		e.f32(j.speed)
		e.u32(j.mark)
	}
	e.i32(s.tree.root)
	e.i32(s.tree.free)
	e.u32(uint32(len(s.tree.nodes)))
	for i := range s.tree.nodes {
		n := &s.tree.nodes[i]
		e.box(n.box)
		e.i32(n.parent)
		e.i32(n.child1)
		e.i32(n.child2)
		e.i32(n.height)
		e.i32(n.data)
	}
	for i := range s.prox {
		p := &s.prox[i]
		e.handle(p.shape.Index, p.shape.Gen)
		e.i32(p.child)
		e.box(p.tight)
		e.flag(p.alive)
		e.flag(p.moved)
	}
	e.u32(uint32(len(s.moveBuf)))
	for _, id := range s.moveBuf {
		e.i32(id)
	}
	e.u32(uint32(len(s.contacts)))
	for i := range s.contacts {
		c := &s.contacts[i]
		e.flag(c.alive)
		if !c.alive {
			continue
		}
		e.handle(c.shapeA.Index, c.shapeA.Gen)
		e.handle(c.shapeB.Index, c.shapeB.Gen)
		e.i32(c.childA)
		e.i32(c.childB)
		e.u64(c.key)
		e.flag(c.touching)
		e.u32(c.mark)
		e.manifold(&c.m)
	}
	e.u32(uint32(len(s.freeC)))
	for _, f := range s.freeC {
		e.i32(f)
	}
	for _, m := range [...]*pairMap{&s.pairs, &s.touch} {
		e.u32(uint32(len(m.keys)))
		for i, k := range m.keys {
			e.u64(k)
			e.i32(m.vals[i])
		}
	}
	for _, l := range s.layers {
		e.u32(l)
	}
	e.u32(uint32(len(s.awake)))
	for _, i := range s.awake {
		e.i32(i)
	}
	s.raw = e.b
	return s.raw
}

func (s *Snapshot) Equal(o *Snapshot) bool {
	return bytes.Equal(s.Bytes(), o.Bytes())
}

func (s *Snapshot) Hash() [32]byte {
	return sha256.Sum256(s.Bytes())
}
