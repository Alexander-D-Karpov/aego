package gizmo

import (
	"math"

	"aego/core/mathx"
	"aego/render/debug"
	"aego/render/view"
	"aego/scene"
)

type Op3 uint8

const (
	Translate3 Op3 = iota
	Rotate3
	Scale3
)

func (o Op3) Label() string {
	switch o {
	case Rotate3:
		return "Rotate"
	case Scale3:
		return "Scale"
	}
	return "Move"
}

type Part3 uint8

const (
	Part3None Part3 = iota
	Part3X
	Part3Y
	Part3Z
	Part3XY
	Part3YZ
	Part3XZ
	Part3Center
)

func (p Part3) Axis() int {
	switch p {
	case Part3X:
		return 0
	case Part3Y:
		return 1
	case Part3Z:
		return 2
	}
	return -1
}

func (p Part3) planeAxes() (int, int, int, bool) {
	switch p {
	case Part3XY:
		return 0, 1, 2, true
	case Part3YZ:
		return 1, 2, 0, true
	case Part3XZ:
		return 0, 2, 1, true
	}
	return 0, 0, 0, false
}

type Colors3 struct {
	X, Y, Z, Center, Hot mathx.Color
}

type Snap3 struct {
	Enabled bool
	Move    float32
	Angle   float32
	Scale   float32
}

const (
	DefaultSize3 float32 = 90
	pickPx3      float32 = 8
	planeLo      float32 = 0.2
	planeHi      float32 = 0.45
)

var (
	units      = [3]mathx.Vec3{{X: 1}, {Y: 1}, {Z: 1}}
	axisParts  = [3]Part3{Part3X, Part3Y, Part3Z}
	planeParts = [3]Part3{Part3YZ, Part3XZ, Part3XY}
)

func camPos(v view.View) mathx.Vec3 {
	return v.Eye.Translation()
}

func camForward(v view.View) mathx.Vec3 {
	return v.Eye.TransformDir(mathx.Vec3{Z: -1}).Normalized()
}

func camRight(v view.View) mathx.Vec3 {
	return v.Eye.TransformDir(mathx.Vec3{X: 1}).Normalized()
}

func tanHalf(fov float32) float32 {
	if fov <= 0 {
		fov = mathx.Pi / 3
	}
	return float32(math.Tan(float64(fov) / 2))
}

func PxToWorld(v view.View, at mathx.Vec3, px float32) float32 {
	h := v.Viewport.H
	if h <= 0 {
		h = 1
	}
	d := at.Sub(camPos(v)).Dot(camForward(v))
	if d < 1e-3 {
		d = 1e-3
	}
	return px * 2 * d * tanHalf(v.FOV) / h
}

type Frame3 struct {
	View     view.View
	Pivot    mathx.Vec3
	Rotation mathx.Quat
	SizePx   float32
}

func (f Frame3) Length() float32 {
	sz := f.SizePx
	if sz <= 0 {
		sz = DefaultSize3
	}
	return PxToWorld(f.View, f.Pivot, sz)
}

func (f Frame3) Axis(i int) mathx.Vec3 {
	r := f.Rotation
	if r.IsZero() {
		r = mathx.QuatIdentity()
	}
	return r.Rotate(units[i]).Normalized()
}

type DrawCtx3 struct {
	Debug  *debug.Draw
	Frame  Frame3
	Colors Colors3
	Hot    Part3
	Active Part3
}

func (c *DrawCtx3) PxToWorld(at mathx.Vec3, px float32) float32 {
	return PxToWorld(c.Frame.View, at, px)
}

func (c *DrawCtx3) Line3(a, b mathx.Vec3, col mathx.Color) {
	c.Debug.Line3(a, b, col)
}

func (c *DrawCtx3) Circle3(center, normal mathx.Vec3, rPx float32, col mathx.Color) {
	c.Debug.Circle3(center, normal, c.PxToWorld(center, rPx), 48, col)
}

