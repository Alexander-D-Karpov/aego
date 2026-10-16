package world

const (
	maxManifoldPoints = 2
	faceTolerance     = 0.0005
	cornerTolerance   = 0.01
)

type ManifoldPoint struct {
	Point              Vec2
	AnchorA            Vec2
	AnchorB            Vec2
	Separation         float32
	NormalImpulse      float32
	TangentImpulse     float32
	TotalNormalImpulse float32
	ID                 uint16
	Persisted          bool
}

type Manifold struct {
	Normal Vec2
	Points [maxManifoldPoints]ManifoldPoint
	Count  int
}

func (m *Manifold) minSeparation() float32 {
	s := float32(1e30)
	for i := 0; i < m.Count; i++ {
		s = min(s, m.Points[i].Separation)
	}
	return s
}

func makeID(a, b int) uint16 {
	return uint16(a&0xff)<<8 | uint16(b&0xff)
}

func swapID(id uint16) uint16 {
	return id>>8 | id<<8
}

func collideConvex(a *convex, xfA xform, b *convex, xfB xform, spec float32, m *Manifold) {
	m.Count = 0
	if a.count == 1 && b.count > 1 {
		collideConvex(b, xfB, a, xfA, spec, m)
		m.Normal = m.Normal.Neg()
		for i := 0; i < m.Count; i++ {
			m.Points[i].ID = swapID(m.Points[i].ID)
		}
		return
	}
	xf := invMulXf(xfA, xfB)
	lb := b.transformed(xf)
	switch {
	case a.count == 1:
		collideCircles(a, &lb, spec, m)
	case b.count == 1:
		collidePolyCircle(a, &lb, spec, m)
	default:
		collidePolys(a, &lb, spec, m)
	}
	for i := 0; i < m.Count; i++ {
		m.Points[i].Point = xfA.apply(m.Points[i].Point)
	}
	m.Normal = xfA.q.apply(m.Normal)
}

func collideCircles(a, b *convex, spec float32, m *Manifold) {
	pA, pB := a.v[0], b.v[0]
	n, dist := normalizeLen(pB.Sub(pA))
	sep := dist - a.radius - b.radius
	if sep > spec {
		return
	}
	if dist == 0 {
		n = Vec2{X: 1}
	}
	cA := pA.Add(n.Scale(a.radius))
	cB := pB.Sub(n.Scale(b.radius))
	m.Normal = n
	m.Points[0] = ManifoldPoint{Point: cA.Lerp(cB, 0.5), Separation: sep}
	m.Count = 1
}

func collidePolyCircle(a, b *convex, spec float32, m *Manifold) {
	c := b.v[0]
	rA, rB := a.radius, b.radius
	idx := 0
	sep := float32(-3.4e38)
	for i := 0; i < a.count; i++ {
		s := a.n[i].Dot(c.Sub(a.v[i]))
		if s > sep {
			sep, idx = s, i
		}
	}
	if sep-rA-rB > spec {
		return
	}
	j := idx + 1
	if j == a.count {
		j = 0
	}
	v1, v2 := a.v[idx], a.v[j]
	u1 := c.Sub(v1).Dot(v2.Sub(v1))
	u2 := c.Sub(v2).Dot(v1.Sub(v2))
	vertex := func(v Vec2, id int) {
		n, d := normalizeLen(c.Sub(v))
		if d == 0 {
			n = a.n[idx]
		}
		s := d - rA - rB
		if s > spec {
			return
		}
		cA := v.Add(n.Scale(rA))
		cB := c.Sub(n.Scale(rB))
		m.Normal = n
		m.Points[0] = ManifoldPoint{Point: cA.Lerp(cB, 0.5), Separation: s, ID: makeID(id, 0)}
		m.Count = 1
	}
	switch {
	case u1 < 0 && sep > fltEpsilon:
		vertex(v1, idx)
	case u2 < 0 && sep > fltEpsilon:
		vertex(v2, j)
	default:
		n := a.n[idx]
		cA := c.Add(n.Scale(rA - c.Sub(v1).Dot(n)))
		cB := c.Sub(n.Scale(rB))
		s := cB.Sub(cA).Dot(n)
		if s > spec {
			return
		}
		m.Normal = n
		m.Points[0] = ManifoldPoint{Point: cA.Lerp(cB, 0.5), Separation: s, ID: makeID(idx, 0)}
		m.Count = 1
	}
}

