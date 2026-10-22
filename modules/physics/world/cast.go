package world

const (
	castIters = 30
	gjkIters  = 20
)

type CastHit struct {
	Point    Vec2
	Normal   Vec2
	Fraction float32
	Depth    float32
	Shape    ShapeHandle
	Body     BodyHandle
	Initial  bool
}

type proxy struct {
	v [maxPolyVerts]Vec2
	n int
	r float32
}

func proxyOf(c *convex) proxy {
	p := proxy{n: c.count, r: c.radius}
	copy(p.v[:c.count], c.v[:c.count])
	return p
}

func (p *proxy) support(d Vec2) int {
	best, bv := 0, p.v[0].Dot(d)
	for i := 1; i < p.n; i++ {
		if v := p.v[i].Dot(d); v > bv {
			best, bv = i, v
		}
	}
	return best
}

type simplexVertex struct {
	wA, wB, w Vec2
	a         float32
	iA, iB    int
}

type simplex struct {
	v     [3]simplexVertex
	count int
}

func (s *simplex) searchDir() Vec2 {
	if s.count == 1 {
		return s.v[0].w.Neg()
	}
	e := s.v[1].w.Sub(s.v[0].w)
	if e.Cross(s.v[0].w.Neg()) > 0 {
		return crossSV(1, e)
	}
	return crossVS(e, 1)
}

func (s *simplex) witness() (Vec2, Vec2) {
	switch s.count {
	case 1:
		return s.v[0].wA, s.v[0].wB
	case 2:
		a := s.v[0].wA.Scale(s.v[0].a).Add(s.v[1].wA.Scale(s.v[1].a))
		b := s.v[0].wB.Scale(s.v[0].a).Add(s.v[1].wB.Scale(s.v[1].a))
		return a, b
	}
	p := s.v[0].wA.Scale(s.v[0].a).Add(s.v[1].wA.Scale(s.v[1].a)).Add(s.v[2].wA.Scale(s.v[2].a))
	return p, p
}

func (s *simplex) solve2() {
	w1, w2 := s.v[0].w, s.v[1].w
	e12 := w2.Sub(w1)
	d2 := -w1.Dot(e12)
	if d2 <= 0 {
		s.v[0].a, s.count = 1, 1
		return
	}
	d1 := w2.Dot(e12)
	if d1 <= 0 {
		s.v[1].a = 1
		s.v[0], s.count = s.v[1], 1
		return
	}
	inv := 1 / (d1 + d2)
	s.v[0].a, s.v[1].a = d1*inv, d2*inv
}

func (s *simplex) solve3() {
	w1, w2, w3 := s.v[0].w, s.v[1].w, s.v[2].w
	e12, e13, e23 := w2.Sub(w1), w3.Sub(w1), w3.Sub(w2)
	d12a, d12b := w2.Dot(e12), -w1.Dot(e12)
	d13a, d13b := w3.Dot(e13), -w1.Dot(e13)
	d23a, d23b := w3.Dot(e23), -w2.Dot(e23)
	n := e12.Cross(e13)
	d123a, d123b, d123c := n*w2.Cross(w3), n*w3.Cross(w1), n*w1.Cross(w2)
	switch {
	case d12b <= 0 && d13b <= 0:
		s.v[0].a, s.count = 1, 1
	case d12a > 0 && d12b > 0 && d123c <= 0:
		inv := 1 / (d12a + d12b)
		s.v[0].a, s.v[1].a, s.count = d12a*inv, d12b*inv, 2
	case d13a > 0 && d13b > 0 && d123b <= 0:
		inv := 1 / (d13a + d13b)
		s.v[0].a, s.v[2].a = d13a*inv, d13b*inv
		s.v[1], s.count = s.v[2], 2
	case d12a <= 0 && d23b <= 0:
		s.v[1].a = 1
		s.v[0], s.count = s.v[1], 1
	case d13a <= 0 && d23a <= 0:
		s.v[2].a = 1
		s.v[0], s.count = s.v[2], 1
	case d23a > 0 && d23b > 0 && d123a <= 0:
		inv := 1 / (d23a + d23b)
		s.v[1].a, s.v[2].a = d23a*inv, d23b*inv
		s.v[0], s.count = s.v[2], 2
	default:
		inv := 1 / (d123a + d123b + d123c)
		s.v[0].a, s.v[1].a, s.v[2].a = d123a*inv, d123b*inv, d123c*inv
		s.count = 3
	}
}