func (c *DrawCtx3) Handle3(world mathx.Vec3, sizePx float32, col mathx.Color) {
	h := c.PxToWorld(world, sizePx) * 0.5
	e := mathx.Vec3{X: h, Y: h, Z: h}
	c.Debug.AABB3(mathx.AABB3{Min: world.Sub(e), Max: world.Add(e)}, col)
}

func (c *DrawCtx3) tint(p Part3, base mathx.Color) mathx.Color {
	if c.Active != Part3None {
		if p == c.Active {
			return c.Colors.Hot
		}
		return base
	}
	if p == c.Hot {
		return c.Colors.Hot
	}
	return base
}

func (c *DrawCtx3) axisColor(i int) mathx.Color {
	switch i {
	case 0:
		return c.Colors.X
	case 1:
		return c.Colors.Y
	}
	return c.Colors.Z
}

func perpendicular(v mathx.Vec3) mathx.Vec3 {
	if mathx.Abs(v.X) < 0.9 {
		return v.Cross(mathx.Vec3{X: 1}).Normalized()
	}
	return v.Cross(mathx.Vec3{Y: 1}).Normalized()
}

func (c *DrawCtx3) arrow(tip, dir mathx.Vec3, size float32, col mathx.Color) {
	a := perpendicular(dir)
	b := dir.Cross(a)
	base := tip.Sub(dir.Scale(size))
	for _, s := range [4]mathx.Vec3{a, a.Neg(), b, b.Neg()} {
		c.Line3(tip, base.Add(s.Scale(size*0.35)), col)
	}
}

func (c *DrawCtx3) square(o, u, w mathx.Vec3, l float32, col mathx.Color) {
	at := func(x, y float32) mathx.Vec3 {
		return o.Add(u.Scale(l * x)).Add(w.Scale(l * y))
	}
	p0, p1, p2, p3 := at(planeLo, planeLo), at(planeHi, planeLo), at(planeHi, planeHi), at(planeLo, planeHi)
	c.Line3(p0, p1, col)
	c.Line3(p1, p2, col)
	c.Line3(p2, p3, col)
	c.Line3(p3, p0, col)
}

func Draw3(op Op3, c *DrawCtx3) {
	f := c.Frame
	o := f.Pivot
	l := f.Length()
	for i := 0; i < 3; i++ {
		a := f.Axis(i)
		col := c.tint(axisParts[i], c.axisColor(i))
		end := o.Add(a.Scale(l))
		switch op {
		case Rotate3:
			c.Debug.Circle3(o, a, l, 64, col)
		case Scale3:
			c.Line3(o, end, col)
			c.Handle3(end, 9, col)
		default:
			c.Line3(o, end, col)
			c.arrow(end, a, l*0.14, col)
		}
	}
	if op == Translate3 {
		for n := 0; n < 3; n++ {
			c.square(o, f.Axis((n+1)%3), f.Axis((n+2)%3), l, c.tint(planeParts[n], c.axisColor(n)))
		}
	}
	if op != Rotate3 {
		c.Handle3(o, 10, c.tint(Part3Center, c.Colors.Center))
	}
}

func rayPlane(r mathx.Ray3, point, normal mathx.Vec3) (float32, bool) {
	den := normal.Dot(r.Dir)
	if mathx.Abs(den) < 1e-6 {
		return 0, false
	}
	t := normal.Dot(point.Sub(r.Origin)) / den
	return t, t >= 0
}

func rayPointDist(r mathx.Ray3, p mathx.Vec3) (float32, float32) {
	dd := r.Dir.Dot(r.Dir)
	if dd == 0 {
		return 0, p.Sub(r.Origin).Len()
	}
	t := p.Sub(r.Origin).Dot(r.Dir) / dd
	if t < 0 {
		t = 0
	}
	return t, r.At(t).Sub(p).Len()
}

