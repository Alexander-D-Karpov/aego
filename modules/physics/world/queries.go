package world

type queryState struct {
	w       *World
	filter  QueryFilter
	origin  Vec2
	dir     Vec2
	maxDist float32
	best    RayHit
	found   bool
	hits    []RayHit
	n       int
	shapes  []ShapeHandle
	point   Vec2
	probe   convex
	probeXf xform
	pick    ShapeHandle
	picked  bool
	rayFn   func(int32, Vec2, Vec2, float32) float32
	allFn   func(int32, Vec2, Vec2, float32) float32
	pointFn func(int32) bool
	shapeFn func(int32) bool
	atFn    func(int32) bool
}

func (q *queryState) init(w *World) {
	q.w = w
	q.rayFn = q.rayClosest
	q.allFn = q.rayAll
	q.pointFn = q.pointHit
	q.shapeFn = q.shapeHit
	q.atFn = q.atHit
}

func (q *queryState) accept(s *shape) bool {
	if s.dead {
		return false
	}
	f := &q.filter
	if s.trigger && !f.IncludeTriggers {
		return false
	}
	if !f.ExcludeBody.IsZero() && s.body == f.ExcludeBody {
		return false
	}
	return (f.LayerMask>>q.w.bodies.items[s.body.Index].layer)&1 != 0
}

func (q *queryState) cast(s *shape, child int, maxT float32) (float32, Vec2, bool) {
	b := &q.w.bodies.items[s.body.Index]
	lp := b.xf.invApply(q.origin)
	ld := b.xf.q.invApply(q.dir)
	var t float32
	var n Vec2
	var ok bool
	if s.g.kind == KindChain {
		seg, _, _ := s.g.chain.segment(child)
		t, n, ok = raySegment(seg.v[0], seg.v[1], lp, ld, maxT, s.oneSided)
	} else {
		t, n, ok = rayConvex(&s.g.c, lp, ld, maxT)
	}
	if !ok {
		return 0, Vec2{}, false
	}
	return t, b.xf.q.apply(n), true
}

func (q *queryState) hitAt(t float32, n Vec2, sh ShapeHandle, bh BodyHandle) RayHit {
	return RayHit{
		Point:    q.origin.Add(q.dir.Scale(t)),
		Normal:   n,
		Distance: t,
		Fraction: t / q.maxDist,
		Shape:    sh,
		Body:     bh,
	}
}

func hitLess(a, b RayHit) bool {
	if a.Distance != b.Distance {
		return a.Distance < b.Distance
	}
	return a.Shape.Index < b.Shape.Index
}

func (q *queryState) rayClosest(id int32, _, _ Vec2, maxFraction float32) float32 {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if !q.accept(s) {
		return -1
	}
	t, n, ok := q.cast(s, int(pi.child), maxFraction*q.maxDist)
	if !ok {
		return -1
	}
	hit := q.hitAt(t, n, pi.shape, s.body)
	if q.found && !hitLess(hit, q.best) {
		return q.best.Fraction
	}
	q.best, q.found = hit, true
	return hit.Fraction
}

func (q *queryState) rayAll(id int32, _, _ Vec2, maxFraction float32) float32 {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if !q.accept(s) {
		return -1
	}
	t, n, ok := q.cast(s, int(pi.child), maxFraction*q.maxDist)
	if !ok {
		return -1
	}
	size := len(q.hits)
	hit := q.hitAt(t, n, pi.shape, s.body)
	if q.n == size {
		if !hitLess(hit, q.hits[size-1]) {
			return q.hits[size-1].Fraction
		}
		q.n--
	}
	i := q.n
	for i > 0 && hitLess(hit, q.hits[i-1]) {
		q.hits[i] = q.hits[i-1]
		i--
	}
	q.hits[i] = hit
	q.n++
	if q.n == size {
		return q.hits[size-1].Fraction
	}
	return 1
}

func (q *queryState) add(h ShapeHandle) {
	for _, x := range q.shapes[:q.n] {
		if x == h {
			return
		}
	}
	q.shapes[q.n] = h
	q.n++
}

func (q *queryState) pointHit(id int32) bool {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if s.g.kind == KindChain || !q.accept(s) {
		return true
	}
	b := &q.w.bodies.items[s.body.Index]
	if pointInConvex(&s.g.c, b.xf.invApply(q.point)) {
		q.add(pi.shape)
	}
	return q.n < len(q.shapes)
}

