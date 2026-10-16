package world

import (
	"slices"
	"time"
)

func (w *World) Step(dt float32) {
	if !finite(dt) || dt <= 0 {
		return
	}
	w.lastForces, w.forces = w.forces, w.lastForces[:0]
	w.events = append(w.events[:0], w.pending...)
	w.pending = w.pending[:0]
	w.releaseDead()
	w.stats.Time = StageTimes{}
	w.stats.BroadphaseCandidates = 0
	w.stats.Islands = 0
	w.inStep = true
	w.stepOnce(dt)
	w.inStep = false
	items := w.bodies.items
	for _, i := range w.awake {
		b := &items[i]
		b.force, b.torque = Vec2{}, 0
	}
	w.compactAwake()
	w.steps++
	w.countStats()
}

func (w *World) stepOnce(h float32) {
	t := time.Now()
	w.updatePairs()
	t = w.lap(t, &w.stats.Time.Broad)
	w.collide(h)
	w.lap(t, &w.stats.Time.Narrow)
	w.solve(h)
	t = time.Now()
	w.syncProxies(h)
	t = w.lap(t, &w.stats.Time.Sync)
	w.finishEvents()
	w.lap(t, &w.stats.Time.Events)
}

func (w *World) lap(start time.Time, dst *time.Duration) time.Time {
	now := time.Now()
	*dst += now.Sub(start)
	return now
}

func (w *World) releaseDead() {
	for _, h := range w.deadS {
		w.shapes.release(h)
	}
	w.deadS = append(w.deadS[:0], w.dyingS...)
	w.dyingS = w.dyingS[:0]
	for _, h := range w.deadB {
		w.bodies.release(h)
	}
	w.deadB = append(w.deadB[:0], w.dyingB...)
	w.dyingB = w.dyingB[:0]
}

func (w *World) ensureProx(id int32) {
	for int(id) >= len(w.prox) {
		w.prox = append(w.prox, proxyInfo{})
	}
}

func (w *World) createProxies(sh ShapeHandle, s *shape, b *body) {
	n := s.g.children()
	s.proxies = s.proxies[:0]
	m := w.margin()
	for ci := 0; ci < n; ci++ {
		tight := s.g.childAABB(ci, b.xf)
		id := w.tree.create(boxExpand(tight, m), 0)
		w.tree.setData(id, id)
		w.ensureProx(id)
		w.prox[id] = proxyInfo{shape: sh, child: int32(ci), tight: tight, alive: true, moved: true}
		w.moveBuf = append(w.moveBuf, id)
		s.proxies = append(s.proxies, id)
	}
}

func (w *World) destroyProxies(s *shape) {
	for _, id := range s.proxies {
		w.tree.destroy(id)
		w.prox[id] = proxyInfo{}
	}
	s.proxies = s.proxies[:0]
}

func (w *World) syncProxies(h float32) {
	items := w.bodies.items
	for _, i := range w.awake {
		b := &items[i]
		if b.dead || (!b.awake && b.mark != w.mark) {
			continue
		}
		w.syncBody(b, h)
	}
}