func closestOnLine(r mathx.Ray3, p, a mathx.Vec3) (float32, float32, bool) {
	w := p.Sub(r.Origin)
	aa, ab, dd := a.Dot(a), a.Dot(r.Dir), r.Dir.Dot(r.Dir)
	aw, dw := a.Dot(w), r.Dir.Dot(w)
	den := aa*dd - ab*ab
	if mathx.Abs(den) < 1e-9*aa*dd || aa == 0 {
		return 0, 0, false
	}
	return (ab*dw - dd*aw) / den, (aa*dw - ab*aw) / den, true
}

func raySegment(r mathx.Ray3, a, b mathx.Vec3) (float32, float32) {
	seg := b.Sub(a)
	s, _, ok := closestOnLine(r, a, seg)
	if !ok {
		s = 0
	}
	return rayPointDist(r, a.Add(seg.Scale(mathx.Clamp(s, 0, 1))))
}

func inBand(v, l float32) bool {
	return v >= l*planeLo && v <= l*planeHi
}

func Hit3(op Op3, f Frame3, ray mathx.Ray3) Part3 {
	o := f.Pivot
	l := f.Length()
	tol := PxToWorld(f.View, o, pickPx3)
	if op != Rotate3 {
		if _, d := rayPointDist(ray, o); d < tol*1.5 {
			return Part3Center
		}
	}
	best, bestT := Part3None, float32(math.MaxFloat32)
	try := func(p Part3, t float32) {
		if t >= 0 && t < bestT {
			best, bestT = p, t
		}
	}
	for i := 0; i < 3; i++ {
		a := f.Axis(i)
		if op == Rotate3 {
			if t, ok := rayPlane(ray, o, a); ok && mathx.Abs(ray.At(t).Sub(o).Len()-l) < tol {
				try(axisParts[i], t)
			}
			continue
		}
		if t, d := raySegment(ray, o, o.Add(a.Scale(l))); d < tol {
			try(axisParts[i], t)
		}
	}
	if op == Translate3 {
		for n := 0; n < 3; n++ {
			t, ok := rayPlane(ray, o, f.Axis(n))
			if !ok {
				continue
			}
			p := ray.At(t).Sub(o)
			if inBand(p.Dot(f.Axis((n+1)%3)), l) && inBand(p.Dot(f.Axis((n+2)%3)), l) {
				try(planeParts[n], t)
			}
		}
	}
	return best
}

type Delta3 struct {
	Move   mathx.Vec3
	Rotate mathx.Quat
	Scale  mathx.Vec3
}