func gjk(a, b *proxy) (Vec2, Vec2, float32) {
	var s simplex
	s.v[0] = simplexVertex{wA: a.v[0], wB: b.v[0], w: b.v[0].Sub(a.v[0]), a: 1}
	s.count = 1
	var saveA, saveB [3]int
	for range gjkIters {
		n := s.count
		for i := 0; i < n; i++ {
			saveA[i], saveB[i] = s.v[i].iA, s.v[i].iB
		}
		switch s.count {
		case 2:
			s.solve2()
		case 3:
			s.solve3()
		}
		if s.count == 3 {
			break
		}
		d := s.searchDir()
		if d.LenSq() < fltEpsilon*fltEpsilon {
			break
		}
		nv := &s.v[s.count]
		nv.iA, nv.iB = a.support(d.Neg()), b.support(d)
		nv.wA, nv.wB = a.v[nv.iA], b.v[nv.iB]
		nv.w = nv.wB.Sub(nv.wA)
		dup := false
		for i := 0; i < n; i++ {
			if nv.iA == saveA[i] && nv.iB == saveB[i] {
				dup = true
				break
			}
		}
		if dup {
			break
		}
		s.count++
	}
	pA, pB := s.witness()
	return pA, pB, pB.Sub(pA).Len()
}

func separation(pA, pB Vec2, d float32, ca, cb, delta Vec2) Vec2 {
	if d > fltEpsilon {
		return pA.Sub(pB).Scale(1 / d)
	}
	if n, l := normalizeLen(ca.Sub(cb)); l > 0 {
		return n
	}
	if n, l := normalizeLen(delta.Neg()); l > 0 {
		return n
	}
	return Vec2{Y: -1}
}

func castPair(a *proxy, delta Vec2, b *proxy, slop float32) (CastHit, bool) {
	target, tol := 0.25*slop, 0.125*slop
	r := a.r + b.r
	moved := *a
	var t float32
	for it := 0; it < castIters; it++ {
		if t > 0 {
			off := delta.Scale(t)
			for i := 0; i < a.n; i++ {
				moved.v[i] = a.v[i].Add(off)
			}
		}
		pA, pB, d := gjk(&moved, b)
		gap := d - r
		if d < fltEpsilon {
			return CastHit{}, false
		}
		n := pB.Sub(pA).Scale(1 / d)
		if gap < target+tol {
			return CastHit{Point: pB.Sub(n.Scale(b.r)), Normal: n.Neg(), Fraction: t}, true
		}
		speed := delta.Dot(n)
		if speed <= 0 {
			return CastHit{}, false
		}
		t += (gap - target) / speed
		if t > 1 {
			return CastHit{}, false
		}
	}
	return CastHit{}, false
}

func (q *queryState) target(id int32) (*shape, convex, bool) {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if !q.accept(s) {
		return nil, convex{}, false
	}
	g := s.g.child(int(pi.child))
	c := g.transformed(q.w.bodies.items[s.body.Index].xf)
	if s.g.kind == KindChain && s.oneSided && c.n[0].Dot(q.castCentroid.Sub(c.v[0])) < 0 {
		return nil, convex{}, false
	}
	return s, c, true
}

// overlap reports how deep the mover sits in obstacle c; cores that intersect fall back to the SAT manifold.
func (q *queryState) overlap(c *convex, delta Vec2) (CastHit, bool) {
	pb := proxyOf(c)
	pA, pB, d := gjk(&q.castA, &pb)
	gap := d - q.castA.r - pb.r
	if gap >= 0 && d > fltEpsilon {
		return CastHit{}, false
	}
	if d > fltEpsilon {
		n := pA.Sub(pB).Scale(1 / d)
		return CastHit{Point: pB.Add(n.Scale(pb.r)), Normal: n, Depth: -gap, Initial: true}, true
	}
	var m Manifold
	collideConvex(c, identityXf(), &q.castC, identityXf(), 0, &m)
	if m.Count == 0 {
		n := separation(pA, pB, d, q.castCentroid, c.centroid, delta)
		return CastHit{Point: pB.Add(n.Scale(pb.r)), Normal: n, Initial: true}, true
	}
	deep := 0
	for i := 1; i < m.Count; i++ {
		if m.Points[i].Separation < m.Points[deep].Separation {
			deep = i
		}
	}
	sep := m.Points[deep].Separation
	return CastHit{Point: m.Points[deep].Point.Sub(m.Normal.Scale(0.5 * sep)), Normal: m.Normal, Depth: max(-sep, 0), Initial: true}, true
}

