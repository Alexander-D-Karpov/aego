package world

import (
	"aego/core/mathx"
)

const (
	maxPolyVerts   = 8
	maxChainPoints = 4096
)

type GeometryKind uint8

const (
	KindCircle GeometryKind = iota
	KindBox
	KindCapsule
	KindPolygon
	KindChain
)

func (k GeometryKind) String() string {
	switch k {
	case KindCircle:
		return "circle"
	case KindBox:
		return "box"
	case KindCapsule:
		return "capsule"
	case KindPolygon:
		return "polygon"
	case KindChain:
		return "chain"
	}
	return "unknown"
}

type Geometry interface {
	Kind() GeometryKind
}

type Circle struct {
	R float32
}

type Box struct {
	Half Vec2
}

type Capsule struct {
	HalfLen float32
	R       float32
}

type Polygon struct {
	Pts [maxPolyVerts]Vec2
	N   int
}

type Chain struct {
	Pts       []Vec2
	Loop      bool
	GhostPrev Vec2
	GhostNext Vec2
}

func (Circle) Kind() GeometryKind  { return KindCircle }
func (Box) Kind() GeometryKind     { return KindBox }
func (Capsule) Kind() GeometryKind { return KindCapsule }
func (Polygon) Kind() GeometryKind { return KindPolygon }
func (Chain) Kind() GeometryKind   { return KindChain }

type convex struct {
	v        [maxPolyVerts]Vec2
	n        [maxPolyVerts]Vec2
	count    int
	radius   float32
	centroid Vec2
}

func (c *convex) computeNormals() {
	if c.count == 2 {
		e, _ := normalizeLen(c.v[1].Sub(c.v[0]))
		c.n[0] = rightPerp(e)
		c.n[1] = c.n[0].Neg()
		return
	}
	for i := 0; i < c.count; i++ {
		j := i + 1
		if j == c.count {
			j = 0
		}
		e, _ := normalizeLen(c.v[j].Sub(c.v[i]))
		c.n[i] = rightPerp(e)
	}
}

func (c *convex) transformed(x xform) convex {
	out := convex{count: c.count, radius: c.radius, centroid: x.apply(c.centroid)}
	for i := 0; i < c.count; i++ {
		out.v[i] = x.apply(c.v[i])
		out.n[i] = x.q.apply(c.n[i])
	}
	return out
}

func (c *convex) aabb(x xform) AABB {
	lo := x.apply(c.v[0])
	hi := lo
	for i := 1; i < c.count; i++ {
		p := x.apply(c.v[i])
		lo, hi = min2(lo, p), max2(hi, p)
	}
	return boxExpand(AABB{Min: lo, Max: hi}, c.radius)
}

type chainData struct {
	pts       []Vec2
	loop      bool
	ghostPrev Vec2
	ghostNext Vec2
}

func (c *chainData) segments() int {
	if c.loop {
		return len(c.pts)
	}
	return len(c.pts) - 1
}

func (c *chainData) point(i int) Vec2 {
	n := len(c.pts)
	return c.pts[((i%n)+n)%n]
}

func (c *chainData) segment(i int) (convex, Vec2, Vec2) {
	v1, v2 := c.point(i), c.point(i+1)
	var g0, g3 Vec2
	switch {
	case c.loop:
		g0, g3 = c.point(i-1), c.point(i+2)
	default:
		if i == 0 {
			g0 = c.ghostPrev
		} else {
			g0 = c.pts[i-1]
		}
		if i+2 < len(c.pts) {
			g3 = c.pts[i+2]
		} else {
			g3 = c.ghostNext
		}
	}
	s := convex{count: 2, centroid: v1.Add(v2).Scale(0.5)}
	s.v[0], s.v[1] = v1, v2
	s.computeNormals()
	return s, g0, g3
}

type geom struct {
	kind  GeometryKind
	c     convex
	chain *chainData
}

func (g *geom) children() int {
	if g.kind == KindChain {
		return g.chain.segments()
	}
	return 1
}

func (g *geom) child(i int) convex {
	if g.kind == KindChain {
		s, _, _ := g.chain.segment(i)
		return s
	}
	return g.c
}