func (q *queryState) shapeHit(id int32) bool {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if !q.accept(s) {
		return true
	}
	b := &q.w.bodies.items[s.body.Index]
	var m Manifold
	g := s.g.child(int(pi.child))
	collideConvex(&g, b.xf, &q.probe, q.probeXf, 0, &m)
	if m.Count > 0 {
		q.add(pi.shape)
	}
	return q.n < len(q.shapes)
}

func (q *queryState) atHit(id int32) bool {
	pi := &q.w.prox[id]
	s := &q.w.shapes.items[pi.shape.Index]
	if s.g.kind == KindChain || !q.accept(s) {
		return true
	}
	if q.picked && pi.shape.Index >= q.pick.Index {
		return true
	}
	b := &q.w.bodies.items[s.body.Index]
	if pointInConvex(&s.g.c, b.xf.invApply(q.point)) {
		q.pick, q.picked = pi.shape, true
	}
	return true
}

func sortHandles(list []ShapeHandle) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j].Index < list[j-1].Index; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func (w *World) Raycast(origin, dir Vec2, maxDist float32, f QueryFilter) (RayHit, bool) {
	d, l := normalizeLen(dir)
	if l == 0 || !finite2(origin) || !finite2(dir) || !finite(maxDist) || maxDist <= 0 {
		return RayHit{}, false
	}
	q := &w.q
	q.filter, q.origin, q.dir, q.maxDist = f, origin, d, maxDist
	q.best, q.found = RayHit{}, false
	w.tree.raycast(origin, origin.Add(d.Scale(maxDist)), 1, q.rayFn)
	return q.best, q.found
}

func (w *World) RaycastAll(origin, dir Vec2, maxDist float32, f QueryFilter, out []RayHit) int {
	d, l := normalizeLen(dir)
	if len(out) == 0 || l == 0 || !finite2(origin) || !finite2(dir) || !finite(maxDist) || maxDist <= 0 {
		return 0
	}
	q := &w.q
	q.filter, q.origin, q.dir, q.maxDist = f, origin, d, maxDist
	q.hits, q.n = out, 0
	w.tree.raycast(origin, origin.Add(d.Scale(maxDist)), 1, q.allFn)
	n := q.n
	q.hits = nil
	return n
}

func (w *World) OverlapPoint(p Vec2, f QueryFilter, out []ShapeHandle) int {
	if len(out) == 0 || !finite2(p) {
		return 0
	}
	q := &w.q
	q.filter, q.point, q.shapes, q.n = f, p, out, 0
	w.tree.query(AABB{Min: p, Max: p}, q.pointFn)
	n := q.n
	q.shapes = nil
	sortHandles(out[:n])
	return n
}

func (w *World) overlapProbe(f QueryFilter, out []ShapeHandle) int {
	q := &w.q
	q.filter, q.shapes, q.n = f, out, 0
	w.tree.query(q.probe.aabb(q.probeXf), q.shapeFn)
	n := q.n
	q.shapes = nil
	sortHandles(out[:n])
	return n
}

func (w *World) OverlapCircle(c Vec2, r float32, f QueryFilter, out []ShapeHandle) int {
	if len(out) == 0 || !finite2(c) || !finite(r) || r <= 0 {
		return 0
	}
	q := &w.q
	q.probe = convex{count: 1, radius: r}
	q.probeXf = xform{p: c, q: Rot{C: 1}}
	return w.overlapProbe(f, out)
}

func (w *World) OverlapBox(c, half Vec2, angle float32, f QueryFilter, out []ShapeHandle) int {
	if len(out) == 0 || !finite2(c) || !finite2(half) || !finite(angle) || half.X <= 0 || half.Y <= 0 {
		return 0
	}
	q := &w.q
	q.probe = convex{count: 4}
	q.probe.v[0] = Vec2{X: -half.X, Y: -half.Y}
	q.probe.v[1] = Vec2{X: half.X, Y: -half.Y}
	q.probe.v[2] = Vec2{X: half.X, Y: half.Y}
	q.probe.v[3] = Vec2{X: -half.X, Y: half.Y}
	q.probe.computeNormals()
	q.probeXf = xform{p: c, q: makeRot(angle)}
	return w.overlapProbe(f, out)
}

func (w *World) ShapeAt(p Vec2, f QueryFilter) (ShapeHandle, bool) {
	if !finite2(p) {
		return ShapeHandle{}, false
	}
	q := &w.q
	q.filter, q.point, q.picked = f, p, false
	w.tree.query(AABB{Min: p, Max: p}, q.atFn)
	return q.pick, q.picked
}