func findMaxSeparation(p1, p2 *convex) (int, float32) {
	best := 0
	maxSep := float32(-3.4e38)
	for i := 0; i < p1.count; i++ {
		n, v1 := p1.n[i], p1.v[i]
		si := float32(3.4e38)
		for j := 0; j < p2.count; j++ {
			si = min(si, n.Dot(p2.v[j].Sub(v1)))
		}
		if si > maxSep {
			maxSep, best = si, i
		}
	}
	return best, maxSep
}

func incident(ref Vec2, p *convex) int {
	best := 0
	minDot := float32(3.4e38)
	for i := 0; i < p.count; i++ {
		if d := ref.Dot(p.n[i]); d < minDot {
			minDot, best = d, i
		}
	}
	return best
}

func next(i, count int) int {
	if i+1 < count {
		return i + 1
	}
	return 0
}

type segDist struct {
	f1, f2     float32
	c1, c2     Vec2
	distanceSq float32
}

func segmentDistance(p1, q1, p2, q2 Vec2) segDist {
	var r segDist
	d1, d2 := q1.Sub(p1), q2.Sub(p2)
	w := p1.Sub(p2)
	dd1, dd2 := d1.Dot(d1), d2.Dot(d2)
	rd1, rd2 := w.Dot(d1), w.Dot(d2)
	const epsSq = fltEpsilon * fltEpsilon
	switch {
	case dd1 < epsSq || dd2 < epsSq:
		switch {
		case dd1 >= epsSq:
			r.f1 = clampf(-rd1/dd1, 0, 1)
		case dd2 >= epsSq:
			r.f2 = clampf(rd2/dd2, 0, 1)
		}
	default:
		d12 := d1.Dot(d2)
		denom := dd1*dd2 - d12*d12
		var f1 float32
		if denom != 0 {
			f1 = clampf((d12*rd2-rd1*dd2)/denom, 0, 1)
		}
		f2 := (d12*f1 + rd2) / dd2
		switch {
		case f2 < 0:
			f2 = 0
			f1 = clampf(-rd1/dd1, 0, 1)
		case f2 > 1:
			f2 = 1
			f1 = clampf((d12-rd1)/dd1, 0, 1)
		}
		r.f1, r.f2 = f1, f2
	}
	r.c1 = p1.Add(d1.Scale(r.f1))
	r.c2 = p2.Add(d2.Scale(r.f2))
	r.distanceSq = r.c1.Sub(r.c2).LenSq()
	return r
}

func collidePolys(a, b *convex, spec float32, m *Manifold) {
	edgeA, sepA := findMaxSeparation(a, b)
	edgeB, sepB := findMaxSeparation(b, a)
	radius := a.radius + b.radius
	if sepA > spec+radius || sepB > spec+radius {
		return
	}
	const linearSlop = 0.005
	flip := sepB > 0.1*linearSlop+sepA
	if flip {
		edgeA = incident(b.n[edgeB], a)
	} else {
		edgeB = incident(a.n[edgeA], b)
	}
	if (!flip && sepA > 0.1*linearSlop) || (flip && sepB > 0.1*linearSlop) {
		i11, i12 := edgeA, next(edgeA, a.count)
		i21, i22 := edgeB, next(edgeB, b.count)
		v11, v12 := a.v[i11], a.v[i12]
		v21, v22 := b.v[i21], b.v[i22]
		r := segmentDistance(v11, v12, v21, v22)
		vertex := func(pa, pb Vec2, ia, ib int) {
			n, dist := normalizeLen(pb.Sub(pa))
			if dist == 0 || dist > spec+radius {
				return
			}
			c1 := pa.Add(n.Scale(a.radius))
			c2 := pb.Sub(n.Scale(b.radius))
			m.Normal = n
			m.Points[0] = ManifoldPoint{Point: c1.Lerp(c2, 0.5), Separation: dist - radius, ID: makeID(ia, ib)}
			m.Count = 1
		}
		switch {
		case edgesOverlap(v11, v12, v21, v22, linearSlop):
		case r.f1 == 0 && r.f2 == 0:
			vertex(v11, v21, i11, i21)
			return
		case r.f1 == 0 && r.f2 == 1:
			vertex(v11, v22, i11, i22)
			return
		case r.f1 == 1 && r.f2 == 0:
			vertex(v12, v21, i12, i21)
			return
		case r.f1 == 1 && r.f2 == 1:
			vertex(v12, v22, i12, i22)
			return
		}
	}
	clipPolygons(a, b, edgeA, edgeB, flip, spec, m)
}