func (g *geom) childAABB(i int, x xform) AABB {
	if g.kind == KindChain {
		s, _, _ := g.chain.segment(i)
		return s.aabb(x)
	}
	return g.c.aabb(x)
}

func validTransform(t Transform) error {
	if !finite2(t.Position) || !finite(t.Angle) {
		return bad("local transform is not finite")
	}
	return nil
}

func buildGeom(g Geometry, local Transform, slop float32) (geom, error) {
	if err := validTransform(local); err != nil {
		return geom{}, err
	}
	x := local.xf()
	switch s := g.(type) {
	case Circle:
		if !finite(s.R) || s.R <= 0 {
			return geom{}, bad("circle radius %v must be positive", s.R)
		}
		out := geom{kind: KindCircle, c: convex{count: 1, radius: s.R}}
		out.c.v[0] = x.p
		out.c.centroid = x.p
		return out, nil
	case Box:
		if !finite2(s.Half) || s.Half.X <= slop || s.Half.Y <= slop {
			return geom{}, bad("box half extents %v must exceed linear slop %v", s.Half, slop)
		}
		hx, hy := s.Half.X, s.Half.Y
		out := geom{kind: KindBox, c: convex{count: 4}}
		out.c.v[0] = x.apply(Vec2{X: -hx, Y: -hy})
		out.c.v[1] = x.apply(Vec2{X: hx, Y: -hy})
		out.c.v[2] = x.apply(Vec2{X: hx, Y: hy})
		out.c.v[3] = x.apply(Vec2{X: -hx, Y: hy})
		out.c.computeNormals()
		out.c.centroid = x.p
		return out, nil
	case Capsule:
		if !finite(s.HalfLen) || !finite(s.R) || s.HalfLen <= slop || s.R <= 0 {
			return geom{}, bad("capsule half length %v and radius %v must be positive", s.HalfLen, s.R)
		}
		out := geom{kind: KindCapsule, c: convex{count: 2, radius: s.R}}
		out.c.v[0] = x.apply(Vec2{X: -s.HalfLen})
		out.c.v[1] = x.apply(Vec2{X: s.HalfLen})
		out.c.computeNormals()
		out.c.centroid = x.p
		return out, nil
	case Polygon:
		return buildPolygon(s, x, slop)
	case Chain:
		return buildChain(s, x, slop)
	case nil:
		return geom{}, bad("shape has no geometry")
	}
	return geom{}, bad("unknown geometry %T", g)
}

func buildPolygon(s Polygon, x xform, slop float32) (geom, error) {
	n := s.N
	if n < 3 || n > maxPolyVerts {
		return geom{}, bad("polygon needs 3..%d points, got %d", maxPolyVerts, n)
	}
	var p [maxPolyVerts]Vec2
	for i := 0; i < n; i++ {
		if !finite2(s.Pts[i]) {
			return geom{}, bad("polygon point %d is not finite", i)
		}
		p[i] = s.Pts[i]
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if p[i].Sub(p[j]).LenSq() < slop*slop {
				return geom{}, bad("polygon points %d and %d are closer than linear slop", i, j)
			}
		}
	}
	var area float32
	for i := 0; i < n; i++ {
		area += p[i].Cross(p[(i+1)%n])
	}
	if mathx.Abs(area)*0.5 < slop*slop {
		return geom{}, bad("polygon is degenerate")
	}
	if area < 0 {
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			p[i], p[j] = p[j], p[i]
		}
	}
	for i := 0; i < n; i++ {
		a, b, c := p[i], p[(i+1)%n], p[(i+2)%n]
		e1, _ := normalizeLen(b.Sub(a))
		e2, _ := normalizeLen(c.Sub(b))
		if e1.Cross(e2) <= 1e-4 {
			return geom{}, bad("polygon is not strictly convex at point %d", (i+1)%n)
		}
	}
	out := geom{kind: KindPolygon, c: convex{count: n}}
	for i := 0; i < n; i++ {
		out.c.v[i] = x.apply(p[i])
	}
	out.c.computeNormals()
	out.c.centroid = polygonMass(&out.c, 1).center
	return out, nil
}