func (q *queryState) castHit(id int32) bool {
	s, c, ok := q.target(id)
	if !ok {
		return true
	}
	hit, ok := q.overlap(&c, q.castDelta)
	if !ok {
		pb := proxyOf(&c)
		hit, ok = castPair(&q.castA, q.castDelta, &pb, q.w.cfg.LinearSlop)
	}
	if !ok {
		return true
	}
	hit.Shape, hit.Body = q.w.prox[id].shape, s.body
	if !q.castFound || hit.Fraction < q.castBest.Fraction ||
		(hit.Fraction == q.castBest.Fraction && hit.Shape.Index < q.castBest.Shape.Index) {
		q.castBest, q.castFound = hit, true
	}
	return true
}

func (q *queryState) penHit(id int32) bool {
	s, c, ok := q.target(id)
	if !ok {
		return true
	}
	hit, ok := q.overlap(&c, Vec2{})
	if !ok {
		return true
	}
	hit.Shape, hit.Body = q.w.prox[id].shape, s.body
	q.penOut[q.penN] = hit
	q.penN++
	return q.penN < len(q.penOut)
}

func (w *World) castProbe(c *convex, delta Vec2, f QueryFilter) (CastHit, bool) {
	q := &w.q
	q.filter = f
	q.castC, q.castA, q.castCentroid, q.castDelta = *c, proxyOf(c), c.centroid, delta
	q.castBest, q.castFound = CastHit{}, false
	w.tree.query(boxSweep(c.aabb(identityXf()), delta), q.castFn)
	return q.castBest, q.castFound
}

func (w *World) CastCircle(c Vec2, r float32, delta Vec2, f QueryFilter) (CastHit, bool) {
	if !finite2(c) || !finite(r) || r <= 0 || !finite2(delta) {
		return CastHit{}, false
	}
	p := convex{count: 1, radius: r, centroid: c}
	p.v[0] = c
	return w.castProbe(&p, delta, f)
}

func (w *World) CastBox(c, half Vec2, angle float32, delta Vec2, f QueryFilter) (CastHit, bool) {
	if !finite2(c) || !finite2(half) || !finite(angle) || half.X <= 0 || half.Y <= 0 || !finite2(delta) {
		return CastHit{}, false
	}
	p := convex{count: 4}
	p.v[0] = Vec2{X: -half.X, Y: -half.Y}
	p.v[1] = Vec2{X: half.X, Y: -half.Y}
	p.v[2] = Vec2{X: half.X, Y: half.Y}
	p.v[3] = Vec2{X: -half.X, Y: half.Y}
	p.computeNormals()
	t := p.transformed(xform{p: c, q: makeRot(angle)})
	return w.castProbe(&t, delta, f)
}

func (w *World) CastCapsule(a, b Vec2, r float32, delta Vec2, f QueryFilter) (CastHit, bool) {
	if !finite2(a) || !finite2(b) || !finite(r) || r <= 0 || !finite2(delta) {
		return CastHit{}, false
	}
	if b.Sub(a).LenSq() < w.cfg.LinearSlop*w.cfg.LinearSlop {
		return w.CastCircle(a.Lerp(b, 0.5), r, delta, f)
	}
	p := convex{count: 2, radius: r, centroid: a.Lerp(b, 0.5)}
	p.v[0], p.v[1] = a, b
	p.computeNormals()
	return w.castProbe(&p, delta, f)
}

func (w *World) CastBody(h BodyHandle, offset, delta Vec2, f QueryFilter) (CastHit, bool) {
	b, err := w.getBody(h)
	if err != nil || !finite2(offset) || !finite2(delta) {
		return CastHit{}, false
	}
	f.ExcludeBody = h
	xf := xform{p: b.xf.p.Add(offset), q: b.xf.q}
	var best CastHit
	found := false
	for _, sh := range b.shapes {
		s := &w.shapes.items[sh.Index]
		if s.dead || s.trigger || s.g.kind == KindChain {
			continue
		}
		c := s.g.c.transformed(xf)
		if hit, ok := w.castProbe(&c, delta, f); ok && (!found || hit.Fraction < best.Fraction) {
			best, found = hit, true
		}
	}
	return best, found
}

func (w *World) Penetration(h BodyHandle, f QueryFilter, out []CastHit) int {
	b, err := w.getBody(h)
	if err != nil || len(out) == 0 {
		return 0
	}
	f.ExcludeBody = h
	q := &w.q
	q.filter, q.penOut, q.penN = f, out, 0
	for _, sh := range b.shapes {
		s := &w.shapes.items[sh.Index]
		if s.dead || s.trigger || s.g.kind == KindChain {
			continue
		}
		c := s.g.c.transformed(b.xf)
		q.castC, q.castA, q.castCentroid = c, proxyOf(&c), c.centroid
		w.tree.query(c.aabb(identityXf()), q.penFn)
		if q.penN == len(out) {
			break
		}
	}
	n := q.penN
	q.penOut = nil
	return n
}