func edgesOverlap(p1, q1, p2, q2 Vec2, slop float32) bool {
	t, l := normalizeLen(q1.Sub(p1))
	e2 := q2.Sub(p2)
	if l == 0 || abs32(t.Cross(e2)) > 1e-4*e2.Len() {
		return false
	}
	a, b := p2.Sub(p1).Dot(t), q2.Sub(p1).Dot(t)
	return min(max(a, b), l)-max(min(a, b), 0) > slop
}

func clipPolygons(a, b *convex, edgeA, edgeB int, flip bool, spec float32, m *Manifold) {
	p1, p2 := a, b
	i11, i21 := edgeA, edgeB
	if flip {
		p1, p2 = b, a
		i11, i21 = edgeB, edgeA
	}
	i12, i22 := next(i11, p1.count), next(i21, p2.count)
	normal := p1.n[i11]
	v11, v12 := p1.v[i11], p1.v[i12]
	v21, v22 := p2.v[i21], p2.v[i22]
	tangent := crossSV(1, normal)
	lower1 := float32(0)
	upper1 := v12.Sub(v11).Dot(tangent)
	upper2 := v21.Sub(v11).Dot(tangent)
	lower2 := v22.Sub(v11).Dot(tangent)
	vLower, vUpper := v22, v21
	if lower2 < lower1 && upper2-lower2 > fltEpsilon {
		vLower = v22.Lerp(v21, (lower1-lower2)/(upper2-lower2))
	}
	if upper2 > upper1 && upper2-lower2 > fltEpsilon {
		vUpper = v22.Lerp(v21, (upper1-lower2)/(upper2-lower2))
	}
	sepLower := vLower.Sub(v11).Dot(normal)
	sepUpper := vUpper.Sub(v11).Dot(normal)
	r1, r2 := p1.radius, p2.radius
	vLower = vLower.Add(normal.Scale(0.5 * (r1 - r2 - sepLower)))
	vUpper = vUpper.Add(normal.Scale(0.5 * (r1 - r2 - sepUpper)))
	radius := r1 + r2
	add := func(p Vec2, s float32, id uint16) {
		if s <= spec {
			m.Points[m.Count] = ManifoldPoint{Point: p, Separation: s, ID: id}
			m.Count++
		}
	}
	if !flip {
		m.Normal = normal
		add(vLower, sepLower-radius, makeID(i11, i22))
		add(vUpper, sepUpper-radius, makeID(i12, i21))
		return
	}
	m.Normal = normal.Neg()
	add(vUpper, sepUpper-radius, makeID(i21, i12))
	add(vLower, sepLower-radius, makeID(i22, i11))
}

func collideChainSegment(seg *convex, g0, g3 Vec2, oneSided bool, xfA xform, b *convex, xfB xform, spec float32, m *Manifold) {
	collideConvex(seg, xfA, b, xfB, spec, m)
	if m.Count == 0 {
		return
	}
	v1, v2 := seg.v[0], seg.v[1]
	e, _ := normalizeLen(v2.Sub(v1))
	n := rightPerp(e)
	nl := xfA.q.invApply(m.Normal)
	if oneSided {
		cb := xfA.invApply(xfB.apply(b.centroid))
		if cb.Sub(v1).Dot(n) < 0 {
			m.Count = 0
			return
		}
	}
	s := float32(1)
	if !oneSided && nl.Dot(n) < 0 {
		s, n = -1, n.Neg()
	}
	if nl.Dot(n) >= 1-faceTolerance {
		return
	}
	snap := oneSided && nl.Dot(n) < 0
	if !snap {
		var adj Vec2
		var convexCorner bool
		if nl.Dot(e) < 0 {
			e0, l := normalizeLen(v1.Sub(g0))
			convexCorner = l > 0 && s*e0.Cross(e) > cornerTolerance
			adj = rightPerp(e0).Scale(s)
		} else {
			e2, l := normalizeLen(g3.Sub(v2))
			convexCorner = l > 0 && s*e.Cross(e2) > cornerTolerance
			adj = rightPerp(e2).Scale(s)
		}
		if convexCorner {
			span := adj.Cross(n)
			if adj.Cross(nl)*span < 0 || nl.Cross(n)*span < 0 {
				m.Count = 0
			}
			return
		}
	}
	nw := xfA.q.apply(n)
	k := 0
	for i := 0; i < m.Count; i++ {
		p := m.Points[i]
		cb := p.Point.Add(m.Normal.Scale(0.5 * p.Separation))
		sep := xfA.invApply(cb).Sub(v1).Dot(n)
		if sep > spec {
			continue
		}
		p.Separation = sep
		p.Point = cb.Sub(nw.Scale(0.5 * sep))
		m.Points[k] = p
		k++
	}
	m.Count = k
	m.Normal = nw
}