func buildChain(s Chain, x xform, slop float32) (geom, error) {
	n := len(s.Pts)
	switch {
	case n < 2:
		return geom{}, bad("chain needs at least 2 points")
	case s.Loop && n < 3:
		return geom{}, bad("chain loop needs at least 3 points")
	case n > maxChainPoints:
		return geom{}, bad("chain has %d points, the limit is %d", n, maxChainPoints)
	}
	ch := &chainData{pts: make([]Vec2, n), loop: s.Loop}
	for i, p := range s.Pts {
		if !finite2(p) {
			return geom{}, bad("chain point %d is not finite", i)
		}
		ch.pts[i] = x.apply(p)
	}
	segs := ch.segments()
	for i := 0; i < segs; i++ {
		if ch.point(i+1).Sub(ch.point(i)).LenSq() < slop*slop {
			return geom{}, bad("chain segment %d is shorter than linear slop", i)
		}
	}
	if !s.Loop {
		if !finite2(s.GhostPrev) || !finite2(s.GhostNext) {
			return geom{}, bad("chain ghost vertices are not finite")
		}
		gp, gn := x.apply(s.GhostPrev), x.apply(s.GhostNext)
		if s.GhostPrev == (Vec2{}) || gp.Sub(ch.pts[0]).LenSq() < slop*slop {
			gp = ch.pts[0].Add(ch.pts[0].Sub(ch.pts[1]))
		}
		if s.GhostNext == (Vec2{}) || gn.Sub(ch.pts[n-1]).LenSq() < slop*slop {
			gn = ch.pts[n-1].Add(ch.pts[n-1].Sub(ch.pts[n-2]))
		}
		ch.ghostPrev, ch.ghostNext = gp, gn
	}
	out := geom{kind: KindChain, chain: ch}
	var c Vec2
	for _, p := range ch.pts {
		c = c.Add(p)
	}
	out.c.centroid = c.Scale(1 / float32(n))
	return out, nil
}

type massData struct {
	mass    float32
	center  Vec2
	inertia float32
}

func polygonMass(c *convex, density float32) massData {
	s := c.v[0]
	var area, inertia float32
	var center Vec2
	const inv3 = float32(1.0 / 3.0)
	for i := 0; i < c.count; i++ {
		e1 := c.v[i].Sub(s)
		e2 := c.v[(i+1)%c.count].Sub(s)
		d := e1.Cross(e2)
		tri := 0.5 * d
		area += tri
		center = center.Add(e1.Add(e2).Scale(tri * inv3))
		intx2 := e1.X*e1.X + e2.X*e1.X + e2.X*e2.X
		inty2 := e1.Y*e1.Y + e2.Y*e1.Y + e2.Y*e2.Y
		inertia += (0.25 * inv3 * d) * (intx2 + inty2)
	}
	if area <= 0 {
		return massData{center: s}
	}
	center = center.Scale(1 / area)
	mass := density * area
	return massData{mass: mass, center: center.Add(s), inertia: density*inertia - mass*center.Dot(center)}
}

func (g *geom) mass(density float32) massData {
	switch g.kind {
	case KindCircle:
		r := g.c.radius
		m := density * mathx.Pi * r * r
		return massData{mass: m, center: g.c.v[0], inertia: 0.5 * m * r * r}
	case KindCapsule:
		r := g.c.radius
		rr := r * r
		p1, p2 := g.c.v[0], g.c.v[1]
		length := p2.Sub(p1).Len()
		circle := density * mathx.Pi * rr
		box := density * 2 * r * length
		lc := 4 * r / (3 * mathx.Pi)
		h := 0.5 * length
		return massData{
			mass:    circle + box,
			center:  p1.Lerp(p2, 0.5),
			inertia: circle*(0.5*rr+h*h+2*h*lc) + box*(4*rr+length*length)/12,
		}
	case KindBox, KindPolygon:
		return polygonMass(&g.c, density)
	}
	return massData{center: g.c.centroid}
}