func IdentityDelta3() Delta3 {
	return Delta3{Rotate: mathx.QuatIdentity(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}
}

type Drag3 struct {
	Op     Op3
	Part   Part3
	Pivot  mathx.Vec3
	axis   mathx.Vec3
	normal mathx.Vec3
	right  mathx.Vec3
	start  mathx.Vec3
	param  float32
	length float32
}

func BeginDrag3(op Op3, part Part3, f Frame3, ray mathx.Ray3) (Drag3, bool) {
	d := Drag3{Op: op, Part: part, Pivot: f.Pivot, right: camRight(f.View), length: f.Length()}
	if part == Part3None {
		return d, false
	}
	if i := part.Axis(); i >= 0 {
		if op == Rotate3 {
			d.normal = f.Axis(i)
			return d.grab(ray)
		}
		d.axis = f.Axis(i)
		s, _, ok := closestOnLine(ray, d.Pivot, d.axis)
		if !ok {
			return d, false
		}
		d.param = s
		return d, op != Scale3 || mathx.Abs(s) > 1e-5
	}
	if op == Rotate3 {
		return d, false
	}
	if part == Part3Center {
		d.normal = camForward(f.View)
	} else {
		_, _, n, _ := part.planeAxes()
		d.normal = f.Axis(n)
	}
	return d.grab(ray)
}

func (d Drag3) grab(ray mathx.Ray3) (Drag3, bool) {
	t, ok := rayPlane(ray, d.Pivot, d.normal)
	if !ok {
		return d, false
	}
	d.start = ray.At(t)
	return d, true
}

func snapValue(v, step float32) float32 {
	if step <= 0 {
		return v
	}
	return float32(math.Round(float64(v/step))) * step
}

func snapScale(f float32, s Snap3) float32 {
	if s.Enabled {
		f = 1 + snapValue(f-1, s.Scale)
	}
	if f < 0.001 {
		f = 0.001
	}
	return f
}

func (d *Drag3) Update(ray mathx.Ray3, snap Snap3) (Delta3, bool) {
	out := IdentityDelta3()
	if d.Op == Rotate3 {
		t, ok := rayPlane(ray, d.Pivot, d.normal)
		if !ok {
			return out, false
		}
		v0, v1 := d.start.Sub(d.Pivot), ray.At(t).Sub(d.Pivot)
		ang := mathx.Atan2(d.normal.Dot(v0.Cross(v1)), v0.Dot(v1))
		if snap.Enabled {
			ang = snapValue(ang, mathx.Deg2Rad(snap.Angle))
		}
		out.Rotate = mathx.QuatAxisAngle(d.normal, ang)
		return out, true
	}
	if i := d.Part.Axis(); i >= 0 {
		s, _, ok := closestOnLine(ray, d.Pivot, d.axis)
		if !ok {
			return out, false
		}
		if d.Op == Scale3 {
			f := snapScale(s/d.param, snap)
			switch i {
			case 0:
				out.Scale.X = f
			case 1:
				out.Scale.Y = f
			default:
				out.Scale.Z = f
			}
			return out, true
		}
		m := s - d.param
		if snap.Enabled {
			m = snapValue(m, snap.Move)
		}
		out.Move = d.axis.Scale(m)
		return out, true
	}
	t, ok := rayPlane(ray, d.Pivot, d.normal)
	if !ok {
		return out, false
	}
	m := ray.At(t).Sub(d.start)
	if d.Op == Scale3 {
		f := snapScale(1+m.Dot(d.right)/d.length, snap)
		out.Scale = mathx.Vec3{X: f, Y: f, Z: f}
		return out, true
	}
	if snap.Enabled {
		m = mathx.Vec3{X: snapValue(m.X, snap.Move), Y: snapValue(m.Y, snap.Move), Z: snapValue(m.Z, snap.Move)}
	}
	out.Move = m
	return out, true
}

func identityRot(q mathx.Quat) bool {
	return q.IsZero() || (mathx.Abs(q.X) < 1e-7 && mathx.Abs(q.Y) < 1e-7 && mathx.Abs(q.Z) < 1e-7)
}

func ApplyDelta3(parent mathx.Mat4, local scene.Transform3D, pivot mathx.Vec3, d Delta3) scene.Transform3D {
	out := local
	if out.Rotation.IsZero() {
		out.Rotation = mathx.QuatIdentity()
	}
	if out.Scale == (mathx.Vec3{}) {
		out.Scale = mathx.Vec3{X: 1, Y: 1, Z: 1}
	}
	inv, ok := parent.InverseAffine()
	if !ok {
		return local
	}
	if d.Move != (mathx.Vec3{}) {
		out.Position = out.Position.Add(inv.TransformDir(d.Move))
	}
	if !identityRot(d.Rotate) {
		if _, pr, _, ok := parent.Decompose(); ok {
			world := parent.TransformPoint(out.Position)
			out.Position = inv.TransformPoint(pivot.Add(d.Rotate.Rotate(world.Sub(pivot))))
			out.Rotation = pr.Inverse().Mul(d.Rotate).Mul(pr).Mul(out.Rotation).Normalized()
		}
	}
	if d.Scale != (mathx.Vec3{}) {
		out.Scale = mathx.Vec3{X: out.Scale.X * d.Scale.X, Y: out.Scale.Y * d.Scale.Y, Z: out.Scale.Z * d.Scale.Z}
	}
	return out
}
