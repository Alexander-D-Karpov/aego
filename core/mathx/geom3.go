package mathx

type AABB3 struct{ Min, Max Vec3 }

type Ray3 struct{ Origin, Dir Vec3 }

type Plane struct {
	Normal Vec3
	D      float32
}

type Frustum struct {
	Planes [6]Plane
}

func AABB3FromPoints(ps ...Vec3) AABB3 {
	if len(ps) == 0 {
		return AABB3{}
	}
	b := AABB3{Min: ps[0], Max: ps[0]}
	for _, p := range ps[1:] {
		b = b.Expand(p)
	}
	return b
}

func (b AABB3) Center() Vec3 {
	return b.Min.Add(b.Max).Scale(0.5)
}

func (b AABB3) Extents() Vec3 {
	return b.Max.Sub(b.Min).Scale(0.5)
}

func (b AABB3) Size() Vec3 {
	return b.Max.Sub(b.Min)
}

func (b AABB3) Intersects(o AABB3) bool {
	return b.Min.X <= o.Max.X && o.Min.X <= b.Max.X &&
		b.Min.Y <= o.Max.Y && o.Min.Y <= b.Max.Y &&
		b.Min.Z <= o.Max.Z && o.Min.Z <= b.Max.Z
}

func (b AABB3) Contains(p Vec3) bool {
	return p.X >= b.Min.X && p.Y >= b.Min.Y && p.Z >= b.Min.Z &&
		p.X <= b.Max.X && p.Y <= b.Max.Y && p.Z <= b.Max.Z
}

func (b AABB3) Expand(p Vec3) AABB3 {
	return AABB3{Min: Vec3Min(b.Min, p), Max: Vec3Max(b.Max, p)}
}

func (b AABB3) Union(o AABB3) AABB3 {
	return AABB3{Min: Vec3Min(b.Min, o.Min), Max: Vec3Max(b.Max, o.Max)}
}

func (b AABB3) Transformed(m Mat4) AABB3 {
	c := m.TransformPoint(b.Center())
	e := b.Extents()
	r := Vec3{
		X: Abs(m[0])*e.X + Abs(m[4])*e.Y + Abs(m[8])*e.Z,
		Y: Abs(m[1])*e.X + Abs(m[5])*e.Y + Abs(m[9])*e.Z,
		Z: Abs(m[2])*e.X + Abs(m[6])*e.Y + Abs(m[10])*e.Z,
	}
	return AABB3{Min: c.Sub(r), Max: c.Add(r)}
}

func (r Ray3) At(t float32) Vec3 {
	return r.Origin.Add(r.Dir.Scale(t))
}

func (r Ray3) IntersectAABB(b AABB3) (float32, bool) {
	tmin, tmax := -Inf(), Inf()
	o := [3]float32{r.Origin.X, r.Origin.Y, r.Origin.Z}
	d := [3]float32{r.Dir.X, r.Dir.Y, r.Dir.Z}
	lo := [3]float32{b.Min.X, b.Min.Y, b.Min.Z}
	hi := [3]float32{b.Max.X, b.Max.Y, b.Max.Z}
	for i := 0; i < 3; i++ {
		if Abs(d[i]) < 1e-12 {
			if o[i] < lo[i] || o[i] > hi[i] {
				return 0, false
			}
			continue
		}
		inv := 1 / d[i]
		t1, t2 := (lo[i]-o[i])*inv, (hi[i]-o[i])*inv
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tmin, tmax = max(tmin, t1), min(tmax, t2)
		if tmin > tmax {
			return 0, false
		}
	}
	if tmax < 0 {
		return 0, false
	}
	return max(tmin, 0), true
}

func (r Ray3) IntersectPlane(p Plane) (float32, bool) {
	denom := p.Normal.Dot(r.Dir)
	if Abs(denom) < 1e-9 {
		return 0, false
	}
	t := -(p.Normal.Dot(r.Origin) + p.D) / denom
	return t, t >= 0
}

func PlaneFromPointNormal(point, normal Vec3) Plane {
	n := normal.Normalized()
	return Plane{Normal: n, D: -n.Dot(point)}
}

func (p Plane) Distance(v Vec3) float32 {
	return p.Normal.Dot(v) + p.D
}

func planeFromVec4(v Vec4) Plane {
	n := v.XYZ()
	l := n.Len()
	if l == 0 {
		return Plane{}
	}
	return Plane{Normal: n.Scale(1 / l), D: v.W / l}
}

func FrustumFromMat4(m Mat4) Frustum {
	row := func(i int) Vec4 {
		return Vec4{X: m[i], Y: m[4+i], Z: m[8+i], W: m[12+i]}
	}
	r0, r1, r2, r3 := row(0), row(1), row(2), row(3)
	return Frustum{Planes: [6]Plane{
		planeFromVec4(r3.Add(r0)),
		planeFromVec4(r3.Sub(r0)),
		planeFromVec4(r3.Add(r1)),
		planeFromVec4(r3.Sub(r1)),
		planeFromVec4(r3.Add(r2)),
		planeFromVec4(r3.Sub(r2)),
	}}
}

func (f Frustum) ContainsPoint(p Vec3) bool {
	for _, pl := range f.Planes {
		if pl.Distance(p) < 0 {
			return false
		}
	}
	return true
}

func (f Frustum) ContainsAABB(b AABB3) bool {
	for _, pl := range f.Planes {
		v := b.Min
		if pl.Normal.X >= 0 {
			v.X = b.Max.X
		}
		if pl.Normal.Y >= 0 {
			v.Y = b.Max.Y
		}
		if pl.Normal.Z >= 0 {
			v.Z = b.Max.Z
		}
		if pl.Distance(v) < 0 {
			return false
		}
	}
	return true
}