func rayConvex(c *convex, p, d Vec2, maxT float32) (float32, Vec2, bool) {
	switch c.count {
	case 1:
		return rayCircle(c.v[0], c.radius, p, d, maxT)
	case 2:
		if c.radius == 0 {
			return raySegment(c.v[0], c.v[1], p, d, maxT, false)
		}
		return rayCapsule(c.v[0], c.v[1], c.radius, p, d, maxT)
	}
	return rayPolygon(c, p, d, maxT)
}

func rayCircle(center Vec2, r float32, p, d Vec2, maxT float32) (float32, Vec2, bool) {
	s := p.Sub(center)
	b := s.Dot(s) - r*r
	if b < 0 {
		return 0, Vec2{}, false
	}
	c := s.Dot(d)
	sigma := c*c - b
	if sigma < 0 || c > 0 {
		return 0, Vec2{}, false
	}
	t := -c - sqrtf(sigma)
	if t < 0 || t > maxT {
		return 0, Vec2{}, false
	}
	n, _ := normalizeLen(s.Add(d.Scale(t)))
	return t, n, true
}

func raySegment(v1, v2, p, d Vec2, maxT float32, oneSided bool) (float32, Vec2, bool) {
	e := v2.Sub(v1)
	en, l := normalizeLen(e)
	if l == 0 {
		return 0, Vec2{}, false
	}
	n := rightPerp(en)
	num := n.Dot(v1.Sub(p))
	den := n.Dot(d)
	if den == 0 || (oneSided && num > 0) {
		return 0, Vec2{}, false
	}
	t := num / den
	if t < 0 || t > maxT {
		return 0, Vec2{}, false
	}
	q := p.Add(d.Scale(t))
	s := q.Sub(v1).Dot(e) / e.Dot(e)
	if s < 0 || s > 1 {
		return 0, Vec2{}, false
	}
	if num > 0 {
		n = n.Neg()
	}
	return t, n, true
}

func distSqToSegment(p, a, b Vec2) float32 {
	e := b.Sub(a)
	t := float32(0)
	if l := e.LenSq(); l > 0 {
		t = clampf(p.Sub(a).Dot(e)/l, 0, 1)
	}
	return p.Sub(a.Add(e.Scale(t))).LenSq()
}

func rayCapsule(v0, v1 Vec2, r float32, p, d Vec2, maxT float32) (float32, Vec2, bool) {
	if distSqToSegment(p, v0, v1) < r*r {
		return 0, Vec2{}, false
	}
	e, _ := normalizeLen(v1.Sub(v0))
	nn := rightPerp(e).Scale(r)
	best := maxT
	var bn Vec2
	found := false
	try := func(t float32, n Vec2, hit bool) {
		if hit && t <= best {
			best, bn, found = t, n, true
		}
	}
	try(raySegment(v0.Add(nn), v1.Add(nn), p, d, best, true))
	try(raySegment(v1.Sub(nn), v0.Sub(nn), p, d, best, true))
	try(rayCircle(v0, r, p, d, best))
	try(rayCircle(v1, r, p, d, best))
	return best, bn, found
}

func rayPolygon(c *convex, p, d Vec2, maxT float32) (float32, Vec2, bool) {
	lower, upper := float32(0), maxT
	index := -1
	for i := 0; i < c.count; i++ {
		num := c.n[i].Dot(c.v[i].Sub(p))
		den := c.n[i].Dot(d)
		switch {
		case den == 0:
			if num < 0 {
				return 0, Vec2{}, false
			}
		case den < 0 && num < lower*den:
			lower = num / den
			index = i
		case den > 0 && num < upper*den:
			upper = num / den
		}
		if upper < lower {
			return 0, Vec2{}, false
		}
	}
	if index < 0 {
		return 0, Vec2{}, false
	}
	return lower, c.n[index], true
}

func pointInConvex(c *convex, p Vec2) bool {
	switch c.count {
	case 1:
		return p.Sub(c.v[0]).LenSq() <= c.radius*c.radius
	case 2:
		return distSqToSegment(p, c.v[0], c.v[1]) <= c.radius*c.radius
	}
	for i := 0; i < c.count; i++ {
		if c.n[i].Dot(p.Sub(c.v[i])) > c.radius {
			return false
		}
	}
	return true
}