func (w *World) compactAwake() {
	items := w.bodies.items
	n := 0
	for _, i := range w.awake {
		b := &items[i]
		if w.bodies.alive[i] && b.awake && !b.dead {
			w.awake[n] = i
			n++
			continue
		}
		b.listed = false
	}
	w.awake = w.awake[:n]
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (w *World) syncBody(b *body, h float32) {
	m := w.margin()
	for _, sh := range b.shapes {
		s := &w.shapes.items[sh.Index]
		for ci, id := range s.proxies {
			tight := s.g.childAABB(ci, b.xf)
			p := &w.prox[id]
			p.tight = tight
			box := tight
			if b.bullet && h > 0 {
				box = boxSweep(tight, b.v.Scale(h))
			}
			if !b.bullet && boxContains(w.tree.fat(id), box) {
				continue
			}
			w.tree.move(id, boxExpand(box, m))
			if !p.moved {
				p.moved = true
				w.moveBuf = append(w.moveBuf, id)
			}
		}
	}
}

func sortKey(p *proxyInfo) uint64 {
	return uint64(p.shape.Index)<<32 | uint64(uint32(p.child))
}

func (w *World) updatePairs() {
	w.newPairs = w.newPairs[:0]
	for _, id := range w.moveBuf {
		if !w.prox[id].alive {
			continue
		}
		w.queryProxy = id
		w.tree.query(w.tree.fat(id), w.pairFn)
	}
	for _, id := range w.moveBuf {
		w.prox[id].moved = false
	}
	w.moveBuf = w.moveBuf[:0]
	w.stats.BroadphaseCandidates += len(w.newPairs)
	slices.SortFunc(w.newPairs, func(a, b candidate) int {
		switch {
		case a.lo != b.lo:
			if a.lo < b.lo {
				return -1
			}
			return 1
		case a.hi != b.hi:
			if a.hi < b.hi {
				return -1
			}
			return 1
		}
		return 0
	})
	for i, c := range w.newPairs {
		if i > 0 && w.newPairs[i-1].lo == c.lo && w.newPairs[i-1].hi == c.hi {
			continue
		}
		if _, ok := w.pairs.get(pairKey(c.a, c.b)); ok {
			continue
		}
		w.addContact(c.a, c.b)
	}
}

func (w *World) pairQuery(other int32) bool {
	id := w.queryProxy
	if other == id {
		return true
	}
	po := &w.prox[other]
	if !po.alive || (po.moved && other < id) {
		return true
	}
	pa := &w.prox[id]
	if pa.shape == po.shape {
		return true
	}
	if _, ok := w.pairs.get(pairKey(id, other)); ok {
		return true
	}
	if !w.shouldCollide(pa.shape, po.shape) {
		return true
	}
	ka, kb := sortKey(pa), sortKey(po)
	if ka > kb {
		ka, kb = kb, ka
	}
	w.newPairs = append(w.newPairs, candidate{a: id, b: other, lo: ka, hi: kb})
	return true
}

func (w *World) shouldCollide(ha, hb ShapeHandle) bool {
	a, b := &w.shapes.items[ha.Index], &w.shapes.items[hb.Index]
	if a.dead || b.dead || a.body == b.body || (a.trigger && b.trigger) {
		return false
	}
	if a.g.kind == KindChain && b.g.kind == KindChain {
		return false
	}
	ba, bb := &w.bodies.items[a.body.Index], &w.bodies.items[b.body.Index]
	if ba.typ != Dynamic && bb.typ != Dynamic {
		if !a.trigger && !b.trigger {
			return false
		}
		if ba.typ == Static && bb.typ == Static {
			return false
		}
	}
	if (w.layers[ba.layer]>>bb.layer)&1 == 0 || (ba.mask>>bb.layer)&1 == 0 || (bb.mask>>ba.layer)&1 == 0 {
		return false
	}
	for _, jh := range ba.joints {
		j := &w.joints.items[jh.Index]
		if !j.collide && (j.a == b.body || j.b == b.body) {
			return false
		}
	}
	return true
}

func (w *World) addContact(pa, pb int32) {
	A, B := w.prox[pa], w.prox[pb]
	sa, sb := &w.shapes.items[A.shape.Index], &w.shapes.items[B.shape.Index]
	swap := A.shape.Index > B.shape.Index
	switch {
	case sa.g.kind == KindChain:
		swap = false
	case sb.g.kind == KindChain:
		swap = true
	}
	if swap {
		A, B, sa, sb, pa, pb = B, A, sb, sa, pb, pa
	}
	var i int32
	if n := len(w.freeC); n > 0 {
		i = w.freeC[n-1]
		w.freeC = w.freeC[:n-1]
	} else {
		w.contacts = append(w.contacts, contact{})
		i = int32(len(w.contacts) - 1)
	}
	w.contacts[i] = contact{
		shapeA:      A.shape,
		shapeB:      B.shape,
		bodyA:       sa.body,
		bodyB:       sb.body,
		childA:      A.child,
		childB:      B.child,
		proxyA:      pa,
		proxyB:      pb,
		key:         pairKey(pa, pb),
		friction:    sqrtf(sa.friction * sb.friction),
		restitution: max(sa.restitution, sb.restitution),
		trigger:     sa.trigger || sb.trigger,
		alive:       true,
	}
	w.pairs.put(w.contacts[i].key, i)
	ba, bb := &w.bodies.items[sa.body.Index], &w.bodies.items[sb.body.Index]
	ba.contacts = append(ba.contacts, i)
	bb.contacts = append(bb.contacts, i)
}

func unlink(list []int32, i int32) []int32 {
	for k, x := range list {
		if x == i {
			last := len(list) - 1
			list[k] = list[last]
			return list[:last]
		}
	}
	return list
}

func (w *World) destroyContact(i int32, wake bool) {
	c := &w.contacts[i]
	if c.touching {
		c.touching = false
		w.touching--
		w.touchEnd(c, &c.m)
	}
	w.manifolds -= b2i(c.m.Count > 0)
	w.points -= c.m.Count
	w.pairs.del(c.key)
	ba, bb := &w.bodies.items[c.bodyA.Index], &w.bodies.items[c.bodyB.Index]
	ba.contacts = unlink(ba.contacts, i)
	bb.contacts = unlink(bb.contacts, i)
	if wake {
		w.wake(ba)
		w.wake(bb)
	}
	c.alive = false
	c.m = Manifold{}
	w.freeC = append(w.freeC, i)
}

func (w *World) dropShapeContacts(sh ShapeHandle) {
	for i := range w.contacts {
		c := &w.contacts[i]
		if c.alive && (c.shapeA == sh || c.shapeB == sh) {
			w.destroyContact(int32(i), true)
		}
	}
}

func shapePairKey(c *contact) uint64 {
	return pairKey(int32(c.shapeA.Index), int32(c.shapeB.Index))
}

func (w *World) touchBegin(i int32, c *contact) {
	k := shapePairKey(c)
	n, _ := w.touch.get(k)
	w.touch.put(k, n+1)
	if n == 0 {
		var p Vec2
		if c.m.Count > 0 {
			p = c.m.Points[0].Point
		}
		w.began = append(w.began, beginRec{c: i, rel: w.relVel(c, p)})
	}
}

func (w *World) touchEnd(c *contact, m *Manifold) {
	k := shapePairKey(c)
	n, _ := w.touch.get(k)
	if n > 1 {
		w.touch.put(k, n-1)
		return
	}
	w.touch.del(k)
	kind := EndContact
	if c.trigger {
		kind = EndTrigger
	}
	ev := w.event(c, m, kind)
	ev.NormalImpulse = 0
	if w.inStep {
		w.events = append(w.events, ev)
	} else {
		w.pending = append(w.pending, ev)
	}
}

func (w *World) relVel(c *contact, p Vec2) Vec2 {
	ba, bb := &w.bodies.items[c.bodyA.Index], &w.bodies.items[c.bodyB.Index]
	va := ba.v.Add(crossSV(ba.w, p.Sub(ba.center)))
	vb := bb.v.Add(crossSV(bb.w, p.Sub(bb.center)))
	return va.Sub(vb)
}

func (w *World) event(c *contact, m *Manifold, kind EventKind) ContactEvent {
	ev := ContactEvent{
		Kind:   kind,
		ShapeA: c.shapeA,
		ShapeB: c.shapeB,
		BodyA:  c.bodyA,
		BodyB:  c.bodyB,
		Normal: m.Normal.Neg(),
	}
	if m.Count > 0 {
		var p Vec2
		for j := 0; j < m.Count; j++ {
			p = p.Add(m.Points[j].Point)
			ev.NormalImpulse += m.Points[j].TotalNormalImpulse
		}
		ev.Point = p.Scale(1 / float32(m.Count))
	}
	ev.RelativeVelocity = w.relVel(c, ev.Point)
	return ev
}

func (w *World) collide(h float32) {
	w.seen++
	items := w.bodies.items
	cand := w.cand[:0]
	for _, bi := range w.awake {
		b := &items[bi]
		if !b.awake || b.dead {
			continue
		}
		for _, ci := range b.contacts {
			if c := &w.contacts[ci]; c.seen != w.seen {
				c.seen = w.seen
				cand = append(cand, ci)
			}
		}
	}
	slices.Sort(cand)
	w.cand = cand
	for _, ci := range cand {
		c := &w.contacts[ci]
		if !c.alive {
			continue
		}
		if !boxOverlap(w.tree.fat(c.proxyA), w.tree.fat(c.proxyB)) {
			w.destroyContact(ci, false)
			continue
		}
		w.updateContact(ci, c, &items[c.bodyA.Index], &items[c.bodyB.Index], h)
	}
}

func (w *World) updateContact(i int32, c *contact, ba, bb *body, h float32) {
	sa, sb := &w.shapes.items[c.shapeA.Index], &w.shapes.items[c.shapeB.Index]
	old := c.m
	spec := w.cfg.ContactOffset
	switch {
	case c.trigger:
		spec = 0
	case ba.bullet || bb.bullet:
		spec = max(spec, bb.v.Sub(ba.v).Len()*h)
	}
	gb := sb.g.child(int(c.childB))
	if sa.g.kind == KindChain {
		seg, g0, g3 := sa.g.chain.segment(int(c.childA))
		collideChainSegment(&seg, g0, g3, sa.oneSided, ba.xf, &gb, bb.xf, spec, &c.m)
	} else {
		ga := sa.g.c
		collideConvex(&ga, ba.xf, &gb, bb.xf, spec, &c.m)
	}
	for k := 0; k < c.m.Count; k++ {
		p := &c.m.Points[k]
		p.AnchorA = p.Point.Sub(ba.center)
		p.AnchorB = p.Point.Sub(bb.center)
		p.NormalImpulse, p.TangentImpulse, p.TotalNormalImpulse, p.Persisted = 0, 0, 0, false
		for j := 0; j < old.Count; j++ {
			if old.Points[j].ID == p.ID {
				p.NormalImpulse, p.TangentImpulse, p.Persisted = old.Points[j].NormalImpulse, old.Points[j].TangentImpulse, true
				break
			}
		}
	}
	w.manifolds += b2i(c.m.Count > 0) - b2i(old.Count > 0)
	w.points += c.m.Count - old.Count
	touching := c.m.Count > 0
	if touching && !c.trigger {
		touching = c.m.minSeparation() <= w.cfg.LinearSlop
	}
	switch {
	case touching && !c.touching:
		c.touching = true
		w.touching++
		w.touchBegin(i, c)
		w.wake(ba)
		w.wake(bb)
	case !touching && c.touching:
		c.touching = false
		w.touching--
		w.touchEnd(c, &old)
	}
}

func (w *World) finishEvents() {
	for _, r := range w.began {
		c := &w.contacts[r.c]
		if !c.alive || !c.touching {
			continue
		}
		kind := BeginContact
		if c.trigger {
			kind = BeginTrigger
		}
		ev := w.event(c, &c.m, kind)
		ev.RelativeVelocity = r.rel
		w.events = append(w.events, ev)
	}
	w.began = w.began[:0]
}

func (w *World) countStats() {
	s := &w.stats
	s.Steps = w.steps
	s.Bodies = w.liveBodies
	s.Active = len(w.awake)
	s.Sleeping = w.liveMovable - s.Active
	s.Shapes = w.liveShapes
	s.TouchingPairs = w.touching
	s.Manifolds = w.manifolds
	s.ContactPoints = w.points
	s.Joints = w.joints.count
}

func (w *World) rebuildDerived() {
	w.awake = w.awake[:0]
	w.seen = 0
	w.liveBodies, w.liveMovable, w.liveShapes = 0, 0, 0
	w.touching, w.manifolds, w.points = 0, 0, 0
	for i := range w.bodies.items {
		if !w.bodies.alive[i] {
			continue
		}
		b := &w.bodies.items[i]
		b.listed = false
		if b.dead {
			continue
		}
		w.liveBodies++
		if b.typ == Static {
			continue
		}
		w.liveMovable++
		if b.awake {
			w.listAwake(b)
		}
	}
	for i := range w.shapes.items {
		if w.shapes.alive[i] && !w.shapes.items[i].dead {
			w.liveShapes++
		}
	}
	for i := range w.contacts {
		c := &w.contacts[i]
		c.seen = 0
		if !c.alive {
			continue
		}
		w.touching += b2i(c.touching)
		w.manifolds += b2i(c.m.Count > 0)
		w.points += c.m.Count
	}
}
